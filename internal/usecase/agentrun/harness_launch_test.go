package agentrun

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The stub suite cannot catch a wrong command line: a shell stub accepts any
// argv, so every stub test passed while the real Grok binary rejected the
// invocation with "unexpected argument '--sandbox'". These tests check the
// argument vectors the adapters build against the harnesses themselves.
//
// They spend no model quota and need no credentials — each harness is asked
// something it can answer locally — and they skip when the harness is not
// installed, so the suite stays runnable on a bare machine.

// usageRejections are the ways a CLI says it did not understand our arguments.
var usageRejections = []string{"unexpected argument", "Unknown option", "unknown option", "unknown flag", "invalid value"}

func assertNoUsageRejection(t *testing.T, harness, output string) {
	t.Helper()
	for _, rejection := range usageRejections {
		if strings.Contains(output, rejection) {
			t.Fatalf("%s rejected the arguments the adapter builds:\n%s", harness, strings.TrimSpace(output))
		}
	}
}

// TestPiAcceptsTheArgumentsWeBuild runs the real pi with the adapter's own
// argument vector. Pi parses its flags before contacting a provider, so an
// unusable API key still proves the flags themselves were understood, and the
// assertion holds with or without network access.
func TestPiAcceptsTheArgumentsWeBuild(t *testing.T) {
	runner := NewPiRunner("")
	if !runner.Available() {
		t.Skip("pi CLI not installed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	args := piCommandArgs(Request{
		Prompt: "say hi", Sandbox: SandboxReadOnly,
		Model: "openai/gpt-4o-mini", ReasoningEffort: "low",
	})
	// --no-session keeps the probe from leaving a stored session behind.
	args = append([]string{"--no-session", "--api-key", "onecatch-probe-not-a-key"}, args...)
	cmd := exec.CommandContext(ctx, "pi", args...)
	cmd.Dir = t.TempDir()
	output, _ := cmd.CombinedOutput()
	assertNoUsageRejection(t, "pi", string(output))
}

// TestDshAcceptsTheArgumentsAndPatchWeBuild asks the real dsh to compose its
// profile with the adapter's own patch overlay and print the result. That is
// entirely local: it validates the launcher flags, the two `--` separators, and
// that the generated YAML is accepted and actually reconfigures the session log.
func TestDshAcceptsTheArgumentsAndPatchWeBuild(t *testing.T) {
	runner := NewDshRunner("", t.TempDir())
	if !runner.Available() {
		t.Skip("dsh CLI not installed")
	}
	root := t.TempDir()
	patchPath, cleanup, err := writeDshPatch(root, Request{Model: "deepseek-v4-flash"})
	if err != nil {
		t.Fatalf("write patch: %v", err)
	}
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	name, args := dshCommand("dsh", patchPath, "say hi")
	// Replace the task with the launcher's own config dump, which composes the
	// whole profile and exits without booting an agent.
	args = append(args[:len(args)-3], "--dump-config")
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = t.TempDir()
	output, err := cmd.CombinedOutput()
	assertNoUsageRejection(t, "dsh", string(output))
	if err != nil {
		t.Fatalf("dsh could not compose the profile with our patch: %v\n%s", err, strings.TrimSpace(string(output)))
	}
	composed := string(output)
	// The adapter recovers its event stream by reading this log, so the patch
	// has to actually land: plain text, unpacked, and under our own root.
	for _, want := range []string{"session-persistence-jsonl", "compression: none", "packChunks: false", root} {
		if !strings.Contains(composed, want) {
			t.Fatalf("composed profile is missing %q; the patch did not take effect", want)
		}
	}
}

// TestDshLaunchResolvesTheNodeScript pins the launch shape against the real
// installation: the published dsh command is a Node script whose loader refuses
// to start without --expose-internals, a flag NODE_OPTIONS will not carry.
func TestDshLaunchResolvesTheNodeScript(t *testing.T) {
	resolved, err := exec.LookPath("dsh")
	if err != nil {
		t.Skip("dsh CLI not installed")
	}
	target, err := filepath.EvalSymlinks(resolved)
	if err != nil {
		t.Fatalf("resolve dsh: %v", err)
	}
	name, args := dshCommand("dsh", "/tmp/patch.yml", "say hi")
	if !strings.HasSuffix(target, ".js") && !strings.HasSuffix(target, ".mjs") {
		// A future release shipping a real executable needs no Node wrapper.
		if name != "dsh" {
			t.Fatalf("a non-script dsh must launch directly, got %s", name)
		}
		return
	}
	if name != "node" || args[0] != "--expose-internals" {
		t.Fatalf("a script dsh must launch under node --expose-internals, got %s %v", name, args)
	}
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}
}

// TestGrokProbeAndRunShareOneLaunchShape guards against the two paths drifting:
// the model probe and a real run must build the same invocation, or a probe can
// succeed while every run fails.
func TestGrokProbeAndRunShareOneLaunchShape(t *testing.T) {
	command, err := grokCommand(Request{Sandbox: SandboxReadOnly})
	if err != nil {
		t.Fatalf("command: %v", err)
	}
	source, err := os.ReadFile("grok.go")
	if err != nil {
		t.Fatalf("read adapter: %v", err)
	}
	if strings.Count(string(source), `"agent"`) != 1 {
		t.Fatal("the launch shape is spelled more than once in grok.go; build it through grokCommand")
	}
	if command.args[0] != "agent" || command.args[len(command.args)-1] != "stdio" {
		t.Fatalf("unexpected launch shape %v", command.args)
	}
}
