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
	ErrUnknownStep           = errors.New("workflow step does not exist")
)

const (
	PauseReasonAskHuman     = "ask_human"
	PauseReasonReworkLimit  = "rework_limit_reached"
	PauseReasonFailureLimit = "max_consecutive_failures_reached"
)

// ErrInvalidOutcome reports a verdict the step is not allowed to return.
type ErrInvalidOutcome struct {
	StepID string
	Reason string
}

func (e ErrInvalidOutcome) Error() string {
	return fmt.Sprintf("workflow step %q returned an invalid outcome: %s", e.StepID, e.Reason)
}

// NewRun creates a ready run for a workflow snapshot with every step pending.
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
		CurrentStepID: EntrySteps(def)[0],
		Revision:      1,
		Sessions:      make(map[string]string),
		Nodes:         make(map[string]NodeState, len(def.Steps)),
		StartedAt:     now,
		UpdatedAt:     now,
	}
	for _, step := range def.Steps {
		run.Nodes[step.ID] = NodeState{StepID: step.ID, Status: NodePending}
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
	ensureNodes(def, &out)
	out.Status = RunRunning
	out.UpdatedAt = now
	return out, nil
}

// ReadySteps returns pending steps whose dependencies have all passed, in
// definition order.
func ReadySteps(def Definition, run Run) []Step {
	var ready []Step
	for _, step := range def.Steps {
		if run.Nodes[step.ID].Status != NodePending {
			continue
		}
		ok := true
		for _, dependency := range Upstream(def, step.ID) {
			if run.Nodes[dependency].Status != NodeCompleted {
				ok = false
				break
			}
		}
		if ok {
			ready = append(ready, step)
		}
	}
	return ready
}

// AllCompleted reports whether every step has passed.
func AllCompleted(def Definition, run Run) bool {
	for _, step := range def.Steps {
		if run.Nodes[step.ID].Status != NodeCompleted {
			return false
		}
	}
	return len(def.Steps) > 0
}

// ApplyOutcome records a finished step's verdict. pass completes the step;
// rework sends Feedback to the target's inbox and reopens the path between
// them; ask_human pauses the run with the step's question. A rework edge that
// has used up its rounds pauses the run instead of looping again.
func ApplyOutcome(input Definition, run Run, stepID string, outcome Outcome, now time.Time) (Run, error) {
	def := Normalize(input)
	if _, ok := FindStep(def, stepID); !ok {
		return Run{}, fmt.Errorf("%w: %s", ErrUnknownStep, stepID)
	}
	if run.Status != RunRunning {
		return Run{}, fmt.Errorf("%w: apply outcome from %s", ErrInvalidRunState, run.Status)
	}
	out := cloneRun(run)
	ensureNodes(def, &out)
	node := out.Nodes[stepID]
	node.Verdict = outcome.Verdict
	node.Summary = strings.TrimSpace(outcome.Summary)
	node.Error = ""
	node.Failures = 0
	node.Inbox = nil
	node.FinishedAt = now
	out.UpdatedAt = now

	switch outcome.Verdict {
	case VerdictPass:
		node.Status = NodeCompleted
		out.Nodes[stepID] = node
	case VerdictAskHuman:
		node.Status = NodePaused
		out.Nodes[stepID] = node
		out.Status = RunPaused
		out.PauseReason = PauseReasonAskHuman
		out.LastError = node.Summary
	case VerdictRework:
		edge, err := reworkEdge(def, stepID, outcome.Target)
		if err != nil {
			return Run{}, err
		}
		limit := edge.MaxRounds
		if limit == 0 {
			limit = def.Policy.MaxReworkRounds
		}
		key := EdgeKey(edge.From, edge.To)
		if out.ReworkRounds[key] >= limit {
			node.Status = NodePaused
			out.Nodes[stepID] = node
			out.Status = RunPaused
			out.PauseReason = PauseReasonReworkLimit
			out.LastError = fmt.Sprintf("%s sent work back to %s %d times; latest feedback: %s", edge.From, edge.To, limit, feedbackText(outcome))
			return out, nil
		}
		if out.ReworkRounds == nil {
			out.ReworkRounds = map[string]int{}
		}
		out.ReworkRounds[key]++
		out.Nodes[stepID] = node
		for pathStep := range ReworkPath(def, edge.From, edge.To) {
			reset := out.Nodes[pathStep]
			reset.Status = NodePending
			out.Nodes[pathStep] = reset
		}
		deliver(&out, edge.To, Message{From: stepID, Text: feedbackText(outcome), At: now})
	default:
		return Run{}, ErrInvalidOutcome{StepID: stepID, Reason: fmt.Sprintf("unknown verdict %q", outcome.Verdict)}
	}
	return out, nil
}

