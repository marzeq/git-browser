package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"git-browser/internal/repository"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func TestServerRendersCorePages(t *testing.T) {
	root := t.TempDir()
	if err := initRepo(root, "demo"); err != nil {
		t.Fatal(err)
	}

	store, err := repository.Discover(root)
	if err != nil {
		t.Fatal(err)
	}

	server, err := NewServer(store, Config{
		CloneSSHPrefix: "git@localhost:repos/",
	})
	if err != nil {
		t.Fatal(err)
	}

	repo, err := store.Open("demo")
	if err != nil {
		t.Fatal(err)
	}
	rev, err := repo.DefaultRevision()
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		path string
		want string
	}{
		{path: "/", want: "Repositories"},
		{path: "/demo", want: "clone ssh: <span class=\"copy-link\" data-copy-text=\"git@localhost:repos/demo\"><code>git@localhost:repos/demo</code></span>"},
		{path: "/demo", want: "revision: <strong>master</strong>"},
		{path: "/demo", want: "README.md"},
		{path: "/demo/blob/" + rev.Name + "/README.md", want: "aria-label=\"Breadcrumb\""},
		{path: "/demo/blob/" + rev.Name + "/README.md", want: "/demo/raw/blob/" + rev.Name + "/README.md"},
		{path: "/demo/log/" + rev.Name, want: "/demo/tree/"},
		{path: "/demo/log/" + rev.Name + "?path=README.md", want: "back to file"},
		{path: "/demo/branches", want: "feature"},
		{path: "/demo/tree/feature/", want: "README.md"},
		{path: "/demo/tree/feature/", want: "/demo/branches?rev=feature"},
	}

	for _, tc := range tests {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		req.Host = "localhost:8080"
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("%s: got status %d, want %d", tc.path, rec.Code, http.StatusOK)
		}
		if !strings.Contains(rec.Body.String(), tc.want) {
			t.Fatalf("%s: response did not contain %q\nbody:\n%s", tc.path, tc.want, rec.Body.String())
		}
	}
}

func TestServerRendersConfiguredClonePrefixes(t *testing.T) {
	root := t.TempDir()
	if err := initRepo(root, "demo"); err != nil {
		t.Fatal(err)
	}

	store, err := repository.Discover(root)
	if err != nil {
		t.Fatal(err)
	}

	server, err := NewServer(store, Config{
		CloneSSHPrefix:   "ssh://git.example.com/repos/",
		CloneHTTPSPrefix: "https://git.example.com/repos/",
	})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/demo", nil)
	req.Host = "localhost:8080"
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"ssh://git.example.com/repos/demo",
		"https://git.example.com/repos/demo",
		"clone ssh:",
		"clone https:",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("response did not contain %q\nbody:\n%s", want, body)
		}
	}
}

func TestServerRefreshesRepositoryListBetweenRequests(t *testing.T) {
	root := t.TempDir()
	if err := initRepo(root, "alpha"); err != nil {
		t.Fatal(err)
	}

	store, err := repository.Discover(root)
	if err != nil {
		t.Fatal(err)
	}

	server, err := NewServer(store, Config{
		CloneSSHPrefix: "git@localhost:repos/",
	})
	if err != nil {
		t.Fatal(err)
	}

	first := httptest.NewRequest(http.MethodGet, "/", nil)
	first.Host = "localhost:8080"
	firstRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(firstRec, first)

	if firstRec.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d", firstRec.Code, http.StatusOK)
	}
	if strings.Contains(firstRec.Body.String(), "beta") {
		t.Fatalf("unexpected repository in initial response\nbody:\n%s", firstRec.Body.String())
	}

	if err := initRepo(root, "beta"); err != nil {
		t.Fatal(err)
	}

	waitForCondition(t, 2*time.Second, func() bool {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Host = "localhost:8080"
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, req)
		return rec.Code == http.StatusOK && strings.Contains(rec.Body.String(), "beta")
	})
}

func TestServerServesRawBlob(t *testing.T) {
	root := t.TempDir()
	if err := initRepo(root, "demo"); err != nil {
		t.Fatal(err)
	}

	store, err := repository.Discover(root)
	if err != nil {
		t.Fatal(err)
	}

	server, err := NewServer(store, Config{
		CloneSSHPrefix: "git@localhost:repos/",
	})
	if err != nil {
		t.Fatal(err)
	}

	repo, err := store.Open("demo")
	if err != nil {
		t.Fatal(err)
	}
	rev, err := repo.DefaultRevision()
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/demo/raw/blob/"+rev.Name+"/README.md", nil)
	req.Host = "localhost:8080"
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusOK)
	}
	if body := rec.Body.String(); body != "hello\n" {
		t.Fatalf("got raw body %q, want %q", body, "hello\n")
	}
	if contentType := rec.Header().Get("Content-Type"); !strings.Contains(contentType, "text/plain") {
		t.Fatalf("got content type %q, want text/plain", contentType)
	}
}

func TestServerRendersEmptyRepositoryPage(t *testing.T) {
	root := t.TempDir()
	if _, err := git.PlainInit(filepath.Join(root, "empty"), false); err != nil {
		t.Fatal(err)
	}

	store, err := repository.Discover(root)
	if err != nil {
		t.Fatal(err)
	}

	server, err := NewServer(store, Config{
		CloneSSHPrefix: "",
	})
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		path string
		want string
	}{
		{path: "/empty", want: "This repository has no commits yet."},
		{path: "/empty/branches", want: "No branches found."},
	}

	for _, tc := range tests {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		req.Host = "localhost:8080"
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("%s: got status %d, want %d", tc.path, rec.Code, http.StatusOK)
		}
		if !strings.Contains(rec.Body.String(), tc.want) {
			t.Fatalf("%s: response did not contain %q\nbody:\n%s", tc.path, tc.want, rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), "clone ssh:") || strings.Contains(rec.Body.String(), "clone https:") {
			t.Fatalf("%s: response unexpectedly contained clone URL block\nbody:\n%s", tc.path, rec.Body.String())
		}
	}
}

