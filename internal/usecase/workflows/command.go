package workflows

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"
	domainworkspaces "github.com/openmodu/onecatch/internal/domain/workspaces"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

const commandOutputTail = 12 * 1024

// runCommand executes a command step in the workspace and streams its output
// as one tool call. FinalMessage carries the tail of the combined output and
// Succeeded reports exit status zero.
func (s *Usecase) runCommand(ctx context.Context, definition domainworkflows.Definition, step domainworkflows.Step, workspace domainworkspaces.Workspace, run domainworkflows.Run, sink agentrun.Sink) (agentrun.Result, error) {
	if workspace.RemoteFS != nil {
		return agentrun.Result{}, fmt.Errorf("command_local_only: command steps cannot run in a remote FS workspace")
	}
	stepCtx, cancel := context.WithTimeout(ctx, time.Duration(definition.Policy.StepTimeoutSeconds)*time.Second)
	defer cancel()
	shell, flag := "/bin/sh", "-c"
	if runtime.GOOS == "windows" {
		shell, flag = "cmd", "/C"
	}
	cmd := exec.CommandContext(stepCtx, shell, flag, step.Command)
	cmd.Dir = workspace.Path
	cmd.Env = append(os.Environ(), "ONECATCH_RUN_ID="+run.ID, "ONECATCH_STEP_ID="+step.ID, "ONECATCH_FEEDBACK="+messageTexts(run.Nodes[step.ID].Inbox))
	cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
	cmd.WaitDelay = 10 * time.Second

	streamID := "command_" + step.ID
	output := &tailWriter{limit: commandOutputTail, emit: func(chunk string) {
		sink(agentrun.Event{Kind: agentrun.KindToolResult, StreamID: streamID, Phase: agentrun.StreamDelta, Text: chunk, At: s.now()})
	}}
	cmd.Stdout, cmd.Stderr = output, output
	sink(agentrun.Event{Kind: agentrun.KindStarted, At: s.now()})
	sink(agentrun.Event{Kind: agentrun.KindToolUse, Text: step.Command, At: s.now()})
	sink(agentrun.Event{Kind: agentrun.KindToolResult, StreamID: streamID, Phase: agentrun.StreamStart, At: s.now()})
	err := cmd.Run()
	sink(agentrun.Event{Kind: agentrun.KindToolResult, StreamID: streamID, Phase: agentrun.StreamEnd, Failed: err != nil, At: s.now()})

	result := agentrun.Result{FinalMessage: output.String(), Succeeded: err == nil}
	var exitErr *exec.ExitError
	switch {
	case ctx.Err() != nil:
		return result, ctx.Err()
	case errors.Is(stepCtx.Err(), context.DeadlineExceeded):
		result.FinalMessage = fmt.Sprintf("timed out after %ds\n%s", definition.Policy.StepTimeoutSeconds, result.FinalMessage)
		err = nil
	case errors.As(err, &exitErr):
		result.FinalMessage = fmt.Sprintf("exit code %d\n%s", exitErr.ExitCode(), result.FinalMessage)
		err = nil
	}
	status := "passed"
	if !result.Succeeded {
		status = "failed"
	}
	sink(agentrun.Event{Kind: agentrun.KindResult, Text: "Command " + status, Failed: !result.Succeeded, At: s.now()})
	return result, err
}

// commandOutcome maps an exit status to a verdict. A failing command with a
// rework edge sends its output back as feedback; without one it is a step
// failure that retries and then pauses for a person.
func commandOutcome(definition domainworkflows.Definition, step domainworkflows.Step, result agentrun.Result, err error) (domainworkflows.Outcome, string) {
	if err != nil {
		return domainworkflows.Outcome{}, err.Error()
	}
	output := strings.TrimSpace(result.FinalMessage)
	if result.Succeeded {
		summary := fmt.Sprintf("`%s` passed.", step.Command)
		if output != "" {
			summary += "\n\n```\n" + tailRunes(output, 2000) + "\n```"
		}
		return domainworkflows.Outcome{Verdict: domainworkflows.VerdictPass, Summary: summary}, ""
	}
	feedback := fmt.Sprintf("`%s` failed:\n\n```\n%s\n```", step.Command, output)
	edges := domainworkflows.ReworkEdges(definition, step.ID)
	if len(edges) == 0 {
		return domainworkflows.Outcome{}, feedback
	}
	return domainworkflows.Outcome{Verdict: domainworkflows.VerdictRework, Target: edges[0].To, Summary: fmt.Sprintf("`%s` failed", step.Command), Feedback: feedback}, ""
}

func tailRunes(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return "…" + string(runes[len(runes)-limit:])
}

// tailWriter forwards output as it arrives and keeps only the last limit
// bytes for the verdict, so a chatty test suite cannot exhaust memory.
type tailWriter struct {
	mu    sync.Mutex
	limit int
	buf   []byte
	emit  func(string)
}

func (w *tailWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.emit(string(p))
	w.buf = append(w.buf, p...)
	if extra := len(w.buf) - w.limit; extra > 0 {
		w.buf = w.buf[extra:]
	}
	return len(p), nil
}

func (w *tailWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return strings.ToValidUTF8(string(w.buf), "")
}
