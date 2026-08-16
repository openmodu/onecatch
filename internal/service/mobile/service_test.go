package mobile

import (
	"context"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	gitrepo "github.com/openmodu/oneshot/internal/repo/git"
	"github.com/openmodu/oneshot/internal/service/worker"
	"github.com/openmodu/oneshot/internal/usecase/agentrun"
)

type mobileTestEngine struct {
	mu       sync.Mutex
	requests []agentrun.Request
}

func (*mobileTestEngine) Available(runtime agentrun.Runtime) bool {
	return runtime == agentrun.RuntimeCodex
}

func (e *mobileTestEngine) Run(_ context.Context, request agentrun.Request, sink agentrun.Sink) (agentrun.Result, error) {
	e.mu.Lock()
	e.requests = append(e.requests, request)
	e.mu.Unlock()
	sink(agentrun.Event{Kind: agentrun.KindMessage, Text: "remote analysis", At: time.Now().UTC()})
	return agentrun.Result{FinalMessage: "done", SessionID: "session-mobile", Succeeded: true}, nil
}

func TestRemoteWorkerLifecycleAndReadOnlyRun(t *testing.T) {
	workspace := initMobileTestRepo(t)
	engine := &mobileTestEngine{}
	server := worker.NewServer("mobile-worker", "Mobile Worker", "secret", map[string]string{"oneshot": workspace}, engine, 1)
	server.SetGitInspector(gitrepo.New(""))
	server.EnablePairing("PAIR1234", time.Now().Add(time.Minute), true)
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	service, err := NewService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	frames := make(chan RunFrame, 8)
	service.SetEmitter(func(frame RunFrame) { frames <- frame })

	paired, err := service.PairWorker(context.Background(), httpServer.URL, "PAIR1234")
	if err != nil {
		t.Fatalf("pair worker: %v", err)
	}
	if paired.ID != "mobile-worker" || !paired.HasToken {
		t.Fatalf("paired worker = %+v", paired)
	}
	status, err := service.CheckWorker(context.Background(), paired.ID)
	if err != nil {
		t.Fatalf("check worker: %v", err)
	}
	if !status.Health.Runtimes["codex"] {
		t.Fatalf("health = %+v", status.Health)
	}
	workspaces, err := service.ListWorkspaces(context.Background(), paired.ID)
	if err != nil || len(workspaces) != 1 || workspaces[0].ID != "oneshot" {
		t.Fatalf("workspaces = %+v, err=%v", workspaces, err)
	}

	run, err := service.StartRun(context.Background(), StartRunInput{
		WorkerID: paired.ID, WorkspaceID: "oneshot", Runtime: "codex", Prompt: "review this repository",
	})
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for run.Status == "running" && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
		run, err = service.GetRun(run.ID)
		if err != nil {
			t.Fatal(err)
		}
	}
	if run.Status != "succeeded" || run.Result == nil || run.Result.FinalMessage != "done" {
		t.Fatalf("run = %+v", run)
	}
	if len(run.Events) != 1 || run.Events[0].Text != "remote analysis" {
		t.Fatalf("events = %+v", run.Events)
	}
	engine.mu.Lock()
	requests := append([]agentrun.Request{}, engine.requests...)
	engine.mu.Unlock()
	if len(requests) != 1 || requests[0].Sandbox != agentrun.SandboxReadOnly {
		t.Fatalf("requests = %+v", requests)
	}

	terminal := false
	for len(frames) > 0 {
		frame := <-frames
		if frame.RunID == run.ID && frame.Status == "succeeded" {
			terminal = true
		}
	}
	if !terminal {
		t.Fatal("missing terminal run frame")
	}
}

func TestStartRunRejectsDirtyRemoteWorkspace(t *testing.T) {
	workspace := initMobileTestRepo(t)
	if err := os.WriteFile(filepath.Join(workspace, "dirty.txt"), []byte("dirty"), 0o600); err != nil {
		t.Fatal(err)
	}
	server := worker.NewServer("dirty-worker", "Dirty Worker", "secret", map[string]string{"oneshot": workspace}, &mobileTestEngine{}, 1)
	server.SetGitInspector(gitrepo.New(""))
	server.EnablePairing("PAIR1234", time.Now().Add(time.Minute), true)
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	service, err := NewService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	if _, err := service.PairWorker(context.Background(), httpServer.URL, "PAIR1234"); err != nil {
		t.Fatal(err)
	}
	_, err = service.StartRun(context.Background(), StartRunInput{
		WorkerID: "dirty-worker", WorkspaceID: "oneshot", Runtime: "codex", Prompt: "review",
	})
	if err == nil || err.Error() != "mobile_workspace_dirty: the mapped workspace must be clean before a mobile run" {
		t.Fatalf("start dirty run error = %v", err)
	}
}

