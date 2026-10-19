package keeldesktop

import (
	"fmt"
	"image"
	"testing"

	"github.com/dyike/keel/gio/io/key"
	"github.com/dyike/keel/ui/el"
)

func TestComposerInitiallyAcceptsTyping(t *testing.T) {
	a := dagTestApp(t)
	other := ""
	root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element {
		return el.Div().WFull().HFull().P(20).Gap(12).Child(a.composer(cx), el.Input().ID("other").Name("其他输入").Bind(&other))
	}))
	router, frame, click := alignmentRouter(root, 800, 400, 1)
	router.Queue(key.EditEvent{Range: key.Range{}, Text: "开始输入"})
	frame()
	if a.prompt != "开始输入" {
		t.Fatalf("initial composer did not receive typing: %q", a.prompt)
	}
	field, ok := alignmentNode(router.AppendSemantics(nil), "其他输入", false)
	if !ok {
		t.Fatal("missing alternate input")
	}
	click(field.Bounds)
	for range 5 {
		frame()
	}
	router.Queue(key.EditEvent{Range: key.Range{}, Text: "其他字段"})
	frame()
	if other != "其他字段" || a.prompt != "开始输入" {
		t.Fatalf("redraw stole focus: prompt=%q other=%q", a.prompt, other)
	}
	a.newTask("")
	frame()
	router.Queue(key.EditEvent{Range: key.Range{}, Text: "新的对话"})
	frame()
	if a.prompt != "新的对话" || other != "其他字段" {
		t.Fatalf("new task did not restore composer focus: prompt=%q other=%q", a.prompt, other)
	}
}

func TestEmptyComposerFirstClickPaintsCaret(t *testing.T) {
	for _, dark := range []bool{false, true} {
		for _, scale := range []int{1, 2} {
			t.Run(fmt.Sprintf("dark=%v/scale=%d", dark, scale), func(t *testing.T) {
				applyPalette(dark)
				defer applyPalette(false)
				a := dagTestApp(t)
				other := ""
				focusOther := true
				var focused bool
				root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element {
					if focusOther {
						cx.Focus("other")
						focusOther = false
					}
					box := el.Div().WFull().HFull().P(20).Gap(12).Child(a.composer(cx), el.Input().ID("other").Name("其他输入").Bind(&other))
					focused = cx.FocusWithin("prompt")
					return box
				}))
				router, frame, click := alignmentRouter(root, 800, 400, scale)
				// Explicitly focus the other field even when the composer auto-focuses.
				focusOther = true
				frame()
				frame()
				before := interactionPixelsScaled(t, root, router, 800*scale, 400*scale, float32(scale))
				field, ok := alignmentNode(router.AppendSemantics(nil), "任务指令", false)
				if !ok {
					t.Fatal("missing composer input")
				}
				click(field.Bounds)
				if !focused {
					t.Fatal("first click did not focus composer")
				}
				after := interactionPixelsScaled(t, root, router, 800*scale, 400*scale, float32(scale))
				// The placeholder stays unchanged. A focused empty editor must add
				// visible caret ink at its left edge, inside the field's clip.
				area := field.Bounds.Intersect(image.Rect(field.Bounds.Min.X, field.Bounds.Min.Y, field.Bounds.Min.X+10*scale, field.Bounds.Max.Y))
				changed := 0
				for y := area.Min.Y; y < area.Max.Y; y++ {
					for x := area.Min.X; x < area.Max.X; x++ {
						if before.RGBAAt(x, y) != after.RGBAAt(x, y) {
							changed++
						}
					}
				}
				if changed < 10*scale*scale {
					t.Fatalf("focused empty composer has no visible caret: changed=%d field=%v", changed, field.Bounds)
				}
				router.Queue(key.EditEvent{Range: key.Range{}, Text: "你好"})
				frame()
				if a.prompt != "你好" || other != "" {
					t.Fatalf("first click routed typing incorrectly: prompt=%q other=%q", a.prompt, other)
				}
			})
		}
	}
}
