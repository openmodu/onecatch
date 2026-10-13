package keeldesktop

import (
	"bytes"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gioui.org/f32"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/locale"
	"github.com/dyike/keel/ui/theme"
	"github.com/dyike/keel/ui/window"
)

func longToolResult() string {
	var text strings.Builder
	text.WriteString("# 原始标题\n**原始 Markdown**\n```go\npackage main\n```\n")
	for i := range 80 {
		fmt.Fprintf(&text, "第 %02d 行 diff --git a/docs/decisions.md b/docs/decisions.md\n", i)
	}
	return text.String()
}

func TestToolResultScrollsAndCopiesLiteralOutput(t *testing.T) {
	for _, scale := range []int{1, 2} {
		t.Run(fmt.Sprintf("scale=%d", scale), func(t *testing.T) {
			a := conversationFixture(t)
			text := longToolResult()
			root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element {
				return el.Div().WFull().HFull().P(20).Bg(theme.Surface).Child(
					a.toolCodeSection(cx, toolCodeContent{"结果", "text", text}, "literal-result"),
					el.Text("工具结果后的正文"),
				)
			}))
			router, frame, click := alignmentRouter(root, 440, 500, scale)
			field, ok := alignmentNode(router.AppendSemantics(nil), text, false)
			if !ok || field.Bounds.Dy() > toolResultMaxHeight*scale {
				t.Fatalf("result viewport is missing or too tall: %v", field.Bounds)
			}
			footer, ok := alignmentNode(router.AppendSemantics(nil), "工具结果后的正文", false)
			if !ok || footer.Bounds.Max.Y > 400*scale {
				t.Fatalf("long result pushed following content out of view: %v", footer.Bounds)
			}
			before := interactionPixelsScaled(t, root, router, 440*scale, 500*scale, float32(scale))
			p := f32.Pt(float32(field.Bounds.Min.X+30*scale), float32(field.Bounds.Min.Y+30*scale))
			router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: p}, pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: p, Scroll: f32.Pt(0, float32(150*scale))})
			for range 5 {
				frame()
			}
			after := interactionPixelsScaled(t, root, router, 440*scale, 500*scale, float32(scale))
			crop := field.Bounds.Inset(16 * scale)
			pixels := func(img *image.RGBA) []byte {
				var rows []byte
				for y := crop.Min.Y; y < crop.Max.Y; y++ {
					start := img.PixOffset(crop.Min.X, y)
					rows = append(rows, img.Pix[start:start+crop.Dx()*4]...)
				}
				return rows
			}
			if bytes.Equal(pixels(before), pixels(after)) {
				t.Fatal("wheel did not scroll tool result")
			}
			copy, ok := alignmentNode(router.AppendSemantics(nil), locale.Current().Copy, true)
			if !ok {
				t.Fatal("copy button scrolled out of view")
			}
			click(copy.Bounds)
			_, data, copied := router.WriteClipboard()
			if !copied || string(data) != text {
				t.Fatalf("copy lost literal content: %q", data)
			}
			click(field.Bounds)
			selectionKey(router, frame, "A")
			selectionKey(router, frame, "C")
			_, data, copied = router.WriteClipboard()
			if !copied || string(data) != text {
				t.Fatal("native text selection lost Markdown markers or offscreen output")
			}
			router.Queue(key.EditEvent{Text: "changed"})
			frame()
			if a.conversation.toolResults["literal-result"].document.Content().Text() != text {
				t.Fatal("read-only output was edited")
			}
		})
	}
}

func TestToolResultStreamingPreservesSelectionAndCopy(t *testing.T) {
	a := conversationFixture(t)
	text := "已输出的正文"
	root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element {
		return el.Div().WFull().HFull().P(20).Child(a.toolCodeSection(cx, toolCodeContent{"结果", "text", text}, "stream-result"))
	}))
	router, frame, click := alignmentRouter(root, 440, 500, 1)
	field, ok := alignmentNode(router.AppendSemantics(nil), text, false)
	if !ok || field.Bounds.Dy() > 80 {
		t.Fatal("short result did not use its natural height")
	}
	click(field.Bounds)
	selectionKey(router, frame, "A")
	text += "，追加的新内容。"
	frame()
	frame()
	selectionKey(router, frame, "C")
	_, data, copied := router.WriteClipboard()
	if !copied || string(data) != "已输出的正文" {
		t.Fatalf("append reset selected prefix: %q", data)
	}
	copy, ok := alignmentNode(router.AppendSemantics(nil), locale.Current().Copy, true)
	if !ok {
		t.Fatal("missing full-result copy button")
	}
	click(copy.Bounds)
	_, data, copied = router.WriteClipboard()
	if !copied || string(data) != text {
		t.Fatalf("copy button retained stale output: %q", data)
	}
	text = "{\n\t\"ok\": true\n}\n"
	frame()
	frame()
	click(copy.Bounds)
	_, data, copied = router.WriteClipboard()
	if !copied || string(data) != text {
		t.Fatalf("full copy changed original tab indentation: %q", data)
	}
}

func TestRenderLongToolResult(t *testing.T) {
	dir := os.Getenv("ONECATCH_KEEL_PREVIEWS")
	if dir == "" {
		t.Skip("optional preview")
	}
	a := conversationFixture(t)
	root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element {
		return el.Div().WFull().HFull().P(24).Gap(16).Bg(theme.Surface).Child(
			a.toolCodeSection(cx, toolCodeContent{"命令", "bash", "git diff -- docs/decisions.md"}, "preview-command"),
			a.toolCodeSection(cx, toolCodeContent{"结果", "text", longToolResult()}, "preview-result"),
			el.Text("结果再长，也能继续查看后面的对话。"),
		)
	}))
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := window.Screenshot(screenshotRoot{core.Func(root.Layout)}, 800, 550, filepath.Join(dir, "tool-result-scroll.png")); err != nil {
		t.Fatal(err)
	}
}
