package workflows

import (
	"context"
	"testing"

	tasks "github.com/openmodu/onecatch/internal/domain/tasks"
	workflows "github.com/openmodu/onecatch/internal/domain/workflows"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

func TestWorkflowStepEffortsOverrideSharedRuntimeDefault(t *testing.T) {
	definition := workflows.Normalize(workflows.Definition{ID: "step_efforts", Name: "Step efforts", Steps: []workflows.Step{
		{ID: "one", Name: "One", Runtime: "codex", ReasoningEffort: "low"},
		{ID: "two", Name: "Two", Runtime: "codex", ReasoningEffort: "high"},
		{ID: "three", Name: "Three", Runtime: "codex"},
	}, Edges: []workflows.Edge{{From: "one", To: "two"}, {From: "two", To: "three"}}})
	engine := &scriptedEngine{available: map[agentrun.Runtime]bool{agentrun.RuntimeCodex: true}}
	for range 3 {
		engine.scripts = append(engine.scripts, engineScript{result: success(`{"verdict":"pass","summary":"done"}`, "session")})
	}
	uc, store, task := setupUsecase(t, definition, engine)
	uc.SetExecutionSettingsResolver(func(_ context.Context, _ tasks.Task, d workflows.Definition) (workflows.Definition, RunResolution, error) {
		return d, RunResolution{RuntimeSettings: map[string]workflows.ResolvedRuntimeSettings{"codex": {ReasoningEffort: "medium"}}}, nil
	})
	run, err := uc.ExecuteTask(context.Background(), task.ID)
	if err != nil || run.Status != workflows.RunCompleted || len(engine.calls) != 3 {
		t.Fatalf("execution failed: %+v, %v, calls=%d", run, err, len(engine.calls))
	}
	for i, want := range []string{"low", "high", "medium"} {
		if engine.calls[i].ReasoningEffort != want {
			t.Fatalf("step %d effort=%q, want %q", i, engine.calls[i].ReasoningEffort, want)
		}
	}
	snapshot, err := store.Repos.Workflows.GetRunDefinition(context.Background(), run.ID)
	if err != nil || snapshot.Steps[0].ReasoningEffort != "low" || snapshot.Steps[1].ReasoningEffort != "high" || snapshot.Steps[2].ReasoningEffort != "" {
		t.Fatalf("step efforts not persisted: %+v, %v", snapshot.Steps, err)
	}
}
