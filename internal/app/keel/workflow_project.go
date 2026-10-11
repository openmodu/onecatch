package keeldesktop

import (
	"fmt"
	"strings"
	"time"

	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/theme"
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
				core.Update(a.loadProjectWorkflows)
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
		core.Update(func() {
			a.projectWorkflows.loading = false
			if !a.closed && err == nil {
				a.applyProjectWorkflows(id, items)
			}
		})
	}()
}

// applyProjectWorkflows takes a fresh read of workspace id's workflow files.
// It changes nothing when no file changed, so views keep their state.
func (a *app) applyProjectWorkflows(id string, items []desktopservice.ProjectWorkflow) {
	s := &a.projectWorkflows
	if a.workspaceID != id {
		return
	}
	signature := id
	for _, item := range items {
		signature += "|" + item.File + "@" + item.Revision + "!" + item.Error
	}
	if s.workspaceID == id && s.signature == signature {
		return
	}
	s.workspaceID, s.signature, s.items = id, signature, items
	a.syncWorkflowEntries()
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

// openProjectWorkflowFile shows a workflow file in the files panel, at line
// when it is the line to fix.
func (a *app) openProjectWorkflowFile(file string, line int) {
	a.closeSettings()
	a.page = ""
	a.workbench.Visible, a.workbench.Tab = true, "files"
	a.openFileMatch(a.workspaceID, desktopservice.WorkspaceFileMatch{Path: file, Line: line})
}

func (a *app) exportWorkflowToProject(id string) {
	workspaceID := a.workspaceID
	if workspaceID == "" {
		a.fail(fmt.Errorf("先打开一个项目，再把工作流导出到项目"))
		return
	}
	go func() {
		path, err := a.backend.service.ExportWorkflowToProject(a.ctx, workspaceID, id)
		core.Update(func() {
			if a.closed {
				return
			}
			a.fail(err)
			if err == nil {
				a.projectWorkflows.signature = ""
				a.loadProjectWorkflows()
				a.errorText = ""
				_ = path
			}
		})
	}()
}

// projectWorkflowSection lists the open project's workflow files in the
// workflow library, each drawn from its file.
func (a *app) projectWorkflowSection(cx *el.Context) el.Element {
	section := el.Div().Gap(4).Pb(12)
	name := a.workspaceForID(a.workspaceID).Name
	if a.workspaceID == "" {
		return section.Child(el.Text("未选择项目").TextSize(12).TextColor(theme.Muted).Py(8))
	}
	if len(a.projectWorkflows.items) == 0 || a.projectWorkflows.workspaceID != a.workspaceID {
		return section.Child(el.Text(name + " · 暂无项目工作流").TextSize(12).TextColor(theme.Muted))
	}
	section.Child(el.Text("项目工作流 · " + name).TextSize(13).Medium())
	for _, item := range a.projectWorkflows.items {
		item := item
		content := el.Div().Grow().W(el.Dp(0)).Gap(6)
		open := button("打开文件", func() { a.openProjectWorkflowFile(item.File, item.ErrorLine) }, kit.ButtonGhost).Size(30).Compact(true)
		if item.Error != "" {
			content.Child(el.Div().Row().Items(el.Center).Gap(8).Child(el.Text(item.File).Mono().TextSize(12).Medium(), el.Text("无法使用").TextSize(11).TextColor(theme.DangerText)))
			message := item.Error
			if item.ErrorLine > 0 {
				message = fmt.Sprintf("第 %d 行：%s", item.ErrorLine, item.Error)
			}
			content.Child(el.Text(message).TextSize(12).TextColor(theme.DangerText).MaxLines(3))
		} else {
			d := item.Definition
			content.Child(el.Div().Row().Items(el.Center).Gap(8).Child(el.Text(d.Name).Medium().TextSize(14).MaxLines(1), el.Text(item.File).Mono().TextSize(11).TextColor(theme.Muted).MaxLines(1)))
			if d.Description != "" {
				content.Child(el.Text(d.Description).TextSize(12).TextColor(theme.Muted).MaxLines(2))
			}
			content.Child(el.Text(workflowSummary(d)).TextSize(11).TextColor(theme.Muted).MaxLines(1),
				el.Div().WFull().NoShrink().ScrollX().Pt(4).Child(flowChart(cx, d, flowChartOptions{ID: "project-chart/" + d.ID})))
		}
		section.Child(el.Div().Row().Items(el.Start).Gap(12).Py(14).Child(
			el.Div().Size(el.Dp(32)).Rounded(8).Bg(theme.Subtle).Center().Child(lucide(cx, "file-code-2", 16, theme.Muted)),
			content, open.Render(cx)), el.Div().H(el.Dp(1)).WFull().Bg(seamColor).NoShrink())
	}
	return section
}
