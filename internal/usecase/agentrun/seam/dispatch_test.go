package seam

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// recordingExecutor stands in for a machine, so routing can be asserted
// without needing two of them.
type recordingExecutor struct {
	name string
	got  []Command
	out  Outcome
	err  error
}

func (e *recordingExecutor) Describe() string { return e.name }

func (e *recordingExecutor) Run(_ context.Context, cmd Command) (Outcome, error) {
	e.got = append(e.got, cmd)
	return e.out, e.err
}

func newTestSession(t *testing.T, root string) *Session {
	t.Helper()
	t.Setenv(DirEnv, t.TempDir())
	s, err := NewSession("test-run", Target{Root: root})
	if err != nil {
		t.Fatalf("new session: %v", err)
	}
	return s
}

// The model's command goes to the target, unwrapped, starting in the session's
// working directory. Nothing from the envelope's preamble travels with it.
func TestDispatchRoutesModelCallToTarget(t *testing.T) {
	s := newTestSession(t, "/srv/app")
	remote := &recordingExecutor{name: "target"}
	local := &recordingExecutor{name: "local"}
	d := &Dispatcher{Session: s, Remote: remote, Local: local}

	raw := buildEnvelope(`go build ./...`, "/tmp/claude-1111-cwd")
	code := d.Dispatch(context.Background(), []string{raw}, io.Discard, io.Discard)

	if code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
	if len(local.got) != 0 {
		t.Fatalf("a model command ran locally: %+v", local.got)
	}
	if len(remote.got) != 1 {
		t.Fatalf("target ran %d commands, want 1", len(remote.got))
	}
	if want := "go build ./..."; remote.got[0].Command != want {
		t.Errorf("Command = %q, want %q", remote.got[0].Command, want)
	}
	if remote.got[0].Dir != "/srv/app" {
		t.Errorf("Dir = %q, want the session's cwd", remote.got[0].Dir)
	}
}

// The harness's own shell use stays on this machine. Sending it to the target
// breaks every hook and plugin launch in the session.
func TestDispatchKeepsHarnessInvocationsLocal(t *testing.T) {
	s := newTestSession(t, "/srv/app")
	remote := &recordingExecutor{name: "target"}
	local := &recordingExecutor{name: "local"}
	d := &Dispatcher{Session: s, Remote: remote, Local: local}

	raw := `node "${CLAUDE_PLUGIN_ROOT}/scripts/session-lifecycle-hook.mjs" SessionStart`
	d.Dispatch(context.Background(), []string{raw}, io.Discard, io.Discard)

	if len(remote.got) != 0 {
		t.Fatalf("a harness hook was sent to the target: %+v", remote.got)
	}
	if len(local.got) != 1 || local.got[0].Command != raw {
		t.Fatalf("local ran %+v, want the hook verbatim", local.got)
	}
	// A hook runs where the harness put it, not in the target's directory.
	if local.got[0].Dir != "" {
		t.Errorf("Dir = %q, want the process's own working directory", local.got[0].Dir)
	}
}

func TestDispatchPropagatesExitCode(t *testing.T) {
	s := newTestSession(t, "/srv/app")
	remote := &recordingExecutor{name: "target", out: Outcome{ExitCode: 2}}
	d := &Dispatcher{Session: s, Remote: remote, Local: &recordingExecutor{name: "local"}}

	code := d.Dispatch(context.Background(), []string{buildEnvelope("false", "")}, io.Discard, io.Discard)
	if code != 2 {
		t.Errorf("exit = %d, want the command's own 2", code)
	}
}

// A target that could not be reached must not look like a command that failed:
// the agent's next move is completely different.
func TestDispatchReportsTransportFailureDistinctly(t *testing.T) {
	s := newTestSession(t, "/srv/app")
	remote := &recordingExecutor{name: "target", err: ErrTransport}
	d := &Dispatcher{Session: s, Remote: remote, Local: &recordingExecutor{name: "local"}}

	var stderr bytes.Buffer
	code := d.Dispatch(context.Background(), []string{buildEnvelope("true", "")}, io.Discard, &stderr)
	if code != ExitTransportFailure {
		t.Errorf("exit = %d, want %d", code, ExitTransportFailure)
	}
	if !strings.Contains(stderr.String(), "seam:") {
		t.Errorf("stderr did not explain the failure: %q", stderr.String())
	}
}

// An envelope shape the parser does not know still goes to the target — the
// safe direction — but says so, because it means the harness changed.
func TestDispatchWarnsOnUnknownEnvelope(t *testing.T) {
	s := newTestSession(t, "/srv/app")
	remote := &recordingExecutor{name: "target"}
	var warned []string
	d := &Dispatcher{
		Session: s, Remote: remote, Local: &recordingExecutor{name: "local"},
		Warn: func(msg string) { warned = append(warned, msg) },
	}
	raw := `source /Users/op/.claude/shell-snapshots/s.sh 2>/dev/null || true && something-new 'x'`
	d.Dispatch(context.Background(), []string{raw}, io.Discard, io.Discard)

	if len(remote.got) != 1 {
		t.Fatalf("an unrecognised envelope was not forwarded to the target: %+v", remote.got)
	}
	if len(warned) != 1 || !strings.Contains(warned[0], "conformance") {
		t.Errorf("warning = %v, want one pointing at the conformance suite", warned)
	}
}

