package keeldesktop

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/gpu/headless"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/markdown"
	"github.com/dyike/keel/ui/theme"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

func selectionKey(router *input.Router, frame func(), name key.Name) {
	router.Queue(key.Event{Name: name, Modifiers: key.ModShortcut, State: key.Press}, key.Event{Name: name, Modifiers: key.ModShortcut, State: key.Release})
	frame()
	frame()
}

func TestConversationSelectionIncludesHeadingAndQuote(t *testing.T) {
	a := conversationFixture(t)
	e := &entry{key: "select-blocks", text: "## 标题文字\n\n开头正文。\n\n> 引用段落。\n\n末尾正文。"}
	root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element {
		return el.Div().WFull().HFull().P(20).Child(a.conversationDocument(cx, e, false))
	}))
	router, frame, click := alignmentRouter(root, 700, 500, 1)
	para, ok := alignmentNode(router.AppendSemantics(nil), "开头正文。", false)
	if !ok {
		t.Fatal("missing paragraph")
	}
	click(para.Bounds)
	selectionKey(router, frame, "A")
	selectionKey(router, frame, "C")
	_, data, ok := router.WriteClipboard()
	if !ok || string(data) != "标题文字\n\n开头正文。\n\n引用段落。\n\n末尾正文。" {
		t.Fatalf("document selection omitted blocks: %q", data)
	}
}

func TestConversationSelectionAutoScrollsEmbeddedDocument(t *testing.T) {
	a := conversationFixture(t)
	e := &entry{key: "select-scroll", text: "开头文字\n\n" + strings.Repeat("中间段落。\n\n", 30) + "末尾文字"}
	var offset float32
	root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element {
		return el.Div().WFull().HFull().P(20).Child(el.Div().ID("selection-scroll").H(el.Dp(180)).ScrollY().Child(a.conversationDocument(cx, e, false)).Decorate(func(gtx layout.Context, draw func()) { draw(); offset, _, _ = cx.ScrollState("selection-scroll") }))
	}))
	var router input.Router
	var ops op.Ops
	now := time.Now()
	frame := func() {
		now = now.Add(time.Second / 60)
		ops.Reset()
		root.Layout(layout.Context{Ops: &ops, Source: router.Source(), Now: now, Constraints: layout.Exact(image.Pt(700, 500)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}})
		router.Frame(&ops)
	}
	for range 5 {
		frame()
	}
	para, ok := alignmentNode(router.AppendSemantics(nil), "开头文字", false)
	if !ok {
		t.Fatal("missing opening paragraph")
	}
	p := f32.Pt(float32(para.Bounds.Min.X+2), float32(para.Bounds.Min.Y+para.Bounds.Dy()/2))
	router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: p})
	frame()
	end := f32.Pt(p.X, 199)
	router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: end})
	for range 120 {
		frame()
	}
	if offset < 300 {
		t.Fatalf("embedded selection failed to scroll outer conversation: %v", offset)
	}
	router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: end})
	frame()
	selectionKey(&router, frame, "C")
	_, data, ok := router.WriteClipboard()
	if !ok || strings.Count(string(data), "中间段落") < 5 {
		t.Fatalf("selection did not extend with scrolling: %q", data)
	}
}

