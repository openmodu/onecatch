package workflows

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

func reviewLoopDefinition() Definition {
	return Definition{
		ID:          "review_loop",
		Name:        "实现与审查",
		EntryStepID: "implement",
		Steps: []Step{
			{
				ID:          "implement",
				Name:        "实现",
				Runtime:     "codex",
				RolePrompt:  "你是负责落地代码的实现者",
				Instruction: "实现需求并完成验证",
				Transitions: map[string]string{
					"ready_for_review": "review",
					"need_human":       TargetPause,
				},
			},
			{
				ID:          "review",
				Name:        "审查",
				Runtime:     "claude",
				RolePrompt:  "你是独立、严格的代码审查者",
				Instruction: "审查实现和测试",
				Transitions: map[string]string{
					"changes_requested": "implement",
					"approved":          TargetDone,
					"invalid":           TargetFail,
				},
			},
		},
	}
}

func TestNormalizeDefaultsAndCopiesTransitions(t *testing.T) {
	input := reviewLoopDefinition()
	got := Normalize(input)
	if got.Policy.MaxTransitions != DefaultMaxTransitions || got.Policy.MaxConsecutiveFailures != DefaultMaxConsecutiveFailures || got.Policy.StepTimeoutSeconds != DefaultStepTimeoutSeconds {
		t.Fatalf("unexpected defaults: %+v", got.Policy)
	}
	got.Steps[0].Transitions["ready_for_review"] = TargetDone
	if input.Steps[0].Transitions["ready_for_review"] != "review" {
		t.Fatal("Normalize mutated the input transitions")
	}
}

func TestValidateReviewLoop(t *testing.T) {
	if err := Validate(reviewLoopDefinition()); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateRejectsInvalidGraphs(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Definition)
		code string
	}{
		{name: "unknown entry", edit: func(d *Definition) { d.EntryStepID = "missing" }, code: "unknown_step"},
		{name: "duplicate step", edit: func(d *Definition) { d.Steps[1].ID = "implement" }, code: "duplicate"},
		{name: "unknown target", edit: func(d *Definition) { d.Steps[0].Transitions["ready_for_review"] = "missing" }, code: "unknown_step"},
		{name: "unreachable step", edit: func(d *Definition) { d.Steps[0].Transitions["ready_for_review"] = TargetDone }, code: "unreachable"},
		{name: "no completion path", edit: func(d *Definition) { d.Steps[1].Transitions["approved"] = "implement" }, code: "no_completion_path"},
		{name: "invalid signal", edit: func(d *Definition) { d.Steps[0].Transitions["Ready Now"] = TargetDone }, code: "invalid_signal"},
		{name: "missing runtime", edit: func(d *Definition) { d.Steps[0].Runtime = "" }, code: "required"},
		{name: "missing role prompt", edit: func(d *Definition) { d.Steps[0].RolePrompt = "" }, code: "required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			def := reviewLoopDefinition()
			tt.edit(&def)
			err := Validate(def)
			var validationErrs ValidationErrors
			if !errors.As(err, &validationErrs) {
				t.Fatalf("Validate() error = %v, want ValidationErrors", err)
			}
			for _, issue := range validationErrs {
				if issue.Code == tt.code {
					return
				}
			}
			t.Fatalf("issues = %+v, want code %q", validationErrs, tt.code)
		})
	}
}

func TestParseOutcome(t *testing.T) {
	tests := []struct {
		name    string
		text    string
		want    Outcome
		wantErr bool
	}{
		{name: "raw", text: `{"signal":"approved","content":"looks good"}`, want: Outcome{Signal: "approved", Content: "looks good"}},
		{name: "fenced", text: "```json\n{\"signal\":\"approved\",\"content\":\"完成\"}\n```", want: Outcome{Signal: "approved", Content: "完成"}},
		{name: "extra prose", text: "done\n{\"signal\":\"approved\",\"content\":\"ok\"}", wantErr: true},
		{name: "unknown field", text: `{"signal":"approved","content":"ok","target":"$done"}`, wantErr: true},
		{name: "missing content", text: `{"signal":"approved","content":""}`, wantErr: true},
		{name: "multiple values", text: `{"signal":"approved","content":"ok"} {}`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseOutcome(tt.text)
			if tt.wantErr {
				if !errors.Is(err, ErrProtocol) {
					t.Fatalf("ParseOutcome() error = %v, want ErrProtocol", err)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("ParseOutcome() = %+v, %v, want %+v", got, err, tt.want)
			}
		})
	}
}

