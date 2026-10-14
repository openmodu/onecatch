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
			if err != nil || snapshot.Steps[0].Model != "new-model" || snapshot.Steps[0].Prompt != latest.Steps[0].Prompt {
				t.Fatalf("snapshot did not adopt template settings: %+v, %v", snapshot, err)
			}
		})
	}
}

func TestRefreshStepSettingsSkipsIncompatibleAndDirectSteps(t *testing.T) {
	original := oneStepPauseWorkflow()
	original.Steps[0].Model = "old"
	for _, change := range []func(*domainworkflows.Definition){
		func(d *domainworkflows.Definition) { d.ID = "different" },
		func(d *domainworkflows.Definition) { d.Steps[0].ID = "renamed" },
	} {
		latest := original
		latest.Steps = append([]domainworkflows.Step(nil), original.Steps...)
		latest.Steps[0].Model = "new"
		change(&latest)
		if got := RefreshStepSettings(original, latest); got.Steps[0].Model != "old" {
			t.Fatal("incompatible step changed")
		}
	}
	original.ID = directAgentWorkflowID
	latest := original
	latest.Steps = append([]domainworkflows.Step(nil), original.Steps...)
	latest.Steps[0].Model = "new"
	if got := RefreshStepSettings(original, latest); got.Steps[0].Model != "old" {
		t.Fatal("direct conversation model changed")
	}
}

func TestNextAttemptRefreshesAgentModelAndSandbox(t *testing.T) {
	for _, changeAgent := range []bool{false, true} {
		t.Run(map[bool]string{false: "permissions", true: "agent"}[changeAgent], func(t *testing.T) {
			ctx := context.Background()
			def := reviewLoop()
			def.Steps[0].Model = "old"
			def.Steps[0].Sandbox = "full"
			runtime := agentrun.RuntimeCodex
			if changeAgent {
				runtime = agentrun.RuntimeClaude
			}
			engine := &scriptedEngine{available: map[agentrun.Runtime]bool{agentrun.RuntimeCodex: true, agentrun.RuntimeClaude: true}, scripts: []engineScript{
				{result: success(`{"verdict":"pass","summary":"first impl"}`, "impl")},
				{result: success(`{"verdict":"rework","summary":"retry"}`, "review")},
				{runtime: runtime, result: success(`{"verdict":"pass","summary":"updated impl"}`, "new-impl")},
				{result: success(`{"verdict":"ask_human","summary":"confirm"}`, "review")},
			}}
			uc, store, task := setupUsecase(t, def, engine)
			engine.onCall = func(index int) {
				if index == 0 {
					latest := def
					latest.Steps = append([]domainworkflows.Step(nil), def.Steps...)
					latest.Steps[0].Runtime = string(runtime)
					latest.Steps[0].Model = "new"
					latest.Steps[0].Sandbox = "read-only"
					if _, err := store.Repos.Workflows.UpdateDefinition(ctx, def.ID, latest); err != nil {
						t.Error(err)
					}
				}
			}
			if _, err := uc.ExecuteTask(ctx, task.ID); err != nil {
				t.Fatal(err)
			}
			old, next := engine.calls[0], engine.calls[2]
			if old.Model != "old" || old.Sandbox != "workspace-write" || next.Model != "new" || next.Sandbox != "read-only" || next.Runtime != runtime {
				t.Fatalf("configuration not refreshed: %+v", engine.calls)
			}
			if changeAgent && next.ResumeSessionID != "" {
				t.Fatal("reused another Agent's session")
			}
			if !changeAgent && next.ResumeSessionID != "impl" {
				t.Fatal("permission/model update lost session")
			}
		})
	}
}

func TestResumeAdoptsAddedWorkflowStep(t *testing.T) {
	ctx := context.Background()
	def := oneStepPauseWorkflow()
	engine := &scriptedEngine{available: map[agentrun.Runtime]bool{agentrun.RuntimeCodex: true}, scripts: []engineScript{{result: success(`{"verdict":"ask_human","summary":"pause"}`, "impl")}}}
	uc, store, task := setupUsecase(t, def, engine)
	run, err := uc.ExecuteTask(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	latest := def
	latest.Steps = append(append([]domainworkflows.Step(nil), def.Steps...), domainworkflows.Step{ID: "review", Name: "New Review", Runtime: "codex", Sandbox: "read-only"})
	latest.Edges = []domainworkflows.Edge{{From: "implement", To: "review"}}
	if _, err := store.Repos.Workflows.UpdateDefinition(ctx, def.ID, latest); err != nil {
		t.Fatal(err)
	}
	engine.scripts = []engineScript{{result: success(`{"verdict":"pass","summary":"impl done"}`, "impl")}, {result: success(`{"verdict":"pass","summary":"new review done"}`, "review")}}
	resumed, err := uc.ResumeRun(ctx, run.ID, "continue")
	if err != nil || resumed.Status != domainworkflows.RunCompleted || len(engine.calls) != 3 || resumed.Nodes["review"].Status != domainworkflows.NodeCompleted {
		t.Fatalf("added step not executed: %+v %v", resumed, err)
	}
}
