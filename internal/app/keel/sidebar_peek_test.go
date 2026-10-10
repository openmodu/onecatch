package keeldesktop

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/theme"
)

func TestCollapsedSidebarHoverPreviewAndNestedMenus(t *testing.T) {
	applyPalette(false)
	for _, scale := range []int{1, 2} {
		t.Run(fmt.Sprint(scale), func(t *testing.T) {
			a := conversationFixture(t)
			root := el.Root(a)
			var router input.Router
			var ops op.Ops
			now := time.Now()
			frame := func() {
				ops.Reset()
				root.Layout(layout.Context{Ops: &ops, Source: router.Source(), Now: now, Constraints: layout.Exact(image.Pt(1120*scale, 800*scale)), Metric: unit.Metric{PxPerDp: float32(scale), PxPerSp: float32(scale)}})
				router.Frame(&ops)
			}
			settle := func() {
				for range 3 {
					frame()
				}
			}
			move := func(x, y float32) {
				router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: f32.Pt(x, y)})
				settle()
			}
			click := func(box image.Rectangle) {
				p := f32.Pt(float32(box.Min.X+box.Dx()/2), float32(box.Min.Y+box.Dy()/2))
				router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: p}, pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: p})
				frame()
				router.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: p})
				settle()
			}
			find := func(label string) image.Rectangle {
				t.Helper()
				box, ok := findSkillControl(router.AppendSemantics(nil), label)
				if !ok {
					t.Fatalf("missing %q", label)
				}
				return box
			}
			settle()
			click(find("收起侧栏"))
			if !a.ui.Preferences.Collapsed || a.ui.SidebarPeek {
				t.Fatalf("collapse failed: collapsed=%v peek=%v blocked=%v", a.ui.Preferences.Collapsed, a.ui.SidebarPeek, a.ui.SidebarPeekBlocked)
			}
			toggle := find("展开侧栏")
			move(float32(700*scale), float32(300*scale))
			move(float32(toggle.Min.X+toggle.Dx()/2), float32(toggle.Min.Y+toggle.Dy()/2))
			if !a.ui.SidebarPeek || !a.ui.Preferences.Collapsed {
				t.Fatal("hover did not open a temporary preview")
			}
			panel, ok := alignmentNode(router.AppendSemantics(nil), "侧栏预览", false)
			if !ok || panel.Bounds.Min != image.Pt(8*scale, 8*scale) || panel.Bounds.Dy() != 560*scale {
				t.Fatal("preview misplaced", panel.Bounds)
			}
			project := find("modu")
			move(float32(project.Min.X+project.Dx()/2), float32(project.Min.Y+project.Dy()/2))
			now = now.Add(200 * time.Millisecond)
			settle()
			if !a.ui.SidebarPeek {
				t.Fatal("moving into preview closed it")
			}
			click(find("项目操作"))
			edit := find("编辑项目…")
			move(float32(edit.Min.X+edit.Dx()/2), float32(edit.Min.Y+edit.Dy()/2))
			now = now.Add(500 * time.Millisecond)
			settle()
			if !a.ui.SidebarPeek || !a.projectMenu(a.workspaces[0]).Value() {
				t.Fatal("nested menu closed the preview")
			}
			router.Queue(key.Event{Name: key.NameEscape, State: key.Press}, key.Event{Name: key.NameEscape, State: key.Release})
			settle()
			if a.projectMenu(a.workspaces[0]).Value() {
				t.Fatal("Escape did not close nested menu")
			}
			move(float32(700*scale), float32(300*scale))
			now = now.Add(100 * time.Millisecond)
			settle()
			if !a.ui.SidebarPeek {
				t.Fatal("preview closed before grace period")
			}
			now = now.Add(100 * time.Millisecond)
			settle()
			if a.ui.SidebarPeek {
				t.Fatal("preview stayed open after pointer left")
			}
			toggle = find("展开侧栏")
			move(float32(toggle.Min.X+toggle.Dx()/2), float32(toggle.Min.Y+toggle.Dy()/2))
			if !a.ui.SidebarPeek {
				t.Fatal("preview did not reopen")
			}
			click(find("展开侧栏"))
			if a.ui.Preferences.Collapsed || a.ui.SidebarPeek {
				t.Fatal("click did not pin sidebar open")
			}
			find("收起侧栏")
		})
	}
}

