package worker

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

type questionEngine struct{}

func (questionEngine) Available(agentrun.Runtime) bool { return true }
func (questionEngine) SupportsInteractivePermissions(agentrun.Runtime, agentrun.Sandbox) bool {
	return false
}
func (questionEngine) SupportsInteractiveUserInput(agentrun.Runtime) bool { return true }
func (questionEngine) Run(ctx context.Context, req agentrun.Request, sink agentrun.Sink) (agentrun.Result, error) {
	input := agentrun.UserInputRequest{ID: "codex-input/thread/turn/21", Async: true, WaitForDelivery: true, Questions: []agentrun.UserInputQuestion{{ID: "q", Question: "Scope?"}}}
	sink(agentrun.Event{Kind: agentrun.KindUserInputRequest, UserInput: &input})
	response, err := req.UserInputHandler(ctx, input)
	if err != nil {
		return agentrun.Result{}, err
	}
	response.Delivered <- nil
	sink(agentrun.Event{Kind: agentrun.KindUserInputResolved, UserInput: &input, UserInputResponse: &response})
	return agentrun.Result{Succeeded: true, FinalMessage: response.Answers["q"]}, nil
}
func TestRemoteUserInputRoundTrip(t *testing.T) {
	server := httptest.NewServer(NewServer("remote", "Remote", "secret", map[string]string{"project": t.TempDir()}, questionEngine{}, 1).Handler())
	defer server.Close()
	client := NewClient()
	config := Config{ID: "remote", BaseURL: server.URL, Token: "secret", Enabled: true}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	requested := make(chan string, 1)
	finished := make(chan error, 1)
	go func() {
		result, err := client.Execute(ctx, config, ExecuteRequest{RunID: "run-question", WorkspaceID: "project", Runtime: agentrun.RuntimeCodex, Sandbox: agentrun.SandboxReadOnly, Prompt: "review"}, func(e agentrun.Event) {
			if e.Kind == agentrun.KindUserInputRequest {
				requested <- e.UserInput.ID
			}
		})
		if err == nil && result.FinalMessage != "custom" {
			t.Errorf("result: %+v", result)
		}
		finished <- err
	}()
	var id string
	select {
	case id = <-requested:
	case <-ctx.Done():
		t.Fatal("no request")
	}
	response := agentrun.UserInputResponse{Answers: map[string]string{"q": " custom "}}
	if err := client.RespondUserInput(ctx, config, "wrong-run", id, response); err == nil {
		t.Fatal("cross-run answer accepted")
	}
	if err := client.RespondUserInput(ctx, config, "run-question", id, agentrun.UserInputResponse{}); err == nil {
		t.Fatal("incomplete answer accepted")
	}
	if err := client.RespondUserInput(ctx, config, "run-question", id, response); err != nil {
		t.Fatal(err)
	}
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	if err := client.RespondUserInput(ctx, config, "run-question", id, response); err == nil {
		t.Fatal("stale answer accepted")
	}
}
