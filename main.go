package main

import (
	"flag"
	"log"
	"net/http"

	"git-browser/internal/repository"
	"git-browser/internal/web"
)

func main() {
	sshUser := flag.String("ssh-user", "git", "SSH user allowed to access repositories")
	cloneRoot := flag.String("cloneroot", "", "repositories location root used for generating clone URLs (relative to the ssh user's home directory)")
	root := flag.String("root", "repos", "where the actual repositories are stored (absolute path)")
	listen := flag.String("listen", ":8080", "HTTP listen address")
	flag.Parse()

	store, err := repository.Discover(*root)
	if err != nil {
		log.Fatalf("discover repositories: %v", err)
	}

	server, err := web.NewServer(store, web.Config{
		SSHUser:   *sshUser,
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
