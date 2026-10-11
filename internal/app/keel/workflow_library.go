package keeldesktop

import (
	"fmt"

	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/theme"
)

// workflowLibrary shares the settings content area instead of nesting another sidebar.
func (a *app) workflowLibrary(cx *el.Context) el.Element {
	list := el.Div().ID("workflow-library").WFull()
	count := 0
	for _, definition := range a.definitions {
		if definition.ID == "single_agent" {
			continue
		}
		d := definition
		count++
		icon := "workflow"
		heading := el.Div().Row().Items(el.Center).Gap(8).Child(
			el.Text(d.Name).Medium().TextSize(14).MaxLines(1),
		)
		content := el.Div().Grow().W(el.Dp(0)).Gap(6).Child(heading)
		if d.Description != "" {
			content.Child(el.Text(d.Description).TextSize(12).TextColor(theme.Muted).MaxLines(2))
		}
		content.Child(el.Text(workflowSummary(d)).TextSize(11).TextColor(theme.Muted).MaxLines(1),
			el.Div().WFull().NoShrink().ScrollX().Pt(4).Child(flowChart(cx, d, flowChartOptions{ID: "workflow-chart/" + d.ID})))
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
	header := el.Div().Row().Items(el.Center).NoShrink().Child(el.Text(fmt.Sprintf("%d 个工作流", count)).TextSize(12).TextColor(theme.Muted).Grow(), a.newWorkflowMenu(cx))
	if count == 0 {
		list.Child(el.Div().Py(32).Gap(8).Child(el.Text("还没有工作流").Medium(), el.Text("点击右上角 +，从空白或内置模板开始。").TextSize(12).TextColor(theme.Muted)))
	}
	return el.Div().WFull().Child(header, list)
}
