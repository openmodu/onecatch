package keeldesktop

import (
	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/theme"
	"github.com/dyike/keel/ui/window"
	workflows "github.com/openmodu/onecatch/internal/domain/workflows"
	"os"
	"path/filepath"
	"testing"
)

func TestLoopControlsShowRoundAndPauseOrContinue(t *testing.T) {
	a := conversationFixture(t)
	a.detail = pausedPipelineDetail()
	a.detail.Run.LoopEnabled = true
	a.detail.Run.LoopRound = 4
	a.detail.Active = true
	root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element { return el.Div().WFull().HFull().Child(a.workflowLoopBar(cx)) }))
	router, frame, _ := alignmentRouter(root, 760, 180, 1)
	frame()
	if _, ok := alignmentNode(router.AppendSemantics(nil), "第 4 轮 · 循环中", false); !ok {
		t.Fatal("missing live round")
	}
	if _, ok := alignmentNode(router.AppendSemantics(nil), "暂停", true); !ok {
		t.Fatal("missing interrupt")
	}
	a.detail.Active = false
	frame()
	if _, ok := alignmentNode(router.AppendSemantics(nil), "第 4 轮 · 已暂停", false); !ok {
		t.Fatal("lost paused round")
	}
	if _, ok := alignmentNode(router.AppendSemantics(nil), "继续", true); !ok {
		t.Fatal("missing resume")
	}
}

func TestRenderOuterLoopPreview(t *testing.T) {
	dir := os.Getenv("ONECATCH_KEEL_PREVIEWS")
	if dir == "" {
		t.Skip("optional preview")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, active := range []bool{true, false} {
		a := conversationFixture(t)
		a.detail = pausedPipelineDetail()
		a.detail.Workflow.Steps = a.detail.Workflow.Steps[:3]
		a.detail.Workflow.Steps[0].Name = "方案设计"
		a.detail.Workflow.Steps[1].Name = "具体实现"
		a.detail.Workflow.Steps[2].Name = "CodeReview"
		a.detail.Workflow.Edges = []workflows.Edge{{From: "design", To: "implement"}, {From: "implement", To: "review"}, {From: "review", To: "implement", Kind: workflows.EdgeRework}}
		a.detail.Run.LoopEnabled = true
		a.detail.Run.LoopRound = 4
		a.detail.Active = active
		a.detail.Run.CurrentStepID = "implement"
		n := a.detail.Run.Nodes["implement"]
		n.Status = workflows.NodeRunning
		if !active {
			n.Status = workflows.NodePaused
		}
		a.detail.Run.Nodes["implement"] = n
		n = a.detail.Run.Nodes["review"]
		n.Status = workflows.NodePending
		a.detail.Run.Nodes["review"] = n
		a.prompt = "下一轮优先优化性能；保留当前接口，补上并发测试。"
		root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element {
			return el.Div().WFull().HFull().P(20).Bg(theme.Surface).Gap(16).Child(
				el.Text("工作流 · 外层 Loop").TextSize(18).Bold(),
				a.workflowLoopBar(cx),
				el.Div().Row().Gap(20).Child(el.Div().W(el.Dp(420)).Child(a.workflowProgress(cx)), el.Div().Grow().W(el.Dp(0)).Gap(12).Child(
					el.Text("补充指令").TextSize(14).Bold(),
					el.Text(map[bool]string{true: "运行中也可以留言；需要立即调整方向时，先暂停。", false: "当前第 4 轮已暂停，发送补充指令后接着本轮继续。"}[active]).TextSize(12).TextColor(theme.Muted),
					a.composer(cx),
				)),
			)
		}))
		name := "loop-paused.png"
		if active {
			name = "loop-running.png"
		}
		if err := window.Screenshot(screenshotRoot{core.Func(root.Layout)}, 960, 1040, filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}
}
