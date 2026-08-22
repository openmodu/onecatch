//go:build conformance

// Conformance suite for the shell and exec-server seams.
//
// These tests answer one question per harness: when the model calls its shell
// tool, does the call arrive at the place we intercept, and in the shape our
// parser expects? Nothing else about a harness upgrade is as dangerous as a
// change here, because the failure is invisible from inside the agent — it
// reports that it acted on the target while the command ran on the operator's
// own machine.
//
// The suite spends no model tokens: an embedded mock model scripts exactly one
// tool call. It needs no remote host either — the recorders run what they are
// given locally, because what is under test is the shape of the interception,
// not the destination.
//
//	go test -tags conformance ./internal/usecase/agentrun/seam/
//
// Each test skips when its harness is not installed, so the suite is safe to
// wire into CI on machines that have neither.
package seam

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/openmodu/onecatch/internal/usecase/agentrun/seam/seamtest"
)

// probeTimeout bounds one scripted turn. A healthy run takes a few seconds;
// the bound exists for the harness that starts and then hangs, which must not
// hang the suite with it.
const probeTimeout = 90 * time.Second

func TestMain(m *testing.M) {
	// Dispatch before testing's flag parsing: the harness invokes this binary
	// with an envelope, not with test flags.
	switch os.Getenv(recorderRoleEnv) {
	case "shell":
		os.Exit(runShellRecorder())
	case "execserver":
		os.Exit(runExecServerRecorder())
	}
	os.Exit(m.Run())
}

// canaryCommand is what the mock instructs the harness to run.
//
// It carries a single quote on purpose. The harness escapes the model's
// command into its envelope, and the escaping is the part of the contract most
// likely to be got wrong by a parser written from a clean example. Asserting
// that the command survives the round trip byte for byte tests the quoting
// rules without having to know them.
func canaryCommand(marker string) string {
	return fmt.Sprintf("echo %s; echo \"it's quoted\"", marker)
}

var cwdFileRE = regexp.MustCompile(`^/.*claude-[0-9a-f]+-cwd$`)

// TestClaudeCodeShellSeam drives Claude Code through the mock and asserts the
// shape of what reaches CLAUDE_CODE_SHELL_PREFIX.
func TestClaudeCodeShellSeam(t *testing.T) {
	binary := lookHarness(t, "claude")
	self := testBinary(t)

	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	workspace := filepath.Join(dir, "workspace")
	recordFile := filepath.Join(dir, "records.jsonl")
	mustMkdirAll(t, filepath.Join(home, ".claude"), workspace)
	// A settings file with an empty permissions block keeps Claude Code from
	// attempting interactive first-run setup in a headless probe.
	mustWrite(t, filepath.Join(home, ".claude", "settings.json"), `{"permissions":{}}`+"\n")

	marker := "SEAM_CANARY_CLAUDE"
	command := canaryCommand(marker)
	mock := seamtest.StartMock(seamtest.DialectAnthropic, command)
	defer mock.Close()

	env := harnessEnv(func(key string) bool {
		// A live key or OAuth session could reach the real API and spend
		// quota; a stray CLAUDE_* could steer the headless run.
		return key == "HOME" ||
			strings.HasPrefix(key, "ANTHROPIC_") ||
			strings.HasPrefix(key, "CLAUDE_")
	})
	env = append(env,
		"HOME="+home,
		"ANTHROPIC_API_KEY=dummy",
		"ANTHROPIC_BASE_URL="+mock.BaseURL(),
		"CLAUDE_CODE_SHELL_PREFIX="+self,
		"CLAUDE_TELEMETRY_OPT_OUT=1",
		"DO_NOT_TRACK=1",
		recorderRoleEnv+"=shell",
		recordFileEnv+"="+recordFile,
	)

	runHarness(t, binary, []string{
		"--dangerously-skip-permissions",
		"-p", "Follow the tool-call instructions exactly.",
	}, env, workspace)

	if !mock.Wait(probeTimeout) {
		t.Fatal("the harness never completed the scripted tool call; " +
			"the mock's dialect or the harness's launch flags have drifted")
	}

	records := mustRecords(t, recordFile)
	var modelCalls, internalCalls int
	for _, r := range records {
		if len(r.Argv) == 0 {
			t.Errorf("recorded an invocation with no arguments at all")
			continue
		}
		// The contract: one argv element, no -c. A parser written for
		// `bash -c <envelope>` breaks the day this changes, silently.
		if len(r.Argv) != 1 {
			t.Errorf("invocation had %d arguments, want exactly 1: %q", len(r.Argv), r.Argv)
		}
		env := ParseClaudeCode(r.Argv[0])
		switch env.Kind {
		case KindModel:
			modelCalls++
			if env.Command != command {
				t.Errorf("model command did not survive the round trip:\n got: %q\nwant: %q",
					env.Command, command)
			}
			// Nothing local may ride along to the target.
			for _, leak := range []string{"shell-snapshots", "shopt ", "unalias", home} {
				if strings.Contains(env.Command, leak) {
					t.Errorf("forwarded command still carries %q: %q", leak, env.Command)
				}
			}
			if env.CwdFile == "" {
				t.Error("no cwd redirect in the envelope: `cd` will stop persisting " +
					"between tool calls, with no error anywhere")
			} else if !cwdFileRE.MatchString(env.CwdFile) {
				t.Errorf("cwd file %q does not look like the harness's own; "+
					"check the redirect is still being parsed", env.CwdFile)
			}
		case KindInternal:
			internalCalls++
			// These are the harness's own hooks and plugin launches. Routing
			// them to a target breaks every one of them: they name local
			// interpreters and local paths.
			if strings.Contains(r.Argv[0], "eval '") {
				t.Errorf("classified as internal but carries an eval wrapper: %q", r.Argv[0])
			}
		case KindUnknown:
			t.Errorf("unrecognised envelope shape, which routes to the target unparsed: %q",
				r.Argv[0])
		}
	}
	if modelCalls == 0 {
		t.Fatal("no model Bash call reached the shell prefix: the seam is bypassed")
	}
	t.Logf("claude %s: %d model call(s), %d internal invocation(s)",
		harnessVersion(t, binary), modelCalls, internalCalls)
}

