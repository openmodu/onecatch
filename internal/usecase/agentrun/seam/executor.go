package seam

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/openmodu/onecatch/internal/sshcredentials"
	"github.com/openmodu/onecatch/internal/sshendpoint"
)

var shellVariableRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Outcome is what running one command produced.
type Outcome struct {
	// ExitCode is the command's own status. A failing command is data the
	// agent must be able to reason about, so it is never reported as an error.
	ExitCode int
	// Cwd is the working directory the command left behind, which becomes the
	// starting directory of the next one.
	Cwd string
}

// Command is one shell command to run.
type Command struct {
	Command string
	Dir     string
	// Env is added to the target shell for this command. It is deliberately
	// explicit: the process running the local harness may hold provider keys
	// and other credentials which must never be inherited by an SSH command.
	Env map[string]string
	// Stdin is nil for ordinary one-shot commands. Codex's exec-server uses a
	// pipe here for interactive processes and feeds it through process/write.
	Stdin   io.Reader
	Stdout  io.Writer
	Stderr  io.Writer
	Timeout time.Duration
}

// Executor runs a shell command somewhere.
//
// An error means the executor itself failed and the command's fate is
// unknown — the connection dropped, ssh could not authenticate. That is a
// different thing from a command that ran and exited non-zero, and conflating
// them is expensive in both directions: a transport failure reported as a
// command failure sends the agent chasing a bug that does not exist, and a
// command failure reported as a transport failure invites a retry of something
// that already had its effect.
type Executor interface {
	Run(ctx context.Context, cmd Command) (Outcome, error)
	Describe() string
}

// NewExecutor returns the executor for a target: SSH when it names a host,
// local otherwise. A local target is for tests and for running an agent
// against this machine through the same code path the remote one uses.
func NewExecutor(t Target) Executor {
	if strings.TrimSpace(t.Host) == "" {
		return &localExecutor{}
	}
	return &sshExecutor{target: t}
}

// --- command wrapping ------------------------------------------------------

// wrapped builds the shell program that runs cmd and reports both its exit
// status and its resulting working directory in one round trip.
//
// Two markers come back out of band. The exit status rides on stdout because
// ssh reports its *own* failures as exit 255, indistinguishable from a command
// that genuinely exited 255; carrying the real status behind an unguessable
// marker makes the marker's absence the signal that the transport failed. The
// working directory rides on stderr because there is nowhere else for it to
// live: the harness starts a new process per tool call, so `cd` only persists
// if each call reports where it ended up.
//
// The command runs inside a subshell so that an `exit` in it — which agents
// write routinely — terminates the command rather than the wrapper, which
// would swallow the status marker and make an ordinary non-zero exit look like
// a broken connection. The cost is that `exit` also skips the working-directory
// report, so a command that exits explicitly does not move the session's cwd.
// That is the right way round: a missed `cd` is visible and harmless, a
// misreported exit status is neither.
func wrapped(cmd Command, statusMarker, cwdMarker string) string {
	inner := cmd.Command
	if len(cmd.Env) > 0 {
		keys := make([]string, 0, len(cmd.Env))
		for key := range cmd.Env {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		var exports strings.Builder
		for _, key := range keys {
			if !shellVariableRE.MatchString(key) {
				continue
			}
			exports.WriteString("export ")
			exports.WriteString(key)
			exports.WriteByte('=')
			exports.WriteString(shellQuote(cmd.Env[key]))
			exports.WriteByte('\n')
		}
		inner = exports.String() + inner
	}
	if cmd.Dir != "" {
		// Chained with && so a missing directory fails loudly rather than
		// running the command somewhere unintended.
		inner = "cd " + shellQuote(cmd.Dir) + " && " + inner
	}
	inner += "\n__oc_c=$?; printf '\\n%s%s\\n' " + shellQuote(cwdMarker) + " \"$(pwd -P)\" >&2; exit $__oc_c"
	return "( " + inner + "\n); __oc_rc=$?; printf '\\n%s%d\\n' " + shellQuote(statusMarker) + " \"$__oc_rc\""
}

// newMarker returns an unguessable marker. It must be unguessable rather than
// merely unusual: a command that printed a fixed marker could forge its own
// exit status.
func newMarker(kind string) string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "__oc_" + kind + strconv.FormatInt(time.Now().UnixNano(), 16) + "__"
	}
	return "__oc_" + kind + "_" + hex.EncodeToString(b[:]) + "__"
}

