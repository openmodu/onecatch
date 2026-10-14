package keeldesktop

import (
	"testing"
	"time"

	"github.com/dyike/keel/ui/el"
	workflows "github.com/openmodu/onecatch/internal/domain/workflows"
	desktop "github.com/openmodu/onecatch/internal/service/desktop"
)

func workflowChangePanelFixture(t *testing.T, status workflows.RunStatus, active bool) *app {
	t.Helper()
	a := sessionPanelFixture(t)
	a.workbench.Tab = "status"
	a.detail.Workflow.ID = "editable"
	a.detail.Run.Status = status
	a.detail.Active = active
	a.workbench.WorkflowCheck = &sessionWorkflowState{
		RunID:     a.detail.Run.ID,
		Selection: a.selection,
		CheckedAt: time.Now(),
		Result: desktop.RunWorkflowCheck{
			Changed: true,
			Changes: []string{"步骤「实现功能」：提示词"},
		},
	}
	return a
}

func TestSessionWorkflowUpdateRequiresPausedAndInactive(t *testing.T) {
	for _, test := range []struct {
		name           string
		status         workflows.RunStatus
		active         bool
		updateDisabled bool
		pauseVisible   bool
	}{
		{name: "running", status: workflows.RunRunning, active: true, updateDisabled: true, pauseVisible: true},
		{name: "stopping", status: workflows.RunPaused, active: true, updateDisabled: true, pauseVisible: true},
		{name: "paused", status: workflows.RunPaused, updateDisabled: false},
		{name: "completed", status: workflows.RunCompleted, updateDisabled: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			a := workflowChangePanelFixture(t, test.status, test.active)
			router, _, _ := alignmentRouter(el.Root(a), 1280, 800, 1)
			nodes := router.AppendSemantics(nil)
			update, ok := alignmentButton(nodes, "更新工作流配置")
			if !ok {
				t.Fatal("missing workflow update action")
			}
			if update.Disabled != test.updateDisabled {
				t.Fatalf("update disabled = %v, want %v", update.Disabled, test.updateDisabled)
			}
			_, pauseVisible := alignmentButton(nodes, "暂停")
			if pauseVisible != test.pauseVisible {
				t.Fatalf("pause visible = %v, want %v", pauseVisible, test.pauseVisible)
			}
		})
	}
}

func TestSessionWorkflowPanelHidesForDirectAgent(t *testing.T) {
	a := workflowChangePanelFixture(t, workflows.RunPaused, false)
	a.detail.Workflow.ID = "single_agent"
	router, _, _ := alignmentRouter(el.Root(a), 1280, 800, 1)
	if _, ok := alignmentButton(router.AppendSemantics(nil), "更新工作流配置"); ok {
		t.Fatal("direct Agent session exposed workflow update action")
	}
}
