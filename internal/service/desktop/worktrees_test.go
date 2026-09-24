package desktop

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	domaintasks "github.com/openmodu/onecatch/internal/domain/tasks"
	"github.com/openmodu/onecatch/internal/repo/workspacelock"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

func worktreeGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com", "GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

func worktreeFixture(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	root, _ = filepath.EvalSymlinks(root)
	repo := filepath.Join(root, "project")
	linked := filepath.Join(root, "任务 worktree")
	if err := os.Mkdir(repo, 0700); err != nil {
		t.Fatal(err)
	}
	worktreeGit(t, repo, "init", "-b", "main")
	if err := os.Mkdir(filepath.Join(repo, "src"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "src", "hello.txt"), []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}
	worktreeGit(t, repo, "add", ".")
	worktreeGit(t, repo, "commit", "-m", "initial")
	worktreeGit(t, repo, "worktree", "add", "-b", "feature/test", linked)
	return repo, linked
}

type worktreeEngine struct{ requests chan agentrun.Request }

func (*worktreeEngine) Available(agentrun.Runtime) bool { return true }
func (e *worktreeEngine) Run(_ context.Context, req agentrun.Request, _ agentrun.Sink) (agentrun.Result, error) {
	e.requests <- req
	return agentrun.Result{Succeeded: true, SessionID: "worktree-test", FinalMessage: `{"signal":"completed","content":"done"}`}, nil
}

