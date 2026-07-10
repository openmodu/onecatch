package workflows

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/openmodu/oneshot/internal/agentrun"
	domaintasks "github.com/openmodu/oneshot/internal/domain/tasks"
	domainworkflows "github.com/openmodu/oneshot/internal/domain/workflows"
	domainworkspaces "github.com/openmodu/oneshot/internal/domain/workspaces"
)

type dagResult struct {
	step    domainworkflows.Step
	stepRun domainworkflows.StepRun
	result  agentrun.Result
	err     error
}

func (s *Usecase) driveDAG(ctx context.Context, task domaintasks.Task, workspace domainworkspaces.Workspace, definition domainworkflows.Definition, run domainworkflows.Run, instruction string) (domainworkflows.Run, error) {
	if run.Sessions == nil {
		run.Sessions = make(map[string]string)
	}
	if run.Nodes == nil {
		run.Nodes = make(map[string]domainworkflows.NodeState, len(definition.Steps))
		for _, step := range definition.Steps {
			run.Nodes[step.ID] = domainworkflows.NodeState{StepID: step.ID, Status: domainworkflows.NodePending}
		}
	}
	for stepID, node := range run.Nodes {
		if node.Status == domainworkflows.NodeRunning || node.Status == domainworkflows.NodePaused || node.Status == domainworkflows.NodeFailed {
			node.Status = domainworkflows.NodePending
			node.Error = ""
			run.Nodes[stepID] = node
		}
	}

	for run.Status == domainworkflows.RunRunning {
		ready := readyDAGSteps(definition, run.Nodes)
		if len(ready) == 0 {
			if allDAGNodesCompleted(run.Nodes) {
				run.Status = domainworkflows.RunCompleted
				run.CompletedAt = s.now()
				run.UpdatedAt = s.now()
				var err error
				run, err = s.workflows.UpdateRun(ctx, run, run.Revision)
				if err != nil {
					return run, err
				}
				if err := s.setTaskStatus(ctx, &task, domaintasks.StatusCompleted); err != nil {
					return run, err
				}
				if err := s.workflows.WriteRunSummary(ctx, run.ID, buildSummary(task, definition, run)); err != nil {
					return run, err
				}
				_ = s.appendEvent(ctx, run.ID, "run.completed", "", map[string]any{"mode": "dag", "nodes": len(run.Nodes)})
				return run, nil
			}
			return s.pauseDAG(ctx, &task, definition, run, "dag_blocked", "no DAG node is ready")
		}

		for _, step := range ready {
			node := run.Nodes[step.ID]
			node.Status = domainworkflows.NodeRunning
			node.Attempt++
			node.StartedAt = s.now()
			node.FinishedAt = time.Time{}
			run.Nodes[step.ID] = node
		}
		run.UpdatedAt = s.now()
		updated, err := s.workflows.UpdateRun(ctx, run, run.Revision)
		if err != nil {
			return run, err
		}
		run = updated
		batchCtx, cancel := context.WithCancel(ctx)
		results := make(chan dagResult, len(ready))
		runSnapshot := cloneDAGRun(run)
		for _, step := range ready {
			step := step
			go func() {
				results <- s.executeDAGStep(batchCtx, task, workspace, definition, runSnapshot, step, instruction)
			}()
		}
		paused := false
		pauseReason := ""
		pauseMessage := ""
		for range ready {
			item := <-results
			node := run.Nodes[item.step.ID]
			node.FinishedAt = s.now()
			if item.result.SessionID != "" {
				run.Sessions[item.step.ID] = item.result.SessionID
			}
			if item.err != nil {
				if ctx.Err() != nil || (paused && errors.Is(item.err, context.Canceled)) {
					node.Status = domainworkflows.NodePending
				} else {
					node.Status = domainworkflows.NodePaused
					node.Error = item.err.Error()
					paused, pauseReason, pauseMessage = true, dagPauseReason(item.err), item.err.Error()
					cancel()
				}
			} else {
				outcome, parseErr := domainworkflows.ParseOutcome(item.result.FinalMessage)
				target, declared := item.step.Transitions[outcome.Signal]
				if parseErr != nil || !declared {
					if parseErr == nil {
						parseErr = domainworkflows.ErrUnknownSignal{StepID: item.step.ID, Signal: outcome.Signal}
					}
					node.Status, node.Error = domainworkflows.NodePaused, parseErr.Error()
					paused, pauseReason, pauseMessage = true, "workflow_protocol_error", parseErr.Error()
					cancel()
				} else {
					node.Signal, node.Content = outcome.Signal, outcome.Content
					run.TransitionCount++
					run.History = append(run.History, domainworkflows.TransitionRecord{FromStepID: item.step.ID, Signal: outcome.Signal, Target: target, Content: outcome.Content, At: s.now()})
					switch target {
					case domainworkflows.TargetDone:
						node.Status = domainworkflows.NodeCompleted
					case domainworkflows.TargetPause:
						node.Status = domainworkflows.NodePaused
						paused, pauseReason, pauseMessage = true, domainworkflows.PauseReasonWorkflowSignal, outcome.Content
						cancel()
					case domainworkflows.TargetFail:
						node.Status = domainworkflows.NodeFailed
						run.Status, run.LastError = domainworkflows.RunFailed, outcome.Content
						cancel()
					}
				}
			}
			run.Nodes[item.step.ID] = node
			run.UpdatedAt = s.now()
			updated, updateErr := s.workflows.UpdateRun(context.Background(), run, run.Revision)
			if updateErr != nil {
				cancel()
				return run, updateErr
			}
			run = updated
		}
		cancel()
		instruction = ""
		if ctx.Err() != nil {
			return s.pauseDAG(context.Background(), &task, definition, run, PauseReasonInterrupted, ctx.Err().Error())
		}
		if run.Status == domainworkflows.RunFailed {
			_ = s.setTaskStatus(ctx, &task, domaintasks.StatusFailed)
			_ = s.appendEvent(ctx, run.ID, "run.failed", "", map[string]any{"error": run.LastError, "mode": "dag"})
			return run, nil
		}
		if run.TransitionCount >= definition.Policy.MaxTransitions {
			paused, pauseReason, pauseMessage = true, domainworkflows.PauseReasonTransitionLimit, "maximum DAG transitions reached"
		}
		if paused {
			return s.pauseDAG(ctx, &task, definition, run, pauseReason, pauseMessage)
		}
	}
	return run, nil
}

