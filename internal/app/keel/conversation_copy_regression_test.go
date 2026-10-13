package keeldesktop

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/pointer"
	"github.com/dyike/keel/ui/el"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

func TestConversationRepeatedDragCopyAfterScrolling(t *testing.T) {
	a := conversationFixture(t)
	a.transcript = newTranscript()
	var source strings.Builder
	for i := 0; i < 36; i++ {
		fmt.Fprintf(&source, "## 第 %02d 节\n\n- 模板发布不检查类型表。\n- `internal/sandbox_tool/usecase`：发布、回滚、删除、查询的领域规则和事务边界。\n\n第 %02d 段，混合 **中文文字** 和 `inline code`，用于稳定选择复制。\n\n", i, i)
	}
	a.transcript.items["long-copy"] = &entry{key: "long-copy", kind: agentrun.KindMessage, text: source.String(), at: time.Now()}
	a.syncMessages()
	a.scroller.SetFollow(false)
	root := el.Root(a)
	router, frame, click := alignmentRouter(root, 1280, 800, 1)
	for range 5 {
		frame()
	}
	for attempt := 0; attempt < 8; attempt++ {
		prompt, ok := alignmentNode(router.AppendSemantics(nil), "任务指令", false)
		if !ok {
			t.Fatal("missing prompt")
		}
		if attempt%2 == 0 {
			click(prompt.Bounds)
		}
		p := f32.Pt(650, 300)
		router.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: p, Scroll: f32.Pt(0, 120)})
		for range 4 {
			frame()
		}
		var start, end f32.Point
		found := false
		for i := 0; i < 36; i++ {
			label := fmt.Sprintf("第 %02d 段，混合 中文文字 和 inline code，用于稳定选择复制。", i)
			node, ok := alignmentNode(router.AppendSemantics(nil), label, false)
			if ok && node.Bounds.Min.Y > 150 && node.Bounds.Max.Y < 550 {
				start = f32.Pt(float32(node.Bounds.Min.X+1), float32(node.Bounds.Min.Y+node.Bounds.Dy()/2))
				end = f32.Pt(float32(node.Bounds.Max.X-2), start.Y)
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("attempt %d: no visible paragraph", attempt)
		}
		if attempt%2 == 1 {
			start, end = end, start
		}
		router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: start}, pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: start})
		if attempt%3 != 0 {
			frame()
		}
		router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: end})
		if attempt%3 != 0 {
			frame()
			frame()
		}
		router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: end})
		frame()
		frame()
		selectionKey(router, frame, "C")
		_, data, ok := router.WriteClipboard()
		if !ok || !strings.Contains(string(data), "稳定选择复制") {
			t.Fatalf("attempt %d: drag did not copy paragraph: %q", attempt, data)
		}
	}
}
