package keeldesktop

import (
	"time"

	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/theme"
	workflows "github.com/openmodu/onecatch/internal/domain/workflows"
	desktop "github.com/openmodu/onecatch/internal/service/desktop"
)

type sessionWorkflowState struct {
	RunID         string
	Selection     uint64
	Checking      bool
	CheckedAt     time.Time
	Result        desktop.RunWorkflowCheck
	Error, Notice string
}

func (a *app) checkSessionWorkflow(force bool) bool {
	if a.closed || a.backend == nil || a.backend.service == nil || a.detail == nil || a.busy || a.workbench.Tab != "status" || !a.workbench.Visible && a.workbench.DetachedWindow == nil {
		return false
	}
	id := a.detail.Run.ID
	if id == "" || a.detail.Workflow.ID == "" || a.detail.Workflow.ID == "single_agent" {
		return false
	}
	s := a.workbench.WorkflowCheck
	if s == nil || s.RunID != id || s.Selection != a.selection {
		s = &sessionWorkflowState{RunID: id, Selection: a.selection}
		a.workbench.WorkflowCheck = s
	}
	if s.Checking || !force && time.Since(s.CheckedAt) < projectWorkflowPoll {
		return false
	}
	s.Checking = true
	go func() {
		result, err := a.backend.service.CheckRunWorkflow(a.ctx, id)
		core.Update(func() {
			if a.closed || a.selection != s.Selection || a.detail == nil || a.detail.Run.ID != id || a.workbench.WorkflowCheck != s {
				return
			}
			s.Checking, s.CheckedAt = false, time.Now()
			s.Result, s.Error = result, ""
			if err != nil {
				s.Error = err.Error()
			}
		})
	}()
	return true
}

func (a *app) sessionWorkflowConfig(cx *el.Context) el.Element {
	a.checkSessionWorkflow(false)
	s := a.workbench.WorkflowCheck
	if s == nil || s.RunID != a.detail.Run.ID || s.Selection != a.selection {
		s = &sessionWorkflowState{}
	}
	box := el.Div().ID("session-workflow-config").Bg(theme.Subtle).Rounded(10).P(12).Gap(8)
	check := button("检查变更", func() { a.checkSessionWorkflow(true) }, kit.ButtonGhost).ID("session-workflow-check").Compact(true).Size(28)
	check.SetDisabled(a.busy || s.Checking)
	box.Child(el.Div().Row().Items(el.Center).Gap(8).Child(el.Text("工作流配置").TextSize(12).Medium().Grow(), check.Render(cx)))
	switch {
	case s.Error != "":
		box.Child(el.Text("无法检查配置变更：" + s.Error).TextSize(11).TextColor(theme.Warning))
	case s.CheckedAt.IsZero():
		box.Child(el.Text("正在检查配置变更…").TextSize(11).TextColor(theme.Muted))
	case !s.Result.Changed:
		label := "配置与当前工作流一致"
		if s.Notice != "" {
			label = s.Notice
		}
		box.Child(el.Text(label).TextSize(11).TextColor(theme.Muted))
	default:
		box.Child(el.Text("检测到工作流配置变更").TextSize(12).Medium().TextColor(theme.Warning))
		for _, change := range s.Result.Changes {
			box.Child(el.Text("• " + change).TextSize(11).TextColor(theme.Muted))
		}
		hint := "更新后仍保持暂停；已有执行记录和进度保留，新配置用于后续执行。"
		if a.detail.Active {
			hint = "必须先暂停，并等待后台执行完全停止后才能更新。"
		} else if a.detail.Run.Status != workflows.RunPaused {
			hint = "只有已暂停的 Session 可以手动更新配置。"
		}
		box.Child(el.Text(hint).TextSize(11).TextColor(theme.Muted))
		update := button("更新工作流配置", a.updateSessionWorkflow, kit.ButtonSecondary).ID("session-workflow-update").Compact(true).Size(28)
		update.SetDisabled(a.busy || s.Checking || a.detail.Active || a.detail.Run.Status != workflows.RunPaused)
		actions := el.Div().Row().Wrap().Gap(6)
		if a.detail.Active {
			pause := button("暂停", a.stop, kit.ButtonGhost).ID("session-workflow-pause").Compact(true).Size(28)
			pause.SetDisabled(a.busy)
			actions.Child(pause.Render(cx))
		}
		box.Child(actions.Child(update.Render(cx)))
	}
	return box
}

func (a *app) updateSessionWorkflow() {
	if a.busy || a.backend == nil || a.detail == nil || a.detail.Active || a.detail.Run.Status != workflows.RunPaused {
		return
	}
	s := a.workbench.WorkflowCheck
	if s == nil || s.RunID != a.detail.Run.ID || s.Selection != a.selection || s.Checking || s.Error != "" || !s.Result.Changed {
		return
	}
	id, selection := a.detail.Run.ID, a.selection
	a.busy = true
	go func() {
		_, err := a.backend.service.UpdateRunWorkflow(a.ctx, id)
		core.Update(func() {
			if a.closed {
				return
			}
			a.busy = false
			if selection != a.selection || a.detail == nil || a.detail.Run.ID != id {
				return
			}
			if err != nil {
				s.Error = err.Error()
				s.CheckedAt = time.Now()
			} else {
				a.workbench.WorkflowCheck = &sessionWorkflowState{RunID: id, Selection: selection, Notice: "工作流配置已更新，Session 保持暂停"}
			}
			a.reloadRun(id)
			a.checkSessionWorkflow(true)
		})
	}()
}
