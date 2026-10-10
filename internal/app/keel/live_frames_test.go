package keeldesktop

import (
	"strings"
	"testing"
	"time"

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
