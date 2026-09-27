package desktop

import (
	"context"
	"testing"

	domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"
	"github.com/openmodu/onecatch/internal/service/worker"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
	workflowuc "github.com/openmodu/onecatch/internal/usecase/workflows"
)

type conversationRemote struct {
	ids      []string
	sessions []string
}

func (r *conversationRemote) RunRemote(_ context.Context, id, _ string, req agentrun.Request, _ agentrun.Sink) (agentrun.Result, error) {
	r.ids = append(r.ids, id)
	r.sessions = append(r.sessions, req.ResumeSessionID)
	return agentrun.Result{Succeeded: true, SessionID: "remote-thread", FinalMessage: "remote reply"}, nil
}

func TestDirectConversationPersistsWorkerAndResumesRemoteSession(t *testing.T) {
	app, orchestrator := newStorageTestApp(t)
	ctx := context.Background()
	if err := app.InitializeSettings(ctx); err != nil {
		t.Fatal(err)
	}
	if err := app.EnsureBuiltinDefinitions(ctx); err != nil {
		t.Fatal(err)
	}
	settings, err := app.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	settings.Experimental.RemoteWorkersEnabled = true
	if _, err := app.UpdateExperimentalSettings(ctx, settings.Experimental, settings.Revision); err != nil {
		t.Fatal(err)
	}
	if _, err := app.workers.Save(ctx, worker.Input{ID: "sandbox-test", Name: "Sandbox", Provider: worker.ProviderVolcengineSandbox, BaseURL: "https://example.volceapi.com/?faasInstanceName=test", Token: "test-token", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	workspace, err := app.AddWorkspace(ctx, AddWorkspaceInput{Path: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	input := CreateTaskInput{WorkspaceID: workspace.ID, WorkflowID: directAgentWorkflowID, Title: "Remote chat", Prompt: "hello", Harness: "codex", WorkerID: "sandbox-test"}
	task, err := app.CreateTask(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := app.store.Repos.Tasks.GetTask(ctx, task.ID)
	if err != nil || stored.WorkerID != input.WorkerID {
		t.Fatalf("stored task = %+v, %v", stored, err)
	}
	resolved, resolution, err := app.resolveRunSettings(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Steps[0].WorkerID != input.WorkerID {
		t.Fatalf("worker not resolved: %+v", resolved.Steps)
	}
	original, err := app.GetDefinition(ctx, directAgentWorkflowID)
	if err != nil {
		t.Fatal(err)
	}
	if original.Steps[0].WorkerID == input.WorkerID {
		t.Fatal("shared workflow was mutated")
	}
	remote := &conversationRemote{}
	orchestrator.SetRemoteExecutor(remote)
	run, err := orchestrator.StartTaskResolved(ctx, task.ID, resolved, resolution)
	if err != nil {
		t.Fatal(err)
	}
	run, err = orchestrator.ExecuteRun(ctx, run.ID)
	if err != nil || run.Status != domainworkflows.RunCompleted {
		t.Fatalf("first run = %+v, %v", run, err)
	}
	run, err = orchestrator.ResumeRunWithProfile(ctx, run.ID, "continue", workflowuc.ResumeProfile{Harness: "codex"})
	if err != nil || run.Status != domainworkflows.RunCompleted {
		t.Fatalf("resume = %+v, %v", run, err)
	}
	if len(remote.ids) != 2 || remote.ids[0] != input.WorkerID || remote.ids[1] != input.WorkerID || remote.sessions[1] != "remote-thread" {
		t.Fatalf("routing = %+v, sessions = %+v", remote.ids, remote.sessions)
	}
	savedDefinition, err := app.store.Repos.Workflows.GetRunDefinition(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if resumeUsesLocalHarness(savedDefinition, "", "codex") {
		t.Fatal("remote resume requires a local binary")
	}
	if _, err := app.persistAttachments(ctx, task, []string{"/local/image.png"}); errorCode(err) != "worker_attachments_unsupported" {
		t.Fatalf("attachment error = %v", err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*CreateTaskInput)
		code   string
	}{
		{"missing", func(i *CreateTaskInput) { i.WorkerID = "missing" }, "worker_not_found"},
		{"harness", func(i *CreateTaskInput) { i.Harness = "claude" }, "sandbox_codex_required"},
		{"worktree", func(i *CreateTaskInput) { i.WorktreeMode = "new" }, "worktree_local_only"},
		{"attachments", func(i *CreateTaskInput) { i.AttachmentPaths = []string{"/local/file"} }, "worker_attachments_unsupported"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			next := input
			tc.mutate(&next)
			if _, err := app.CreateTask(ctx, next); errorCode(err) != tc.code {
				t.Fatalf("error=%v; want %s", err, tc.code)
			}
		})
	}
	settings, _ = app.GetSettings(ctx)
	settings.Experimental.RemoteWorkersEnabled = false
	if _, err := app.UpdateExperimentalSettings(ctx, settings.Experimental, settings.Revision); err != nil {
		t.Fatal(err)
	}
	if _, err := app.CreateTask(ctx, input); errorCode(err) != "experimental_remote_workers_disabled" {
		t.Fatalf("disabled error = %v", err)
	}
	if _, err := app.PreviewRun(ctx, task.ID); errorCode(err) != "experimental_remote_workers_disabled" {
		t.Fatalf("disabled preview = %v", err)
	}
}
