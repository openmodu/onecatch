package keeldesktop

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/theme"
	domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"
	desktopservice "github.com/openmodu/onecatch/internal/service/desktop"
)

// starterWorkflow returns a new draft: "blank" is a single step, any other
// kind copies the built-in workflow with that ID.
func starterWorkflow(kind string) domainworkflows.Definition {
	for _, template := range desktopservice.WorkflowTemplates() {
		if template.ID == kind {
			template.ID = "my_" + template.ID
			template.Name = "我的" + template.Name
			return domainworkflows.Normalize(template)
		}
	}
	return domainworkflows.Normalize(domainworkflows.Definition{ID: "my_workflow", Name: "我的工作流", Steps: []domainworkflows.Step{
		{ID: "execute", Name: "执行", Runtime: "codex", Sandbox: "workspace-write"},
	}})
}

func (a *app) newWorkflowMenu(cx *el.Context) el.Element {
	if a.ui.WorkflowNew == nil {
		menu := kit.Menu().Width(220).Placement(el.Bottom, el.End)
		menu.ContentItem("空白", "", menuIcon("square", "空白"), func() { a.newWorkflowFrom("blank") })
		for _, template := range desktopservice.WorkflowTemplates() {
			id := template.ID
			menu.ContentItem(template.Name, "", menuIcon("workflow", template.Name), func() { a.newWorkflowFrom(id) })
		}
		menu.Trigger(el.ViewFunc(func(cx *el.Context) el.Element {
			return iconAction(cx, "new-workflow", "新建工作流", "plus", menu.Toggle, menu.Value())
		}))
		a.ui.WorkflowNew = menu
	}
	return a.ui.WorkflowNew.Render(cx)
}

// workflowPath reads a graph left to right: steps that can run together are
// joined with ∥, and ↺ marks a workflow that can send work back.
func workflowPath(d domainworkflows.Definition) string {
	levels := dagAutoLayout(d)
	columns := map[float64][]string{}
	var xs []float64
	for _, s := range d.Steps {
		x := levels[s.ID].X
		if _, ok := columns[x]; !ok {
			xs = append(xs, x)
		}
		columns[x] = append(columns[x], s.Name)
	}
	sort.Float64s(xs)
	parts := make([]string, 0, len(xs))
	for _, x := range xs {
		parts = append(parts, strings.Join(columns[x], " ∥ "))
	}
	path := strings.Join(parts, " → ")
	for _, e := range d.Edges {
		if e.Kind == domainworkflows.EdgeRework {
			return path + " ↺"
		}
	}
	return path
}

func workflowSummary(d domainworkflows.Definition) string {
	rework := 0
	for _, e := range d.Edges {
		if e.Kind == domainworkflows.EdgeRework {
			rework++
		}
	}
	if rework == 0 {
		return fmt.Sprintf("%d 个步骤", len(d.Steps))
	}
	return fmt.Sprintf("%d 个步骤 · %d 条返工 · 每条最多 %d 轮", len(d.Steps), rework, d.Policy.MaxReworkRounds)
}

// workflowSettings is the Settings section for workflows: the library, or the
// canvas editor for the workflow being edited. Workflows live only here.
func (a *app) workflowSettings(cx *el.Context) el.Element {
	if !a.ui.WorkflowSource {
		return a.workflowLibrary(cx)
	}
	back := button("返回工作流", func() { a.ui.WorkflowSource, a.ui.WorkflowFullscreen = false, false }, kit.ButtonGhost).Size(30).Compact(true)
	toolbar := el.Div().Row().Items(el.Center).H(el.Dp(40)).NoShrink().Child(back.Render(cx), el.Div().Grow(), el.Text(a.ui.WorkflowDraft.Name).MaxLines(1).TextSize(12).TextColor(theme.Muted), a.newWorkflowMenu(cx))
	return el.Div().Grow().H(el.Dp(0)).Child(toolbar, a.dagEditor(cx))
}

