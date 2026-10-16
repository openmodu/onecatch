package keeldesktop

import (
	"fmt"
	"testing"
	"time"

	"github.com/dyike/keel/gio/f32"
	"github.com/dyike/keel/gio/io/pointer"
	"github.com/dyike/keel/ui/el"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

// File previews and folded process rows have very different heights. Reading
// back through them must not briefly move a visible card when earlier rows
// enter the virtual list and replace their estimated heights.
func TestConversationScrollDoesNotBounceWhileMeasuringHistory(t *testing.T) {
	for _, scale := range []int{1, 2} {
		t.Run(fmt.Sprint(scale), func(t *testing.T) {
			a := conversationFixture(t)
			clear(a.transcript.items)
			for i := range 60 {
				key := fmt.Sprintf("edit-%02d", i)
				change := previewFileChange()
				change.Path = fmt.Sprintf("/repo/file-%02d.go", i)
				a.transcript.items[key] = &entry{key: key, kind: agentrun.KindFileChange, text: change.Path, changes: []agentrun.FileChange{change}, at: time.Unix(int64(2*i), 0)}
				key = fmt.Sprintf("thought-%02d", i)
				a.transcript.items[key] = &entry{key: key, kind: agentrun.KindReasoning, text: "检查下一处修改", at: time.Unix(int64(2*i+1), 0)}
			}
			a.syncMessages()
			root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element {
				return el.Div().WFull().HFull().Child(a.conversationScroll(cx))
			}))
			router, frame, _ := alignmentRouter(root, 1000, 700, scale)
			for range 8 {
				frame()
			}
			positions := func() map[string]int {
				out := map[string]int{}
				for i := range 60 {
					name := fmt.Sprintf("file-%02d.go", i)
					if n, ok := alignmentNode(router.AppendSemantics(nil), name, false); ok && n.Bounds.Min.Y >= 0 && n.Bounds.Max.Y <= 700*scale {
						out[name] = n.Bounds.Min.Y
					}
				}
				return out
			}
			initial := positions()
			if len(initial) == 0 {
				t.Fatal("fixture has no visible file cards")
			}
			visited := map[string]bool{}
			comparisons := 0
			for _, dy := range []int{-96, 96} {
				for step := range 80 {
					previous := positions()
					router.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(float32(800*scale), float32(350*scale)), Scroll: f32.Pt(0, float32(dy*scale))})
					frame()
					before := positions()
					for name, y := range before {
						visited[name] = true
						if old, ok := previous[name]; ok && (y-old)*dy > 0 {
							t.Fatalf("scroll %d, delta %d: %s moved against input: %d -> %d", step, dy, name, old, y)
						}
					}
					// Also cover uninterrupted input across measurement frames.
					if step%4 != 0 {
						continue
					}
					for settle := range 3 {
						// Unchanged background snapshots must not disturb the anchor.
						a.syncMessages()
						frame()
						for name, y := range positions() {
							if old, ok := before[name]; ok {
								comparisons++
								if y != old {
									t.Fatalf("scroll %d, delta %d, idle frame %d: %s jumped from %d to %d without input", step, dy, settle, name, old, y)
								}
							}
						}
					}
				}
			}
			if len(visited) < 10 || comparisons < 20 {
				t.Fatalf("insufficient scrolling coverage: %d cards, %d position checks", len(visited), comparisons)
			}
		})
	}
}
