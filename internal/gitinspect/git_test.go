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
	for _, args := range [][]string{{"-C", repo, "config", "user.name", "Oneshot Test"}, {"-C", repo, "config", "user.email", "oneshot@example.test"}} {
		if output, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, output)
		}
	}
	if err := inspector.StageAll(context.Background(), repo); err != nil {
		t.Fatal(err)
	}
	stagedDiff, err := inspector.Diff(context.Background(), repo, true)
	if err != nil || !strings.Contains(stagedDiff, "+package main") {
		t.Fatalf("staged diff = %q, %v", stagedDiff, err)
	}
	snapshot, err = inspector.Inspect(context.Background(), repo)
	if err != nil || len(snapshot.Files) != 1 || snapshot.Files[0].Path != "main.go" || snapshot.Files[0].Index != "A" {
		t.Fatalf("staged snapshot = %+v, %v", snapshot, err)
	}
	hash, err := inspector.Commit(context.Background(), repo, "feat: add main package")
	if err != nil || len(hash) < 7 {
		t.Fatalf("Commit() = %q, %v", hash, err)
	}
	if _, err := inspector.Commit(context.Background(), repo, "invalid\nmessage"); err == nil {
		t.Fatal("Commit() accepted a multi-line message")
	}
	if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	worktreeDiff, err := inspector.Diff(context.Background(), repo, false)
	if err != nil || !strings.Contains(worktreeDiff, "+func main()") {
		t.Fatalf("worktree diff = %q, %v", worktreeDiff, err)
	}
	remote := filepath.Join(t.TempDir(), "remote.git")
	if output, err := exec.Command("git", "init", "--bare", remote).CombinedOutput(); err != nil {
		t.Fatalf("git init --bare: %v: %s", err, output)
	}
	if output, err := exec.Command("git", "-C", repo, "remote", "add", "origin", remote).CombinedOutput(); err != nil {
		t.Fatalf("git remote add: %v: %s", err, output)
	}
	if err := inspector.Push(context.Background(), repo, "origin"); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("git", "--git-dir", remote, "show-ref").CombinedOutput(); err != nil || !strings.Contains(string(output), hash) {
		t.Fatalf("remote refs = %q, %v", output, err)
	}
}
