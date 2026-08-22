package seam

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// ExitTransportFailure is returned when the command could not be run at all.
//
// It is deliberately distinct from any status a command itself might produce,
// so the agent can tell "your command failed" from "we could not reach the
// target" — two situations that call for completely different next moves.
const ExitTransportFailure = 125

// Dispatcher routes one shell invocation to the machine it belongs on.
type Dispatcher struct {
	Session *Session
	// Remote runs the model's commands.
	Remote Executor
	// Local runs the harness's own shell use. It is a separate executor rather
	// than a flag because the two destinations are genuinely different
	// machines, and the whole point of this package is never to confuse them.
	Local Executor
	// Timeout bounds one command. Zero means unbounded.
	Timeout time.Duration
	// Warn receives anomalies worth an operator's attention. Nil discards
	// them.
	Warn func(string)
}

// NewDispatcher builds the dispatcher for a session.
func NewDispatcher(s *Session) *Dispatcher {
	return &Dispatcher{
		Session: s,
		Remote:  NewExecutor(s.Target),
		Local:   &localExecutor{},
	}
}

// Dispatch runs one shell-prefix invocation and returns the status to exit
// with.
//
// argv is what the harness passed. Claude Code 2.1.239 passes the whole
// envelope as a single element with no -c in front of it; the elements are
// joined defensively so a harness that splits it still works.
func (d *Dispatcher) Dispatch(ctx context.Context, argv []string, stdout, stderr io.Writer) int {
	if len(argv) == 0 {
		fmt.Fprintln(stderr, "seam: expected a command argument")
		return 2
	}
	env := ParseClaudeCode(strings.Join(argv, " "))

	switch env.Kind {
	case KindInternal:
		// The harness's own hooks and plugin launches. They name local
		// interpreters and local paths; sending them to the target would break
		// every one of them for no benefit.
		return d.run(ctx, d.Local, Command{
			Command: env.Command,
			Stdout:  stdout,
			Stderr:  stderr,
			Timeout: d.Timeout,
		}, stderr, "")

	case KindUnknown:
		// Routed to the target like a model call, because the two ways of
		// being wrong are not symmetric: an internal command sent to the
		// target fails loudly, while a model command run locally executes on
		// the operator's own disk while the agent reports success against the
		// target. But it is worth saying out loud — a steady stream of these
		// means the harness changed shape.
		d.warn("seam: unrecognised command envelope; forwarding it to " +
			d.Remote.Describe() + " unparsed. The harness may have changed shape; " +
			"run `task test:conformance`.")
		fallthrough

	default: // KindModel
		return d.run(ctx, d.Remote, Command{
			Command: env.Command,
			Dir:     d.Session.Cwd,
			Stdout:  stdout,
			Stderr:  stderr,
			Timeout: d.Timeout,
		}, stderr, env.CwdFile)
	}
}

func (d *Dispatcher) run(ctx context.Context, ex Executor, cmd Command, stderr io.Writer, cwdFile string) int {
	out, err := ex.Run(ctx, cmd)
	if err != nil {
		fmt.Fprintln(stderr, "seam:", err)
		return ExitTransportFailure
	}
	if out.Cwd != "" && ex == d.Remote {
		d.persistCwd(out.Cwd, cwdFile, stderr)
	}
	return out.ExitCode
}

// persistCwd records where the command ended up, in both places that need it.
//
// The session file is how the *next* invocation knows where to start; the
// harness's own file is how the harness believes `cd` persisted. Neither is
// optional, and a failure to write either is reported rather than swallowed:
// silently losing the working directory means the agent's next command runs
// somewhere it did not choose, which it has no way to notice.
func (d *Dispatcher) persistCwd(cwd, cwdFile string, stderr io.Writer) {
	if err := d.Session.SetCwd(cwd); err != nil {
		fmt.Fprintf(stderr,
			"seam: could not record the working directory %s: %v\n"+
				"seam: later commands will keep running in %s until this is fixed\n",
			cwd, err, d.Session.Cwd)
	}
	if cwdFile == "" {
		return
	}
	// The harness reads this local file to learn where its shell ended up.
	// Forwarded verbatim the redirect would have written it on the *target*,
	// and `cd` would stop persisting with no error anywhere.
	if err := os.WriteFile(cwdFile, []byte(cwd+"\n"), 0o600); err != nil {
		fmt.Fprintf(stderr, "seam: could not update the harness's record of the working directory: %v\n", err)
	}
}

func (d *Dispatcher) warn(msg string) {
	if d.Warn != nil {
		d.Warn(msg)
	}
}
