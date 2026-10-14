package keeldesktop

import (
	"image"
	"strings"
	"testing"
	"time"

	"github.com/dyike/keel/gio/io/input"
	"github.com/dyike/keel/gio/layout"
	"github.com/dyike/keel/gio/op"
	"github.com/dyike/keel/gio/unit"
	"github.com/dyike/keel/ui/el"
	"github.com/openmodu/onecatch/internal/service/desktop/runstream"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

// A burst of deltas schedules one UI update, which merges every frame of the
// open run and rebuilds the conversation once.
func TestLiveFramesBatchPerUpdate(t *testing.T) {
	a := conversationFixture(t)
	var queued []func()
	a.postUpdate = func(fn func()) { queued = append(queued, fn) }
	delta := func(run string, rev uint64, text string) runstream.Frame {
		f := runstream.Frame{RunID: run, StepRunID: "live", Seq: 50}
		f.Kind, f.StreamID, f.Phase, f.Revision, f.Text, f.At = agentrun.KindMessage, "answer", agentrun.StreamDelta, rev, text, time.Now()
		return f
	}
	for i, word := range []string{"实时", "渲染", "不卡"} {
		a.receiveFrame(delta(a.runID, uint64(i+1), word))
	}
	a.receiveFrame(delta("other-run", 9, "不该出现"))
	if len(queued) != 1 {
		t.Fatalf("%d UI updates scheduled for one burst", len(queued))
	}
	queued[0]()
	var got string
	for _, e := range a.entries {
		if e.kind == agentrun.KindMessage && e.step == "live" {
			got = e.text
		}
	}
	if got != "实时渲染不卡" || strings.Contains(got, "不该出现") {
		t.Fatalf("merged answer %q", got)
	}
	// The next frame schedules a new update.
	a.receiveFrame(delta(a.runID, 4, "。"))
	if len(queued) != 2 {
		t.Fatal("a frame after the update was not scheduled")
	}
}

// Rows are re-measured only when what they draw changes.
func TestRowSignatureTracksContent(t *testing.T) {
	e := &entry{key: "a", kind: agentrun.KindMessage, text: "hi", revision: 1, streaming: true}
	base := rowSignature(e, true, true)
	if rowSignature(e, true, true) != base {
		t.Fatal("signature is not stable")
	}
	grown := *e
	grown.text, grown.revision = "hi there", 2
	if rowSignature(&grown, true, true) == base || rowSignature(e, false, true) == base || rowSignature(e, true, false) == base {
		t.Fatal("content, run activity or last-row changes must re-measure the row")
	}
	// A settled message draws the same whether or not the run is active.
	settled := entry{key: "b", kind: agentrun.KindMessage, text: "done", revision: 3}
	if rowSignature(&settled, true, true) != rowSignature(&settled, false, false) {
		t.Fatal("a settled message must not re-measure when the run ends")
	}
}

// Keel repaints the whole window for every update. A folded tool's streaming
// output changes nothing on screen, so it must not request a redraw.
func TestLiveFramesSkipRedrawForFoldedToolOutput(t *testing.T) {
	a := conversationFixture(t)
	a.detail.Active = true
	a.postUpdate = func(func()) {}
	a.syncMessages()
	root := el.Root(a)
	var router input.Router
	var ops op.Ops
	frame := func() {
		ops.Reset()
		root.Layout(layout.Context{Ops: &ops, Now: time.Now(), Source: router.Source(), Constraints: layout.Exact(image.Pt(1288, 804)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}})
		router.Frame(&ops)
	}
	at := time.Now()
	revision := uint64(0)
	apply := func(kind agentrun.EventKind, stream string, phase agentrun.StreamPhase, text string) bool {
		revision++
		f := runstream.Frame{RunID: a.runID, StepRunID: "live", Seq: 90}
		f.Kind, f.StreamID, f.Phase, f.Revision, f.Text, f.At = kind, stream, phase, revision, text, at.Add(time.Duration(revision)*time.Millisecond)
		a.receiveFrame(f)
		return a.applyLiveFrames()
	}
	if !apply(agentrun.KindToolUse, "build", agentrun.StreamEnd, `Bash {"command":"go test ./..."}`) {
		t.Fatal("a new tool row did not redraw")
	}
	a.scroller.ScrollToEnd()
	for range 3 {
		frame()
	}
	toolKey := ""
	for _, e := range a.entries {
		for _, p := range e.process {
			if p.step == "live" && p.kind == agentrun.KindToolUse {
				toolKey = p.key
			}
		}
	}
	if toolKey == "" {
		t.Fatal("live tool row missing")
	}

	if !apply(agentrun.KindToolResult, "build", agentrun.StreamStart, "") {
		// The call's state mark changes when its first output arrives.
		t.Log("first output drew no visible change")
	}
	frame()
	for i := range 20 {
		if apply(agentrun.KindToolResult, "build", agentrun.StreamDelta, "ok  pkg/"+string(rune('a'+i))+"\n") {
			t.Fatalf("folded output delta %d requested a redraw", i)
		}
	}
	var result string
	for _, e := range a.entries {
		for _, p := range e.process {
			if p.key == toolKey && len(p.details) > 0 {
				result = p.details[len(p.details)-1].text
			}
		}
	}
	if !strings.HasPrefix(result, "ok  pkg/a\n") || !strings.HasSuffix(result, "ok  pkg/t\n") {
		t.Fatalf("hidden output was not merged: %q", result)
	}

	// Finishing the call shows its state and duration.
	if !apply(agentrun.KindToolResult, "build", agentrun.StreamEnd, "done\n") {
		t.Fatal("a finished tool did not redraw")
	}
	frame()

	// Opened, the same output is on screen and must redraw.
	a.toggleConversation(toolKey, false)
	frame()
	if !apply(agentrun.KindToolResult, "build", agentrun.StreamDelta, "shown\n") {
		t.Fatal("opened tool output did not redraw")
	}
	a.toggleConversation(toolKey, false)
	frame()

	// A visible reply streaming at the bottom always redraws.
	if !apply(agentrun.KindMessage, "answer", agentrun.StreamStart, "完成") {
		t.Fatal("a new reply did not redraw")
	}
	frame()
	if !apply(agentrun.KindMessage, "answer", agentrun.StreamDelta, "了") {
		t.Fatal("a visible streaming reply did not redraw")
	}
}

// Reused record copies must build the same conversation as fresh ones, as
// tool results pair, queue snapshots fold and records are replaced.
func TestReusedEntryCopiesMatchFreshBuild(t *testing.T) {
	tr := newTranscript()
	var copies entryCopies
	at := time.Date(2026, 10, 14, 9, 0, 0, 0, time.Local)
	frames := []runstream.Frame{}
	add := func(step, stream string, kind agentrun.EventKind, phase agentrun.StreamPhase, rev uint64, text string, offset int) {
		f := runstream.Frame{RunID: "r", StepRunID: step, Seq: int64(offset)}
		f.Kind, f.StreamID, f.Phase, f.Revision, f.Text, f.At = kind, stream, phase, rev, text, at.Add(time.Duration(offset)*time.Second)
		frames = append(frames, f)
	}
	add("s1", "q", agentrun.KindQueueStatus, "", 1, `{"state":"queued"}`, 0)
	add("s1", "q", agentrun.KindQueueStatus, "", 2, `{"state":"ready"}`, 0)
	add("s1", "a", agentrun.KindToolUse, agentrun.StreamEnd, 1, `Bash {"command":"ls"}`, 1)
	add("s1", "a", agentrun.KindToolResult, agentrun.StreamStart, 1, "", 2)
	add("s1", "a", agentrun.KindToolResult, agentrun.StreamDelta, 2, "a.go\n", 2)
	add("s1", "b", agentrun.KindToolUse, agentrun.StreamEnd, 1, `Read {"file_path":"a.go"}`, 3)
	add("s1", "a", agentrun.KindToolResult, agentrun.StreamEnd, 3, "a.go\nb.go\n", 2)
	add("s1", "b", agentrun.KindToolResult, agentrun.StreamEnd, 1, "package main", 4)
	add("s1", "m", agentrun.KindMessage, agentrun.StreamStart, 1, "完成", 5)
	add("s2", "c", agentrun.KindToolUse, agentrun.StreamEnd, 1, `Bash {"command":"pwd"}`, 6)
	add("s1", "m", agentrun.KindMessage, agentrun.StreamDelta, 2, "了", 5)
	for i, f := range frames {
		tr.frame(f)
		reused := buildConversationEntriesWith(tr.visible(), &copies)
		fresh := buildConversationEntries(tr.visible())
		if len(reused) != len(fresh) {
			t.Fatalf("frame %d: %d rows, fresh build has %d", i, len(reused), len(fresh))
		}
		for j := range fresh {
			if rowSignature(reused[j], true, false) != rowSignature(fresh[j], true, false) {
				t.Fatalf("frame %d row %d (%s) differs from a fresh build", i, j, fresh[j].key)
			}
		}
	}
}

// Deltas append in place; text already handed out must not change, and a
// snapshot replaces the stream's text.
func TestTranscriptDeltasAppendWithoutChangingEarlierText(t *testing.T) {
	tr := newTranscript()
	frame := func(phase agentrun.StreamPhase, rev uint64, text string) string {
		f := runstream.Frame{RunID: "r", StepRunID: "s", Seq: 1}
		f.Kind, f.StreamID, f.Phase, f.Revision, f.Text = agentrun.KindToolResult, "out", phase, rev, text
		tr.frame(f)
		return tr.items[eventKey("s", "out", 1, agentrun.KindToolResult)].text
	}
	first := frame(agentrun.StreamStart, 1, "a")
	second := frame(agentrun.StreamDelta, 2, "b")
	third := frame(agentrun.StreamDelta, 3, "c")
	if frame(agentrun.StreamDelta, 3, "stale") != "abc" {
		t.Fatal("a delta with an old revision was applied")
	}
	if first != "a" || second != "ab" || third != "abc" {
		t.Fatalf("earlier text changed: %q %q %q", first, second, third)
	}
	if got := frame(agentrun.StreamEnd, 4, "final"); got != "final" {
		t.Fatalf("snapshot text %q", got)
	}
	if got := frame(agentrun.StreamDelta, 5, "!"); got != "final!" {
		t.Fatalf("delta after a snapshot appended to stale text: %q", got)
	}
	if third != "abc" {
		t.Fatalf("returned text changed after more deltas: %q", third)
	}
}