func TestConversationSelectionBackgroundContainsTextInk(t *testing.T) {
	defer applyPalette(false)
	applyPalette(true)
	for _, size := range []string{"small", "medium", "large", "extra-large"} {
		for _, scale := range []int{1, 2} {
			t.Run(fmt.Sprintf("size=%v/scale=%d", size, scale), func(t *testing.T) {
				a := conversationFixture(t)
				a.ui.Preferences.ChatFontSize = size
				e := &entry{key: "selection-ink", text: "第一行中文 mixed words，继续排版。第二行的文本较长，需要换行才能显示完整。最后一行用来核对选区的位置。"}
				root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element {
					return el.Div().WFull().HFull().P(20).Child(a.conversationDocument(cx, e, false))
				}))
				router, frame, click := alignmentRouter(root, 420, 300, scale)
				gpu, err := headless.NewWindow(420*scale, 300*scale)
				if err != nil {
					t.Fatal(err)
				}
				defer gpu.Release()
				snap := func() *image.RGBA {
					t.Helper()
					var ops op.Ops
					root.Layout(layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Constraints: layout.Exact(image.Pt(420*scale, 300*scale)), Metric: unit.Metric{PxPerDp: float32(scale), PxPerSp: float32(scale)}})
					router.Frame(&ops)
					if err := gpu.Frame(&ops); err != nil {
						t.Fatal(err)
					}
					pixels := image.NewRGBA(image.Rect(0, 0, 420*scale, 300*scale))
					if err := gpu.Screenshot(pixels); err != nil {
						t.Fatal(err)
					}
					return pixels
				}
				before := snap()
				node, ok := alignmentNode(router.AppendSemantics(nil), e.text, false)
				if !ok {
					t.Fatal("missing text")
				}
				click(node.Bounds)
				selectionKey(router, frame, "A")
				after := snap()
				ink, background := image.Rectangle{}, image.Rectangle{}
				for y := node.Bounds.Min.Y; y < node.Bounds.Max.Y; y++ {
					for x := node.Bounds.Min.X; x < node.Bounds.Max.X; x++ {
						c := color.NRGBAModel.Convert(before.At(x, y)).(color.NRGBA)
						if c.R > 200 && c.G > 200 && c.B > 200 {
							ink = ink.Union(image.Rect(x, y, x+1, y+1))
						}
						if color.NRGBAModel.Convert(after.At(x, y)).(color.NRGBA) == markdown.SelectionBg {
							background = background.Union(image.Rect(x, y, x+1, y+1))
						}
					}
				}
				if ink.Empty() || background.Empty() || ink.Min.Y < background.Min.Y || ink.Max.Y > background.Max.Y {
					t.Fatalf("selection background is vertically detached: ink=%v background=%v", ink, background)
				}
			})
		}
	}
}

func TestToolDetailsSelectAndCopyIndependently(t *testing.T) {
	a := conversationFixture(t)
	e := &entry{key: "copy-tool", kind: agentrun.KindToolUse, text: "printf hello", details: []*entry{{text: "first result"}, {text: "second result"}}}
	a.toggleConversation(e.key, false)
	root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element {
		return el.Div().WFull().HFull().P(20).Child(a.processItem(cx, e, false, false))
	}))
	router, frame, click := alignmentRouter(root, 700, 500, 1)
	for i, detail := range e.details {
		node, found := alignmentNode(router.AppendSemantics(nil), detail.text, false)
		if !found {
			t.Fatalf("missing selectable result %d", i)
		}
		click(node.Bounds)
		selectionKey(router, frame, "A")
		selectionKey(router, frame, "C")
		_, data, ok := router.WriteClipboard()
		if !ok || string(data) != e.details[i].text {
			t.Fatalf("result %d copied %q", i, data)
		}
	}
	if e.details[0].text != "first result" || e.details[1].text != "second result" {
		t.Fatal("read-only selection changed output")
	}
}

func TestStreamingAppendKeepsSelectedPrefix(t *testing.T) {
	a := conversationFixture(t)
	a.detail.Active = true
	e := &entry{key: "select-stream", text: "已生成的正文", streaming: true}
	root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element {
		return el.Div().WFull().HFull().P(20).Child(a.conversationDocument(cx, e, false))
	}))
	router, frame, click := alignmentRouter(root, 700, 400, 1)
	node, ok := alignmentNode(router.AppendSemantics(nil), e.text, false)
	if !ok {
		t.Fatal("missing streaming text")
	}
	click(node.Bounds)
	selectionKey(router, frame, "A")
	e.text += "，继续生成的新文字。"
	frame()
	frame()
	selectionKey(router, frame, "C")
	_, data, ok := router.WriteClipboard()
	if !ok || string(data) != "已生成的正文" {
		t.Fatalf("streaming update cleared selection: %q", data)
	}
}

func TestFontChangeReflowsCachedConversationText(t *testing.T) {
	a := conversationFixture(t)
	a.ui.Preferences.ChatFontSize = "small"
	e := &entry{key: "selection-font-reflow", text: strings.Repeat("中文正文 mixed words，用于验证字号变化后的换行和选区。", 5)}
	root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element {
		return el.Div().WFull().HFull().P(20).Child(a.conversationDocument(cx, e, false))
	}))
	router, frame, _ := alignmentRouter(root, 420, 900, 1)
	before, ok := alignmentNode(router.AppendSemantics(nil), e.text, false)
	if !ok {
		t.Fatal("missing initial paragraph")
	}
	a.ui.Preferences.ChatFontSize = "extra-large"
	for range 4 {
		frame()
	}
	after, ok := alignmentNode(router.AppendSemantics(nil), e.text, false)
	if !ok || after.Bounds.Dy() <= before.Bounds.Dy() {
		t.Fatalf("larger font retained cached geometry: before=%v after=%v", before.Bounds, after.Bounds)
	}
}

