package keeldesktop

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dyike/keel/gio/f32"
	"github.com/dyike/keel/gio/io/input"
	"github.com/dyike/keel/gio/io/pointer"
	"github.com/dyike/keel/gio/layout"
	"github.com/dyike/keel/gio/op"
	"github.com/dyike/keel/gio/unit"
	"github.com/dyike/keel/ui/el"
	"time"
)

func TestLongUserMessageDisclosureReflowAndFullCopy(t *testing.T) {
	applyPalette(false)
	for _, scale := range []int{1, 2} {
		t.Run(fmt.Sprint(scale), func(t *testing.T) {
			a := conversationFixture(t)
			e := &entry{key: "long-user", user: true, text: strings.Repeat("正文中文 body words\n\n", 18) + "最后一段"}
			a.entries = []*entry{e}
			height := 900
			root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element { return el.Div().WFull().HFull().Child(a.message(cx, 0)) }))
			var router input.Router
			var ops op.Ops
			frame := func() {
				ops.Reset()
				root.Layout(layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: float32(scale), PxPerSp: float32(scale)}, Constraints: layout.Exact(image.Pt(800*scale, height*scale))})
				router.Frame(&ops)
			}
			click := func(r image.Rectangle) {
				p := f32.Pt(float32(r.Min.X+r.Dx()/2), float32(r.Min.Y+r.Dy()/2))
				router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: p}, pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: p})
				frame()
				router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: p})
				frame()
				frame()
			}
			frame()
			frame()
			frame()
			button, ok := findSkillControl(router.AppendSemantics(nil), "显示更多")
			if !ok {
				t.Fatal("long bubble has no disclosure")
			}
			if button.Min.Y < 440*scale || button.Min.Y > 500*scale {
				t.Fatal("Wails height budget changed", button)
			}
			doc := a.docs[e.key]
			if !doc.IsClamped() || !strings.HasSuffix(doc.RenderedText(), "最后一段") || doc.Source() != e.text {
				t.Fatal("preview lost message content")
			}
			click(button)
			button, ok = findSkillControl(router.AppendSemantics(nil), "收起")
			if !ok || doc.IsClamped() {
				t.Fatal("expand failed")
			}
			click(button)
			if !doc.IsClamped() {
				t.Fatal("collapse failed")
			}
			height = 500
			frame()
			frame()
			frame()
			button, ok = findSkillControl(router.AppendSemantics(nil), "显示更多")
			if !ok || button.Min.Y > 320*scale {
				t.Fatal("preview did not follow 52 percent of window", button)
			}
			// Copy from the collapsed bubble must use the original source.
			router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: f32.Pt(550*float32(scale), 40*float32(scale))})
			frame()
			frame()
			copy, ok := findSkillControl(router.AppendSemantics(nil), "复制消息")
			if !ok {
				t.Fatal("copy action missing")
			}
			click(copy)
			if _, data, ok := router.WriteClipboard(); !ok || string(data) != e.text {
				t.Fatal("copy truncated long source")
			}
			e.text = "short"
			frame()
			frame()
			frame()
			if _, ok := findSkillControl(router.AppendSemantics(nil), "显示更多"); ok || doc.IsClamped() {
				t.Fatal("short bubble retained collapse UI")
			}
		})
	}
}

func TestRenderLongInputAndUserMessage(t *testing.T) {
	dir := os.Getenv("ONECATCH_KEEL_PREVIEWS")
	if dir == "" {
		t.Skip("set ONECATCH_KEEL_PREVIEWS to render")
	}
	defer applyPalette(false)
	for _, dark := range []bool{false, true} {
		applyPalette(dark)
		a := conversationFixture(t)
		a.dark = dark
		source := "**提交结果**\n\n| hash | 提交 |\n|---|---|\n| `fc86753` | feat(el): add italic text style |\n| `aceeacd` | feat(kit): reveal sidebar suffix on row hover |\n\n" + strings.Repeat("正文说明：长消息可以展开查看，复制保留完整内容。\n\n", 18) + "这是最后一段。"
		a.prompt = source
		a.transcript = newTranscript()
		a.transcript.items["long-user"] = &entry{key: "long-user", user: true, text: source}
		a.syncMessages()
		requested := false
		root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element {
			view := a.Render(cx)
			if !requested {
				cx.Focus("prompt")
				requested = true
			}
			return view
		}))
		path := filepath.Join(dir, fmt.Sprintf("long-input-user-dark-%v.png", dark))
		if err := screenshotWithFocus(root, 1288, 1050, path); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLongUserMessageDisclosureInConversationScroller(t *testing.T) {
	applyPalette(false)
	a := conversationFixture(t)
	source := strings.Repeat("正文中文：对话列表需要根据展开和收起更新高度。\n\n", 24)
	e := &entry{key: "long-user-scroller", user: true, text: source}
	a.transcript = newTranscript()
	a.transcript.items[e.key] = e
	a.syncMessages()
	router, frame, click := alignmentRouter(el.Root(a), 1000, 800, 1)
	settle := func() {
		for range 6 {
			frame()
		}
	}
	settle()
	more, ok := findSkillControl(router.AppendSemantics(nil), "显示更多")
	if !ok {
		t.Fatal("conversation has no disclosure")
	}
	click(more)
	settle()
	less, ok := findSkillControl(router.AppendSemantics(nil), "收起")
	if !ok {
		t.Fatal("expanded list did not reveal the collapse action")
	}
	if a.docs[e.key].IsClamped() {
		t.Fatal("expanded list retained preview")
	}
	click(less)
	settle()
	more, ok = findSkillControl(router.AppendSemantics(nil), "显示更多")
	if !ok || !a.docs[e.key].IsClamped() {
		t.Fatal("collapsed list did not restore preview")
	}
	if more.Min.Y < 0 || more.Max.Y > 700 {
		t.Fatal("collapse left a stale scroll offset", more)
	}
}
