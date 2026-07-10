// Package workflows orchestrates local Agent workflow runs. It composes the
// pure workflow state machine, local repositories, runtime adapters, workspace
// locking and read-only git inspection.
package workflows

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/openmodu/oneshot/internal/agentrun"
	domaintasks "github.com/openmodu/oneshot/internal/domain/tasks"
	domainworkflows "github.com/openmodu/oneshot/internal/domain/workflows"
	domainworkspaces "github.com/openmodu/oneshot/internal/domain/workspaces"
)

const (
	PauseReasonInterrupted        = "interrupted"
	PauseReasonRuntimeUnavailable = "runtime_unavailable"
	PauseReasonWorkspaceLocked    = "workspace_locked"
)

type TaskRepository interface {
	GetTask(context.Context, string) (domaintasks.Task, error)
	SaveTask(context.Context, domaintasks.Task) error
	GetWorkspace(context.Context, string) (domainworkspaces.Workspace, error)
}

type WorkflowRepository interface {
	GetDefinition(context.Context, string) (domainworkflows.Definition, error)
	SaveRun(context.Context, domainworkflows.Run, domainworkflows.Definition) error
	GetRun(context.Context, string) (domainworkflows.Run, error)
	GetRunDefinition(context.Context, string) (domainworkflows.Definition, error)
	UpdateRun(context.Context, domainworkflows.Run, int64) (domainworkflows.Run, error)
	SaveStepRun(context.Context, domainworkflows.StepRun) error
	ListStepRuns(context.Context, string) ([]domainworkflows.StepRun, error)
	AppendEvent(context.Context, domainworkflows.WorkflowEvent) (domainworkflows.WorkflowEvent, error)
	AppendRuntimeEvent(context.Context, string, string, json.RawMessage) (domainworkflows.RuntimeEvent, error)
	WriteRunSummary(context.Context, string, string) error
}

type Engine interface {
	Available(agentrun.Runtime) bool
	Run(context.Context, agentrun.Request, agentrun.Sink) (agentrun.Result, error)
}

type WorkspaceLocker interface {
	Acquire(context.Context, string, string, string) (func() error, error)
}

type GitInspector interface {
	Inspect(context.Context, string) (domainworkspaces.GitSnapshot, error)
}

type IDGenerator func(prefix string) string

type Usecase struct {
	tasks     TaskRepository
	workflows WorkflowRepository
	engine    Engine
	locker    WorkspaceLocker
	git       GitInspector
	now       func() time.Time
	newID     IDGenerator
}

func NewUsecase(tasks TaskRepository, workflows WorkflowRepository, engine Engine, locker WorkspaceLocker, git GitInspector) *Usecase {
	return &Usecase{
		tasks:     tasks,
		workflows: workflows,
		engine:    engine,
		locker:    locker,
		git:       git,
		now:       time.Now,
		newID:     randomID,
	}
}

// StartTask creates and persists a running Run and its private workflow
// snapshot. Execution is deliberately separate so desktop callers receive the
// durable run ID before a potentially long Agent process starts.
func (s *Usecase) StartTask(ctx context.Context, taskID string) (domainworkflows.Run, error) {
	task, _, definition, err := s.loadTaskContext(ctx, taskID)
	if err != nil {
		return domainworkflows.Run{}, err
	}
	run, err := domainworkflows.NewRun(definition, s.newID("run"), s.now())
	if err != nil {
		return domainworkflows.Run{}, err
	}
	run.TaskID = task.ID
	run, err = domainworkflows.Start(definition, run, s.now())
	if err != nil {
		return domainworkflows.Run{}, err
	}
	if err := s.workflows.SaveRun(ctx, run, definition); err != nil {
		return run, err
	}
	if err := s.setTaskStatus(ctx, &task, domaintasks.StatusRunning); err != nil {
		return run, err
	}
	if err := s.appendEvent(ctx, run.ID, "run.started", "", map[string]any{"taskId": task.ID, "workflowId": definition.ID}); err != nil {
		return run, err
	}
	return run, nil
}

// ExecuteTask is the synchronous compatibility entry point used by tests and
// non-UI callers.
func (s *Usecase) ExecuteTask(ctx context.Context, taskID string) (domainworkflows.Run, error) {
	run, err := s.StartTask(ctx, taskID)
	if err != nil {
		return run, err
	}
	return s.ExecuteRun(ctx, run.ID)
}