func TestManageRemoteWorkspaceLifecycle(t *testing.T) {
	source := initMobileTestRepo(t)
	workerRoot := t.TempDir()
	server := worker.NewServer("workspace-worker", "Workspace Worker", "secret", nil, &mobileTestEngine{}, 1)
	if err := server.SetWorkspaceRegistry(context.Background(), worker.NewWorkspaceRegistry(filepath.Join(workerRoot, "workspaces.json"))); err != nil {
		t.Fatal(err)
	}
	server.SetGitInspector(gitrepo.New(""))
	server.EnablePairing("PAIR1234", time.Now().Add(time.Minute), true)
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	service, err := NewService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	paired, err := service.PairWorker(context.Background(), httpServer.URL, "PAIR1234")
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := service.PrepareWorkspace(context.Background(), paired.ID, "mobile-project", worker.WorkspacePrepareRequest{
		Name: "Mobile Project", RemoteURL: source, Revision: "HEAD",
	})
	if err != nil || prepared.Mapping.Name != "Mobile Project" || !prepared.Mapping.Managed {
		t.Fatalf("prepared = %+v, %v", prepared, err)
	}
	items, err := service.ListWorkspaces(context.Background(), paired.ID)
	if err != nil || len(items) != 1 || items[0].ID != "mobile-project" {
		t.Fatalf("workspaces = %+v, %v", items, err)
	}
	if err := service.RemoveWorkspace(context.Background(), paired.ID, "mobile-project", true); err != nil {
		t.Fatal(err)
	}
	items, err = service.ListWorkspaces(context.Background(), paired.ID)
	if err != nil || len(items) != 0 {
		t.Fatalf("workspaces after remove = %+v, %v", items, err)
	}
}

func TestRunHistoryPersistsAcrossServiceRestart(t *testing.T) {
	workspace := initMobileTestRepo(t)
	server := worker.NewServer("history-worker", "History Worker", "secret", map[string]string{"oneshot": workspace}, &mobileTestEngine{}, 1)
	server.SetGitInspector(gitrepo.New(""))
	server.EnablePairing("PAIR1234", time.Now().Add(time.Minute), true)
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	root := t.TempDir()
	service, err := NewService(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.PairWorker(context.Background(), httpServer.URL, "PAIR1234"); err != nil {
		t.Fatal(err)
	}
	run, err := service.StartRun(context.Background(), StartRunInput{
		WorkerID: "history-worker", WorkspaceID: "oneshot", Runtime: "codex", Prompt: "remember this run",
	})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for run.Status == "running" && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
		run, err = service.GetRun(run.ID)
		if err != nil {
			t.Fatal(err)
		}
	}
	if run.Status != "succeeded" {
		t.Fatalf("run status = %q", run.Status)
	}
	service.Close()

	reopened, err := NewService(root)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	runs := reopened.ListRuns()
	if len(runs) != 1 || runs[0].ID != run.ID || runs[0].Prompt != "remember this run" {
		t.Fatalf("runs = %+v", runs)
	}
	if len(runs[0].Events) != 1 || runs[0].Result == nil || runs[0].Result.FinalMessage != "done" {
		t.Fatalf("persisted run = %+v", runs[0])
	}
	if runs[0].ConversationID != runs[0].ID {
		t.Fatalf("new conversation id = %q, want %q", runs[0].ConversationID, runs[0].ID)
	}
}

func TestFollowUpKeepsConversationID(t *testing.T) {
	workspace := initMobileTestRepo(t)
	server := worker.NewServer("conversation-worker", "Conversation Worker", "secret", map[string]string{"oneshot": workspace}, &mobileTestEngine{}, 1)
	server.SetGitInspector(gitrepo.New(""))
	server.EnablePairing("PAIR1234", time.Now().Add(time.Minute), true)
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	service, err := NewService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	if _, err := service.PairWorker(context.Background(), httpServer.URL, "PAIR1234"); err != nil {
		t.Fatal(err)
	}
	first, err := service.StartRun(context.Background(), StartRunInput{
		WorkerID: "conversation-worker", WorkspaceID: "oneshot", Runtime: "codex", Prompt: "first turn",
	})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for first.Status == "running" && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
		first, err = service.GetRun(first.ID)
		if err != nil {
			t.Fatal(err)
		}
	}
	second, err := service.StartRun(context.Background(), StartRunInput{
		WorkerID: "conversation-worker", WorkspaceID: "oneshot", ConversationID: first.ConversationID,
		Runtime: "codex", Prompt: "follow up", ResumeSessionID: "session-mobile",
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.ConversationID != first.ConversationID {
		t.Fatalf("follow-up conversation id = %q, want %q", second.ConversationID, first.ConversationID)
	}
}

func initMobileTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runMobileGit(t, dir, "init", "-q")
	runMobileGit(t, dir, "config", "user.email", "mobile@example.com")
	runMobileGit(t, dir, "config", "user.name", "Mobile Test")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# mobile\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runMobileGit(t, dir, "add", "README.md")
	runMobileGit(t, dir, "commit", "-qm", "initial")
	return dir
}

func runMobileGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	commandArgs := append([]string{"-C", dir}, args...)
	if output, err := exec.Command("git", commandArgs...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
}
