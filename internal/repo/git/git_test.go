package gitrepo_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openmodu/onecatch/internal/repo/git"
)

func TestInspectNonRepositoryAndRepositoryStatus(t *testing.T) {
	inspector := gitrepo.New("")
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
	for _, args := range [][]string{{"-C", repo, "config", "user.name", "OneCatch Test"}, {"-C", repo, "config", "user.email", "onecatch@example.test"}} {
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
	initial, err := inspector.Inspect(context.Background(), repo)
	if err != nil || initial.Branch == "" {
		t.Fatalf("initial branch = %q, %v", initial.Branch, err)
	}
	if err := inspector.CreateBranch(context.Background(), repo, "feature/branch-management"); err != nil {
		t.Fatal(err)
	}
	branches, err := inspector.ListBranches(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	foundCurrent := false
	for _, branch := range branches {
		foundCurrent = foundCurrent || branch.Name == "feature/branch-management" && branch.Current
	}
	if len(branches) != 2 || !foundCurrent {
		t.Fatalf("branches = %+v", branches)
	}
	if err := inspector.SwitchBranch(context.Background(), repo, initial.Branch); err != nil {
		t.Fatal(err)
	}
	if err := inspector.SwitchBranch(context.Background(), repo, "missing"); err == nil {
		t.Fatal("SwitchBranch() accepted a missing branch")
	}
	if err := inspector.CreateBranch(context.Background(), repo, "feature/branch-management"); err == nil {
		t.Fatal("CreateBranch() accepted a duplicate branch")
	}
	if err := inspector.CreateBranch(context.Background(), repo, "invalid branch"); err == nil {
		t.Fatal("CreateBranch() accepted an invalid branch name")
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

// The first porcelain record of an unstaged change starts with a space, and
// paths outside ASCII used to arrive C-quoted; both used to corrupt the first
// reported path.
func TestInspectReportsVerbatimPathsForUnstagedAndRenamedFiles(t *testing.T) {
	inspector := gitrepo.New("")
	repo := t.TempDir()
	for _, args := range [][]string{
		{"init", repo},
		{"-C", repo, "config", "user.name", "OneCatch Test"},
		{"-C", repo, "config", "user.email", "onecatch@example.test"},
	} {
		if output, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, output)
		}
	}
	for _, name := range []string{"cmd/progress.md", "文档/说明.md", "docs/old.md"} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(repo, name)), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(repo, name), []byte("first\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := inspector.StageAll(context.Background(), repo); err != nil {
		t.Fatal(err)
	}
	if _, err := inspector.Commit(context.Background(), repo, "feat: seed the workspace"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"cmd/progress.md", "文档/说明.md"} {
		if err := os.WriteFile(filepath.Join(repo, name), []byte("first\nsecond\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if output, err := exec.Command("git", "-C", repo, "mv", "docs/old.md", "docs/new.md").CombinedOutput(); err != nil {
		t.Fatalf("git mv: %v: %s", err, output)
	}
	snapshot, err := inspector.Inspect(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]string{}
	for _, file := range snapshot.Files {
		found[file.Path] = file.Index + file.Worktree
	}
	if len(snapshot.Files) != 3 {
		t.Fatalf("files = %+v", snapshot.Files)
	}
	if found["cmd/progress.md"] != " M" {
		t.Fatalf("cmd/progress.md status = %q in %+v", found["cmd/progress.md"], snapshot.Files)
	}
	if found["文档/说明.md"] != " M" {
		t.Fatalf("文档/说明.md status = %q in %+v", found["文档/说明.md"], snapshot.Files)
	}
	if found["docs/new.md"] != "R " {
		t.Fatalf("docs/new.md status = %q in %+v", found["docs/new.md"], snapshot.Files)
	}
	worktreeDiff, err := inspector.Diff(context.Background(), repo, false)
	if err != nil || !strings.Contains(worktreeDiff, "+++ b/文档/说明.md") {
		t.Fatalf("worktree diff = %q, %v", worktreeDiff, err)
	}
}
