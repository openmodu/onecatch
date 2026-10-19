package keeldesktop

import (
	"fmt"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/dyike/keel/gio/io/input"
	"github.com/dyike/keel/gio/op"
	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/locale"
	"github.com/dyike/keel/ui/theme"
	"github.com/dyike/keel/ui/window"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

// Check actual painted content, including the focused editor and wrapped table
// rows: layout-only assertions cannot catch a transparent divider/focus fill.
func TestConversationTableDividersAndComposerSurface(t *testing.T) {
	defer applyPalette(false)
	const source = "| hash | 提交 |\n|---|---|\n| fc86753 | feat(el): add italic text style |\n| aceeacd | feat(kit): reveal sidebar suffix on row hover and keep a longer description wrapping on narrow screens |\n| 353abbb | feat(markdown): keep chunked parsing with local plugins |"
	for _, dark := range []bool{false, true} {
		for _, scale := range []int{1, 2} {
			t.Run(fmt.Sprintf("dark=%v/scale=%d", dark, scale), func(t *testing.T) {
				applyPalette(dark)
				a := conversationFixture(t)
				// Compare unfocused and focused surfaces independently of startup focus.
				a.composerFocusPending = false
				a.dark, a.prompt = dark, "继续提交代码"
				e := &entry{key: "table-style", text: source}
				focus, requested, focused := false, false, false
				root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element {
					view := el.Div().WFull().HFull().P(20).Gap(20).Bg(theme.Bg).
						Child(a.conversationDocument(cx, e, false), a.composer(cx))
					if focus && !requested {
						cx.Focus("prompt")
						requested = true
					}
					focused = cx.Focused("prompt")
					return view
				}))
				var router input.Router
				widget := core.Func(func(gtx core.C) core.D {
					gtx.Source = router.Source()
					var ops op.Ops
					pre := gtx
					pre.Ops = &ops
					for range 4 {
						ops.Reset()
						root.Layout(pre)
						router.Frame(&ops)
					}
					dims := root.Layout(gtx)
					router.Frame(gtx.Ops)
					return dims
				})
				for _, state := range []string{"idle", "focused", "dividers-off", "dividers-on"} {
					focus = state != "idle"
					if state == "dividers-off" {
						a.setTableDividers(false)
					}
					if state == "dividers-on" {
						a.setTableDividers(true)
					}
					dir := t.TempDir()
					if preview := os.Getenv("ONECATCH_KEEL_PREVIEWS"); preview != "" {
						dir = preview
						if err := os.MkdirAll(dir, 0755); err != nil {
							t.Fatal(err)
						}
					}
					path := filepath.Join(dir, fmt.Sprintf("conversation-surfaces-dark-%v-%dx-%s.png", dark, scale, state))
					if err := window.ScreenshotAtScale(screenshotRoot{widget}, 620, 650, float32(scale), path); err != nil {
						t.Fatal(err)
					}
					if focused != focus {
						t.Fatalf("focus=%v, want %v", focused, focus)
					}
					file, err := os.Open(path)
					if err != nil {
						t.Fatal(err)
					}
					pixels, err := png.Decode(file)
					file.Close()
					if err != nil {
						t.Fatal(err)
					}
					at := func(x, y int) color.NRGBA { return color.NRGBAModel.Convert(pixels.At(x, y)).(color.NRGBA) }
					nodes := router.AppendSemantics(nil)
					header, ok := alignmentNode(nodes, "hash | 提交", false)
					if !ok {
						t.Fatal("missing table header")
					}
					last, ok := alignmentNode(nodes, "353abbb | feat(markdown): keep chunked parsing with local plugins", false)
					if !ok {
						t.Fatal("missing last table row")
					}
					// A single internal divider must cross the whole header and all
					// body rows, within the horizontal viewport of a wide table.
					dividerX := -1
					for x := header.Bounds.Min.X + 2*scale; x < header.Bounds.Max.X-2*scale; x++ {
						if at(x, header.Bounds.Min.Y+scale) == seamColor {
							dividerX = x
							break
						}
					}
					if state == "dividers-off" {
						if dividerX >= 0 {
							t.Fatal("disabled column divider is still visible")
						}
						for x := header.Bounds.Min.X; x < min(header.Bounds.Max.X, 600*scale); x++ {
							if at(x, header.Bounds.Max.Y) == seamColor {
								t.Fatal("disabled row divider is still visible")
							}
						}
					} else {
						if dividerX < 0 {
							t.Fatal("missing vertical divider")
						}
						for y := header.Bounds.Min.Y; y < last.Bounds.Max.Y; y++ {
							if at(dividerX, y) != seamColor {
								t.Fatalf("vertical divider interrupted at %d,%d", dividerX, y)
							}
						}
						for _, label := range []string{
							"fc86753 | feat(el): add italic text style",
							"aceeacd | feat(kit): reveal sidebar suffix on row hover and keep a longer description wrapping on narrow screens",
							"353abbb | feat(markdown): keep chunked parsing with local plugins",
						} {
							row, ok := alignmentNode(nodes, label, false)
							if !ok {
								t.Fatalf("missing row %q", label)
							}
							for x := row.Bounds.Min.X; x < min(row.Bounds.Max.X, 600*scale); x++ {
								if at(x, row.Bounds.Min.Y-1) != seamColor {
									t.Fatalf("horizontal divider missing at %d,%d", x, row.Bounds.Min.Y-1)
								}
							}
							if at(row.Bounds.Min.X, row.Bounds.Min.Y+3*scale) != theme.Bg {
								t.Fatal("table outer frame reappeared")
							}
						}
					}
					editor, ok := alignmentNode(nodes, "任务指令", false)
					if !ok {
						t.Fatal("missing composer editor")
					}
					surface := theme.Subtle
					y := editor.Bounds.Min.Y + editor.Bounds.Dy()/2
					for _, x := range []int{editor.Bounds.Min.X - 4*scale, editor.Bounds.Max.X - 16*scale} {
						if got := at(x, y); got != surface {
							t.Fatalf("%s composer at %d,%d = %v, want %v", state, x, y, got, surface)
						}
					}
					if a.prompt != "继续提交代码" {
						t.Fatal("focusing composer changed its draft")
					}
					if theme.Border.A != 0 || !theme.Current().Frameless {
						t.Fatal("local palette leaked into other controls")
					}
				}
			})
		}
	}
}

