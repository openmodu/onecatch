package desktop

import (
	"reflect"
	"time"

	"github.com/kballard/go-shellquote"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

// Repair old calls that were saved without item IDs. Match the native command
// and timestamp, never the order of parallel completions. Existing IDs win.
func restoreCodexToolCalls(views []RuntimeEventView, cache map[string][]agentrun.CodexToolHistory) {
	session := ""
	needed := false
	for _, view := range views {
		if view.Kind == string(agentrun.KindStarted) {
			session = view.Text
		}
		if (view.Kind == string(agentrun.KindToolUse) || view.Kind == string(agentrun.KindToolResult)) && view.StreamID == "" {
			needed = true
		}
	}
	if !needed || session == "" {
		return
	}
	history, ok := cache[session]
	if !ok {
		history = agentrun.ReadCodexToolHistory(session)
		cache[session] = history
	}
	restoreCodexToolCallsFromHistory(views, history)
}

func restoreCodexToolCallsFromHistory(views []RuntimeEventView, history []agentrun.CodexToolHistory) {
	calls := map[string]bool{}
	for i := range views {
		view := &views[i]
		if view.Kind != string(agentrun.KindToolUse) {
			continue
		}
		if view.StreamID != "" {
			calls[view.StreamID] = true
			continue
		}
		at, _ := time.Parse(time.RFC3339Nano, view.At)
		words, _ := shellquote.Split(view.Text)
		match := ""
		for _, record := range history {
			if at.IsZero() || record.At.IsZero() || at.Sub(record.At).Abs() > 2*time.Second {
				continue
			}
			same := record.Text != "" && record.Text == view.Text
			same = same || len(words) > 0 && len(record.Command) > 0 && reflect.DeepEqual(words, record.Command)
			if !same {
				continue
			}
			if match != "" && match != record.ID {
				match = ""
				break
			}
			match = record.ID
		}
		if match != "" {
			view.StreamID = match
			view.Revision++
			calls[match] = true
		}
	}
	for i := range views {
		view := &views[i]
		if view.Kind != string(agentrun.KindToolResult) || view.StreamID != "" {
			continue
		}
		at, _ := time.Parse(time.RFC3339Nano, view.At)
		match := ""
		for _, record := range history {
			if !calls[record.ID] || record.Output != view.Text {
				continue
			}
			// Completed-item time disambiguates repeated/empty outputs. If it
			// is unavailable, only a nonempty unique output is recoverable.
			if !record.FinishedAt.IsZero() {
				if at.IsZero() || at.Sub(record.FinishedAt).Abs() > 100*time.Millisecond {
					continue
				}
			} else if view.Text == "" {
				continue
			}
			if match != "" && match != record.ID {
				match = ""
				break
			}
			match = record.ID
		}
		if match != "" {
			view.StreamID = match
			view.Revision++
		}
	}
}
