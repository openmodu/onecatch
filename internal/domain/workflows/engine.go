package workflows

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrInvalidRunState       = errors.New("workflow run state does not allow this operation")
	ErrRunDefinitionMismatch = errors.New("workflow run definition mismatch")
	ErrUnknownStep           = errors.New("workflow current step does not exist")
)

const (
	PauseReasonTransitionLimit = "max_transitions_reached"
	PauseReasonFailureLimit    = "max_consecutive_failures_reached"
	PauseReasonWorkflowSignal  = "workflow_signal"
)

type ErrUnknownSignal struct {
	StepID string
	Signal string
}

func (e ErrUnknownSignal) Error() string {
	return fmt.Sprintf("workflow step %q does not declare signal %q", e.StepID, e.Signal)
}

// NewRun creates a ready run for a workflow snapshot.
func NewRun(input Definition, runID string, now time.Time) (Run, error) {
	def := Normalize(input)
	if err := Validate(def); err != nil {
		return Run{}, err
	}
	if strings.TrimSpace(runID) == "" {
		return Run{}, fmt.Errorf("run id is required")
	}
	run := Run{
		ID:            runID,
		WorkflowID:    def.ID,
		Status:        RunReady,
		CurrentStepID: def.EntryStepID,
		Revision:      1,
		Sessions:      make(map[string]string),
		StartedAt:     now,
		UpdatedAt:     now,
	}
	if def.Mode == ModeDAG {
		run.Nodes = make(map[string]NodeState, len(def.Steps))
		for _, step := range def.Steps {
			run.Nodes[step.ID] = NodeState{StepID: step.ID, Status: NodePending}
		}
	}
	return run, nil
}

func Start(input Definition, run Run, now time.Time) (Run, error) {
	def := Normalize(input)
	if err := validateRunDefinition(def, run); err != nil {
		return Run{}, err
	}
	if run.Status != RunReady {
		return Run{}, fmt.Errorf("%w: start from %s", ErrInvalidRunState, run.Status)
	}
	out := cloneRun(run)
	out.Status = RunRunning
	out.UpdatedAt = now
	return out, nil
}

// Advance applies one valid Agent outcome. The returned run is a copy; on any
// error the caller's input remains unchanged.
func Advance(input Definition, run Run, outcome Outcome, now time.Time) (Run, error) {
	def := Normalize(input)
	if err := validateRunDefinition(def, run); err != nil {
		return Run{}, err
	}
	if run.Status != RunRunning {
		return Run{}, fmt.Errorf("%w: advance from %s", ErrInvalidRunState, run.Status)
	}
	step, ok := findStep(def, run.CurrentStepID)
	if !ok {
		return Run{}, fmt.Errorf("%w: %s", ErrUnknownStep, run.CurrentStepID)
	}
	target, ok := step.Transitions[outcome.Signal]
	if !ok {
		return Run{}, ErrUnknownSignal{StepID: step.ID, Signal: outcome.Signal}
	}

	out := cloneRun(run)
	out.TransitionCount++
	out.ConsecutiveFailures = 0
	out.LastError = ""
	out.PauseReason = ""
	out.UpdatedAt = now
	out.History = append(out.History, TransitionRecord{
		FromStepID: step.ID,
		Signal:     outcome.Signal,
		Target:     target,
		Content:    outcome.Content,
		At:         now,
	})

	switch target {
	case TargetDone:
		out.Status = RunCompleted
		out.CompletedAt = now
	case TargetPause:
		out.Status = RunPaused
		out.PauseReason = PauseReasonWorkflowSignal
	case TargetFail:
		out.Status = RunFailed
		out.LastError = outcome.Content
	default:
		out.CurrentStepID = target
		if out.TransitionCount >= def.Policy.MaxTransitions {
			out.Status = RunPaused
			out.PauseReason = PauseReasonTransitionLimit
		} else {
			out.Status = RunRunning
		}
	}
	return out, nil
}