func TestServerOmitsCloneURLsWhenUnconfigured(t *testing.T) {
	root := t.TempDir()
	if err := initRepo(root, "demo"); err != nil {
		t.Fatal(err)
	}

	store, err := repository.Discover(root)
	if err != nil {
		t.Fatal(err)
	}

	server, err := NewServer(store, Config{})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/demo", nil)
	req.Host = "localhost:8080"
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusOK)
	}
	if strings.Contains(rec.Body.String(), "clone ssh:") || strings.Contains(rec.Body.String(), "clone https:") {
		t.Fatalf("response unexpectedly contained clone URL block\nbody:\n%s", rec.Body.String())
	}
}

func TestServerStripsDotGitSuffixOnlyInUI(t *testing.T) {
	root := t.TempDir()
	if err := initRepo(root, "demo.git"); err != nil {
		t.Fatal(err)
	}

	store, err := repository.Discover(root)
	if err != nil {
		t.Fatal(err)
	}

	server, err := NewServer(store, Config{
		CloneSSHPrefix: "git@localhost:repos/",
	})
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		path    string
		want    []string
		wantNot []string
	}{
		{
			path:    "/",
			want:    []string{">demo</a>"},
			wantNot: []string{">demo.git</a>"},
		},
		{
			path:    "/demo.git",
			want:    []string{"<title>demo</title>", "<h1><a href=\"/demo.git\">demo</a>"},
			wantNot: []string{"<title>demo.git</title>", "<h1><a href=\"/demo.git\">demo.git</a>"},
		},
		{
			path:    "/demo.git",
			want:    []string{"git@localhost:repos/demo.git", "data-copy-text=\"git@localhost:repos/demo.git\"", "clone ssh:"},
			wantNot: nil,
		},
	}

	for _, tc := range tests {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		req.Host = "localhost:8080"
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("%s: got status %d, want %d", tc.path, rec.Code, http.StatusOK)
		}

		body := rec.Body.String()
		for _, want := range tc.want {
			if !strings.Contains(body, want) {
				t.Fatalf("%s: response did not contain %q\nbody:\n%s", tc.path, want, body)
			}
		}
		for _, wantNot := range tc.wantNot {
			if strings.Contains(body, wantNot) {
				t.Fatalf("%s: response unexpectedly contained %q\nbody:\n%s", tc.path, wantNot, body)
			}
		}
	}
}

func TestServerSupportsNestedRepositoryPaths(t *testing.T) {
	root := t.TempDir()
	if err := initRepo(root, filepath.Join("acme", "demo.git")); err != nil {
		t.Fatal(err)
	}

	store, err := repository.Discover(root)
	if err != nil {
		t.Fatal(err)
	}

	server, err := NewServer(store, Config{
		CloneSSHPrefix: "git@localhost:repos/",
	})
	if err != nil {
		t.Fatal(err)
	}

	repo, err := store.Open("acme/demo.git")
	if err != nil {
		t.Fatal(err)
	}
	rev, err := repo.DefaultRevision()
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		path string
		want string
	}{
		{path: "/", want: "href=\"/acme/demo.git\""},
		{path: "/acme/demo.git", want: "git@localhost:repos/acme/demo.git"},
		{path: "/acme/demo.git/tree/" + rev.Name + "/", want: "README.md"},
		{path: "/acme/demo.git/raw/blob/" + rev.Name + "/README.md", want: "hello\n"},
	}

	for _, tc := range tests {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		req.Host = "localhost:8080"
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("%s: got status %d, want %d", tc.path, rec.Code, http.StatusOK)
		}
		if !strings.Contains(rec.Body.String(), tc.want) {
			t.Fatalf("%s: response did not contain %q\nbody:\n%s", tc.path, tc.want, rec.Body.String())
		}
	}
}

func waitForCondition(t *testing.T, timeout time.Duration, check func() bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("condition was not met before timeout")
}

func initRepo(root, name string) error {
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	repo, err := git.PlainInit(path, false)
	if err != nil {
		return err
	}

	if err := os.WriteFile(filepath.Join(path, "README.md"), []byte("hello\n"), 0o644); err != nil {
		return err
	}
	if err := os.Mkdir(filepath.Join(path, "src"), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(path, "src", "main.go"), []byte("package main\n"), 0o644); err != nil {
		return err
	}

	wt, err := repo.Worktree()
	if err != nil {
		return err
	}
	if _, err := wt.Add("README.md"); err != nil {
		return err
	}
	if _, err := wt.Add("src/main.go"); err != nil {
		return err
	}

	hash, err := wt.Commit("initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test",
			Email: "test@example.com",
			When:  time.Unix(0, 0),
		},
	})
	if err != nil {
		return err
	}

	if err := repo.CreateBranch(&config.Branch{Name: "feature"}); err != nil {
		return err
	}

	return repo.Storer.SetReference(plumbing.NewHashReference(plumbing.NewBranchReferenceName("feature"), hash))
}
