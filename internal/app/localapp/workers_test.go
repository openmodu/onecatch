package localapp

import (
	"context"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/openmodu/oneshot/internal/agentrun"
	"github.com/openmodu/oneshot/internal/worker"
)

type remoteWritingEngine struct{}

func (remoteWritingEngine) Available(runtime agentrun.Runtime) bool {
	return runtime == agentrun.RuntimeCodex
}

type remoteApprovalEngine struct {
	decision chan agentrun.PermissionDecision
}

func (remoteApprovalEngine) Available(runtime agentrun.Runtime) bool {
	return runtime == agentrun.RuntimeClaude
}

func (engine remoteApprovalEngine) Run(ctx context.Context, request agentrun.Request, sink agentrun.Sink) (agentrun.Result, error) {
	permission := agentrun.PermissionRequest{ID: "request-1", ToolName: "WebFetch"}
	sink(agentrun.Event{Kind: agentrun.KindPermissionRequest, Permission: &permission})
	decision, err := request.PermissionHandler(ctx, permission)
	if err != nil {
		return agentrun.Result{}, err
	}
	engine.decision <- decision
	return agentrun.Result{Succeeded: true}, nil
}

func (remoteWritingEngine) Run(_ context.Context, request agentrun.Request, _ agentrun.Sink) (agentrun.Result, error) {
	err := os.WriteFile(filepath.Join(request.Workspace, "remote.txt"), []byte("delivered\n"), 0o644)
	return agentrun.Result{Succeeded: err == nil, FinalMessage: "done"}, err
}

func TestRemoteExecutorDeliversWritableChangesLocally(t *testing.T) {
	root := t.TempDir()
	local := filepath.Join(root, "local")
	if err := os.Mkdir(local, 0o755); err != nil {
		t.Fatal(err)
	}
	runWorkerGitTest(t, local, "init")
	runWorkerGitTest(t, local, "config", "user.email", "localapp-test@example.com")
	runWorkerGitTest(t, local, "config", "user.name", "Localapp Test")
	if err := os.WriteFile(filepath.Join(local, "tracked.txt"), []byte("initial\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runWorkerGitTest(t, local, "add", "tracked.txt")
	runWorkerGitTest(t, local, "commit", "-m", "initial")
	runWorkerGitTest(t, local, "remote", "add", "origin", local)

	workerState := filepath.Join(root, "worker-state")
	service := worker.NewServer("remote", "Remote", "secret", nil, remoteWritingEngine{}, 1)
	if err := service.SetWorkspaceRegistry(context.Background(), worker.NewWorkspaceRegistry(filepath.Join(workerState, "workspaces.json"))); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(service.Handler())
	defer server.Close()
	registry := worker.NewRegistry(filepath.Join(root, "workers.json"))
	if _, err := registry.Save(context.Background(), worker.Input{ID: "remote", Name: "Remote", BaseURL: server.URL, Token: "secret", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	executor := &remoteExecutor{registry: registry, client: worker.NewClient(), preparations: newRemotePreparationRegistry()}
	result, err := executor.RunRemote(context.Background(), "remote", "workspace", agentrun.Request{
		RunID: "workflow-run", Runtime: agentrun.RuntimeCodex, Workspace: local,
		Sandbox: agentrun.SandboxWorkspaceWrite, Prompt: "write",
	}, nil)
	if err != nil || !result.Succeeded {
		t.Fatalf("remote result = %+v, %v", result, err)
	}
	if content, readErr := os.ReadFile(filepath.Join(local, "remote.txt")); readErr != nil || string(content) != "delivered\n" {
		t.Fatalf("local delivered file = %q, %v", content, readErr)
	}
	remote := filepath.Join(workerState, "projects", "workspace")
	if status := runWorkerGitTest(t, remote, "status", "--porcelain"); strings.TrimSpace(status) != "" {
		t.Fatalf("remote workspace was not cleaned: %q", status)
	}
}

func TestRemoteExecutorRoutesPermissionDecisionBackToWorker(t *testing.T) {
	engine := remoteApprovalEngine{decision: make(chan agentrun.PermissionDecision, 1)}
	workspace := t.TempDir()
	runWorkerGitTest(t, workspace, "init")
	runWorkerGitTest(t, workspace, "config", "user.email", "localapp-test@example.com")
	runWorkerGitTest(t, workspace, "config", "user.name", "Localapp Test")
	if err := os.WriteFile(filepath.Join(workspace, "tracked.txt"), []byte("initial\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runWorkerGitTest(t, workspace, "add", "tracked.txt")
	runWorkerGitTest(t, workspace, "commit", "-m", "initial")
	runWorkerGitTest(t, workspace, "remote", "add", "origin", workspace)
	service := worker.NewServer("remote", "Remote", "secret", nil, engine, 1)
	if err := service.SetWorkspaceRegistry(context.Background(), worker.NewWorkspaceRegistry(filepath.Join(t.TempDir(), "workspaces.json"))); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(service.Handler())
	defer server.Close()
	registry := worker.NewRegistry(filepath.Join(t.TempDir(), "workers.json"))
	if _, err := registry.Save(context.Background(), worker.Input{ID: "remote", Name: "Remote", BaseURL: server.URL, Token: "secret", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	client := worker.NewClient()
	permissions := newRemotePermissionRegistry(client)
	executor := &remoteExecutor{registry: registry, client: client, permissions: permissions, preparations: newRemotePreparationRegistry()}
	requested := make(chan struct{})
	finished := make(chan error, 1)
	go func() {
		_, err := executor.RunRemote(context.Background(), "remote", "workspace", agentrun.Request{
			RunID: "workflow-run", Runtime: agentrun.RuntimeClaude, Workspace: workspace,
			Sandbox: agentrun.SandboxReadOnly, Prompt: "review",
		}, func(event agentrun.Event) {
			if event.Kind == agentrun.KindPermissionRequest {
				close(requested)
			}
		})
		finished <- err
	}()
	select {
	case <-requested:
	case <-time.After(5 * time.Second):
		t.Fatal("permission request was not delivered")
	}
	found, err := permissions.resolve("workflow-run", "request-1", "allow_once")
	if err != nil || !found {
		t.Fatalf("resolve permission = %v, %v", found, err)
	}
	select {
	case decision := <-engine.decision:
		if decision.Behavior != "allow" || decision.DecisionClassification != "user_temporary" {
			t.Fatalf("decision = %+v", decision)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("permission decision did not reach worker")
	}
	if err := <-finished; err != nil {
		t.Fatalf("remote execute: %v", err)
	}
}

func TestRemotePreparationRegistryCoalescesConcurrentRequests(t *testing.T) {
	registry := newRemotePreparationRegistry()
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	prepare := func() error {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		return nil
	}
	results := make(chan error, 2)
	go func() { results <- registry.ensure(context.Background(), "worker\x00workspace\x00revision", prepare) }()
	<-started
	go func() { results <- registry.ensure(context.Background(), "worker\x00workspace\x00revision", prepare) }()
	close(release)
	if err := <-results; err != nil {
		t.Fatal(err)
	}
	if err := <-results; err != nil {
		t.Fatal(err)
	}
	if err := registry.ensure(context.Background(), "worker\x00workspace\x00revision", prepare); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("prepare calls = %d, want 1", calls.Load())
	}
}

func runWorkerGitTest(t *testing.T, directory string, args ...string) string {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = directory
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
	return string(output)
}