// RecordFailure records a runtime or outcome-protocol failure for the current
// step. Below the configured limit it remains running so an orchestrator may
// retry; at the limit it pauses for human intervention.
func RecordFailure(input Definition, run Run, message string, now time.Time) (Run, error) {
	def := Normalize(input)
	if err := validateRunDefinition(def, run); err != nil {
		return Run{}, err
	}
	if run.Status != RunRunning {
		return Run{}, fmt.Errorf("%w: record failure from %s", ErrInvalidRunState, run.Status)
	}
	out := cloneRun(run)
	out.ConsecutiveFailures++
	out.LastError = strings.TrimSpace(message)
	out.UpdatedAt = now
	if out.ConsecutiveFailures >= def.Policy.MaxConsecutiveFailures {
		out.Status = RunPaused
		out.PauseReason = PauseReasonFailureLimit
	}
	return out, nil
}

func Resume(input Definition, run Run, now time.Time) (Run, error) {
	def := Normalize(input)
	if err := validateRunDefinition(def, run); err != nil {
		return Run{}, err
	}
	if run.Status != RunPaused {
		return Run{}, fmt.Errorf("%w: resume from %s", ErrInvalidRunState, run.Status)
	}
	out := cloneRun(run)
	out.Status = RunRunning
	out.ConsecutiveFailures = 0
	out.PauseReason = ""
	out.LastError = ""
	out.UpdatedAt = now
	return out, nil
}

// Pause stops automatic execution without consuming a transition. It is used
// for explicit interruption and infrastructure conditions such as a missing
// runtime. Resume reruns CurrentStepID.
func Pause(input Definition, run Run, reason, message string, now time.Time) (Run, error) {
	def := Normalize(input)
	if err := validateRunDefinition(def, run); err != nil {
		return Run{}, err
	}
	if run.Status != RunRunning {
		return Run{}, fmt.Errorf("%w: pause from %s", ErrInvalidRunState, run.Status)
	}
	out := cloneRun(run)
	out.Status = RunPaused
	out.PauseReason = strings.TrimSpace(reason)
	out.LastError = strings.TrimSpace(message)
	out.UpdatedAt = now
	return out, nil
}

// Cancel permanently stops a run that is not actively executing. Running
// processes must first be interrupted and persisted as paused by the
// orchestrator, which keeps cancellation free of process-lifecycle races.
func Cancel(input Definition, run Run, now time.Time) (Run, error) {
	def := Normalize(input)
	if err := validateRunDefinition(def, run); err != nil {
		return Run{}, err
	}
	if run.Status != RunReady && run.Status != RunPaused {
		return Run{}, fmt.Errorf("%w: cancel from %s", ErrInvalidRunState, run.Status)
	}
	out := cloneRun(run)
	out.Status = RunCancelled
	out.PauseReason = ""
	out.UpdatedAt = now
	return out, nil
}

func validateRunDefinition(def Definition, run Run) error {
	if err := Validate(def); err != nil {
		return err
	}
	if run.WorkflowID != def.ID {
		return fmt.Errorf("%w: run=%s definition=%s", ErrRunDefinitionMismatch, run.WorkflowID, def.ID)
	}
	if _, ok := findStep(def, run.CurrentStepID); !ok {
		return fmt.Errorf("%w: %s", ErrUnknownStep, run.CurrentStepID)
	}
	return nil
}

func findStep(def Definition, id string) (Step, bool) {
	for _, step := range def.Steps {
		if step.ID == id {
			return step, true
		}
	}
	return Step{}, false
}

func cloneRun(run Run) Run {
	out := run
	if run.Sessions != nil {
		out.Sessions = make(map[string]string, len(run.Sessions))
		for stepID, sessionID := range run.Sessions {
			out.Sessions[stepID] = sessionID
		}
	}
	out.History = append([]TransitionRecord(nil), run.History...)
	if run.Nodes != nil {
		out.Nodes = make(map[string]NodeState, len(run.Nodes))
		for stepID, state := range run.Nodes {
			out.Nodes[stepID] = state
		}
	}
	return out
}
