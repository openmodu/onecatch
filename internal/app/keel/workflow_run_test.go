package keeldesktop

import (
	"image"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/window"
	domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"
	desktopservice "github.com/openmodu/onecatch/internal/service/desktop"
)

// pausedPipelineDetail is the design-to-PR workflow stopped after E2E sent
// work back to implementation twice and the rework limit paused the run.
func pausedPipelineDetail() *desktopservice.RunDetail {
	definition := starterWorkflow("design_to_pr")
	nodes := map[string]domainworkflows.NodeState{
		"design":    {StepID: "design", Status: domainworkflows.NodeCompleted, Attempt: 1, Verdict: "pass", Summary: "拆成 store、handler 两层，新增迁移脚本。"},
		"implement": {StepID: "implement", Status: domainworkflows.NodeCompleted, Attempt: 3, Verdict: "pass", Summary: "修复登录超时，补充并发测试。"},
		"review":    {StepID: "review", Status: domainworkflows.NodeCompleted, Attempt: 3, Verdict: "pass", Summary: "改动符合方案。"},
		"e2e":       {StepID: "e2e", Status: domainworkflows.NodePaused, Attempt: 3, Verdict: "rework", Summary: "login spec 仍然超时", Inbox: []domainworkflows.Message{{From: "human", Text: "先跳过 flaky 用例"}}},
		"pr":        {StepID: "pr", Status: domainworkflows.NodePending},
	}
	run := domainworkflows.Run{ID: "run_preview", TaskID: "task_preview", WorkflowID: definition.ID, Status: domainworkflows.RunPaused, CurrentStepID: "e2e", Nodes: nodes,
		ReworkRounds: map[string]int{domainworkflows.EdgeKey("e2e", "implement"): 3, domainworkflows.EdgeKey("review", "implement"): 1},
		PauseReason:  domainworkflows.PauseReasonReworkLimit, LastError: "e2e sent work back to implement 3 times", StartedAt: time.Now()}
	return &desktopservice.RunDetail{Run: run, Workflow: definition}
}

func TestWorkflowProgressOffersRerunAndMessages(t *testing.T) {
	a := &app{detail: pausedPipelineDetail()}
	root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element {
		return el.Div().WFull().HFull().ScrollY().Child(a.workflowProgress(cx))
	}))
	var router input.Router
	var ops op.Ops
	root.Layout(layout.Context{Ops: &ops, Now: time.Now(), Source: router.Source(), Constraints: layout.Exact(image.Pt(360, 2000)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}})
	router.Frame(&ops)
	labels := map[string]int{}
	var walk func([]input.SemanticNode)
	walk = func(nodes []input.SemanticNode) {
		for _, n := range nodes {
			labels[n.Desc.Label]++
			walk(n.Children)
		}
	}
	walk(router.AppendSemantics(nil))
	if labels["从这里重跑"] == 0 || labels["留言"] == 0 || labels["已暂停 · 第 3 次 · 退回 implement 3/3 · 1 条待处理意见"] == 0 {
		t.Fatalf("a paused run should show rework rounds, pending messages, rerun and message: %v", labels)
	}
	a.openStepAction("review", true)
	if a.workbench.StepAction == nil || !a.workbench.StepAction.Rerun || a.workbench.StepAction.StepID != "review" {
		t.Fatalf("step action = %+v", a.workbench.StepAction)
	}
	a.detail.Active = true
	a.workbench.StepAction = nil
	labels = map[string]int{}
	ops.Reset()
	root.Layout(layout.Context{Ops: &ops, Now: time.Now(), Source: router.Source(), Constraints: layout.Exact(image.Pt(360, 2000)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}})
	router.Frame(&ops)
	walk(router.AppendSemantics(nil))
	if labels["从这里重跑"] != 0 || labels["留言"] == 0 {
		t.Fatalf("an active run can take messages but not reruns: %v", labels)
	}
}

func TestRenderWorkflowProgressPreview(t *testing.T) {
	dir := os.Getenv("ONECATCH_KEEL_PREVIEWS")
	if dir == "" {
		t.Skip("set ONECATCH_KEEL_PREVIEWS to render the run progress")
	}
	a := &app{detail: pausedPipelineDetail()}
	a.openStepAction("implement", true)
	a.workbench.StepAction.Text = "超时阈值改成 30s 再试"
	view := el.ViewFunc(func(cx *el.Context) el.Element {
		return el.Div().WFull().HFull().P(14).Child(a.workflowProgress(cx))
	})
	if err := window.Screenshot(el.Root(view), 360, 1100, filepath.Join(dir, "workflow-run-progress.png")); err != nil {
		t.Fatal(err)
	}
}

func TestRenderWorkflowLibraryPreview(t *testing.T) {
	dir := os.Getenv("ONECATCH_KEEL_PREVIEWS")
	if dir == "" {
		t.Skip("set ONECATCH_KEEL_PREVIEWS to render the workflow library")
	}
	a := dagTestApp(t)
	a.ui.WorkflowSource = false
	if err := window.Screenshot(el.Root(workflowView{a}), 1000, 760, filepath.Join(dir, "workflow-library.png")); err != nil {
		t.Fatal(err)
	}
}

// Dependencies between consecutive steps run straight, in either orientation;
// only rework loops take lanes, and no two loops share one.
func TestFlowRoutingKeepsDependenciesDirect(t *testing.T) {
	d := starterWorkflow("design_to_pr")
	for _, vertical := range []bool{false, true} {
		rects, _, _ := flowLayout(d, flowChartOptions{Vertical: vertical, Width: 320})
		lanes := map[float32]bool{}
		for _, e := range routeFlow(d, rects, vertical) {
			if e.Kind == domainworkflows.EdgeNext && len(e.Points) != 4 {
				t.Fatalf("vertical=%v: dependency %s→%s detoured: %v", vertical, e.Source, e.Target, e.Points)
			}
			if e.Kind == domainworkflows.EdgeRework {
				lane := e.Points[1].Y
				if vertical {
					lane = e.Points[1].X
				}
				if lanes[lane] {
					t.Fatalf("vertical=%v: two loops share lane %v", vertical, lane)
				}
				lanes[lane] = true
			}
		}
		if len(lanes) != 3 {
			t.Fatalf("vertical=%v: rework lanes = %v", vertical, lanes)
		}
	}
}
