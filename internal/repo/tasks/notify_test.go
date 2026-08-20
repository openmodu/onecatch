package tasks_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	domaintasks "github.com/openmodu/onecatch/internal/domain/tasks"
	domainworkspaces "github.com/openmodu/onecatch/internal/domain/workspaces"
	repotasks "github.com/openmodu/onecatch/internal/repo/tasks"
	localdata "github.com/openmodu/onecatch/internal/repo/store/local"
)

type countingNotifier struct{ marks int }

func (c *countingNotifier) MarkDirty() { c.marks++ }

func notifyingFixture(t *testing.T) (repotasks.TasksRepo, *countingNotifier, domainworkspaces.Workspace) {
	t.Helper()
	store, err := localdata.OpenStore(filepath.Join(t.TempDir(), ".onecatch"))
	if err != nil {
		t.Fatal(err)
	}
	notifier := &countingNotifier{}
	repo := repotasks.WithNotifier(store.Repos.Tasks, notifier)
	workspace := domainworkspaces.Workspace{
		ID: "ws_1", Name: "demo", Path: t.TempDir(),
		CreatedAt: time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC),
	}
	if err := repo.SaveWorkspace(context.Background(), workspace); err != nil {
		t.Fatal(err)
	}
	notifier.marks = 0
	return repo, notifier, workspace
}

func sampleTask(workspaceID, id string) domaintasks.Task {
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	return domaintasks.Task{
		ID: id, WorkspaceID: workspaceID, Title: "demo", Prompt: "do it",
		WorkflowID: "single_agent", Status: domaintasks.StatusReady,
		CreatedAt: now, UpdatedAt: now,
	}
}

// The sidebar's task list is the reason this decorator exists: it used to stay
// fresh by re-reading every task file on a timer, and now waits to be told.
// Every write that changes what the list renders has to report itself, or a
// task appears only when something unrelated happens to fire.
func TestNotifyingRepoMarksEveryListVisibleWrite(t *testing.T) {
	ctx := context.Background()
	repo, notifier, workspace := notifyingFixture(t)

	if err := repo.SaveTask(ctx, sampleTask(workspace.ID, "task_1")); err != nil {
		t.Fatal(err)
	}
	if notifier.marks != 1 {
		t.Fatalf("marks after save = %d, want 1", notifier.marks)
	}
	if err := repo.DeleteTask(ctx, "task_1"); err != nil {
		t.Fatal(err)
	}
	if notifier.marks != 2 {
		t.Fatalf("marks after delete = %d, want 2", notifier.marks)
	}
	if err := repo.SaveWorkspace(ctx, workspace); err != nil {
		t.Fatal(err)
	}
	if notifier.marks != 3 {
		t.Fatalf("marks after workspace save = %d, want 3", notifier.marks)
	}
}

// A rejected write changed nothing, so announcing it would send the frontend to
// re-read the same files it already has.
func TestNotifyingRepoStaysQuietWhenTheWriteFails(t *testing.T) {
	ctx := context.Background()
	repo, notifier, _ := notifyingFixture(t)

	orphan := sampleTask("ws_missing", "task_1")
	if err := repo.SaveTask(ctx, orphan); err == nil {
		t.Fatal("saving a task into an unknown workspace should fail")
	}
	if err := repo.DeleteTask(ctx, "task_missing"); !errors.Is(err, domaintasks.ErrNotFound) {
		t.Fatalf("deleting an unknown task = %v, want ErrNotFound", err)
	}
	if notifier.marks != 0 {
		t.Fatalf("marks after failed writes = %d, want 0", notifier.marks)
	}
}

// Reads are the overwhelming majority of calls. One that marked the lists dirty
// would make the frontend refresh in response to its own refresh.
func TestNotifyingRepoDoesNotMarkOnReads(t *testing.T) {
	ctx := context.Background()
	repo, notifier, workspace := notifyingFixture(t)
	if err := repo.SaveTask(ctx, sampleTask(workspace.ID, "task_1")); err != nil {
		t.Fatal(err)
	}
	notifier.marks = 0

	if _, err := repo.ListTasks(ctx, workspace.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetTask(ctx, "task_1"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ListWorkspaces(ctx); err != nil {
		t.Fatal(err)
	}
	if notifier.marks != 0 {
		t.Fatalf("marks after reads = %d, want 0", notifier.marks)
	}
}

// A nil notifier is the production path for anything that does not care, so the
// decorator has to hand back a repository that still works.
func TestWithNotifierWithoutANotifierReturnsTheRepositoryUntouched(t *testing.T) {
	store, err := localdata.OpenStore(filepath.Join(t.TempDir(), ".onecatch"))
	if err != nil {
		t.Fatal(err)
	}
	if got := repotasks.WithNotifier(store.Repos.Tasks, nil); got != store.Repos.Tasks {
		t.Fatal("a nil notifier must not wrap the repository")
	}
}
