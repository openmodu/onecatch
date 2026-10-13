package keeldesktop

import (
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dyike/keel/gio/io/input"
	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/theme"
	"github.com/dyike/keel/ui/window"
	workflows "github.com/openmodu/onecatch/internal/domain/workflows"
)

func loopSemantic(nodes []input.SemanticNode, label string) (input.SemanticDesc, bool) {
	for _, node := range nodes {
		if node.Desc.Label == label {
			return node.Desc, true
		}
		if desc, ok := loopSemantic(node.Children, label); ok {
			return desc, true
		}
	}
	return input.SemanticDesc{}, false
}

func TestComposerLoopSwitchTrackIsVisibleInBothThemes(t *testing.T) {
	defer applyPalette(false)
	for _, dark := range []bool{false, true} {
		for _, enabled := range []bool{false, true} {
			for _, scale := range []int{1, 2} {
				t.Run(fmt.Sprintf("dark=%v/on=%v/scale=%d", dark, enabled, scale), func(t *testing.T) {
					applyPalette(dark)
					a := conversationFixture(t)
					a.task, a.detail, a.runID = nil, nil, ""
					d := starterWorkflow("design_to_pr")
					d.LoopEnabled = enabled
					a.definitions = append(a.definitions, d)
					a.workflow.SetValue(d.ID)
					root := el.Root(el.ViewFunc(a.composer))
					router, _, _ := alignmentRouter(root, 900, 400, scale)
					control, ok := alignmentNode(router.AppendSemantics(nil), "工作流循环", true)
					if !ok {
						t.Fatal("missing loop switch")
					}
					label := map[bool]string{true: "已开启", false: "已关闭"}[enabled]
					if _, ok := alignmentNode(router.AppendSemantics(nil), label, false); !ok {
						t.Fatal("loop switch has no explicit state label")
					}
					pixels := interactionPixelsScaled(t, root, router, 900*scale, 400*scale, float32(scale))
					bg := theme.Subtle
					visible, colored := 0, 0
					for y := control.Bounds.Min.Y; y < control.Bounds.Max.Y; y++ {
						for x := control.Bounds.Min.X; x < control.Bounds.Min.X+36*scale; x++ {
							c := color.NRGBAModel.Convert(pixels.At(x, y)).(color.NRGBA)
							if abs(int(c.R)-int(bg.R))+abs(int(c.G)-int(bg.G))+abs(int(c.B)-int(bg.B)) > 100 {
								visible++
							}
							if int(max(c.R, c.G, c.B))-int(min(c.R, c.G, c.B)) > 30 {
								colored++
							}
						}
					}
					if visible < 400*scale*scale {
						t.Fatalf("switch track fades into input surface: %d contrasting pixels", visible)
					}
					if enabled && colored < 300*scale*scale {
						t.Fatalf("enabled switch has no clear accent color: %d colored pixels", colored)
					}
					if dir := os.Getenv("ONECATCH_KEEL_PREVIEWS"); dir != "" && scale == 1 {
						if err := os.MkdirAll(dir, 0755); err != nil {
							t.Fatal(err)
						}
						name := fmt.Sprintf("loop-switch-dark-%v-on-%v.png", dark, enabled)
						if err := window.Screenshot(screenshotRoot{core.Func(root.Layout)}, 900, 400, filepath.Join(dir, name)); err != nil {
							t.Fatal(err)
						}
					}
				})
			}
		}
	}
}

