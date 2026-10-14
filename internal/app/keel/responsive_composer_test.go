package keeldesktop

import (
	"fmt"
	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/window"
	"os"
	"path/filepath"
	"testing"
)

func TestResponsiveComposerStaysSingleLine(t *testing.T) {
	applyPalette(false)
	for _, width := range []int{1120, 860, 680} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			a := conversationFixture(t)
			a.ui.Preferences.SidebarWidth = 320
			a.detail = nil
			a.prompt = "test"
			root := el.Root(a)
			router, frame, _ := alignmentRouter(root, width, 800, 1)
			frame()
			frame()
			send, ok := findSkillControl(router.AppendSemantics(nil), "发送")
			if !ok {
				t.Fatal("missing send button")
			}
			template, ok := alignmentTextAtRow(router.AppendSemantics(nil), "模板", send)
			if !ok {
				t.Fatal("template and send button are not on the same row")
			}
			if template.Min.Y >= send.Max.Y || template.Max.Y <= send.Min.Y {
				t.Fatalf("composer wrapped: send=%v template=%v", send, template)
			}
			if a.ui.Preferences.SidebarWidth != 320 || a.ui.Preferences.Collapsed {
				t.Fatal("responsive layout changed saved preferences")
			}
			for _, label := range []string{"Terminal", "Changes", "Files", "更多操作"} {
				if _, ok := findSkillControl(router.AppendSemantics(nil), label); !ok {
					t.Fatalf("missing %s", label)
				}
			}
			if dir := os.Getenv("ONECATCH_KEEL_PREVIEWS"); dir != "" {
				widget := core.Func(func(gtx core.C) core.D { gtx.Source = router.Source(); return root.Layout(gtx) })
				if err := window.Screenshot(screenshotRoot{widget}, width, 800, filepath.Join(dir, fmt.Sprintf("responsive-composer-%d.png", width))); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
