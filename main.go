package main

import (
	"flag"
	"log"
	"net/http"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"git-browser/internal/repository"
	"git-browser/internal/web"
)

func main() {
	cloneUser := flag.String("clone-user", "git", "SSH user allowed to access repositories")
	cloneHost := flag.String("clone-host", "", "hostname used for generating clone URLs (defaults to the request host)")
	cloneRoot := flag.String("clone-root", "", "repositories location root used for generating clone URLs (relative to the ssh user's home directory)")
	root := flag.String("root", "repos", "where the actual repositories are stored (absolute path or relative to the current user's home directory)")
	hide := flag.String("hide", "", "semicolon-separated list of repositories to hide from the UI")
	listen := flag.String("listen", ":8080", "HTTP listen address")
	flag.Parse()

	current, err := user.Current()
	if err != nil {
		log.Fatalf("resolve current user: %v", err)
	}

	resolvedRoot, defaultCloneRoot, err := resolveRoot(current.Username, *root)
	if err != nil {
		log.Fatalf("resolve root: %v", err)
	}
	if *cloneRoot == "" {
		*cloneRoot = defaultCloneRoot
	}

	store, err := repository.Discover(resolvedRoot, parseHiddenRepositories(*hide)...)
	if err != nil {
		log.Fatalf("discover repositories: %v", err)
	}
	stopRefresh := store.StartPeriodicRefresh(5*time.Minute, func(err error) {
		log.Printf("refresh repositories: %v", err)
	})
	defer stopRefresh()

	server, err := web.NewServer(store, web.Config{
		CloneUser: *cloneUser,
		CloneHost: *cloneHost,
		CloneRoot: *cloneRoot,
	})
	if err != nil {
		log.Fatalf("create server: %v", err)
	}

	log.Printf("serving %s on %s", store.Root(), *listen)
	if err := http.ListenAndServe(*listen, server.Handler()); err != nil {
		log.Fatal(err)
	}
}

func resolveRoot(username, root string) (string, string, error) {
	account, err := user.Lookup(username)
	if err != nil {
		return "", "", err
	}

	trimmed := strings.TrimSpace(root)
	if trimmed == "" {
		return account.HomeDir, "", nil
	}
	if filepath.IsAbs(trimmed) {
		return trimmed, "", nil
	}

	return filepath.Join(account.HomeDir, filepath.FromSlash(trimmed)), filepath.ToSlash(trimmed), nil
}

func parseHiddenRepositories(raw string) []string {
	parts := strings.Split(raw, ";")
	hidden := make([]string, 0, len(parts))
	for _, part := range parts {
		name := strings.TrimSpace(part)
		if name == "" {
			continue
		}
		hidden = append(hidden, name)
	}
	return hidden
}
