// Command onecatchsh is the shell OneCatch hands a coding agent when its run
// targets a remote machine.
//
// Claude Code is told about it through CLAUDE_CODE_SHELL_PREFIX and invokes it
// once per Bash tool call, passing the whole command envelope as a single
// argument. Claude's file hooks also invoke its claude-hook mode. This program
// forwards shell commands, mirrors native file operations, and reproduces
// locally the bookkeeping the harness expects to find on this machine.
//
// It is not a program anyone runs by hand. It exists as a separate binary
// because harnesses want a *program path* for their shell hook, not a command
// line: Claude Code stats the value of CLAUDE_CODE_SHELL_PREFIX directly, so
// anything with arguments in it is looked up as one filename and fails.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/openmodu/onecatch/internal/usecase/agentrun/seam"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx))
}

func run(ctx context.Context) int {
	name := os.Getenv(seam.SessionEnv)
	if name == "" {
		// Nothing bound this process to a run. Falling back to a local shell
		// here would be the worst available outcome: the agent believes it is
		// operating on the target, and would silently act on this machine
		// instead. Refuse instead, visibly.
		fmt.Fprintf(os.Stderr,
			"onecatchsh: %s is not set, so there is no target to run this on.\n"+
				"            This program is the agent's shell; it is launched by OneCatch,\n"+
				"            not run by hand.\n", seam.SessionEnv)
		return seam.ExitTransportFailure
	}
	session, err := seam.LoadSession(name)
	if err != nil {
		fmt.Fprintf(os.Stderr,
			"onecatchsh: run %q has no session, refusing to run this command locally.\n"+
				"            The agent expects it to run on the target.\n"+
				"            Reason: %v\n", name, err)
		return seam.ExitTransportFailure
	}
	if len(os.Args) == 2 && os.Args[1] == "exec-server" {
		server, err := seam.NewExecServer(ctx, session, os.Getenv("ONECATCH_EXEC_WORKSPACE"))
		if err != nil {
			fmt.Fprintln(os.Stderr, "onecatchsh exec-server:", err)
			return seam.ExitTransportFailure
		}
		defer func() { _ = server.Close() }()
		if err := server.Serve(ctx, os.Stdin, os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, "onecatchsh exec-server:", err)
			return seam.ExitTransportFailure
		}
		return 0
	}
	if len(os.Args) == 2 && os.Args[1] == "claude-hook" {
		if err := seam.RunClaudeHook(ctx, session, os.Stdin, os.Stdout); err != nil {
			// Hook stdout is a JSON protocol channel. Diagnostics must stay on
			// stderr so Claude can always parse the response.
			fmt.Fprintln(os.Stderr, "onecatchsh claude-hook:", err)
			_, _ = fmt.Fprintln(os.Stdout, "{}")
		}
		return 0
	}

	d := seam.NewDispatcher(session)
	// Warnings go to stderr, which is where the agent reads them. An envelope
	// shape we no longer recognise is something the model should see rather
	// than something only a log file knows about.
	d.Warn = func(msg string) { fmt.Fprintln(os.Stderr, msg) }
	return d.Dispatch(ctx, os.Args[1:], os.Stdout, os.Stderr)
}
