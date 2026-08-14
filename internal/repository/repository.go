package repository

import (
	"bytes"
	"errors"
	"io"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
)

var (
	ErrNotTree         = errors.New("path is not a tree")
	ErrNotFile         = errors.New("path is not a file")
	ErrEmptyRepository = errors.New("repository has no commits")
)

var readmeNames = []string{
	"README.md",
	"README.txt",
	"README",
	"Readme.md",
}

type Repository struct {
	name string
	path string
	repo *git.Repository
}

type ResolvedRevision struct {
	Name   string
	Hash   plumbing.Hash
	Commit *object.Commit
}

type TreeEntry struct {
	Name        string
	Path        string
	IsDir       bool
	IsSubmodule bool
	Mode        filemode.FileMode
}

type Readme struct {
	Name    string
	Content string
}

type Blob struct {
	Name        string
	Path        string
	Content     []byte
	ContentType string
	Text        bool
}

type CommitPage struct {
	Commits []*object.Commit
	HasMore bool
}

type Branch struct {
	Name    string
	Hash    plumbing.Hash
	Current bool
}

type Tag struct {
	Name string
	Hash plumbing.Hash
}

func Open(name, path string) (*Repository, error) {
	repo, err := git.PlainOpen(path)
	if err != nil {
		return nil, err
	}

	return &Repository{name: name, path: path, repo: repo}, nil
}

func (r *Repository) Name() string {
	return r.name
}

func (r *Repository) DefaultRevision() (*ResolvedRevision, error) {
	head, err := r.repo.Head()
	if err != nil {
		if errors.Is(err, plumbing.ErrReferenceNotFound) {
			return nil, ErrEmptyRepository
		}
		return nil, err
	}

	name := shortReferenceName(head)
	commit, err := r.repo.CommitObject(head.Hash())
	if err != nil {
		return nil, err
	}

	return &ResolvedRevision{Name: name, Hash: head.Hash(), Commit: commit}, nil
}

func (r *Repository) ResolveRevision(spec string) (*ResolvedRevision, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" || spec == "HEAD" {
		return r.DefaultRevision()
	}

	hash, err := r.repo.ResolveRevision(plumbing.Revision(spec))
	if err != nil {
		if errors.Is(err, plumbing.ErrReferenceNotFound) {
			return nil, ErrEmptyRepository
		}
		return nil, err
	}

	commit, err := r.repo.CommitObject(*hash)
	if err != nil {
		return nil, err
	}

	return &ResolvedRevision{Name: spec, Hash: *hash, Commit: commit}, nil
}

func (r *Repository) Tree(commit *object.Commit, treePath string) (*object.Tree, error) {
	tree, err := commit.Tree()
	if err != nil {
		return nil, err
	}
	if treePath == "" {
		return tree, nil
	}

	subtree, err := tree.Tree(treePath)
	if err != nil {
		if errors.Is(err, object.ErrDirectoryNotFound) || errors.Is(err, object.ErrEntryNotFound) {
			return nil, ErrNotTree
		}
		return nil, err
	}

	return subtree, nil
}

