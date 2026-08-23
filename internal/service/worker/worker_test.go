package worker

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	domainworkspaces "github.com/openmodu/onecatch/internal/domain/workspaces"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

type fakeGit struct{ path string }

func (g *fakeGit) Inspect(_ context.Context, workspace string) (domainworkspaces.GitSnapshot, error) {
	g.path = workspace
	return domainworkspaces.GitSnapshot{IsRepo: true, Branch: "main", Files: []domainworkspaces.GitFile{{Path: "a.go", Worktree: "M"}}}, nil
}

type fakeEngine struct{}

func (fakeEngine) Available(runtime agentrun.Runtime) bool { return runtime == agentrun.RuntimeCodex }
func (fakeEngine) SupportsInteractivePermissions(agentrun.Runtime, agentrun.Sandbox) bool {
	return true
}
func (fakeEngine) Run(_ context.Context, request agentrun.Request, sink agentrun.Sink) (agentrun.Result, error) {
	sink(agentrun.Event{Kind: agentrun.KindMessage, Text: request.Workspace})
	return agentrun.Result{Succeeded: true, SessionID: "remote-session", FinalMessage: `{"signal":"completed","content":"remote done"}`}, nil
}

type capturingEngine struct{ requests chan agentrun.Request }

func (capturingEngine) Available(runtime agentrun.Runtime) bool {
	return runtime == agentrun.RuntimeCodex
}
func (capturingEngine) SupportsInteractivePermissions(agentrun.Runtime, agentrun.Sandbox) bool {
	return true
}
func (e capturingEngine) Run(_ context.Context, request agentrun.Request, sink agentrun.Sink) (agentrun.Result, error) {
	e.requests <- request
	sink(agentrun.Event{Kind: agentrun.KindMessage, Text: request.Workspace})
	return agentrun.Result{Succeeded: true, SessionID: "remote-session", FinalMessage: `{"signal":"completed","content":"remote done"}`}, nil
}

type writingEngine struct{}

