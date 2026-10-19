package workflows

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// pipeline is design → implement → review → e2e → pr, with review and e2e able
// to send work back to implement, and review able to send it to design.
func pipeline() Definition {
	return Definition{
		ID:   "pipeline",
		Name: "设计到 PR",
		Steps: []Step{
			{ID: "design", Name: "架构", Runtime: "claude", Model: "opus", Sandbox: "read-only"},
			{ID: "implement", Name: "实现", Runtime: "codex", Sandbox: "workspace-write"},
			{ID: "review", Name: "审查", Runtime: "claude", Model: "sonnet", Sandbox: "read-only"},
			{ID: "e2e", Name: "E2E", Kind: StepCommand, Command: "npm run e2e"},
			{ID: "pr", Name: "PR", Runtime: "modu", Sandbox: "workspace-write"},
		},
		Edges: []Edge{
			{From: "design", To: "implement"},
			{From: "implement", To: "review"},
			{From: "review", To: "e2e"},
			{From: "e2e", To: "pr"},
			{From: "review", To: "implement", Kind: EdgeRework},
			{From: "review", To: "design", Kind: EdgeRework},
			{From: "e2e", To: "implement", Kind: EdgeRework, MaxRounds: 1},
		},
	}
}

var now = time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)

func startedRun(t *testing.T, def Definition) Run {
	t.Helper()
	run, err := NewRun(def, "run_1", now)
	if err != nil {
		t.Fatalf("NewRun() error = %v", err)
	}
	run, err = Start(def, run, now)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	return run
}

func pass(t *testing.T, def Definition, run Run, stepID string) Run {
	t.Helper()
	out, err := ApplyOutcome(def, run, stepID, Outcome{Verdict: VerdictPass, Summary: stepID + " done"}, now)
	if err != nil {
		t.Fatalf("ApplyOutcome(%s) error = %v", stepID, err)
	}
	return out
}

func readyIDs(def Definition, run Run) string {
	var ids []string
	for _, step := range ReadySteps(def, run) {
		ids = append(ids, step.ID)
	}
	return strings.Join(ids, ",")
}