// ExecuteRun drives a previously prepared running Run.
func (s *Usecase) ExecuteRun(ctx context.Context, runID string) (domainworkflows.Run, error) {
	run, err := s.workflows.GetRun(ctx, runID)
	if err != nil {
		return domainworkflows.Run{}, err
	}
	definition, err := s.workflows.GetRunDefinition(ctx, runID)
	if err != nil {
		return run, err
	}
	if run.Status != domainworkflows.RunRunning {
		return run, fmt.Errorf("%w: execute from %s", domainworkflows.ErrInvalidRunState, run.Status)
	}
	task, err := s.tasks.GetTask(ctx, run.TaskID)
	if err != nil {
		return run, err
	}
	workspace, err := s.tasks.GetWorkspace(ctx, task.WorkspaceID)
	if err != nil {
		return run, err
	}
	release, err := s.locker.Acquire(ctx, workspace.ID, workspace.Path, run.ID)
	if err != nil {
		paused, pauseErr := domainworkflows.Pause(definition, run, PauseReasonWorkspaceLocked, err.Error(), s.now())
		if pauseErr != nil {
			return run, err
		}
		paused, updateErr := s.workflows.UpdateRun(context.Background(), paused, run.Revision)
		if updateErr != nil {
			return run, updateErr
		}
		_ = s.setTaskStatus(context.Background(), &task, domaintasks.StatusPaused)
		_ = s.appendEvent(context.Background(), run.ID, "run.paused", run.CurrentStepID, map[string]any{"reason": PauseReasonWorkspaceLocked, "error": err.Error()})
		return paused, err
	}
	defer release()
	return s.drive(ctx, task, workspace, definition, run, "")
}

// RecoverRun turns a durable running Run with no live workspace owner into a
// resumable paused Run. If another process still owns the workspace lock it is
// left untouched.
func (s *Usecase) RecoverRun(ctx context.Context, runID string) (domainworkflows.Run, error) {
	run, err := s.workflows.GetRun(ctx, runID)
	if err != nil {
		return domainworkflows.Run{}, err
	}
	if run.Status != domainworkflows.RunRunning {
		return run, nil
	}
	definition, err := s.workflows.GetRunDefinition(ctx, runID)
	if err != nil {
		return run, err
	}
	task, err := s.tasks.GetTask(ctx, run.TaskID)
	if err != nil {
		return run, err
	}
	workspace, err := s.tasks.GetWorkspace(ctx, task.WorkspaceID)
	if err != nil {
		return run, err
	}
	release, err := s.locker.Acquire(ctx, workspace.ID, workspace.Path, run.ID)
	if err != nil {
		return run, err
	}
	defer release()
	message := "desktop exited before the run completed"
	paused, err := domainworkflows.Pause(definition, run, PauseReasonInterrupted, message, s.now())
	if err != nil {
		return run, err
	}
	paused, err = s.workflows.UpdateRun(ctx, paused, run.Revision)
	if err != nil {
		return run, err
	}
	if err := s.setTaskStatus(ctx, &task, domaintasks.StatusPaused); err != nil {
		return paused, err
	}
	if err := s.appendEvent(ctx, run.ID, "run.paused", run.CurrentStepID, map[string]any{"reason": PauseReasonInterrupted, "error": message, "recovered": true}); err != nil {
		return paused, err
	}
	return paused, nil
}

// ResumeRun resumes the current step from the Run's private workflow snapshot.
// instruction is injected once into the first resumed step prompt.
func (s *Usecase) ResumeRun(ctx context.Context, runID, instruction string) (domainworkflows.Run, error) {
	run, err := s.workflows.GetRun(ctx, runID)
	if err != nil {
		return domainworkflows.Run{}, err
	}
	definition, err := s.workflows.GetRunDefinition(ctx, runID)
	if err != nil {
		return run, err
	}
	task, err := s.tasks.GetTask(ctx, run.TaskID)
	if err != nil {
		return run, err
	}
	workspace, err := s.tasks.GetWorkspace(ctx, task.WorkspaceID)
	if err != nil {
		return run, err
	}
	release, err := s.locker.Acquire(ctx, workspace.ID, workspace.Path, run.ID)
	if err != nil {
		return run, err
	}
	defer release()
	resumed, err := domainworkflows.Resume(definition, run, s.now())
	if err != nil {
		return run, err
	}
	resumed, err = s.workflows.UpdateRun(ctx, resumed, run.Revision)
	if err != nil {
		return run, err
	}
	if err := s.setTaskStatus(ctx, &task, domaintasks.StatusRunning); err != nil {
		return resumed, err
	}
	if err := s.appendEvent(ctx, run.ID, "run.resumed", resumed.CurrentStepID, map[string]any{"hasInstruction": strings.TrimSpace(instruction) != ""}); err != nil {
		return resumed, err
	}
	return s.drive(ctx, task, workspace, definition, resumed, instruction)
}

