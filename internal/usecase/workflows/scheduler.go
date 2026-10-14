package workflows

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	domaintasks "github.com/openmodu/onecatch/internal/domain/tasks"
	domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"
	domainworkspaces "github.com/openmodu/onecatch/internal/domain/workspaces"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

const (
	PauseReasonBlocked      = "blocked"
	maxStoredSummaryRunes   = 8000
	protocolReminderMessage = "Your previous reply did not end with the required verdict JSON object. Finish the step and end with exactly one JSON object as described in the response contract."
)

// stepResult is what a step goroutine reports back to the scheduler. The
// scheduler is the only writer of the Run, so goroutines never touch it.
type stepResult struct {
	step      domainworkflows.Step
	sessionID string
	outcome   domainworkflows.Outcome
	// failure is a retryable problem with the step itself; pause is an
	// infrastructure condition that retrying cannot fix.
	failure string
	pause   string
	err     error
}

// drive schedules a running Run until it completes, pauses or ctx ends. Steps
// start as soon as their dependencies pass, up to the concurrency limit, and
// every verdict is persisted before the next scheduling decision.
func (s *Usecase) drive(ctx context.Context, task domaintasks.Task, workspace domainworkspaces.Workspace, definition domainworkflows.Definition, run domainworkflows.Run, preserveSteps ...string) (domainworkflows.Run, error) {
	run = domainworkflows.CloneRun(run)
	if run.Sessions == nil {
		run.Sessions = make(map[string]string)
	}
	if run.Nodes == nil {
		run.Nodes = make(map[string]domainworkflows.NodeState, len(definition.Steps))
	}
	for _, step := range definition.Steps {
		node, ok := run.Nodes[step.ID]
		if !ok || node.Status == domainworkflows.NodeRunning {
			node.StepID, node.Status = step.ID, domainworkflows.NodePending
			run.Nodes[step.ID] = node
		}
	}

	stepCtx, cancelSteps := context.WithCancel(ctx)
	defer func() { cancelSteps() }()
	results := make(chan stepResult)
	inflight := map[string]bool{}
	var pauseReason, pauseMessage string

	for {
		if run.Status == domainworkflows.RunRunning && pauseReason == "" && ctx.Err() == nil {
			// Steps must never start under a context cancelled for an
			// earlier batch; they would fail at once and be retried forever.
			if stepCtx.Err() != nil && len(inflight) == 0 {
				var cancel context.CancelFunc
				stepCtx, cancel = context.WithCancel(ctx)
				cancelSteps = cancel
			}
			if err := s.startReadySteps(ctx, stepCtx, task, workspace, &definition, &run, inflight, results, preserveSteps); err != nil {
				cancelSteps()
				s.drain(results, inflight)
				if errors.Is(err, errExecutionSettings) {
					return s.pause(context.Background(), &task, definition, run, "execution_settings_invalid", err.Error())
				}
				return run, err
			}
		}
		if len(inflight) == 0 {
			// The outer loop advances only after a successful complete round.
			if run.Status == domainworkflows.RunCompleted && run.LoopEnabled && pauseReason == "" && ctx.Err() == nil {
				refreshed, err := s.refreshLoopDefinition(ctx, task, definition, &run)
				if err != nil {
					run.Status = domainworkflows.RunRunning
					return s.pause(context.Background(), &task, definition, run, "execution_settings_invalid", err.Error())
				}
				definition = refreshed
				next, err := domainworkflows.NextLoopRound(definition, run, s.now())
				if err != nil {
					return run, err
				}
				updated, err := s.workflows.UpdateRun(context.Background(), next, run.Revision)
				if err != nil {
					return run, err
				}
				run = updated
				if err := s.appendEvent(context.Background(), run.ID, "loop.round.started", "", map[string]any{"round": run.LoopRound}); err != nil {
					return run, err
				}
				continue
			}
			// A run the last verdict just completed may still hold messages.
			if (run.Status == domainworkflows.RunRunning || run.Status == domainworkflows.RunCompleted) && pauseReason == "" && ctx.Err() == nil {
				reopened, err := s.reopenForMessages(ctx, definition, &run)
				if err != nil {
					return run, err
				}
				if reopened {
					continue
				}
			}
			break
		}
		item := <-results
		delete(inflight, item.step.ID)
		var err error
		run, err = s.applyStepResult(ctx, task, definition, run, item, &pauseReason, &pauseMessage)
		if err != nil {
			cancelSteps()
			s.drain(results, inflight)
			return run, err
		}
		// Stop siblings only when the run stops. Completion leaves nothing
		// running, and may still reopen steps for late messages.
		if pauseReason != "" || run.Status != domainworkflows.RunRunning && run.Status != domainworkflows.RunCompleted {
			cancelSteps()
		}
	}

	switch {
	case ctx.Err() != nil:
		if run.LoopEnabled && run.Status == domainworkflows.RunCompleted {
			next, err := domainworkflows.NextLoopRound(definition, run, s.now())
			if err != nil {
				return run, err
			}
			run = next
		}
		return s.pause(context.Background(), &task, definition, run, PauseReasonInterrupted, ctx.Err().Error())
	case pauseReason != "":
		return s.pause(ctx, &task, definition, run, pauseReason, pauseMessage)
	case run.Status == domainworkflows.RunPaused:
		if err := s.setTaskStatus(ctx, &task, domaintasks.StatusPaused); err != nil {
			return run, err
		}
		_ = s.appendEvent(ctx, run.ID, "run.paused", run.CurrentStepID, map[string]any{"reason": run.PauseReason, "error": run.LastError})
		return run, nil
	case run.Status == domainworkflows.RunCompleted:
		if err := s.setTaskStatus(ctx, &task, domaintasks.StatusCompleted); err != nil {
			return run, err
		}
		if err := s.workflows.WriteRunSummary(ctx, run.ID, buildSummary(task, definition, run)); err != nil {
			return run, err
		}
		_ = s.appendEvent(ctx, run.ID, "run.completed", run.CurrentStepID, map[string]any{"steps": len(definition.Steps)})
		return run, nil
	case run.Status != domainworkflows.RunRunning:
		return run, nil
	default:
		return s.pause(ctx, &task, definition, run, PauseReasonBlocked, "no step is ready to run")
	}
}

