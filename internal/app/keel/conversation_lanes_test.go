package keeldesktop

import (
	"fmt"
	"image"
	"strings"
	"testing"

	"github.com/dyike/keel/gio/f32"
	"github.com/dyike/keel/gio/io/pointer"
	"github.com/dyike/keel/ui/el"
)

func TestParallelLanesFixedHeightAndIndependentScrolling(t *testing.T) {
	for _, scale := range []int{1, 2} {
		t.Run(fmt.Sprint(scale), func(t *testing.T) {
			a := parallelConversationFixture(t, 1)
			a.transcript.items["a0"].text = ""
			for i := range 100 {
				a.transcript.items["a0"].text += fmt.Sprintf("左侧第 %d 段\n\n", i)
			}
			a.transcript.items["b0"].text = "右侧开头\n\n" + strings.Repeat("右侧内容。\n\n", 30)
			a.syncMessages()
			a.scroller.SetFollow(false)
			for _, lane := range a.conversation.lanes {
				lane.scroller.SetFollow(false)
			}
			root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element {
				return el.Div().WFull().HFull().Child(a.conversationScroll(cx))
			}))
			router, frame, _ := alignmentRouter(root, 1100, 800, scale)
			for range 8 {
				frame()
			}
			bounds := func(name string) image.Rectangle {
				t.Helper()
				node, ok := alignmentNode(router.AppendSemantics(nil), name, false)
				if !ok {
					t.Fatalf("missing %s", name)
				}
				return node.Bounds
			}
			left, right := bounds("并行任务 1"), bounds("并行任务 2")
			if left.Dy() != parallelLaneHeight*scale || right.Dy() != left.Dy() || left.Min.Y != right.Min.Y {
				t.Fatalf("unequal lane viewports: %v %v", left, right)
			}
			leftText, rightText := bounds("左侧第 8 段"), bounds("右侧开头")
			p := f32.Pt(float32(left.Min.X+left.Dx()/2), float32(left.Min.Y+left.Dy()/2))
			router.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: p, Scroll: f32.Pt(0, float32(160*scale))})
			frame()
			if got := bounds("右侧开头"); got != rightText {
				t.Fatalf("left wheel moved right text: %v -> %v", rightText, got)
			}
			if got := bounds("左侧第 8 段"); got.Min.Y >= leftText.Min.Y {
				t.Fatalf("left lane did not scroll: %v -> %v", leftText, got)
			}
			for range 4 {
				frame()
			}
			afterScroll := bounds("左侧第 8 段")
			a.transcript.items["b0"].text += strings.Repeat("后台追加右侧。\n\n", 40)
			a.transcript.items["b0"].revision++
			a.syncMessages()
			for i := range 6 {
				frame()
				if got := bounds("并行任务 1"); got != left {
					t.Fatalf("frame %d: growing lane moved outer row: %v -> %v", i, left, got)
				}
				if got := bounds("左侧第 8 段"); got != afterScroll {
					t.Fatalf("frame %d: background growth moved reader: %v -> %v", i, afterScroll, got)
				}
			}
			// Removing messages must prune their independent scroll state.
			a.transcript = newTranscript()
			a.syncMessages()
			if len(a.conversation.lanes) != 0 {
				t.Fatal("removed lanes retained")
			}
		})
	}
}

func TestConversationGrowingVisibleReplyKeepsReadingPosition(t *testing.T) {
	for _, parallel := range []bool{false, true} {
		t.Run(fmt.Sprint(parallel), func(t *testing.T) {
			a := parallelConversationFixture(t, 1)
			if !parallel {
				a.detail.Workflow.Steps = nil
			}
			a.transcript.items["a0"].text = ""
			for i := range 100 {
				a.transcript.items["a0"].text += fmt.Sprintf("阅读位置第 %d 段\n\n", i)
			}
			a.syncMessages()
			a.scroller.SetFollow(false)
			for _, lane := range a.conversation.lanes {
				lane.scroller.SetFollow(false)
			}
			root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element { return el.Div().WFull().HFull().Child(a.conversationScroll(cx)) }))
			router, frame, _ := alignmentRouter(root, 1100, 800, 1)
			for range 8 {
				frame()
			}
			p := f32.Pt(300, 300)
			router.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: p, Scroll: f32.Pt(0, 600)})
			for range 5 {
				frame()
			}
			node, ok := alignmentNode(router.AppendSemantics(nil), "阅读位置第 20 段", false)
			if !ok {
				t.Fatal("reading marker missing")
			}
			before := node.Bounds
			for update := range 3 {
				a.transcript.items["a0"].text += "新增正文。\n\n"
				a.transcript.items["a0"].revision++
				a.syncMessages()
				for frameIndex := range 3 {
					frame()
					node, ok := alignmentNode(router.AppendSemantics(nil), "阅读位置第 20 段", false)
					if !ok || node.Bounds != before {
						t.Fatalf("update %d frame %d moved reader: %v -> %v (found=%v)", update, frameIndex, before, node.Bounds, ok)
					}
				}
			}
		})
	}
}

func TestParallelLaneCopySurvivesOtherLaneUpdate(t *testing.T) {
	a := parallelConversationFixture(t, 1)
	want := "并行任务中的这段正文应完整复制。"
	a.transcript.items["a0"].text = want
	a.transcript.items["b0"].text = "另一列持续输出。"
	a.syncMessages()
	root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element { return el.Div().WFull().HFull().Child(a.conversationScroll(cx)) }))
	router, frame, _ := alignmentRouter(root, 1100, 800, 1)
	for range 6 {
		frame()
	}
	paragraph, ok := alignmentNode(router.AppendSemantics(nil), want, false)
	if !ok {
		t.Fatal("missing selected paragraph")
	}
	start := f32.Pt(float32(paragraph.Bounds.Min.X+1), float32(paragraph.Bounds.Min.Y+paragraph.Bounds.Dy()/2))
	end := f32.Pt(float32(paragraph.Bounds.Max.X-1), start.Y)
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: start})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: end})
	frame()
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: end})
	frame()
	a.transcript.items["b0"].text += strings.Repeat("新内容。\n\n", 30)
	a.transcript.items["b0"].revision++
	a.syncMessages()
	frame()
	selectionKey(router, frame, "C")
	_, data, ok := router.WriteClipboard()
	if !ok || string(data) != want {
		t.Fatalf("parallel copy = %q, want %q", data, want)
	}
}