// CancelRun marks a ready or paused run and its task as cancelled. Active runs
// are interrupted first by the application service, which causes drive to
// persist the paused state before this method is called.
func (s *Usecase) CancelRun(ctx context.Context, runID string) (domainworkflows.Run, error) {
	run, err := s.workflows.GetRun(ctx, runID)
	if err != nil {
		return domainworkflows.Run{}, err
	}
	definition, err := s.workflows.GetRunDefinition(ctx, runID)
	if err != nil {
		return run, err
	}
	cancelled, err := domainworkflows.Cancel(definition, run, s.now())
	if err != nil {
		return run, err
	}
	cancelled, err = s.workflows.UpdateRun(ctx, cancelled, run.Revision)
	if err != nil {
		return run, err
	}
	task, err := s.tasks.GetTask(ctx, run.TaskID)
	if err != nil {
		return cancelled, err
	}
	if err := s.setTaskStatus(ctx, &task, domaintasks.StatusCancelled); err != nil {
		return cancelled, err
	}
	if err := s.appendEvent(ctx, run.ID, "run.cancelled", run.CurrentStepID, map[string]any{}); err != nil {
		return cancelled, err
	}
	return cancelled, nil
}

func (s *Usecase) drive(ctx context.Context, task domaintasks.Task, workspace domainworkspaces.Workspace, definition domainworkflows.Definition, run domainworkflows.Run, instruction string) (domainworkflows.Run, error) {
	if run.Sessions == nil {
		run.Sessions = make(map[string]string)
	}
	for run.Status == domainworkflows.RunRunning {
		step, ok := findStep(definition, run.CurrentStepID)
		if !ok {
			return run, fmt.Errorf("workflow step %q not found", run.CurrentStepID)
		}
		attempt, err := s.nextAttempt(ctx, run.ID, step.ID)
		if err != nil {
			return run, err
		}
		stepRun := domainworkflows.StepRun{
			ID:              s.newID("step"),
			RunID:           run.ID,
			StepID:          step.ID,
			Attempt:         attempt,
			Status:          domainworkflows.StepRunRunning,
			SessionIDBefore: run.Sessions[step.ID],
			StartedAt:       s.now(),
		}
		if err := s.workflows.SaveStepRun(ctx, stepRun); err != nil {
			return run, err
		}
		if err := s.appendEvent(ctx, run.ID, "step.started", step.ID, map[string]any{"stepRunId": stepRun.ID, "attempt": attempt, "runtime": step.Runtime}); err != nil {
			return run, err
		}
		if err := s.recordGit(ctx, run.ID, step.ID, "before", workspace.Path); err != nil {
			return run, err
		}

		runtime := agentrun.Runtime(step.Runtime)
		if !runtime.Valid() || !s.engine.Available(runtime) {
			message := fmt.Sprintf("runtime %q is unavailable", step.Runtime)
			stepRun.Status = domainworkflows.StepRunFailed
			stepRun.Error = message
			stepRun.FinishedAt = s.now()
			if err := s.workflows.SaveStepRun(ctx, stepRun); err != nil {
				return run, err
			}
			if err := s.appendEvent(ctx, run.ID, "step.failed", step.ID, map[string]any{"stepRunId": stepRun.ID, "error": message}); err != nil {
				return run, err
			}
			paused, err := domainworkflows.Pause(definition, run, PauseReasonRuntimeUnavailable, message, s.now())
			if err != nil {
				return run, err
			}
			run, err = s.workflows.UpdateRun(ctx, paused, run.Revision)
			if err != nil {
				return run, err
			}
			if err := s.appendEvent(ctx, run.ID, "run.paused", step.ID, map[string]any{"reason": PauseReasonRuntimeUnavailable, "error": message}); err != nil {
				return run, err
			}
			if err := s.setTaskStatus(ctx, &task, domaintasks.StatusPaused); err != nil {
				return run, err
			}
			return run, nil
		}

		stepCtx, cancel := context.WithTimeout(ctx, time.Duration(definition.Policy.StepTimeoutSeconds)*time.Second)
		var streamErr error
		result, runErr := s.engine.Run(stepCtx, agentrun.Request{
			Runtime:         runtime,
			Workspace:       workspace.Path,
			Prompt:          composePrompt(task, definition, step, run, instruction),
			Model:           step.Model,
			Sandbox:         allowedSandbox(step.Sandbox, workspace.DefaultSandbox),
			ResumeSessionID: run.Sessions[step.ID],
		}, func(event agentrun.Event) {
			payload, err := json.Marshal(event)
			if err == nil && streamErr == nil {
				_, streamErr = s.workflows.AppendRuntimeEvent(context.Background(), run.ID, stepRun.ID, payload)
			}
		})
		cancel()
		if streamErr != nil && runErr == nil {
			runErr = fmt.Errorf("persist runtime event: %w", streamErr)
		}
		if result.SessionID != "" {
			run.Sessions[step.ID] = result.SessionID
			stepRun.SessionIDAfter = result.SessionID
		}
		if err := s.recordGit(ctx, run.ID, step.ID, "after", workspace.Path); err != nil && ctx.Err() == nil {
			return run, err
		}

		if ctx.Err() != nil {
			stepRun.Status = domainworkflows.StepRunInterrupted
			stepRun.Error = ctx.Err().Error()
			stepRun.FinishedAt = s.now()
			if err := s.workflows.SaveStepRun(context.Background(), stepRun); err != nil {
				return run, err
			}
			paused, err := domainworkflows.Pause(definition, run, PauseReasonInterrupted, ctx.Err().Error(), s.now())
			if err != nil {
				return run, err
			}
			run, err = s.workflows.UpdateRun(context.Background(), paused, run.Revision)
			if err != nil {
				return run, err
			}
			_, _ = s.workflows.AppendEvent(context.Background(), domainworkflows.WorkflowEvent{RunID: run.ID, Type: "run.paused", StepID: step.ID, Payload: mustJSON(map[string]any{"reason": PauseReasonInterrupted}), At: s.now()})
			_ = s.setTaskStatus(context.Background(), &task, domaintasks.StatusPaused)
			return run, ctx.Err()
		}

		if runErr != nil || !result.Succeeded {
			message := failureMessage(runErr, result.FinalMessage)
			stepRun.Status = domainworkflows.StepRunFailed
			stepRun.Error = message
			stepRun.FinishedAt = s.now()
			if err := s.workflows.SaveStepRun(ctx, stepRun); err != nil {
				return run, err
			}
			next, err := domainworkflows.RecordFailure(definition, run, message, s.now())
			if err != nil {
				return run, err
			}
			run, err = s.workflows.UpdateRun(ctx, next, run.Revision)
			if err != nil {
				return run, err
			}
			if err := s.appendEvent(ctx, run.ID, "step.failed", step.ID, map[string]any{"stepRunId": stepRun.ID, "error": message, "consecutiveFailures": run.ConsecutiveFailures}); err != nil {
				return run, err
			}
			if run.Status == domainworkflows.RunPaused {
				if err := s.setTaskStatus(ctx, &task, domaintasks.StatusPaused); err != nil {
					return run, err
				}
				if err := s.appendEvent(ctx, run.ID, "run.paused", step.ID, map[string]any{"reason": run.PauseReason, "error": run.LastError}); err != nil {
					return run, err
				}
				return run, nil
			}
			instruction = ""
			continue
		}

		outcome, protocolErr := domainworkflows.ParseOutcome(result.FinalMessage)
		if protocolErr == nil {
			if _, declared := step.Transitions[outcome.Signal]; !declared {
				protocolErr = domainworkflows.ErrUnknownSignal{StepID: step.ID, Signal: outcome.Signal}
			}
		}
		if protocolErr != nil {
			stepRun.Status = domainworkflows.StepRunFailed
			stepRun.Error = protocolErr.Error()
			stepRun.FinishedAt = s.now()
			if err := s.workflows.SaveStepRun(ctx, stepRun); err != nil {
				return run, err
			}
			next, err := domainworkflows.RecordFailure(definition, run, protocolErr.Error(), s.now())
			if err != nil {
				return run, err
			}
			run, err = s.workflows.UpdateRun(ctx, next, run.Revision)
			if err != nil {
				return run, err
			}
			if err := s.appendEvent(ctx, run.ID, "step.protocol_error", step.ID, map[string]any{"stepRunId": stepRun.ID, "error": protocolErr.Error()}); err != nil {
				return run, err
			}
			if run.Status == domainworkflows.RunPaused {
				if err := s.setTaskStatus(ctx, &task, domaintasks.StatusPaused); err != nil {
					return run, err
				}
				if err := s.appendEvent(ctx, run.ID, "run.paused", step.ID, map[string]any{"reason": run.PauseReason, "error": run.LastError}); err != nil {
					return run, err
				}
				return run, nil
			}
			instruction = ""
			continue
		}

		stepRun.Status = domainworkflows.StepRunSucceeded
		stepRun.Signal = outcome.Signal
		stepRun.Content = outcome.Content
		stepRun.FinishedAt = s.now()
		if err := s.workflows.SaveStepRun(ctx, stepRun); err != nil {
			return run, err
		}
		next, err := domainworkflows.Advance(definition, run, outcome, s.now())
		if err != nil {
			return run, err
		}
		run, err = s.workflows.UpdateRun(ctx, next, run.Revision)
		if err != nil {
			return run, err
		}
		if err := s.appendEvent(ctx, run.ID, "transition.applied", step.ID, map[string]any{"stepRunId": stepRun.ID, "signal": outcome.Signal, "target": run.History[len(run.History)-1].Target}); err != nil {
			return run, err
		}
		instruction = ""
	}

	switch run.Status {
	case domainworkflows.RunCompleted:
		if err := s.setTaskStatus(ctx, &task, domaintasks.StatusCompleted); err != nil {
			return run, err
		}
		if err := s.workflows.WriteRunSummary(ctx, run.ID, buildSummary(task, definition, run)); err != nil {
			return run, err
		}
		if err := s.appendEvent(ctx, run.ID, "run.completed", run.CurrentStepID, map[string]any{"transitions": run.TransitionCount}); err != nil {
			return run, err
		}
	case domainworkflows.RunPaused:
		if err := s.setTaskStatus(ctx, &task, domaintasks.StatusPaused); err != nil {
			return run, err
		}
		if err := s.appendEvent(ctx, run.ID, "run.paused", run.CurrentStepID, map[string]any{"reason": run.PauseReason}); err != nil {
			return run, err
		}
	case domainworkflows.RunFailed:
		if err := s.setTaskStatus(ctx, &task, domaintasks.StatusFailed); err != nil {
			return run, err
		}
		if err := s.appendEvent(ctx, run.ID, "run.failed", run.CurrentStepID, map[string]any{"error": run.LastError}); err != nil {
			return run, err
		}
	}
	return run, nil
}