func (s *Usecase) drain(results chan stepResult, inflight map[string]bool) {
	for len(inflight) > 0 {
		item := <-results
		delete(inflight, item.step.ID)
	}
}

// startReadySteps claims queued human instructions, delivers them, and starts
// every ready step that fits under the concurrency limit.
func (s *Usecase) startReadySteps(ctx, stepCtx context.Context, task domaintasks.Task, workspace domainworkspaces.Workspace, currentDefinition *domainworkflows.Definition, run *domainworkflows.Run, inflight map[string]bool, results chan<- stepResult, preserveSteps []string) error {
	definition := *currentDefinition
	var ready []domainworkflows.Step
	for _, step := range domainworkflows.ReadySteps(definition, *run) {
		if !inflight[step.ID] {
			ready = append(ready, step)
		}
	}
	if len(ready) == 0 {
		return nil
	}
	refreshed, err := s.refreshStartingSettings(ctx, task, definition, run, ready, preserveSteps)
	if err != nil {
		return err
	}
	definition = refreshed
	*currentDefinition = refreshed
	limit := run.MaxConcurrency
	if limit < 1 {
		limit = int(s.maxConcurrency.Load())
	}
	if room := limit - len(inflight); room < len(ready) {
		if room <= 0 {
			return nil
		}
		ready = ready[:room]
	}
	for i, step := range ready {
		ready[i], _ = domainworkflows.FindStep(refreshed, step.ID)
	}
	if err := s.deliverInstructions(ctx, definition, run, ready); err != nil {
		return err
	}
	for _, step := range ready {
		node := run.Nodes[step.ID]
		node.Status = domainworkflows.NodeRunning
		node.Attempt++
		node.Error = ""
		node.StartedAt = s.now()
		node.FinishedAt = time.Time{}
		run.Nodes[step.ID] = node
		run.CurrentStepID = step.ID
	}
	run.UpdatedAt = s.now()
	updated, err := s.workflows.UpdateRun(ctx, *run, run.Revision)
	if err != nil {
		return err
	}
	*run = updated
	snapshot := domainworkflows.CloneRun(updated)
	for _, step := range ready {
		inflight[step.ID] = true
		go func(step domainworkflows.Step) {
			results <- s.executeStep(stepCtx, task, workspace, definition, snapshot, step)
		}(step)
	}
	return nil
}

