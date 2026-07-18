package worker

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/openmodu/oneshot/internal/agentrun"
	domainworkspaces "github.com/openmodu/oneshot/internal/domain/workspaces"
)

type fakeGit struct{ path string }

func (g *fakeGit) Inspect(_ context.Context, workspace string) (domainworkspaces.GitSnapshot, error) {
	g.path = workspace
	return domainworkspaces.GitSnapshot{IsRepo: true, Branch: "main", Files: []domainworkspaces.GitFile{{Path: "a.go", Worktree: "M"}}}, nil
}

type fakeEngine struct{}

func (fakeEngine) Available(runtime agentrun.Runtime) bool { return runtime == agentrun.RuntimeCodex }
func (fakeEngine) Run(_ context.Context, request agentrun.Request, sink agentrun.Sink) (agentrun.Result, error) {
	sink(agentrun.Event{Kind: agentrun.KindMessage, Text: request.Workspace})
	return agentrun.Result{Succeeded: true, SessionID: "remote-session", FinalMessage: `{"signal":"completed","content":"remote done"}`}, nil
}

type capturingEngine struct{ requests chan agentrun.Request }

func (capturingEngine) Available(runtime agentrun.Runtime) bool {
	return runtime == agentrun.RuntimeCodex
}
func (e capturingEngine) Run(_ context.Context, request agentrun.Request, sink agentrun.Sink) (agentrun.Result, error) {
	e.requests <- request
	sink(agentrun.Event{Kind: agentrun.KindMessage, Text: request.Workspace})
	return agentrun.Result{Succeeded: true, SessionID: "remote-session", FinalMessage: `{"signal":"completed","content":"remote done"}`}, nil
}

// blockingEngine streams one event, signals it started, then blocks until
// either the run context is cancelled (a real interrupt) or release is closed
// (a test-cleanup backstop so a blocked run never hangs server.Close).
type blockingEngine struct {
	started chan struct{}
	release chan struct{}
}

func newBlockingEngine(t *testing.T) blockingEngine {
	engine := blockingEngine{started: make(chan struct{}), release: make(chan struct{})}
	t.Cleanup(func() { close(engine.release) })
	return engine
}

func (blockingEngine) Available(agentrun.Runtime) bool { return true }
func (e blockingEngine) Run(ctx context.Context, _ agentrun.Request, sink agentrun.Sink) (agentrun.Result, error) {
	sink(agentrun.Event{Kind: agentrun.KindMessage, Text: "working"})
	close(e.started)
	select {
	case <-ctx.Done():
		return agentrun.Result{Succeeded: false, FinalMessage: "stopped"}, ctx.Err()
	case <-e.release:
		return agentrun.Result{Succeeded: false, FinalMessage: "released"}, nil
	}
}

func TestRegistryMasksTokenAndUsesPrivateFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workers.json")
	registry := NewRegistry(path)
	info, err := registry.Save(context.Background(), Input{ID: "mac-mini", Name: "Mac mini", BaseURL: "http://127.0.0.1:9231", Token: "secret", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(info)
	if strings.Contains(string(payload), "secret") || !info.HasToken {
		t.Fatalf("public worker leaked or lost token state: %s", payload)
	}
	config, err := registry.Get(context.Background(), "mac-mini")
	if err != nil || config.Token != "secret" {
		t.Fatalf("stored config = %+v, %v", config, err)
	}
	stat, err := os.Stat(path)
	if err != nil || stat.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, %v", stat.Mode().Perm(), err)
	}
}

func TestServerAuthenticatesAndStreamsMappedWorkspace(t *testing.T) {
	engine := capturingEngine{requests: make(chan agentrun.Request, 1)}
	server := httptest.NewServer(NewServer("remote-1", "Remote", "secret", map[string]string{"project": "/tmp/project"}, engine, 0).Handler())
	defer server.Close()
	unauthorized, err := http.Get(server.URL + "/v1/health")
	if err != nil {
		t.Fatal(err)
	}
	_ = unauthorized.Body.Close()
	if unauthorized.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", unauthorized.StatusCode)
	}
	client := NewClient()
	config := Config{ID: "remote-1", BaseURL: server.URL, Token: "secret", Enabled: true}
	health, err := client.Health(context.Background(), config)
	if err != nil || !health.Runtimes["codex"] || health.Runtimes["claude"] {
		t.Fatalf("health = %+v, %v", health, err)
	}
	var events []agentrun.Event
	result, err := client.Execute(context.Background(), config, ExecuteRequest{RunID: "run-1", WorkspaceID: "project", Runtime: agentrun.RuntimeCodex, Model: "gpt-test", ReasoningEffort: "high", ServiceTier: "priority", Sandbox: agentrun.SandboxReadOnly, Prompt: "review"}, func(e agentrun.Event) { events = append(events, e) })
	if err != nil || !result.Succeeded || len(events) != 1 || events[0].Text != "/tmp/project" {
		t.Fatalf("execute result=%+v events=%+v err=%v", result, events, err)
	}
	request := <-engine.requests
	if request.Model != "gpt-test" || request.ReasoningEffort != "high" || request.ServiceTier != "priority" {
		t.Fatalf("remote model settings = %+v", request)
	}
	_, err = client.Execute(context.Background(), config, ExecuteRequest{RunID: "run-2", WorkspaceID: "missing", Runtime: agentrun.RuntimeCodex, Prompt: "review"}, nil)
	var remote RemoteError
	if !errors.As(err, &remote) || remote.Code != "worker_workspace_unmapped" {
		t.Fatalf("unmapped error = %v", err)
	}
	// A run needs a valid id so an interrupt can address it later.
	_, err = client.Execute(context.Background(), config, ExecuteRequest{WorkspaceID: "project", Runtime: agentrun.RuntimeCodex, Prompt: "review"}, nil)
	if !errors.As(err, &remote) || remote.Code != "worker_invalid_request" {
		t.Fatalf("missing run id error = %v", err)
	}
}

