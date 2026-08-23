package agentrun

import (
	"context"
	"errors"
	"testing"

	domainharnesses "github.com/openmodu/onecatch/internal/domain/harnesses"
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
	engine := NewEngine(Config{Binaries: map[string]string{
		"pi": "onecatch-absent-pi", "grok": "onecatch-absent-grok", "dsh": "onecatch-absent-dsh",
	}})
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

// Every catalogued harness must have a runner, and every runner must be
// catalogued. A harness in one but not the other is exactly how `grok` shipped
// selectable but unable to start: the engine could drive it while task
// validation rejected it.
func TestCatalogAndEngineAgree(t *testing.T) {
	engine := NewEngine(Config{})
	for _, harness := range domainharnesses.Catalog() {
		if engine.Runner(Runtime(harness.ID)) == nil {
			t.Fatalf("catalogued harness %q has no registered runner", harness.ID)
		}
		if !Runtime(harness.ID).Valid() {
			t.Fatalf("catalogued harness %q is not a valid runtime", harness.ID)
		}
	}
	for _, runtime := range engine.Runtimes() {
		if !domainharnesses.IsKnown(string(runtime)) {
			t.Fatalf("engine drives runtime %q, which the domain would reject as an invalid task", runtime)
		}
	}
	if len(engine.Runtimes()) != len(domainharnesses.Catalog()) {
		t.Fatalf("engine registers %d runtimes for %d catalogued harnesses", len(engine.Runtimes()), len(domainharnesses.Catalog()))
	}
}

// The catalog claims what each harness can do; the adapters have to agree, or a
// user is offered a control the run will reject.
func TestCatalogResumeClaimsMatchAdapters(t *testing.T) {
	stub := stubBinary(t, "", "", 0)
	engine := NewEngine(Config{Binaries: map[string]string{"dsh": stub}, DshSessionRoot: t.TempDir()})
	for _, harness := range domainharnesses.Catalog() {
		if harness.CanResume {
			continue
		}
		runner := engine.Runner(Runtime(harness.ID))
		_, err := runner.Run(context.Background(), Request{
			Prompt: "carry on", Workspace: t.TempDir(), ResumeSessionID: "session-1",
		}, nil)
		if err == nil {
			t.Fatalf("harness %q is catalogued as unable to resume but accepted one", harness.ID)
		}
	}
}
