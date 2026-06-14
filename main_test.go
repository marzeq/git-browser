package main

import (
	"os/user"
	"path/filepath"
	"testing"
)

func TestResolveRoot(t *testing.T) {
	current, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}

	t.Run("nested root", func(t *testing.T) {
		resolved, cloneRoot, err := resolveRoot(current.Username, "repos/nested")
		if err != nil {
			t.Fatal(err)
		}
		if resolved != filepath.Join(current.HomeDir, "repos", "nested") {
			t.Fatalf("got resolved root %q", resolved)
		}
		if cloneRoot != "repos/nested" {
			t.Fatalf("got clone root %q", cloneRoot)
		}
	})

	t.Run("empty root uses home", func(t *testing.T) {
		resolved, cloneRoot, err := resolveRoot(current.Username, "")
		if err != nil {
			t.Fatal(err)
		}
		if resolved != current.HomeDir {
			t.Fatalf("got resolved root %q", resolved)
		}
		if cloneRoot != "" {
			t.Fatalf("got clone root %q", cloneRoot)
		}
	})
}
