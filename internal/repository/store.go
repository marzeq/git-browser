package repository

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-git/go-git/v5"
)

type Info struct {
	Name string
	Path string
}

type Store struct {
	root   string
	hidden map[string]struct{}

	mu         sync.RWMutex
	repos      map[string]Info
	cache      map[string]*Repository
	refreshing bool
}

func Discover(root string, hiddenRepos ...string) (*Store, error) {
	hidden := normalizeHiddenRepos(hiddenRepos)
	repos, err := discoverRepos(root, hidden)
	if err != nil {
		return nil, err
	}

	return &Store{
		root:   root,
		hidden: hidden,
		repos:  repos,
		cache:  make(map[string]*Repository),
	}, nil
}

func (s *Store) Root() string {
	return s.root
}

func (s *Store) List() []Info {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]Info, 0, len(s.repos))
	for _, repo := range s.repos {
		items = append(items, repo)
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].Name < items[j].Name
	})

	return items
}

func (s *Store) Open(name string) (*Repository, error) {
	s.mu.RLock()
	info, ok := s.repos[name]
	if !ok {
		s.mu.RUnlock()
		return nil, os.ErrNotExist
	}

	if repo, ok := s.cache[name]; ok {
		s.mu.RUnlock()
		return repo, nil
	}
	s.mu.RUnlock()

	repo, err := Open(info.Name, info.Path)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if cached, ok := s.cache[name]; ok {
		return cached, nil
	}
	if _, ok := s.repos[name]; !ok {
		return nil, os.ErrNotExist
	}

	s.cache[name] = repo
	return repo, nil
}

func (s *Store) SplitPath(raw string) (string, string, bool) {
	clean := strings.Trim(strings.TrimSpace(raw), "/")
	if clean == "" {
		return "", "", false
	}

	parts := strings.Split(clean, "/")

	s.mu.RLock()
	defer s.mu.RUnlock()

	for i := len(parts); i >= 1; i-- {
		candidate := strings.Join(parts[:i], "/")
		if _, ok := s.repos[candidate]; !ok {
			continue
		}
		if i == len(parts) {
			return candidate, "", true
		}
		return candidate, "/" + strings.Join(parts[i:], "/"), true
	}

	return "", "", false
}

func (s *Store) Refresh() error {
	repos, err := discoverRepos(s.root, s.hidden)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.repos = repos
	s.cache = make(map[string]*Repository)
	return nil
}

func (s *Store) RefreshAsync(onError func(error)) bool {
	s.mu.Lock()
	if s.refreshing {
		s.mu.Unlock()
		return false
	}
	s.refreshing = true
	s.mu.Unlock()

	go func() {
		err := s.Refresh()

		s.mu.Lock()
		s.refreshing = false
		s.mu.Unlock()

		if err != nil && onError != nil {
			onError(err)
		}
	}()

	return true
}

func (s *Store) StartPeriodicRefresh(interval time.Duration, onError func(error)) func() {
	if interval <= 0 {
		return func() {}
	}

	done := make(chan struct{})
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s.RefreshAsync(onError)
			case <-done:
				return
			}
		}
	}()

	var once sync.Once
	return func() {
		once.Do(func() {
			close(done)
		})
	}
}

func normalizeHiddenRepos(hiddenRepos []string) map[string]struct{} {
	hidden := make(map[string]struct{}, len(hiddenRepos))
	for _, repo := range hiddenRepos {
		name := strings.Trim(filepath.ToSlash(strings.TrimSpace(repo)), "/")
		if name == "" {
			continue
		}
		hidden[name] = struct{}{}
	}
	return hidden
}

func discoverRepos(root string, hidden map[string]struct{}) (map[string]Info, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}

	repos := make(map[string]Info)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		found, err := appendRepo(repos, hidden, root, entry.Name())
		if err != nil {
			return nil, err
		}
		if found {
			continue
		}

		children, err := os.ReadDir(filepath.Join(root, entry.Name()))
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, err
		}
		for _, child := range children {
			if !child.IsDir() {
				continue
			}
			if _, err := appendRepo(repos, hidden, root, entry.Name(), child.Name()); err != nil {
				return nil, err
			}
		}
	}

	return repos, nil
}

func appendRepo(repos map[string]Info, hidden map[string]struct{}, root string, parts ...string) (bool, error) {
	name := filepath.ToSlash(filepath.Join(parts...))
	repoPath := filepath.Join(append([]string{root}, parts...)...)
	if _, err := git.PlainOpen(repoPath); err != nil {
		if errors.Is(err, git.ErrRepositoryNotExists) {
			return false, nil
		}
		return false, err
	}

	if _, ok := hidden[name]; ok {
		return true, nil
	}

	repos[name] = Info{Name: name, Path: repoPath}
	return true, nil
}
