package desktop

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/openmodu/onecatch/internal/service/mobile"
	"github.com/openmodu/onecatch/internal/service/worker"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
	"github.com/openmodu/onecatch/pkg/localfile"
)

func TestHostedRunsShareDesktopExecutionAndSurvivePhoneDisconnect(t *testing.T) {
	ctx := context.Background()
	engine := &fifoEngine{started: make(chan agentrun.Request, 4), release: make(chan struct{}, 4)}
	app, _ := newLocalTestApp(t, engine)
	// Runtime availability uses an installed, inert executable; execution below
	// is driven by fifoEngine, never by a real model or account.
	binary, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	app.runtimes.engine = agentrun.NewEngine(agentrun.Config{ModuIntegration: "cli", Binaries: map[string]string{"modu": binary}})
	workspace, err := app.AddWorkspace(ctx, AddWorkspaceInput{Path: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	options := app.hostWorkerOptions(t.TempDir())
	server := worker.NewServer("test-host", "Desktop", "secret", nil, app.runtimes, 1)
	server.SetSharedRuns(options.SharedRuns)
	sharedWorkspaces, err := options.SharedWorkspaces(ctx)
	if err != nil {
		t.Fatal(err)
	}
	server.SetSharedWorkspaces(sharedWorkspaces)
	server.EnablePairing("PAIR1234", time.Now().Add(time.Minute), true)
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	root := t.TempDir()
	phone, err := mobile.NewService(root)
	if err != nil {
		t.Fatal(err)
	}
	defer phone.Close()
	if _, err := phone.PairWorker(ctx, httpServer.URL, "PAIR1234"); err != nil {
		t.Fatal(err)
	}
	// Existing desktop tasks must be visible on a phone that did not create them.
	task, err := app.CreateTask(ctx, CreateTaskInput{WorkspaceID: workspace.ID, WorkflowID: directAgentWorkflowID, Title: "desktop task", Prompt: "desktop prompt", Harness: "modu", Sandbox: "workspace-write"})
	if err != nil {
		t.Fatal(err)
	}
	desktopRun, err := app.StartRun(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	awaitHostedRequest(t, engine)
	views := phone.ListRuns()
	if len(views) != 1 || views[0].ID != desktopRun.ID || views[0].ConversationID != task.ID || !views[0].Shared {
		t.Fatalf("desktop history = %+v", views)
	}
	engine.release <- struct{}{}
	awaitHostedStatus(t, phone, desktopRun.ID, "succeeded")

	// The workspace is not a clean Git clone. Shared tasks must use the desktop
	// workspace policy, and Modu must receive the coding (bash-enabled) sandbox.
	started, err := phone.StartRun(ctx, mobile.StartRunInput{WorkerID: "test-host", WorkspaceID: workspace.ID, Runtime: "modu", Prompt: "run pwd"})
	if err != nil {
		t.Fatal(err)
	}
	request := awaitHostedRequest(t, engine)
	if request.Sandbox != agentrun.SandboxWorkspaceWrite || request.Runtime != agentrun.RuntimeModu {
		t.Fatalf("request = %+v", request)
	}
	detail, err := app.GetRunDetail(ctx, started.ID)
	if err != nil || detail.Task.ID != started.ConversationID || detail.Task.Prompt != "run pwd" {
		t.Fatalf("same desktop run = %+v, %v", detail, err)
	}
	phone.Close()
	restarted, err := mobile.NewService(root)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	view, err := restarted.GetRun(started.ID)
	if err != nil || view.Status != "running" {
		t.Fatalf("after phone restart = %+v, %v", view, err)
	}
	engine.release <- struct{}{}
	awaitHostedStatus(t, restarted, started.ID, "succeeded")

	resumed, err := restarted.StartRun(ctx, mobile.StartRunInput{WorkerID: "test-host", WorkspaceID: workspace.ID, ConversationID: started.ConversationID, Runtime: "modu", Prompt: "continue here"})
	if err != nil || resumed.ID != started.ID {
		t.Fatalf("resume = %+v, %v", resumed, err)
	}
	request = awaitHostedRequest(t, engine)
	if request.ResumeSessionID != "session_fifo" {
		t.Fatalf("session = %q", request.ResumeSessionID)
	}
	if err := restarted.InterruptRun(ctx, resumed.ID); err != nil {
		t.Fatal(err)
	}
	awaitHostedStatus(t, restarted, resumed.ID, "paused")
	transcript, err := restarted.GetRun(resumed.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, event := range transcript.Events {
		if event.Kind == "user_message" && event.Text == "continue here" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing shared follow-up: %+v", transcript.Events)
	}

	// Auth applies equally to history and execution.
	response, err := http.Get(httpServer.URL + "/v1/shared-runs")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated history status = %d", response.StatusCode)
	}
	// Deleting on desktop removes the corresponding phone cache after a sync.
	if err := app.DeleteTask(ctx, task.ID); err != nil {
		t.Fatal(err)
	}
	for _, view := range restarted.ListRuns() {
		if view.ID == desktopRun.ID {
			t.Fatal("deleted desktop task remained in mobile history")
		}
	}
	httpServer.Close()
	if views := restarted.ListRuns(); len(views) != 1 || views[0].ID != started.ID {
		t.Fatalf("offline cache = %+v", views)
	}
}

func awaitHostedRequest(t *testing.T, engine *fifoEngine) agentrun.Request {
	t.Helper()
	select {
	case request := <-engine.started:
		return request
	case <-time.After(5 * time.Second):
		t.Fatal("execution did not start")
	}
	return agentrun.Request{}
}
func awaitHostedStatus(t *testing.T, phone *mobile.Service, id, status string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		view, err := phone.GetRun(id)
		if err != nil {
			t.Fatal(err)
		}
		if view.Status == status {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	view, _ := phone.GetRun(id)
	t.Fatalf("wanted %s, got %+v", status, view)
}

func TestHostedRunsImportLegacyHistoryOnce(t *testing.T) {
	ctx := context.Background()
	app, store := newLocalTestApp(t, completingEngine{})
	workspace, err := app.AddWorkspace(ctx, AddWorkspaceInput{Path: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	host := &hostedRuns{app: app}
	at := time.Now().UTC().Add(-time.Hour)
	input := worker.SharedRun{ID: "mobile_old1", ConversationID: "mobile_old1", WorkspaceID: workspace.ID,
		Runtime: agentrun.RuntimeModu, Prompt: "old phone prompt", Status: "succeeded", StartedAt: at, FinishedAt: &at,
		Events: []agentrun.Event{{Kind: agentrun.KindToolUse, StreamID: "call", Text: "bash pwd", At: at}, {Kind: agentrun.KindToolResult, StreamID: "call", Phase: agentrun.StreamEnd, Text: "/workspace", At: at}},
		Result: &agentrun.Result{FinalMessage: "old answer", SessionID: "old-session", Succeeded: true}}
	for i := 0; i < 2; i++ {
		imported, err := host.Import(ctx, input)
		if err != nil || imported.ID != input.ID || len(imported.Events) != 2 || imported.Events[0].Phase != "" || imported.Events[0].Kind != agentrun.KindToolUse {
			t.Fatalf("import = %+v, %v", imported, err)
		}
	}
	detail, err := app.GetRunDetail(ctx, input.ID)
	if err != nil || len(detail.StepRuns) != 1 || detail.Task.Prompt != input.Prompt || detail.StepRuns[0].SessionIDAfter != "old-session" {
		t.Fatalf("desktop import = %+v, %v", detail, err)
	}
	second := input
	second.ID, second.Prompt = "mobile_old2", "second phone prompt"
	second.StartedAt = at.Add(time.Minute)
	imported, err := host.Import(ctx, second)
	if err != nil || imported.ConversationID != input.ConversationID || imported.Prompt != second.Prompt {
		t.Fatalf("follow-up import = %+v, %v", imported, err)
	}
	// Historical tasks outside the published workspace set must stay private.
	workspace.Hidden = true
	if err := store.Repos.Tasks.SaveWorkspace(ctx, workspace); err != nil {
		t.Fatal(err)
	}
	page, err := host.List(ctx, "")
	if err != nil || len(page.Items) != 0 {
		t.Fatalf("hidden history = %+v, %v", page, err)
	}
	if _, err := host.Get(ctx, input.ID); err == nil {
		t.Fatal("hidden run was exposed")
	}
	if _, err := host.Import(ctx, input); err == nil {
		t.Fatal("hidden run was imported")
	}
}

func TestHostedRunsMigratePhoneCacheAndPaginate(t *testing.T) {
	ctx := context.Background()
	app, store := newLocalTestApp(t, completingEngine{})
	workspace, err := app.AddWorkspace(ctx, AddWorkspaceInput{Path: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	server := worker.NewServer("test-host", "Desktop", "secret", nil, app.runtimes, 1)
	server.SetSharedRuns(&hostedRuns{app: app})
	mappings, err := app.hostedWorkspaces(ctx)
	if err != nil {
		t.Fatal(err)
	}
	server.SetSharedWorkspaces(mappings)
	server.EnablePairing("PAIR1234", time.Now().Add(time.Minute), true)
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	root := t.TempDir()
	at := time.Now().UTC()
	legacy := mobile.RunView{ID: "mobile_cached", ConversationID: "mobile_cached", WorkerID: "test-host", WorkspaceID: workspace.ID,
		Runtime: agentrun.RuntimeModu, Prompt: "phone-only prompt", Status: "failed", Error: "Tool not found: bash", StartedAt: at,
		Events: []agentrun.Event{{Kind: agentrun.KindError, Text: "Tool not found: bash", At: at}}}
	if err := localfile.WriteJSONAtomic(filepath.Join(root, "runs.json"), []mobile.RunView{legacy}); err != nil {
		t.Fatal(err)
	}
	phone, err := mobile.NewService(root)
	if err != nil {
		t.Fatal(err)
	}
	defer phone.Close()
	if _, err := phone.PairWorker(ctx, httpServer.URL, "PAIR1234"); err != nil {
		t.Fatal(err)
	}
	views := phone.ListRuns()
	if len(views) != 1 || !views[0].Shared || views[0].ID != legacy.ID {
		t.Fatalf("migrated cache = %+v", views)
	}
	detail, err := app.GetRunDetail(ctx, legacy.ID)
	if err != nil || detail.Run.Status != "paused" || detail.Workflow.Steps[0].Sandbox != "workspace-write" {
		t.Fatalf("migrated desktop record = %+v, %v", detail, err)
	}
	// Reconnects do not create another copy, and subsequent pages are retained.
	for i := 0; i < 105; i++ {
		run := detail.Run
		run.ID = fmt.Sprintf("run_history_%d", i)
		run.StartedAt = at.Add(time.Duration(i+1) * time.Second)
		if err := store.Repos.Workflows.SaveRun(ctx, run, detail.Workflow); err != nil {
			t.Fatal(err)
		}
	}
	if views := phone.ListRuns(); len(views) != 106 {
		t.Fatalf("paged run count = %d", len(views))
	}
	if views := phone.ListRuns(); len(views) != 106 {
		t.Fatalf("reconnect run count = %d", len(views))
	}
}