// The end-to-end property everything else exists for: `cd` in one tool call is
// still in effect for the next one, and the harness's own record agrees.
//
// The local executor stands in for the target here, which exercises the real
// wrapping, the real marker extraction and the real session file — everything
// except the ssh hop.
func TestDispatchPersistsWorkingDirectoryAcrossCalls(t *testing.T) {
	// The root is resolved through symlinks up front: `pwd -P` reports the
	// physical path, and on macOS /var is a symlink to /private/var, so an
	// unresolved expectation fails for a reason that has nothing to do with
	// the code under test.
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	s := newTestSession(t, root)
	d := NewDispatcher(s)
	cwdFile := filepath.Join(t.TempDir(), "claude-abcd-cwd")

	// Call one changes directory.
	var out bytes.Buffer
	if code := d.Dispatch(context.Background(),
		[]string{buildEnvelope("cd sub && pwd", cwdFile)}, &out, io.Discard); code != 0 {
		t.Fatalf("first call exited %d; output: %q", code, out.String())
	}
	if got := strings.TrimSpace(out.String()); got != filepath.Join(root, "sub") {
		t.Errorf("first call printed %q, want %q", got, filepath.Join(root, "sub"))
	}
	// The session moved with it.
	if want := filepath.Join(root, "sub"); s.Cwd != want {
		t.Errorf("session cwd = %q, want %q", s.Cwd, want)
	}
	// And so did the harness's own record, which is the only thing that makes
	// the harness believe `cd` persisted.
	recorded, readErr := os.ReadFile(cwdFile)
	if readErr != nil {
		t.Fatalf("the harness's cwd file was never written: %v", readErr)
	}
	if got := strings.TrimSpace(string(recorded)); got != filepath.Join(root, "sub") {
		t.Errorf("cwd file = %q, want %q", got, filepath.Join(root, "sub"))
	}

	// Call two starts where call one finished — through a freshly loaded
	// session, exactly as a new process would see it.
	reloaded, loadErr := LoadSession("test-run")
	if loadErr != nil {
		t.Fatalf("reload session: %v", loadErr)
	}
	out.Reset()
	d2 := NewDispatcher(reloaded)
	if code := d2.Dispatch(context.Background(),
		[]string{buildEnvelope("pwd", "")}, &out, io.Discard); code != 0 {
		t.Fatalf("second call exited %d", code)
	}
	if got := strings.TrimSpace(out.String()); got != filepath.Join(root, "sub") {
		t.Errorf("second call ran in %q, want %q — `cd` did not persist", got, filepath.Join(root, "sub"))
	}
}

// A command's own output must never be confused with the wrapper's markers,
// including when the command prints something marker-shaped itself.
func TestExecutorSeparatesOutputFromMarkers(t *testing.T) {
	var stdout, stderr bytes.Buffer
	out, err := (&localExecutor{}).Run(context.Background(), Command{
		Command: `printf 'hello\n'; printf 'oops\n' >&2; exit 3`,
		Stdout:  &stdout, Stderr: &stderr,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if out.ExitCode != 3 {
		t.Errorf("ExitCode = %d, want 3", out.ExitCode)
	}
	if got := stdout.String(); got != "hello\n" {
		t.Errorf("stdout = %q, want %q — a marker leaked into the command's output", got, "hello\n")
	}
	if got := stderr.String(); got != "oops\n" {
		t.Errorf("stderr = %q, want %q", got, "oops\n")
	}
	// `exit` runs inside the subshell, so the working directory is not
	// reported. That is the documented trade for containing `exit`.
	if out.Cwd != "" {
		t.Logf("cwd reported despite an explicit exit: %q", out.Cwd)
	}
}

// The marker must survive being split across writes, which is what actually
// happens when a command's output arrives in pipe-sized pieces.
func TestMarkerWriterHandlesSplitWrites(t *testing.T) {
	var out bytes.Buffer
	w := newMarkerWriter(&out, "\nMARK")
	full := "some output\nMARK42\n"
	for i := 0; i < len(full); i++ {
		if _, err := w.Write([]byte(full[i : i+1])); err != nil {
			t.Fatal(err)
		}
	}
	value, ok := w.Finish()
	if !ok {
		t.Fatal("marker not found when written one byte at a time")
	}
	if value != "42" {
		t.Errorf("value = %q, want %q", value, "42")
	}
	if out.String() != "some output" {
		t.Errorf("passthrough = %q, want %q", out.String(), "some output")
	}
}

// Output longer than the held-back tail has to reach the caller as it is
// produced, not be buffered until the command exits.
func TestMarkerWriterStreamsPastTheHoldBack(t *testing.T) {
	var out bytes.Buffer
	w := newMarkerWriter(&out, "\nMARK")
	big := strings.Repeat("x", markerSlack*3)
	if _, err := w.Write([]byte(big)); err != nil {
		t.Fatal(err)
	}
	if out.Len() == 0 {
		t.Fatal("nothing was passed through before Finish: the writer is buffering everything")
	}
	if out.Len() >= len(big) {
		t.Fatalf("everything was passed through (%d bytes): nothing is held back for the marker", out.Len())
	}
	if _, ok := w.Finish(); ok {
		t.Error("found a marker that was never written")
	}
	if out.String() != big {
		t.Error("the stream was not reproduced byte for byte")
	}
}
