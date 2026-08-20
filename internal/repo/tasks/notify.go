package tasks

import (
	"context"

	domaintasks "github.com/openmodu/onecatch/internal/domain/tasks"
	domainworkspaces "github.com/openmodu/onecatch/internal/domain/workspaces"
)

// Notifier receives a mark whenever a write changed something the task or
// workspace lists render.
type Notifier interface {
	MarkDirty()
}

// notifyingRepo wraps a TasksRepo and reports every mutation the sidebar can
// see. Decorating the repository keeps the notification in one place: tasks are
// created, renamed, pinned, queued and deleted from a dozen handlers, and every
// one of them lands here.
type notifyingRepo struct {
	TasksRepo
	notify Notifier
}

// WithNotifier returns repo decorated so list-visible mutations mark it dirty.
// A nil notifier returns the repo untouched.
func WithNotifier(repo TasksRepo, notify Notifier) TasksRepo {
	if repo == nil || notify == nil {
		return repo
	}
	return &notifyingRepo{TasksRepo: repo, notify: notify}
}

func (r *notifyingRepo) SaveTask(ctx context.Context, task domaintasks.Task) error {
	if err := r.TasksRepo.SaveTask(ctx, task); err != nil {
		return err
	}
	r.notify.MarkDirty()
	return nil
}

func (r *notifyingRepo) DeleteTask(ctx context.Context, id string) error {
	if err := r.TasksRepo.DeleteTask(ctx, id); err != nil {
		return err
	}
	r.notify.MarkDirty()
	return nil
}

func (r *notifyingRepo) SaveWorkspace(ctx context.Context, workspace domainworkspaces.Workspace) error {
	if err := r.TasksRepo.SaveWorkspace(ctx, workspace); err != nil {
		return err
	}
	r.notify.MarkDirty()
	return nil
}
