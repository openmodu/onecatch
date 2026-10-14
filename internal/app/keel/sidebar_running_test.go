package keeldesktop

import (
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"testing"

	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/window"
	tasks "github.com/openmodu/onecatch/internal/domain/tasks"
)

func TestSidebarSingleAgentRunningLightPaints(t *testing.T) {
	defer applyPalette(false)
	for _, dark := range []bool{false, true} {
		for _, harness := range []string{"codex", "claude", "pi"} {
			for _, scale := range []int{1, 2} {
				t.Run(fmt.Sprintf("dark=%v/harness=%s/scale=%d", dark, harness, scale), func(t *testing.T) {
					applyPalette(dark)
					a := conversationFixture(t)
					a.tasks["modu"][0].Harness = harness
					a.tasks["modu"][0].Status = tasks.StatusRunning
					root := el.Root(a)
					router, frame, _ := alignmentRouter(root, 1120, 900, scale)
					frame()
					light, ok := alignmentNode(router.AppendSemantics(nil), "正在运行", false)
					if !ok {
						t.Fatal("missing running light")
					}
					icon, ok := alignmentNode(router.AppendSemantics(nil), "Agent 图标："+harness, false)
					if !ok || light.Bounds.Max.X+4*scale != icon.Bounds.Min.X || light.Bounds.Min.Y != icon.Bounds.Min.Y+5*scale {
						t.Fatalf("running light moved from the leading gutter: light=%v icon=%v", light.Bounds, icon.Bounds)
					}
					pixels := interactionPixelsScaled(t, root, router, 1120*scale, 900*scale, float32(scale))
					green := 0
					for y := light.Bounds.Min.Y; y < light.Bounds.Max.Y; y++ {
						for x := light.Bounds.Min.X; x < light.Bounds.Max.X; x++ {
							c := color.NRGBAModel.Convert(pixels.At(x, y)).(color.NRGBA)
							if int(c.G) > int(c.R)+20 && int(c.G) > int(c.B)+20 {
								green++
							}
						}
					}
					if green < 2*scale*scale {
						t.Fatalf("running light has no visible green pixels: %d at %v, center=%v", green, light.Bounds, pixels.At(light.Bounds.Min.X+light.Bounds.Dx()/2, light.Bounds.Min.Y+light.Bounds.Dy()/2))
					}
					a.tasks["modu"][0].Status = tasks.StatusCompleted
					frame()
					frame()
					if _, ok := alignmentNode(router.AppendSemantics(nil), "正在运行", false); ok {
						t.Fatal("completed task retained its running light")
					}
				})
			}
		}
	}
}

func TestRenderSidebarRunningLights(t *testing.T) {
	dir := os.Getenv("ONECATCH_KEEL_PREVIEWS")
	if dir == "" {
		t.Skip("optional preview")
	}
	a := conversationFixture(t)
	a.tasks["modu"] = nil
	for _, harness := range []string{"codex", "claude", "pi"} {
		a.tasks["modu"] = append(a.tasks["modu"], tasks.Task{ID: harness, Title: harness + " 正在运行", WorkspaceID: "modu", WorkflowID: "single_agent", Harness: harness, Status: tasks.StatusRunning})
	}
	root := el.Root(a)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := window.Screenshot(screenshotRoot{core.Func(root.Layout)}, 1120, 760, filepath.Join(dir, "sidebar-running-lights.png")); err != nil {
		t.Fatal(err)
	}
}