// deliverInstructions moves queued human guidance into step inboxes. Guidance
// addressed to a step waits in that step's inbox; the rest goes to the steps
// starting now.
func (s *Usecase) deliverInstructions(ctx context.Context, definition domainworkflows.Definition, run *domainworkflows.Run, starting []domainworkflows.Step) error {
	items, err := s.workflows.ClaimInstructions(ctx, run.ID)
	if err != nil || len(items) == 0 {
		return err
	}
	ids := make([]string, 0, len(items))
	for _, item := range items {
		text := item.Content
		if len(item.Attachments) > 0 {
			text += "\nAttachments:\n- " + strings.Join(item.Attachments, "\n- ")
		}
		message := domainworkflows.Message{From: domainworkflows.MessageFromHuman, Text: text, At: item.CreatedAt}
		if _, ok := domainworkflows.FindStep(definition, item.StepID); ok {
			*run = domainworkflows.Deliver(*run, item.StepID, message)
		} else {
			for _, step := range starting {
				*run = domainworkflows.Deliver(*run, step.ID, message)
			}
		}
		ids = append(ids, item.ID)
	}
	return s.appendEvent(ctx, run.ID, "instruction.applied", "", map[string]any{"instructionIds": ids, "count": len(ids)})
}

// reopenForMessages runs when nothing is left to schedule. Guidance queued
// while the last steps were running is delivered now: to the step it names, or
// to the final steps. Any passed step holding a message is reopened with the
// steps after it, so the run continues instead of finishing with unread input.
func (s *Usecase) reopenForMessages(ctx context.Context, definition domainworkflows.Definition, run *domainworkflows.Run) (bool, error) {
	var sinks []domainworkflows.Step
	for _, id := range domainworkflows.SinkSteps(definition) {
		step, _ := domainworkflows.FindStep(definition, id)
		sinks = append(sinks, step)
	}
	if err := s.deliverInstructions(ctx, definition, run, sinks); err != nil {
		return false, err
	}
	next, reopened := domainworkflows.ReopenForInbox(definition, *run)
	if len(reopened) == 0 {
		return false, nil
	}
	next.Status = domainworkflows.RunRunning
	next.CompletedAt = time.Time{}
	next.UpdatedAt = s.now()
	updated, err := s.workflows.UpdateRun(ctx, next, run.Revision)
	if err != nil {
		return false, err
	}
	*run = updated
	_ = s.appendEvent(ctx, run.ID, "steps.reopened", "", map[string]any{"steps": reopened, "reason": "message"})
	return true, nil
}

