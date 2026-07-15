package workflows

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/openmodu/oneshot/internal/agentrun"
	localdata "github.com/openmodu/oneshot/internal/data/local"
	domaintasks "github.com/openmodu/oneshot/internal/domain/tasks"
	domainworkflows "github.com/openmodu/oneshot/internal/domain/workflows"
	domainworkspaces "github.com/openmodu/oneshot/internal/domain/workspaces"
	"github.com/openmodu/oneshot/internal/workspacelock"
)

type engineScript struct {
	runtime agentrun.Runtime
	result  agentrun.Result
	err     error
}

type scriptedEngine struct {
	scripts   []engineScript
	calls     []agentrun.Request
	available map[agentrun.Runtime]bool
}

func (e *scriptedEngine) Available(runtime agentrun.Runtime) bool {
	return e.available[runtime]
}

func (e *scriptedEngine) Run(_ context.Context, request agentrun.Request, sink agentrun.Sink) (agentrun.Result, error) {
	e.calls = append(e.calls, request)
	if len(e.scripts) == 0 {
		return agentrun.Result{}, errors.New("unexpected engine call")
	}
	script := e.scripts[0]
	e.scripts = e.scripts[1:]
	if script.runtime != "" && script.runtime != request.Runtime {
		return agentrun.Result{}, fmt.Errorf("runtime = %s, want %s", request.Runtime, script.runtime)
	}
	sink(agentrun.Event{Kind: agentrun.KindMessage, Text: "streamed", At: time.Now()})
	return script.result, script.err
}

type fixedGitInspector struct {
	mu    sync.Mutex
	calls int
}

func (g *fixedGitInspector) Inspect(context.Context, string) (domainworkspaces.GitSnapshot, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.calls++
	return domainworkspaces.GitSnapshot{IsRepo: true, Head: "abc", Status: " M main.go", DiffStat: "main.go | 1 +"}, nil
}