func TestRenderCollapsedSidebarPreview(t *testing.T) {
	dir := os.Getenv("ONECATCH_KEEL_PREVIEWS")
	if dir == "" {
		t.Skip("set ONECATCH_KEEL_PREVIEWS to render")
	}
	defer applyPalette(false)
	for _, dark := range []bool{false, true} {
		applyPalette(dark)
		a := conversationFixture(t)
		a.dark = dark
		a.ui.Preferences.Collapsed = true
		root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element { a.ui.SidebarPeek = true; return a.Render(cx) }))
		if err := screenshotWithFocus(root, 1120, 800, filepath.Join(dir, fmt.Sprintf("sidebar-peek-dark-%v.png", dark))); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSidebarToggleSharesNativeTrafficLightCenter(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("native traffic lights are macOS only")
	}
	for _, width := range []int{860, 1120} {
		for _, scale := range []int{1, 2} {
			t.Run(fmt.Sprintf("%d/%dx", width, scale), func(t *testing.T) {
				a := conversationFixture(t)
				router, frame, click := alignmentRouter(el.Root(a), width, 800, scale)
				find := func(label string) image.Rectangle {
					t.Helper()
					box, ok := findSkillControl(router.AppendSemantics(nil), label)
					if !ok {
						t.Fatalf("missing %s", label)
					}
					return box
				}
				assertAligned := func(box image.Rectangle) {
					t.Helper()
					if box.Min.Y+box.Dy()/2 != int(mainTitleBarHeight/2)*scale || box.Min.X != int(macSidebarToggleLeft)*scale {
						t.Fatalf("toggle %v differs from native center %.0fdp and shared left %.0fdp", box, mainTitleBarHeight/2, macSidebarToggleLeft)
					}
				}
				if width >= 861 {
					fixed := find("收起侧栏")
					assertAligned(fixed)
					click(fixed)
				}
				collapsed := find("展开侧栏")
				assertAligned(collapsed)
				// Open the overlay through state here; the hover lifecycle has its own
				// real-pointer regression. Check geometry after layout settles.
				a.ui.SidebarPeek = true
				frame()
				frame()
				var inPreview func([]input.SemanticNode) (image.Rectangle, bool)
				inPreview = func(nodes []input.SemanticNode) (image.Rectangle, bool) {
					for _, node := range nodes {
						if node.Desc.Label == "侧栏预览" {
							return findSkillControl(node.Children, "展开侧栏")
						}
						if box, ok := inPreview(node.Children); ok {
							return box, true
						}
					}
					return image.Rectangle{}, false
				}
				box, ok := inPreview(router.AppendSemantics(nil))
				if !ok {
					t.Fatal("missing preview toggle")
				}
				assertAligned(box)
			})
		}
	}
}

// Check rendered pixels: Sidebar owns child fills, so changing only its outer
// container previously left the pane at the same shade as the conversation.
func TestDarkSidebarPreviewHasAnElevatedSurface(t *testing.T) {
	applyPalette(true)
	defer applyPalette(false)
	a := conversationFixture(t)
	a.dark = true
	a.ui.Preferences.Collapsed = true
	root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element { a.ui.SidebarPeek = true; return a.Render(cx) }))
	paneColor, pageColor := theme.Surface, theme.Bg
	path := filepath.Join(t.TempDir(), "preview.png")
	if err := screenshotWithFocus(root, 1120, 800, path); err != nil {
		t.Fatal(err)
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
	at := func(x, y int) color.NRGBA { return color.NRGBAModel.Convert(pixels.At(x*2, y*2)).(color.NRGBA) }
	if got := at(15, 150); got != paneColor {
		t.Fatalf("preview filled by its child: got %v want %v", got, paneColor)
	}
	if got := at(600, 80); got != pageColor {
		t.Fatalf("pane palette leaked into page: got %v want %v", got, pageColor)
	}
	if paneColor == pageColor {
		t.Fatal("preview indistinguishable from page")
	}
	if theme.Bg != pageColor {
		t.Fatal("scoped sidebar palette changed the global palette")
	}
}
