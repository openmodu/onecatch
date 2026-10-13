package keeldesktop

import (
	"fmt"
	"strings"

	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/theme"
	domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"
)

func (a *app) workflowNodeHeading(cx *el.Context, e *entry, result bool) el.Element {
	var step domainworkflows.Step
	var run domainworkflows.StepRun
	index := -1
	if a.detail != nil {
		for _, r := range a.detail.StepRuns {
			if r.ID != e.step {
				continue
			}
			run = r
			for i, s := range a.detail.Workflow.Steps {
				if s.ID == r.StepID {
					step, index = s, i
					break
				}
			}
			break
		}
	}
	if index < 0 {
		if result {
			return el.Div().Hidden(true)
		}
		return el.Div().Pl(2).Pt(10).Pb(6).Gap(4).Child(el.Text(e.stepLabel).TextSize(14).Bold(), el.Text(e.stepContext).TextSize(12).TextColor(theme.Muted))
	}
	title := el.Div().Row().Wrap().Items(el.Center).Gap(8).Child(
		el.Text(fmt.Sprintf("节点 %02d", index+1)).TextSize(10).Medium().TextColor(theme.Info).Px(7).Py(4).Rounded(5).Bg(mixColor(theme.Info, theme.Surface, .10)),
		el.Text(step.Name).TextSize(15).Bold().TextColor(theme.Text),
	)
	if result {
		title.Child(el.Text("执行结果").TextSize(11).TextColor(theme.Muted))
	}
	meta := []string{}
	if run.Attempt > 0 {
		meta = append(meta, fmt.Sprintf("第 %d 次执行", run.Attempt))
	}
	meta = append(meta, stepAgentLabel(step))
	if !result {
		status := runStatus(string(run.Status))
		if run.Status == domainworkflows.StepRunSucceeded {
			status = "已完成"
		}
		meta = append(meta, status)
	}
	box := el.Div().WFull().Gap(6).Child(title, el.Text(strings.Join(meta, " · ")).TextSize(11).TextColor(theme.Muted))
	if result {
		box.Pb(12).Child(el.Div().WFull().H(el.Dp(1)).Bg(seamColor).Mt(5))
	} else {
		box.Mt(12).Mb(8).Px(12).Py(10).Rounded(8).Bg(mixColor(theme.Info, theme.Surface, .045))
		if a.detail.Workflow.Name != "" {
			box.Child(el.Text(a.detail.Workflow.Name).TextSize(10).TextColor(theme.Muted))
		}
	}
	return box
}