func TestConversationSelectionAcrossVirtualMessages(t *testing.T) {
	for _, scale := range []int{1, 2} {
		t.Run(fmt.Sprint(scale), func(t *testing.T) {
			a := conversationFixture(t)
			clear(a.transcript.items)
			for i := 0; i < 24; i++ {
				key := fmt.Sprintf("message-%02d", i)
				a.transcript.items[key] = &entry{key: key, seq: int64(i * 2), step: "one", kind: agentrun.KindMessage, text: fmt.Sprintf("第%02d块正文：可跨消息选择复制。", i), at: time.Unix(int64(i*2), 0)}
				tool := fmt.Sprintf("tool-%02d", i)
				a.transcript.items[tool] = &entry{key: tool, seq: int64(i*2 + 1), step: "one", kind: agentrun.KindToolUse, text: "Read", at: time.Unix(int64(i*2+1), 0)}
			}
			a.syncMessages()
			a.scroller.SetFollow(false)
			root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element { return el.Div().WFull().HFull().Child(a.conversationScroll(cx)) }))
			var router input.Router
			var ops op.Ops
			now := time.Now()
			frame := func() {
				now = now.Add(time.Second / 60)
				ops.Reset()
				root.Layout(layout.Context{Ops: &ops, Source: router.Source(), Now: now, Constraints: layout.Exact(image.Pt(700*scale, 300*scale)), Metric: unit.Metric{PxPerDp: float32(scale), PxPerSp: float32(scale)}})
				router.Frame(&ops)
			}
			for range 5 {
				frame()
			}
			para, ok := alignmentNode(router.AppendSemantics(nil), "第00块正文：可跨消息选择复制。", false)
			if !ok {
				t.Fatal("missing first message")
			}
			start := f32.Pt(float32(para.Bounds.Min.X+1), float32(para.Bounds.Min.Y+para.Bounds.Dy()/2))
			router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: start})
			frame()
			end := f32.Pt(start.X, float32(297*scale))
			router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: end})
			for range 360 {
				frame()
			}
			router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: end})
			frame()
			frame()
			selectionKey(&router, frame, "C")
			_, data, ok := router.WriteClipboard()
			if !ok || !strings.HasPrefix(string(data), "第00块正文") || strings.Count(string(data), "可跨消息选择复制") < 10 {
				t.Fatalf("cross-message copy truncated: %q", data)
			}
			// The origin row must have been unmounted; its selection is still in the clipboard.
			if _, ok := alignmentNode(router.AppendSemantics(nil), "第00块正文：可跨消息选择复制。", false); ok {
				t.Fatal("test did not scroll beyond first row")
			}
			selectionKey(&router, frame, "A")
			selectionKey(&router, frame, "C")
			_, data, ok = router.WriteClipboard()
			if !ok || strings.Count(string(data), "可跨消息选择复制") != 24 {
				t.Fatalf("select all omitted virtual rows: %q", data)
			}
			// Reverse direction from a visible later message. The origin row
			// unmounts while the pointer stays at the top edge.
			var anchor image.Rectangle
			for _, e := range a.entries {
				if e.kind != agentrun.KindMessage {
					continue
				}
				node, found := alignmentNode(router.AppendSemantics(nil), e.text, false)
				if found && node.Bounds.Min.Y >= 0 && node.Bounds.Max.Y < 300*scale {
					anchor = node.Bounds
				}
			}
			if anchor.Empty() {
				t.Fatal("no visible reverse-drag anchor")
			}
			start = f32.Pt(float32(anchor.Min.X+1), float32(anchor.Min.Y+anchor.Dy()/2))
			router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: start}, pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: start})
			frame()

			end = f32.Pt(start.X, float32(2*scale))
			router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: end})
			for range 240 {
				frame()
			}
			router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: end})
			frame()
			frame()
			selectionKey(&router, frame, "C")
			_, data, ok = router.WriteClipboard()
			if !ok || strings.Count(string(data), "可跨消息选择复制") < 8 {
				t.Fatalf("upward virtual drag stalled or lost selection: %q", data)
			}
		})
	}
}