// shellQuote renders s as a single POSIX shell word.
//
// The '"'"' idiom is used rather than backslash escaping, which is not
// portable inside single quotes. The empty string must still produce a word.
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if !strings.ContainsAny(s, "\\'\"`${}[]()|&;<>*?!~# \t\n") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

// --- marker extraction -----------------------------------------------------

// markerWriter passes a stream through while holding back enough of its tail
// to recognise a trailing marker, which it strips and reports.
//
// Holding back a bounded tail rather than buffering the whole stream is what
// lets a long build's output reach the agent as it is produced, and keeps a
// command that prints without limit from being held entirely in memory.
type markerWriter struct {
	out    io.Writer
	marker []byte
	hold   int

	mu      sync.Mutex
	pending []byte
	value   string
	found   bool
	err     error
}

// markerSlack is how much room past the marker is reserved for its value: a
// path, or an exit status. Paths are bounded by PATH_MAX on any target reach
// of this design can talk to.
const markerSlack = 4096

func newMarkerWriter(out io.Writer, marker string) *markerWriter {
	return &markerWriter{
		out:    out,
		marker: []byte(marker),
		hold:   len(marker) + markerSlack,
	}
}

func (w *markerWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pending = append(w.pending, p...)
	if len(w.pending) > w.hold {
		flush := len(w.pending) - w.hold
		if _, err := w.write(w.pending[:flush]); err != nil {
			w.err = err
		}
		w.pending = w.pending[flush:]
	}
	// The full length is reported even when the downstream writer failed: a
	// short write would make the producing command block or die, which is not
	// something a display-side filter should ever cause.
	return len(p), nil
}

func (w *markerWriter) write(b []byte) (int, error) {
	if w.out == nil {
		return len(b), nil
	}
	return w.out.Write(b)
}

// Finish flushes what is held back, having removed the marker, and reports the
// value that followed it.
func (w *markerWriter) Finish() (string, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	idx := bytes.LastIndex(w.pending, w.marker)
	if idx < 0 {
		if len(w.pending) > 0 {
			_, _ = w.write(w.pending)
			w.pending = nil
		}
		return "", false
	}
	tail := w.pending[idx+len(w.marker):]
	if end := bytes.IndexByte(tail, '\n'); end >= 0 {
		tail = tail[:end]
	}
	if idx > 0 {
		_, _ = w.write(w.pending[:idx])
	}
	w.pending = nil
	w.value, w.found = strings.TrimSpace(string(tail)), true
	return w.value, true
}

// --- local ----------------------------------------------------------------

type localExecutor struct{}

func (l *localExecutor) Describe() string { return "local" }

func (l *localExecutor) Run(ctx context.Context, cmd Command) (Outcome, error) {
	status, cwdMark := newMarker("rc"), newMarker("cwd")
	program := wrapped(cmd, status, cwdMark)
	return runShell(ctx, cmd, func(c context.Context) *exec.Cmd {
		return exec.CommandContext(c, "/bin/sh", "-c", program)
	}, status, cwdMark, "local shell")
}

// --- ssh ------------------------------------------------------------------

type sshExecutor struct{ target Target }

func (s *sshExecutor) Describe() string { return s.target.String() }

func (s *sshExecutor) Run(ctx context.Context, cmd Command) (Outcome, error) {
	status, cwdMark := newMarker("rc"), newMarker("cwd")
	program := remotePOSIXShellProgram(wrapped(cmd, status, cwdMark))
	endpoint, err := sshendpoint.Parse(s.target.Host)
	if err != nil {
		return Outcome{}, fmt.Errorf("%w (ssh %s): %v", ErrTransport, s.target.Host, err)
	}

	binary := s.target.SSHBinary
	if binary == "" {
		binary = "ssh"
	}
	// The command must be exactly one argv element: ssh joins several trailing
	// arguments with spaces before handing them to the remote shell, which
	// would silently re-split anything containing whitespace.
	args := append(s.sshArgs(endpoint.Port), endpoint.Host, program)
	var authenticationEnvironment []string
	if s.target.CredentialID != "" {
		configured := exec.Command(binary)
		if err := sshcredentials.ConfigureCommand(configured, s.target.CredentialID, s.target.AskPassBinary); err != nil {
			return Outcome{}, fmt.Errorf("%w (ssh %s): configure password authentication: %v", ErrTransport, s.target.Host, err)
		}
		authenticationEnvironment = configured.Env
	}
	return runShell(ctx, cmd, func(c context.Context) *exec.Cmd {
		process := exec.CommandContext(c, binary, args...)
		process.Env = authenticationEnvironment
		return process
	}, status, cwdMark, "ssh "+s.target.Host)
}

