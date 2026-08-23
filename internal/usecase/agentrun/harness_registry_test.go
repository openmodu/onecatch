package agentrun

import (
	"context"
	"errors"
	"testing"

	domainagents "github.com/openmodu/onecatch/internal/domain/agents"
)

func TestRuntimeValidityCoversEveryHarness(t *testing.T) {
	for _, runtime := range []Runtime{RuntimeCodex, RuntimeClaude, RuntimeModu, RuntimePi, RuntimeGrok, RuntimeDsh} {
		if !runtime.Valid() {
			t.Fatalf("runtime %q must be valid", runtime)
		}
	}
	if Runtime("gemini").Valid() {
		t.Fatal("an unregistered runtime must not be valid")
	}
}

func TestEngineRegistersEveryHarness(t *testing.T) {
	engine := NewEngine(Config{})
	for _, runtime := range []Runtime{RuntimeCodex, RuntimeClaude, RuntimeModu, RuntimePi, RuntimeGrok, RuntimeDsh} {
		if engine.Runner(runtime) == nil {
			t.Fatalf("no runner registered for %q", runtime)
		}
	}
}

func TestEngineRejectsUninstalledHarness(t *testing.T) {
	// Point every new harness at a binary that cannot resolve, so the engine
	// must fail fast rather than spawn anything.
	engine := NewEngine(Config{
		PiBinary:   "onecatch-absent-pi",
		GrokBinary: "onecatch-absent-grok",
		DshBinary:  "onecatch-absent-dsh",
	})
	for _, runtime := range []Runtime{RuntimePi, RuntimeGrok, RuntimeDsh} {
		if engine.Available(runtime) {
			t.Fatalf("runtime %q reported available with a missing binary", runtime)
		}
		_, err := engine.Run(context.Background(), Request{Runtime: runtime, Prompt: "hi", Workspace: t.TempDir()}, nil)
		var unavailable ErrRuntimeUnavailable
		if !errors.As(err, &unavailable) {
			t.Fatalf("runtime %q: err = %v, want ErrRuntimeUnavailable", runtime, err)
		}
	}
}

func TestAvailableRuntimesKeepsAStableOrder(t *testing.T) {
	stub := stubBinary(t, "", "", 0)
	engine := NewEngineWithRunners(
		NewPiRunner(stub),
		NewGrokRunner(stub),
		NewDshRunner(stub, t.TempDir()),
	)
	got := engine.AvailableRuntimes()
	want := []Runtime{RuntimePi, RuntimeGrok, RuntimeDsh}
	if len(got) != len(want) {
		t.Fatalf("available = %v, want %v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("available = %v, want %v", got, want)
		}
	}
}

// Whether a harness can pause for approval is a property of the harness and the
// sandbox, and hosts must be able to ask without testing the runtime id.
func TestInteractivePermissionCapability(t *testing.T) {
	stub := stubBinary(t, "", "", 0)
	engine := NewEngineWithRunners(
		NewClaudeRunner(stub), NewGrokRunner(stub), NewPiRunner(stub),
		NewDshRunner(stub, t.TempDir()), NewCodexRunner(stub),
	)
	for _, testCase := range []struct {
		runtime Runtime
		sandbox Sandbox
		want    bool
		why     string
	}{
		// Claude reaches its control channel only through the read-only launch
		// shape; a write-capable run skips permissions instead.
		{RuntimeClaude, SandboxReadOnly, true, "claude read-only"},
		{RuntimeClaude, SandboxWorkspaceWrite, false, "claude workspace-write"},
		// ACP carries permission requests in every mode, so Grok can ask while
		// it still has write access.
		{RuntimeGrok, SandboxReadOnly, true, "grok read-only"},
		{RuntimeGrok, SandboxWorkspaceWrite, true, "grok workspace-write"},
		// The full sandbox is blanket consent for unattended automation; there
		// is nobody watching to answer a card.
		{RuntimeGrok, SandboxFull, false, "grok full"},
		// Neither harness has a channel to ask through.
		{RuntimePi, SandboxWorkspaceWrite, false, "pi"},
		{RuntimeDsh, SandboxWorkspaceWrite, false, "dsh"},
		{RuntimeCodex, SandboxReadOnly, false, "codex"},
	} {
		if got := engine.SupportsInteractivePermissions(testCase.runtime, testCase.sandbox); got != testCase.want {
			t.Fatalf("%s: interactive permissions = %v, want %v", testCase.why, got, testCase.want)
		}
	}
	// An unregistered runtime must answer no rather than panic.
	if engine.SupportsInteractivePermissions(Runtime("gemini"), SandboxReadOnly) {
		t.Fatal("an unregistered runtime must not claim interactive permissions")
	}
}

// The domain layer cannot import agentrun, so it keeps its own list of harness
// identifiers for task and settings validation. A runtime added to the engine
// but missed there is accepted everywhere except the moment a user runs it —
// which is exactly how `grok` shipped able to be selected but not started.
func TestDomainRuntimesMatchEngine(t *testing.T) {
	engine := NewEngine(Config{})
	for _, id := range domainagents.KnownRuntimes {
		if !Runtime(id).Valid() {
			t.Fatalf("domain lists runtime %q, which the engine does not recognize", id)
		}
		if engine.Runner(Runtime(id)) == nil {
			t.Fatalf("domain lists runtime %q, which has no registered runner", id)
		}
	}
	for _, runtime := range []Runtime{RuntimeCodex, RuntimeClaude, RuntimeModu, RuntimePi, RuntimeGrok, RuntimeDsh} {
		if !domainagents.IsKnownRuntime(string(runtime)) {
			t.Fatalf("engine drives runtime %q, which the domain would reject as an invalid task", runtime)
		}
	}
}