func TestWorktreeTaskUsesBoundDirectoryAndSurvivesReload(t *testing.T) {
	ctx := context.Background()
	engine := &worktreeEngine{requests: make(chan agentrun.Request, 1)}
	app, store := newLocalTestApp(t, engine)
	repo, linked := worktreeFixture(t)
	project, err := app.AddWorkspace(ctx, AddWorkspaceInput{Path: filepath.Join(repo, "src")})
	if err != nil {
		t.Fatal(err)
	}
	items, err := app.GitListWorktrees(ctx, project.ID)
	if err != nil || len(items) != 2 {
		t.Fatalf("worktrees: %+v, %v", items, err)
	}
	var selection string
	for _, item := range items {
		if item.Branch == "feature/test" {
			selection = item.ContextID
			if item.Path != filepath.Join(linked, "src") {
				t.Fatalf("lost relative cwd: %+v", item)
			}
		}
	}
	task, err := app.CreateTask(ctx, CreateTaskInput{WorkspaceID: project.ID, WorktreeID: selection, WorkflowID: "single_agent", Title: "worktree", Prompt: "finish it"})
	if err != nil {
		t.Fatal(err)
	}
	saved, err := store.Repos.Tasks.GetTask(ctx, task.ID)
	if err != nil || saved.Worktree == nil || saved.WorkspaceID != project.ID {
		t.Fatalf("saved binding: %+v, %v", saved, err)
	}
	resolved, err := app.GetWorkspace(ctx, "task:"+task.ID)
	if err != nil || resolved.Path != filepath.Join(linked, "src") {
		t.Fatalf("resolved: %+v %v", resolved, err)
	}
	if err := os.WriteFile(filepath.Join(linked, "src", "changed.txt"), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := app.GitStatus(ctx, "task:"+task.ID)
	if err != nil || snapshot.Branch != "feature/test" || len(snapshot.Files) != 1 {
		t.Fatalf("status: %+v %v", snapshot, err)
	}
	main, err := app.GitStatus(ctx, project.ID)
	if err != nil || len(main.Files) != 0 {
		t.Fatalf("main changed: %+v %v", main, err)
	}
	if _, err := app.orchestrator.StartTask(ctx, task.ID); err != nil {
		t.Fatal(err)
	}
	// Use a second task for the full execute lifecycle.
	next, err := app.CreateTask(ctx, CreateTaskInput{WorkspaceID: project.ID, WorktreeID: selection, WorkflowID: "single_agent", Title: "execute", Prompt: "finish it"})
	if err != nil {
		t.Fatal(err)
	}
	run, err := app.orchestrator.StartTask(ctx, next.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.orchestrator.ExecuteRun(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case req := <-engine.requests:
		if req.Workspace != filepath.Join(linked, "src") {
			t.Fatalf("wrong agent cwd: %s", req.Workspace)
		}
	case <-time.After(time.Second):
		t.Fatal("agent not called")
	}
	if err := os.RemoveAll(linked); err != nil {
		t.Fatal(err)
	}
	if _, err := app.GetWorkspace(ctx, "task:"+task.ID); errorCode(err) != "worktree_missing" {
		t.Fatalf("missing checkout fallback: %v", err)
	}
	if _, err := app.GetRunDetail(ctx, run.ID); err != nil {
		t.Fatalf("history should remain readable: %v", err)
	}
}

func TestWorktreeLocksAndForeignSelection(t *testing.T) {
	ctx := context.Background()
	app, _ := newLocalTestApp(t, completingEngine{})
	repo, _ := worktreeFixture(t)
	project, err := app.AddWorkspace(ctx, AddWorkspaceInput{Path: repo})
	if err != nil {
		t.Fatal(err)
	}
	subproject, err := app.AddWorkspace(ctx, AddWorkspaceInput{Path: filepath.Join(repo, "src")})
	if err != nil {
		t.Fatal(err)
	}
	items, err := app.GitListWorktrees(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	var linkedID string
	for _, item := range items {
		if !item.Main {
			linkedID = item.ContextID
		}
	}
	if _, err := app.CreateTask(ctx, CreateTaskInput{WorkspaceID: subproject.ID, WorktreeID: linkedID, WorkflowID: "single_agent", Title: "invalid", Prompt: "run"}); errorCode(err) != "worktree_invalid" {
		t.Fatalf("foreign selection: %v", err)
	}
	main, err := app.resolveTaskWorkspace(ctx, domaintasks.Task{WorkspaceID: project.ID})
	if err != nil {
		t.Fatal(err)
	}
	sub, err := app.resolveTaskWorkspace(ctx, domaintasks.Task{WorkspaceID: subproject.ID})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := app.GetWorkspace(ctx, linkedID)
	if err != nil {
		t.Fatal(err)
	}
	if main.LockID != sub.LockID || main.LockID == linked.LockID {
		t.Fatalf("lock identities: %s %s %s", main.LockID, sub.LockID, linked.LockID)
	}
	locks := workspacelock.New(t.TempDir())
	release, err := locks.Acquire(ctx, main.LockID, main.Path, "run_main")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if _, err := locks.Acquire(ctx, sub.LockID, sub.Path, "run_sub"); err == nil {
		t.Fatal("same checkout must lock")
	}
	releaseLinked, err := locks.Acquire(ctx, linked.LockID, linked.Path, "run_linked")
	if err != nil {
		t.Fatal(err)
	}
	defer releaseLinked()
}

func TestWorktreeAttachmentsPreviewAndGitExclusion(t *testing.T) {
	ctx := context.Background()
	app, _ := newLocalTestApp(t, completingEngine{})
	repo, linked := worktreeFixture(t)
	project, err := app.AddWorkspace(ctx, AddWorkspaceInput{Path: repo})
	if err != nil {
		t.Fatal(err)
	}
	items, err := app.GitListWorktrees(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	var id string
	for _, item := range items {
		if !item.Main {
			id = item.ContextID
		}
	}
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "reference.png")
	if err := os.WriteFile(source, data.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	task, err := app.CreateTask(ctx, CreateTaskInput{WorkspaceID: project.ID, WorktreeID: id, WorkflowID: "single_agent", Title: "image", Prompt: "inspect", AttachmentPaths: []string{source}})
	if err != nil {
		t.Fatal(err)
	}
	if !pathWithin(task.Attachments[0].StoredPath, linked) {
		t.Fatalf("attachment not in selected checkout: %+v", task.Attachments)
	}
	preview, mime, err := app.ReadAttachmentPreview(ctx, task.Attachments[0].StoredPath)
	if err != nil || mime != "image/png" || !bytes.Equal(preview, data.Bytes()) {
		t.Fatalf("preview: %s %v", mime, err)
	}
	snapshot, err := app.GitStatus(ctx, "task:"+task.ID)
	if err != nil || len(snapshot.Files) != 0 {
		t.Fatalf("private attachment leaked into Git status: %+v %v", snapshot, err)
	}
}

func TestProjectAutoWorktreeCreatesPerSessionAndKeepsExistingBinding(t *testing.T) {
	ctx := context.Background()
	app, store := newLocalTestApp(t, completingEngine{})
	repo, _ := worktreeFixture(t)
	enabled := true
	project, err := app.AddWorkspace(ctx, AddWorkspaceInput{Path: filepath.Join(repo, "src"), AutoWorktree: &enabled})
	if err != nil {
		t.Fatal(err)
	}
	savedProject, err := store.Repos.Tasks.GetWorkspace(ctx, project.ID)
	if err != nil || !savedProject.AutoWorktree {
		t.Fatalf("preference not persisted: %+v %v", savedProject, err)
	}
	// Dirty source changes must remain here, rather than leaking into new sessions.
	if err := os.WriteFile(filepath.Join(repo, "src", "hello.txt"), []byte("uncommitted"), 0600); err != nil {
		t.Fatal(err)
	}
	create := func(mode, id string) domaintasks.Task {
		t.Helper()
		task, err := app.CreateTask(ctx, CreateTaskInput{WorkspaceID: project.ID, WorktreeMode: mode, WorktreeID: id, WorkflowID: "single_agent", Title: "session", Prompt: "work"})
		if err != nil {
			t.Fatal(err)
		}
		return task
	}
	first, second := create("", ""), create("", "")
	if first.Worktree == nil || second.Worktree == nil {
		t.Fatal("new sessions need worktrees")
	}
	if first.Worktree.Root == second.Worktree.Root || first.Worktree.Branch == second.Worktree.Branch {
		t.Fatal("sessions shared an automatically created checkout")
	}
	if !first.Worktree.Managed || !second.Worktree.Managed {
		t.Fatal("missing ownership marker")
	}
	for _, task := range []domaintasks.Task{first, second} {
		stored, err := store.Repos.Tasks.GetTask(ctx, task.ID)
		if err != nil || stored.Worktree.Path != task.Worktree.Path {
			t.Fatalf("binding not persisted: %+v %v", stored, err)
		}
		content, err := os.ReadFile(filepath.Join(task.Worktree.Path, "hello.txt"))
		if err != nil || string(content) != "hello" {
			t.Fatalf("wrong checkout content: %q %v", content, err)
		}
	}
	// Explicit choices override the enabled project default for just one session.
	direct := create("project", "")
	if direct.Worktree != nil {
		t.Fatal("explicit project directory ignored")
	}
	reused := create("", first.Worktree.ContextID)
	if reused.Worktree.Root != first.Worktree.Root {
		t.Fatal("explicit existing checkout ignored")
	}
	// Updating unrelated project fields must not reset an omitted preference.
	renamed, err := app.UpdateWorkspace(ctx, UpdateWorkspaceInput{ID: project.ID, Path: project.Path, Name: "renamed"})
	if err != nil || !renamed.AutoWorktree {
		t.Fatalf("unrelated edit lost preference: %+v %v", renamed, err)
	}
	disabled := false
	if _, err := app.UpdateWorkspace(ctx, UpdateWorkspaceInput{ID: project.ID, Path: project.Path, AutoWorktree: &disabled}); err != nil {
		t.Fatal(err)
	}
	if create("", "").Worktree != nil {
		t.Fatal("disabled preference still created a checkout")
	}
	explicit := create("new", "")
	if explicit.Worktree == nil || explicit.Worktree.Root == first.Worktree.Root {
		t.Fatal("explicit creation should override disabled default")
	}
	before, err := app.GetWorkspace(ctx, "task:"+first.ID)
	if err != nil || before.Path != first.Worktree.Path {
		t.Fatalf("old session retargeted after disabling: %+v %v", before, err)
	}
	// Starting and recovering the same session use the existing checkout.
	run, err := app.orchestrator.StartTask(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.orchestrator.ExecuteRun(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := app.orchestrator.ResumeRun(ctx, run.ID, "continue"); err != nil {
		t.Fatal(err)
	}
	after, err := app.GetWorkspace(ctx, "task:"+first.ID)
	if err != nil || after.Path != before.Path {
		t.Fatalf("run retargeted: %+v %v", after, err)
	}
	items, err := app.GitListWorktrees(ctx, project.ID)
	if err != nil || len(items) != 5 {
		t.Fatalf("unexpected worktrees, run must not create another: %+v %v", items, err)
	}
}

func TestAutomaticWorktreeFailureDoesNotRunInProject(t *testing.T) {
	ctx := context.Background()
	app, _ := newLocalTestApp(t, completingEngine{})
	repo := t.TempDir()
	worktreeGit(t, repo, "init")
	enabled := true
	project, err := app.AddWorkspace(ctx, AddWorkspaceInput{Path: repo, AutoWorktree: &enabled})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.CreateTask(ctx, CreateTaskInput{WorkspaceID: project.ID, WorkflowID: "single_agent", Title: "new", Prompt: "work"}); errorCode(err) != "worktree_create_failed" {
		t.Fatalf("expected initial commit failure: %v", err)
	}
	tasks, err := app.ListTasks(ctx, project.ID)
	if err != nil || len(tasks) != 0 {
		t.Fatalf("failed creation persisted fallback task: %+v %v", tasks, err)
	}
	if _, err := app.AddWorkspace(ctx, AddWorkspaceInput{Path: t.TempDir(), AutoWorktree: &enabled}); errorCode(err) != "worktree_git_required" {
		t.Fatalf("ordinary directory accepted automatic Git mode: %v", err)
	}
}

func TestWorktreeModeValidation(t *testing.T) {
	for _, input := range []struct{ mode, id string }{{"unknown", ""}, {"new", "some-id"}, {"project", "some-id"}, {"existing", ""}} {
		if _, err := resolveWorktreeMode(true, input.mode, input.id); err == nil {
			t.Fatalf("invalid selection accepted: %+v", input)
		}
	}
}
