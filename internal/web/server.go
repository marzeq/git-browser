package web

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"git-browser/internal/repository"

	"github.com/go-git/go-git/v5/plumbing"
)

const commitsPerPage = 100

type Server struct {
	store     *repository.Store
	templates *template.Template
	config    Config
}

type Config struct {
	CloneSSHPrefix   string
	CloneHTTPSPrefix string
}

type cloneLink struct {
	Label string
	URL   string
}

type breadcrumb struct {
	Name string
	URL  string
}

type treeEntryView struct {
	Name        string
	URL         string
	Location    string
	IsDir       bool
	IsSubmodule bool
}

type branchView struct {
	Name    string
	URL     string
	Hash    string
	Current bool
}

type commitView struct {
	Hash    string
	URL     string
	Author  string
	Date    string
	Subject string
}

type pageData struct {
	Title string
}

type indexData struct {
	pageData
	Repos []repoLink
}

type repoLink struct {
	Name string
	URL  string
}

type repoData struct {
	pageData
	RepoName             string
	HomeURL              string
	CloneURLs            []cloneLink
	Revision             string
	RevisionLabel        string
	Empty                bool
	TreeURL              string
	LogURL               string
	BranchesURL          string
	Breadcrumbs          []breadcrumb
	Entries              []treeEntryView
	ReadmeName           string
	Readme               string
	ReadmeIsMD           bool
	MarkdownBaseURL      string
	MarkdownImageBaseURL string
}

type blobData struct {
	pageData
	RepoName             string
	HomeURL              string
	CloneURLs            []cloneLink
	Revision             string
	RevisionLabel        string
	Empty                bool
	TreeURL              string
	LogURL               string
	BranchesURL          string
	Breadcrumbs          []breadcrumb
	FilePath             string
	DirectoryURL         string
	HistoryURL           string
	RawURL               string
	Content              string
	ContentType          string
	IsMarkdown           bool
	MarkdownBaseURL      string
	MarkdownImageBaseURL string
}

type logData struct {
	pageData
	RepoName      string
	HomeURL       string
	CloneURLs     []cloneLink
	Revision      string
	RevisionLabel string
	Empty         bool
	TreeURL       string
	LogURL        string
	BranchesURL   string
	Path          string
	BackURL       string
	Commits       []commitView
	PreviousURL   string
	NextURL       string
	Page          int
}

type branchesData struct {
	pageData
	RepoName      string
	HomeURL       string
	CloneURLs     []cloneLink
	Revision      string
	RevisionLabel string
	Empty         bool
	TreeURL       string
	LogURL        string
	BranchesURL   string
	Branches      []branchView
}

func NewServer(store *repository.Store, config Config) (*Server, error) {
	tmpl, err := parseTemplates()
	if err != nil {
		return nil, err
	}

	return &Server{store: store, templates: tmpl, config: config}, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/static/style.css", s.serveStyle)
	mux.HandleFunc("/static/copy.js", s.serveCopyScript)
	mux.HandleFunc("/", s.route)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/static/") {
			s.store.RefreshAsync(func(err error) {
				log.Printf("refresh repositories: %v", err)
			})
		}
		mux.ServeHTTP(w, r)
	})
}

func (s *Server) route(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" {
		s.index(w, r)
		return
	}

	repoName, rest, ok := s.store.SplitPath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if redirectToCanonicalRepoURL(w, r, repoName) {
		return
	}

	if rest == "" || rest == "/" {
		s.repoHome(w, r, repoName)
		return
	}

	section, tail := shiftPath(rest)
	switch section {
	case "tree":
		s.tree(w, r, repoName, tail)
	case "blob":
		s.blob(w, r, repoName, tail, false)
	case "raw":
		s.raw(w, r, repoName, tail)
	case "log":
		s.log(w, r, repoName, tail)
	case "branches":
		s.branches(w, r, repoName)
	default:
		http.NotFound(w, r)
	}
}

func redirectToCanonicalRepoURL(w http.ResponseWriter, r *http.Request, repoName string) bool {
	canonicalRepoPath := "/" + displayRepoName(repoName)
	if r.URL.Path == canonicalRepoPath || strings.HasPrefix(r.URL.Path, canonicalRepoPath+"/") {
		return false
	}
	dotGitRepoPath := canonicalRepoPath + ".git"
	if r.URL.Path != dotGitRepoPath && !strings.HasPrefix(r.URL.Path, dotGitRepoPath+"/") {
		return false
	}
	suffix := strings.TrimPrefix(r.URL.Path, dotGitRepoPath)

	target := (&url.URL{
		Path:     canonicalRepoPath + suffix,
		RawQuery: r.URL.RawQuery,
	}).String()
	http.Redirect(w, r, target, http.StatusPermanentRedirect)
	return true
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	repos := s.store.List()
	items := make([]repoLink, 0, len(repos))
	for _, repo := range repos {
		items = append(items, repoLink{
			Name: displayRepoName(repo.Name),
			URL:  homeURL(repo.Name),
		})
	}

	data := indexData{
		pageData: pageData{Title: "Repositories"},
		Repos:    items,
	}
	s.render(w, "index.html", data)
}