// CheckOutcome reports whether stepID may return outcome under def.
func CheckOutcome(input Definition, stepID string, outcome Outcome) error {
	def := Normalize(input)
	switch outcome.Verdict {
	case VerdictPass, VerdictAskHuman:
		return nil
	case VerdictRework:
		_, err := reworkEdge(def, stepID, outcome.Target)
		return err
	default:
		return ErrInvalidOutcome{StepID: stepID, Reason: fmt.Sprintf("unknown verdict %q", outcome.Verdict)}
	}
}

func reworkEdge(def Definition, stepID, target string) (Edge, error) {
	edges := ReworkEdges(def, stepID)
	if len(edges) == 0 {
		return Edge{}, ErrInvalidOutcome{StepID: stepID, Reason: "the step has no rework edge"}
	}
	if target == "" {
		if len(edges) == 1 {
			return edges[0], nil
		}
		return Edge{}, ErrInvalidOutcome{StepID: stepID, Reason: "rework target is required when several are allowed"}
	}
	for _, edge := range edges {
		if edge.To == target {
			return edge, nil
		}
	}
	return Edge{}, ErrInvalidOutcome{StepID: stepID, Reason: fmt.Sprintf("%q is not a rework target", target)}
}

func feedbackText(outcome Outcome) string {
	if text := strings.TrimSpace(outcome.Feedback); text != "" {
		return text
	}
	return strings.TrimSpace(outcome.Summary)
}

// RecordStepFailure records a runtime or protocol failure. Below the policy
// limit the step returns to pending for another attempt; at the limit the run
// pauses for a human.
func RecordStepFailure(input Definition, run Run, stepID, message string, now time.Time) Run {
	def := Normalize(input)
	out := cloneRun(run)
	ensureNodes(def, &out)
	node := out.Nodes[stepID]
	node.Failures++
	node.Error = strings.TrimSpace(message)
	node.FinishedAt = now
	node.Status = NodePending
	out.UpdatedAt = now
	if node.Failures >= def.Policy.MaxConsecutiveFailures {
		node.Status = NodePaused
		out.Status = RunPaused
		out.PauseReason = PauseReasonFailureLimit
		out.LastError = node.Error
	}
	out.Nodes[stepID] = node
	return out
}

// ReopenForInbox reopens every passed step that has been sent a message since
// it passed, together with the steps after it, so no message is left behind
// in a step that will not run again. It returns the reopened steps.
func ReopenForInbox(input Definition, run Run) (Run, []string) {
	def := Normalize(input)
	out := cloneRun(run)
	ensureNodes(def, &out)
	var reopened []string
	for _, step := range def.Steps {
		node := out.Nodes[step.ID]
		if node.Status != NodeCompleted || len(node.Inbox) == 0 {
			continue
		}
		for _, stepID := range append([]string{step.ID}, keys(Descendants(def, step.ID))...) {
			reset := out.Nodes[stepID]
			if reset.Status == NodeCompleted {
				reset.Status = NodePending
				out.Nodes[stepID] = reset
				reopened = append(reopened, stepID)
			}
		}
	}
	return out, reopened
}

func keys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for key := range set {
		out = append(out, key)
	}
	return out
}

// Deliver queues a message for stepID's next execution.
func Deliver(run Run, stepID string, message Message) Run {
	out := cloneRun(run)
	deliver(&out, stepID, message)
	return out
}

