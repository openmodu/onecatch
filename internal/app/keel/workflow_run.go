package keeldesktop

import (
	"fmt"
	"strings"

	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/theme"
	domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"
	desktopservice "github.com/openmodu/onecatch/internal/service/desktop"
)

// stepActionState is the open "rerun from here" or "message this step" form in
// the run's status panel.
type stepActionState struct {
	RunID, StepID string
	Rerun         bool
	Text          string
}

func stepStatusLabel(node domainworkflows.NodeState) (string, string) {
	switch node.Status {
	case domainworkflows.NodeRunning:
		return "运行中", "loader-circle"
	case domainworkflows.NodeCompleted:
		return "通过", "circle-check"
	case domainworkflows.NodePaused:
		if node.Verdict == domainworkflows.VerdictAskHuman {
			return "等你回答", "message-square"
		}
		return "已暂停", "triangle-alert"
	case domainworkflows.NodeFailed:
		return "失败", "x"
	default:
		if node.Verdict == domainworkflows.VerdictRework {
			return "已退回，等待重跑", "rotate-ccw"
		}
		if len(node.Inbox) > 0 {
			return "有新意见，等待重跑", "rotate-ccw"
		}
		return "等待", "clock"
	}
}

// workflowProgress shows every step of the run with its live state, rework
// rounds and pending messages, and lets the user rerun from a step or leave a
// step a message. A single-step conversation keeps the plain stepper.
func (a *app) workflowProgress(cx *el.Context) el.Element {
	d := a.detail
	section := el.Div().Gap(10).Py(8).Child(el.Text("工作流").TextSize(12).Medium())
	width := a.inspectorWidth(cx) - 28
	if width <= 0 {
		width = 320
	}
	// The chart shows the whole graph; clicking a step narrows the cards below
	// to that step, and clicking it again shows them all.
	section.Child(el.Div().WFull().NoShrink().Child(flowChart(cx, d.Workflow, flowChartOptions{ID: "run-chart", Vertical: true, Width: width, Nodes: d.Run.Nodes, Rounds: d.Run.ReworkRounds, Selected: a.workbench.StepFocus, OnSelect: func(id string) {
		if a.workbench.StepFocus == id {
			id = ""
		}
		a.workbench.StepFocus = id
	}})))
	resumable := !d.Active && (d.Run.Status == domainworkflows.RunPaused || d.Run.Status == domainworkflows.RunCompleted)
	open := d.Run.Status != domainworkflows.RunCancelled
	for _, step := range d.Workflow.Steps {
		step := step
		node := d.Run.Nodes[step.ID]
		label, icon := stepStatusLabel(node)
		fg := theme.Muted
		switch node.Status {
		case domainworkflows.NodeCompleted:
			fg = theme.Success
		case domainworkflows.NodeRunning:
			fg = theme.Info
		case domainworkflows.NodePaused, domainworkflows.NodeFailed:
			fg = theme.Warning
		}
		if a.workbench.StepFocus != "" && a.workbench.StepFocus != step.ID {
			continue
		}
		card := el.Div().ID("run-step/" + step.ID).Bg(theme.Subtle).Rounded(8).P(10).Gap(6)
		card.Child(el.Div().Row().Items(el.Center).Gap(8).Child(lucide(cx, icon, 14, fg), el.Text(step.Name).TextSize(12).Medium().Grow().MaxLines(1), el.Text(stepAgentLabel(step)).Mono().TextSize(10).TextColor(theme.Muted).MaxLines(1)))
		meta := []string{label}
		if node.Attempt > 1 {
			meta = append(meta, fmt.Sprintf("第 %d 次", node.Attempt))
		}
		for _, edge := range domainworkflows.ReworkEdges(d.Workflow, step.ID) {
			limit := edge.MaxRounds
			if limit == 0 {
				limit = d.Workflow.Policy.MaxReworkRounds
			}
			if used := d.Run.ReworkRounds[domainworkflows.EdgeKey(edge.From, edge.To)]; used > 0 {
				meta = append(meta, fmt.Sprintf("退回 %s %d/%d", edge.To, used, limit))
			}
		}
		if n := len(node.Inbox); n > 0 {
			meta = append(meta, fmt.Sprintf("%d 条待处理意见", n))
		}
		card.Child(el.Text(strings.Join(meta, " · ")).TextSize(10).TextColor(theme.Muted))
		if text := strings.TrimSpace(node.Summary); text != "" {
			card.Child(el.Text(text).TextSize(11).TextColor(theme.Muted).MaxLines(3))
		}
		if node.Error != "" && node.Status != domainworkflows.NodeCompleted {
			card.Child(el.Text(node.Error).TextSize(11).TextColor(theme.DangerText).MaxLines(3))
		}
		if open {
			actions := el.Div().Row().Gap(6)
			if resumable {
				actions.Child(button("从这里重跑", func() { a.openStepAction(step.ID, true) }, kit.ButtonGhost).ID("run-step-rerun/" + step.ID).Compact(true).Size(26).Render(cx))
			}
			actions.Child(button("留言", func() { a.openStepAction(step.ID, false) }, kit.ButtonGhost).ID("run-step-message/" + step.ID).Compact(true).Size(26).Render(cx))
			card.Child(actions)
		}
		if action := a.workbench.StepAction; action != nil && action.RunID == d.Run.ID && action.StepID == step.ID {
			card.Child(a.stepActionForm(cx, step))
		}
		section.Child(card)
	}
	return section
}

