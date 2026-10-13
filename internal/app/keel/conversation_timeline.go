package keeldesktop

import (
	"fmt"
	"sort"

	workflows "github.com/openmodu/onecatch/internal/domain/workflows"
	desktop "github.com/openmodu/onecatch/internal/service/desktop"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

const kindTimelineHeader agentrun.EventKind = "timeline_header"

// Overlapping execution intervals form a set of fixed lanes. Finished lanes
// remain in the set until the other executions finish, so later output leaves
// a blank cell rather than moving into another step's column.
func buildWorkflowTimeline(entries []*entry, detail *desktop.RunDetail) []*entry {
	if detail == nil || len(detail.Workflow.Steps) < 2 {
		return entries
	}
	runs := append([]workflows.StepRun(nil), detail.StepRuns...)
	sort.SliceStable(runs, func(i, j int) bool { return runs[i].StartedAt.Before(runs[j].StartedAt) })
	groups := map[string][]string{}
	headers := map[string]*entry{}
	for _, run := range runs {
		if step, ok := workflows.FindStep(detail.Workflow, run.StepID); ok {
			status := runStatus(string(run.Status))
			if run.Status == workflows.StepRunSucceeded {
				status = "已完成"
			}
			headers[run.ID] = &entry{key: "lane/" + run.ID, kind: kindTimelineHeader, stepLabel: step.Name + " · " + stepAgentLabel(step), stepContext: fmt.Sprintf("第 %d 次执行 · %s", run.Attempt, status)}
		}
	}
	for i := 0; i < len(runs); {
		if runs[i].StartedAt.IsZero() {
			i++
			continue
		}
		end := runs[i].FinishedAt
		j := i + 1
		for j < len(runs) && (end.IsZero() || runs[j].StartedAt.Before(end)) {
			if runs[j].FinishedAt.IsZero() || (!end.IsZero() && runs[j].FinishedAt.After(end)) {
				end = runs[j].FinishedAt
			}
			j++
		}
		if j-i > 1 {
			lanes := make([]string, j-i)
			for k := i; k < j; k++ {
				lanes[k-i] = runs[k].ID
			}
			for _, id := range lanes {
				groups[id] = lanes
			}
		}
		i = j
	}
	out := make([]*entry, 0, len(entries))
	var row *entry
	var current string

	for _, e := range entries {
		lanes := groups[e.step]
		if e.user || e.kind == kindTransition || len(lanes) == 0 {
			out = append(out, e)
			row = nil
			current = ""

			continue
		}
		lane := 0
		for lanes[lane] != e.step {
			lane++
		}
		// Each lane keeps chronological flow within this parallel segment.
		// A finished lane leaves blank space at its end, not between records.
		if current != lanes[0] {
			header := &entry{key: "timeline-header/" + e.key, at: e.at, columns: make([]*entry, len(lanes))}
			for i, id := range lanes {
				header.columns[i] = headers[id]
			}
			out = append(out, header)
		}
		if row == nil || current != lanes[0] {
			row = &entry{key: "timeline/" + e.key, at: e.at, columns: make([]*entry, len(lanes))}
			out = append(out, row)

			current = lanes[0]
		}
		cell := *e
		cell.stepLabel, cell.stepContext = "", ""
		if previous := row.columns[lane]; previous == nil {
			row.columns[lane] = &cell
		} else if len(previous.stack) > 0 {
			previous.stack = append(previous.stack, &cell)
		} else {
			row.columns[lane] = &entry{key: "lane/" + previous.key, step: e.step, stack: []*entry{previous, &cell}}
		}
	}
	return out
}

func (a *app) invalidateConversationEntry(key string) {
	var contains func(*entry) bool
	contains = func(e *entry) bool {
		if e.key == key {
			return true
		}
		for _, child := range e.stack {
			if contains(child) {
				return true
			}
		}
		for _, p := range e.process {
			if contains(p) {
				return true
			}
		}
		return false
	}
	for _, row := range a.conversationRows {
		for _, cell := range row.columns {
			if cell != nil && contains(cell) {
				a.scroller.Invalidate(row.key)
				return
			}
		}
	}
	a.scroller.Invalidate(key)
}

func (a *app) displayedConversationRows() []*entry {
	if a.conversationRows != nil {
		return a.conversationRows
	}
	return a.entries
}
