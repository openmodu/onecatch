package keeldesktop

import (
	"fmt"
	"strings"

	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/theme"
	workflows "github.com/openmodu/onecatch/internal/domain/workflows"
)

type composerLoopState struct {
	key     string
	enabled bool
	control *kit.SwitchView
}

func (a *app) composerWorkflow() (workflows.Definition, bool) {
	if a.detail != nil {
		d := a.detail.Workflow
		return d, d.ID != "single_agent" && len(d.Steps) > 0
	}
	id := a.workflow.Value()
	if a.task != nil {
		id = a.task.WorkflowID
	}
	d, ok := a.findDefinition(id)
	return d, ok && id != "single_agent" && len(d.Steps) > 0
}

func (a *app) syncComposerLoop() {
	d, ok := a.composerWorkflow()
	if !ok {
		return
	}
	key, enabled := "draft/"+d.ID, d.LoopEnabled
	if a.detail != nil && a.runID != "" {
		key, enabled = "run/"+a.runID, a.detail.Run.LoopEnabled
	}
	s := &a.composerLoop
	if s.key != key {
		*s = composerLoopState{key: key, enabled: enabled}
		s.control = kit.Switch("工作流循环", enabled).Size(kit.SwitchMedium).OnChange(func(on bool) {
			a.composerLoop.enabled = on
			if a.detail != nil && a.runID != "" {
				a.setRunLoop(on)
			}
		})
	}
	if a.detail != nil && a.runID != "" && !a.busy {
		s.enabled = enabled
	}
	s.control.SetValue(s.enabled)
}

func (a *app) composerLoopControl(cx *el.Context) el.Element {
	if _, ok := a.composerWorkflow(); !ok {
		return nil
	}
	a.syncComposerLoop()
	s := &a.composerLoop
	disabled := a.busy || a.loading || a.instructionBusy()
	if a.detail != nil {
		disabled = disabled || a.detail.Active || a.detail.Run.Status != workflows.RunPaused && a.detail.Run.Status != workflows.RunCompleted && a.detail.Run.Status != workflows.RunReady
	}
	s.control.SetDisabled(disabled)
	s.control.Color(theme.Info)
	// The app's frameless palette has a transparent Border, which Switch
	// uses for its off track. Give this control a visible track and thumb.
	palette := theme.Current()
	palette.Border = theme.RGB(0x758197)
	palette.Surface = theme.RGB(0xffffff)
	palette.Subtle = theme.RGB(0x8490a3)
	if disabled {
		palette.Border = theme.RGB(0xd5dbe5)
	}
	state, tone := "已关闭", theme.Muted
	chipBg := theme.Surface
	chipBorder := mixColor(theme.Muted, theme.Subtle, .45)
	if s.enabled {
		state, tone = "已开启", theme.Info
		chipBg = mixColor(theme.Info, theme.Surface, .08)
		chipBorder = mixColor(theme.Info, theme.Subtle, .5)
	}
	control := el.Div().Row().Items(el.Center).Gap(8).Px(9).Py(5).Rounded(9).Bg(chipBg).Border(1, chipBorder).
		Child(cx.Themed(palette, s.control), el.Text(state).TextSize(11).Medium().TextColor(tone))
	hint := "执行一轮后结束"
	if s.enabled {
		hint = "整轮通过后自动开始下一轮"
	}
	row := el.Div().ID("composer-loop").Row().Wrap().Items(el.Center).Gap(8).WFull().Child(
		control, el.Text(hint).TextSize(11).TextColor(theme.Muted),
	)
	if a.detail != nil && s.enabled {
		row.Child(el.Text(fmt.Sprintf("第 %d 轮", max(1, a.detail.Run.LoopRound))).TextSize(11).TextColor(theme.Info))
	}
	if a.detail != nil && a.detail.Active {
		pause := button("暂停后切换", a.stop, kit.ButtonGhost).Compact(true).Size(26)
		pause.SetDisabled(a.busy || a.loading || a.instructionBusy())
		row.Child(pause.Render(cx))
	} else if a.detail != nil && (a.detail.Run.Status == workflows.RunPaused || a.detail.Run.Status == workflows.RunCompleted) {
		resume := button("继续", func() {
			if strings.TrimSpace(a.prompt) == "" {
				a.prompt = "继续当前工作流，保留原有目标与约束。"
			}
			a.send()
		}, kit.ButtonGhost).Compact(true).Size(26)
		resume.SetDisabled(a.busy || a.loading || a.instructionBusy())
		row.Child(resume.Render(cx))
	}
	return row
}