func (a *app) openStepAction(stepID string, rerun bool) {
	if a.detail == nil {
		return
	}
	a.workbench.StepAction = &stepActionState{RunID: a.detail.Run.ID, StepID: stepID, Rerun: rerun}
}

func (a *app) stepActionForm(cx *el.Context, step domainworkflows.Step) el.Element {
	action := a.workbench.StepAction
	hint, confirm := "写给「"+step.Name+"」的话，它下次运行时会看到", "发送"
	if action.Rerun {
		hint, confirm = "可选：告诉「"+step.Name+"」这次要注意什么", "重跑"
	}
	note := "它和之后的步骤会接着各自原来的会话重新运行；之前的步骤保持不变。"
	if !action.Rerun {
		note = "不会改变流程。如果它正在运行，意见在它下一次运行时生效。"
	}
	submit := button(confirm, a.submitStepAction, kit.ButtonPrimary).ID("run-step-submit").Compact(true).Size(28)
	submit.SetDisabled(a.busy || !action.Rerun && strings.TrimSpace(action.Text) == "")
	return el.Div().Gap(8).Pt(4).Child(
		el.TextArea().ID("run-step-text/"+step.ID).Name(hint).Placeholder(hint).Bind(&action.Text).AutoGrow(2, 6).WFull().Bg(theme.Surface).Rounded(6).P(8).TextSize(12),
		el.Text(note).TextSize(10).TextColor(theme.Muted),
		el.Div().Row().Gap(6).Child(el.Div().Grow(), button("取消", func() { a.workbench.StepAction = nil }, kit.ButtonGhost).Compact(true).Size(28).Render(cx), submit.Render(cx)),
	)
}

func (a *app) submitStepAction() {
	action := a.workbench.StepAction
	if action == nil || a.busy {
		return
	}
	runID, stepID, rerun, text := action.RunID, action.StepID, action.Rerun, strings.TrimSpace(action.Text)
	service := a.backend.service
	a.busy = true
	go func() {
		var err error
		if rerun {
			_, err = service.ResumeRunConfigured(a.ctx, runID, desktopservice.ResumeRunInput{FromStepID: stepID, Instruction: text})
		} else {
			_, err = service.EnqueueInstruction(a.ctx, runID, desktopservice.InstructionInput{StepID: stepID, Content: text})
		}
		core.Update(func() {
			if a.closed {
				return
			}
			a.busy = false
			a.fail(err)
			if err == nil {
				a.workbench.StepAction = nil
			}
			a.reloadRun(runID)
		})
	}()
}
