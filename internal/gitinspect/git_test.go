package gitinspect_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openmodu/oneshot/internal/gitinspect"
)

func TestInspectNonRepositoryAndRepositoryStatus(t *testing.T) {
	inspector := gitinspect.New("")
	nonRepo := t.TempDir()
	snapshot, err := inspector.Inspect(context.Background(), nonRepo)
	if err != nil || snapshot.IsRepo {
		t.Fatalf("non-repo snapshot = %+v, %v", snapshot, err)
	}

	repo := t.TempDir()
	command := exec.Command("git", "init", repo)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	snapshot, err = inspector.Inspect(context.Background(), repo)
	if err != nil || !snapshot.IsRepo || !strings.Contains(snapshot.Status, "?? main.go") {
		t.Fatalf("repo snapshot = %+v, %v", snapshot, err)
	}
}