func (s *Server) repoHome(w http.ResponseWriter, r *http.Request, repoName string) {
	repo, err := s.store.Open(repoName)
	if err != nil {
		s.writeRepoError(w, r, err)
		return
	}

	rev, err := repo.DefaultRevision()
	if err != nil {
		if errors.Is(err, repository.ErrEmptyRepository) {
			s.renderEmptyRepoPage(w, r, repo)
			return
		}
		s.writeError(w, r, http.StatusInternalServerError, err)
		return
	}

	s.renderTreePage(w, r, repo, rev, "")
}

func (s *Server) renderEmptyRepoPage(w http.ResponseWriter, r *http.Request, repo *repository.Repository) {
	displayName := displayRepoName(repo.Name())
	data := repoData{
		pageData:    pageData{Title: displayName},
		RepoName:    displayName,
		HomeURL:     homeURL(repo.Name()),
		CloneURLs:   s.cloneURLs(repo.Name()),
		Empty:       true,
		BranchesURL: branchesURL(repo.Name(), ""),
	}

	s.render(w, "repo.html", data)
}

func (s *Server) tree(w http.ResponseWriter, r *http.Request, repoName, tail string) {
	repo, err := s.store.Open(repoName)
	if err != nil {
		s.writeRepoError(w, r, err)
		return
	}

	rev, treePath, err := splitRevisionAndPath(repo, tail)
	if err != nil {
		s.writeError(w, r, http.StatusNotFound, err)
		return
	}

	s.renderTreePage(w, r, repo, rev, treePath)
}

func (s *Server) renderTreePage(w http.ResponseWriter, r *http.Request, repo *repository.Repository, rev *repository.ResolvedRevision, treePath string) {
	tree, err := repo.Tree(rev.Commit, treePath)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, repository.ErrNotTree) {
			status = http.StatusNotFound
		}
		s.writeError(w, r, status, err)
		return
	}

	entries := repo.TreeEntries(tree, treePath)
	submoduleLocations, err := repo.SubmoduleLocations(rev.Commit)
	if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, err)
		return
	}
	readme, err := repo.Readme(tree)
	if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, err)
		return
	}

	entryViews := make([]treeEntryView, 0, len(entries))
	for _, entry := range entries {
		if entry.IsSubmodule {
			entryViews = append(entryViews, treeEntryView{
				Name:        entry.Name,
				Location:    submoduleLocations[entry.Path],
				IsSubmodule: true,
			})
			continue
		}
		target := "blob"
		if entry.IsDir {
			target = "tree"
		}
		entryViews = append(entryViews, treeEntryView{
			Name:  entry.Name,
			URL:   objectURL(repo.Name(), target, rev.Name, entry.Path),
			IsDir: entry.IsDir,
		})
	}

	displayName := displayRepoName(repo.Name())
	data := repoData{
		pageData:             pageData{Title: displayName},
		RepoName:             displayName,
		HomeURL:              homeURL(repo.Name()),
		CloneURLs:            s.cloneURLs(repo.Name()),
		Revision:             rev.Name,
		RevisionLabel:        displayRevision(rev),
		TreeURL:              objectURL(repo.Name(), "tree", rev.Name, treePath),
		LogURL:               repoURL(repo.Name(), "log", rev.Name),
		BranchesURL:          branchesURL(repo.Name(), rev.Name),
		Entries:              entryViews,
		MarkdownBaseURL:      objectURL(repo.Name(), "blob", rev.Name, treePath) + "/",
		MarkdownImageBaseURL: rawObjectURL(repo.Name(), "blob", rev.Name, treePath) + "/",
	}

	if readme != nil {
		data.ReadmeName = readme.Name
		data.Readme = readme.Content
		data.ReadmeIsMD = isMarkdownFile(readme.Name)
	}
	if treePath != "" {
		data.Breadcrumbs = breadcrumbs(repo.Name(), rev.Name, treePath)
	}

	s.render(w, "repo.html", data)
}

