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
	key            string
	step, stream   string
	seq            int64
	kind           agentrun.EventKind
	text           string
	attachments    []conversationAttachment
	at             time.Time
	queueUpdatedAt time.Time
	revision       uint64
	streaming      bool
	failed         bool
	user           bool
	permission     *agentrun.PermissionRequest
	question       *agentrun.UserInputRequest
	answer         *agentrun.UserInputResponse
	process        []*entry
	details        []*entry
	settled        bool
	transition     *workflowTransition
	recipient      string
	// stepLabel names the workflow step that produced this row; it is set on
	// the first row of each run of output from one step.
	pageGroup   string
	columns     []*entry
	stack       []*entry
	stepLabel   string
	stepContext string
}

// kindTransition is a workflow event shown in the conversation: a step
// sending work back, or asking the person a question.
const kindTransition agentrun.EventKind = "workflow_transition"

// transcript joins durable history and live snapshots by stream identity and
// revision. Subscribe before loading history; merge rather than replace so a
// late disk read never rolls a live response back.
type transcript struct {
	complete            bool
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

func requestKey(step, id string) string { return step + "/request/" + id }

func (t *transcript) merge(e entry, delta bool) bool {
	if e.kind == agentrun.KindPermissionResolved && e.permission != nil {
		t.resolvedPermissions[requestKey(e.step, e.permission.ID)] = true
	}
	if e.kind == agentrun.KindUserInputResolved && e.question != nil {
		t.resolvedQuestions[requestKey(e.step, e.question.ID)] = true
		if e.answer != nil && !e.answer.Cancelled && !e.answer.Closed {
			var lines []string
			for _, q := range e.question.Questions {
				if answer := e.answer.Answers[q.ID]; answer != "" && !q.IsSecret {
					lines = append(lines, q.Question+"\n\n"+answer)
				}
			}
			if len(lines) > 0 {
				e.text, e.user, e.kind, e.question = strings.Join(lines, "\n\n"), true, agentrun.KindMessage, nil
			}
		}
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
	return t.merge(entry{key: eventKey(f.StepRunID, f.StreamID, f.Seq, f.Kind), step: f.StepRunID, stream: f.StreamID, seq: f.Seq, kind: f.Kind, text: f.Text, at: f.At, revision: f.Revision, streaming: f.Phase != "" && f.Phase != agentrun.StreamEnd, failed: f.Failed, permission: f.Permission, question: f.UserInput, answer: f.UserInputResponse}, f.Phase == agentrun.StreamDelta)
}

func (t *transcript) history(events []desktopservice.RuntimeEventView) {
	for _, v := range events {
		at, _ := time.Parse(time.RFC3339Nano, v.At)
		queueUpdatedAt, _ := time.Parse(time.RFC3339Nano, v.QueueUpdatedAt)
		t.merge(entry{key: eventKey(v.StepRunID, v.StreamID, v.Seq, agentrun.EventKind(v.Kind)), step: v.StepRunID, stream: v.StreamID, seq: v.Seq, kind: agentrun.EventKind(v.Kind), text: v.Text, at: at, queueUpdatedAt: queueUpdatedAt, revision: v.Revision, streaming: v.Streaming, failed: v.Failed, permission: v.Permission, question: v.UserInput, answer: v.UserInputResponse}, false)
	}
}

func (t *transcript) detail(d desktopservice.RunDetail) {
	attachments := make([]conversationAttachment, 0, len(d.Task.Attachments))
	for _, item := range d.Task.Attachments {
		attachments = append(attachments, conversationAttachment{Path: item.StoredPath, Name: item.Name, MIMEType: item.MIMEType})
	}
	t.items["prompt"] = &entry{key: "prompt", text: d.Task.Prompt, attachments: attachments, user: true, at: d.Task.CreatedAt}
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
	// Instructions are a complete snapshot. Rebuild applied turns only;
	// pending guidance belongs to the composer's queue until it is consumed.
	for key := range t.items {
		if strings.HasPrefix(key, "instruction/") {
			delete(t.items, key)
		}
	}
	for _, i := range d.Instructions {
		if i.Status != domainworkflows.InstructionApplied {
			continue
		}
		key := "instruction/" + i.ID
		at := i.CreatedAt
		if !i.AppliedAt.IsZero() {
			at = i.AppliedAt
		}
		recipient := i.StepID
		if step, ok := domainworkflows.FindStep(d.Workflow, i.StepID); ok {
			recipient = step.Name
		}
		t.items[key] = &entry{key: key, text: i.Content, attachments: instructionAttachments(i.Attachments), user: true, at: at, recipient: recipient}
	}
	t.transitions(d)
	t.history(d.RuntimeEvents)
}

// transitions turns rework and ask-human verdicts into conversation rows, so
// the hand-offs between steps read in order with the steps' own output.
func (t *transcript) transitions(d desktopservice.RunDetail) {
	if len(d.Workflow.Steps) < 2 {
		return
	}
	name := func(id string) string {
		if step, ok := domainworkflows.FindStep(d.Workflow, id); ok {
			return step.Name
		}
		return id
	}
	for _, event := range d.Events {
		if event.Type != "step.completed" {
			continue
		}
		var payload struct {
			Verdict, Summary, Feedback, Target string
		}
		if json.Unmarshal([]byte(event.Payload), &payload) != nil {
			continue
		}
		var text string
		switch payload.Verdict {
		case domainworkflows.VerdictRework:
			text = fmt.Sprintf("↩ 「%s」退回给「%s」：%s", name(event.StepID), name(payload.Target), firstLine(payload.Feedback))
		case domainworkflows.VerdictAskHuman:
			text = fmt.Sprintf("?「%s」在问你：%s", name(event.StepID), firstLine(payload.Summary))
		default:
			continue
		}
		at, _ := time.Parse(time.RFC3339Nano, event.At)
		key := fmt.Sprintf("transition/%d", event.Seq)
		t.items[key] = &entry{key: key, kind: kindTransition, text: text, at: at, transition: &workflowTransition{verdict: payload.Verdict, source: name(event.StepID), target: name(payload.Target), feedback: payload.Feedback, summary: payload.Summary}}
	}
}

func firstLine(text string) string {
	text = strings.TrimSpace(text)
	if i := strings.IndexByte(text, '\n'); i >= 0 {
		return strings.TrimSpace(text[:i]) + " …"
	}
	return text
}

func (t *transcript) visible() []*entry {
	out := make([]*entry, 0, len(t.items))
	for _, e := range t.items {
		switch e.kind {
		case agentrun.KindStarted, agentrun.KindUsage, agentrun.KindResult, agentrun.KindPermissionResolved, agentrun.KindUserInputResolved:
			continue
		}
		if e.permission != nil && t.resolvedPermissions[requestKey(e.step, e.permission.ID)] || e.question != nil && t.resolvedQuestions[requestKey(e.step, e.question.ID)] {
			continue
		}
		if e.text == "" && len(e.attachments) == 0 && e.permission == nil && e.question == nil && e.kind != agentrun.KindToolResult {
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
