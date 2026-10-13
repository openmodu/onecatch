package workflows

import (
	"context"
	"testing"

	domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

func TestResumeUsesLatestWorkflowModelAndKeepsSession(t *testing.T) {
	for _, verdict := range []string{"pass", "ask_human"} {
		t.Run(verdict, func(t *testing.T) {
			ctx := context.Background()
			definition := oneStepPauseWorkflow()
			definition.Steps[0].Model = "old-model"
			engine := &scriptedEngine{
				available: map[agentrun.Runtime]bool{agentrun.RuntimeCodex: true},
				scripts: []engineScript{
					{result: success(`{"verdict":"`+verdict+`","summary":"first turn"}`, "existing-session")},
					{result: success(`{"verdict":"pass","summary":"continued"}`, "existing-session")},
				},
			}
			usecase, store, task := setupUsecase(t, definition, engine)
			run, err := usecase.ExecuteTask(ctx, task.ID)
			if err != nil {
				t.Fatal(err)
			}
			latest := definition
			latest.Steps = append([]domainworkflows.Step(nil), definition.Steps...)
			latest.Steps[0].Model = "new-model"
			latest.Steps[0].Prompt = "Changed template prompt"
			if _, err := store.Repos.Workflows.UpdateDefinition(ctx, latest.ID, latest); err != nil {
				t.Fatal(err)
			}
			if _, err := usecase.ResumeRun(ctx, run.ID, "继续"); err != nil {
				t.Fatal(err)
			}
			request := engine.calls[1]
			if engine.calls[0].Model != "old-model" || request.Model != "new-model" || request.ResumeSessionID != "existing-session" {
				t.Fatalf("model refresh lost session or history: %+v", engine.calls)
			}
			snapshot, err := store.Repos.Workflows.GetRunDefinition(ctx, run.ID)
			if err != nil || snapshot.Steps[0].Model != "new-model" || snapshot.Steps[0].Prompt != definition.Steps[0].Prompt {
				t.Fatalf("snapshot refresh changed non-model settings: %+v, %v", snapshot, err)
			}
		})
	}
}

func TestRefreshStepModelsSkipsIncompatibleAndDirectSteps(t *testing.T) {
	original := oneStepPauseWorkflow()
	original.Steps[0].Model = "old"
	for _, change := range []func(*domainworkflows.Definition){
		func(d *domainworkflows.Definition) { d.ID = "different" },
		func(d *domainworkflows.Definition) { d.Steps[0].ID = "renamed" },
		func(d *domainworkflows.Definition) { d.Steps[0].Runtime = "claude" },
		func(d *domainworkflows.Definition) { d.Steps[0].WorkerID = "remote" },
	} {
		latest := original
		latest.Steps = append([]domainworkflows.Step(nil), original.Steps...)
		latest.Steps[0].Model = "new"
		change(&latest)
		if got := RefreshStepModels(original, latest); got.Steps[0].Model != "old" {
			t.Fatal("incompatible step changed")
		}
	}
	original.ID = directAgentWorkflowID
	latest := original
	latest.Steps = append([]domainworkflows.Step(nil), original.Steps...)
	latest.Steps[0].Model = "new"
	if got := RefreshStepModels(original, latest); got.Steps[0].Model != "old" {
		t.Fatal("direct conversation model changed")
	}
}
