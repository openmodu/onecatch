package keeldesktop

import (
	"fmt"
	"sort"
	"time"

	workflows "github.com/openmodu/onecatch/internal/domain/workflows"
	desktop "github.com/openmodu/onecatch/internal/service/desktop"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

const kindTimelineHeader agentrun.EventKind = "timeline_header"

// Overlapping execution intervals form a fixed set of lanes. Serial attempts
// reuse a free lane: transitive overlap must not turn retries into additional
// concurrent tasks. Each new attempt keeps its own heading inside the lane.
func buildWorkflowTimeline(entries []*entry, detail *desktop.RunDetail) []*entry {
	if detail == nil || len(detail.Workflow.Steps) < 2 {
		return entries
	}
	runs := append([]workflows.StepRun(nil), detail.StepRuns...)
	sort.SliceStable(runs, func(i, j int) bool { return runs[i].StartedAt.Before(runs[j].StartedAt) })
	groups := map[string][]string{}
	laneIndexes := map[string]int{}
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
			var lanes []string
			var ends []time.Time
			for k := i; k < j; k++ {
				lane := -1
				for slot, end := range ends {
					if !end.IsZero() && !runs[k].StartedAt.Before(end) {
						lane = slot
						break
					}
				}
				if lane < 0 {
					lane = len(lanes)
					lanes = append(lanes, runs[k].ID)
					ends = append(ends, runs[k].FinishedAt)
				} else {
					ends[lane] = runs[k].FinishedAt
				}
				laneIndexes[runs[k].ID] = lane
			}
			if len(lanes) > 1 {
				for k := i; k < j; k++ {
					groups[runs[k].ID] = lanes
				}
			}
		}
		i = j
	}
	out := make([]*entry, 0, len(entries))
	var row *entry
	var current string
	var pageGroup string
	var lastRuns []string

	for _, e := range entries {
		lanes := groups[e.step]
		if e.user || e.kind == kindTransition || len(lanes) == 0 {
			out = append(out, e)
			row = nil
			current = ""

			continue
		}
		lane := laneIndexes[e.step]
		// Each lane keeps chronological flow within this parallel segment.
		// A finished lane leaves blank space at its end, not between records.
		if current != lanes[0] {
			header := &entry{kind: kindTimelineHeader, key: "timeline-header/" + e.key, at: e.at, columns: make([]*entry, len(lanes))}
			for i, id := range lanes {
				header.columns[i] = headers[id]
			}
			pageGroup = header.key
			header.pageGroup = pageGroup
			out = append(out, header)
		}
		if row == nil || current != lanes[0] {
			row = &entry{key: "timeline/" + e.key, pageGroup: pageGroup, at: e.at, columns: make([]*entry, len(lanes))}
			out = append(out, row)

			current = lanes[0]
			lastRuns = make([]string, len(lanes))
		}
		cell := *e
		cell.stepLabel, cell.stepContext = "", ""
		if lastRuns[lane] != e.step {
			if e.step != lanes[lane] || lastRuns[lane] != "" {
				if header := headers[e.step]; header != nil {
					cell.stepLabel, cell.stepContext = header.stepLabel, header.stepContext
				}
			}
			lastRuns[lane] = e.step
		}
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
				if isParallelBody(row) {
					for i := range row.columns {
						if lane := a.conversation.lanes[parallelLaneID(row, i)]; lane != nil {
							for _, item := range lane.entries {
								if contains(item) && !lane.rendered[item.key] {
									lane.scroller.Invalidate(item.key)
								}
							}
						}
					}
				} else {
					a.scroller.Invalidate(row.key)
				}
				return
			}
		}
	}
	if !a.conversation.renderedRows[key] {
		a.scroller.Invalidate(key)
	}
}

func (a *app) displayedConversationRows() []*entry {
	if a.conversationRows != nil {
		return a.conversationRows
	}
	return a.entries
}