func TestReviewLoopCanReturnAndComplete(t *testing.T) {
	def := reviewLoopDefinition()
	now := time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC)
	run, err := NewRun(def, "run_1", now)
	if err != nil {
		t.Fatal(err)
	}
	run, err = Start(def, run, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}

	steps := []struct {
		outcome Outcome
		step    string
		status  RunStatus
	}{
		{Outcome{Signal: "ready_for_review", Content: "implemented"}, "review", RunRunning},
		{Outcome{Signal: "changes_requested", Content: "add tests"}, "implement", RunRunning},
		{Outcome{Signal: "ready_for_review", Content: "tests added"}, "review", RunRunning},
		{Outcome{Signal: "approved", Content: "approved"}, "review", RunCompleted},
	}
	for i, step := range steps {
		run, err = Advance(def, run, step.outcome, now.Add(time.Duration(i+2)*time.Second))
		if err != nil {
			t.Fatalf("Advance(%d) error = %v", i, err)
		}
		if run.CurrentStepID != step.step || run.Status != step.status {
			t.Fatalf("Advance(%d) run = %+v, want step=%s status=%s", i, run, step.step, step.status)
		}
	}
	if run.TransitionCount != 4 || len(run.History) != 4 || run.CompletedAt.IsZero() {
		t.Fatalf("unexpected completed run: %+v", run)
	}
}

func TestUnknownSignalDoesNotMutateRun(t *testing.T) {
	def := reviewLoopDefinition()
	now := time.Now()
	run, _ := NewRun(def, "run_1", now)
	run, _ = Start(def, run, now)
	want := cloneRun(run)
	_, err := Advance(def, run, Outcome{Signal: "invented", Content: "go elsewhere"}, now)
	var unknown ErrUnknownSignal
	if !errors.As(err, &unknown) {
		t.Fatalf("Advance() error = %v, want ErrUnknownSignal", err)
	}
	if !reflect.DeepEqual(run, want) {
		t.Fatalf("input run mutated: got %+v want %+v", run, want)
	}
}

func TestTerminalTargets(t *testing.T) {
	def := reviewLoopDefinition()
	now := time.Now()
	for _, tt := range []struct {
		signal string
		want   RunStatus
	}{
		{signal: "need_human", want: RunPaused},
	} {
		run, _ := NewRun(def, "run_1", now)
		run, _ = Start(def, run, now)
		got, err := Advance(def, run, Outcome{Signal: tt.signal, Content: "reason"}, now)
		if err != nil || got.Status != tt.want {
			t.Fatalf("Advance(%s) = %+v, %v", tt.signal, got, err)
		}
	}

	run, _ := NewRun(def, "run_2", now)
	run, _ = Start(def, run, now)
	run, _ = Advance(def, run, Outcome{Signal: "ready_for_review", Content: "ready"}, now)
	got, err := Advance(def, run, Outcome{Signal: "invalid", Content: "broken output"}, now)
	if err != nil || got.Status != RunFailed || got.LastError != "broken output" {
		t.Fatalf("fail terminal = %+v, %v", got, err)
	}
}

func TestTransitionLimitPausesAtNextStepAndCanResume(t *testing.T) {
	def := reviewLoopDefinition()
	def.Policy.MaxTransitions = 2
	now := time.Now()
	run, _ := NewRun(def, "run_1", now)
	run, _ = Start(def, run, now)
	run, _ = Advance(def, run, Outcome{Signal: "ready_for_review", Content: "ready"}, now)
	run, err := Advance(def, run, Outcome{Signal: "changes_requested", Content: "fix"}, now)
	if err != nil || run.Status != RunPaused || run.CurrentStepID != "implement" || run.PauseReason != PauseReasonTransitionLimit {
		t.Fatalf("limited run = %+v, %v", run, err)
	}
	run, err = Resume(def, run, now.Add(time.Second))
	if err != nil || run.Status != RunRunning || run.CurrentStepID != "implement" {
		t.Fatalf("resumed run = %+v, %v", run, err)
	}
}

