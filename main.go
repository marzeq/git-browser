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
	sshUser := flag.String("ssh-user", "git", "SSH user allowed to access repositories")
	cloneHost := flag.String("clone-host", "", "hostname used for generating clone URLs (defaults to the request host)")
	cloneRoot := flag.String("clone-root", "", "repositories location root used for generating clone URLs (relative to the ssh user's home directory)")
	root := flag.String("root", "repos", "where the actual repositories are stored (absolute path)")
	hide := flag.String("hide", "", "semicolon-separated list of repositories to hide from the UI")
	listen := flag.String("listen", ":8080", "HTTP listen address")
	flag.Parse()

	store, err := repository.Discover(*root, parseHiddenRepositories(*hide)...)
	if err != nil {
		log.Fatalf("discover repositories: %v", err)
	}
	stopRefresh := store.StartPeriodicRefresh(5*time.Minute, func(err error) {
		log.Printf("refresh repositories: %v", err)
	})
	defer stopRefresh()

	server, err := web.NewServer(store, web.Config{
		SSHUser:   *sshUser,
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

func resolveRoot(username, root string) (string, string, error) {
	account, err := user.Lookup(username)
	if err != nil {
		return "", "", err
	}

	if root == "" {
		return account.HomeDir, "", nil
	}
	if filepath.IsAbs(root) {
		return filepath.Clean(root), "", nil
	}

	return filepath.Join(account.HomeDir, root), filepath.Clean(root), nil
}