func TestExecuteTaskRunsReviewLoopAndResumesEachStepSession(t *testing.T) {
	definition := reviewLoop()
	engine := &scriptedEngine{
		available: map[agentrun.Runtime]bool{agentrun.RuntimeCodex: true, agentrun.RuntimeClaude: true},
		scripts: []engineScript{
			{runtime: agentrun.RuntimeCodex, result: success(`{"signal":"ready_for_review","content":"implemented"}`, "impl-1")},
			{runtime: agentrun.RuntimeClaude, result: success(`{"signal":"changes_requested","content":"add tests"}`, "review-1")},
			{runtime: agentrun.RuntimeCodex, result: success(`{"signal":"ready_for_review","content":"tests added"}`, "impl-2")},
			{runtime: agentrun.RuntimeClaude, result: success(`{"signal":"approved","content":"looks good"}`, "review-2")},
		},
	}
	usecase, store, task := setupUsecase(t, definition, engine)

	run, err := usecase.ExecuteTask(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != domainworkflows.RunCompleted || run.TransitionCount != 4 {
		t.Fatalf("run = %+v", run)
	}
	if len(engine.calls) != 4 {
		t.Fatalf("engine calls = %d", len(engine.calls))
	}
	wantRuntimes := []agentrun.Runtime{agentrun.RuntimeCodex, agentrun.RuntimeClaude, agentrun.RuntimeCodex, agentrun.RuntimeClaude}
	for i, runtime := range wantRuntimes {
		if engine.calls[i].Runtime != runtime {
			t.Fatalf("call %d runtime = %s, want %s", i, engine.calls[i].Runtime, runtime)
		}
	}
	if engine.calls[2].ResumeSessionID != "impl-1" || engine.calls[3].ResumeSessionID != "review-1" {
		t.Fatalf("resume sessions = %q, %q", engine.calls[2].ResumeSessionID, engine.calls[3].ResumeSessionID)
	}
	if engine.calls[0].Sandbox != agentrun.SandboxWorkspaceWrite {
		t.Fatalf("sandbox = %s, want workspace-write clamp", engine.calls[0].Sandbox)
	}
	if !strings.Contains(engine.calls[0].Prompt, "ready_for_review") || !strings.Contains(engine.calls[1].Prompt, "implemented") {
		t.Fatalf("prompts did not include outcome contract/handoff:\n%s\n---\n%s", engine.calls[0].Prompt, engine.calls[1].Prompt)
	}
	storedTask, err := store.Repos.Tasks.GetTask(context.Background(), task.ID)
	if err != nil || storedTask.Status != domaintasks.StatusCompleted {
		t.Fatalf("stored task = %+v, %v", storedTask, err)
	}
	stepRuns, err := store.Repos.Workflows.ListStepRuns(context.Background(), run.ID)
	if err != nil || len(stepRuns) != 4 {
		t.Fatalf("step runs = %+v, %v", stepRuns, err)
	}
	for _, stepRun := range stepRuns {
		events, err := store.Repos.Workflows.ListRuntimeEvents(context.Background(), run.ID, stepRun.ID, 0, 10)
		if err != nil || len(events) != 1 {
			t.Fatalf("runtime events for %s = %+v, %v", stepRun.ID, events, err)
		}
	}
	locks, err := filepath.Glob(filepath.Join(store.Data.Paths.Locks, "*.lock"))
	if err != nil {
		t.Fatal(err)
	}
	if len(locks) != 0 {
		t.Fatalf("workspace lock was not released: %v", locks)
	}
	summary, err := os.ReadFile(filepath.Join(store.Data.Paths.Runs, run.ID, "SUMMARY.md"))
	if err != nil || !strings.Contains(string(summary), "looks good") {
		t.Fatalf("summary = %q, %v", summary, err)
	}
}

func TestResumeRunUsesSnapshotSessionAndHumanInstruction(t *testing.T) {
	definition := oneStepPauseWorkflow()
	engine := &scriptedEngine{
		available: map[agentrun.Runtime]bool{agentrun.RuntimeCodex: true},
		scripts: []engineScript{
			{runtime: agentrun.RuntimeCodex, result: success(`{"signal":"need_human","content":"please decide"}`, "session-1")},
			{runtime: agentrun.RuntimeCodex, result: success(`{"signal":"approved","content":"done"}`, "session-2")},
		},
	}
	usecase, store, task := setupUsecase(t, definition, engine)
	paused, err := usecase.ExecuteTask(context.Background(), task.ID)
	if err != nil || paused.Status != domainworkflows.RunPaused {
		t.Fatalf("paused run = %+v, %v", paused, err)
	}
	root := store.Data.Paths.Root
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := localdata.OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	resumedUsecase := NewUsecase(reopened.Repos.Tasks, reopened.Repos.Workflows, engine, workspacelock.New(reopened.Data.Paths.Locks), &fixedGitInspector{})
	resumeNow := time.Date(2026, 7, 10, 16, 0, 0, 0, time.UTC)
	resumedUsecase.now = func() time.Time {
		resumeNow = resumeNow.Add(time.Second)
		return resumeNow
	}
	resumedUsecase.newID = func(prefix string) string { return prefix + "_resumed" }
	completed, err := resumedUsecase.ResumeRun(context.Background(), paused.ID, "继续并确认当前结果")
	if err != nil || completed.Status != domainworkflows.RunCompleted {
		t.Fatalf("resumed run = %+v, %v", completed, err)
	}
	if engine.calls[1].ResumeSessionID != "session-1" || !strings.Contains(engine.calls[1].Prompt, "继续并确认当前结果") {
		t.Fatalf("resume request = %+v", engine.calls[1])
	}
	events, err := reopened.Repos.Workflows.ListEvents(context.Background(), completed.ID, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	foundInstruction := false
	for _, event := range events {
		if event.Type == "run.resumed" && strings.Contains(string(event.Payload), `"instruction":"继续并确认当前结果"`) {
			foundInstruction = true
		}
	}
	if !foundInstruction {
		t.Fatalf("run.resumed event did not persist the human instruction: %+v", events)
	}
	stepRuns, err := reopened.Repos.Workflows.ListStepRuns(context.Background(), completed.ID)
	if err != nil || len(stepRuns) != 2 || stepRuns[1].Attempt != 2 {
		t.Fatalf("step runs = %+v, %v", stepRuns, err)
	}
}

func TestQueuedInstructionsAreClaimedPriorityFirstAtStepBoundary(t *testing.T) {
	definition := oneStepPauseWorkflow()
	engine := &scriptedEngine{
		available: map[agentrun.Runtime]bool{agentrun.RuntimeCodex: true},
		scripts:   []engineScript{{runtime: agentrun.RuntimeCodex, result: success(`{"signal":"approved","content":"done"}`, "session-1")}},
	}
	usecase, store, task := setupUsecase(t, definition, engine)
	run, err := usecase.StartTask(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 7, 10, 15, 30, 0, 0, time.UTC)
	for _, instruction := range []domainworkflows.Instruction{
		{ID: "normal", Content: "run the ordinary check", CreatedAt: base},
		{ID: "priority", Content: "fix the urgent failure", Priority: true, CreatedAt: base.Add(time.Second)},
	} {
		if _, err := store.Repos.Workflows.EnqueueInstruction(context.Background(), run.ID, instruction); err != nil {
			t.Fatal(err)
		}
	}
	completed, err := usecase.ExecuteRun(context.Background(), run.ID)
	if err != nil || completed.Status != domainworkflows.RunCompleted {
		t.Fatalf("ExecuteRun() = %+v, %v", completed, err)
	}
	prompt := engine.calls[0].Prompt
	if priority, normal := strings.Index(prompt, "fix the urgent failure"), strings.Index(prompt, "run the ordinary check"); priority < 0 || normal < 0 || priority > normal {
		t.Fatalf("instruction order in prompt = %q", prompt)
	}
	instructions, err := store.Repos.Workflows.ListInstructions(context.Background(), run.ID)
	if err != nil || len(instructions) != 2 || instructions[0].Status != domainworkflows.InstructionApplied || instructions[1].Status != domainworkflows.InstructionApplied {
		t.Fatalf("ListInstructions() = %+v, %v", instructions, err)
	}
	events, err := store.Repos.Workflows.ListEvents(context.Background(), run.ID, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	foundApplied := false
	for _, event := range events {
		foundApplied = foundApplied || event.Type == "instruction.applied"
	}
	if !foundApplied {
		t.Fatalf("instruction.applied event missing: %+v", events)
	}
}

func TestStepRunPersistsUsageAndDuration(t *testing.T) {
	definition := oneStepPauseWorkflow()
	result := success(`{"signal":"approved","content":"done"}`, "session-usage")
	result.Usage = agentrun.Usage{InputTokens: 1234, CachedInputTokens: 900, CacheCreationInputTokens: 40, OutputTokens: 321, ReasoningOutputTokens: 123}
	engine := &scriptedEngine{available: map[agentrun.Runtime]bool{agentrun.RuntimeCodex: true}, scripts: []engineScript{{result: result}}}
	usecase, store, task := setupUsecase(t, definition, engine)
	run, err := usecase.ExecuteTask(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	stepRuns, err := store.Repos.Workflows.ListStepRuns(context.Background(), run.ID)
	if err != nil || len(stepRuns) != 1 {
		t.Fatalf("ListStepRuns() = %+v, %v", stepRuns, err)
	}
	if stepRuns[0].InputTokens != 1234 || stepRuns[0].CachedInputTokens != 900 || stepRuns[0].CacheCreationInputTokens != 40 || stepRuns[0].OutputTokens != 321 || stepRuns[0].ReasoningOutputTokens != 123 || stepRuns[0].DurationMS <= 0 {
		t.Fatalf("usage and duration = %+v", stepRuns[0])
	}
}

func TestUnknownSignalIsRecordedAsFailureAndRetried(t *testing.T) {
	definition := oneStepPauseWorkflow()
	engine := &scriptedEngine{
		available: map[agentrun.Runtime]bool{agentrun.RuntimeCodex: true},
		scripts: []engineScript{
			{runtime: agentrun.RuntimeCodex, result: success(`{"signal":"invented","content":"wrong"}`, "session-1")},
			{runtime: agentrun.RuntimeCodex, result: success(`{"signal":"approved","content":"done"}`, "session-2")},
		},
	}
	usecase, store, task := setupUsecase(t, definition, engine)
	run, err := usecase.ExecuteTask(context.Background(), task.ID)
	if err != nil || run.Status != domainworkflows.RunCompleted {
		t.Fatalf("run = %+v, %v", run, err)
	}
	stepRuns, err := store.Repos.Workflows.ListStepRuns(context.Background(), run.ID)
	if err != nil || len(stepRuns) != 2 || stepRuns[0].Status != domainworkflows.StepRunFailed || !strings.Contains(stepRuns[0].Error, "invented") {
		t.Fatalf("step runs = %+v, %v", stepRuns, err)
	}
}

func TestUnavailableRuntimePausesWithoutFallback(t *testing.T) {
	definition := oneStepPauseWorkflow()
	engine := &scriptedEngine{available: map[agentrun.Runtime]bool{}}
	usecase, store, task := setupUsecase(t, definition, engine)
	run, err := usecase.ExecuteTask(context.Background(), task.ID)
	if err != nil || run.Status != domainworkflows.RunPaused || run.PauseReason != PauseReasonRuntimeUnavailable {
		t.Fatalf("run = %+v, %v", run, err)
	}
	if len(engine.calls) != 0 {
		t.Fatalf("unavailable runtime was invoked: %+v", engine.calls)
	}
	storedTask, err := store.Repos.Tasks.GetTask(context.Background(), task.ID)
	if err != nil || storedTask.Status != domaintasks.StatusPaused {
		t.Fatalf("stored task = %+v, %v", storedTask, err)
	}
}

func setupUsecase(t *testing.T, definition domainworkflows.Definition, engine Engine) (*Usecase, *localdata.Store, domaintasks.Task) {
	t.Helper()
	root := filepath.Join(t.TempDir(), ".oneshot")
	store, err := localdata.OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	now := time.Date(2026, 7, 10, 15, 0, 0, 0, time.UTC)
	workspace := domainworkspaces.Workspace{ID: "ws_1", Name: "Project", Path: t.TempDir(), DefaultSandbox: "workspace-write", CreatedAt: now, LastOpenedAt: now}
	if err := store.Repos.Tasks.SaveWorkspace(context.Background(), workspace); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Repos.Workflows.SaveDefinition(context.Background(), definition); err != nil {
		t.Fatal(err)
	}
	task := domaintasks.Task{ID: "task_1", WorkspaceID: workspace.ID, Title: "Task", Prompt: "Implement the requested change", WorkflowID: definition.ID, Status: domaintasks.StatusReady, CreatedAt: now, UpdatedAt: now}
	if err := store.Repos.Tasks.SaveTask(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	git := &fixedGitInspector{}
	usecase := NewUsecase(store.Repos.Tasks, store.Repos.Workflows, engine, workspacelock.New(store.Data.Paths.Locks), git)
	sequence := 0
	var stateMu sync.Mutex
	usecase.newID = func(prefix string) string {
		stateMu.Lock()
		defer stateMu.Unlock()
		sequence++
		return fmt.Sprintf("%s_%d", prefix, sequence)
	}
	usecase.now = func() time.Time {
		stateMu.Lock()
		defer stateMu.Unlock()
		now = now.Add(time.Second)
		return now
	}
	return usecase, store, task
}

type concurrentEngine struct {
	mu        sync.Mutex
	active    int
	maxActive int
	requests  []agentrun.Request
}

func (e *concurrentEngine) Available(agentrun.Runtime) bool { return true }
func (e *concurrentEngine) Run(ctx context.Context, request agentrun.Request, sink agentrun.Sink) (agentrun.Result, error) {
	e.mu.Lock()
	e.active++
	if e.active > e.maxActive {
		e.maxActive = e.active
	}
	e.requests = append(e.requests, request)
	e.mu.Unlock()
	select {
	case <-ctx.Done():
		return agentrun.Result{}, ctx.Err()
	case <-time.After(40 * time.Millisecond):
	}
	e.mu.Lock()
	e.active--
	e.mu.Unlock()
	return success(`{"signal":"completed","content":"node complete"}`, "session"), nil
}

func TestExecuteDAGRunsRootsInParallelAndJoins(t *testing.T) {
	definition := domainworkflows.Definition{
		ID: "parallel_review", Name: "Parallel", Mode: domainworkflows.ModeDAG, EntryStepID: "security",
		Policy: domainworkflows.Policy{MaxTransitions: 10, MaxConsecutiveFailures: 3, StepTimeoutSeconds: 10},
		Steps: []domainworkflows.Step{
			{ID: "security", Name: "Security", Runtime: "codex", Sandbox: "read-only", RolePrompt: "Review security", Instruction: "Review", Transitions: map[string]string{"completed": domainworkflows.TargetDone}},
			{ID: "tests", Name: "Tests", Runtime: "claude", Sandbox: "read-only", RolePrompt: "Review tests", Instruction: "Review", Transitions: map[string]string{"completed": domainworkflows.TargetDone}},
			{ID: "synthesis", Name: "Synthesis", Runtime: "codex", Sandbox: "workspace-write", DependsOn: []string{"security", "tests"}, RolePrompt: "Synthesize", Instruction: "Apply findings", Transitions: map[string]string{"completed": domainworkflows.TargetDone}},
		},
	}
	engine := &concurrentEngine{}
	usecase, _, task := setupUsecase(t, definition, engine)
	run, err := usecase.ExecuteTask(context.Background(), task.ID)
	if err != nil || run.Status != domainworkflows.RunCompleted {
		t.Fatalf("DAG run = %+v, %v", run, err)
	}
	engine.mu.Lock()
	defer engine.mu.Unlock()
	if engine.maxActive != 2 || len(engine.requests) != 3 {
		t.Fatalf("max active = %d, requests = %d", engine.maxActive, len(engine.requests))
	}
	if !strings.Contains(engine.requests[2].Prompt, "security: node complete") || !strings.Contains(engine.requests[2].Prompt, "tests: node complete") {
		t.Fatalf("join prompt missing dependency results: %s", engine.requests[2].Prompt)
	}
}

func TestDAGRunSnapshotLimitsConcurrencyWithoutBreakingJoin(t *testing.T) {
	definition := domainworkflows.Definition{ID: "limited_dag", Name: "Limited", Mode: domainworkflows.ModeDAG, EntryStepID: "one", Policy: domainworkflows.Policy{MaxTransitions: 10, MaxConsecutiveFailures: 3, StepTimeoutSeconds: 10}, Steps: []domainworkflows.Step{
		{ID: "one", Name: "One", Runtime: "codex", Sandbox: "read-only", RolePrompt: "One", Instruction: "One", Transitions: map[string]string{"completed": domainworkflows.TargetDone}},
		{ID: "two", Name: "Two", Runtime: "claude", Sandbox: "read-only", RolePrompt: "Two", Instruction: "Two", Transitions: map[string]string{"completed": domainworkflows.TargetDone}},
		{ID: "join", Name: "Join", Runtime: "codex", Sandbox: "workspace-write", DependsOn: []string{"one", "two"}, RolePrompt: "Join", Instruction: "Join", Transitions: map[string]string{"completed": domainworkflows.TargetDone}},
	}}
	engine := &concurrentEngine{}
	usecase, _, task := setupUsecase(t, definition, engine)
	run, err := usecase.StartTaskResolved(context.Background(), task.ID, definition, RunResolution{MaxLocalDAGConcurrency: 1})
	if err != nil {
		t.Fatal(err)
	}
	run, err = usecase.ExecuteRun(context.Background(), run.ID)
	if err != nil || run.Status != domainworkflows.RunCompleted {
		t.Fatalf("run = %+v, %v", run, err)
	}
	engine.mu.Lock()
	defer engine.mu.Unlock()
	if engine.maxActive != 1 || len(engine.requests) != 3 {
		t.Fatalf("max active = %d, requests = %d", engine.maxActive, len(engine.requests))
	}
	if !strings.Contains(engine.requests[2].Prompt, "one: node complete") || !strings.Contains(engine.requests[2].Prompt, "two: node complete") {
		t.Fatalf("join lost dependency results: %s", engine.requests[2].Prompt)
	}
}

type fakeRemoteExecutor struct {
	workerID    string
	workspaceID string
}

func (e *fakeRemoteExecutor) RunRemote(_ context.Context, workerID, workspaceID string, _ agentrun.Request, sink agentrun.Sink) (agentrun.Result, error) {
	e.workerID, e.workspaceID = workerID, workspaceID
	sink(agentrun.Event{Kind: agentrun.KindMessage, Text: "remote event"})
	return success(`{"signal":"completed","content":"remote complete"}`, "remote-session"), nil
}

func TestExecuteDAGDispatchesConfiguredRemoteNode(t *testing.T) {
	definition := domainworkflows.Definition{ID: "remote_dag", Name: "Remote", Mode: domainworkflows.ModeDAG, EntryStepID: "review", Policy: domainworkflows.Policy{MaxTransitions: 5, MaxConsecutiveFailures: 2, StepTimeoutSeconds: 10}, Steps: []domainworkflows.Step{{ID: "review", Name: "Remote review", Runtime: "codex", WorkerID: "mac-mini", Sandbox: "read-only", RolePrompt: "Review", Instruction: "Review remotely", Transitions: map[string]string{"completed": domainworkflows.TargetDone}}}}
	engine := &scriptedEngine{available: map[agentrun.Runtime]bool{}}
	usecase, _, task := setupUsecase(t, definition, engine)
	remote := &fakeRemoteExecutor{}
	usecase.SetRemoteExecutor(remote)
	run, err := usecase.ExecuteTask(context.Background(), task.ID)
	if err != nil || run.Status != domainworkflows.RunCompleted {
		t.Fatalf("remote DAG = %+v, %v", run, err)
	}
	if remote.workerID != "mac-mini" || remote.workspaceID != "ws_1" || len(engine.calls) != 0 {
		t.Fatalf("remote dispatch = %+v, local calls = %d", remote, len(engine.calls))
	}
}

func TestExecuteSerialDispatchesConfiguredRemoteStep(t *testing.T) {
	// A serial workflow step with a WorkerID must route to the worker, not pause
	// on local runtime availability — the engine here has no local runtimes.
	definition := domainworkflows.Definition{ID: "remote_serial", Name: "Remote Serial", Mode: domainworkflows.ModeSerial, EntryStepID: "review", Policy: domainworkflows.Policy{MaxTransitions: 5, MaxConsecutiveFailures: 2, StepTimeoutSeconds: 10}, Steps: []domainworkflows.Step{{ID: "review", Name: "Remote review", Runtime: "codex", WorkerID: "mac-mini", Sandbox: "read-only", RolePrompt: "Review", Instruction: "Review remotely", Transitions: map[string]string{"completed": domainworkflows.TargetDone}}}}
	engine := &scriptedEngine{available: map[agentrun.Runtime]bool{}}
	usecase, _, task := setupUsecase(t, definition, engine)
	remote := &fakeRemoteExecutor{}
	usecase.SetRemoteExecutor(remote)
	run, err := usecase.ExecuteTask(context.Background(), task.ID)
	if err != nil || run.Status != domainworkflows.RunCompleted {
		t.Fatalf("remote serial = %+v, %v", run, err)
	}
	if remote.workerID != "mac-mini" || remote.workspaceID != "ws_1" || len(engine.calls) != 0 {
		t.Fatalf("remote dispatch = %+v, local calls = %d", remote, len(engine.calls))
	}
}

func reviewLoop() domainworkflows.Definition {
	return domainworkflows.Definition{
		ID: "review_loop", Name: "Review Loop", EntryStepID: "implement",
		Policy: domainworkflows.Policy{MaxTransitions: 10, MaxConsecutiveFailures: 3, StepTimeoutSeconds: 10},
		Steps: []domainworkflows.Step{
			{ID: "implement", Name: "Implement", Runtime: "codex", Sandbox: "full", RolePrompt: "You implement.", Instruction: "Implement and test.", Transitions: map[string]string{"ready_for_review": "review", "need_human": domainworkflows.TargetPause}},
			{ID: "review", Name: "Review", Runtime: "claude", RolePrompt: "You review.", Instruction: "Review current changes.", Transitions: map[string]string{"changes_requested": "implement", "approved": domainworkflows.TargetDone}},
		},
	}
}

func oneStepPauseWorkflow() domainworkflows.Definition {
	return domainworkflows.Definition{
		ID: "single_loop", Name: "Single", EntryStepID: "implement",
		Policy: domainworkflows.Policy{MaxTransitions: 10, MaxConsecutiveFailures: 3, StepTimeoutSeconds: 10},
		Steps: []domainworkflows.Step{{
			ID: "implement", Name: "Implement", Runtime: "codex", RolePrompt: "You implement.", Instruction: "Work on the task.",
			Transitions: map[string]string{"need_human": domainworkflows.TargetPause, "approved": domainworkflows.TargetDone},
		}},
	}
}

func success(message, sessionID string) agentrun.Result {
	return agentrun.Result{Succeeded: true, FinalMessage: message, SessionID: sessionID}
}
