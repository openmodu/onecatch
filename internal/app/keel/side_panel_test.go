package keeldesktop

import (
	"fmt"
	"github.com/dyike/keel/ui/el"
	"os"
	"path/filepath"
	"testing"
)

func TestRightPanelSwitchAndToggle(t *testing.T) {
	a := alignmentFixture(t)
	a.workbench.Visible = true
	a.workbench.Tab = "review"
	a.toggleTerminal()
	if !a.terminal.Open || a.workbench.Visible {
		t.Fatal("terminal did not replace inspector")
	}
	a.toggleTerminal()
	if a.terminal.Open {
		t.Fatal("terminal did not close")
	}
	a.workbench.Visible = true
	a.workbench.Tab = "files"
	a.toggleInspector("files")
	if a.workbench.Visible {
		t.Fatal("active files panel did not close")
	}
}

func TestRenderRightPanelCards(t *testing.T) {
	dir := os.Getenv("ONECATCH_KEEL_PREVIEWS")
	if dir == "" {
		t.Skip("set ONECATCH_KEEL_PREVIEWS")
	}
	defer applyPalette(false)
	for _, dark := range []bool{false, true} {
		for _, panel := range []string{"terminal", "files", "review"} {
			a := alignmentFixture(t)
			a.dark = dark
			applyPalette(dark)
			a.workbench.Width = 520
			a.terminal.Open = panel == "terminal"
			a.workbench.Visible = panel != "terminal"
			a.workbench.Tab = panel
			if panel == "review" {
				state := a.reviewState(a.workspaceContext())
				state.Files = parseGitReview("diff --git a/docs/kit/input.md b/docs/kit/input.md\n--- a/docs/kit/input.md\n+++ b/docs/kit/input.md\n@@ -25,4 +25,5 @@\n Input rendering\n-old text\n+New text wraps naturally within the panel instead of forcing horizontal scrolling. 长行根据面板宽度自动换行，并保留完整内容。\n+Added scrollbar behavior\n Unchanged context\ndiff --git a/ui/sidebar.go b/ui/sidebar.go\n--- a/ui/sidebar.go\n+++ b/ui/sidebar.go\n@@ -1 +1 @@\n-old\n+new\n", "worktree")
			}
			if err := screenshotWithFocus(el.Root(a), 1280, 800, filepath.Join(dir, fmt.Sprintf("right-panel-%s-%v.png", panel, dark))); err != nil {
				t.Fatal(err)
			}
			if panel == "review" {
				state := a.reviewState(a.workspaceContext())
				state.SideBySide = true
				if err := screenshotWithFocus(el.Root(a), 1280, 800, filepath.Join(dir, fmt.Sprintf("review-paired-%v.png", dark))); err != nil {
					t.Fatal(err)
				}
				state.FilePicker.SetValue(true)
				state.FileSearchFocus = true
				if err := screenshotWithFocus(el.Root(a), 1280, 800, filepath.Join(dir, fmt.Sprintf("review-file-search-%v.png", dark))); err != nil {
					t.Fatal(err)
				}
				state.FilePicker.SetValue(false)
				state.ShowFiles = true
				a.workbench.Maximized = true
				a.ui.Preferences.Collapsed = true
				if err := screenshotWithFocus(el.Root(a), 1280, 800, filepath.Join(dir, fmt.Sprintf("review-file-sidebar-%v.png", dark))); err != nil {
					t.Fatal(err)
				}
				state.ShowFiles = false
				a.workbench.Maximized = false
				state.Options.SetValue(true)
				if err := screenshotWithFocus(el.Root(a), 1280, 800, filepath.Join(dir, fmt.Sprintf("review-options-%v.png", dark))); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
}