// applyStepResult folds one finished step into the Run and persists it.
func (s *Usecase) applyStepResult(ctx context.Context, task domaintasks.Task, definition domainworkflows.Definition, run domainworkflows.Run, item stepResult, pauseReason, pauseMessage *string) (domainworkflows.Run, error) {
	stepID := item.step.ID
	if item.sessionID != "" {
		run.Sessions[stepID] = item.sessionID
	}
	node := run.Nodes[stepID]
	switch {
	case item.err != nil && (ctx.Err() != nil || errors.Is(item.err, context.Canceled)):
		// Interrupted, or cancelled because a sibling paused the run. The step
		// keeps its session and inbox and runs again on resume.
		node.Status = domainworkflows.NodePending
		node.FinishedAt = s.now()
		run.Nodes[stepID] = node
	case item.pause != "":
		node.Status = domainworkflows.NodePaused
		node.Error = item.err.Error()
		node.FinishedAt = s.now()
		run.Nodes[stepID] = node
		if *pauseReason == "" {
			*pauseReason, *pauseMessage = item.pause, item.err.Error()
		}
		_ = s.appendEvent(ctx, run.ID, "step.failed", stepID, map[string]any{"error": item.err.Error(), "reason": item.pause})
	case item.failure != "":
		run = domainworkflows.RecordStepFailure(definition, run, stepID, item.failure, s.now())
		if strings.HasPrefix(item.failure, protocolFailure) {
			run = domainworkflows.Deliver(run, stepID, domainworkflows.Message{From: "onecatch", Text: protocolReminderMessage, At: s.now()})
		}
		_ = s.appendEvent(ctx, run.ID, "step.failed", stepID, map[string]any{"error": run.Nodes[stepID].Error, "failures": run.Nodes[stepID].Failures})
	default:
		outcome := item.outcome
		outcome.Summary = truncateRunes(outcome.Summary, maxStoredSummaryRunes)
		outcome.Feedback = truncateRunes(outcome.Feedback, maxStoredSummaryRunes)
		next, err := domainworkflows.ApplyOutcome(definition, run, stepID, outcome, s.now())
		if err != nil {
			run = domainworkflows.RecordStepFailure(definition, run, stepID, err.Error(), s.now())
			break
		}
		run = next
		payload := map[string]any{"verdict": outcome.Verdict, "summary": outcome.Summary}
		if outcome.Verdict == domainworkflows.VerdictRework {
			payload["target"] = reworkTarget(definition, stepID, outcome.Target)
			payload["feedback"] = outcome.Feedback
		}
		_ = s.appendEvent(ctx, run.ID, "step.completed", stepID, payload)
	}
	run.UpdatedAt = s.now()
	// The last verdict and completion land in one write, so a reader never
	// sees every step passed while the run still claims to be running.
	if run.Status == domainworkflows.RunRunning && *pauseReason == "" && domainworkflows.AllCompleted(definition, run) {
		run.Status = domainworkflows.RunCompleted
		run.CompletedAt = run.UpdatedAt
	}
	return s.workflows.UpdateRun(context.Background(), run, run.Revision)
}

func reworkTarget(definition domainworkflows.Definition, stepID, target string) string {
	if target != "" {
		return target
	}
	if edges := domainworkflows.ReworkEdges(definition, stepID); len(edges) > 0 {
		return edges[0].To
	}
	return ""
}

func (s *Usecase) pause(ctx context.Context, task *domaintasks.Task, definition domainworkflows.Definition, run domainworkflows.Run, reason, message string) (domainworkflows.Run, error) {
	if run.Status != domainworkflows.RunRunning {
		return run, nil
	}
	paused, err := domainworkflows.Pause(definition, run, reason, message, s.now())
	if err != nil {
		return run, err
	}
	paused, err = s.workflows.UpdateRun(ctx, paused, run.Revision)
	if err != nil {
		return run, err
	}
	_ = s.setTaskStatus(ctx, task, domaintasks.StatusPaused)
	_ = s.appendEvent(ctx, run.ID, "run.paused", run.CurrentStepID, map[string]any{"reason": reason, "error": message})
	if reason == PauseReasonInterrupted {
		return paused, ctx.Err()
	}
	return paused, nil
}

const protocolFailure = "the reply did not end with a valid verdict JSON object"

