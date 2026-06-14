package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os/user"
	"path/filepath"
	"strings"

	"git-browser/internal/repository"
	"git-browser/internal/web"
)

func main() {
	sshUser := flag.String("ssh-user", "git", "SSH user allowed to access repositories")
	root := flag.String("root", "repos", "repository root relative to the SSH user's home directory")
	listen := flag.String("listen", ":8080", "HTTP listen address")
	flag.Parse()

	resolvedRoot, cloneRoot, err := resolveRoot(*sshUser, *root)
	if err != nil {
		log.Fatalf("resolve repository root: %v", err)
	}

	store, err := repository.Discover(resolvedRoot)
	if err != nil {
		log.Fatalf("discover repositories: %v", err)
	}

	server, err := web.NewServer(store, web.Config{
		SSHUser:   *sshUser,
		CloneRoot: cloneRoot,
	})
	if err != nil {
		log.Fatalf("create server: %v", err)
	}

	log.Printf("serving %s on %s", store.Root(), *listen)
	if err := http.ListenAndServe(*listen, server.Handler()); err != nil {
		log.Fatal(err)
	}
}

func resolveRoot(userName, root string) (string, string, error) {
	account, err := user.Lookup(userName)
	if err != nil {
		return "", "", err
	}

	cleaned := filepath.Clean(root)
	if root == "" || cleaned == "." {
		return account.HomeDir, "", nil
	}
	if filepath.IsAbs(cleaned) || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("root must stay within %s's home directory", userName)
	}

	return filepath.Join(account.HomeDir, cleaned), filepath.ToSlash(cleaned), nil
}
