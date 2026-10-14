package keeldesktop

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	domainworkspaces "github.com/openmodu/onecatch/internal/domain/workspaces"
	desktopservice "github.com/openmodu/onecatch/internal/service/desktop"
)

func gitFixture(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "init", "-b", "main")
	runGit(t, root, "config", "user.email", "keel-test@example.invalid")
	runGit(t, root, "config", "user.name", "Keel Test")
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", "main.go")
	runGit(t, root, "commit", "-m", "initial")
	return root
}
func runGit(t *testing.T, root string, args ...string) string {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = root
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}
func TestWorktreeChoiceCreatesTaskInSelectedCheckout(t *testing.T) {
	ctx := context.Background()
	b, err := openBackendWithEngine(ctx, t.TempDir(), testEngine{})
	if err != nil {
		t.Fatal(err)
	}
	a := newApp(b)
	defer a.close()
	projectRoot := gitFixture(t)
	workspace, err := b.service.AddWorkspace(ctx, desktopservice.AddWorkspaceInput{Path: projectRoot, Name: "Keel project"})
	if err != nil {
		t.Fatal(err)
	}
	linkedRoot, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	linked := filepath.Join(linkedRoot, "linked")
	runGit(t, projectRoot, "worktree", "add", "-b", "feature/keel", linked)
	entries, err := b.service.GitListWorktrees(ctx, workspace.ID)
	if err != nil {
		t.Fatal(err)
	}
	var selected domainworkspaces.Worktree
	for _, entry := range entries {
		if entry.Branch == "feature/keel" {
			selected = entry
		}
	}
	if selected.ContextID == "" {
		t.Fatalf("worktree not discovered: %+v", entries)
	}
	a.workspaces = []domainworkspaces.Workspace{workspace}
	a.workspaceID = workspace.ID
	a.runtime.SetValue("codex")
	a.workbench.WorktreeMode = "existing"
	a.workbench.Worktree = &selected
	input := a.createTaskInput("Use the chosen checkout")
	if input.WorktreeID != selected.ContextID || a.workspaceContext() != selected.ContextID {
		t.Fatal("choice did not enter execution context")
	}
	input.Title = "Existing checkout task"
	task, err := b.service.CreateTask(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if task.Worktree == nil || task.Worktree.Path != linked {
		t.Fatalf("task bound to wrong path: %+v", task.Worktree)
	}
	a.task = &task
	if a.workspaceContext() != "task:"+task.ID {
		t.Fatal("task execution context not locked")
	}
	if _, err = b.service.GitSwitchBranch(ctx, a.workspaceContext(), "main"); err == nil {
		t.Fatal("bound task must reject a branch switch")
	}
	a.task = nil
	a.workbench.WorktreeMode = "new"
	a.workbench.Worktree = nil
	input = a.createTaskInput("Create a new checkout")
	input.Title = "New checkout task"
	created, err := b.service.CreateTask(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if created.Worktree == nil || created.Worktree.Path == projectRoot || created.Worktree.Path == linked {
		t.Fatalf("new checkout not created: %+v", created.Worktree)
	}
	a.workbench.WorktreeMode = "project"
	input = a.createTaskInput("Use main")
	input.Title = "Project directory task"
	direct, err := b.service.CreateTask(ctx, input)
	if err != nil || direct.Worktree != nil {
		t.Fatalf("project choice created worktree: %+v, %v", direct.Worktree, err)
	}
}
func TestFileDraftsSurviveContextChangesAndRejectConcurrentWrites(t *testing.T) {
	ctx := context.Background()
	b, err := openBackendWithEngine(ctx, t.TempDir(), testEngine{})
	if err != nil {
		t.Fatal(err)
	}
	a := newApp(b)
	defer a.close()
	workspace, err := b.service.AddWorkspace(ctx, desktopservice.AddWorkspaceInput{Path: gitFixture(t), Name: "Editor"})
	if err != nil {
		t.Fatal(err)
	}
	doc, err := b.service.ReadWorkspaceFile(ctx, workspace.ID, "main.go")
	if err != nil {
		t.Fatal(err)
	}
	buffer := newFileBuffer(doc)
	buffer.Editor.SetValue("package main\n\nfunc main() { println(1) }\n")
	state := a.fileState(workspace.ID)
	state.Buffers[doc.Path] = buffer
	state.Active = doc.Path
	_ = a.fileState("different-context")
	if a.fileState(workspace.ID).Buffers[doc.Path] != buffer || !buffer.dirty() {
		t.Fatal("context switch discarded file buffer")
	}
	input := desktopservice.WriteWorkspaceFileInput{WorkspaceID: workspace.ID, Path: doc.Path, Content: buffer.Editor.Value(), ExpectedHash: doc.Hash}
	if err := os.WriteFile(filepath.Join(workspace.Path, doc.Path), []byte("package main\n// external edit\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = b.service.WriteWorkspaceFile(ctx, input); err == nil {
		t.Fatal("stale file revision overwrote external edit")
	}
	content, _ := os.ReadFile(filepath.Join(workspace.Path, doc.Path))
	if !strings.Contains(string(content), "external edit") {
		t.Fatal("conflicting save changed file")
	}
	outside := filepath.Join(t.TempDir(), "outside.go")
	os.WriteFile(outside, []byte("private"), 0600)
	if _, err = b.service.ReadWorkspaceFile(ctx, workspace.ID, outside); err == nil {
		t.Fatal("file editor escaped workspace root")
	}
}
func TestGitReviewIncludesBothScopesAndUntrackedText(t *testing.T) {
	patch := `diff --git a/file name.go b/file name.go
--- a/file name.go
+++ b/file name.go
@@ -1,2 +1,2 @@
--- old text
+++ new text
 context
`
	staged := parseGitReview(patch, "staged")
	if len(staged) != 1 || staged[0].Path != "file name.go" || staged[0].Additions != 1 || staged[0].Deletions != 1 {
		t.Fatalf("review = %+v", staged)
	}
	lines := staged[0].Hunks[0].Lines
	if lines[0].Text != "-- old text" || lines[1].Text != "++ new text" || lines[2].Old != 2 || lines[2].New != 2 {
		t.Fatalf("hunk headers confused with content: %+v", lines)
	}
	snapshot := domainworkspaces.GitSnapshot{Files: []domainworkspaces.GitFile{{Path: "file name.go", Index: "M", Worktree: "M"}, {Path: "new.go", Index: "?", Worktree: "?"}, {Path: "binary.png", Worktree: "M"}}}
	files := buildGitReview(snapshot, patch, patch, map[string]string{"new.go": "first\nsecond\n"})
	if len(files) != 3 || len(files[0].Hunks) != 2 || files[0].Hunks[1].Scope != "worktree" || files[0].Additions != 2 || files[1].Additions != 2 || len(files[2].Hunks) != 0 {
		t.Fatalf("merged review: %+v", files)
	}
	quoted := parseGitReview("diff --git \"a/\\344\\270\\255.go\" \"b/\\344\\270\\255.go\"\n--- \"a/\\344\\270\\255.go\"\n+++ \"b/\\344\\270\\255.go\"\n@@ -0,0 +1 @@\n+你好\n", "worktree")
	if len(quoted) != 1 || quoted[0].Path != "中.go" {
		t.Fatalf("quoted UTF-8 path: %+v", quoted)
	}
}
func TestProjectEditingPreservesRemoteIdentityAndDefaults(t *testing.T) {
	b, err := openBackendWithEngine(context.Background(), t.TempDir(), testEngine{})
	if err != nil {
		t.Fatal(err)
	}
	a := newApp(b)
	defer a.close()
	workspace := domainworkspaces.Workspace{ID: "remote", Name: "Remote", Path: "/srv/code", DefaultSandbox: "read-only", AutoWorktree: false, RemoteFS: &domainworkspaces.RemoteFS{Host: "server", Root: "/srv/code", Username: "user", SSHOptions: []string{"-p", "2222"}}}
	a.workspaces = []domainworkspaces.Workspace{workspace}
	a.editProject(workspace.ID)
	input, err := a.projectInput()
	if err != nil || input.RemoteFS == nil || input.RemoteFS.Host != "server" || input.DefaultSandbox != "read-only" || len(input.RemoteFS.SSHOptions) != 2 || input.Path != "/srv/code" {
		t.Fatalf("remote defaults lost: %+v, %v", input, err)
	}
	a.newProject()
	if a.project.ID != "" || a.projectPath != "" || a.project.Kind.Value() != "local" {
		t.Fatal("new project reused editing state")
	}
}
