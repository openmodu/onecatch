package tasks_test

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	domaintasks "github.com/openmodu/onecatch/internal/domain/tasks"
	domainworkspaces "github.com/openmodu/onecatch/internal/domain/workspaces"
	localdata "github.com/openmodu/onecatch/internal/repo/store/local"
	taskrepo "github.com/openmodu/onecatch/internal/repo/tasks"
)

func TestTasksRepoPersistsAcrossReopen(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), ".onecatch")
	store, err := localdata.OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	workspace := domainworkspaces.Workspace{
		ID:             "ws_1",
		Name:           "OneCatch",
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
	if err != nil || !reflect.DeepEqual(gotTask, task) {
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

func TestWorkspaceListPrioritizesPinnedThenRecent(t *testing.T) {
	ctx := context.Background()
	store, err := localdata.OpenStore(filepath.Join(t.TempDir(), ".onecatch"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	base := time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)
	items := []domainworkspaces.Workspace{
		{ID: "recent", Name: "Recent", Path: filepath.Join(t.TempDir(), "recent"), CreatedAt: base, LastOpenedAt: base.Add(2 * time.Hour)},
		{ID: "pinned", Name: "Pinned", Path: filepath.Join(t.TempDir(), "pinned"), Pinned: true, CreatedAt: base, LastOpenedAt: base},
		{ID: "older", Name: "Older", Path: filepath.Join(t.TempDir(), "older"), CreatedAt: base, LastOpenedAt: base.Add(time.Hour)},
		{ID: "hidden", Name: "Hidden", Path: filepath.Join(t.TempDir(), "hidden"), Hidden: true, CreatedAt: base, LastOpenedAt: base.Add(3 * time.Hour)},
	}
	for _, item := range items {
		if err := store.Repos.Tasks.SaveWorkspace(ctx, item); err != nil {
			t.Fatal(err)
		}
	}
	got, err := store.Repos.Tasks.ListWorkspaces(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].ID != "pinned" || got[1].ID != "recent" || got[2].ID != "older" {
		t.Fatalf("ListWorkspaces() order = %+v", got)
	}
}

func TestWorkspaceRemoteLocationIncludesSSHHost(t *testing.T) {
	store, err := localdata.OpenStore(filepath.Join(t.TempDir(), ".onecatch"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	first := domainworkspaces.Workspace{ID: "remote-a", Name: "A", Path: "/srv/project", RemoteFS: &domainworkspaces.RemoteFS{Host: "host-a", Root: "/srv/project"}}
	second := domainworkspaces.Workspace{ID: "remote-b", Name: "B", Path: "/srv/project", RemoteFS: &domainworkspaces.RemoteFS{Host: "host-b", Root: "/srv/project"}}
	if err := store.Repos.Tasks.SaveWorkspace(ctx, first); err != nil {
		t.Fatal(err)
	}
	if err := store.Repos.Tasks.SaveWorkspace(ctx, second); err != nil {
		t.Fatalf("same root on another host should be allowed: %v", err)
	}
	otherUser := first
	otherUser.ID = "remote-a-other-user"
	otherUser.RemoteFS = &domainworkspaces.RemoteFS{Host: "host-a", Root: "/srv/project", Username: "other"}
	if err := store.Repos.Tasks.SaveWorkspace(ctx, otherUser); err != nil {
		t.Fatalf("same host and root for another user should be allowed: %v", err)
	}
	duplicate := first
	duplicate.ID = "remote-a-copy"
	if err := store.Repos.Tasks.SaveWorkspace(ctx, duplicate); !errors.Is(err, taskrepo.ErrWorkspacePathExists) {
		t.Fatalf("duplicate remote location error = %v", err)
	}
}