func TestComposerLoopSwitchIsInsideInputAndKeepsDraft(t *testing.T) {
	a := conversationFixture(t)
	a.task, a.detail, a.taskID, a.runID = nil, nil, "", ""
	d := starterWorkflow("design_to_pr")
	d.LoopEnabled = false
	a.definitions = append(a.definitions, d)
	a.workflow.SetValue(d.ID)
	a.prompt = "实现接口并补全测试"
	root := el.Root(el.ViewFunc(a.composer))
	router, frame, click := alignmentRouter(root, 900, 400, 1)
	control, ok := alignmentNode(router.AppendSemantics(nil), "工作流循环", true)
	if !ok {
		t.Fatal("workflow has no loop switch")
	}
	surface, ok := loopSemantic(router.AppendSemantics(nil), "消息输入框")
	if !ok || !control.Bounds.In(surface.Bounds) {
		t.Fatalf("switch is outside input surface: switch=%v surface=%v", control.Bounds, surface.Bounds)
	}
	input, ok := loopSemantic(router.AppendSemantics(nil), "任务指令")
	if !ok || control.Bounds.Max.Y > input.Bounds.Min.Y {
		t.Fatal("switch does not sit above input text")
	}
	click(control.Bounds)
	frame()
	if !a.composerLoop.enabled || !a.composerLoop.control.Value() || a.prompt != "实现接口并补全测试" {
		t.Fatal("loop switch lost its value or changed the message")
	}
	click(control.Bounds)
	frame()
	if a.composerLoop.enabled || a.prompt != "实现接口并补全测试" {
		t.Fatal("loop switch cannot be turned off")
	}
	a.workflow.SetValue("single_agent")
	frame()
	if _, ok := loopSemantic(router.AppendSemantics(nil), "工作流循环"); ok {
		t.Fatal("single Agent shows workflow loop switch")
	}
}

func TestComposerLoopSwitchFollowsRunAndDisablesUntilPaused(t *testing.T) {
	a := conversationFixture(t)
	a.detail = pausedPipelineDetail()
	a.runID = a.detail.Run.ID
	a.detail.Run.LoopEnabled, a.detail.Run.LoopRound = true, 3
	a.detail.Active = true
	root := el.Root(el.ViewFunc(a.composer))
	router, frame, _ := alignmentRouter(root, 900, 400, 1)
	control, ok := loopSemantic(router.AppendSemantics(nil), "工作流循环")
	if !ok || !control.Disabled || !a.composerLoop.control.Value() {
		t.Fatal("active run loop state is wrong or mutable")
	}
	if _, ok := alignmentNode(router.AppendSemantics(nil), "暂停后切换", true); !ok {
		t.Fatal("active run offers no way to pause for switching")
	}
	a.detail.Active = false
	frame()
	control, ok = loopSemantic(router.AppendSemantics(nil), "工作流循环")
	if !ok || control.Disabled {
		t.Fatal("paused workflow cannot switch loop mode")
	}
	a.detail.Run.LoopEnabled = false
	frame()
	if a.composerLoop.control.Value() {
		t.Fatal("switch retained stale loop state")
	}
	if dir := os.Getenv("ONECATCH_KEEL_PREVIEWS"); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		stable := core.Func(func(gtx core.C) core.D {
			gtx.Now = gtx.Now.Add(time.Second)
			return root.Layout(gtx)
		})
		if err := window.Screenshot(screenshotRoot{stable}, 900, 400, filepath.Join(dir, "composer-loop-switch.png")); err != nil {
			t.Fatal(err)
		}
	}
}

func TestComposerLoopDefaultsDoNotLeakBetweenWorkflows(t *testing.T) {
	a := conversationFixture(t)
	a.task, a.detail, a.runID = nil, nil, ""
	d := starterWorkflow("design_to_pr")
	d.LoopEnabled = true
	other := workflows.Definition{ID: "other", Name: "Other", Steps: d.Steps}
	a.definitions = []workflows.Definition{d, other}
	a.workflow.SetValue(d.ID)
	a.syncComposerLoop()
	if !a.composerLoop.enabled {
		t.Fatal("workflow loop default was lost")
	}
	a.workflow.SetValue(other.ID)
	a.syncComposerLoop()
	if a.composerLoop.enabled {
		t.Fatal("loop choice leaked into a different workflow")
	}
}
