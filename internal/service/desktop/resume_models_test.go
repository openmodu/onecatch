package desktop

import (
	"context"
	"testing"
	"time"

	domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"
)

func TestWorkflowFollowUpUsesEditedTemplateModel(t *testing.T) {
	engine := &verdictEngine{}
	app, _ := newLocalTestApp(t, engine)
	ctx := context.Background()
	workspace, err := app.AddWorkspace(ctx, AddWorkspaceInput{Path: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	task, err := app.CreateTask(ctx, CreateTaskInput{WorkspaceID: workspace.ID, WorkflowID: "implement_review", Title: "model update", Prompt: "build it"})
	if err != nil {
		t.Fatal(err)
	}
	run, err := app.StartRun(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	wait := func() RunDetail {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			detail, err := app.GetRunDetail(ctx, run.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !detail.Active && detail.Run.Status == domainworkflows.RunCompleted {
				return detail
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatal("run did not complete")
		return RunDetail{}
	}
	wait()
	for _, model := range []string{"opus", ""} {
		latest, err := app.GetDefinition(ctx, task.WorkflowID)
		if err != nil {
			t.Fatal(err)
		}
		latest.Steps[1].Model = model
		if _, err := app.store.Repos.Workflows.UpdateDefinition(ctx, latest.ID, latest); err != nil {
			t.Fatal(err)
		}
		want := model
		if want == "" {
			settings, err := app.GetSettings(ctx)
			if err != nil {
				t.Fatal(err)
			}
			want = settings.Runtimes["claude"].DefaultModel
		}
		if _, err := app.ResumeRunConfigured(ctx, run.ID, ResumeRunInput{Instruction: "继续检查", Harness: "codex"}); err != nil {
			t.Fatal(err)
		}
		detail := wait()
		if detail.Workflow.Steps[1].Model != want {
			t.Fatalf("stored model = %q, want %q", detail.Workflow.Steps[1].Model, want)
		}
		engine.mu.Lock()
		request := engine.requests[len(engine.requests)-1]
		engine.mu.Unlock()
		if request.Model != want || request.ResumeSessionID != "session_claude" {
			t.Fatalf("follow-up did not apply latest model to existing session: %+v", request)
		}
	}
}
