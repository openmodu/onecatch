package keeldesktop

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"
	desktopservice "github.com/openmodu/onecatch/internal/service/desktop"
	"github.com/openmodu/onecatch/internal/service/desktop/runstream"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

type entry struct {
	key          string
	step, stream string
	seq          int64
	kind         agentrun.EventKind
	text         string
	at           time.Time
	revision     uint64
	streaming    bool
	failed       bool
	user         bool
	permission   *agentrun.PermissionRequest
	question     *agentrun.UserInputRequest
	process      []*entry
	details      []*entry
	settled      bool
}

// transcript joins durable history and live snapshots by stream identity and
// revision. Subscribe before loading history; merge rather than replace so a
// late disk read never rolls a live response back.
type transcript struct {
	items               map[string]*entry
	resolvedPermissions map[string]bool
	resolvedQuestions   map[string]bool
}

func newTranscript() *transcript {
	return &transcript{items: make(map[string]*entry), resolvedPermissions: make(map[string]bool), resolvedQuestions: make(map[string]bool)}
}

func eventKey(step, stream string, seq int64, kind agentrun.EventKind) string {
	if stream != "" {
		if kind == agentrun.KindToolUse || kind == agentrun.KindToolResult {
			return step + "/stream/" + stream + "/" + string(kind)
		}
		return step + "/stream/" + stream
	}
	return fmt.Sprintf("%s/event/%d/%s", step, seq, kind)
}

func (t *transcript) merge(e entry, delta bool) bool {
	if e.kind == agentrun.KindPermissionResolved && e.permission != nil {
		t.resolvedPermissions[e.permission.ID] = true
	}
	if e.kind == agentrun.KindUserInputResolved && e.question != nil {
		t.resolvedQuestions[e.question.ID] = true
	}
	old := t.items[e.key]
	if old != nil && (e.revision <= old.revision) {
		return false
	}
	if delta && old != nil {
		e.text = old.text + e.text
	}
	if old != nil {
		// Keep the logical message at its starting position as tokens arrive.
		e.at = old.at
	}
	t.items[e.key] = &e
	return true
}

func (t *transcript) frame(f runstream.Frame) bool {
	return t.merge(entry{key: eventKey(f.StepRunID, f.StreamID, f.Seq, f.Kind), step: f.StepRunID, stream: f.StreamID, seq: f.Seq, kind: f.Kind, text: f.Text, at: f.At, revision: f.Revision, streaming: f.Phase != "" && f.Phase != agentrun.StreamEnd, failed: f.Failed, permission: f.Permission, question: f.UserInput}, f.Phase == agentrun.StreamDelta)
}

func (t *transcript) history(events []desktopservice.RuntimeEventView) {
	for _, v := range events {
		at, _ := time.Parse(time.RFC3339Nano, v.At)
		t.merge(entry{key: eventKey(v.StepRunID, v.StreamID, v.Seq, agentrun.EventKind(v.Kind)), step: v.StepRunID, stream: v.StreamID, seq: v.Seq, kind: agentrun.EventKind(v.Kind), text: v.Text, at: at, revision: v.Revision, streaming: v.Streaming, failed: v.Failed, permission: v.Permission, question: v.UserInput}, false)
	}
}

func (t *transcript) detail(d desktopservice.RunDetail) {
	t.items["prompt"] = &entry{key: "prompt", text: d.Task.Prompt, user: true, at: d.Task.CreatedAt}
	// Older, text-only continuations are stored as workflow events rather
	// than queued instructions. They must be part of the conversation too.
	for _, event := range d.Events {
		if event.Type != "run.resumed" {
			continue
		}
		var payload struct {
			Instruction string `json:"instruction"`
		}
		if json.Unmarshal([]byte(event.Payload), &payload) != nil || strings.TrimSpace(payload.Instruction) == "" {
			continue
		}
		at, _ := time.Parse(time.RFC3339Nano, event.At)
		key := fmt.Sprintf("resume/%d", event.Seq)
		t.items[key] = &entry{key: key, text: payload.Instruction, user: true, at: at}
	}
	// Instructions are a complete snapshot. Remove stale/removed entries so
	// deleting a queued message does not leave a ghost bubble behind.
	for key := range t.items {
		if strings.HasPrefix(key, "instruction/") {
			delete(t.items, key)
		}
	}
	for _, i := range d.Instructions {
		if i.Status == domainworkflows.InstructionRemoved {
			continue
		}
		key := "instruction/" + i.ID
		at := i.CreatedAt
		if !i.AppliedAt.IsZero() {
			at = i.AppliedAt
		}
		t.items[key] = &entry{key: key, text: i.Content, user: true, at: at}
	}
	t.history(d.RuntimeEvents)
}

func (t *transcript) visible() []*entry {
	out := make([]*entry, 0, len(t.items))
	for _, e := range t.items {
		switch e.kind {
		case agentrun.KindStarted, agentrun.KindUsage, agentrun.KindResult, agentrun.KindPermissionResolved, agentrun.KindUserInputResolved:
			continue
		}
		if e.permission != nil && t.resolvedPermissions[e.permission.ID] || e.question != nil && t.resolvedQuestions[e.question.ID] {
			continue
		}
		if e.text == "" && e.permission == nil && e.question == nil && e.kind != agentrun.KindToolResult {
			continue
		}
		out = append(out, e)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].at.Equal(out[j].at) {
			if out[i].step == out[j].step && out[i].seq != out[j].seq {
				return out[i].seq < out[j].seq
			}
			return out[i].key < out[j].key
		}
		return out[i].at.Before(out[j].at)
	})
	return out
}
