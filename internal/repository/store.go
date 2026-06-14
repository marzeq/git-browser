package repository

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/go-git/go-git/v5"
)

type Info struct {
	Name string
	Path string
}

type Store struct {
	root  string
	repos map[string]Info

	mu    sync.Mutex
	cache map[string]*Repository
}

func Discover(root string) (*Store, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}

	repos := make(map[string]Info)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		path := filepath.Join(root, name)
		if _, err := git.PlainOpen(path); err != nil {
			if errors.Is(err, git.ErrRepositoryNotExists) {
				continue
			}
			return nil, err
		}

		repos[name] = Info{Name: name, Path: path}
	}

	return &Store{
		root:  root,
		repos: repos,
		cache: make(map[string]*Repository),
	}, nil
}

func (s *Store) Root() string {
	return s.root
}

func (s *Store) List() []Info {
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
	info, ok := s.repos[name]
	if !ok {
		return nil, os.ErrNotExist
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if repo, ok := s.cache[name]; ok {
		return repo, nil
	}

	repo, err := Open(info.Name, info.Path)
	if err != nil {
		return nil, err
	}

	s.cache[name] = repo
	return repo, nil
}
