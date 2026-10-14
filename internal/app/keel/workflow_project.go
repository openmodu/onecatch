package keeldesktop

import (
	"strings"
	"time"

	"github.com/dyike/keel/ui/core"
	domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"
	desktopservice "github.com/openmodu/onecatch/internal/service/desktop"
)

// Project workflows are YAML files in the open project's
// .onecatch/workflows. They are read-only in the app: the file is the source,
// and every view re-renders shortly after the file changes on disk.
type projectWorkflowState struct {
	workspaceID, signature string
	items                  []desktopservice.ProjectWorkflow
	loading                bool
}

const projectWorkflowPoll = 2 * time.Second

// watchProjectWorkflows keeps the open project's workflow files current.
func (a *app) watchProjectWorkflows() {
	go func() {
		ticker := time.NewTicker(projectWorkflowPoll)
		defer ticker.Stop()
		for {
			select {
			case <-a.ctx.Done():
				return
			case <-ticker.C:
				core.UpdateIf(func() bool {
					a.loadProjectWorkflows()
					return a.checkSessionWorkflow(false)
				})
			}
		}
	}()
}

// loadProjectWorkflows reads the open project's workflow files and replaces
// the cached list only when a file was added, removed or changed.
func (a *app) loadProjectWorkflows() {
	s := &a.projectWorkflows
	id := a.workspaceID
	if a.closed || s.loading {
		return
	}
	if id == "" {
		*s = projectWorkflowState{}
		return
	}
	s.loading = true
	go func() {
		items, err := a.backend.service.ListProjectWorkflows(a.ctx, id)
		core.UpdateIf(func() bool {
			a.projectWorkflows.loading = false
			if !a.closed && err == nil {
				return a.applyProjectWorkflows(id, items)
			}
			return false
		})
	}()
}

// applyProjectWorkflows takes a fresh read of workspace id's workflow files.
// It changes nothing when no file changed, so views keep their state.
func (a *app) applyProjectWorkflows(id string, items []desktopservice.ProjectWorkflow) bool {
	s := &a.projectWorkflows
	if a.workspaceID != id {
		return false
	}
	signature := id
	for _, item := range items {
		signature += "|" + item.File + "@" + item.Revision + "!" + item.Error
	}
	if s.workspaceID == id && s.signature == signature {
		return false
	}
	s.workspaceID, s.signature, s.items = id, signature, items
	a.syncWorkflowEntries()
	return true
}

// projectDefinitions returns the usable workflow files of the open project.
func (a *app) projectDefinitions() []domainworkflows.Definition {
	if a.projectWorkflows.workspaceID != a.workspaceID {
		return nil
	}
	var out []domainworkflows.Definition
	for _, item := range a.projectWorkflows.items {
		if item.Error == "" {
			out = append(out, item.Definition)
		}
	}
	return out
}

// selectableDefinitions lists every workflow a new task in this project can
// use: the project's files first, then the app's own workflows.
func (a *app) selectableDefinitions() []domainworkflows.Definition {
	out := a.projectDefinitions()
	for _, d := range a.definitions {
		if d.ID != "single_agent" {
			out = append(out, d)
		}
	}
	return out
}

func (a *app) findDefinition(id string) (domainworkflows.Definition, bool) {
	for _, d := range a.selectableDefinitions() {
		if d.ID == id {
			return d, true
		}
	}
	return domainworkflows.Definition{}, false
}

func isProjectWorkflow(id string) bool { return strings.HasPrefix(id, "repo-") }
