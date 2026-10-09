package keeldesktop

import (
	"context"
	"encoding/json"

	"github.com/dyike/keel/ui/kit"
	domainworkspaces "github.com/openmodu/onecatch/internal/domain/workspaces"
	desktopservice "github.com/openmodu/onecatch/internal/service/desktop"
	"github.com/openmodu/onecatch/internal/service/worker"
)

// The same latest-step rule as inspectorContext.activeWorkerID. Writable
// remote turns return their patches locally and clean up the remote clone.
func activeInspectorWorkerID(detail *desktopservice.RunDetail) string {
	if detail == nil {
		return ""
	}
	stepID := detail.Run.CurrentStepID
	if len(detail.StepRuns) > 0 && detail.StepRuns[len(detail.StepRuns)-1].StepID != "" {
		stepID = detail.StepRuns[len(detail.StepRuns)-1].StepID
	}
	for _, step := range detail.Workflow.Steps {
		if step.ID == stepID && step.Sandbox == "read-only" && step.WorkerID != "local" {
			return step.WorkerID
		}
	}
	return ""
}

// Periodic refreshes and token updates must not undo a manual override. The
// docked, maximized and detached inspector all share this app state.
func (a *app) syncInspectorGitSource() bool {
	w := &a.workbench
	id := a.workspaceContext()
	var workers []worker.Info
	project := a.workspaceForID(id)
	if !worktreeBound(id) && project.RemoteFS == nil {
		for _, entry := range a.workers {
			if entry.Enabled {
				workers = append(workers, entry)
			}
		}
	}
	w.Workers = workers
	active := ""
	if a.task != nil {
		active = activeInspectorWorkerID(a.detail)
	}
	// The registry signature stays private; it may contain a sandbox URL.
	data, _ := json.Marshal(struct {
		Context, Active string
		Workers         []worker.Info
	}{id, active, workers})
	key := string(data)
	if w.SourceFollowKey == key {
		return false
	}
	w.SourceFollowKey = key
	source := "local"
	choices := []kit.SelectOption{{Value: "local", Label: "本地"}}
	for _, entry := range workers {
		choices = append(choices, kit.SelectOption{Value: entry.ID, Label: entry.Name})
		if entry.ID == active {
			source = active
		}
	}
	w.Source, w.NewBranch, w.Error = source, "", ""
	w.Snapshot, w.Branches, w.Loading = nil, nil, false
	w.Version++ // Reject a result fetched for the previous step or registry.
	w.SourcePicker.SetEntries(choices...)
	w.SourcePicker.SetValue(source)
	return true
}

type inspectorGitResult struct {
	snapshot domainworkspaces.GitSnapshot
	branches []domainworkspaces.GitBranch
	err      error
}

func readInspectorGit(ctx context.Context, service *desktopservice.Service, id, source string) inspectorGitResult {
	var result inspectorGitResult
	if source == "local" {
		result.snapshot, result.err = service.GitStatus(ctx, id)
		if result.err == nil && result.snapshot.IsRepo && !worktreeBound(id) {
			result.branches, result.err = service.GitListBranches(ctx, id)
		}
	} else {
		result.snapshot, result.err = service.WorkerGitStatus(ctx, source, id)
	}
	return result
}

func (a *app) applyInspectorGit(version uint64, id, source string, result inspectorGitResult) {
	w := &a.workbench
	if a.closed || w.Version != version || a.workspaceContext() != id || w.Source != source {
		return
	}
	w.Loading = false
	if result.err != nil {
		w.Error = safeWorkerError(result.err, "")
		return
	}
	w.Snapshot, w.Branches = &result.snapshot, result.branches
	if source == "local" {
		a.captionGit = &result.snapshot
	}
	opts := make([]kit.SelectOption, 0, len(result.branches))
	for _, branch := range result.branches {
		opts = append(opts, kit.SelectOption{Value: branch.Name, Label: branch.Name})
	}
	if len(opts) == 0 {
		opts = append(opts, kit.SelectOption{Value: result.snapshot.Branch, Label: gitBranchLabel(&result.snapshot), Disabled: true})
	}
	w.BranchPicker.SetEntries(opts...)
	w.BranchPicker.SetValue(result.snapshot.Branch)
}

func (a *app) followActiveInspectorSource() {
	if a.syncInspectorGitSource() && !a.closed && (a.workbench.Visible || a.workbench.DetachedWindow != nil) && a.workbench.Tab == "git" {
		a.loadInspectorGit()
	}
}

func (a *app) selectInspectorGitSource(id string) {
	w := &a.workbench
	valid := id == "local"
	for _, entry := range w.Workers {
		valid = valid || entry.ID == id && entry.Enabled
	}
	if !valid || a.closed || w.Mutating || id == w.Source {
		w.SourcePicker.SetValue(w.Source)
		return
	}
	w.Source, w.NewBranch, w.Error = id, "", ""
	w.Snapshot, w.Branches = nil, nil
	w.Version++
	w.SourcePicker.SetValue(id)
	a.loadInspectorGit()
}
