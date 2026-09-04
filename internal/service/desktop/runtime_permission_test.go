package desktop

import (
	"context"
	"testing"
	"time"

	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

func TestRuntimeRegistryResolvesPendingPermission(t *testing.T) {
	registry := &RuntimeRegistry{pendingPermissions: make(map[string]*pendingPermission)}
	request := agentrun.PermissionRequest{
		ID: "permission-1", ToolName: "WebFetch",
		Suggestions: []agentrun.PermissionUpdate{{"type": "addRules"}},
	}
	result := make(chan agentrun.PermissionDecision, 1)
	go func() {
		decision, _ := registry.awaitPermission(context.Background(), "run-1", "step-1", request)
		result <- decision
	}()

	deadline := time.Now().Add(time.Second)
	for {
		registry.permissionMu.Lock()
		pending := registry.pendingPermissions[request.ID] != nil
		registry.permissionMu.Unlock()
		if pending {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("permission was not registered")
		}
		time.Sleep(time.Millisecond)
	}
	if err := registry.ResolvePermission("another-run", request.ID, "allow_once"); err == nil {
		t.Fatal("another run resolved the permission")
	}
	if err := registry.ResolvePermission("run-1", request.ID, "allow_always"); err != nil {
		t.Fatal(err)
	}
	select {
	case decision := <-result:
		if decision.Behavior != "allow" || decision.DecisionClassification != "user_permanent" || len(decision.UpdatedPermissions) != 1 {
			t.Fatalf("decision = %+v", decision)
		}
	case <-time.After(time.Second):
		t.Fatal("permission decision was not delivered")
	}
}

func TestRuntimeRegistryResolvesPendingUserInput(t *testing.T) {
	registry := &RuntimeRegistry{pendingUserInputs: make(map[string]*pendingUserInput)}
	request := agentrun.UserInputRequest{ID: "input-1", Questions: []agentrun.UserInputQuestion{
		{ID: "base", Question: "Where from?", Options: []agentrun.UserInputOption{{Label: "main"}, {Label: "current"}}},
		{ID: "name", Question: "Which name?", Options: []agentrun.UserInputOption{{Label: "feature"}, {Label: "fix"}}},
	}}
	result := make(chan agentrun.UserInputResponse, 1)
	go func() {
		response, _ := registry.awaitUserInput(context.Background(), "run-1", "step-1", request)
		result <- response
	}()
	waitUntil := time.Now().Add(time.Second)
	for {
		registry.userInputMu.Lock()
		pending := registry.pendingUserInputs[request.ID] != nil
		registry.userInputMu.Unlock()
		if pending {
			break
		}
		if time.Now().After(waitUntil) {
			t.Fatal("user input was not registered")
		}
		time.Sleep(time.Millisecond)
	}
	if err := registry.ResolveUserInput("run-1", request.ID, agentrun.UserInputResponse{Answers: map[string]string{"base": "main"}}); err == nil {
		t.Fatal("incomplete answers were accepted")
	}
	if err := registry.ResolveUserInput("another-run", request.ID, agentrun.UserInputResponse{Cancelled: true}); err == nil {
		t.Fatal("another run resolved the input")
	}
	if err := registry.ResolveUserInput("run-1", request.ID, agentrun.UserInputResponse{Answers: map[string]string{"base": " main ", "name": "custom-name", "extra": "ignored"}}); err != nil {
		t.Fatal(err)
	}
	select {
	case response := <-result:
		if response.Cancelled || response.Answers["base"] != "main" || response.Answers["name"] != "custom-name" || len(response.Answers) != 2 {
			t.Fatalf("response = %+v", response)
		}
	case <-time.After(time.Second):
		t.Fatal("user input response was not delivered")
	}
}
