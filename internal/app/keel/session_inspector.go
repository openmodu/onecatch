package keeldesktop

import (
	"fmt"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/theme"
	desktopservice "github.com/openmodu/onecatch/internal/service/desktop"
	"regexp"
	"time"
)

func normalizedSessionInput(input, read, write int) int {
	if read+write > input {
		return input + read + write
	}
	return input
}
func sessionCacheRate(input, read int) string {
	if input <= 0 {
		return "—"
	}
	return fmt.Sprintf("%.1f%%", float64(read)/float64(input)*100)
}

var safeResumeID = regexp.MustCompile(`^[A-Za-z0-9._:/-]+$`)

func runtimeResumeCommand(runtime, id string) string {
	if !safeResumeID.MatchString(id) {
		return ""
	}
	switch runtime {
	case "trae":
		return "traecli resume " + id
	case "codex":
		return "codex resume " + id
	case "claude":
		return "claude --resume " + id
	case "modu":
		return "modu_code --resume " + id
	case "pi":
		return "pi --session " + id
	case "grok":
		return "grok --resume " + id
	}
	return ""
}

type recoverySession struct{ Step, Name, Runtime, ID, Command string }

func recoverySessions(d *desktopservice.RunDetail) []recoverySession {
	latest := map[string]string{}
	for _, run := range d.StepRuns {
		id := run.SessionIDAfter
		if id == "" {
			id = run.SessionIDBefore
		}
		if id != "" {
			latest[run.StepID] = id
		}
	}
	var result []recoverySession
	for _, step := range d.Workflow.Steps {
		id := d.Run.Sessions[step.ID]
		if id == "" {
			id = latest[step.ID]
		}
		if id == "" {
			continue
		}
		name := step.Name
		if name == "" {
			name = step.ID
		}
		result = append(result, recoverySession{step.ID, name, string(step.Runtime), id, runtimeResumeCommand(string(step.Runtime), id)})
	}
	return result
}
func (a *app) sessionRecovery(cx *el.Context) el.Element {
	section := el.Div().Gap(10).Child(el.Text("恢复 Session").TextSize(12).Medium())
	sessions := recoverySessions(a.detail)
	if len(sessions) == 0 {
		return section.Child(el.Text("运行时尚未返回 Session ID").TextSize(11).TextColor(theme.Muted))
	}
	if a.workbench.StatusCopies == nil {
		a.workbench.StatusCopies = map[string]*kit.CopyButtonView{}
	}
	for _, session := range sessions {
		session := session
		copyControl := func(kind, value string) el.Element {
			key := a.detail.Run.ID + "/" + session.Step + "/" + kind + "/" + value
			control := a.workbench.StatusCopies[key]
			if control == nil {
				control = kit.CopyButton(func() string { return value })
				a.workbench.StatusCopies[key] = control
				control.Content(el.ViewFunc(func(cx *el.Context) el.Element {
					label := kind
					if control.Copied() {
						label = "已复制"
					}
					return el.Text(label).TextSize(11)
				}))
			}
			return control.Render(cx)
		}
		label := "Session ID"
		if session.Runtime == "codex" {
			label = "Thread ID"
		}
		card := el.Div().Bg(theme.Subtle).Rounded(8).P(12).Gap(8).Child(el.Text(session.Name+" · "+session.Runtime).TextSize(12).Medium(), el.Text(label).TextSize(10).TextColor(theme.Muted), el.Text(session.ID).Mono().TextSize(11))
		actions := el.Div().Row().Wrap().Gap(6).Child(copyControl("复制 ID", session.ID))
		if session.Command != "" {
			card.Child(el.Div().Rounded(5).Bg(theme.Surface).P(8).Child(el.Text(session.Command).Mono().TextSize(11)))
			actions.Child(copyControl("复制恢复命令", session.Command), button("在终端恢复", func() {
				a.workbench.Visible = false
				a.terminal.Maximized = false
				a.createTerminalCommand("", session.Command)
			}, kit.ButtonGhost).Compact(true).Size(28).Render(cx))
			card.Child(el.Text("在项目终端填入命令，按 Enter 恢复。").TextSize(10).TextColor(theme.Muted))
		}
		section.Child(card.Child(actions))
	}
	return section
}

// Persisted durations win. Only the current, unfinished attempt of a live run
// may use wall time; stale "running" records must never keep ticking.
func sessionDuration(d *desktopservice.RunDetail, now time.Time) int64 {
	var total int64
	for i, step := range d.StepRuns {
		if step.DurationMS > 0 {
			total += step.DurationMS
			continue
		}
		if step.StartedAt.IsZero() {
			continue
		}
		end := step.FinishedAt
		if end.IsZero() && i == len(d.StepRuns)-1 && string(d.Run.Status) == "running" && string(step.Status) == "running" && step.StepID == d.Run.CurrentStepID {
			end = now
		}
		// Missing historical timings are unknown, not the age of the session.
		if !end.IsZero() {
			total += max(0, end.Sub(step.StartedAt).Milliseconds())
		}
	}
	return total
}
func sessionNumber(value int) string {
	raw := fmt.Sprint(value)
	for i := len(raw) - 3; i > 0; i -= 3 {
		raw = raw[:i] + "," + raw[i:]
	}
	return raw
}
func sessionElapsed(ms int64) string {
	seconds := max(int64(0), ms/1000)
	if seconds >= 3600 {
		return fmt.Sprintf("%d 小时 %d 分", seconds/3600, seconds%3600/60)
	}
	if seconds >= 60 {
		return fmt.Sprintf("%d 分 %d 秒", seconds/60, seconds%60)
	}
	return fmt.Sprintf("%d 秒", seconds)
}
