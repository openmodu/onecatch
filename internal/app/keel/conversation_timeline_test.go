package keeldesktop

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/window"
	workflows "github.com/openmodu/onecatch/internal/domain/workflows"
	desktop "github.com/openmodu/onecatch/internal/service/desktop"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

func timelineDetail(at time.Time) *desktop.RunDetail {
	return &desktop.RunDetail{Workflow: workflows.Definition{Steps: []workflows.Step{{ID: "a", Name: "学习 APPO 服务实现逻辑"}, {ID: "b", Name: "AKX Monorepo 项目学习"}, {ID: "c", Name: "迁移方案"}}}, StepRuns: []workflows.StepRun{
		{ID: "a", StepID: "a", Attempt: 1, StartedAt: at, FinishedAt: at.Add(20 * time.Second), Status: workflows.StepRunSucceeded},
		{ID: "b", StepID: "b", Attempt: 1, StartedAt: at, FinishedAt: at.Add(4 * time.Second), Status: workflows.StepRunSucceeded},
		{ID: "c", StepID: "c", Attempt: 1, StartedAt: at.Add(20 * time.Second), FinishedAt: at.Add(30 * time.Second)},
	}}
}

func TestWorkflowTimelineKeepsFinishedLaneBlank(t *testing.T) {
	at := time.Date(2026, 10, 12, 10, 0, 0, 0, time.Local)
	entries := []*entry{{key: "prompt", user: true, at: at}, {key: "a1", step: "a", at: at.Add(time.Second)}, {key: "b1", step: "b", at: at.Add(time.Second + 100*time.Millisecond)}, {key: "a2", step: "a", at: at.Add(9 * time.Second)}, {key: "c1", step: "c", at: at.Add(21 * time.Second)}}
	rows := buildWorkflowTimeline(entries, timelineDetail(at))
	if len(rows) != 4 {
		t.Fatalf("got %d rows", len(rows))
	}
	if len(rows[1].columns) != 2 || rows[1].columns[0].stepLabel == "" || rows[1].columns[1].stepLabel == "" {
		t.Fatal("missing lane headers")
	}
	if len(rows[2].columns[0].stack) != 2 || rows[2].columns[0].stack[0].key != "a1" || rows[2].columns[0].stack[1].key != "a2" || rows[2].columns[1].key != "b1" {
		t.Fatal("lanes must flow independently, with the finished lane shorter")
	}
	if rows[3] != entries[4] {
		t.Fatal("serial successor must remain full width")
	}
	entries = append(entries[:3], &entry{key: "followup", user: true, at: at.Add(2 * time.Second)}, entries[3])
	rows = buildWorkflowTimeline(entries, timelineDetail(at))
	if !rows[3].user || len(rows[4].columns) != 2 {
		t.Fatal("user message must split timeline and repeat headers")
	}
}