// TestClaudeCodeDeniesLocalFileTools asserts that permissions.deny in a
// settings file still removes the native file tools in print mode.
//
// This is the safety property of exec mode. Those tools call straight into
// Node's fs with no seam to redirect them, so an agent that keeps them reads
// and writes the operator's disk while believing it is on the target. If deny
// ever stops working, exec mode is unsafe and the launcher must refuse.
func TestClaudeCodeDeniesLocalFileTools(t *testing.T) {
	binary := lookHarness(t, "claude")
	self := testBinary(t)

	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	workspace := filepath.Join(dir, "workspace")
	recordFile := filepath.Join(dir, "records.jsonl")
	mustMkdirAll(t, filepath.Join(home, ".claude"), workspace)
	mustWrite(t, filepath.Join(home, ".claude", "settings.json"), `{"permissions":{}}`+"\n")

	denied := []string{"Read", "Edit", "Write", "NotebookEdit", "Glob", "Grep"}
	settings := filepath.Join(dir, "deny.json")
	mustWrite(t, settings, `{"permissions":{"deny":["`+strings.Join(denied, `","`)+`"]}}`+"\n")

	// The mock replies with a tool call for whichever shell tool is
	// advertised; what matters here is the tool list in the request, which the
	// mock sees before it answers.
	mock := seamtest.StartMock(seamtest.DialectAnthropic, "echo SEAM_DENY_PROBE")
	defer mock.Close()

	env := harnessEnv(func(key string) bool {
		return key == "HOME" ||
			strings.HasPrefix(key, "ANTHROPIC_") ||
			strings.HasPrefix(key, "CLAUDE_")
	})
	env = append(env,
		"HOME="+home,
		"ANTHROPIC_API_KEY=dummy",
		"ANTHROPIC_BASE_URL="+mock.BaseURL(),
		"CLAUDE_CODE_SHELL_PREFIX="+self,
		"CLAUDE_TELEMETRY_OPT_OUT=1", "DO_NOT_TRACK=1",
		recorderRoleEnv+"=shell",
		recordFileEnv+"="+recordFile,
	)
	runHarness(t, binary, []string{
		"--settings", settings,
		"--dangerously-skip-permissions",
		"-p", "Follow the tool-call instructions exactly.",
	}, env, workspace)

	if !mock.Wait(probeTimeout) {
		t.Fatal("the harness never completed the scripted tool call")
	}
	for _, name := range denied {
		if mock.SawTool(name) {
			t.Errorf("%s was still advertised to the model despite permissions.deny; "+
				"exec mode would let the agent touch the local filesystem", name)
		}
	}
	if !mock.SawTool("Bash") {
		t.Error("Bash was not advertised: the deny list is too broad, or the tool was renamed")
	}
}

