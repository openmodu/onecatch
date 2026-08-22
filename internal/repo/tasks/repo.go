package tasks

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	domaintasks "github.com/openmodu/onecatch/internal/domain/tasks"
	domainworkspaces "github.com/openmodu/onecatch/internal/domain/workspaces"
	"github.com/openmodu/onecatch/pkg/localfile"
)

var ErrWorkspacePathExists = errors.New("workspace path already exists")

type TasksRepo interface {
	SaveWorkspace(context.Context, domainworkspaces.Workspace) error
	GetWorkspace(context.Context, string) (domainworkspaces.Workspace, error)
	ListWorkspaces(context.Context) ([]domainworkspaces.Workspace, error)
	SaveTask(context.Context, domaintasks.Task) error
	UpdateTaskTitle(context.Context, string, string, string, time.Time) (bool, error)
	UpdateTaskStatus(context.Context, string, domaintasks.Status, time.Time) (domaintasks.Task, error)
	GetTask(context.Context, string) (domaintasks.Task, error)
	ListTasks(context.Context, string) ([]domaintasks.Task, error)
	DeleteTask(context.Context, string) error
}

type tasksImpl struct {
	workspacesRoot string
	tasksRoot      string
	mu             sync.RWMutex
}

func NewTasksRepo(workspacesRoot, tasksRoot string) TasksRepo {
	return &tasksImpl{workspacesRoot: workspacesRoot, tasksRoot: tasksRoot}
}

func (r *tasksImpl) SaveWorkspace(ctx context.Context, workspace domainworkspaces.Workspace) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := domainworkspaces.Validate(workspace); err != nil {
		return err
	}
	if !localfile.ValidID(workspace.ID) {
		return domainworkspaces.ErrInvalid
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	items, err := r.listWorkspacesLocked()
	if err != nil {
		return err
	}
	for _, item := range items {
		if item.ID != workspace.ID && sameWorkspaceLocation(item, workspace) {
			return ErrWorkspacePathExists
		}
	}
	if err := localfile.WriteJSONAtomic(r.workspacePath(workspace.ID), workspace); err != nil {
		return fmt.Errorf("save workspace: %w", err)
	}
	return nil
}

func sameWorkspaceLocation(left, right domainworkspaces.Workspace) bool {
	if left.RemoteFS == nil || right.RemoteFS == nil {
		return left.RemoteFS == nil && right.RemoteFS == nil && filepath.Clean(left.Path) == filepath.Clean(right.Path)
	}
	return strings.EqualFold(strings.TrimSpace(left.RemoteFS.Host), strings.TrimSpace(right.RemoteFS.Host)) &&
		strings.EqualFold(strings.TrimSpace(left.RemoteFS.Username), strings.TrimSpace(right.RemoteFS.Username)) &&
		path.Clean(left.RemoteFS.Root) == path.Clean(right.RemoteFS.Root)
}