func (s *Usecase) loadTaskContext(ctx context.Context, taskID string) (domaintasks.Task, domainworkspaces.Workspace, domainworkflows.Definition, error) {
	task, err := s.tasks.GetTask(ctx, taskID)
	if err != nil {
		return domaintasks.Task{}, domainworkspaces.Workspace{}, domainworkflows.Definition{}, err
	}
	workspace, err := s.tasks.GetWorkspace(ctx, task.WorkspaceID)
	if err != nil {
		return domaintasks.Task{}, domainworkspaces.Workspace{}, domainworkflows.Definition{}, err
	}
	definition, err := s.workflows.GetDefinition(ctx, task.WorkflowID)
	if err != nil {
		return domaintasks.Task{}, domainworkspaces.Workspace{}, domainworkflows.Definition{}, err
	}
	return task, workspace, definition, nil
}

func (s *Usecase) nextAttempt(ctx context.Context, runID, stepID string) (int, error) {
	items, err := s.workflows.ListStepRuns(ctx, runID)
	if err != nil {
		return 0, err
	}
	maxAttempt := 0
	for _, item := range items {
		if item.StepID == stepID && item.Attempt > maxAttempt {
			maxAttempt = item.Attempt
		}
	}
	return maxAttempt + 1, nil
}

func (s *Usecase) setTaskStatus(ctx context.Context, task *domaintasks.Task, status domaintasks.Status) error {
	task.Status = status
	task.UpdatedAt = s.now()
	return s.tasks.SaveTask(ctx, *task)
}

