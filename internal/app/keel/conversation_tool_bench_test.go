package keeldesktop

import (
	"fmt"
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

// A run with many finished tool calls (results ~8 KB each), plus one live
// tool whose output streams in deltas.
func toolHeavyBench(b *testing.B, rounds, tools int) (*app, func()) {
	t := &testing.T{}
	a := conversationFixture(t)
	b.Cleanup(a.close)
	at := time.Date(2026, 10, 10, 8, 0, 0, 0, time.Local)
	out := strings.Repeat("drwxr-xr-x  12 user staff  384 Oct 10 08:00 internal/app/keel\n", 130)
	for i := range rounds {
		step := fmt.Sprintf("s%d", i)
		base := at.Add(time.Duration(i) * time.Minute)
		user := entry{key: "u" + step, user: true, text: fmt.Sprintf("问题 %d", i), at: base}
		a.transcript.items[user.key] = &user
		for j := range tools {
			id := fmt.Sprintf("c%d", j)
			use := entry{key: step + "/stream/" + id + "/tool_use", step: step, stream: id, kind: agentrun.KindToolUse, text: `Bash {"command":"ls -la"}`, at: base.Add(time.Duration(j+1) * time.Second), revision: 1}
			res := entry{key: step + "/stream/" + id + "/tool_result", step: step, stream: id, kind: agentrun.KindToolResult, text: out, at: base.Add(time.Duration(j+1)*time.Second + time.Millisecond), revision: 1}
			a.transcript.items[use.key], a.transcript.items[res.key] = &use, &res
		}
		reply := entry{key: "a" + step, step: step, kind: agentrun.KindMessage, text: benchAnswer, at: base.Add(50 * time.Second)}
		a.transcript.items[reply.key] = &reply
	}
	a.detail.Active = true
	a.postUpdate = func(func()) {}
	a.syncMessages()
	root := el.Root(a)
	var router input.Router
	var ops op.Ops
	frame := func() {
		ops.Reset()
		root.Layout(layout.Context{Ops: &ops, Now: time.Now(), Source: router.Source(), Constraints: layout.Exact(image.Pt(1288*2, 804*2)), Metric: unit.Metric{PxPerDp: 2, PxPerSp: 2}})
		router.Frame(&ops)
	}
	a.scroller.ScrollToEnd()
	for range 3 {
		frame()
	}
	return a, frame
}

var liveToolSeq int

func liveToolFrame(i int) runstream.Frame {
	at := time.Date(2026, 10, 10, 12, 0, 0, 0, time.Local)
	f := runstream.Frame{RunID: "preview", StepRunID: "live", Seq: 99999}
	f.StreamID = "lc"
	if i == 0 {
		f.Kind, f.Phase, f.Revision, f.Text, f.At = agentrun.KindToolUse, agentrun.StreamEnd, 1, `Bash {"command":"go test ./..."}`, at
		return f
	}
	f.Kind, f.Phase, f.Revision, f.Text, f.At = agentrun.KindToolResult, agentrun.StreamDelta, uint64(i+1), "ok  github.com/openmodu/onecatch/internal/pkg  0.123s\n", at.Add(time.Millisecond)
	return f
}

func BenchmarkToolHeavyLiveTool(b *testing.B) {
	for _, size := range []int{10, 40} {
		b.Run(fmt.Sprintf("rounds=%d", size), func(b *testing.B) {
			a, frame := toolHeavyBench(b, size, 10)
			a.receiveFrame(liveToolFrame(0))
			a.applyLiveFrames()
			frame()
			b.Run("sync", func(b *testing.B) {
				b.ReportAllocs()
				for range b.N {
					liveToolSeq++
					a.receiveFrame(liveToolFrame(liveToolSeq))
					a.applyLiveFrames()
				}
			})
			b.Run("layout", func(b *testing.B) {
				b.ReportAllocs()
				for range b.N {
					frame()
				}
			})
			// Open the last finished round's group and every tool in it.
			step := fmt.Sprintf("s%d", size-1)
			for _, e := range a.entries {
				if e.step == step && len(e.process) > 0 {
					a.toggleConversation(e.key, false)
					for _, p := range e.process {
						a.toggleConversation(p.key, false)
					}
				}
			}
			a.scroller.ScrollToEnd()
			for range 3 {
				frame()
			}
			b.Run("layout-open", func(b *testing.B) {
				b.ReportAllocs()
				for range b.N {
					frame()
				}
			})
		})
	}
}