func TestInterruptStopsAnInFlightRun(t *testing.T) {
	engine := newBlockingEngine(t)
	server := httptest.NewServer(NewServer("remote-1", "Remote", "secret", map[string]string{"project": "/tmp/project"}, engine, 0).Handler())
	defer server.Close()
	client := NewClient()
	config := Config{ID: "remote-1", BaseURL: server.URL, Token: "secret", Enabled: true}

	firstEvent := make(chan agentrun.Event, 1)
	resultCh := make(chan agentrun.Result, 1)
	go func() {
		result, _ := client.Execute(context.Background(), config, ExecuteRequest{RunID: "run-live", WorkspaceID: "project", Runtime: agentrun.RuntimeCodex, Sandbox: agentrun.SandboxReadOnly, Prompt: "long"}, func(e agentrun.Event) {
			select {
			case firstEvent <- e:
			default:
			}
		})
		resultCh <- result
	}()

	// Receiving the event proves the response is streamed: the engine emits it,
	// then blocks — so a buffered protocol could not have delivered it yet.
	select {
	case event := <-firstEvent:
		if event.Text != "working" {
			t.Fatalf("streamed event = %+v", event)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no event streamed before completion")
	}
	if err := client.Interrupt(context.Background(), config, "run-live"); err != nil {
		t.Fatalf("interrupt: %v", err)
	}
	select {
	case result := <-resultCh:
		if result.FinalMessage != "stopped" {
			t.Fatalf("interrupted result = %+v", result)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run did not stop after interrupt")
	}
	// Interrupting an unknown run is a safely-ignorable not-found.
	err := client.Interrupt(context.Background(), config, "ghost")
	var remote RemoteError
	if !errors.As(err, &remote) || remote.Code != "worker_run_not_found" {
		t.Fatalf("ghost interrupt = %v", err)
	}
}

func TestWorkerGitStatus(t *testing.T) {
	worker := NewServer("remote-1", "Remote", "secret", map[string]string{"project": "/tmp/project"}, fakeEngine{}, 0)
	server := httptest.NewServer(worker.Handler())
	defer server.Close()
	client := NewClient()
	config := Config{ID: "remote-1", BaseURL: server.URL, Token: "secret", Enabled: true}

	// Without an inspector the capability is reported as unavailable.
	_, err := client.GitStatus(context.Background(), config, "project")
	var remote RemoteError
	if !errors.As(err, &remote) || remote.Code != "worker_git_unsupported" {
		t.Fatalf("git without inspector = %v", err)
	}

	git := &fakeGit{}
	worker.SetGitInspector(git)
	snapshot, err := client.GitStatus(context.Background(), config, "project")
	if err != nil || !snapshot.IsRepo || snapshot.Branch != "main" || len(snapshot.Files) != 1 {
		t.Fatalf("git status = %+v, %v", snapshot, err)
	}
	if git.path != "/tmp/project" {
		t.Fatalf("inspected path = %q, want the mapped workspace", git.path)
	}
	_, err = client.GitStatus(context.Background(), config, "missing")
	if !errors.As(err, &remote) || remote.Code != "worker_workspace_unmapped" {
		t.Fatalf("git unmapped = %v", err)
	}
}

func TestServerRejectsWhenAtCapacity(t *testing.T) {
	engine := newBlockingEngine(t)
	server := httptest.NewServer(NewServer("remote-1", "Remote", "secret", map[string]string{"project": "/tmp/project"}, engine, 1).Handler())
	defer server.Close()
	client := NewClient()
	config := Config{ID: "remote-1", BaseURL: server.URL, Token: "secret", Enabled: true}

	go func() {
		_, _ = client.Execute(context.Background(), config, ExecuteRequest{RunID: "run-hold", WorkspaceID: "project", Runtime: agentrun.RuntimeCodex, Sandbox: agentrun.SandboxReadOnly, Prompt: "hold"}, nil)
	}()
	select {
	case <-engine.started:
	case <-time.After(5 * time.Second):
		t.Fatal("holding run never started")
	}
	_, err := client.Execute(context.Background(), config, ExecuteRequest{RunID: "run-extra", WorkspaceID: "project", Runtime: agentrun.RuntimeCodex, Sandbox: agentrun.SandboxReadOnly, Prompt: "extra"}, nil)
	var remote RemoteError
	if !errors.As(err, &remote) || remote.Code != "worker_busy" {
		t.Fatalf("capacity error = %v", err)
	}
	_ = client.Interrupt(context.Background(), config, "run-hold")
}
