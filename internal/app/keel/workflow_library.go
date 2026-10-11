package keeldesktop

import (
	"fmt"

	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/theme"
	domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"
)

// workflowLibrary shares the settings content area instead of nesting another sidebar.
func (a *app) workflowLibrary(cx *el.Context) el.Element {
	list := el.Div().ID("workflow-library").Grow().H(el.Dp(0)).ScrollY()
	count := 0
	for _, definition := range a.definitions {
		if definition.ID == "single_agent" {
			continue
		}
		d := definition
		count++
		mode, icon := "串行流程", "workflow"
		if d.Mode == domainworkflows.ModeDAG {
			mode, icon = "并行 DAG", "git-branch"
		}
		heading := el.Div().Row().Items(el.Center).Gap(8).Child(
			el.Text(d.Name).Medium().TextSize(14).MaxLines(1),
			el.Text(mode).TextSize(11).TextColor(theme.Muted).Bg(theme.Subtle).Rounded(4).Px(6).Py(3),
		)
		content := el.Div().Grow().W(el.Dp(0)).Gap(6).Child(heading)
		if d.Description != "" {
			content.Child(el.Text(d.Description).TextSize(12).TextColor(theme.Muted).MaxLines(2))
		}
		content.Child(el.Text(fmt.Sprintf("%d 个步骤 · %s", len(d.Steps), workflowPath(d))).TextSize(11).TextColor(theme.Muted).MaxLines(1))
		edit := button("配置", func() {
			if a.editingWorkflow == d.ID && a.ui.WorkflowDraft.ID == d.ID {
				a.ui.WorkflowSource = true
				return
			}
			a.selectWorkflow(d.ID, true)
		}, kit.ButtonGhost).Size(30).Compact(true)
		row := el.Div().Row().Items(el.Center).Gap(12).Py(16).Child(
			el.Div().Size(el.Dp(32)).Rounded(8).Bg(theme.Subtle).Center().Child(lucide(cx, icon, 16, theme.Muted)),
			content, edit.Render(cx), a.workflowRowMenu(d).Render(cx),
		)
		list.Child(row, el.Div().H(el.Dp(1)).WFull().Bg(seamColor).NoShrink())
	}
	header := el.Div().Row().Items(el.Center).Pb(12).NoShrink().Child(el.Text(fmt.Sprintf("%d 个工作流", count)).TextSize(12).TextColor(theme.Muted).Grow(), a.newWorkflowMenu(cx))
	if count == 0 {
		list.Child(el.Div().Py(32).Gap(8).Child(el.Text("还没有工作流").Medium(), el.Text("点击右上角 +，从实现与审查或并行 DAG 开始。").TextSize(12).TextColor(theme.Muted)))
	}
	return el.Div().Grow().H(el.Dp(0)).Child(header, list)
}
