package keeldesktop

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dyike/keel/gio/io/input"
	"github.com/dyike/keel/gio/io/key"
	"github.com/dyike/keel/gio/layout"
	"github.com/dyike/keel/gio/op"
	"github.com/dyike/keel/gio/unit"
	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/theme"
	"github.com/dyike/keel/ui/window"
)

func TestRenderComposerAccessMenu(t *testing.T) {
	dir := os.Getenv("ONECATCH_KEEL_PREVIEWS")
	if dir == "" {
		t.Skip("set ONECATCH_KEEL_PREVIEWS")
	}
	defer applyPalette(false)
	for _, project := range []bool{false, true} {
		for _, dark := range []bool{false, true} {
			applyPalette(dark)
			a := alignmentFixture(t)
			a.task = nil
			a.sandbox.SetValue("full")
			focusID := a.sandbox.FocusID()
			height := 450
			if project {
				a.newProject()
				a.settings.Security.AllowFullSandbox = true
				a.project.Access.SetValue("full")
				focusID = a.project.Access.FocusID()
				height = 720
			}
			var router input.Router
			var ops op.Ops
			requested := false
			root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element {
				if !requested {
					cx.Focus(focusID)
					requested = true
				}
				if project {
					return a.Render(cx)
				}
				return el.Div().WFull().HFull().Bg(theme.Bg).P(20).Child(a.composer(cx))
			}))
			frame := func() {
				ops.Reset()
				root.Layout(layout.Context{Ops: &ops, Now: time.Now(), Source: router.Source(), Constraints: layout.Exact(image.Pt(800, height)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}})
				router.Frame(&ops)
			}
			frame()
			frame()
			router.Queue(key.Event{Name: key.NameSpace, State: key.Press}, key.Event{Name: key.NameSpace, State: key.Release})
			frame()
			frame()
			widget := core.Func(func(gtx core.C) core.D { gtx.Source = router.Source(); return root.Layout(gtx) })
			if err := window.Screenshot(screenshotRoot{widget}, 800, height, filepath.Join(dir, fmt.Sprintf("access-menu-project-%v-dark-%v.png", project, dark))); err != nil {
				t.Fatal(err)
			}
		}
	}

}
