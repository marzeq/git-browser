package repository

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func TestDiscoverListsDirectRepositories(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "plain"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := initRepo(root, "alpha"); err != nil {
		t.Fatal(err)
	}
	if err := initRepo(root, "beta"); err != nil {
		t.Fatal(err)
	}

	store, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}

	repos := store.List()
	if len(repos) != 2 {
		t.Fatalf("got %d repositories, want 2", len(repos))
	}
	if repos[0].Name != "alpha" || repos[1].Name != "beta" {
		t.Fatalf("unexpected repositories: %#v", repos)
	}
}

func TestResolveRevisionAndReadme(t *testing.T) {
	root := t.TempDir()
	if err := initRepo(root, "demo"); err != nil {
		t.Fatal(err)
	}

	store, err := Discover(root)
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
	if rev.Name == "" {
		t.Fatal("default revision name is empty")
	}

	tree, err := repo.Tree(rev.Commit, "")
	if err != nil {
		t.Fatal(err)
	}

	readme, err := repo.Readme(tree)
	if err != nil {
		t.Fatal(err)
	}
	if readme == nil || readme.Name != "README.md" {
		t.Fatalf("unexpected readme: %#v", readme)
	}

	blob, err := repo.Blob(rev.Commit, "README.md")
	if err != nil {
		t.Fatal(err)
	}
	if !blob.Text {
		t.Fatal("README blob should be text")
	}
}

func TestDefaultRevisionReturnsEmptyRepositoryError(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "empty")
	if _, err := git.PlainInit(path, false); err != nil {
		t.Fatal(err)
	}

	store, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}

	repo, err := store.Open("empty")
	if err != nil {
		t.Fatal(err)
	}

	_, err = repo.DefaultRevision()
	if !errors.Is(err, ErrEmptyRepository) {
		t.Fatalf("got error %v, want %v", err, ErrEmptyRepository)
	}
}

func TestLogOrdersMergeHistoryByCommitterTime(t *testing.T) {
	root := t.TempDir()
	if err := initRepoWithMergeHistory(root, "demo"); err != nil {
		t.Fatal(err)
	}

	store, err := Discover(root)
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

	page, err := repo.Log(rev.Commit, "", 0, 10)
	if err != nil {
		t.Fatal(err)
	}

	var messages []string
	for _, commit := range page.Commits {
		messages = append(messages, commit.Message)
	}

	want := []string{
		"merge feature",
		"main change",
		"feature change",
		"initial commit",
	}

	if len(messages) < len(want) {
		t.Fatalf("got %d commits, want at least %d: %#v", len(messages), len(want), messages)
	}

	for i, wantMessage := range want {
		if messages[i] != wantMessage {
			t.Fatalf("commit %d = %q, want %q; full order: %#v", i, messages[i], wantMessage, messages)
		}
	}
}

func initRepo(root, name string) error {
	path := filepath.Join(root, name)
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

	_, err = wt.Commit("initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test",
			Email: "test@example.com",
			When:  time.Unix(0, 0),
		},
	})
	return err
}

func initRepoWithMergeHistory(root, name string) error {
	path := filepath.Join(root, name)
	repo, err := git.PlainInit(path, false)
	if err != nil {
		return err
	}

	wt, err := repo.Worktree()
	if err != nil {
		return err
	}

	writeAndAdd := func(name, content string) error {
		if err := os.WriteFile(filepath.Join(path, name), []byte(content), 0o644); err != nil {
			return err
		}
		_, err := wt.Add(name)
		return err
	}

	commitFile := func(message, fileName, content string, when time.Time, opts *git.CommitOptions) (plumbing.Hash, error) {
		if err := writeAndAdd(fileName, content); err != nil {
			return plumbing.ZeroHash, err
		}
		if opts == nil {
			opts = &git.CommitOptions{}
		}
		opts.Author = &object.Signature{
			Name:  "Test",
			Email: "test@example.com",
			When:  when,
		}
		return wt.Commit(message, opts)
	}

	initialHash, err := commitFile("initial commit", "README.md", "initial\n", time.Unix(0, 0), nil)
	if err != nil {
		return err
	}

	if err := wt.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName("feature"),
		Create: true,
		Hash:   initialHash,
	}); err != nil {
		return err
	}

	featureHash, err := commitFile("feature change", "feature.txt", "feature\n", time.Unix(60, 0), nil)
	if err != nil {
		return err
	}

	if err := wt.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName("master"),
		Force:  true,
	}); err != nil {
		return err
	}

	mainHash, err := commitFile("main change", "main.txt", "main\n", time.Unix(120, 0), nil)
	if err != nil {
		return err
	}

	if err := repo.CreateBranch(&config.Branch{Name: "feature"}); err != nil && err != git.ErrBranchExists {
		return err
	}

	_, err = wt.Commit("merge feature", &git.CommitOptions{
		AllowEmptyCommits: true,
		Author: &object.Signature{
			Name:  "Test",
			Email: "test@example.com",
			When:  time.Unix(180, 0),
		},
		Parents: []plumbing.Hash{mainHash, featureHash},
	})
	return err
}