func (r *tasksImpl) GetWorkspace(ctx context.Context, id string) (domainworkspaces.Workspace, error) {
	if err := ctx.Err(); err != nil {
		return domainworkspaces.Workspace{}, err
	}
	if !localfile.ValidID(id) {
		return domainworkspaces.Workspace{}, domainworkspaces.ErrNotFound
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.getWorkspaceLocked(id)
}

func (r *tasksImpl) ListWorkspaces(ctx context.Context) ([]domainworkspaces.Workspace, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.listWorkspacesLocked()
}

func (r *tasksImpl) SaveTask(ctx context.Context, task domaintasks.Task) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := domaintasks.Validate(task); err != nil {
		return err
	}
	if !localfile.ValidID(task.ID) || !localfile.ValidID(task.WorkspaceID) {
		return domaintasks.ErrInvalid
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, err := r.getWorkspaceLocked(task.WorkspaceID); err != nil {
		return err
	}
	if err := localfile.WriteJSONAtomic(r.taskPath(task.ID), task); err != nil {
		return fmt.Errorf("save task: %w", err)
	}
	return nil
}

// UpdateTaskTitle replaces only the title when it still matches expected.
// Keeping the comparison and write under the repository lock prevents an
// asynchronous title refinement from overwriting concurrent task state writes.
func (r *tasksImpl) UpdateTaskTitle(ctx context.Context, id, expected, title string, updatedAt time.Time) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if !localfile.ValidID(id) {
		return false, domaintasks.ErrNotFound
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	task, err := r.getTaskLocked(id)
	if err != nil {
		return false, err
	}
	if task.Title != expected {
		return false, nil
	}
	task.Title = strings.TrimSpace(title)
	task.UpdatedAt = updatedAt
	if err := domaintasks.Validate(task); err != nil {
		return false, err
	}
	if err := localfile.WriteJSONAtomic(r.taskPath(task.ID), task); err != nil {
		return false, fmt.Errorf("update task title: %w", err)
	}
	return true, nil
}

// UpdateTaskStatus changes only execution state, preserving metadata that may
// have been updated while a long-running Agent held an older task snapshot.
func (r *tasksImpl) UpdateTaskStatus(ctx context.Context, id string, status domaintasks.Status, updatedAt time.Time) (domaintasks.Task, error) {
	if err := ctx.Err(); err != nil {
		return domaintasks.Task{}, err
	}
	if !localfile.ValidID(id) {
		return domaintasks.Task{}, domaintasks.ErrNotFound
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	task, err := r.getTaskLocked(id)
	if err != nil {
		return domaintasks.Task{}, err
	}
	task.Status = status
	task.UpdatedAt = updatedAt
	if err := domaintasks.Validate(task); err != nil {
		return domaintasks.Task{}, err
	}
	if err := localfile.WriteJSONAtomic(r.taskPath(task.ID), task); err != nil {
		return domaintasks.Task{}, fmt.Errorf("update task status: %w", err)
	}
	return task, nil
}

func (r *tasksImpl) GetTask(ctx context.Context, id string) (domaintasks.Task, error) {
	if err := ctx.Err(); err != nil {
		return domaintasks.Task{}, err
	}
	if !localfile.ValidID(id) {
		return domaintasks.Task{}, domaintasks.ErrNotFound
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.getTaskLocked(id)
}

func (r *tasksImpl) getTaskLocked(id string) (domaintasks.Task, error) {
	var task domaintasks.Task
	if err := localfile.ReadJSON(r.taskPath(id), &task); errors.Is(err, os.ErrNotExist) {
		return domaintasks.Task{}, domaintasks.ErrNotFound
	} else if err != nil {
		return domaintasks.Task{}, fmt.Errorf("get task: %w", err)
	}
	return task, nil
}

func (r *tasksImpl) ListTasks(ctx context.Context, workspaceID string) ([]domaintasks.Task, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	entries, err := os.ReadDir(r.tasksRoot)
	if err != nil {
		return nil, fmt.Errorf("list task files: %w", err)
	}
	var out []domaintasks.Task
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		var task domaintasks.Task
		if err := localfile.ReadJSON(filepath.Join(r.tasksRoot, entry.Name()), &task); err != nil {
			return nil, fmt.Errorf("read task %s: %w", entry.Name(), err)
		}
		if workspaceID == "" || task.WorkspaceID == workspaceID {
			if !task.DeletedAt.IsZero() {
				continue
			}
			out = append(out, task)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Pinned != out[j].Pinned {
			return out[i].Pinned
		}
		if out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out, nil
}

func (r *tasksImpl) DeleteTask(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !localfile.ValidID(id) {
		return domaintasks.ErrNotFound
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	var task domaintasks.Task
	if err := localfile.ReadJSON(r.taskPath(id), &task); errors.Is(err, os.ErrNotExist) {
		return domaintasks.ErrNotFound
	} else if err != nil {
		return err
	}
	task.DeletedAt = time.Now().UTC()
	task.UpdatedAt = task.DeletedAt
	return localfile.WriteJSONAtomic(r.taskPath(id), task)
}

func (r *tasksImpl) getWorkspaceLocked(id string) (domainworkspaces.Workspace, error) {
	var workspace domainworkspaces.Workspace
	if err := localfile.ReadJSON(r.workspacePath(id), &workspace); errors.Is(err, os.ErrNotExist) {
		return domainworkspaces.Workspace{}, domainworkspaces.ErrNotFound
	} else if err != nil {
		return domainworkspaces.Workspace{}, fmt.Errorf("get workspace: %w", err)
	}
	return workspace, nil
}

func (r *tasksImpl) listWorkspacesLocked() ([]domainworkspaces.Workspace, error) {
	entries, err := os.ReadDir(r.workspacesRoot)
	if err != nil {
		return nil, fmt.Errorf("list workspace files: %w", err)
	}
	var out []domainworkspaces.Workspace
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		var workspace domainworkspaces.Workspace
		if err := localfile.ReadJSON(filepath.Join(r.workspacesRoot, entry.Name()), &workspace); err != nil {
			return nil, fmt.Errorf("read workspace %s: %w", entry.Name(), err)
		}
		if workspace.Hidden {
			continue
		}
		out = append(out, workspace)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Pinned != out[j].Pinned {
			return out[i].Pinned
		}
		if out[i].LastOpenedAt.Equal(out[j].LastOpenedAt) {
			return strings.Compare(out[i].Name, out[j].Name) < 0
		}
		return out[i].LastOpenedAt.After(out[j].LastOpenedAt)
	})
	return out, nil
}

func (r *tasksImpl) workspacePath(id string) string {
	return filepath.Join(r.workspacesRoot, id+".json")
}

func (r *tasksImpl) taskPath(id string) string {
	return filepath.Join(r.tasksRoot, id+".json")
}