// TestCodexExecServerSeam drives Codex through the mock with an
// environments.toml pointing at this binary, and asserts the model's command
// arrives over the exec-server protocol.
func TestCodexExecServerSeam(t *testing.T) {
	binary := lookHarness(t, "codex")
	self := testBinary(t)

	dir := t.TempDir()
	codexHome := filepath.Join(dir, "codex-home")
	workspace := filepath.Join(dir, "workspace")
	recordFile := filepath.Join(dir, "records.jsonl")
	mustMkdirAll(t, codexHome, workspace)

	// The environment definition codex spawns. `program` + `args` is the stdio
	// transport; 0.149 also accepts a `url` for a WebSocket exec-server, which
	// this suite does not exercise.
	mustWrite(t, filepath.Join(codexHome, "environments.toml"), fmt.Sprintf(`default = "seam"
include_local = false

[[environments]]
id = "seam"
program = %q
env = { %s = "execserver", %s = %q }
`, self, recorderRoleEnv, recordFileEnv, recordFile))

	marker := "SEAM_CANARY_CODEX"
	command := canaryCommand(marker)
	mock := seamtest.StartMock(seamtest.DialectResponses, command)
	defer mock.Close()

	env := harnessEnv(func(key string) bool { return key == "OPENAI_API_KEY" })
	env = append(env, "CODEX_HOME="+codexHome)

	runHarness(t, binary, []string{
		"-c", `model_providers.seam.name="seam"`,
		"-c", fmt.Sprintf("model_providers.seam.base_url=%q", mock.BaseURL()),
		"-c", `model_providers.seam.wire_api="responses"`,
		"-c", `model_providers.seam.requires_openai_auth=false`,
		"-c", `model_providers.seam.supports_websockets=false`,
		"-c", `model_provider="seam"`,
		"-c", `model="seam-mock"`,
		"-c", `sandbox_mode="workspace-write"`,
		"-a", "never",
		"exec", "--ephemeral", "--skip-git-repo-check",
		"Follow the tool-call instructions exactly.",
	}, env, workspace)

	if !mock.Wait(probeTimeout) {
		t.Fatal("codex never completed the scripted tool call; the exec-server " +
			"seam or the environments.toml schema has drifted")
	}

	records := mustRecords(t, recordFile)
	if len(records) == 0 {
		t.Fatal("codex never spawned the environment program: environments.toml " +
			"was ignored, and every tool call ran on this machine instead")
	}
	methods := map[string]int{}
	var sawCommand bool
	for _, r := range records {
		methods[r.Method]++
		if r.Method != "process/start" {
			continue
		}
		var p struct {
			Argv []string `json:"argv"`
			Cwd  string   `json:"cwd"`
		}
		if err := json.Unmarshal(r.Params, &p); err != nil {
			t.Errorf("process/start params did not parse: %v", err)
			continue
		}
		// The observed shape is [shell, -lc, script]: codex derives the shell
		// from what environment/info reported and puts the model's command in
		// the last element, unmodified. A parser that assumed the command was
		// the whole argv joined would corrupt every command containing a space.
		if len(p.Argv) < 3 {
			t.Errorf("process/start argv = %q, want [shell, -lc, script]", p.Argv)
			continue
		}
		script := p.Argv[len(p.Argv)-1]
		if script != command {
			t.Errorf("the model's command did not survive the round trip:\n got: %q\nwant: %q",
				script, command)
		}
		// Paths cross this protocol as file:// URIs in both directions, so a
		// server handing back a plain path is rejected rather than misread.
		if !strings.HasPrefix(p.Cwd, "file://") {
			t.Errorf("cwd = %q, want a file:// URI", p.Cwd)
		}
		sawCommand = true
	}
	if methods["initialize"] == 0 {
		t.Error("codex never sent initialize; the protocol handshake changed")
	}
	if !sawCommand {
		t.Errorf("the model's command never arrived as process/start; methods seen: %v",
			sortedMethods(methods))
	}
	// The command arriving is half the contract; its output getting back to
	// the model is the other half. A seam that accepts commands and returns
	// nothing looks like a working agent producing empty results, which is
	// harder to diagnose than an outright failure.
	if out, ok := mock.Result(); !ok || !strings.Contains(out, marker) {
		t.Errorf("the command's output never reached the model (got %q); "+
			"codex accepted the command but the result path is broken", truncate(out, 400))
	}
	// Not a failure: a new method means codex grew a capability this recorder
	// answers with method-not-found, which is worth knowing before it becomes
	// a required one.
	t.Logf("codex %s exec-server methods: %v", harnessVersion(t, binary), sortedMethods(methods))
}

// --- helpers ---------------------------------------------------------------

func lookHarness(t *testing.T, name string) string {
	t.Helper()
	p, err := exec.LookPath(name)
	if err != nil {
		t.Skipf("%s is not installed; skipping its seam conformance", name)
	}
	return p
}

// testBinary is this test binary's own path, which the harnesses will exec.
func testBinary(t *testing.T) string {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatalf("locate the test binary: %v", err)
	}
	return self
}

// harnessEnv copies this process's environment, dropping what the probe must
// not inherit, and clears the recorder role so the harness's own children do
// not become recorders.
func harnessEnv(strip func(key string) bool) []string {
	var env []string
	for _, kv := range os.Environ() {
		key, _, _ := strings.Cut(kv, "=")
		if key == recorderRoleEnv || key == recordFileEnv || strip(key) {
			continue
		}
		env = append(env, kv)
	}
	return env
}

func runHarness(t *testing.T, binary string, args, env []string, dir string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Env = env
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	// The exit status is not asserted. A harness that made the tool call and
	// then exited non-zero for its own reasons has still answered the question
	// this suite asks; the mock's Wait is the real gate.
	if err != nil {
		t.Logf("%s exited with %v; output:\n%s", filepath.Base(binary), err, truncate(string(out), 4000))
	}
}

func harnessVersion(t *testing.T, binary string) string {
	t.Helper()
	out, err := exec.Command(binary, "--version").CombinedOutput()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
}

func mustRecords(t *testing.T, path string) []record {
	t.Helper()
	records, err := readRecords(path)
	if err != nil {
		t.Fatalf("read records: %v", err)
	}
	return records
}

func mustMkdirAll(t *testing.T, dirs ...string) {
	t.Helper()
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatalf("create %s: %v", d, err)
		}
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func sortedMethods(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "\n...[truncated]"
}