// remotePOSIXShellProgram makes the command independent of the account's
// login shell. OpenSSH passes its remote command through that shell first, and
// shells such as fish reject the POSIX assignments used by wrapped. Keep the
// outer command deliberately simple, then let /bin/sh parse the actual seam
// program.
func remotePOSIXShellProgram(program string) string {
	return "exec /bin/sh -c " + shellQuote(program)
}

func (s *sshExecutor) sshArgs(port int) []string {
	batchMode := "yes"
	if s.target.CredentialID != "" {
		batchMode = "no"
	}
	args := []string{
		// Every connection after the first runs unattended, so a prompt it
		// cannot answer must fail rather than hang a tool call invisibly.
		"-o", "BatchMode=" + batchMode,
		"-o", "ClearAllForwardings=yes",
		// Agent forwarding stays off. On a host with a hostile root, a
		// forwarded agent socket lets that host authenticate as the operator
		// everywhere else they can reach — a far larger blast radius than the
		// session they thought they opened.
		"-o", "ForwardAgent=no",
		// Do not let a broad SendEnv rule in ~/.ssh/config copy the local
		// harness's provider credentials or machine-specific environment onto
		// the target. Explicit process/start env is handled separately.
		"-o", "SendEnv=-*",
		"-o", "ConnectTimeout=10",
		// Detect a dead link instead of blocking forever on a half-open
		// connection: the agent must see a timeout it can retry, not a tool
		// call that never returns.
		"-o", "ServerAliveInterval=15",
		"-o", "ServerAliveCountMax=3",
		"-T",
	}
	if port != 0 {
		args = append(args, "-p", fmt.Sprintf("%d", port))
	}
	if s.target.CredentialID != "" {
		args = append(args,
			"-o", "PubkeyAuthentication=no",
			"-o", "KbdInteractiveAuthentication=no",
			"-o", "PreferredAuthentications=password",
			"-o", "NumberOfPasswordPrompts=1",
		)
	}
	if socket, err := s.controlPath(); err == nil {
		// One authenticated connection reused across tool calls. Without it
		// every command pays a full handshake, and on a host wanting a
		// password or a hardware token it pays one per command.
		args = append(args,
			"-o", "ControlMaster=auto",
			"-o", "ControlPath="+socket,
			"-o", "ControlPersist=600",
		)
	}
	for _, option := range s.target.SSHOptions {
		if strings.TrimSpace(option) != "" {
			args = append(args, "-o", option)
		}
	}
	if username := strings.TrimSpace(s.target.Username); username != "" {
		args = append(args, "-l", username)
	}
	return args
}

// controlPath names the multiplexing socket for this destination.
//
// The name is a hash because a unix socket path is capped at 104 bytes on
// macOS and 108 on Linux, and ssh fails opaquely when it overruns.
func (s *sshExecutor) controlPath() (string, error) {
	dir := filepath.Join(os.TempDir(), "onecatch-seam")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(strings.TrimSpace(s.target.Username) + "\x00" + s.target.Host))
	return filepath.Join(dir, "c-"+hex.EncodeToString(sum[:6])+".sock"), nil
}

// CloseSSHControlMaster tears down the multiplexed connection for a target, so
// no connection outlives the run that opened it. Leaving a live master to
// someone else's server is exactly the residue this design exists to avoid.
func CloseSSHControlMaster(ctx context.Context, t Target) error {
	s := &sshExecutor{target: t}
	socket, err := s.controlPath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(socket); err != nil {
		return nil // never opened, or already gone
	}
	binary := t.SSHBinary
	if binary == "" {
		binary = "ssh"
	}
	endpoint, err := sshendpoint.Parse(t.Host)
	if err != nil {
		return err
	}
	args := []string{"-o", "ControlPath=" + socket, "-O", "exit"}
	if endpoint.Port != 0 {
		args = append(args, "-p", fmt.Sprintf("%d", endpoint.Port))
	}
	if username := strings.TrimSpace(t.Username); username != "" {
		args = append(args, "-l", username)
	}
	return exec.CommandContext(ctx, binary, append(args, endpoint.Host)...).Run()
}