func (s *Usecase) appendEvent(ctx context.Context, runID, eventType, stepID string, payload any) error {
	_, err := s.workflows.AppendEvent(ctx, domainworkflows.WorkflowEvent{RunID: runID, Type: eventType, StepID: stepID, Payload: mustJSON(payload), At: s.now()})
	return err
}

func (s *Usecase) recordGit(ctx context.Context, runID, stepID, phase, workspace string) error {
	if s.git == nil {
		return nil
	}
	snapshot, err := s.git.Inspect(ctx, workspace)
	if err != nil {
		return s.appendEvent(ctx, runID, "git.inspect_failed", stepID, map[string]any{"phase": phase, "error": err.Error()})
	}
	return s.appendEvent(ctx, runID, "git.snapshot", stepID, map[string]any{"phase": phase, "snapshot": snapshot})
}

func composePrompt(task domaintasks.Task, definition domainworkflows.Definition, step domainworkflows.Step, run domainworkflows.Run, instruction string) string {
	parts := []string{
		"# Oneshot workflow step",
		"",
		"## Task",
		task.Prompt,
		"",
		"## Your role",
		step.RolePrompt,
		"",
		"## Step instruction",
		step.Instruction,
	}
	if strings.TrimSpace(instruction) != "" {
		parts = append(parts, "", "## Human instruction", instruction)
	}
	if len(run.History) > 0 {
		parts = append(parts, "", "## Recent workflow outcomes")
		start := len(run.History) - 8
		if start < 0 {
			start = 0
		}
		for _, item := range run.History[start:] {
			parts = append(parts, fmt.Sprintf("- %s --%s--> %s: %s", item.FromStepID, item.Signal, item.Target, item.Content))
		}
	}
	signals := make([]string, 0, len(step.Transitions))
	for signal := range step.Transitions {
		signals = append(signals, signal)
	}
	sort.Strings(signals)
	parts = append(parts, "", "## Allowed outcomes")
	for _, signal := range signals {
		parts = append(parts, fmt.Sprintf("- `%s`: transitions to `%s`", signal, step.Transitions[signal]))
	}
	parts = append(parts,
		"",
		"## Final response contract",
		"Return exactly one JSON object as your final response, with no Markdown fence and no text before or after it:",
		`{"signal":"<one allowed signal>","content":"concise result and handoff context"}`,
		"The signal must be one of the allowed outcomes above. Do not invent a target or another signal.",
	)
	return strings.Join(parts, "\n") + "\n"
}