func (r *Repository) TreeEntries(tree *object.Tree, treePath string) []TreeEntry {
	entries := make([]TreeEntry, 0, len(tree.Entries))
	for _, entry := range tree.Entries {
		entryPath := joinGitPath(treePath, entry.Name)
		entries = append(entries, TreeEntry{
			Name:        entry.Name,
			Path:        entryPath,
			IsDir:       entry.Mode == filemode.Dir,
			IsSubmodule: entry.Mode == filemode.Submodule,
			Mode:        entry.Mode,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})

	return entries
}

// SubmoduleLocations returns the configured clone location for each submodule
// path at the given commit. Reading .gitmodules from the commit keeps historic
// tree pages independent of the repository's current worktree state.
func (r *Repository) SubmoduleLocations(commit *object.Commit) (map[string]string, error) {
	locations := make(map[string]string)
	tree, err := commit.Tree()
	if err != nil {
		return nil, err
	}

	file, err := tree.File(".gitmodules")
	if err != nil {
		if errors.Is(err, object.ErrFileNotFound) {
			return locations, nil
		}
		return nil, err
	}

	content, err := readAll(file)
	if err != nil {
		return nil, err
	}

	modules := gitconfig.NewModules()
	if err := modules.Unmarshal(content); err != nil {
		return nil, err
	}
	for _, module := range modules.Submodules {
		locations[module.Path] = module.URL
	}

	return locations, nil
}

func (r *Repository) Readme(tree *object.Tree) (*Readme, error) {
	for _, name := range readmeNames {
		file, err := tree.File(name)
		if err != nil {
			if errors.Is(err, object.ErrFileNotFound) {
				continue
			}
			return nil, err
		}

		content, err := readAll(file)
		if err != nil {
			return nil, err
		}

		return &Readme{Name: name, Content: string(content)}, nil
	}

	return nil, nil
}

func (r *Repository) Blob(commit *object.Commit, filePath string) (*Blob, error) {
	tree, err := commit.Tree()
	if err != nil {
		return nil, err
	}

	file, err := tree.File(filePath)
	if err != nil {
		if errors.Is(err, object.ErrFileNotFound) {
			return nil, ErrNotFile
		}
		return nil, err
	}

	content, err := readAll(file)
	if err != nil {
		return nil, err
	}

	contentType := detectContentType(content)
	return &Blob{
		Name:        file.Name,
		Path:        filePath,
		Content:     content,
		ContentType: contentType,
		Text:        isTextContent(content, contentType),
	}, nil
}

func (r *Repository) Log(commit *object.Commit, filePath string, page, perPage int) (*CommitPage, error) {
	options := &git.LogOptions{
		From:  commit.Hash,
		Order: git.LogOrderCommitterTime,
	}
	if filePath != "" {
		options.FileName = &filePath
	}

	iter, err := r.repo.Log(options)
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	skip := page * perPage
	limit := skip + perPage + 1
	items := make([]*object.Commit, 0, perPage+1)
	index := 0

	for {
		next, err := iter.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}

		if index < skip {
			index++
			continue
		}
		if len(items) >= limit-skip {
			break
		}

		items = append(items, next)
		index++
	}

	hasMore := len(items) > perPage
	if hasMore {
		items = items[:perPage]
	}

	return &CommitPage{Commits: items, HasMore: hasMore}, nil
}

func (r *Repository) Branches() ([]Branch, error) {
	head, err := r.repo.Head()
	if err != nil {
		if !errors.Is(err, plumbing.ErrReferenceNotFound) {
			return nil, err
		}
		head = nil
	}

	iter, err := r.repo.Branches()
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var branches []Branch
	err = iter.ForEach(func(ref *plumbing.Reference) error {
		current := false
		if head != nil {
			current = head.Name() == ref.Name()
		}
		branches = append(branches, Branch{
			Name:    ref.Name().Short(),
			Hash:    ref.Hash(),
			Current: current,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(branches, func(i, j int) bool {
		return strings.ToLower(branches[i].Name) < strings.ToLower(branches[j].Name)
	})

	return branches, nil
}

func (r *Repository) Tags() ([]Tag, error) {
	iter, err := r.repo.Tags()
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var tags []Tag
	err = iter.ForEach(func(ref *plumbing.Reference) error {
		hash, err := r.repo.ResolveRevision(plumbing.Revision(ref.Name().String()))
		if err != nil {
			return err
		}
		if _, err := r.repo.CommitObject(*hash); err != nil {
			return err
		}
		tags = append(tags, Tag{Name: ref.Name().Short(), Hash: *hash})
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(tags, func(i, j int) bool {
		return strings.ToLower(tags[i].Name) < strings.ToLower(tags[j].Name)
	})
	return tags, nil
}

func shortReferenceName(ref *plumbing.Reference) string {
	if ref.Name().IsBranch() || ref.Name().IsTag() {
		return ref.Name().Short()
	}
	if ref.Type() == plumbing.SymbolicReference {
		return ref.Target().Short()
	}
	return shortHash(ref.Hash().String())
}

func shortHash(hash string) string {
	if len(hash) <= 7 {
		return hash
	}
	return hash[:7]
}

func joinGitPath(parent, name string) string {
	if parent == "" {
		return name
	}
	return parent + "/" + name
}

func readAll(file *object.File) ([]byte, error) {
	reader, err := file.Reader()
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	return io.ReadAll(reader)
}

func detectContentType(content []byte) string {
	if len(content) == 0 {
		return "text/plain; charset=utf-8"
	}
	sniff := content
	if len(sniff) > 512 {
		sniff = sniff[:512]
	}
	return httpDetectContentType(sniff)
}

func isTextContent(content []byte, contentType string) bool {
	if !utf8.Valid(content) {
		return false
	}
	if bytes.IndexByte(content, 0) >= 0 {
		return false
	}

	if strings.HasPrefix(contentType, "text/") {
		return true
	}

	switch {
	case strings.Contains(contentType, "json"),
		strings.Contains(contentType, "xml"),
		strings.Contains(contentType, "javascript"),
		strings.Contains(contentType, "svg+xml"),
		strings.Contains(contentType, "x-sh"):
		return true
	default:
		return false
	}
}