func (writingEngine) Available(runtime agentrun.Runtime) bool {
	return runtime == agentrun.RuntimeCodex
}
func (writingEngine) SupportsInteractivePermissions(agentrun.Runtime, agentrun.Sandbox) bool {
	return true
}
func (writingEngine) Run(_ context.Context, request agentrun.Request, sink agentrun.Sink) (agentrun.Result, error) {
	if err := os.WriteFile(filepath.Join(request.Workspace, "tracked.txt"), []byte("changed remotely\n"), 0o644); err != nil {
		return agentrun.Result{}, err
	}
	if err := os.WriteFile(filepath.Join(request.Workspace, "created.txt"), []byte("created remotely\n"), 0o644); err != nil {
		return agentrun.Result{}, err
	}
	return agentrun.Result{Succeeded: true, FinalMessage: "done"}, nil
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
func (blockingEngine) SupportsInteractivePermissions(agentrun.Runtime, agentrun.Sandbox) bool {
	return true
}
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

func TestPinnedTLSAuthenticatesWorkerCertificate(t *testing.T) {
	server := httptest.NewTLSServer(NewServer("remote-1", "Remote", "secret", map[string]string{"project": "/tmp/project"}, fakeEngine{}, 1).Handler())
	defer server.Close()
	sum := sha256.Sum256(server.Certificate().Raw)
	config := Config{BaseURL: server.URL, Token: "secret", ServerCertificateSHA256: fmt.Sprintf("%x", sum[:])}
	health, err := NewClient().Health(context.Background(), config)
	if err != nil || health.WorkerID != "remote-1" || health.ProtocolVersion != 3 || !health.Capabilities["workspaceSync"] {
		t.Fatalf("health = %+v, %v", health, err)
	}
	config.ServerCertificateSHA256 = strings.Repeat("0", 64)
	if _, err := NewClient().Health(context.Background(), config); err == nil || !strings.Contains(err.Error(), "paired fingerprint") {
		t.Fatalf("wrong certificate pin error = %v", err)
	}
}

func TestPairingReturnsTokenAndPinsTLSCertificateOnce(t *testing.T) {
	service := NewServer("remote-1", "Remote", "secret", nil, fakeEngine{}, 1)
	service.EnablePairing("ABCD-2345", time.Now().Add(time.Minute), false)
	server := httptest.NewTLSServer(service.Handler())
	defer server.Close()

	client := NewClient()
	paired, err := client.Pair(context.Background(), server.URL, "ABCD-2345")
	if err != nil || paired.WorkerID != "remote-1" || paired.Token != "secret" || len(paired.ServerCertificateSHA256) != 64 {
		t.Fatalf("paired = %+v, %v", paired, err)
	}
	health, err := client.Health(context.Background(), Config{
		BaseURL: server.URL, Token: paired.Token, ServerCertificateSHA256: paired.ServerCertificateSHA256,
	})
	if err != nil || health.WorkerID != "remote-1" || !health.Capabilities["pairing"] {
		t.Fatalf("paired health = %+v, %v", health, err)
	}
	if _, err := client.Pair(context.Background(), server.URL, "ABCD-2345"); err == nil || !strings.Contains(err.Error(), "worker_pairing_invalid") {
		t.Fatalf("reused pairing code error = %v", err)
	}
}

func TestPairingRejectsPlainHTTPUnlessExplicitlyAllowed(t *testing.T) {
	service := NewServer("remote-1", "Remote", "secret", nil, fakeEngine{}, 1)
	service.EnablePairing("ABCD-2345", time.Now().Add(time.Minute), false)
	server := httptest.NewServer(service.Handler())
	defer server.Close()
	if _, err := NewClient().Pair(context.Background(), server.URL, "ABCD-2345"); err == nil || !strings.Contains(err.Error(), "worker_pairing_requires_tls") {
		t.Fatalf("plain HTTP pairing error = %v", err)
	}
}

func TestServerAuthenticatesAndStreamsMappedWorkspace(t *testing.T) {
	t.Setenv("ONECATCH_WORKER_TEST_ENV", "remote-value")
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
	result, err := client.Execute(context.Background(), config, ExecuteRequest{RunID: "run-1", WorkspaceID: "project", Runtime: agentrun.RuntimeCodex, Model: "gpt-test", ReasoningEffort: "high", ServiceTier: "priority", Provider: "anthropic", Sandbox: agentrun.SandboxReadOnly, Prompt: "review", EnvironmentAllowlist: []string{"ONECATCH_WORKER_TEST_ENV"}, TimeoutSeconds: 60}, func(e agentrun.Event) { events = append(events, e) })
	if err != nil || !result.Succeeded || len(events) != 1 || events[0].Text != "/tmp/project" {
		t.Fatalf("execute result=%+v events=%+v err=%v", result, events, err)
	}
	request := <-engine.requests
	if request.Model != "gpt-test" || request.ReasoningEffort != "high" || request.ServiceTier != "priority" || request.Provider != "anthropic" || !request.RuntimeDefaultsResolved {
		t.Fatalf("remote model settings = %+v", request)
	}
	if !containsEnvironment(request.Environment, "ONECATCH_WORKER_TEST_ENV=remote-value") {
		t.Fatalf("remote environment = %#v", request.Environment)
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

func TestServerEnforcesRunTimeout(t *testing.T) {
	engine := newBlockingEngine(t)
	server := httptest.NewServer(NewServer("remote-1", "Remote", "secret", map[string]string{"project": "/tmp/project"}, engine, 1).Handler())
	defer server.Close()
	client := NewClient()
	config := Config{ID: "remote-1", BaseURL: server.URL, Token: "secret", Enabled: true}

	startedAt := time.Now()
	_, err := client.Execute(context.Background(), config, ExecuteRequest{RunID: "run-timeout", WorkspaceID: "project", Runtime: agentrun.RuntimeCodex, Sandbox: agentrun.SandboxReadOnly, Prompt: "long", TimeoutSeconds: 1}, nil)
	var remote RemoteError
	if !errors.As(err, &remote) || remote.Code != "worker_execution_failed" {
		t.Fatalf("timeout error = %v", err)
	}
	if elapsed := time.Since(startedAt); elapsed > 3*time.Second {
		t.Fatalf("timeout took %s", elapsed)
	}
}

type permissionEngine struct {
	decision chan agentrun.PermissionDecision
}

func (permissionEngine) Available(runtime agentrun.Runtime) bool {
	return runtime == agentrun.RuntimeClaude
}
func (permissionEngine) SupportsInteractivePermissions(agentrun.Runtime, agentrun.Sandbox) bool {
	return true
}
func (e permissionEngine) Run(ctx context.Context, request agentrun.Request, sink agentrun.Sink) (agentrun.Result, error) {
	permission := agentrun.PermissionRequest{
		ID: "permission-1", ToolName: "Bash",
		Suggestions: []agentrun.PermissionUpdate{{"type": "addRules"}},
	}
	sink(agentrun.Event{Kind: agentrun.KindPermissionRequest, Permission: &permission})
	decision, err := request.PermissionHandler(ctx, permission)
	if err != nil {
		return agentrun.Result{}, err
	}
	e.decision <- decision
	sink(agentrun.Event{Kind: agentrun.KindPermissionResolved, Permission: &permission, PermissionDecision: decision.Behavior})
	return agentrun.Result{Succeeded: true}, nil
}

func TestRemotePermissionDecisionRoundTrip(t *testing.T) {
	engine := permissionEngine{decision: make(chan agentrun.PermissionDecision, 1)}
	server := httptest.NewServer(NewServer("remote-1", "Remote", "secret", map[string]string{"project": "/tmp/project"}, engine, 1).Handler())
	defer server.Close()
	client := NewClient()
	config := Config{ID: "remote-1", BaseURL: server.URL, Token: "secret", Enabled: true}
	requested := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		_, err := client.Execute(context.Background(), config, ExecuteRequest{
			RunID: "run-permission", WorkspaceID: "project", Runtime: agentrun.RuntimeClaude,
			Sandbox: agentrun.SandboxReadOnly, Prompt: "review",
		}, func(event agentrun.Event) {
			if event.Kind == agentrun.KindPermissionRequest {
				close(requested)
			}
		})
		result <- err
	}()
	select {
	case <-requested:
	case <-time.After(5 * time.Second):
		t.Fatal("permission request was not streamed")
	}
	if err := client.RespondPermission(context.Background(), config, "run-permission", "permission-1", "allow_always"); err != nil {
		t.Fatalf("respond permission: %v", err)
	}
	select {
	case decision := <-engine.decision:
		if decision.Behavior != "allow" || decision.DecisionClassification != "user_permanent" || len(decision.UpdatedPermissions) != 1 {
			t.Fatalf("decision = %+v", decision)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("permission decision was not delivered")
	}
	if err := <-result; err != nil {
		t.Fatalf("execute: %v", err)
	}
}

func TestWritableRunSynchronizesThenCleansRemoteWorkspace(t *testing.T) {
	root := t.TempDir()
	local := filepath.Join(root, "local")
	remote := filepath.Join(root, "remote")
	if err := os.Mkdir(local, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, local, "init")
	runGit(t, local, "config", "user.email", "worker-test@example.com")
	runGit(t, local, "config", "user.name", "Worker Test")
	if err := os.WriteFile(filepath.Join(local, "tracked.txt"), []byte("original\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, local, "add", "tracked.txt")
	runGit(t, local, "commit", "-m", "initial")
	command := exec.Command("git", "clone", "--quiet", local, remote)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("clone: %v: %s", err, output)
	}

	server := httptest.NewServer(NewServer("remote-1", "Remote", "secret", map[string]string{"project": remote}, writingEngine{}, 1).Handler())
	defer server.Close()
	client := NewClient()
	config := Config{ID: "remote-1", BaseURL: server.URL, Token: "secret", Enabled: true}
	baseRevision, err := WorkspaceBaseline(context.Background(), local)
	if err != nil {
		t.Fatal(err)
	}
	result, patch, err := client.ExecuteWithPatch(context.Background(), config, ExecuteRequest{
		RunID: "run-write", WorkspaceID: "project", Runtime: agentrun.RuntimeCodex,
		Sandbox: agentrun.SandboxWorkspaceWrite, Prompt: "edit",
		BaseRevision: baseRevision, SyncChanges: true,
	}, nil)
	if err != nil || !result.Succeeded || patch == nil {
		t.Fatalf("execute result=%+v patch=%+v err=%v", result, patch, err)
	}
	if status := runGit(t, remote, "status", "--porcelain"); strings.TrimSpace(status) == "" {
		t.Fatal("remote changes were cleaned before acknowledgement")
	}
	if err := ApplyWorkspacePatch(context.Background(), local, *patch); err != nil {
		t.Fatalf("apply patch: %v", err)
	}
	if got, _ := os.ReadFile(filepath.Join(local, "tracked.txt")); string(got) != "changed remotely\n" {
		t.Fatalf("local tracked content = %q", got)
	}
	if got, _ := os.ReadFile(filepath.Join(local, "created.txt")); string(got) != "created remotely\n" {
		t.Fatalf("local created content = %q", got)
	}
	if err := client.AckPatch(context.Background(), config, "run-write", patch.Digest); err != nil {
		t.Fatalf("ack patch: %v", err)
	}
	if err := client.AckPatch(context.Background(), config, "run-write", patch.Digest); err != nil {
		t.Fatalf("idempotent ack patch: %v", err)
	}
	if status := runGit(t, remote, "status", "--porcelain"); strings.TrimSpace(status) != "" {
		t.Fatalf("remote status after acknowledgement = %q", status)
	}
	if _, err := os.Stat(filepath.Join(remote, "created.txt")); !os.IsNotExist(err) {
		t.Fatalf("remote created file was not cleaned: %v", err)
	}
}

func runGit(t *testing.T, directory string, args ...string) string {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = directory
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
	return string(output)
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

func containsEnvironment(environment []string, value string) bool {
	for _, item := range environment {
		if item == value {
			return true
		}
	}
	return false
}