// executeStep runs one attempt of step and turns its result into a verdict.
func (s *Usecase) executeStep(ctx context.Context, task domaintasks.Task, workspace domainworkspaces.Workspace, definition domainworkflows.Definition, run domainworkflows.Run, step domainworkflows.Step) stepResult {
	attempt := run.Nodes[step.ID].Attempt
	resumeSessionID := run.Sessions[step.ID]
	previousContextTokens := 0
	if resumeSessionID != "" {
		var err error
		if _, previousContextTokens, err = s.nextAttempt(ctx, run.ID, step.ID); err != nil {
			return stepResult{step: step, err: err, pause: "storage_error"}
		}
	}
	stepRun := domainworkflows.StepRun{ID: s.newID("step"), RunID: run.ID, StepID: step.ID, Attempt: attempt, LoopRound: run.LoopRound, Runtime: step.Runtime, Model: step.Model, Sandbox: step.Sandbox, Status: domainworkflows.StepRunRunning, SessionIDBefore: resumeSessionID, StartedAt: s.now()}
	if err := s.workflows.SaveStepRun(ctx, stepRun); err != nil {
		return stepResult{step: step, err: err, pause: "storage_error"}
	}
	_ = s.appendEvent(ctx, run.ID, "step.started", step.ID, map[string]any{"stepRunId": stepRun.ID, "attempt": attempt, "kind": step.Kind, "runtime": step.Runtime, "workerId": step.WorkerID})
	_ = s.recordGit(ctx, run.ID, step.ID, "before", workspace)
	defer func() { _ = s.recordGit(context.Background(), run.ID, step.ID, "after", workspace) }()

	if stepWrites(step) {
		release, err := s.locker.Acquire(ctx, workspaceLockID(workspace), workspace.Path, run.ID)
		if err != nil {
			s.finishStepRun(&stepRun, agentrun.Result{}, domainworkflows.StepRunFailed, err.Error())
			return stepResult{step: step, err: err, pause: PauseReasonWorkspaceLocked}
		}
		defer release()
	}

	collector := s.newRuntimeCollector(run.ID, stepRun.ID, previousContextTokens)
	var result agentrun.Result
	var err error
	if step.Kind == domainworkflows.StepCommand {
		result, err = s.runCommand(ctx, definition, step, workspace, run, collector.Sink())
	} else {
		request := agentrun.Request{RunID: run.ID, StepRunID: stepRun.ID, Runtime: agentrun.Runtime(step.Runtime), Workspace: workspace.Path, Prompt: composeStepPrompt(task, definition, step, run), Model: step.Model, ReasoningEffort: resolvedReasoningEffort(run, step.Runtime), ServiceTier: resolvedServiceTier(run, step.Runtime), MaxContextWindow: resolvedMaxContextWindow(run, step.Runtime), Provider: resolvedRuntimeProvider(run, step.Runtime), Sandbox: allowedSandbox(step.Sandbox, workspace.DefaultSandbox), ResumeSessionID: resumeSessionID, EnvironmentAllowlist: resolvedEnvironmentAllowlist(run, step.Runtime), InterruptGrace: time.Duration(run.InterruptGraceSeconds) * time.Second, RuntimeDefaultsResolved: true, Remote: workspaceRemoteTarget(workspace)}
		result, err = s.dispatchStep(ctx, definition, step, workspace.ID, request, collector.Sink())
	}
	if streamErr := collector.Close(); streamErr != nil && err == nil {
		err = collectorError(streamErr)
	}
	stepRun.SessionIDAfter = result.SessionID
	out := stepResult{step: step, sessionID: result.SessionID}

	switch {
	case ctx.Err() != nil:
		s.finishStepRun(&stepRun, result, domainworkflows.StepRunInterrupted, ctx.Err().Error())
		out.err = ctx.Err()
		return out
	case err != nil && infrastructureFailure(err):
		s.finishStepRun(&stepRun, result, domainworkflows.StepRunFailed, err.Error())
		out.err, out.pause = err, infrastructureReason(err)
		return out
	case step.Kind == domainworkflows.StepCommand:
		out.outcome, out.failure = commandOutcome(definition, step, result, err)
	case err != nil || !result.Succeeded:
		out.failure = failureMessage(err, result.FinalMessage)
	default:
		out.outcome, out.failure = agentOutcome(definition, step, result.FinalMessage)
	}
	if out.failure != "" {
		s.finishStepRun(&stepRun, result, domainworkflows.StepRunFailed, out.failure)
		return out
	}
	stepRun.Verdict, stepRun.Summary = out.outcome.Verdict, truncateRunes(out.outcome.Summary, maxStoredSummaryRunes)
	s.finishStepRun(&stepRun, result, domainworkflows.StepRunSucceeded, "")
	return out
}

