package keeldesktop

import (
	"fmt"
	"image"
	"strings"
	"testing"
	"time"

	"github.com/dyike/keel/gio/f32"
	"github.com/dyike/keel/gio/io/input"
	"github.com/dyike/keel/gio/io/pointer"
	"github.com/dyike/keel/gio/layout"
	"github.com/dyike/keel/gio/op"
	"github.com/dyike/keel/gio/unit"
	"github.com/dyike/keel/ui/el"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

func parallelConversationFixture(t *testing.T, count int) *app {
	a := conversationFixture(t)
	at := time.Date(2026, 10, 12, 10, 0, 0, 0, time.Local)
	a.detail = timelineDetail(at)
	a.transcript = newTranscript()
	for i := range count {
		for _, step := range []string{"a", "b"} {
			text := benchAnswer
			if step == "a" {
				text = strings.Repeat(benchAnswer, 3)
			}
			e := &entry{key: fmt.Sprintf("%s%d", step, i), step: step, kind: agentrun.KindMessage, text: text, at: at.Add(time.Duration(i+1) * time.Millisecond)}
			a.transcript.items[e.key] = e
		}
	}
	a.syncMessages()
	return a
}

func BenchmarkConversationParallelScroll(b *testing.B) {
	for _, count := range []int{4, 60} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			a := parallelConversationFixture(&testing.T{}, count)
			b.Cleanup(a.close)
			root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element { return el.Div().WFull().HFull().Child(a.conversationScroll(cx)) }))
			var router input.Router
			var ops op.Ops
			frame := func() {
				ops.Reset()
				root.Layout(layout.Context{Ops: &ops, Now: time.Now(), Source: router.Source(), Constraints: layout.Exact(image.Pt(1100*2, 700*2)), Metric: unit.Metric{PxPerDp: 2, PxPerSp: 2}})
				router.Frame(&ops)
			}
			for range 8 {
				frame()
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := range b.N {
				dy := float32(-96)
				if i/80%2 != 0 {
					dy = -dy
				}
				router.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(600, 400), Scroll: f32.Pt(0, dy)})
				frame()
			}
		})
	}
}
