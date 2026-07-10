package tasks_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	localdata "github.com/openmodu/oneshot/internal/data/local"
	domaintasks "github.com/openmodu/oneshot/internal/domain/tasks"
	domainworkspaces "github.com/openmodu/oneshot/internal/domain/workspaces"
)

func TestTasksRepoPersistsAcrossReopen(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), ".oneshot")
	store, err := localdata.OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	workspace := domainworkspaces.Workspace{
		ID:             "ws_1",
		Name:           "Oneshot",
		Path:           filepath.Join(t.TempDir(), "project"),
		DefaultSandbox: "workspace-write",
		CreatedAt:      now,
		LastOpenedAt:   now.Add(time.Minute),
	}
	if err := store.Repos.Tasks.SaveWorkspace(ctx, workspace); err != nil {
		t.Fatal(err)
	}
	task := domaintasks.Task{
		ID:          "task_1",
		WorkspaceID: workspace.ID,
		Title:       "实现持久化",
		Prompt:      "实现本地纯文件持久化",
		WorkflowID:  "review_loop",
		Status:      domaintasks.StatusReady,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := store.Repos.Tasks.SaveTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := localdata.OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	gotWorkspace, err := reopened.Repos.Tasks.GetWorkspace(ctx, workspace.ID)
	if err != nil || gotWorkspace.Path != workspace.Path || !gotWorkspace.LastOpenedAt.Equal(workspace.LastOpenedAt) {
		t.Fatalf("GetWorkspace() = %+v, %v", gotWorkspace, err)
	}
	gotTask, err := reopened.Repos.Tasks.GetTask(ctx, task.ID)
	if err != nil || gotTask != task {
		t.Fatalf("GetTask() = %+v, %v, want %+v", gotTask, err, task)
	}
	workspaces, err := reopened.Repos.Tasks.ListWorkspaces(ctx)
	if err != nil || len(workspaces) != 1 {
		t.Fatalf("ListWorkspaces() = %+v, %v", workspaces, err)
	}
	tasks, err := reopened.Repos.Tasks.ListTasks(ctx, workspace.ID)
	if err != nil || len(tasks) != 1 || tasks[0].ID != task.ID {
		t.Fatalf("ListTasks() = %+v, %v", tasks, err)
	}
	if _, err := reopened.Repos.Tasks.GetTask(ctx, "missing"); !errors.Is(err, domaintasks.ErrNotFound) {
		t.Fatalf("GetTask(missing) error = %v", err)
	}
}