func TestFailureLimitPauses(t *testing.T) {
	def := reviewLoopDefinition()
	def.Policy.MaxConsecutiveFailures = 2
	now := time.Now()
	run, _ := NewRun(def, "run_1", now)
	run, _ = Start(def, run, now)
	run, _ = RecordFailure(def, run, "first", now)
	if run.Status != RunRunning || run.ConsecutiveFailures != 1 {
		t.Fatalf("first failure = %+v", run)
	}
	run, err := RecordFailure(def, run, "second", now)
	if err != nil || run.Status != RunPaused || run.PauseReason != PauseReasonFailureLimit {
		t.Fatalf("second failure = %+v, %v", run, err)
	}
}

func TestPauseAndResumeKeepCurrentStep(t *testing.T) {
	def := reviewLoopDefinition()
	now := time.Now()
	run, _ := NewRun(def, "run_1", now)
	run, _ = Start(def, run, now)
	paused, err := Pause(def, run, "interrupted", "cancelled by user", now.Add(time.Second))
	if err != nil || paused.Status != RunPaused || paused.CurrentStepID != "implement" || paused.PauseReason != "interrupted" {
		t.Fatalf("Pause() = %+v, %v", paused, err)
	}
	resumed, err := Resume(def, paused, now.Add(2*time.Second))
	if err != nil || resumed.Status != RunRunning || resumed.CurrentStepID != "implement" {
		t.Fatalf("Resume() = %+v, %v", resumed, err)
	}
}

func TestCompletedSerialRunCanContinueCurrentStep(t *testing.T) {
	def := reviewLoopDefinition()
	now := time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC)
	run, _ := NewRun(def, "run_completed", now)
	run, _ = Start(def, run, now.Add(time.Second))
	run, _ = Advance(def, run, Outcome{Signal: "ready_for_review", Content: "ready"}, now.Add(2*time.Second))
	run.Sessions["review"] = "review-session"
	run, _ = Advance(def, run, Outcome{Signal: "approved", Content: "done"}, now.Add(3*time.Second))

	resumed, err := Resume(def, run, now.Add(4*time.Second))
	if err != nil || resumed.Status != RunRunning || resumed.CurrentStepID != "review" {
		t.Fatalf("Resume(completed) = %+v, %v", resumed, err)
	}
	if !resumed.CompletedAt.IsZero() || resumed.Sessions["review"] != "review-session" {
		t.Fatalf("completed continuation lost completion/session state: %+v", resumed)
	}
}

func TestCancelReadyOrPausedRun(t *testing.T) {
	def := reviewLoopDefinition()
	now := time.Now()
	ready, _ := NewRun(def, "run_ready", now)
	cancelled, err := Cancel(def, ready, now.Add(time.Second))
	if err != nil || cancelled.Status != RunCancelled {
		t.Fatalf("Cancel(ready) = %+v, %v", cancelled, err)
	}

	running, _ := Start(def, ready, now)
	if _, err := Cancel(def, running, now); !errors.Is(err, ErrInvalidRunState) {
		t.Fatalf("Cancel(running) error = %v, want ErrInvalidRunState", err)
	}
	paused, _ := Pause(def, running, "interrupted", "stopped", now)
	cancelled, err = Cancel(def, paused, now)
	if err != nil || cancelled.Status != RunCancelled || cancelled.PauseReason != "" {
		t.Fatalf("Cancel(paused) = %+v, %v", cancelled, err)
	}
}

func TestRunMustMatchDefinitionID(t *testing.T) {
	def := reviewLoopDefinition()
	run, _ := NewRun(def, "run_1", time.Now())
	def.ID = "different_workflow"
	_, err := Start(def, run, time.Now())
	if !errors.Is(err, ErrRunDefinitionMismatch) {
		t.Fatalf("Start() error = %v, want ErrRunDefinitionMismatch", err)
	}
}

func dagDefinition() Definition {
	return Definition{
		ID: "parallel_review", Name: "Parallel review", Mode: ModeDAG, EntryStepID: "security",
		Steps: []Step{
			{ID: "security", Name: "Security", Runtime: "codex", Sandbox: "read-only", RolePrompt: "Review security", Instruction: "Review", Transitions: map[string]string{"completed": TargetDone}},
			{ID: "tests", Name: "Tests", Runtime: "claude", Sandbox: "read-only", RolePrompt: "Review tests", Instruction: "Review", Transitions: map[string]string{"completed": TargetDone}},
			{ID: "synthesis", Name: "Synthesis", Runtime: "codex", Sandbox: "workspace-write", DependsOn: []string{"security", "tests"}, RolePrompt: "Synthesize", Instruction: "Apply findings", Transitions: map[string]string{"completed": TargetDone}},
		},
	}
}

