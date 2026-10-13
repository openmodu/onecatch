package keeldesktop

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/dyike/keel/gio/f32"
	"github.com/dyike/keel/gio/io/pointer"
	"github.com/dyike/keel/ui/el"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

func TestConversationCopiesWrappedRepliesBetweenLiveTools(t *testing.T) {
	for _, scale := range []int{1, 2} {
		t.Run(fmt.Sprint(scale), func(t *testing.T) {
			a := conversationFixture(t)
			a.detail.Active = true
			a.transcript = newTranscript()
			texts := []string{
				"我先核对当前仓库已经解决了哪些问题、还剩哪些性能热点，再给你具体的优化方向。你提到的 `replace` 传递问题，仓库之前做过单模块迁移，我也会确认现在是否还受影响。",
				"当前仓库的单模块迁移已经解决了 Fork 交付问题：`gio/` 和 `typesetting/` 都随 Keel 发布，不再靠应用补 `replace`。我也重新验证了 cgo 状态：macOS 的 `native/*` 可以无 cgo 构建，完整 UI 仍卡在 Gio GL 等绑定上。性能方面，仓库已有虚拟列表、文字缓存和 Metal 批处理；一个值得测量的新方向是 `core.Update` 每次都唤醒所有窗口，可能增加多窗口应用的布局工作。",
			}
			for i, text := range texts {
				id := fmt.Sprintf("copy-reply-%d", i)
				a.transcript.items[id] = &entry{key: id, kind: agentrun.KindMessage, text: text, seq: int64(i * 10), at: time.Unix(int64(i*10), 0)}
			}
			for i := range 7 {
				id := fmt.Sprintf("tool-%d", i)
				a.transcript.items[id] = &entry{key: id, kind: agentrun.KindToolUse, text: `{"command":"git status --short"}`, seq: int64(i + 1), at: time.Unix(int64(i+1), 0), settled: true}
			}
			live := &entry{key: "live-tool", kind: agentrun.KindToolResult, text: "处理中", seq: 12, at: time.Unix(12, 0), streaming: true}
			a.transcript.items[live.key] = live
			a.syncMessages()
			a.scroller.SetFollow(false)
			root := el.Root(a)
			router, frame, _ := alignmentRouter(root, 1600, 1100, scale)
			paint := func() {
				live.text += "."
				live.revision++
				a.syncMessages()
				frame()
			}
			for range 5 {
				paint()
			}
			clock := time.Second
			for attempt := range 8 {
				index := attempt % 2
				want := strings.ReplaceAll(texts[index], "`", "")
				paragraph, ok := alignmentNode(router.AppendSemantics(nil), want, false)
				if !ok {
					t.Fatalf("reply %d not visible", index)
				}
				start := f32.Pt(float32(paragraph.Bounds.Min.X+1), float32(paragraph.Bounds.Min.Y+10*scale))
				end := f32.Pt(float32(paragraph.Bounds.Max.X-1), float32(paragraph.Bounds.Max.Y-10*scale))
				if attempt%4 >= 2 {
					start, end = end, start
				}
				clock += time.Second
				router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: start, Time: clock}, pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: start, Time: clock})
				if attempt%3 != 0 {
					paint()
				}
				clock += 100 * time.Millisecond
				router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: end, Time: clock})
				if attempt%3 != 0 {
					paint()
					paint()
				}
				router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: end, Time: clock + time.Millisecond})
				paint()
				paint()
				selectionKey(router, paint, "C")
				_, copied, ok := router.WriteClipboard()
				if !ok || string(copied) != want {
					t.Fatalf("attempt %d: selected reply copy = %q, want %q", attempt, copied, want)
				}
			}
		})
	}
}

func TestConversationCopySurvivesUnrelatedMessageSnapshot(t *testing.T) {
	a := conversationFixture(t)
	a.detail.Active = true
	a.transcript = newTranscript()
	want := "这段正文已完成，选择后仍应完整复制。"
	a.transcript.items["selected"] = &entry{key: "selected", kind: agentrun.KindMessage, text: want, at: time.Unix(1, 0)}
	tail := &entry{key: "background", kind: agentrun.KindMessage, text: "后台工具运行情况", at: time.Unix(2, 0)}
	a.transcript.items[tail.key] = tail
	a.syncMessages()
	root := el.Root(a)
	router, frame, _ := alignmentRouter(root, 1280, 800, 1)
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
	frame()
	selected := a.docs["selected"]
	for _, snapshot := range []string{tail.text + "：检查通过。", "后台检查结果已修正。", "后台检查结果已修正。追加完整说明。"} {
		tail.text = snapshot
		tail.revision++
		a.syncMessages()
		frame()
		frame()
		selectionKey(router, frame, "C")
		_, copied, ok := router.WriteClipboard()
		if !ok || string(copied) != want || a.docs["selected"] != selected || a.docs[tail.key].Source() != snapshot {
			t.Fatalf("unrelated snapshot %q cleared/truncated selected reply: %q", snapshot, copied)
		}
	}
}

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