func (a *app) newWorkflow() {
	a.newWorkflowFrom("implement_review")
}
func (a *app) newWorkflowFrom(kind string) {
	before := a.currentLocation()
	a.ui.WorkflowSource = true
	a.editingWorkflow = ""
	a.ui.SelectedWorkflow = ""
	d := starterWorkflow(kind)
	base := d.ID
	for suffix := 2; ; suffix++ {
		occupied := false
		for _, existing := range a.definitions {
			if existing.ID == d.ID {
				occupied = true
				break
			}
		}
		if !occupied {
			break
		}
		d.ID = fmt.Sprintf("%s_%d", base, suffix)
	}
	if a.settings.SchemaVersion > 0 {
		e := a.settings.Execution
		d.Policy = domainworkflows.Policy{MaxReworkRounds: e.MaxReworkRounds, MaxConsecutiveFailures: e.MaxConsecutiveFailures, StepTimeoutSeconds: e.StepTimeoutSeconds}
	}
	a.ui.WorkflowDraft = domainworkflows.Normalize(d)
	a.ui.WorkflowRaw, a.ui.WorkflowStep = false, 0
	a.resetWorkflowForm()
	data, _ := json.MarshalIndent(d, "", "  ")
	a.workflowEditor.SetValue(string(data))
	a.workflowEditor.SetReadOnly(false)
	a.recordNavigation(before)
}
func (a *app) deleteWorkflow(id string) {
	if id == "single_agent" {
		return
	}
	a.ui.WorkflowConfirm.Confirm("删除工作流？", "删除后无法创建使用此工作流的新任务。已有执行记录会保留。", func() {
		go func() {
			err := a.backend.service.DeleteDefinition(a.ctx, id)
			core.Update(func() {
				if a.closed {
					return
				}
				a.fail(err)
				if err == nil {
					a.ui.SelectedWorkflow = ""
					a.ui.WorkflowSource = false
					a.refresh()
				}
			})
		}()
	})
}

func (a *app) workflowFullscreen() bool {
	return a.ui.WorkflowFullscreen && a.ui.WorkflowSource && a.settingsPanel != nil && a.settingsPanel.section == "workflows"
}

// workflowView renders the workflow settings on its own, for tests and previews.
type workflowView struct{ app *app }

func (v workflowView) Render(cx *el.Context) el.Element { return v.app.workflowSettings(cx) }

// openWorkflows shows the workflow section of Settings.
func (a *app) openWorkflows() {
	a.openSettings()
	if a.settingsPanel != nil {
		a.settingsPanel.activateSettingsSection("workflows")
	}
}

func (a *app) restoreWorkflowLocation(target location) {
	a.ui.SelectedWorkflow, a.ui.WorkflowSource = target.Workflow, target.Source
	if !target.Source {
		return
	}
	if target.Editor != "" {
		a.editingWorkflow = target.Workflow
		a.workflowEditor.SetValue(target.Editor)
		a.workflowEditor.SetReadOnly(target.Workflow == "single_agent")
		a.ui.WorkflowRaw = true
		if !target.Raw {
			a.toggleWorkflowRaw()
		}
		return
	}
	for _, d := range a.definitions {
		if d.ID == target.Workflow {
			a.editWorkflow(d)
			return
		}
	}
}

func (a *app) workflowRowMenu(d domainworkflows.Definition) *kit.MenuView {
	if a.ui.WorkflowRowMenus == nil {
		a.ui.WorkflowRowMenus = map[string]*kit.MenuView{}
	}
	if menu := a.ui.WorkflowRowMenus[d.ID]; menu != nil {
		return menu
	}
	menu := kit.Menu().Width(150).Placement(el.Bottom, el.End)
	menu.ContentItem("编辑", "", menuIcon("square-pen", "编辑"), func() { a.selectWorkflow(d.ID, true) }).
		ContentItem("删除", "", menuIcon("trash-2", "删除"), func() { a.deleteWorkflow(d.ID) })
	menu.Trigger(el.ViewFunc(func(cx *el.Context) el.Element {
		return iconAction(cx, "workflow-actions/"+d.ID, "编辑 "+d.Name, "ellipsis", menu.Toggle, menu.Value())
	}))
	a.ui.WorkflowRowMenus[d.ID] = menu
	return menu
}
