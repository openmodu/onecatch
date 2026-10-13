package keeldesktop

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/dyike/keel/gio/io/key"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/window"
	domaintasks "github.com/openmodu/onecatch/internal/domain/tasks"
	domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"
	desktopservice "github.com/openmodu/onecatch/internal/service/desktop"
)

func TestMentionTrigger(t *testing.T) {
	for _, c := range []struct {
		text  string
		caret int
		query string
		ok    bool
	}{
		{"@", 1, "", true},
		{"请 @审", 4, "审", true},
		{"@review 再看看", 7, "review", true},
		{"mail@host", 9, "", false},
		{"@review 再看看", 11, "", false},
	} {
		_, query, ok := mentionTrigger(c.text, c.caret)
		if ok != c.ok || query != c.query {
			t.Fatalf("mentionTrigger(%q, %d) = %q, %v", c.text, c.caret, query, ok)
		}
	}
}

func TestSplitStepMentionsAndEarliestStep(t *testing.T) {
	d := starterWorkflow("design_to_pr")
	steps, rest := splitStepMentions(d, "@review @implement 补上并发测试，见 a@b.com @nobody")
	if !reflect.DeepEqual(steps, []string{"review", "implement"}) || rest != "补上并发测试，见 a@b.com @nobody" {
		t.Fatalf("steps=%v rest=%q", steps, rest)
	}
	if got := earliestStep(d, steps); got != "implement" {
		t.Fatalf("earliest = %q, want implement", got)
	}
}

func TestPausedRunNoticeNamesWhoGetsTheMessage(t *testing.T) {
	detail := pausedPipelineDetail()
	detail.Workflow.ID = "design_to_pr"
	reason, receivers := pausedRunNotice(detail.Workflow, detail.Run)
	if !strings.Contains(reason, "E2E 测试") || !strings.Contains(reason, "实现") || !reflect.DeepEqual(receivers, []string{"E2E 测试"}) {
		t.Fatalf("reason=%q receivers=%v", reason, receivers)
	}
	detail.Run.PauseReason = domainworkflows.PauseReasonAskHuman
	detail.Run.Nodes["e2e"] = domainworkflows.NodeState{StepID: "e2e", Status: domainworkflows.NodePaused, Verdict: domainworkflows.VerdictAskHuman, Summary: "要不要跳过 flaky 用例？"}
	if reason, _ := pausedRunNotice(detail.Workflow, detail.Run); !strings.Contains(reason, "在问你：要不要跳过 flaky 用例？") {
		t.Fatalf("ask-human reason = %q", reason)
	}
}

func TestComposerAtSuggestsWorkflowsThenStepsOfARun(t *testing.T) {
	a := conversationFixture(t)
	a.task, a.detail, a.runID = nil, nil, ""
	a.prompt = "修复登录 @设计"
	caret := len([]rune(a.prompt))
	focus := false
	root := el.Root(el.ViewFunc(func(cx *el.Context) el.Element {
		if focus {
			cx.Focus("prompt")
			cx.SelectInput("prompt", caret, caret)
			focus = false
		}
		return el.Div().WFull().HFull().Child(el.Div().Grow(), a.composerContext(cx), a.composer(cx))
	}))
	router, frame, _ := alignmentRouter(root, 900, 700, 1)
	focus = true
	frame()
	frame()
	frame()
	if !a.mentionPicker.open || len(a.mentionPicker.matches) != 1 || a.mentionPicker.matches[0].ID != "design_to_pr" {
		t.Fatalf("@ did not suggest workflows: %+v", a.mentionPicker)
	}
	router.Queue(key.Event{Name: key.NameReturn, State: key.Press}, key.Event{Name: key.NameReturn, State: key.Release})
	frame()
	frame()
	if a.workflow.Value() != "design_to_pr" || a.prompt != "修复登录 " {
		t.Fatalf("workflow=%q prompt=%q", a.workflow.Value(), a.prompt)
	}
	if _, ok := alignmentNode(router.AppendSemantics(nil), "改为单 Agent", true); !ok {
		t.Fatal("the chosen workflow's chart was not shown above the composer")
	}

	// Inside a workflow run, @ lists that run's steps instead.
	detail := pausedPipelineDetail()
	detail.Workflow.ID = "design_to_pr"
	a.task = &domaintasks.Task{ID: "task_preview", WorkflowID: detail.Workflow.ID}
	a.detail, a.runID = detail, detail.Run.ID
	a.prompt = "@实"
	caret = 2
	focus = true
	frame()
	frame()
	frame()
	if !a.mentionPicker.open || len(a.mentionPicker.matches) != 1 || !a.mentionPicker.matches[0].Step || a.mentionPicker.matches[0].ID != "implement" {
		t.Fatalf("@ did not suggest the run's steps: %+v", a.mentionPicker.matches)
	}
	router.Queue(key.Event{Name: key.NameReturn, State: key.Press}, key.Event{Name: key.NameReturn, State: key.Release})
	frame()
	frame()
	if a.prompt != "@implement " {
		t.Fatalf("step mention prompt = %q", a.prompt)
	}
}

