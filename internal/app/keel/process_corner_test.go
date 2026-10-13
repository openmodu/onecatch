package keeldesktop

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/dyike/keel/gio/f32"
	"github.com/dyike/keel/gio/io/input"
	"github.com/dyike/keel/gio/io/pointer"
	"github.com/dyike/keel/gio/op"
	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/theme"
	"github.com/dyike/keel/ui/window"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

func TestProcessCardClipsStatusAndHoverToRoundedCorners(t *testing.T) {
	defer applyPalette(false)
	for _, dark := range []bool{false, true} {
		for _, scale := range []int{1, 2} {
			for _, state := range []string{"running", "failed", "open", "hover"} {
				t.Run(fmt.Sprintf("dark=%v/scale=%d/%s", dark, scale, state), func(t *testing.T) {
					applyPalette(dark)
					a := conversationFixture(t)
					tool := &entry{key: "tool-corner", kind: agentrun.KindToolUse, text: "检查文件", failed: state == "failed"}
					group := &entry{key: "process/tool-corner", process: []*entry{tool}}
					a.entries = []*entry{group}
					a.detail.Active = state == "running"
					if state == "open" {
						a.toggleConversation(group.key, false)
						a.toggleConversation(tool.key, false)
					} else {
						a.conversationOpen(group.key, false)
						a.conversation.open[group.key] = true
						a.conversation.overridden[group.key] = true
					}
					root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element {
						return el.Div().WFull().HFull().P(20).Bg(theme.Bg).Child(a.processGroup(cx, group))
					}))
					var router input.Router
					var row image.Rectangle
					widget := core.Func(func(gtx core.C) core.D {
						gtx.Source = router.Source()
						var ops op.Ops
						pre := gtx
						pre.Ops = &ops
						for range 5 {
							ops.Reset()
							root.Layout(pre)
							router.Frame(&ops)
							if r, ok := findSkillControl(router.AppendSemantics(nil), "执行命令 · 检查文件"); ok {
								row = r
							}
							if state == "hover" && !row.Empty() {
								router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: f32.Pt(float32(row.Min.X+row.Dx()/2), float32(row.Min.Y+row.Dy()/2))})
							}
						}
						d := root.Layout(gtx)
						router.Frame(gtx.Ops)
						return d
					})
					path := filepath.Join(t.TempDir(), "tool.png")
					if dir := os.Getenv("ONECATCH_KEEL_PREVIEWS"); dir != "" {
						path = filepath.Join(dir, fmt.Sprintf("tool-corners-dark-%v-%dx-%s.png", dark, scale, state))
					}
					if err := window.ScreenshotAtScale(screenshotRoot{widget}, 620, 300, float32(scale), path); err != nil {
						t.Fatal(err)
					}
					if row.Empty() {
						t.Fatal("missing tool row")
					}
					f, err := os.Open(path)
					if err != nil {
						t.Fatal(err)
					}
					pixels, err := png.Decode(f)
					f.Close()
					if err != nil {
						t.Fatal(err)
					}
					at := func(p image.Point) color.NRGBA { return color.NRGBAModel.Convert(pixels.At(p.X, p.Y)).(color.NRGBA) }
					if got := at(row.Min); got != theme.Bg {
						t.Fatal("tool background escaped top left radius", got, theme.Bg, row)
					}
					if at(row.Min.Add(image.Pt(3*scale, 18*scale))) == theme.Bg {
						t.Fatal("corner clip erased the card interior")
					}
					if state != "open" && at(row.Max.Sub(image.Pt(1, 1))) != theme.Bg {
						t.Fatal("tool background escaped bottom right radius")
					}
				})
			}
		}
	}
}
