package desktop

import (
	"context"
	"testing"
	"time"

	"github.com/openmodu/oneshot/internal/usecase/agentrun"
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
