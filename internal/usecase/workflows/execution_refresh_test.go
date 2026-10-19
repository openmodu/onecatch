package workflows

import (
	"context"
	domaintasks "github.com/openmodu/onecatch/internal/domain/tasks"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

type deadlineEngine struct {
	*scriptedEngine
	remaining []time.Duration
}

func (e *deadlineEngine) Run(ctx context.Context, request agentrun.Request, sink agentrun.Sink) (agentrun.Result, error) {
	deadline, _ := ctx.Deadline()
	e.remaining = append(e.remaining, time.Until(deadline))
	result, err := e.scriptedEngine.Run(ctx, request, sink)
	if after, _ := ctx.Deadline(); after != deadline {
		panic("in-flight deadline changed")
	}
	return result, err
}

func TestNextAttemptRefreshesTimeout(t *testing.T) {
	ctx := context.Background()
	def := domainworkflows.Normalize(reviewLoop())
	def.Policy.StepTimeoutSeconds = 60
	engine := &deadlineEngine{scriptedEngine: &scriptedEngine{
		available: map[agentrun.Runtime]bool{agentrun.RuntimeCodex: true, agentrun.RuntimeClaude: true},
		scripts: []engineScript{
			{result: success(`{"verdict":"pass","summary":"implemented"}`, "impl")},
			{result: success(`{"verdict":"rework","summary":"retry"}`, "review")},
			{result: success(`{"verdict":"pass","summary":"fixed"}`, "impl")},
			{result: success(`{"verdict":"pass","summary":"done"}`, "review")},
		},
	}}
	uc, store, task := setupUsecase(t, def, engine)
	engine.onCall = func(index int) {
		latest := def
		latest.Policy.StepTimeoutSeconds = 120 + index*60
		// Execution policies refresh together with the timeout.
		latest.Policy.MaxReworkRounds = def.Policy.MaxReworkRounds + 1
		if _, err := store.Repos.Workflows.UpdateDefinition(ctx, def.ID, latest); err != nil {
			t.Error(err)
		}
	}
	run, err := uc.ExecuteTask(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != domainworkflows.RunCompleted || len(engine.remaining) != 4 {
		t.Fatalf("unexpected run: %+v, deadlines: %v", run, engine.remaining)
	}
	for i, remaining := range engine.remaining {
		want := time.Duration(60+i*60) * time.Second
		if remaining > want || remaining < want-5*time.Second {
			t.Errorf("attempt %d deadline remaining = %v, want near %v", i, remaining, want)
		}
	}
	snapshot, err := store.Repos.Workflows.GetRunDefinition(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Policy.StepTimeoutSeconds != 240 || snapshot.Policy.MaxReworkRounds != def.Policy.MaxReworkRounds+1 {
		t.Fatalf("incorrect persisted policy: %+v", snapshot.Policy)
	}
}

func TestNextNodeRefreshesPromptCommandAndRuntimeSettings(t *testing.T) {
	ctx := context.Background()
	def := domainworkflows.Normalize(domainworkflows.Definition{ID: "refresh_all", Name: "Refresh", Steps: []domainworkflows.Step{
		{ID: "one", Name: "One", Runtime: "codex", Prompt: "original first"},
		{ID: "two", Name: "Two", Runtime: "codex", Prompt: "original second"},
		{ID: "command", Name: "Command", Kind: domainworkflows.StepCommand, Command: "printf old > result.txt"},
	}, Edges: []domainworkflows.Edge{{From: "one", To: "two"}, {From: "two", To: "command"}}})
	engine := &scriptedEngine{available: map[agentrun.Runtime]bool{agentrun.RuntimeCodex: true, agentrun.RuntimeClaude: true}, scripts: []engineScript{
		{result: success(`{"verdict":"pass","summary":"one"}`, "one")},
		{result: success(`{"verdict":"pass","summary":"two"}`, "two")},
	}}
	uc, store, task := setupUsecase(t, def, engine)
	grace := 10
	uc.SetExecutionSettingsResolver(func(_ context.Context, _ domaintasks.Task, d domainworkflows.Definition) (domainworkflows.Definition, RunResolution, error) {
		return d, RunResolution{MaxConcurrency: 1, InterruptGraceSeconds: grace, RuntimeSettings: map[string]domainworkflows.ResolvedRuntimeSettings{
			"claude": {Provider: "test-provider", ReasoningEffort: "high", ServiceTier: "fast", MaxContextWindow: true, EnvironmentAllowlist: []string{"REFRESH_TEST"}},
		}}, nil
	})
	engine.onCall = func(index int) {
		if index != 0 {
			return
		}
		latest := def
		latest.Steps = append([]domainworkflows.Step(nil), def.Steps...)
		latest.Steps[0].Prompt = "must not replace in-flight node"
		latest.Steps[1].Prompt = "refreshed second prompt"
		latest.Steps[1].Name = "Renamed"
		latest.Steps[1].Runtime = "claude"
		latest.Steps[1].Model = "new-model"
		latest.Steps[1].ReasoningEffort = "low"
		latest.Steps[1].Sandbox = "read-only"
		latest.Steps[2].Command = "printf refreshed > result.txt"
		grace = 25
		if _, err := store.Repos.Workflows.UpdateDefinition(ctx, def.ID, latest); err != nil {
			t.Error(err)
		}
	}
	run, err := uc.ExecuteTask(ctx, task.ID)
	if err != nil || run.Status != domainworkflows.RunCompleted {
		t.Fatalf("run: %+v, %v", run, err)
	}
	request := engine.calls[1]
	if !strings.Contains(request.Prompt, "refreshed second prompt") || request.Runtime != agentrun.RuntimeClaude || request.Model != "new-model" || request.Sandbox != "read-only" || request.InterruptGrace != 25*time.Second || request.Provider != "test-provider" || request.ReasoningEffort != "low" || request.ServiceTier != "fast" || !request.MaxContextWindow || len(request.EnvironmentAllowlist) != 1 {
		t.Fatalf("stale request: %+v", request)
	}
	workspace, err := store.Repos.Tasks.GetWorkspace(ctx, task.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	output, err := os.ReadFile(filepath.Join(workspace.Path, "result.txt"))
	if err != nil || string(output) != "refreshed" {
		t.Fatalf("stale command: %q, %v", output, err)
	}
	snapshot, err := store.Repos.Workflows.GetRunDefinition(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Steps[0].Prompt != "original first" || snapshot.Steps[1].Name != "Renamed" || run.InterruptGraceSeconds != 25 {
		t.Fatalf("incorrect refreshed snapshot: %+v, %+v", snapshot, run)
	}
}

func TestStartingSettingsRefreshConcurrencyBeforeScheduling(t *testing.T) {
	ctx := context.Background()
	def := domainworkflows.Normalize(domainworkflows.Definition{ID: "refresh_parallel", Name: "Parallel", Steps: []domainworkflows.Step{
		{ID: "one", Name: "One", Runtime: "codex", Sandbox: "read-only"}, {ID: "two", Name: "Two", Runtime: "codex", Sandbox: "read-only"},
	}})
	engine := &concurrentEngine{}
	uc, _, task := setupUsecase(t, def, engine)
	run, err := uc.StartTaskResolved(ctx, task.ID, def, RunResolution{MaxConcurrency: 4})
	if err != nil {
		t.Fatal(err)
	}
	uc.SetExecutionSettingsResolver(func(_ context.Context, _ domaintasks.Task, d domainworkflows.Definition) (domainworkflows.Definition, RunResolution, error) {
		return d, RunResolution{MaxConcurrency: 1}, nil
	})
	run, err = uc.ExecuteRun(ctx, run.ID)
	if err != nil || run.Status != domainworkflows.RunCompleted {
		t.Fatalf("run: %+v, %v", run, err)
	}
	engine.mu.Lock()
	defer engine.mu.Unlock()
	if engine.maxActive != 1 || len(engine.requests) != 2 || run.MaxConcurrency != 1 {
		t.Fatalf("stale concurrency: %d, %+v", engine.maxActive, run)
	}
}