func TestValidateDAG(t *testing.T) {
	def := dagDefinition()
	if err := Validate(def); err != nil {
		t.Fatalf("Validate(DAG) error = %v", err)
	}
	run, err := NewRun(def, "run_dag", time.Now())
	if err != nil || len(run.Nodes) != 3 || run.Nodes["security"].Status != NodePending {
		t.Fatalf("NewRun(DAG) = %+v, %v", run, err)
	}
}

func TestCompletedDAGContinuationReopensOnlyTerminalNodes(t *testing.T) {
	def := dagDefinition()
	now := time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC)
	run, err := NewRun(def, "run_dag_completed", now)
	if err != nil {
		t.Fatal(err)
	}
	run.Status = RunCompleted
	run.CompletedAt = now.Add(time.Minute)
	run.Nodes["security"] = NodeState{StepID: "security", Status: NodeCompleted, Content: "secure", Attempt: 1}
	run.Nodes["tests"] = NodeState{StepID: "tests", Status: NodeCompleted, Content: "passing", Attempt: 1}
	run.Nodes["synthesis"] = NodeState{StepID: "synthesis", Status: NodeCompleted, Content: "applied", Signal: "completed", Attempt: 1}
	run.Sessions["synthesis"] = "synthesis-session"

	resumed, err := Resume(def, run, now.Add(2*time.Minute))
	if err != nil || resumed.Status != RunRunning || !resumed.CompletedAt.IsZero() {
		t.Fatalf("Resume(completed DAG) = %+v, %v", resumed, err)
	}
	if resumed.Nodes["security"].Status != NodeCompleted || resumed.Nodes["tests"].Status != NodeCompleted {
		t.Fatalf("upstream nodes were reopened: %+v", resumed.Nodes)
	}
	terminal := resumed.Nodes["synthesis"]
	if terminal.Status != NodePending || terminal.Content != "" || terminal.Signal != "" || terminal.Attempt != 1 {
		t.Fatalf("terminal node was not reset for continuation: %+v", terminal)
	}
	if resumed.Sessions["synthesis"] != "synthesis-session" {
		t.Fatalf("terminal session was not preserved: %+v", resumed.Sessions)
	}
}

func TestValidateDAGRejectsCycleAndParallelWrites(t *testing.T) {
	cycle := dagDefinition()
	cycle.Steps[0].DependsOn = []string{"synthesis"}
	assertValidationCode(t, Validate(cycle), "dag_cycle")

	writes := dagDefinition()
	writes.Steps[0].Sandbox = "workspace-write"
	writes.Steps[1].Sandbox = "workspace-write"
	assertValidationCode(t, Validate(writes), "parallel_write_conflict")

	remoteWrite := dagDefinition()
	remoteWrite.Steps[2].WorkerID = "mac-mini"
	if err := Validate(remoteWrite); err != nil {
		t.Fatalf("serialized remote write should be valid, got %v", err)
	}
}

func TestValidateAllowsRemoteWorkspaceWriteButRejectsFullSandbox(t *testing.T) {
	serialWrite := reviewLoopDefinition()
	serialWrite.Steps[0].WorkerID = "mac-mini"
	serialWrite.Steps[0].Sandbox = "workspace-write"
	if err := Validate(serialWrite); err != nil {
		t.Fatalf("remote workspace-write serial step should be valid, got %v", err)
	}

	serialWrite.Steps[0].Sandbox = "full"
	assertValidationCode(t, Validate(serialWrite), "remote_full_unsupported")
}

func assertValidationCode(t *testing.T, err error, code string) {
	t.Helper()
	var issues ValidationErrors
	if !errors.As(err, &issues) {
		t.Fatalf("error = %v, want ValidationErrors", err)
	}
	for _, issue := range issues {
		if issue.Code == code {
			return
		}
	}
	t.Fatalf("issues = %+v, want %s", issues, code)
}
