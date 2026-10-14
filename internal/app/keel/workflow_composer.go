package keeldesktop

import (
	"fmt"
	"strings"

	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/theme"
	uiassets "github.com/openmodu/onecatch/internal/app/keel/assets"
	domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"
)

// composerContext sits above the composer. Before a task starts it shows the
// chosen workflow's whole graph; on a paused workflow it says why the run
// stopped and which step a message will reach.
func (a *app) composerContext(cx *el.Context) el.Element {
	if a.task == nil && !a.remoteConversation() && a.workflow.Value() != "single_agent" {
		if d, ok := a.findDefinition(a.workflow.Value()); ok {
			clear := iconAction(cx, "composer-workflow-clear", "改为单 Agent", "x", func() { a.selectLocalConversation(a.runtime.Value(), "single_agent") }, false)
			head := el.Div().Row().Items(el.Center).Gap(8).Child(uiassets.Lucide(cx, "workflow", 14, theme.Muted), el.Text("将按「"+d.Name+"」运行").TextSize(12).Medium(), el.Text(workflowSummary(d)).TextSize(11).TextColor(theme.Muted).Grow().MaxLines(1), clear)
			return el.Div().ID("composer-workflow").WFull().P(12).Gap(10).Rounded(14).Bg(theme.Subtle).Child(head,
				el.Div().WFull().NoShrink().ScrollX().Child(flowChart(cx, d, flowChartOptions{ID: "composer-chart"})))
		}
	}
	if a.detail == nil || a.detail.Active || a.detail.Run.Status != domainworkflows.RunPaused || len(a.detail.Workflow.Steps) < 2 {
		return el.Div().Hidden(true)
	}
	reason, receivers := pausedRunNotice(a.detail.Workflow, a.detail.Run)
	note := "发送的消息会交给「" + strings.Join(receivers, "」「") + "」并继续运行；要交给别的步骤，用 @ 选择。"
	if len(receivers) == 0 {
		note = "发送消息会继续运行；用 @ 选择接收的步骤。"
	}
	return el.Div().ID("composer-paused").Row().Items(el.Start).Gap(10).P(12).Rounded(14).Bg(theme.Subtle).Child(
		kit.Icon(kit.IconWarning).Color(theme.Warning).Size(16).Render(cx),
		el.Div().Grow().W(el.Dp(0)).Gap(4).Child(el.Text(reason).TextSize(12).Medium(), el.Text(note).TextSize(11).TextColor(theme.Muted)))
}

// pausedRunNotice explains a paused workflow run and names the steps an
// unaddressed message reaches: the ones that run first when it resumes.
func pausedRunNotice(d domainworkflows.Definition, run domainworkflows.Run) (string, []string) {
	name := func(id string) string {
		if step, ok := domainworkflows.FindStep(d, id); ok {
			return step.Name
		}
		return id
	}
	var stopped string
	for _, s := range d.Steps {
		if node := run.Nodes[s.ID]; node.Status == domainworkflows.NodePaused || node.Status == domainworkflows.NodeFailed {
			stopped = s.ID
			break
		}
	}
	if stopped == "" {
		stopped = run.CurrentStepID
	}
	node := run.Nodes[stopped]
	var reason string
	switch run.PauseReason {
	case domainworkflows.PauseReasonAskHuman:
		reason = fmt.Sprintf("「%s」在问你：%s", name(stopped), node.Summary)
	case domainworkflows.PauseReasonReworkLimit:
		target := ""
		for _, e := range domainworkflows.ReworkEdges(d, stopped) {
			target = e.To
		}
		reason = fmt.Sprintf("「%s」退回「%s」已达上限：%s", name(stopped), name(target), node.Summary)
	case domainworkflows.PauseReasonFailureLimit:
		reason = fmt.Sprintf("「%s」连续失败 %d 次：%s", name(stopped), node.Failures, node.Error)
	case "interrupted":
		reason = "已暂停：你中断了运行。"
	default:
		reason = "已暂停：" + run.LastError
	}
	// What a plain resume reopens, and which of those can start at once.
	probe := domainworkflows.CloneRun(run)
	for id, n := range probe.Nodes {
		if n.Status == domainworkflows.NodePaused || n.Status == domainworkflows.NodeFailed || n.Status == domainworkflows.NodeRunning {
			n.Status = domainworkflows.NodePending
			probe.Nodes[id] = n
		}
	}
	var receivers []string
	for _, s := range domainworkflows.ReadySteps(d, probe) {
		receivers = append(receivers, s.Name)
	}
	return reason, receivers
}

// labelWorkflowSteps marks where each workflow step's output begins in the
// conversation, so parallel and successive steps can be told apart.
func (a *app) labelWorkflowSteps() {
	if a.detail == nil || len(a.detail.Workflow.Steps) < 2 {
		return
	}
	steps := make(map[string]domainworkflows.Step, len(a.detail.StepRuns))
	for _, run := range a.detail.StepRuns {
		if step, ok := domainworkflows.FindStep(a.detail.Workflow, run.StepID); ok {
			steps[run.ID] = step
		}
	}
	runs := make(map[string]domainworkflows.StepRun, len(a.detail.StepRuns))
	for _, run := range a.detail.StepRuns {
		runs[run.ID] = run
	}
	previous := ""
	for _, e := range a.entries {
		if e.user || e.kind == kindTransition {
			previous = ""
			continue
		}
		step, ok := steps[e.step]
		if !ok || e.step == previous {
			continue
		}
		e.stepLabel = step.Name + " · " + stepAgentLabel(step)
		context := []string{}
		if a.detail.Workflow.Name != "" {
			context = append(context, "工作流："+a.detail.Workflow.Name)
		}
		run := runs[e.step]
		if run.Attempt > 0 {
			context = append(context, fmt.Sprintf("第 %d 次执行", run.Attempt))
		}
		if run.Status != "" {
			status := runStatus(string(run.Status))
			if run.Status == domainworkflows.StepRunSucceeded {
				status = "已完成"
			}
			if run.Status == domainworkflows.StepRunInterrupted {
				status = "已中断"
			}
			context = append(context, status)
		}
		e.stepContext = strings.Join(context, " · ")
		previous = e.step
	}
}
