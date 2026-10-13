package workflows

import (
	"fmt"
	"strings"
	"time"
)

// NextLoopRound wraps a completed workflow without changing its inner graph.
// Attempts and provider sessions survive; each new round owns its rework budget.
func NextLoopRound(def Definition, run Run, now time.Time) (Run, error) {
	if !run.LoopEnabled || run.Status != RunCompleted {
		return Run{}, fmt.Errorf("%w: loop requires a completed round", ErrInvalidRunState)
	}
	if err := Validate(def); err != nil {
		return Run{}, err
	}
	out := cloneRun(run)
	out.LoopRound = max(1, run.LoopRound) + 1
	out.Status = RunRunning
	out.CompletedAt = time.Time{}
	out.UpdatedAt = now
	out.PauseReason, out.LastError = "", ""
	out.ReworkRounds = nil
	var summaries []string
	for _, step := range def.Steps {
		n := out.Nodes[step.ID]
		n.StepID = step.ID
		if n.Summary != "" {
			summaries = append(summaries, step.Name+": "+n.Summary)
		}
		n.Status = NodePending
		n.Verdict, n.Error, n.Summary = "", "", ""
		n.Failures = 0
		n.StartedAt, n.FinishedAt = time.Time{}, time.Time{}
		out.Nodes[step.ID] = n
	}
	out.CurrentStepID = EntrySteps(def)[0]
	message := fmt.Sprintf("开始循环第 %d 轮。根据原始目标、上一轮成果和新的人工指令推进下一项有价值的工作；不要重复已完成的工作。如果目标已完成或缺少后续任务，返回 ask_human 等待人工指导。\n上一轮成果：\n%s", out.LoopRound, strings.Join(summaries, "\n"))
	for _, id := range EntrySteps(def) {
		deliver(&out, id, Message{From: "onecatch", Text: message, At: now})
	}
	return out, nil
}