func TestRenderComposerWorkflowPreview(t *testing.T) {
	dir := os.Getenv("ONECATCH_KEEL_PREVIEWS")
	if dir == "" {
		t.Skip("set ONECATCH_KEEL_PREVIEWS to render the composer")
	}
	a := dagTestApp(t)
	a.page = ""
	view := el.ViewFunc(func(cx *el.Context) el.Element {
		return el.Div().WFull().HFull().P(24).Gap(10).Child(el.Div().Grow(), a.composerContext(cx), a.composer(cx))
	})
	a.selectLocalConversation("codex", "design_to_pr")
	a.prompt = "修复登录超时"
	if err := window.Screenshot(el.Root(view), 960, 640, filepath.Join(dir, "composer-workflow.png")); err != nil {
		t.Fatal(err)
	}
	detail := pausedPipelineDetail()
	a.task = &domaintasks.Task{ID: "task_preview", WorkflowID: detail.Workflow.ID, Sandbox: "workspace-write"}
	a.detail, a.runID, a.prompt = detail, detail.Run.ID, ""
	if err := window.Screenshot(el.Root(view), 960, 420, filepath.Join(dir, "composer-paused.png")); err != nil {
		t.Fatal(err)
	}
}

func TestConversationLabelsStepsAndShowsHandOffs(t *testing.T) {
	a := conversationFixture(t)
	detail := pausedPipelineDetail()
	at := func(s int) string { return time.Date(2026, 10, 11, 10, 0, s, 0, time.UTC).Format(time.RFC3339Nano) }
	detail.StepRuns = []domainworkflows.StepRun{{ID: "sr1", StepID: "implement"}, {ID: "sr2", StepID: "review"}, {ID: "sr3", StepID: "implement"}}
	detail.RuntimeEvents = []desktopservice.RuntimeEventView{
		{StepRunID: "sr1", Seq: 1, Kind: "message", Text: "实现完成", At: at(1)},
		{StepRunID: "sr2", Seq: 1, Kind: "message", Text: "缺少并发测试", At: at(2)},
		{StepRunID: "sr3", Seq: 1, Kind: "message", Text: "已补测试", At: at(4)},
	}
	detail.Events = []desktopservice.WorkflowEventView{{Seq: 9, Type: "step.completed", StepID: "review", Payload: `{"verdict":"rework","target":"implement","summary":"缺测试","feedback":"补上并发测试\n细节见上"}`, At: at(3)}}
	a.task = &domaintasks.Task{ID: "task_preview", WorkflowID: detail.Workflow.ID}
	a.detail = detail
	a.transcript = newTranscript()
	a.transcript.detail(*detail)
	a.syncMessages()
	var got []string
	for _, e := range a.entries {
		if e.user {
			continue
		}
		if e.stepLabel != "" && !strings.Contains(e.stepContext, detail.Workflow.Name) {
			t.Fatalf("workflow name missing: %q", e.stepContext)
		}
		got = append(got, e.stepLabel+"|"+e.text)
	}
	want := []string{"实现 · codex|实现完成", "代码审查 · claude · sonnet|缺少并发测试", "|↩ 「代码审查」退回给「实现」：补上并发测试 …", "实现 · codex|已补测试"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("conversation rows = %q\nwant %q", got, want)
	}
}