func TestRenderWorkflowTimeline(t *testing.T) {
	dir := os.Getenv("ONECATCH_KEEL_PREVIEWS")
	if dir == "" {
		t.Skip("preview requested via ONECATCH_KEEL_PREVIEWS")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	a := conversationFixture(t)
	at := time.Date(2026, 10, 12, 10, 0, 0, 0, time.Local)
	a.detail = timelineDetail(at)
	a.transcript = newTranscript()
	for _, e := range []entry{
		{key: "prompt", user: true, text: "学习两个仓库后，整理迁移方案。", at: at},
		{key: "a1", step: "a", kind: agentrun.KindMessage, text: "先梳理服务接口、领域模型和业务流程。", at: at.Add(time.Second)},
		{key: "b1", step: "b", kind: agentrun.KindMessage, text: "已完成仓库约束和 DDD 分层学习。", at: at.Add(time.Second)},
		{key: "a2", step: "a", kind: agentrun.KindMessage, text: "继续追踪持久化与外部依赖。另一流程已结束，这一时间段右列保持空白。", at: at.Add(9 * time.Second)},
		{key: "c1", step: "c", kind: agentrun.KindMessage, text: "两个并行步骤已完成，开始汇总迁移方案。", at: at.Add(21 * time.Second)},
	} {
		a.transcript.items[e.key] = &e
	}
	a.syncMessages()
	root := el.Root(el.ViewFunc(a.Render))
	if err := window.Screenshot(screenshotRoot{core.Func(root.Layout)}, 1288, 900, filepath.Join(dir, "workflow-timeline.png")); err != nil {
		t.Fatal(err)
	}
}

func TestRenderReadableWorkflowTools(t *testing.T) {
	dir := os.Getenv("ONECATCH_KEEL_PREVIEWS")
	if dir == "" {
		t.Skip("optional preview")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	a := conversationFixture(t)
	tools := []*entry{
		{key: "read-visible", kind: agentrun.KindToolUse, text: `Read {"file_path":"internal/sandbox_tool/domain/tool_image.go","offset":65,"limit":10}`, at: time.Now(), settled: true, details: []*entry{{text: "65 func UpdateToolImage(ctx context.Context) error {\n66     return nil\n67 }"}}},
		{key: "shell-visible", kind: agentrun.KindToolUse, text: "go test ./internal/sandbox_tool/...", at: time.Now(), settled: true, details: []*entry{{text: "ok   internal/sandbox_tool/domain   0.021s"}}},
		{key: "legacy-visible", kind: agentrun.KindToolUse, text: "Read", at: time.Now(), settled: true, details: []*entry{{text: "读取结果仍然可以查看，但旧记录未保留文件路径。"}}},
	}
	group := &entry{key: "process-readable-tools", process: tools}
	a.entries = []*entry{group}
	a.toggleConversation(group.key, false)
	for _, tool := range tools {
		a.toggleConversation(tool.key, false)
	}
	root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element {
		return el.Div().WFull().HFull().P(20).Child(a.processGroup(cx, group))
	}))
	if err := window.Screenshot(screenshotRoot{core.Func(root.Layout)}, 920, 850, filepath.Join(dir, "workflow-tools.png")); err != nil {
		t.Fatal(err)
	}
}

func TestWorkflowTimelineCompactsNearbyEventsWithinLanes(t *testing.T) {
	at := time.Date(2026, 10, 12, 10, 0, 0, 0, time.Local)
	detail := timelineDetail(at)
	detail.StepRuns[1].FinishedAt = at.Add(19 * time.Second)
	rows := buildWorkflowTimeline([]*entry{
		{key: "a1", step: "a", at: at.Add(time.Second)},
		{key: "b1", step: "b", at: at.Add(2 * time.Second)},
		{key: "b2", step: "b", at: at.Add(3 * time.Second)},
		{key: "a2", step: "a", at: at.Add(8 * time.Second)},
	}, detail)
	if len(rows) != 2 || len(rows[1].columns[0].stack) != 2 || len(rows[1].columns[1].stack) != 2 {
		t.Fatalf("nearby events forced serial blank rows: %+v", rows)
	}
	if rows[1].columns[0].stack[1].key != "a2" || rows[1].columns[1].stack[1].key != "b2" {
		t.Fatal("lane event order changed")
	}
}

func TestWorkflowTimelineLiveOutputDoesNotAddBlankRows(t *testing.T) {
	at := time.Now()
	detail := timelineDetail(at)
	detail.Active = true
	detail.StepRuns = detail.StepRuns[:2]
	for i := range detail.StepRuns {
		detail.StepRuns[i].FinishedAt = time.Time{}
		detail.StepRuns[i].Status = workflows.StepRunRunning
	}
	entries := []*entry{{key: "a1", step: "a", at: at.Add(time.Second), streaming: true}, {key: "b1", step: "b", at: at.Add(2 * time.Second), streaming: true}}
	before := buildWorkflowTimeline(entries, detail)
	entries = append(entries, &entry{key: "a2", step: "a", at: at.Add(time.Minute), streaming: true}, &entry{key: "b2", step: "b", at: at.Add(2 * time.Minute), streaming: true})
	after := buildWorkflowTimeline(entries, detail)
	if len(before) != 2 || len(after) != 2 || len(after[1].columns[0].stack) != 2 || len(after[1].columns[1].stack) != 2 {
		t.Fatal("live updates added empty time rows instead of appending in each lane")
	}
}
