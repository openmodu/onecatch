package keeldesktop

import (
	"image"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/locale"
	"github.com/dyike/keel/ui/window"
)

func TestProjectAccessWholeRowAndKeyboard(t *testing.T) {
	if os.Getenv("ONECATCH_KEEL_SELECT_PATCH") != "1" {
		t.Skip("run via go run ./build/keel/go test to test the Keel dependency fix")
	}
	a := conversationFixture(t)
	a.newProject()
	a.settings.Security.AllowFullSandbox = true
	root := el.Root(a)
	var router input.Router
	var ops op.Ops
	frame := func() {
		ops.Reset()
		root.Layout(layout.Context{Ops: &ops, Now: time.Now(), Source: router.Source(), Constraints: layout.Exact(image.Pt(860, 600)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}})
		router.Frame(&ops)
	}
	frame()
	frame()
	box, ok := findSkillControl(router.AppendSemantics(nil), "工作区访问")
	if !ok || box.Dy() != 32 {
		t.Fatalf("access trigger must span its 32dp row: %v, found=%v; run with go run ./build/keel/go test", box, ok)
	}
	// Click the blank top-left padding, where the former text-height trigger
	// never received clicks. Keyboard selection must still update the model.
	p := f32.Pt(float32(box.Min.X+1), float32(box.Min.Y+1))
	router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: p}, pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: p}, pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: p})
	frame()
	frame()
	router.Queue(key.Event{Name: key.NameDownArrow, State: key.Press}, key.Event{Name: key.NameDownArrow, State: key.Release})
	frame()
	router.Queue(key.Event{Name: key.NameReturn, State: key.Press}, key.Event{Name: key.NameReturn, State: key.Release})
	frame()
	frame()
	if a.project.Access.Value() != "full" {
		t.Fatalf("permission did not change through keyboard: %s", a.project.Access.Value())
	}
	// Closing the menu restores the trigger as a Tab stop, with Space reopening.
	router.Queue(key.Event{Name: key.NameSpace, State: key.Press}, key.Event{Name: key.NameSpace, State: key.Release})
	frame()
	frame()
	if _, ok := findSkillControl(router.AppendSemantics(nil), "只读访问"); !ok {
		t.Fatal("menu did not reopen after returning focus")
	}
	router.Queue(key.Event{Name: key.NameEscape, State: key.Press}, key.Event{Name: key.NameEscape, State: key.Release})
	frame()
	frame()
	if a.project.Access.Value() != "full" || !a.projectDialog.Value() {
		t.Fatal("Escape changed selection or closed the parent dialog")
	}
}

// Render several frames through Gio's router before capture, so focus and
// hover are real painted states rather than first-frame approximations.
type screenshotRoot struct{ core.Func }

func (screenshotRoot) FillsWindow() bool { return true }

func screenshotWithFocus(root *el.RootWidget, width, height int, path string) error {
	var router input.Router
	warm := true
	widget := core.Func(func(gtx core.C) core.D {
		gtx.Source = router.Source()
		if warm {
			var ops op.Ops
			pre := gtx
			pre.Ops = &ops
			for range 4 {
				ops.Reset()
				root.Layout(pre)
				router.Frame(&ops)
			}
			warm = false
		}
		return root.Layout(gtx)
	})
	return window.Screenshot(screenshotRoot{widget}, width, height, path)
}

func TestRenderProjectSelectFocus(t *testing.T) {
	dir := os.Getenv("ONECATCH_KEEL_PREVIEWS")
	if dir == "" {
		t.Skip("set ONECATCH_KEEL_PREVIEWS to capture")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	locale.Apply(locale.Chinese())
	a := conversationFixture(t)
	a.newProject()
	a.project.Access.SetValue("full")
	a.settings.Security.AllowFullSandbox = true
	for _, state := range []struct {
		name string
		w, h int
		dark bool
	}{{"project-select-focus", 686, 548, false}, {"project-select-focus-narrow", 860, 600, false}, {"project-select-focus-dark", 860, 600, true}} {
		applyPalette(state.dark)
		focus := a.project.Access.FocusID()
		requested, focused := false, false
		root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element {
			content := a.Render(cx)
			focused = cx.FocusVisible(focus)
			if !requested {
				cx.Focus(focus)
				requested = true
			}
			return content
		}))
		if err := screenshotWithFocus(root, state.w, state.h, filepath.Join(dir, state.name+".png")); err != nil {
			t.Fatal(err)
		}
		if !focused {
			t.Fatal("capture did not reach keyboard focus state")
		}
	}
	applyPalette(false)
}