func deliver(run *Run, stepID string, message Message) {
	if strings.TrimSpace(message.Text) == "" {
		return
	}
	node := run.Nodes[stepID]
	node.StepID = stepID
	node.Inbox = append(append([]Message(nil), node.Inbox...), message)
	run.Nodes[stepID] = node
}

// Resume continues a paused or completed run. With fromStepID it reruns that
// step and everything after it, keeping upstream results and every step's
// session; otherwise interrupted or paused steps run again, and a completed
// run reopens its final steps for a follow-up. A non-empty message goes to
// fromStepID, or to every step that will start next.
func Resume(input Definition, run Run, fromStepID, message string, now time.Time) (Run, error) {
	def := Normalize(input)
	if err := validateRunDefinition(def, run); err != nil {
		return Run{}, err
	}
	if run.Status != RunPaused && run.Status != RunCompleted && run.Status != RunFailed {
		return Run{}, fmt.Errorf("%w: resume from %s", ErrInvalidRunState, run.Status)
	}
	out := cloneRun(run)
	ensureNodes(def, &out)
	reopen := func(stepID string) {
		node := out.Nodes[stepID]
		node.Status = NodePending
		node.Error = ""
		node.Failures = 0
		out.Nodes[stepID] = node
	}
	switch {
	case fromStepID != "":
		if _, ok := FindStep(def, fromStepID); !ok {
			return Run{}, fmt.Errorf("%w: %s", ErrUnknownStep, fromStepID)
		}
		reopen(fromStepID)
		for stepID := range Descendants(def, fromStepID) {
			reopen(stepID)
		}
	case run.Status == RunCompleted:
		for _, stepID := range SinkSteps(def) {
			reopen(stepID)
		}
	default:
		for stepID, node := range out.Nodes {
			if node.Status == NodeRunning || node.Status == NodePaused || node.Status == NodeFailed {
				reopen(stepID)
			}
		}
	}
	out.ReworkRounds = nil
	out.Status = RunRunning
	out.CompletedAt = time.Time{}
	out.PauseReason = ""
	out.LastError = ""
	out.UpdatedAt = now
	if text := strings.TrimSpace(message); text != "" {
		human := Message{From: MessageFromHuman, Text: text, At: now}
		if fromStepID != "" {
			deliver(&out, fromStepID, human)
		} else {
			for _, step := range ReadySteps(def, out) {
				deliver(&out, step.ID, human)
			}
		}
	}
	return out, nil
}

// Pause stops automatic execution. Step states are left as the scheduler
// recorded them; Resume decides what runs again.
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
	return nil
}

// ensureNodes gives every step a node state, so a run whose snapshot gained
// steps (or predates node tracking) schedules them as pending.
func ensureNodes(def Definition, run *Run) {
	if run.Nodes == nil {
		run.Nodes = make(map[string]NodeState, len(def.Steps))
	}
	for _, step := range def.Steps {
		if _, ok := run.Nodes[step.ID]; !ok {
			run.Nodes[step.ID] = NodeState{StepID: step.ID, Status: NodePending}
		}
	}
}

// CloneRun returns a copy that shares no maps or slices with run.
func CloneRun(run Run) Run { return cloneRun(run) }

func cloneRun(run Run) Run {
	out := run
	if run.Sessions != nil {
		out.Sessions = make(map[string]string, len(run.Sessions))
		for stepID, sessionID := range run.Sessions {
			out.Sessions[stepID] = sessionID
		}
	}
	if run.Nodes != nil {
		out.Nodes = make(map[string]NodeState, len(run.Nodes))
		for stepID, state := range run.Nodes {
			state.Inbox = append([]Message(nil), state.Inbox...)
			out.Nodes[stepID] = state
		}
	}
	if run.ReworkRounds != nil {
		out.ReworkRounds = make(map[string]int, len(run.ReworkRounds))
		for key, value := range run.ReworkRounds {
			out.ReworkRounds[key] = value
		}
	}
	return out
}
