package keeldesktop

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/theme"
	domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"
	desktopservice "github.com/openmodu/onecatch/internal/service/desktop"
)

type instructionAction struct{ RunID, InstructionID, Kind string }

func (a *app) instructionBusy() bool {
	return a.instructionAction.RunID != "" && a.instructionAction.RunID == a.runID
}

func (a *app) pendingInstruction(id string) (domainworkflows.Instruction, bool) {
	if a.detail != nil && a.detail.Run.ID == a.runID {
		for _, item := range a.detail.Instructions {
			if item.ID == id && item.Status == domainworkflows.InstructionPending {
				return item, true
			}
		}
	}
	return domainworkflows.Instruction{}, false
}

// Promote the existing instruction in place. Re-enqueueing would duplicate its
// message and attachments and lose its identity in the durable event history.
func applyInstructionAction(ctx context.Context, service *desktopservice.Service, action instructionAction) error {
	if action.Kind == "steer" {
		_, err := service.SteerInstruction(ctx, action.RunID, action.InstructionID)
		return err
	}
	if action.Kind == "remove" {
		return service.RemoveInstruction(ctx, action.RunID, action.InstructionID)
	}
	return fmt.Errorf("unknown instruction action")
}

func (a *app) actOnInstruction(id, kind string) {
	item, found := a.pendingInstruction(id)
	if !found || a.closed || a.busy || a.loading || a.instructionBusy() {
		return
	}
	if kind != "remove" && (kind != "steer" || !item.FollowUp || a.detail.Run.Status != domainworkflows.RunRunning) {
		return
	}
	action := instructionAction{a.runID, id, kind}
	a.instructionAction, a.errorText = action, ""
	selection := a.selection
	go func() {
		ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
		defer cancel()
		err := applyInstructionAction(ctx, a.backend.service, action)
		core.Update(func() {
			if a.closed {
				return
			}
			if a.instructionAction == action {
				a.instructionAction = instructionAction{}
			}
			if a.selection != selection || a.runID != action.RunID {
				return
			}
			a.fail(err)
			// The service may have changed state even on failure (for example a
			// steering rollback). Refresh without touching the current draft.
			a.reloadRun(action.RunID)
		})
	}()
}

func (a *app) pendingInstructionList(cx *el.Context) el.Element {
	list := el.Div().Role("list").Name("待执行的后续消息").Mx(20).MaxH(el.Dp(168)).ScrollY().Gap(1)
	if a.detail == nil || a.detail.Run.ID != a.runID {
		return list.Hidden(true)
	}
	count := 0
	for _, item := range a.detail.Instructions {
		if item.Status != domainworkflows.InstructionPending {
			continue
		}
		instruction := item
		content := strings.TrimSpace(instruction.Content)
		if content == "" {
			content = fmt.Sprintf("%d 个附件", len(instruction.Attachments))
		}
		busy := a.instructionBusy() || a.busy || a.loading
		remove := iconAction(cx, "remove-instruction/"+instruction.ID, "删除后续消息", "trash-2", func() { a.actOnInstruction(instruction.ID, "remove") }, false).(*el.DivEl)
		remove.Disabled(busy)
		actions := el.Div().Row().Items(el.Center).Gap(2)
		if instruction.FollowUp {
			steer := el.Div().ID("steer-instruction/"+instruction.ID).Role("button").Name("调整方向").Focusable(true).CursorPointer().Row().Items(el.Center).Gap(5).H(el.Dp(28)).Px(6).Rounded(6).Disabled(busy || a.detail.Run.Status != domainworkflows.RunRunning).OnClick(func() { a.actOnInstruction(instruction.ID, "steer") }).Hover(func(s *el.Style) { s.Bg(theme.Highlight) }).FocusStyle(neutralFocus).Child(lucide(cx, "corner-down-right", 16, theme.Muted), el.Text("调整方向").TextSize(12).Medium().TextColor(theme.Muted))
			actions.Child(steer)
		}
		actions.Child(remove)
		row := el.Div().Role("listitem").Name(content).ID("pending-instruction/"+instruction.ID).Row().Items(el.Center).Gap(10).MinH(el.Dp(52)).Pl(14).Pr(10).Pb(8).RoundedCorners(10, 10, 0, 0).Bg(mixColor(theme.Surface, theme.Subtle, .96)).Child(lucide(cx, "list-end", 16, theme.Muted), el.Text(content).TextSize(14).LineHeight(1.45).MaxLines(1).Grow().W(el.Dp(0)), actions)
		if count == 0 {
			row.RoundedCorners(18, 18, 0, 0)
		}
		if len(instruction.Attachments) > 0 {
			list.Child(el.Div().ID("pending-content/"+instruction.ID).Bg(mixColor(theme.Surface, theme.Subtle, .96)).RoundedCorners(18, 18, 0, 0).Child(
				row,
				el.Div().Pl(40).Pr(10).Pb(8).Child(a.conversationAttachments(cx, instructionAttachments(instruction.Attachments), "pending/"+instruction.ID, true)),
			))
		} else {
			list.Child(row)
		}
		count++
	}
	return list.Hidden(count == 0)
}
