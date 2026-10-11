package keeldesktop

import (
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

func TestWindowsHeaderKeepsSidebarToggleBesideAppIcon(t *testing.T) {
	a := alignmentFixture(t)
	menu, err := window.NewMenuBar(window.MenuItem{Title: "OneCatch", Children: []window.MenuItem{{Title: "设置", OnSelect: func() {}}}})
	if err != nil {
		t.Fatal(err)
	}
	a.applicationMenu = menu
	for _, collapsed := range []bool{false, true} {
		a.ui.Preferences.Collapsed = collapsed
		root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element { return a.headerForPlatform(cx, "windows") }))
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
	if got := sidebarContentHeight("windows", 600); got != 556 {
		t.Fatalf("sidebar footer exceeds content area: %v", got)
	}
	if got := sidebarContentHeight("darwin", 600); got != 600 {
		t.Fatalf("macOS sidebar changed: %v", got)
	}
}