func (s *Server) raw(w http.ResponseWriter, r *http.Request, repoName, tail string) {
	section, sectionTail := shiftPath(tail)
	if section != "blob" {
		http.NotFound(w, r)
		return
	}

	s.blob(w, r, repoName, sectionTail, true)
}

func (s *Server) blob(w http.ResponseWriter, r *http.Request, repoName, tail string, forceRaw bool) {
	repo, err := s.store.Open(repoName)
	if err != nil {
		s.writeRepoError(w, r, err)
		return
	}

	rev, filePath, err := splitRevisionAndPath(repo, tail)
	if err != nil || filePath == "" {
		s.writeError(w, r, http.StatusNotFound, errors.New("file not found"))
		return
	}

	blob, err := repo.Blob(rev.Commit, filePath)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, repository.ErrNotFile) {
			status = http.StatusNotFound
		}
		s.writeError(w, r, status, err)
		return
	}

	if forceRaw || r.URL.Query().Get("raw") != "" || !blob.Text {
		s.serveRawBlob(w, r, blob)
		return
	}

	dirPath := path.Dir(filePath)
	if dirPath == "." {
		dirPath = ""
	}

	displayName := displayRepoName(repo.Name())
	data := blobData{
		pageData:             pageData{Title: blob.Name},
		RepoName:             displayName,
		HomeURL:              homeURL(repo.Name()),
		CloneURLs:            s.cloneURLs(repo.Name()),
		Revision:             rev.Name,
		RevisionLabel:        displayRevision(rev),
		TreeURL:              objectURL(repo.Name(), "tree", rev.Name, dirPath),
		LogURL:               repoURL(repo.Name(), "log", rev.Name),
		BranchesURL:          branchesURL(repo.Name(), rev.Name),
		FilePath:             filePath,
		DirectoryURL:         objectURL(repo.Name(), "tree", rev.Name, dirPath),
		HistoryURL:           logURL(repo.Name(), rev.Name, filePath, 0),
		RawURL:               rawObjectURL(repo.Name(), "blob", rev.Name, filePath),
		Content:              string(blob.Content),
		ContentType:          blob.ContentType,
		IsMarkdown:           isMarkdownFile(blob.Name),
		MarkdownBaseURL:      objectURL(repo.Name(), "blob", rev.Name, dirPath) + "/",
		MarkdownImageBaseURL: rawObjectURL(repo.Name(), "blob", rev.Name, dirPath) + "/",
	}
	data.Breadcrumbs = blobBreadcrumbs(repo.Name(), rev.Name, filePath)

	s.render(w, "blob.html", data)
}

func (s *Server) serveRawBlob(w http.ResponseWriter, r *http.Request, blob *repository.Blob) {
	w.Header().Set("Content-Type", blob.ContentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(blob.Content)))
	http.ServeContent(w, r, blob.Name, time.Time{}, bytes.NewReader(blob.Content))
}

func (s *Server) log(w http.ResponseWriter, r *http.Request, repoName, tail string) {
	repo, err := s.store.Open(repoName)
	if err != nil {
		s.writeRepoError(w, r, err)
		return
	}

	revision := strings.Trim(strings.TrimSpace(tail), "/")
	if revision == "" {
		s.writeError(w, r, http.StatusNotFound, errors.New("revision not found"))
		return
	}

	rev, err := repo.ResolveRevision(revision)
	if err != nil {
		s.writeError(w, r, http.StatusNotFound, err)
		return
	}

	page := 0
	if raw := r.URL.Query().Get("page"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 {
			s.writeError(w, r, http.StatusBadRequest, errors.New("invalid page"))
			return
		}
		page = value
	}

	filePath := strings.TrimSpace(r.URL.Query().Get("path"))
	result, err := repo.Log(rev.Commit, filePath, page, commitsPerPage)
	if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, err)
		return
	}

	items := make([]commitView, 0, len(result.Commits))
	for _, commit := range result.Commits {
		subject := commit.Message
		if i := strings.IndexByte(subject, '\n'); i >= 0 {
			subject = subject[:i]
		}

		targetURL := objectURL(repo.Name(), "tree", commit.Hash.String(), "")
		if filePath != "" {
			targetURL = objectURL(repo.Name(), "blob", commit.Hash.String(), filePath)
		}
		items = append(items, commitView{
			Hash:    shortHash(commit.Hash.String()),
			URL:     targetURL,
			Author:  commit.Author.Name,
			Date:    commit.Author.When.Format("2006-01-02 15:04"),
			Subject: subject,
		})
	}

	displayName := displayRepoName(repo.Name())
	data := logData{
		pageData:      pageData{Title: fmt.Sprintf("%s log", displayName)},
		RepoName:      displayName,
		HomeURL:       homeURL(repo.Name()),
		CloneURLs:     s.cloneURLs(repo.Name()),
		Revision:      rev.Name,
		RevisionLabel: displayRevision(rev),
		TreeURL:       objectURL(repo.Name(), "tree", rev.Name, ""),
		LogURL:        repoURL(repo.Name(), "log", rev.Name),
		BranchesURL:   branchesURL(repo.Name(), rev.Name),
		Path:          filePath,
		Commits:       items,
		Page:          page,
	}
	if filePath != "" {
		data.BackURL = objectURL(repo.Name(), "blob", rev.Name, filePath)
	}
	if page > 0 {
		data.PreviousURL = logURL(repo.Name(), rev.Name, filePath, page-1)
	}
	if result.HasMore {
		data.NextURL = logURL(repo.Name(), rev.Name, filePath, page+1)
	}

	s.render(w, "log.html", data)
}

