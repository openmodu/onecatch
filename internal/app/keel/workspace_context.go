package keeldesktop

import (
	"context"
	"reflect"
	"time"

	"github.com/dyike/keel/ui/core"
)

func (a *app) workspaceContext() string {
	if a.task != nil && a.task.Worktree != nil {
		return "task:" + a.task.ID
	}
	if a.task == nil && a.workbench.WorktreeMode == "existing" && a.workbench.Worktree != nil {
		return a.workbench.Worktree.ContextID
	}
	return a.workspaceID
}

func (a *app) loadContextGit() {
	a.followActiveInspectorSource()
	id := a.workspaceContext()
	a.captionVersion++
	version := a.captionVersion
	changed := a.captionContext != id
	a.captionContext = id
	if changed {
		a.captionGit = nil
	}
	a.captionLoading = false
	if (a.workbench.Visible || a.workbench.DetachedWindow != nil) && a.workbench.Context != id {
		a.loadInspector()
	}
	if id == "" || a.backend == nil {
		return
	}
	a.captionLoading = true
	go func() {
		ctx, cancel := context.WithTimeout(a.ctx, 8*time.Second)
		defer cancel()
		snapshot, err := a.backend.service.GitStatus(ctx, id)
		core.UpdateIf(func() bool {
			if a.closed || a.captionVersion != version || a.workspaceContext() != id {
				return false
			}
			a.captionLoading = false
			if err == nil && !reflect.DeepEqual(a.captionGit, &snapshot) {
				a.captionGit = &snapshot
				a.buildWorktreeMenu()
				return true
			}
			return false
		})
	}()
}

// A queued task can start after it has already been opened in the UI. Attach
// its stream without resetting the conversation, typed follow-up, or history.
func (a *app) attachQueuedRun(taskID string) {
	a.loading = true
	version := a.selection
	go func() {
		runs, err := a.backend.service.ListRunsByTask(a.ctx, taskID)
		core.Update(func() {
			if a.closed || version != a.selection || a.taskID != taskID || a.runID != "" {
				return
			}
			a.loading = false
			a.fail(err)
			if len(runs) > 0 {
				a.openRun(runs[0].ID)
			}
		})
	}()
}

// Observe external changes and Agent writes without overlapping requests or
// refreshing hidden file buffers. All state checks run on Keel's UI queue.
func (a *app) watchWorkspace() {
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-a.ctx.Done():
				return
			case <-ticker.C:
				core.UpdateIf(func() bool {
					if a.closed || a.page != "" && a.workbench.DetachedWindow == nil {
						return false
					}
					if !a.captionLoading {
						a.loadContextGit()
					}
					if (a.workbench.Visible || a.workbench.DetachedWindow != nil) && a.workbench.Tab == "git" && !a.workbench.Loading && !a.workbench.Mutating {
						a.loadInspectorGit()
					}
					if (a.workbench.Visible || a.workbench.DetachedWindow != nil) && a.workbench.Tab == "files" {
						id := a.workspaceContext()
						if !a.fileState(id).DirectoryLoading[""] {
							a.refreshFileTree(id)
						}
					}
					return a.workbench.Visible || a.workbench.DetachedWindow != nil
				})
			}
		}
	}()
}