func (s *Usecase) finishStepRun(stepRun *domainworkflows.StepRun, result agentrun.Result, status domainworkflows.StepRunStatus, message string) {
	finishStepRun(stepRun, result, s.now())
	stepRun.Status, stepRun.Error = status, message
	_ = s.workflows.SaveStepRun(context.Background(), *stepRun)
}

// agentOutcome reads the verdict from an agent's final reply. A step that has
// nowhere to send work back may simply answer in prose: that is a pass. A step
// with rework edges must say which way it decided. A failed attempt resumes the
// same session with a reminder, so the agent corrects only its reply.
func agentOutcome(definition domainworkflows.Definition, step domainworkflows.Step, final string) (domainworkflows.Outcome, string) {
	if directAgentDefinition(definition) {
		return passWith(final, "Agent completed successfully."), ""
	}
	outcome, err := domainworkflows.ParseOutcome(final)
	if err != nil {
		// Prose with no verdict object is a pass where nothing could be sent
		// back; a malformed object is a real mistake the step must correct.
		if len(domainworkflows.ReworkEdges(definition, step.ID)) == 0 && !strings.HasSuffix(strings.TrimSpace(final), "}") {
			return passWith(final, "Step completed."), ""
		}
		return domainworkflows.Outcome{}, protocolFailure + ": " + err.Error()
	}
	if err := domainworkflows.CheckOutcome(definition, step.ID, outcome); err != nil {
		return domainworkflows.Outcome{}, err.Error()
	}
	return outcome, ""
}

func passWith(text, fallback string) domainworkflows.Outcome {
	text = strings.TrimSpace(text)
	if text == "" {
		text = fallback
	}
	return domainworkflows.Outcome{Verdict: domainworkflows.VerdictPass, Summary: text}
}

func stepWrites(step domainworkflows.Step) bool {
	return step.Kind == domainworkflows.StepCommand || step.Sandbox != string(agentrun.SandboxReadOnly)
}

var infrastructureCodes = []string{"worker_unavailable", "worker_unauthorized", "worker_workspace_unmapped", "worker_runtime_unavailable", "runtime_unavailable", "workspace_locked", "worktree_local_only", "remote_fs_local_agent_required", "command_local_only"}

func infrastructureFailure(err error) bool { return infrastructureReason(err) != "" }

func infrastructureReason(err error) string {
	message := err.Error()
	for _, code := range infrastructureCodes {
		if strings.Contains(message, code) {
			return code
		}
	}
	return ""
}

func truncateRunes(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "…"
}

func buildSummary(task domaintasks.Task, definition domainworkflows.Definition, run domainworkflows.Run) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "# %s\n\n", task.Title)
	fmt.Fprintf(&builder, "- Workflow: %s\n", definition.Name)
	fmt.Fprintf(&builder, "- Status: %s\n", run.Status)
	fmt.Fprintf(&builder, "- Completed: %s\n\n", run.CompletedAt.UTC().Format(time.RFC3339))
	builder.WriteString("## Steps\n\n")
	for _, step := range definition.Steps {
		node := run.Nodes[step.ID]
		fmt.Fprintf(&builder, "### %s (`%s`)\n\n- Verdict: %s\n- Attempts: %d\n\n%s\n\n", step.Name, step.ID, node.Verdict, node.Attempt, node.Summary)
	}
	return builder.String()
}
