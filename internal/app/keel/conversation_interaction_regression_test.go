package keeldesktop

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"strings"
	"testing"
	"time"

	"github.com/dyike/keel/gio/f32"
	"github.com/dyike/keel/gio/gpu/headless"
	"github.com/dyike/keel/gio/io/input"
	"github.com/dyike/keel/gio/io/pointer"
	"github.com/dyike/keel/gio/layout"
	"github.com/dyike/keel/gio/op"
	"github.com/dyike/keel/gio/unit"
	"github.com/dyike/keel/ui/el"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

func interactionPixels(t *testing.T, root *el.RootWidget, router *input.Router, width, height int) *image.RGBA {
	return interactionPixelsScaled(t, root, router, width, height, 1)
}

func interactionPixelsScaled(t *testing.T, root *el.RootWidget, router *input.Router, width, height int, scale float32) *image.RGBA {
	t.Helper()
	gpu, err := headless.NewWindow(width, height)
	if err != nil {
		t.Fatal(err)
	}
	defer gpu.Release()
	var ops op.Ops
	root.Layout(layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Constraints: layout.Exact(image.Pt(width, height)), Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}})
	router.Frame(&ops)
	if err := gpu.Frame(&ops); err != nil {
		t.Fatal(err)
	}
	pixels := image.NewRGBA(image.Rect(0, 0, width, height))
	if err := gpu.Screenshot(pixels); err != nil {
		t.Fatal(err)
	}
	return pixels
}

func TestConversationTextSurvivesStatusMenu(t *testing.T) {
	defer applyPalette(false)
	applyPalette(true)
	a := sessionPanelFixture(t)
	a.workbench.Visible = false
	a.transcript = newTranscript()
	a.transcript.items["answer"] = &entry{key: "answer", kind: agentrun.KindMessage, text: "## 最终建议\n\n普通正文保持可见。\n\n这里有 **加粗文字** 和后续说明。"}
	a.syncMessages()
	root := el.Root(a)
	router, frame, click := alignmentRouter(root, 1280, 800, 1)
	for range 5 {
		frame()
	}
	// Check actual ink, since correct dimensions alone still allow blank text.
	assertInk := func(pixels *image.RGBA) {
		t.Helper()
		for _, label := range []string{"普通正文保持可见。", "这里有 加粗文字 和后续说明。"} {
			node, ok := alignmentNode(router.AppendSemantics(nil), label, false)
			if !ok {
				t.Fatalf("missing paragraph %q", label)
			}
			n := 0
			for y := node.Bounds.Min.Y; y < node.Bounds.Max.Y; y++ {
				for x := node.Bounds.Min.X; x < node.Bounds.Max.X; x++ {
					c := color.NRGBAModel.Convert(pixels.At(x, y)).(color.NRGBA)
					if c.R > 150 && c.G > 150 && c.B > 150 {
						n++
					}
				}
			}
			if n < 20 {
				t.Fatalf("paragraph %q has no visible ink", label)
			}
		}
	}
	assertInk(interactionPixels(t, root, router, 1280, 800))
	more, ok := alignmentNode(router.AppendSemantics(nil), "更多操作", true)
	if !ok {
		t.Fatal("missing status menu trigger")
	}
	click(more.Bounds)
	for range 3 {
		frame()
	}
	status, ok := alignmentNode(router.AppendSemantics(nil), "Session 状态", true)
	if !ok {
		t.Fatal("status menu did not open")
	}
	assertInk(interactionPixels(t, root, router, 1280, 800))
	click(status.Bounds)
	for range 5 {
		frame()
	}
	assertInk(interactionPixels(t, root, router, 1280, 800))
}

func TestComposerLongDocumentScrollsWithWheel(t *testing.T) {
	a := conversationFixture(t)
	a.detail = nil
	for i := 0; i < 40; i++ {
		a.prompt += fmt.Sprintf("第 %02d 行：长指令里的不同内容。\n", i)
	}
	original := a.prompt
	root := el.Root(a)
	router, frame, _ := alignmentRouter(root, 1280, 800, 1)
	for range 4 {
		frame()
	}
	before := interactionPixels(t, root, router, 1280, 800)
	editor, ok := alignmentNode(router.AppendSemantics(nil), "任务指令", false)
	if !ok {
		t.Fatal("missing composer")
	}
	if editor.Bounds.Dy() > 260 {
		t.Fatalf("long composer exceeded its height limit: %v", editor.Bounds)
	}
	crop := editor.Bounds.Inset(12)
	rows := func(pixels *image.RGBA) []byte {
		var data []byte
		for y := crop.Min.Y; y < crop.Max.Y; y++ {
			start := pixels.PixOffset(crop.Min.X, y)
			data = append(data, pixels.Pix[start:start+crop.Dx()*4]...)
		}
		return data
	}
	p := f32.Pt(float32(crop.Min.X+30), float32(crop.Min.Y+30))
	router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: p}, pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: p, Scroll: f32.Pt(0, 100)})
	for range 5 {
		frame()
	}
	after := interactionPixels(t, root, router, 1280, 800)
	if bytes.Equal(rows(before), rows(after)) {
		t.Fatal("wheel failed to move long document content")
	}
	for range 5 {
		frame()
	}
	after = interactionPixels(t, root, router, 1280, 800)
	if bytes.Equal(rows(before), rows(after)) {
		t.Fatal("editor snapped back to caret after wheel scrolling")
	}
	if a.prompt != original || a.promptDocument.Content().Text() != original {
		t.Fatal("scrolling changed draft")
	}
}

func TestProcessOutputTabsRenderAsIndentation(t *testing.T) {
	a := conversationFixture(t)
	e := &entry{key: "tabbed-output", kind: agentrun.KindToolResult, text: "{\n\t\"Query\": \"b2696a60\",\n\t\"Origin\": {\n\t\t\"Hash\": \"abc\"\n\t}\n}"}
	original := e.text
	a.toggleConversation(e.key, false)
	root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element {
		return el.Div().WFull().HFull().P(20).Child(a.processItem(cx, e, false, false))
	}))
	router, frame, _ := alignmentRouter(root, 800, 400, 1)
	frame()
	tabbed := interactionPixels(t, root, router, 800, 400)
	if e.text != original {
		t.Fatal("display mutated original tool output")
	}
	e.text = strings.ReplaceAll(original, "\t", "    ")
	frame()
	spaced := interactionPixels(t, root, router, 800, 400)
	if !bytes.Equal(tabbed.Pix, spaced.Pix) {
		t.Fatal("tab indentation paints differently from spaces (missing glyph squares)")
	}
}