func (s *Server) branches(w http.ResponseWriter, r *http.Request, repoName string) {
	repo, err := s.store.Open(repoName)
	if err != nil {
		s.writeRepoError(w, r, err)
		return
	}

	var rev *repository.ResolvedRevision
	selectedRevision := strings.TrimSpace(r.URL.Query().Get("rev"))
	if selectedRevision != "" {
		rev, err = repo.ResolveRevision(selectedRevision)
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, repository.ErrEmptyRepository) || errors.Is(err, plumbing.ErrReferenceNotFound) {
				status = http.StatusNotFound
			}
			s.writeError(w, r, status, err)
			return
		}
	} else {
		rev, err = repo.DefaultRevision()
		if err != nil {
			if !errors.Is(err, repository.ErrEmptyRepository) {
				s.writeError(w, r, http.StatusInternalServerError, err)
				return
			}
		}
	}

	branches, err := repo.Branches()
	if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, err)
		return
	}

	items := make([]branchView, 0, len(branches))
	for _, branch := range branches {
		items = append(items, branchView{
			Name:    branch.Name,
			URL:     objectURL(repo.Name(), "tree", branch.Name, ""),
			Hash:    shortHash(branch.Hash.String()),
			Current: branch.Current,
		})
	}

	displayName := displayRepoName(repo.Name())
	data := branchesData{
		pageData:    pageData{Title: fmt.Sprintf("%s branches", displayName)},
		RepoName:    displayName,
		HomeURL:     homeURL(repo.Name()),
		CloneURLs:   s.cloneURLs(repo.Name()),
		Empty:       rev == nil,
		BranchesURL: branchesURL(repo.Name(), selectedRevision),
		Branches:    items,
	}
	if rev != nil {
		data.Revision = rev.Name
		data.RevisionLabel = displayRevision(rev)
		data.TreeURL = objectURL(repo.Name(), "tree", rev.Name, "")
		data.LogURL = repoURL(repo.Name(), "log", rev.Name)
		data.BranchesURL = branchesURL(repo.Name(), rev.Name)
	}

	s.render(w, "branches.html", data)
}

func (s *Server) serveStyle(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	http.ServeFileFS(w, r, assets, "static/style.css")
}

func (s *Server) serveCopyScript(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	http.ServeFileFS(w, r, assets, "static/copy.js")
}

func (s *Server) render(w http.ResponseWriter, name string, data any) {
	var body bytes.Buffer
	if err := s.templates.ExecuteTemplate(&body, name, data); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(body.Bytes())
}

func (s *Server) writeRepoError(w http.ResponseWriter, r *http.Request, err error) {
	status := http.StatusInternalServerError
	if errors.Is(err, os.ErrNotExist) {
		status = http.StatusNotFound
	}
	s.writeError(w, r, status, err)
}

func (s *Server) writeError(w http.ResponseWriter, r *http.Request, status int, err error) {
	http.Error(w, http.StatusText(status), status)
}

func splitRevisionAndPath(repo *repository.Repository, raw string) (*repository.ResolvedRevision, string, error) {
	trimmed := strings.Trim(strings.TrimSpace(raw), "/")
	if trimmed == "" {
		return nil, "", errors.New("revision not found")
	}

	parts := strings.Split(trimmed, "/")
	for i := len(parts); i >= 1; i-- {
		candidate := strings.Join(parts[:i], "/")
		rev, err := repo.ResolveRevision(candidate)
		if err != nil {
			continue
		}
		return rev, strings.Join(parts[i:], "/"), nil
	}

	return nil, "", errors.New("revision not found")
}

