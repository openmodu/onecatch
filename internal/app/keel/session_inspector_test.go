package keeldesktop

import (
	"fmt"
	"github.com/dyike/keel/ui/el"
	workflows "github.com/openmodu/onecatch/internal/domain/workflows"
	desktop "github.com/openmodu/onecatch/internal/service/desktop"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSessionUsageAndRecoveryParity(t *testing.T) {
	if normalizedSessionInput(100, 700, 200) != 1000 || normalizedSessionInput(1000, 700, 200) != 1000 || sessionCacheRate(1000, 700) != "70.0%" || sessionCacheRate(0, 0) != "—" {
		t.Fatal("usage normalization mismatch")
	}
	for runtime, want := range map[string]string{"codex": "codex resume abc-123", "claude": "claude --resume abc-123", "modu": "modu_code --resume abc-123", "pi": "pi --session abc-123", "grok": "grok --resume abc-123", "trae": "traecli resume abc-123"} {
		if got := runtimeResumeCommand(runtime, "abc-123"); got != want {
			t.Fatalf("%s: %s", runtime, got)
		}
	}
	if runtimeResumeCommand("codex", "id; touch /tmp/test") != "" {
		t.Fatal("unsafe ID accepted")
	}
	d := &desktop.RunDetail{Workflow: workflows.Definition{Steps: []workflows.Step{{ID: "code", Runtime: "codex"}}}, StepRuns: []workflows.StepRun{{StepID: "code", SessionIDBefore: "old"}, {StepID: "code", SessionIDAfter: "latest"}}}
	sessions := recoverySessions(d)
	if len(sessions) != 1 || sessions[0].ID != "latest" {
		t.Fatalf("missing fallback: %+v", sessions)
	}
	d.Run.Sessions = map[string]string{"code": "current"}
	if recoverySessions(d)[0].ID != "current" {
		t.Fatal("run session did not take precedence")
	}
}
func TestEventNavigatorKeepsHistoricalPoint(t *testing.T) {
	s := eventNavigator{}
	events := []desktop.WorkflowEventView{{Seq: 1}, {Seq: 5}, {Seq: 9}}
	if s.index("run", events) != 2 {
		t.Fatal("should begin at latest")
	}
	s.selectIndex(events, 1)
	events = append(events, desktop.WorkflowEventView{Seq: 12})
	if s.index("run", events) != 1 || s.Following {
		t.Fatal("new event moved historical selection")
	}
	s.Following = true
	if s.index("run", events) != 3 {
		t.Fatal("latest did not follow")
	}
	if s.index("new", events[:1]) != 0 {
		t.Fatal("new session kept stale selection")
	}
}
func sessionPanelFixture(t *testing.T) *app {
	a := conversationFixture(t)
	a.terminal.Open = false
	a.workbench.Visible = true
	a.workbench.Width = 520
	a.detail.Workflow = workflows.Definition{Steps: []workflows.Step{{ID: "code", Name: "实现功能", Runtime: "codex"}}}
	a.detail.Run.Sessions = map[string]string{"code": "019aa23b-1234-5678-90ab-cdef12345678"}
	a.detail.StepRuns = []workflows.StepRun{{StepID: "code", InputTokens: 12000, CachedInputTokens: 9000, CacheCreationInputTokens: 1000, OutputTokens: 1800, ReasoningOutputTokens: 600, DurationMS: 32100}}
	for i := 1; i <= 6; i++ {
		a.detail.Events = append(a.detail.Events, desktop.WorkflowEventView{Seq: int64(i), StepID: "code", Type: "step.completed", At: fmt.Sprintf("2026-10-11T00:01:%02d+08:00", i), Payload: fmt.Sprintf(`{"step":"code","attempt":%d,"signal":"completed","durationMs":32100}`, i)})
	}
	return a
}
func TestSessionMenuAndEventNavigation(t *testing.T) {
	a := sessionPanelFixture(t)
	a.workbench.Visible = false
	router, frame, click := alignmentRouter(el.Root(a), 1280, 800, 1)
	press := func(label string) {
		t.Helper()
		node, ok := alignmentNode(router.AppendSemantics(nil), label, true)
		if !ok {
			t.Fatalf("missing %s", label)
		}
		click(node.Bounds)
		frame()
		frame()
	}
	press("更多操作")
	press("Session 状态")
	if a.workbench.Tab != "status" || !a.workbench.Visible {
		t.Fatal("status menu")
	}
	press("更多操作")
	press("事件时间轴")
	press("上一条")
	if a.workbench.Events.Seq != 5 {
		t.Fatal("previous event")
	}
	press("下一条")
	if a.workbench.Events.Seq != 6 {
		t.Fatal("next event")
	}
	press("上一条")
	press("最新")
	if !a.workbench.Events.Following || a.workbench.Events.Seq != 6 {
		t.Fatal("latest event")
	}
}
func TestRenderSessionPanels(t *testing.T) {
	dir := os.Getenv("ONECATCH_KEEL_PREVIEWS")
	if dir == "" {
		t.Skip("preview not requested")
	}
	defer applyPalette(false)
	for _, dark := range []bool{false, true} {
		for _, panel := range []string{"status", "events"} {
			a := sessionPanelFixture(t)
			a.dark = dark
			applyPalette(dark)
			a.workbench.Tab = panel
			if err := screenshotWithFocus(el.Root(a), 1280, 800, filepath.Join(dir, fmt.Sprintf("session-%s-%v.png", panel, dark))); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestSessionDurationDoesNotTickAfterCompletion(t *testing.T) {
	start := time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)
	d := &desktop.RunDetail{}
	d.Run.Status = workflows.RunCompleted
	d.Run.CurrentStepID = "code"
	d.StepRuns = []workflows.StepRun{
		{StepID: "code", Status: workflows.StepRunRunning, StartedAt: start},
		{StepID: "code", StartedAt: start, FinishedAt: start.Add(30 * time.Second)},
		{StepID: "code", DurationMS: 1200},
	}
	for _, now := range []time.Time{start.Add(time.Hour), start.Add(10 * 24 * time.Hour)} {
		if got := sessionDuration(d, now); got != 31200 {
			t.Fatalf("completed duration grew: %d", got)
		}
	}
	d.Run.Status = workflows.RunRunning
	d.StepRuns = append(d.StepRuns, workflows.StepRun{StepID: "code", Status: workflows.StepRunRunning, StartedAt: start.Add(time.Minute)})
	if got := sessionDuration(d, start.Add(70*time.Second)); got != 41200 {
		t.Fatalf("active duration = %d", got)
	}
	d.StepRuns[len(d.StepRuns)-1].FinishedAt = start.Add(65 * time.Second)
	if got := sessionDuration(d, start.Add(time.Hour)); got != 36200 {
		t.Fatalf("finished attempt kept ticking: %d", got)
	}
	if sessionNumber(13087828) != "13,087,828" || sessionElapsed(125000) != "2 分 5 秒" {
		t.Fatal("metric formatting")
	}
}