func TestValidatePipeline(t *testing.T) {
	if err := Validate(pipeline()); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestWorkflowStepReasoningEffortValidation(t *testing.T) {
	d := pipeline()
	d.Steps[0].ReasoningEffort = " HIGH "
	normalized := Normalize(d)
	if normalized.Steps[0].ReasoningEffort != "high" || Validate(normalized) != nil {
		t.Fatal("valid effort was not normalized")
	}
	d.Steps[0].ReasoningEffort = "unsupported"
	if Validate(d) == nil {
		t.Fatal("unsupported effort accepted")
	}
	d.Steps[0].Runtime, d.Steps[0].ReasoningEffort = "modu", "high"
	if Validate(d) == nil {
		t.Fatal("effort accepted for a runtime without reasoning controls")
	}
}

func TestValidateRejectsBadGraphs(t *testing.T) {
	cases := map[string]func(*Definition){
		"cycle": func(d *Definition) {
			d.Edges = append(d.Edges, Edge{From: "pr", To: "design"})
		},
		"rework_not_upstream": func(d *Definition) {
			d.Edges = append(d.Edges, Edge{From: "design", To: "pr", Kind: EdgeRework})
		},
		"unknown_step": func(d *Definition) {
			d.Edges = append(d.Edges, Edge{From: "design", To: "ghost"})
		},
		"parallel_write_conflict": func(d *Definition) {
			d.Steps = append(d.Steps, Step{ID: "docs", Name: "文档", Runtime: "codex", Sandbox: "workspace-write"})
		},
		"command_local_only": func(d *Definition) {
			d.Steps[3].WorkerID = "remote_box"
		},
		"required": func(d *Definition) {
			d.Steps[3].Command = " "
		},
	}
	for code, mutate := range cases {
		t.Run(code, func(t *testing.T) {
			def := pipeline()
			mutate(&def)
			var issues ValidationErrors
			if err := Validate(def); !errors.As(err, &issues) {
				t.Fatalf("Validate() error = %v, want issues", err)
			}
			for _, issue := range issues {
				if issue.Code == code {
					return
				}
			}
			t.Fatalf("issues = %+v, want code %s", issues, code)
		})
	}
}

func TestSingleStepNeedsNoPromptOrEdges(t *testing.T) {
	def := Definition{ID: "single_agent", Name: "单 Agent", Steps: []Step{{ID: "execute", Name: "执行", Runtime: "codex"}}}
	if err := Validate(def); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestPipelineRunsInDependencyOrder(t *testing.T) {
	def := pipeline()
	run := startedRun(t, def)
	for _, want := range []string{"design", "implement", "review", "e2e", "pr"} {
		if got := readyIDs(def, run); got != want {
			t.Fatalf("ready = %q, want %q", got, want)
		}
		run = pass(t, def, run, want)
	}
	if !AllCompleted(def, run) {
		t.Fatal("pipeline did not complete")
	}
}

func TestReworkReopensOnlyThePathAndDeliversFeedback(t *testing.T) {
	def := pipeline()
	def.Steps = append(def.Steps, Step{ID: "notes", Name: "记录", Runtime: "claude", Sandbox: "read-only"})
	def.Edges = append(def.Edges, Edge{From: "implement", To: "notes"})
	run := startedRun(t, def)
	for _, id := range []string{"design", "implement", "notes"} {
		run = pass(t, def, run, id)
	}
	run, err := ApplyOutcome(def, run, "review", Outcome{Verdict: VerdictRework, Summary: "缺测试", Feedback: "补上并发测试", Target: "implement"}, now)
	if err != nil {
		t.Fatalf("ApplyOutcome(rework) error = %v", err)
	}
	if run.Nodes["design"].Status != NodeCompleted || run.Nodes["notes"].Status != NodeCompleted {
		t.Fatalf("rework reopened steps outside the path: %+v", run.Nodes)
	}
	if run.Nodes["implement"].Status != NodePending || run.Nodes["review"].Status != NodePending {
		t.Fatalf("rework did not reopen the path: %+v", run.Nodes)
	}
	inbox := run.Nodes["implement"].Inbox
	if len(inbox) != 1 || inbox[0].From != "review" || inbox[0].Text != "补上并发测试" {
		t.Fatalf("implement inbox = %+v", inbox)
	}
	if got := readyIDs(def, run); got != "implement" {
		t.Fatalf("ready = %q, want implement", got)
	}
	run = pass(t, def, run, "implement")
	if len(run.Nodes["implement"].Inbox) != 0 {
		t.Fatal("inbox was not cleared after the step passed")
	}
	if run.ReworkRounds[EdgeKey("review", "implement")] != 1 {
		t.Fatalf("rework rounds = %+v", run.ReworkRounds)
	}
}

func TestReworkNeedsTargetWhenSeveralAreAllowed(t *testing.T) {
	def := pipeline()
	if err := CheckOutcome(def, "review", Outcome{Verdict: VerdictRework, Summary: "x"}); err == nil {
		t.Fatal("review has two rework targets; an implicit target must be rejected")
	}
	if err := CheckOutcome(def, "e2e", Outcome{Verdict: VerdictRework, Summary: "x"}); err != nil {
		t.Fatalf("e2e has one rework target: %v", err)
	}
	if err := CheckOutcome(def, "implement", Outcome{Verdict: VerdictRework, Summary: "x"}); err == nil {
		t.Fatal("implement has no rework edge")
	}
}

func TestReworkLimitPausesForHuman(t *testing.T) {
	def := pipeline()
	run := startedRun(t, def)
	for _, id := range []string{"design", "implement", "review"} {
		run = pass(t, def, run, id)
	}
	fail := Outcome{Verdict: VerdictRework, Summary: "e2e failed", Feedback: "login spec times out"}
	run, err := ApplyOutcome(def, run, "e2e", fail, now)
	if err != nil || run.Status != RunRunning {
		t.Fatalf("first rework: status=%s err=%v", run.Status, err)
	}
	for _, id := range []string{"implement", "review"} {
		run = pass(t, def, run, id)
	}
	run, err = ApplyOutcome(def, run, "e2e", fail, now)
	if err != nil {
		t.Fatalf("second rework error = %v", err)
	}
	if run.Status != RunPaused || run.PauseReason != PauseReasonReworkLimit || run.Nodes["e2e"].Status != NodePaused {
		t.Fatalf("status=%s reason=%s node=%+v", run.Status, run.PauseReason, run.Nodes["e2e"])
	}
}

func TestAskHumanThenResumeDeliversAnswer(t *testing.T) {
	def := pipeline()
	run := startedRun(t, def)
	run, err := ApplyOutcome(def, run, "design", Outcome{Verdict: VerdictAskHuman, Summary: "用 Postgres 还是 SQLite？"}, now)
	if err != nil || run.Status != RunPaused || run.LastError != "用 Postgres 还是 SQLite？" {
		t.Fatalf("status=%s lastError=%q err=%v", run.Status, run.LastError, err)
	}
	run, err = Resume(def, run, "", "SQLite", now)
	if err != nil {
		t.Fatalf("Resume() error = %v", err)
	}
	if got := readyIDs(def, run); got != "design" {
		t.Fatalf("ready = %q", got)
	}
	if inbox := run.Nodes["design"].Inbox; len(inbox) != 1 || inbox[0].From != MessageFromHuman || inbox[0].Text != "SQLite" {
		t.Fatalf("design inbox = %+v", inbox)
	}
}

func TestResumeFromStepReopensItAndEverythingAfter(t *testing.T) {
	def := pipeline()
	run := startedRun(t, def)
	for _, id := range []string{"design", "implement", "review", "e2e", "pr"} {
		run = pass(t, def, run, id)
	}
	run.Status = RunCompleted
	run, err := Resume(def, run, "review", "再严格检查一次错误处理", now)
	if err != nil {
		t.Fatalf("Resume() error = %v", err)
	}
	for id, want := range map[string]NodeStatus{"design": NodeCompleted, "implement": NodeCompleted, "review": NodePending, "e2e": NodePending, "pr": NodePending} {
		if run.Nodes[id].Status != want {
			t.Fatalf("%s = %s, want %s", id, run.Nodes[id].Status, want)
		}
	}
	if len(run.Nodes["review"].Inbox) != 1 || len(run.Nodes["e2e"].Inbox) != 0 {
		t.Fatal("resume message must go only to the chosen step")
	}
}

func TestResumeCompletedRunReopensSinks(t *testing.T) {
	def := pipeline()
	run := startedRun(t, def)
	for _, id := range []string{"design", "implement", "review", "e2e", "pr"} {
		run = pass(t, def, run, id)
	}
	run.Status = RunCompleted
	run, err := Resume(def, run, "", "PR 标题改成中文", now)
	if err != nil {
		t.Fatalf("Resume() error = %v", err)
	}
	if got := readyIDs(def, run); got != "pr" {
		t.Fatalf("ready = %q, want pr", got)
	}
}

func TestFailuresRetryThenPause(t *testing.T) {
	def := pipeline()
	def.Policy.MaxConsecutiveFailures = 2
	run := startedRun(t, def)
	run = RecordStepFailure(def, run, "design", "exit 1", now)
	if run.Status != RunRunning || run.Nodes["design"].Status != NodePending {
		t.Fatalf("first failure should retry: %+v", run.Nodes["design"])
	}
	run = RecordStepFailure(def, run, "design", "exit 1", now)
	if run.Status != RunPaused || run.PauseReason != PauseReasonFailureLimit {
		t.Fatalf("second failure should pause: status=%s reason=%s", run.Status, run.PauseReason)
	}
}

func TestParseOutcome(t *testing.T) {
	got, err := ParseOutcome("改好了。\n{\"verdict\":\"rework\",\"summary\":\"测试缺失\",\"target\":\"implement\"}")
	if err != nil {
		t.Fatalf("ParseOutcome() error = %v", err)
	}
	if got.Verdict != VerdictRework || got.Feedback != "测试缺失" || got.Target != "implement" {
		t.Fatalf("outcome = %+v", got)
	}
	for _, bad := range []string{"", "done", `{"verdict":"approve","summary":"x"}`, `{"verdict":"pass"}`, `{"verdict":"pass","summary":"x"} trailing`} {
		if _, err := ParseOutcome(bad); !errors.Is(err, ErrProtocol) {
			t.Fatalf("ParseOutcome(%q) error = %v, want ErrProtocol", bad, err)
		}
	}
}