func shiftPath(raw string) (string, string) {
	clean := strings.TrimPrefix(raw, "/")
	if clean == "" {
		return "", ""
	}
	parts := strings.SplitN(clean, "/", 2)
	if len(parts) == 1 {
		return parts[0], ""
	}
	return parts[0], "/" + parts[1]
}

func breadcrumbs(repoName, revision, treePath string) []breadcrumb {
	parts := strings.Split(treePath, "/")
	items := make([]breadcrumb, 0, len(parts)+1)
	items = append(items, breadcrumb{
		Name: "root",
		URL:  objectURL(repoName, "tree", revision, ""),
	})

	current := ""
	for _, part := range parts {
		current = path.Join(current, part)
		items = append(items, breadcrumb{
			Name: part,
			URL:  objectURL(repoName, "tree", revision, current),
		})
	}

	return items
}

func blobBreadcrumbs(repoName, revision, filePath string) []breadcrumb {
	parts := strings.Split(filePath, "/")
	items := make([]breadcrumb, 0, len(parts)+1)
	items = append(items, breadcrumb{
		Name: "root",
		URL:  objectURL(repoName, "tree", revision, ""),
	})

	current := ""
	for i, part := range parts {
		current = path.Join(current, part)
		target := objectURL(repoName, "tree", revision, current)
		if i == len(parts)-1 {
			target = objectURL(repoName, "blob", revision, current)
		}
		items = append(items, breadcrumb{
			Name: part,
			URL:  target,
		})
	}

	return items
}

func repoURL(repoName, section, revision string) string {
	base := homeURL(repoName)
	if section == "" {
		return base
	}
	return base + "/" + section + "/" + escapeSlashed(revision)
}

func repoSectionURL(repoName, section string) string {
	return homeURL(repoName) + "/" + section
}

func branchesURL(repoName, revision string) string {
	base := repoSectionURL(repoName, "branches")
	if revision == "" {
		return base
	}
	return base + "?rev=" + url.QueryEscape(revision)
}

func homeURL(repoName string) string {
	return "/" + escapeSlashed(displayRepoName(repoName))
}

func objectURL(repoName, section, revision, objectPath string) string {
	base := repoURL(repoName, section, revision)
	if objectPath == "" {
		if section == "tree" {
			return base + "/"
		}
		return base
	}
	return base + "/" + escapeSlashed(objectPath)
}

func rawObjectURL(repoName, section, revision, objectPath string) string {
	base := homeURL(repoName) + "/raw/" + section + "/" + escapeSlashed(revision)
	if objectPath == "" {
		return base
	}
	return base + "/" + escapeSlashed(objectPath)
}

func logURL(repoName, revision, filePath string, page int) string {
	values := url.Values{}
	if filePath != "" {
		values.Set("path", filePath)
	}
	if page > 0 {
		values.Set("page", strconv.Itoa(page))
	}

	base := repoURL(repoName, "log", revision)
	query := values.Encode()
	if query == "" {
		return base
	}
	return base + "?" + query
}

func (s *Server) cloneURLs(repoName string) []cloneLink {
	urls := make([]cloneLink, 0, 2)
	if prefix := strings.TrimSpace(s.config.CloneSSHPrefix); prefix != "" {
		urls = append(urls, cloneLink{
			Label: "ssh",
			URL:   prefix + repoName,
		})
	}
	if prefix := strings.TrimSpace(s.config.CloneHTTPSPrefix); prefix != "" {
		urls = append(urls, cloneLink{
			Label: "https",
			URL:   prefix + repoName,
		})
	}
	return urls
}

func escapeSlashed(value string) string {
	parts := strings.Split(value, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

func shortHash(hash string) string {
	if len(hash) <= 7 {
		return hash
	}
	return hash[:7]
}

func displayRepoName(name string) string {
	if strings.HasSuffix(name, ".git") && len(name) > len(".git") {
		return strings.TrimSuffix(name, ".git")
	}
	return name
}

func isMarkdownFile(name string) bool {
	return strings.EqualFold(path.Ext(name), ".md")
}

func displayRevision(rev *repository.ResolvedRevision) string {
	if rev == nil {
		return ""
	}
	if isLikelyHash(rev.Name) {
		return shortHash(rev.Hash.String())
	}
	return rev.Name
}

func isLikelyHash(value string) bool {
	if len(value) < 7 || len(value) > 40 {
		return false
	}
	for _, r := range value {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') && (r < 'A' || r > 'F') {
			return false
		}
	}
	return true
}
