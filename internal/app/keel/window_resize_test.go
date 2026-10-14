package keeldesktop

import (
	"image"
	"testing"
	"time"

	"github.com/dyike/keel/gio/io/input"
	"github.com/dyike/keel/gio/layout"
	"github.com/dyike/keel/gio/op"
	"github.com/dyike/keel/gio/unit"
	"github.com/dyike/keel/ui/el"
)

// Alternate growth and shrinkage with a new text width on each frame, like a
// window-edge drag. Fixed-size scrolling benchmarks miss layout remeasurement.
func BenchmarkWindowResize(b *testing.B) {
	for _, conversation := range []bool{false, true} {
		name := "draft"
		if conversation {
			name = "conversation"
		}
		b.Run(name, func(b *testing.B) {
			a, _ := conversationBench(b)
			if !conversation {
				a.task, a.detail = nil, nil
				a.taskID, a.runID = "", ""
			}
			root := el.Root(a)
			var router input.Router
			var ops op.Ops
			frame := func(i int) {
				step := i % 240
				if step >= 120 {
					step = 239 - step
				}
				ops.Reset()
				root.Layout(layout.Context{Ops: &ops, Now: time.Now(), Source: router.Source(), Constraints: layout.Exact(image.Pt((860+step*5)*2, (600+step)*2)), Metric: unit.Metric{PxPerDp: 2, PxPerSp: 2}})
				router.Frame(&ops)
			}
			for i := range 3 {
				frame(i)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := range b.N {
				frame(i)
			}
		})
	}
}
