package desktop

import (
	"context"
	"testing"

	tasks "github.com/openmodu/onecatch/internal/domain/tasks"
	workflows "github.com/openmodu/onecatch/internal/domain/workflows"
)

func TestExecutionRefreshReadsLatestDefaultsAndKeepsExplicitChoices(t *testing.T) {
	app, _ := newStorageTestApp(t)
	ctx := context.Background()
	if err := app.InitializeSettings(ctx); err != nil {
		t.Fatal(err)
	}
	workspace, err := app.AddWorkspace(ctx, AddWorkspaceInput{Path: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	task := tasks.Task{WorkspaceID: workspace.ID, Harness: "codex", ReasoningEffort: "high", ServiceTier: "fast"}
	definition := workflows.Normalize(workflows.Definition{ID: "single_agent", Name: "Agent", Steps: []workflows.Step{{ID: "execute", Name: "Execute", Runtime: "codex", Model: "explicit-model", Sandbox: "read-only"}}})
	settings, err := app.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, timeout := range []int{120, 240} {
		settings.Execution.StepTimeoutSeconds = timeout
		settings.Execution.MaxConcurrency = 2
		settings.Execution.InterruptGraceSeconds = 25
		settings.Execution.MaxConsecutiveFailures = 4
		settings.Execution.MaxReworkRounds = 5
		settings, err = app.UpdateExecutionSettings(ctx, settings.Execution, settings.Revision)
		if err != nil {
			t.Fatal(err)
		}
		refreshed, resolution, err := app.resolveExecutionSettings(ctx, task, definition)
		if err != nil {
			t.Fatal(err)
		}
		if refreshed.Policy.StepTimeoutSeconds != timeout || refreshed.Policy.MaxConsecutiveFailures != 4 || refreshed.Policy.MaxReworkRounds != 5 || resolution.MaxConcurrency != 2 || resolution.InterruptGraceSeconds != 25 {
			t.Fatalf("stale execution settings: %+v, %+v", refreshed.Policy, resolution)
		}
		runtime := resolution.RuntimeSettings["codex"]
		if refreshed.Steps[0].Model != "explicit-model" || refreshed.Steps[0].Sandbox != "read-only" || runtime.ReasoningEffort != "high" || runtime.ServiceTier != "fast" {
			t.Fatalf("explicit choices replaced: %+v, %+v", refreshed.Steps[0], runtime)
		}
		definition = refreshed
	}
}
