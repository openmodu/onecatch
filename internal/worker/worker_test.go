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

	"github.com/openmodu/oneshot/internal/agentrun"
)

type fakeEngine struct{}

func (fakeEngine) Available(runtime agentrun.Runtime) bool { return runtime == agentrun.RuntimeCodex }
func (fakeEngine) Run(_ context.Context, request agentrun.Request, sink agentrun.Sink) (agentrun.Result, error) {
	sink(agentrun.Event{Kind: agentrun.KindMessage, Text: request.Workspace})
	return agentrun.Result{Succeeded: true, SessionID: "remote-session", FinalMessage: `{"signal":"completed","content":"remote done"}`}, nil
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

func TestServerAuthenticatesAndExecutesMappedWorkspace(t *testing.T) {
	server := httptest.NewServer(NewServer("remote-1", "Remote", "secret", map[string]string{"project": "/tmp/project"}, fakeEngine{}).Handler())
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
	response, err := client.Execute(context.Background(), config, ExecuteRequest{WorkspaceID: "project", Runtime: agentrun.RuntimeCodex, Sandbox: agentrun.SandboxReadOnly, Prompt: "review"})
	if err != nil || !response.Result.Succeeded || len(response.Events) != 1 || response.Events[0].Text != "/tmp/project" {
		t.Fatalf("execute = %+v, %v", response, err)
	}
	_, err = client.Execute(context.Background(), config, ExecuteRequest{WorkspaceID: "missing", Runtime: agentrun.RuntimeCodex, Prompt: "review"})
	var remote RemoteError
	if !errors.As(err, &remote) || remote.Code != "worker_workspace_unmapped" {
		t.Fatalf("unmapped error = %v", err)
	}
}
