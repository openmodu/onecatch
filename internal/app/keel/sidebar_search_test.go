package keeldesktop

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"gioui.org/io/key"
	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/locale"
	"github.com/dyike/keel/ui/window"
)

func TestSidebarSearchOpensPaletteAndFilters(t *testing.T) {
	applyPalette(false)
	a := conversationFixture(t)
	focused := false
	root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element {
		view := a.Render(cx)
		if a.ui.SearchPalette != nil {
			focused = cx.Focused(a.ui.SearchPalette.FocusID())
		}
		return view
	}))
	router, frame, click := alignmentRouter(root, 1120, 800, 1)
	search, ok := findSkillControl(router.AppendSemantics(nil), "搜索任务")
	if !ok {
		t.Fatal("missing search entry")
	}
	click(search)
	frame()
	frame()
	if a.ui.SearchPalette == nil || !a.ui.SearchPalette.Value() {
		t.Fatal("search did not open")
	}
	if !focused {
		t.Fatal("opening search did not focus input")
	}
	router.Queue(key.EditEvent{Text: "Say hello"})
	frame()
	frame()
	if a.ui.SearchPalette.Query() != "Say hello" || a.ui.SearchPalette.MatchedCount() != 1 {
		t.Fatalf("palette search failed: %q / %d", a.ui.SearchPalette.Query(), a.ui.SearchPalette.MatchedCount())
	}
	if a.query != "" {
		t.Fatal("palette changed sidebar filtering")
	}
	a.ui.SearchCategory = "projects"
	a.refreshSearchItems()
	if a.ui.SearchPalette.MatchedCount() != 0 {
		t.Fatal("project category retained conversation")
	}
	a.ui.SearchPalette.SetQuery("modu")
	frame()
	if a.ui.SearchPalette.MatchedCount() != 1 {
		t.Fatal("project search failed")
	}
	a.ui.SearchPalette.SetQuery("")
	router.Queue(key.Event{Name: key.NameEscape, State: key.Press})
	frame()
	frame()
	if a.ui.SearchPalette.Value() {
		t.Fatal("Escape did not close search")
	}
	a.ui.Preferences.Collapsed = true
	a.focusTaskSearch()
	frame()
	frame()
	if !focused || !a.ui.Preferences.Collapsed || !a.ui.SearchPalette.Value() {
		t.Fatal("shortcut should open palette without expanding sidebar")
	}
	close, ok := findSkillControl(router.AppendSemantics(nil), locale.Current().Close)
	if !ok {
		t.Fatal("missing close button")
	}
	click(close)
	frame()
	frame()
	if a.ui.SearchPalette.Value() {
		t.Fatal("close button did not dismiss search")
	}
}

func TestRenderSidebarSearchPalette(t *testing.T) {
	dir := os.Getenv("ONECATCH_KEEL_PREVIEWS")
	if dir == "" {
		t.Skip("set ONECATCH_KEEL_PREVIEWS")
	}
	defer applyPalette(false)
	for _, dark := range []bool{false, true} {
		applyPalette(dark)
		a := conversationFixture(t)
		a.dark = dark
		a.focusTaskSearch()
		root := el.Root(a)
		router, frame, _ := alignmentRouter(root, 1120, 800, 1)
		frame()
		frame()
		widget := core.Func(func(gtx core.C) core.D { gtx.Source = router.Source(); return root.Layout(gtx) })
		if err := window.Screenshot(screenshotRoot{widget}, 1120, 800, filepath.Join(dir, fmt.Sprintf("search-palette-%v.png", dark))); err != nil {
			t.Fatal(err)
		}
		a.ui.SearchPalette.SetValue(false)
		a.ui.Footer.SetValue(true)
		frame()
		frame()
		if err := window.Screenshot(screenshotRoot{widget}, 1120, 800, filepath.Join(dir, fmt.Sprintf("footer-menu-%v.png", dark))); err != nil {
			t.Fatal(err)
		}
	}
}
