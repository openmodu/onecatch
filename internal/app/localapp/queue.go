package localapp

import (
	"context"
	"sort"
	"strings"
	"time"

	domaintasks "github.com/openmodu/oneshot/internal/domain/tasks"
	domainworkflows "github.com/openmodu/oneshot/internal/domain/workflows"
)

func (a *App) EnqueueTask(ctx context.Context, taskID, confirmationToken string) (domaintasks.Task, error) {
	if err := a.validateTaskSecurity(ctx, taskID, confirmationToken); err != nil {
		return domaintasks.Task{}, err
	}
	task, err := a.store.Repos.Tasks.GetTask(ctx, strings.TrimSpace(taskID))
	if err != nil {
		return task, coded("task_not_found", "task was not found")
	}
	if task.Status != domaintasks.StatusReady && task.Status != domaintasks.StatusQueued {
		return task, coded("task_invalid_state", "only a ready task can be queued")
	}
	now := time.Now().UTC()
	if task.Queue == nil || task.Queue.State == domaintasks.QueueSuperseded {
		task.Queue = &domaintasks.QueueInfo{State: domaintasks.QueueWaiting, EnqueuedAt: now}
	}
	task.Queue.Authorized = true
	task.ExecutionMode = domaintasks.ExecutionQueued
	task.Status = domaintasks.StatusQueued
	task.UpdatedAt = now
	if err := a.store.Repos.Tasks.SaveTask(ctx, task); err != nil {
		return task, err
	}
	a.reconcileWorkspaceQueue(task.WorkspaceID)
	return a.store.Repos.Tasks.GetTask(ctx, task.ID)
}

func (a *App) QueueSnapshot(ctx context.Context, workspaceID string) ([]domaintasks.Task, error) {
	tasks, err := a.store.Repos.Tasks.ListTasks(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	queued := make([]domaintasks.Task, 0)
	for _, task := range tasks {
		if task.ExecutionMode == domaintasks.ExecutionQueued && task.Queue != nil && task.Queue.State != domaintasks.QueueSuperseded {
			queued = append(queued, task)
		}
	}
	sort.SliceStable(queued, func(i, j int) bool {
		if queued[i].Queue.EnqueuedAt.Equal(queued[j].Queue.EnqueuedAt) {
			return queued[i].ID < queued[j].ID
		}
		return queued[i].Queue.EnqueuedAt.Before(queued[j].Queue.EnqueuedAt)
	})
	return queued, nil
}

func (a *App) reconcileQueueForRun(runID string) {
	if a.rootCtx.Err() != nil {
		return
	}
	run, err := a.store.Repos.Workflows.GetRun(context.Background(), runID)
	if err != nil {
		return
	}
	task, err := a.store.Repos.Tasks.GetTask(context.Background(), run.TaskID)
	if err != nil {
		return
	}
	a.reconcileWorkspaceQueue(task.WorkspaceID)
}

// reconcileWorkspaceQueue runs one serialized FIFO scheduling pass. A paused
// or failed queued task remains the active queue head until the user resumes,
// cancels, or deletes it; completed and cancelled tasks release the next item.
func (a *App) reconcileWorkspaceQueue(workspaceID string) {
	if a.rootCtx.Err() != nil || strings.TrimSpace(workspaceID) == "" {
		return
	}
	a.queueMu.Lock()
	defer a.queueMu.Unlock()
	tasks, err := a.store.Repos.Tasks.ListTasks(context.Background(), workspaceID)
	if err != nil {
		return
	}
	for index := range tasks {
		task := &tasks[index]
		if task.ExecutionMode == domaintasks.ExecutionQueued && task.Queue != nil && task.Queue.State == domaintasks.QueueActive && (task.Status == domaintasks.StatusCompleted || task.Status == domaintasks.StatusCancelled) {
			task.Queue.State = domaintasks.QueueSuperseded
			task.UpdatedAt = time.Now().UTC()
			_ = a.store.Repos.Tasks.SaveTask(context.Background(), *task)
		}
	}
	for _, task := range tasks {
		if task.ExecutionMode == domaintasks.ExecutionImmediate && (task.Status == domaintasks.StatusRunning || task.Status == domaintasks.StatusPaused) {
			return
		}
		if task.ExecutionMode == domaintasks.ExecutionQueued && task.Queue != nil && task.Queue.State == domaintasks.QueueActive {
			switch task.Status {
			case domaintasks.StatusRunning, domaintasks.StatusPaused, domaintasks.StatusFailed, domaintasks.StatusReady:
				return
			}
		}
	}
	var candidates []domaintasks.Task
	for _, task := range tasks {
		if task.Status == domaintasks.StatusQueued && task.ExecutionMode == domaintasks.ExecutionQueued && task.Queue != nil && task.Queue.State == domaintasks.QueueWaiting && task.Queue.Authorized {
			candidates = append(candidates, task)
		}
	}
	if len(candidates) == 0 {
		return
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Queue.EnqueuedAt.Equal(candidates[j].Queue.EnqueuedAt) {
			return candidates[i].ID < candidates[j].ID
		}
		return candidates[i].Queue.EnqueuedAt.Before(candidates[j].Queue.EnqueuedAt)
	})
	task := candidates[0]
	now := time.Now().UTC()
	task.Status = domaintasks.StatusReady
	task.Queue.State = domaintasks.QueueActive
	task.Queue.ActivatedAt = now
	task.Queue.ActivationSource = "automatic"
	task.UpdatedAt = now
	if err := a.store.Repos.Tasks.SaveTask(context.Background(), task); err != nil {
		return
	}
	if _, err := a.startRunAuthorized(context.Background(), task.ID); err != nil {
		task.Status = domaintasks.StatusPaused
		task.UpdatedAt = time.Now().UTC()
		_ = a.store.Repos.Tasks.SaveTask(context.Background(), task)
	}
}

func (a *App) startRunAuthorized(ctx context.Context, taskID string) (domainworkflows.Run, error) {
	definition, resolution, err := a.resolveRunSettings(ctx, taskID)
	if err != nil {
		return domainworkflows.Run{}, err
	}
	run, err := a.orchestrator.StartTaskResolved(ctx, taskID, definition, resolution)
	if err != nil {
		return run, mapError(err)
	}
	a.dispatch(run.ID, func(runCtx context.Context) (domainworkflows.Run, error) {
		return a.orchestrator.ExecuteRun(runCtx, run.ID)
	})
	return run, nil
}