func TestRenderConversationTableSurface(t *testing.T) {
	dir := os.Getenv("ONECATCH_KEEL_PREVIEWS")
	if dir == "" {
		t.Skip("set ONECATCH_KEEL_PREVIEWS to render the conversation")
	}
	locale.Apply(locale.Chinese())
	defer applyPalette(false)
	const source = "已完成，提交创建成功，工作区干净，编译和测试通过。\n\n**提交结果**\n\n| hash | 提交 |\n|---|---|\n| `fc86753` | feat(el): add italic text style |\n| `aceeacd` | feat(kit): reveal sidebar suffix on row hover |\n| `353abbb` | feat(markdown): keep chunked parsing with local plugins |\n| `ddd7da1` | fix(el): update auto height after cross-stretch |\n| `691efad` | fix(theme): draw macOS color emoji as images |\n| `2e33a12` | fix(el): let text areas capture plain keys |\n| `f1e3f75` | feat(theme): add opt-in frameless palette |\n| `7dd92d6` | fix(kit): keep a following list pinned to its end |\n\n**验证情况**\n\n- 编译和测试通过。\n- 表格保留内部横竖分隔线，输入区底色统一。"
	for _, dark := range []bool{false, true} {
		applyPalette(dark)
		a := conversationFixture(t)
		a.dark, a.prompt = dark, "继续提交代码"
		a.transcript = newTranscript()
		a.transcript.items["table-preview"] = &entry{key: "table-preview", kind: agentrun.KindMessage, text: source}
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
		if err := screenshotWithFocus(root, 1288, 900, filepath.Join(dir, fmt.Sprintf("conversation-table-dark-%v.png", dark))); err != nil {
			t.Fatal(err)
		}
	}
}