func (s *Usecase) executeDAGStep(ctx context.Context, task domaintasks.Task, workspace domainworkspaces.Workspace, definition domainworkflows.Definition, run domainworkflows.Run, step domainworkflows.Step, instruction string) dagResult {
	attempt := run.Nodes[step.ID].Attempt
	stepRun := domainworkflows.StepRun{ID: s.newID("step"), RunID: run.ID, StepID: step.ID, Attempt: attempt, Status: domainworkflows.StepRunRunning, SessionIDBefore: run.Sessions[step.ID], StartedAt: s.now()}
	if err := s.workflows.SaveStepRun(ctx, stepRun); err != nil {
		return dagResult{step: step, stepRun: stepRun, err: err}
	}
	_ = s.appendEvent(ctx, run.ID, "step.started", step.ID, map[string]any{"stepRunId": stepRun.ID, "attempt": attempt, "runtime": step.Runtime, "workerId": step.WorkerID, "mode": "dag"})
	if err := s.recordGit(ctx, run.ID, step.ID, "before", workspace.Path); err != nil {
		return dagResult{step: step, stepRun: stepRun, err: err}
	}
	var release func() error
	var err error
	if step.Sandbox != "read-only" && (step.WorkerID == "" || step.WorkerID == "local") {
		release, err = s.locker.Acquire(ctx, workspace.ID, workspace.Path, run.ID)
		if err != nil {
			return dagResult{step: step, stepRun: stepRun, err: err}
		}
		defer release()
	}
	request := agentrun.Request{Runtime: agentrun.Runtime(step.Runtime), Workspace: workspace.Path, Prompt: composeDAGPrompt(task, definition, step, run, instruction), Model: step.Model, Sandbox: allowedSandbox(step.Sandbox, workspace.DefaultSandbox), ResumeSessionID: run.Sessions[step.ID]}
	var result agentrun.Result
	if step.WorkerID != "" && step.WorkerID != "local" {
		if s.remote == nil {
			err = fmt.Errorf("worker_unavailable: remote executor is not configured")
		} else {
			result, err = s.remote.RunRemote(ctx, step.WorkerID, workspace.ID, request, s.runtimeSink(run.ID, stepRun.ID))
		}
	} else if !request.Runtime.Valid() || !s.engine.Available(request.Runtime) {
		err = fmt.Errorf("runtime_unavailable: runtime %q is unavailable", step.Runtime)
	} else {
		stepCtx, cancel := context.WithTimeout(ctx, time.Duration(definition.Policy.StepTimeoutSeconds)*time.Second)
		result, err = s.engine.Run(stepCtx, request, s.runtimeSink(run.ID, stepRun.ID))
		cancel()
	}
	stepRun.FinishedAt = s.now()
	stepRun.SessionIDAfter = result.SessionID
	if err != nil || !result.Succeeded {
		stepRun.Status, stepRun.Error = domainworkflows.StepRunFailed, failureMessage(err, result.FinalMessage)
		if err == nil {
			err = fmt.Errorf("dag node failed: %s", stepRun.Error)
		}
	} else {
		stepRun.Status = domainworkflows.StepRunSucceeded
		if outcome, parseErr := domainworkflows.ParseOutcome(result.FinalMessage); parseErr == nil {
			stepRun.Signal, stepRun.Content = outcome.Signal, outcome.Content
		}
	}
	_ = s.workflows.SaveStepRun(context.Background(), stepRun)
	_ = s.recordGit(context.Background(), run.ID, step.ID, "after", workspace.Path)
	return dagResult{step: step, stepRun: stepRun, result: result, err: err}
}