func allowedSandbox(requested, maximum string) agentrun.Sandbox {
	if maximum == "" {
		maximum = string(agentrun.SandboxWorkspaceWrite)
	}
	if requested == "" {
		requested = maximum
	}
	rank := map[string]int{"read-only": 0, "workspace-write": 1, "full": 2}
	requestedRank, ok := rank[requested]
	if !ok {
		requested, requestedRank = "workspace-write", 1
	}
	maximumRank, ok := rank[maximum]
	if !ok {
		maximum, maximumRank = "workspace-write", 1
	}
	if requestedRank > maximumRank {
		return agentrun.Sandbox(maximum)
	}
	return agentrun.Sandbox(requested)
}

func findStep(definition domainworkflows.Definition, id string) (domainworkflows.Step, bool) {
	for _, step := range definition.Steps {
		if step.ID == id {
			return step, true
		}
	}
	return domainworkflows.Step{}, false
}

func failureMessage(err error, finalMessage string) string {
	if err != nil {
		return err.Error()
	}
	if strings.TrimSpace(finalMessage) != "" {
		return finalMessage
	}
	return "agent did not complete successfully"
}

func mustJSON(value any) json.RawMessage {
	payload, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage(`{"error":"encode event payload"}`)
	}
	return payload
}

func buildSummary(task domaintasks.Task, definition domainworkflows.Definition, run domainworkflows.Run) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "# %s\n\n", task.Title)
	fmt.Fprintf(&builder, "- Workflow: %s\n", definition.Name)
	fmt.Fprintf(&builder, "- Status: %s\n", run.Status)
	fmt.Fprintf(&builder, "- Transitions: %d\n", run.TransitionCount)
	fmt.Fprintf(&builder, "- Completed: %s\n\n", run.CompletedAt.UTC().Format(time.RFC3339))
	builder.WriteString("## Outcomes\n\n")
	for _, item := range run.History {
		fmt.Fprintf(&builder, "- `%s` --`%s`--> `%s`: %s\n", item.FromStepID, item.Signal, item.Target, item.Content)
	}
	return builder.String()
}

func randomID(prefix string) string {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return prefix + "_" + hex.EncodeToString(bytes)
}
