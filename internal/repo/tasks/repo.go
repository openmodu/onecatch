package tasks

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	domaintasks "github.com/openmodu/oneshot/internal/domain/tasks"
	domainworkspaces "github.com/openmodu/oneshot/internal/domain/workspaces"
	"github.com/openmodu/oneshot/pkg/localfile"
)

var ErrWorkspacePathExists = errors.New("workspace path already exists")

type TasksRepo interface {
	SaveWorkspace(context.Context, domainworkspaces.Workspace) error
	GetWorkspace(context.Context, string) (domainworkspaces.Workspace, error)
	ListWorkspaces(context.Context) ([]domainworkspaces.Workspace, error)
	SaveTask(context.Context, domaintasks.Task) error
	GetTask(context.Context, string) (domaintasks.Task, error)
	ListTasks(context.Context, string) ([]domaintasks.Task, error)
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
		if item.ID != workspace.ID && filepath.Clean(item.Path) == filepath.Clean(workspace.Path) {
			return ErrWorkspacePathExists
		}
	}
	if err := localfile.WriteJSONAtomic(r.workspacePath(workspace.ID), workspace); err != nil {
		return fmt.Errorf("save workspace: %w", err)
	}
	return nil
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

func (r *tasksImpl) GetTask(ctx context.Context, id string) (domaintasks.Task, error) {
	if err := ctx.Err(); err != nil {
		return domaintasks.Task{}, err
	}
	if !localfile.ValidID(id) {
		return domaintasks.Task{}, domaintasks.ErrNotFound
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
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
			out = append(out, task)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out, nil
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
		out = append(out, workspace)
	}
	sort.Slice(out, func(i, j int) bool {
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