// --- shared ---------------------------------------------------------------

// ErrTransport means the command's fate is unknown: it may have run, it may
// never have started. It is never a statement about the command itself.
var ErrTransport = errors.New("seam: could not run the command on the target")

const transportDiagnosticLimit = 16 * 1024

// diagnosticTailWriter retains enough stderr to explain an SSH or login-shell
// failure without allowing a broken command to buffer unbounded output.
type diagnosticTailWriter struct {
	data      []byte
	truncated bool
}

func (w *diagnosticTailWriter) Write(p []byte) (int, error) {
	if len(p) >= transportDiagnosticLimit {
		w.data = append(w.data[:0], p[len(p)-transportDiagnosticLimit:]...)
		w.truncated = true
		return len(p), nil
	}
	if overflow := len(w.data) + len(p) - transportDiagnosticLimit; overflow > 0 {
		copy(w.data, w.data[overflow:])
		w.data = w.data[:len(w.data)-overflow]
		w.truncated = true
	}
	w.data = append(w.data, p...)
	return len(p), nil
}

func (w *diagnosticTailWriter) String() string {
	result := string(w.data)
	if w.truncated {
		return "...\n" + result
	}
	return result
}

// runShell runs the wrapped program and recovers the two markers from it.
//
// The process is built from a callback rather than passed in so that the
// timeout context is the one the process actually gets. Deriving it after
// exec.CommandContext had already captured the parent left the timeout with
// nothing to cancel — the bound existed in the code and not in the behaviour.
func runShell(ctx context.Context, cmd Command, build func(context.Context) *exec.Cmd, statusMarker, cwdMarker, describe string) (Outcome, error) {
	if cmd.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cmd.Timeout)
		defer cancel()
	}
	proc := build(ctx)
	stdout := newMarkerWriter(cmd.Stdout, "\n"+statusMarker)
	var diagnostic diagnosticTailWriter
	stderrDestination := io.Writer(&diagnostic)
	if cmd.Stderr != nil {
		stderrDestination = io.MultiWriter(cmd.Stderr, &diagnostic)
	}
	stderr := newMarkerWriter(stderrDestination, "\n"+cwdMarker)
	proc.Stdout = stdout
	proc.Stderr = stderr
	// A command must never be left waiting on input that will not arrive.
	var runErr error
	if cmd.Stdin == nil {
		proc.Stdin = bytes.NewReader(nil)
		runErr = proc.Run()
	} else {
		// Do not assign an io.PipeReader directly to Cmd.Stdin. os/exec then
		// waits for its internal copy goroutine after the child exits, while
		// that goroutine is still waiting for the next process/write byte: a
		// fast command is reported as "still running" forever. Owning the
		// StdinPipe lets child exit close the write side and lets us close the
		// source explicitly to unblock the copier.
		stdin, err := proc.StdinPipe()
		if err != nil {
			return Outcome{}, fmt.Errorf("%w (%s): open stdin: %v", ErrTransport, describe, err)
		}
		if err := proc.Start(); err != nil {
			return Outcome{}, fmt.Errorf("%w (%s): start: %v", ErrTransport, describe, err)
		}
		copyDone := make(chan struct{})
		go func() {
			_, _ = io.Copy(stdin, cmd.Stdin)
			_ = stdin.Close()
			close(copyDone)
		}()
		runErr = proc.Wait()
		_ = stdin.Close()
		if closer, ok := cmd.Stdin.(io.Closer); ok {
			_ = closer.Close()
		}
		<-copyDone
	}
	statusText, sawStatus := stdout.Finish()
	cwd, _ := stderr.Finish()

	if !sawStatus {
		// The wrapper never completed. Whatever went wrong, it was not the
		// command reporting a status, so this must not reach the agent as one.
		detail := strings.TrimSpace(diagnostic.String())
		if runErr != nil {
			if detail == "" {
				detail = runErr.Error()
			} else {
				detail = runErr.Error() + ": " + detail
			}
		}
		if detail == "" {
			detail = "remote shell exited without a status marker"
		}
		return Outcome{}, fmt.Errorf("%w (%s): %s", ErrTransport, describe, detail)
	}
	code, err := strconv.Atoi(statusText)
	if err != nil {
		return Outcome{}, fmt.Errorf("%w (%s): unreadable exit status %q", ErrTransport, describe, statusText)
	}
	return Outcome{ExitCode: code, Cwd: cwd}, nil
}
