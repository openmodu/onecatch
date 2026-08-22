//go:build conformance

package agentrun

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/openmodu/onecatch/internal/usecase/agentrun/seam"
	"github.com/openmodu/onecatch/internal/usecase/agentrun/seam/seamtest"
)

// TestCodexRunnerRoutesTheCompleteEnvironment verifies the production runner,
// not only the protocol recorder: Codex starts onecatchsh as its exec-server,
// and the resulting command observes the target directory. Native fs methods
// are covered separately by ExecServer's deterministic tests.
func TestCodexRunnerRoutesTheCompleteEnvironment(t *testing.T) {
	binary, err := exec.LookPath("codex")
	if err != nil {
		t.Skip("codex is not installed; skipping the remote-run conformance")
	}

	dir := t.TempDir()
	codexHome := filepath.Join(dir, "codex-home")
	workspace := filepath.Join(dir, "workspace")
	target := filepath.Join(dir, "target")
	for _, directory := range []string{codexHome, workspace, target} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(t, filepath.Join(target, "TARGET_MARKER"), "")
	writeFile(t, filepath.Join(workspace, "LOCAL_MARKER"), "")

	t.Setenv(seam.DirEnv, filepath.Join(dir, "sessions"))
	t.Setenv(ShellBinaryEnv, buildShellBinary(t, dir))
	mock := seamtest.StartMock(seamtest.DialectResponses, "pwd && ls")
	defer mock.Close()
	writeFile(t, filepath.Join(codexHome, "config.toml"), fmt.Sprintf(`model = "seam-mock"
model_provider = "seam"

[model_providers.seam]
name = "seam"
base_url = %q
wire_api = "responses"
requires_openai_auth = false
supports_websockets = false
`, mock.BaseURL()))

	req := Request{
		Runtime: RuntimeCodex, Workspace: workspace,
		Prompt:  "Follow the tool-call instructions exactly.",
		Sandbox: SandboxWorkspaceWrite, RunID: "conformance-codex-remote",
		Remote: &seam.Target{Root: target},
		Environment: harnessEnvironment(map[string]string{
			"CODEX_HOME": codexHome,
		}),
		InterruptGrace: 5 * time.Second,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if _, err := NewCodexRunner(binary).Run(ctx, req, func(Event) {}); err != nil {
		t.Logf("run returned %v (the mock's verdict is what matters)", err)
	}
	if !mock.Wait(90 * time.Second) {
		t.Fatal("Codex never completed the scripted remote tool call")
	}
	out, _ := mock.Result()
	if !strings.Contains(out, "TARGET_MARKER") || strings.Contains(out, "LOCAL_MARKER") {
		t.Fatalf("Codex command did not run exclusively on the target:\n%s", out)
	}
	if _, err := seam.LoadSession("conformance-codex-remote"); err == nil {
		t.Error("the Codex seam session survived the run")
	}
}

// TestClaudeRunnerRunsCommandsOnTheTarget is the end-to-end assertion this
// whole mechanism exists for: a run configured with a target executes the
// model's commands there, and not in the directory the harness process was
// started in.
//
// The target is a second local directory rather than an ssh host, so the test
// needs no remote machine and can run in CI. That covers everything except the
// ssh hop itself: the launch flags, the session, the shell prefix, the envelope
// parse, the routing decision, the working-directory bookkeeping and the tool
// denial are all the same code either way.
func TestClaudeRunnerRunsCommandsOnTheTarget(t *testing.T) {
	binary, err := exec.LookPath("claude")
	if err != nil {
		t.Skip("claude is not installed; skipping the remote-run conformance")
	}

	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	workspace := filepath.Join(dir, "workspace") // where the harness process lives
	target := filepath.Join(dir, "target")       // where the agent's commands must run
	for _, d := range []string{filepath.Join(home, ".claude"), workspace, target} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(t, filepath.Join(home, ".claude", "settings.json"), `{"permissions":{}}`+"\n")
	// Two markers, so the assertion can tell the machines apart by name rather
	// than by path — a path comparison would pass even if the command had run
	// in the workspace and merely reported the target's directory.
	writeFile(t, filepath.Join(target, "TARGET_MARKER"), "")
	writeFile(t, filepath.Join(workspace, "LOCAL_MARKER"), "")

	t.Setenv(seam.DirEnv, filepath.Join(dir, "sessions"))
	t.Setenv(ShellBinaryEnv, buildShellBinary(t, dir))

	mock := seamtest.StartMock(seamtest.DialectAnthropic, "pwd && ls")
	defer mock.Close()

	req := Request{
		Runtime:   RuntimeClaude,
		Workspace: workspace,
		Prompt:    "Follow the tool-call instructions exactly.",
		Sandbox:   SandboxWorkspaceWrite,
		RunID:     "conformance-remote",
		Remote:    &seam.Target{Root: target},
		Environment: harnessEnvironment(map[string]string{
			"HOME":                     home,
			"ANTHROPIC_API_KEY":        "dummy",
			"ANTHROPIC_BASE_URL":       mock.BaseURL(),
			"CLAUDE_TELEMETRY_OPT_OUT": "1",
			"DO_NOT_TRACK":             "1",
		}),
		InterruptGrace: 5 * time.Second,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if _, err := NewClaudeRunner(binary).Run(ctx, req, func(Event) {}); err != nil {
		t.Logf("run returned %v (the mock's own verdict is what matters)", err)
	}
	if !mock.Wait(90 * time.Second) {
		t.Fatal("the harness never completed the scripted tool call")
	}

	out, _ := mock.Result()
	if !strings.Contains(out, "TARGET_MARKER") {
		t.Errorf("the command did not run on the target; it reported:\n%s", out)
	}
	if strings.Contains(out, "LOCAL_MARKER") {
		t.Errorf("the command ran in the local workspace — the seam is bypassed and the "+
			"agent is acting on this machine while believing it is on the target:\n%s", out)
	}

	// The other half of the contract: with no seam to redirect them, the
	// native file tools must not be offered to the model at all.
	for _, name := range deniedFileTools {
		if mock.SawTool(name) {
			t.Errorf("%s was advertised to the model on a remote run; it would act on this machine", name)
		}
	}

	// A run must not outlive its own state.
	if _, err := seam.LoadSession("conformance-remote"); err == nil {
		t.Error("the session file survived the run")
	}
}

// A read-only remote run can reach nothing at all, and saying so beats
// starting an agent that silently cannot act.
func TestRemoteSetupRejectsReadOnly(t *testing.T) {
	t.Setenv(seam.DirEnv, t.TempDir())
	_, err := setupRemoteClaude(Request{
		Sandbox: SandboxReadOnly,
		Remote:  &seam.Target{Root: "/srv/app"},
	})
	if err == nil {
		t.Fatal("a read-only remote run was accepted")
	}
	if !strings.Contains(err.Error(), "read-only") {
		t.Errorf("error did not explain the conflict: %v", err)
	}
}

// buildShellBinary compiles onecatchsh, which the harness will exec as its
// shell. Building it here rather than depending on a prior `task build` keeps
// the suite self-contained.
func buildShellBinary(t *testing.T, dir string) string {
	t.Helper()
	out := filepath.Join(dir, "onecatchsh")
	cmd := exec.Command("go", "build", "-o", out, "github.com/openmodu/onecatch/cmd/onecatchsh")
	if combined, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build onecatchsh: %v\n%s", err, combined)
	}
	return out
}

// harnessEnvironment builds a child environment from this process's, dropping
// anything that could let a scripted turn reach a real provider or read the
// developer's own harness configuration.
func harnessEnvironment(set map[string]string) []string {
	var env []string
	for _, kv := range os.Environ() {
		key, _, _ := strings.Cut(kv, "=")
		if key == "HOME" || key == "CODEX_HOME" || strings.HasPrefix(key, "OPENAI_") ||
			strings.HasPrefix(key, "ANTHROPIC_") || strings.HasPrefix(key, "CLAUDE_") {
			continue
		}
		env = append(env, kv)
	}
	for k, v := range set {
		env = append(env, k+"="+v)
	}
	return env
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