func TestConversationWorkflowHeaderAlignsWithBody(t *testing.T) {
	a := conversationFixture(t)
	e := &entry{key: "workflow-alignment", kind: agentrun.KindMessage, text: "这里是流程正文。", stepLabel: "代码审查 · trae · GPT-5.6-Sol", stepContext: "工作流：Keel开发工作流 · 第 2 次执行 · 已完成"}
	a.entries = []*entry{e}
	root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element { return a.message(cx, 0) }))
	for _, scale := range []int{1, 2} {
		router, _, _ := alignmentRouter(root, 800, 400, scale)
		header, ok := alignmentNode(router.AppendSemantics(nil), e.stepLabel, false)
		if !ok {
			t.Fatal("missing workflow header")
		}
		body, ok := alignmentNode(router.AppendSemantics(nil), e.text, false)
		if !ok {
			t.Fatal("missing message")
		}
		context, ok := alignmentNode(router.AppendSemantics(nil), e.stepContext, false)
		if !ok {
			t.Fatal("missing workflow name")
		}
		if header.Bounds.Min.X != body.Bounds.Min.X || context.Bounds.Min.X != body.Bounds.Min.X {
			t.Fatalf("scale %d: header=%v context=%v body=%v", scale, header.Bounds, context.Bounds, body.Bounds)
		}
	}
}

func TestCrossMessageSelectionPaintsEveryBlock(t *testing.T) {
	defer applyPalette(false)
	applyPalette(true)
	a := conversationFixture(t)
	clear(a.transcript.items)
	for i, text := range []string{"第一块正文：开始检查实现和测试。", "第二块正文：检查工具执行结果，继续分析问题。", "最后一块正文：三个消息块应当一起选中复制。"} {
		key := fmt.Sprintf("select-%d", i)
		a.transcript.items[key] = &entry{key: key, kind: agentrun.KindMessage, text: text, at: time.Unix(int64(i*2), 0)}
		if i < 2 {
			tool := fmt.Sprintf("tool-%d", i)
			a.transcript.items[tool] = &entry{key: tool, kind: agentrun.KindToolUse, text: "Read", at: time.Unix(int64(i*2+1), 0)}
		}
	}
	a.syncMessages()
	a.entries[0].stepLabel = "CodeReview · trae · GPT-5.6-Sol"
	a.entries[0].stepContext = "工作流：Keel开发工作流 · 第 1 次执行 · 已完成"
	a.scroller.SetFollow(false)
	root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element {
		return el.Div().WFull().HFull().Bg(theme.Bg).Child(a.conversationScroll(cx))
	}))
	router, frame, click := alignmentRouter(root, 760, 600, 1)
	gpu, err := headless.NewWindow(760, 600)
	if err != nil {
		t.Fatal(err)
	}
	defer gpu.Release()
	snap := func() *image.RGBA {
		var ops op.Ops
		root.Layout(layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Constraints: layout.Exact(image.Pt(760, 600)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}})
		router.Frame(&ops)
		if err := gpu.Frame(&ops); err != nil {
			t.Fatal(err)
		}
		pixels := image.NewRGBA(image.Rect(0, 0, 760, 600))
		if err := gpu.Screenshot(pixels); err != nil {
			t.Fatal(err)
		}
		return pixels
	}
	first, ok := alignmentNode(router.AppendSemantics(nil), a.entries[0].text, false)
	if !ok {
		t.Fatal("missing first text")
	}
	click(first.Bounds)
	selectionKey(router, frame, "A")
	frame()
	pixels := snap()
	for _, e := range a.entries {
		if e.kind != agentrun.KindMessage {
			continue
		}
		node, ok := alignmentNode(router.AppendSemantics(nil), e.text, false)
		if !ok {
			t.Fatalf("missing %s", e.key)
		}
		count := 0
		for y := node.Bounds.Min.Y; y < node.Bounds.Max.Y; y++ {
			for x := node.Bounds.Min.X; x < node.Bounds.Max.X; x++ {
				if color.NRGBAModel.Convert(pixels.At(x, y)).(color.NRGBA) == markdown.SelectionBg {
					count++
				}
			}
		}
		if count < 100 {
			t.Fatalf("message %s has no selection background", e.key)
		}
	}
	if dir := os.Getenv("ONECATCH_KEEL_PREVIEWS"); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		file, err := os.Create(filepath.Join(dir, "cross-message-selection.png"))
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		if err := png.Encode(file, pixels); err != nil {
			t.Fatal(err)
		}
	}
}
