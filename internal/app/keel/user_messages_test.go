package keeldesktop

import (
	"bytes"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/locale"
	desktopservice "github.com/openmodu/onecatch/internal/service/desktop"
)

// Exercise the production renderer with reopened service data, rather than
// hand-written fixture entries that bypass the history conversion.
func verifyPersistedUserBubble(t *testing.T, b *backend, detail desktopservice.RunDetail) {
	t.Helper()
	a := newApp(b)
	t.Cleanup(a.close)
	icon, err := png.Decode(bytes.NewReader(appIcon))
	if err != nil {
		t.Fatal(err)
	}
	a.logo = kit.Image(icon, "OneCatch").Size(22, 22).Rounded(5)
	a.runtime.SetValue(detail.Task.Harness)
	a.workflow.SetValue(detail.Task.WorkflowID)
	a.sandbox.SetValue(detail.Task.Sandbox)
	a.task, a.taskID, a.runID = &detail.Task, detail.Task.ID, detail.Run.ID
	a.detail, a.workspaceID = &detail, detail.Workspace.ID
	a.workspaces = append(a.workspaces, detail.Workspace)
	a.transcript.detail(detail)
	a.syncMessages()
	locale.Apply(locale.Chinese())
	applyPalette(false)
	// Render each actual user row to test initial, legacy and new send bubbles
	// without virtual scrolling hiding earlier rows in a longer conversation.
	for index, e := range a.entries {
		if !e.user {
			continue
		}
		root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element { return a.message(cx, index) }))
		for _, scale := range []int{1, 2} {
			router, frame, _ := alignmentRouter(root, 900, 180, scale)
			frame()
			frame()
			bubble, ok := alignmentNode(router.AppendSemantics(nil), "用户消息："+e.text, false)
			if !ok || bubble.Bounds.Dy() < 30*scale || bubble.Bounds.Dy() > 48*scale || bubble.Bounds.Min.X < 450*scale || bubble.Bounds.Max.X < 850*scale {
				t.Fatalf("user bubble %q must be visible on the right at %dx: %v, found=%v", e.text, scale, bubble.Bounds, ok)
			}
		}
	}
	if dir := os.Getenv("ONECATCH_KEEL_PREVIEWS"); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := screenshotWithFocus(el.Root(a), 1120, 900, filepath.Join(dir, "conversation-persisted-user-bubbles.png")); err != nil {
			t.Fatal(err)
		}
	}
}
