package tasks

import (
	"testing"

	domainagents "github.com/openmodu/onecatch/internal/domain/agents"
)

func validTask() Task {
	return Task{ID: "task-1", WorkspaceID: "workspace-1", Title: "Harness picker", Prompt: "Run the task", WorkflowID: "single_agent", Status: StatusReady}
}

func TestValidateAcceptsKnownHarnesses(t *testing.T) {
	for _, harness := range []string{"codex", "claude", "modu"} {
		t.Run(harness, func(t *testing.T) {
			task := validTask()
			task.Harness = harness
			task.Model = "configured-model"
			if err := Validate(task); err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}
}

func TestValidateRejectsUnknownHarness(t *testing.T) {
	task := validTask()
	task.Harness = "unknown"
	if err := Validate(task); err != ErrInvalid {
		t.Fatalf("Validate() error = %v, want %v", err, ErrInvalid)
	}
}

func TestValidateRejectsRuntimeOverridesWithoutHarness(t *testing.T) {
	task := validTask()
	task.Model = "configured-model"
	if err := Validate(task); err != ErrInvalid {
		t.Fatalf("Validate() error = %v, want %v", err, ErrInvalid)
	}
}

func TestValidateAcceptsKnownTaskSandboxes(t *testing.T) {
	for _, sandbox := range []string{"read-only", "workspace-write", "full"} {
		task := validTask()
		task.Sandbox = sandbox
		if err := Validate(task); err != nil {
			t.Fatalf("Validate(%q) error = %v", sandbox, err)
		}
	}
}

func TestValidateRejectsUnknownTaskSandbox(t *testing.T) {
	task := validTask()
	task.Sandbox = "host-unrestricted"
	if err := Validate(task); err != ErrInvalid {
		t.Fatalf("Validate() error = %v, want %v", err, ErrInvalid)
	}
}

// A harness the engine can drive must not be rejected at task creation: that
// combination shows the runtime in the picker and then fails with
// "task is invalid" the moment it is used.
func TestValidateAcceptsEveryKnownHarness(t *testing.T) {
	for _, harness := range domainagents.KnownRuntimes {
		task := validTask()
		task.Harness = harness
		if err := Validate(task); err != nil {
			t.Fatalf("harness %q was rejected: %v", harness, err)
		}
	}
}
