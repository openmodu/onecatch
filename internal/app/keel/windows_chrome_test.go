package keeldesktop

import (
	"github.com/dyike/keel/gio/f32"
	"github.com/dyike/keel/gio/io/system"
	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/window"
	"testing"
)

type captionTestWindow struct {
	maximized bool
	calls     []string
}

func (*captionTestWindow) Focused() bool                                   { return true }
func (*captionTestWindow) Frameless() bool                                 { return true }
func (*captionTestWindow) TitleBarArea(float32, float32, float32, float32) {}
func (w *captionTestWindow) Maximized() bool                               { return w.maximized }
func (w *captionTestWindow) Minimize()                                     { w.calls = append(w.calls, "min") }
func (w *captionTestWindow) ToggleMaximize() {
	w.maximized = !w.maximized
	w.calls = append(w.calls, "max")
}
func (w *captionTestWindow) Close() { w.calls = append(w.calls, "close") }

func TestWindowsCaptionControls(t *testing.T) {
	for _, scale := range []int{1, 2} {
		w := &captionTestWindow{}
		restore := core.SetCurrentWindow(w)
		root := el.Root(el.ViewFunc(windowsWindowButtons))
		router, frame, click := alignmentRouter(root, 140, 44, scale)
		for _, label := range []string{"最小化", "最大化", "还原", "关闭"} {
			frame()
			node, ok := alignmentButton(router.AppendSemantics(nil), label)
			if !ok {
				t.Fatalf("missing %s", label)
			}
			if node.Bounds.Dx() != 44*scale || node.Bounds.Max.X > 140*scale {
				t.Fatalf("caption geometry %v", node.Bounds)
			}
			click(node.Bounds)
		}
		restore()
		if len(w.calls) != 4 || w.calls[0] != "min" || w.calls[3] != "close" || w.maximized {
			t.Fatalf("actions %v", w.calls)
		}
	}
}

func TestDesktopHeaderKeepsSidebarToggleBesideAppIcon(t *testing.T) {
	a := alignmentFixture(t)
	menu, err := window.NewMenuBar(window.MenuItem{Title: "OneCatch", Children: []window.MenuItem{{Title: "设置", OnSelect: func() {}}}})
	if err != nil {
		t.Fatal(err)
	}
	a.applicationMenu = menu
	for _, platform := range []string{"windows", "linux"} {
		for _, collapsed := range []bool{false, true} {
			a.ui.Preferences.Collapsed = collapsed
			root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element { return a.headerForPlatform(cx, platform) }))
			router, _, _ := alignmentRouter(root, 1000, 44, 1)
			nodes := router.AppendSemantics(nil)
			appIcon, ok := alignmentButton(nodes, "OneCatch 应用菜单")
			if !ok {
				t.Fatal("app icon menu missing")
			}
			label := "收起侧栏"
			if collapsed {
				label = "展开侧栏"
			}
			toggle, ok := alignmentButton(nodes, label)
			if !ok || toggle.Bounds.Min.X < appIcon.Bounds.Max.X || toggle.Bounds.Max.Y > 44 {
				t.Fatalf("toggle not next to app icon: %v %v", appIcon.Bounds, toggle.Bounds)
			}
		}
	}
	for _, platform := range []string{"windows", "linux"} {
		if got := sidebarContentHeight(platform, 600); got != 556 {
			t.Fatalf("%s sidebar footer exceeds content area: %v", platform, got)
		}
	}
	if got := sidebarContentHeight("darwin", 600); got != 600 {
		t.Fatalf("macOS sidebar changed: %v", got)
	}
}

func TestDesktopHeaderNativeDragHitRegion(t *testing.T) {
	a := alignmentFixture(t)
	a.ui.Preferences.Collapsed = false
	w := &captionTestWindow{}
	defer core.SetCurrentWindow(w)()
	for _, platform := range []string{"windows", "linux"} {
		for _, scale := range []int{1, 2} {
			root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element { return a.headerForPlatform(cx, platform) }))
			router, _, _ := alignmentRouter(root, 1000, 44, scale)
			position := f32.Pt(float32((a.ui.Preferences.SidebarWidth+100)*float32(scale)), 22*float32(scale))
			if action, ok := router.ActionAt(position); !ok || action != system.ActionMove {
				t.Fatalf("title area does not produce native move action at %v: %v %v", position, action, ok)
			}
			for _, label := range []string{"最小化", "最大化", "关闭", "收起侧栏"} {
				node, ok := alignmentButton(router.AppendSemantics(nil), label)
				if !ok {
					t.Fatal(label)
				}
				p := f32.Pt(float32(node.Bounds.Min.X+node.Bounds.Dx()/2), float32(node.Bounds.Min.Y+node.Bounds.Dy()/2))
				if _, ok := router.ActionAt(p); ok {
					t.Fatalf("%s is covered by drag region", label)
				}
			}
		}
	}
}

type decoratedCaptionTestWindow struct{ captionTestWindow }

func (*decoratedCaptionTestWindow) Frameless() bool { return false }

func TestDecoratedDesktopDoesNotDuplicateCaptionButtons(t *testing.T) {
	w := &decoratedCaptionTestWindow{}
	defer core.SetCurrentWindow(w)()
	root := el.Root(el.ViewFunc(desktopWindowButtons))
	router, _, _ := alignmentRouter(root, 140, 44, 1)
	for _, label := range []string{"最小化", "最大化", "关闭"} {
		if _, ok := alignmentButton(router.AppendSemantics(nil), label); ok {
			t.Fatalf("duplicate system caption button: %s", label)
		}
	}
}