func (s *Usecase) runtimeSink(runID, stepRunID string) agentrun.Sink {
	return func(event agentrun.Event) {
		payload := mustJSON(event)
		_, _ = s.workflows.AppendRuntimeEvent(context.Background(), runID, stepRunID, payload)
	}
}

func readyDAGSteps(definition domainworkflows.Definition, nodes map[string]domainworkflows.NodeState) []domainworkflows.Step {
	var ready []domainworkflows.Step
	for _, step := range definition.Steps {
		if node := nodes[step.ID]; node.Status != domainworkflows.NodePending {
			continue
		}
		dependenciesComplete := true
		for _, dependency := range step.DependsOn {
			if nodes[dependency].Status != domainworkflows.NodeCompleted {
				dependenciesComplete = false
				break
			}
		}
		if dependenciesComplete {
			ready = append(ready, step)
		}
	}
	sort.Slice(ready, func(i, j int) bool { return ready[i].ID < ready[j].ID })
	return ready
}

func allDAGNodesCompleted(nodes map[string]domainworkflows.NodeState) bool {
	if len(nodes) == 0 {
		return false
	}
	for _, node := range nodes {
		if node.Status != domainworkflows.NodeCompleted {
			return false
		}
	}
	return true
}

func (s *Usecase) pauseDAG(ctx context.Context, task *domaintasks.Task, definition domainworkflows.Definition, run domainworkflows.Run, reason, message string) (domainworkflows.Run, error) {
	paused, err := domainworkflows.Pause(definition, run, reason, message, s.now())
	if err != nil {
		return run, err
	}
	paused, err = s.workflows.UpdateRun(ctx, paused, run.Revision)
	if err != nil {
		return run, err
	}
	_ = s.setTaskStatus(ctx, task, domaintasks.StatusPaused)
	_ = s.appendEvent(ctx, run.ID, "run.paused", "", map[string]any{"reason": reason, "error": message, "mode": "dag"})
	return paused, nil
}

func composeDAGPrompt(task domaintasks.Task, definition domainworkflows.Definition, step domainworkflows.Step, run domainworkflows.Run, instruction string) string {
	parts := []string{"# Oneshot DAG node", "", "## Task", task.Prompt, "", "## Your role", step.RolePrompt, "", "## Node instruction", step.Instruction}
	if strings.TrimSpace(instruction) != "" {
		parts = append(parts, "", "## Human instruction", instruction)
	}
	if len(step.DependsOn) > 0 {
		parts = append(parts, "", "## Direct dependency results")
		for _, dependency := range step.DependsOn {
			node := run.Nodes[dependency]
			parts = append(parts, fmt.Sprintf("- %s: %s", dependency, node.Content))
		}
	}
	signals := make([]string, 0, len(step.Transitions))
	for signal := range step.Transitions {
		signals = append(signals, signal)
	}
	sort.Strings(signals)
	parts = append(parts, "", "## Allowed outcomes")
	for _, signal := range signals {
		parts = append(parts, fmt.Sprintf("- `%s`: `%s`", signal, step.Transitions[signal]))
	}
	parts = append(parts, "", "## Final response contract", `Return exactly one JSON object: {"signal":"<allowed signal>","content":"concise result for dependent nodes"}`)
	return strings.Join(parts, "\n") + "\n"
}

func dagPauseReason(err error) string {
	message := err.Error()
	for _, code := range []string{"worker_unavailable", "worker_unauthorized", "worker_workspace_unmapped", "worker_runtime_unavailable", "runtime_unavailable", "workspace_locked"} {
		if strings.Contains(message, code) {
			return code
		}
	}
	return "dag_node_failed"
}

func cloneDAGRun(run domainworkflows.Run) domainworkflows.Run {
	out := run
	out.Sessions = make(map[string]string, len(run.Sessions))
	for stepID, sessionID := range run.Sessions {
		out.Sessions[stepID] = sessionID
	}
	out.Nodes = make(map[string]domainworkflows.NodeState, len(run.Nodes))
	for stepID, node := range run.Nodes {
		out.Nodes[stepID] = node
	}
	out.History = append([]domainworkflows.TransitionRecord(nil), run.History...)
	return out
}
