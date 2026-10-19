// Package workflows contains the runtime-agnostic domain model and state
// machine for user-defined Agent graphs.
//
// A workflow is a directed graph of steps. "next" edges are dependencies and
// must form a DAG: a step runs once every upstream step has passed. "rework"
// edges point back to an upstream step: when the source step returns a rework
// verdict, the target step receives the feedback in its own resumed session and
// only the steps on the path between them run again.
package workflows

import (
	"encoding/json"
	"time"
)

const (
	StepAgent   = "agent"
	StepCommand = "command"

	EdgeNext   = "next"
	EdgeRework = "rework"

	VerdictPass     = "pass"
	VerdictRework   = "rework"
	VerdictAskHuman = "ask_human"

	DefaultMaxReworkRounds        = 3
	DefaultMaxConsecutiveFailures = 3
	DefaultStepTimeoutSeconds     = 30 * 60
)

// Definition is an editable workflow template. Each Run persists a complete
// snapshot so later template edits cannot alter active or historical runs.
type Definition struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Steps       []Step    `json:"steps"`
	Edges       []Edge    `json:"edges,omitempty"`
	Policy      Policy    `json:"policy"`
	LoopEnabled bool      `json:"loopEnabled,omitempty"`
	Layout      Layout    `json:"layout,omitempty"`
	CreatedAt   time.Time `json:"createdAt,omitempty"`
	UpdatedAt   time.Time `json:"updatedAt,omitempty"`
}

// Step is one node of the graph. An agent step drives a coding-agent runtime
// and keeps one provider session for the whole run, so every revisit resumes
// that step's own context. A command step runs a shell command in the
// workspace; exit status zero passes, anything else is treated as rework.
type Step struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Kind            string `json:"kind,omitempty"`
	Runtime         string `json:"runtime,omitempty"`
	Model           string `json:"model,omitempty"`
	ReasoningEffort string `json:"reasoningEffort,omitempty"`
	Sandbox         string `json:"sandbox,omitempty"`
	WorkerID        string `json:"workerId,omitempty"`
	Prompt          string `json:"prompt,omitempty"`
	Command         string `json:"command,omitempty"`
}

// Edge connects two steps. MaxRounds bounds a rework edge; zero uses the
// workflow policy.
type Edge struct {
	From      string `json:"from"`
	To        string `json:"to"`
	Kind      string `json:"kind"`
	MaxRounds int    `json:"maxRounds,omitempty"`
}

type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type Layout struct {
	Nodes map[string]Point `json:"nodes,omitempty"`
}

// Policy bounds automatic execution. Zero values are filled by Normalize.
type Policy struct {
	MaxReworkRounds        int `json:"maxReworkRounds"`
	MaxConsecutiveFailures int `json:"maxConsecutiveFailures"`
	StepTimeoutSeconds     int `json:"stepTimeoutSeconds"`
}

type RunStatus string

const (
	RunReady     RunStatus = "ready"
	RunRunning   RunStatus = "running"
	RunPaused    RunStatus = "paused"
	RunCompleted RunStatus = "completed"
	RunFailed    RunStatus = "failed"
	RunCancelled RunStatus = "cancelled"
)

// Run is the durable state needed to resume orchestration. Runtime sessions
// are keyed by step ID so revisiting a step resumes only that step's context.
// CurrentStepID names the step that most recently started; it is a display and
// resume hint, not the scheduler's source of truth, which is Nodes.
type Run struct {
	ID                    string                             `json:"id"`
	TaskID                string                             `json:"taskId"`
	WorkflowID            string                             `json:"workflowId"`
	LoopEnabled           bool                               `json:"loopEnabled,omitempty"`
	LoopRound             int                                `json:"loopRound,omitempty"`
	Revision              int64                              `json:"revision"`
	Status                RunStatus                          `json:"status"`
	CurrentStepID         string                             `json:"currentStepId"`
	Sessions              map[string]string                  `json:"sessions,omitempty"`
	Nodes                 map[string]NodeState               `json:"nodes,omitempty"`
	ReworkRounds          map[string]int                     `json:"reworkRounds,omitempty"`
	PauseReason           string                             `json:"pauseReason,omitempty"`
	LastError             string                             `json:"lastError,omitempty"`
	StartedAt             time.Time                          `json:"startedAt"`
	UpdatedAt             time.Time                          `json:"updatedAt"`
	CompletedAt           time.Time                          `json:"completedAt,omitempty"`
	MaxConcurrency        int                                `json:"maxConcurrency,omitempty"`
	InterruptGraceSeconds int                                `json:"interruptGraceSeconds,omitempty"`
	RuntimeSettings       map[string]ResolvedRuntimeSettings `json:"runtimeSettings,omitempty"`
}

// RunListQuery is a repository-level query over durable Run snapshots. A
// non-nil TaskIDs slice scopes the result to those tasks; an empty slice then
// intentionally matches no runs.
type RunListQuery struct {
	TaskIDs      []string
	TitleTaskIDs []string
	Status       RunStatus
	Keyword      string
	Cursor       string
	Limit        int
}

type RunPage struct {
	Items      []Run
	NextCursor string
	Total      int
}

type ResolvedRuntimeSettings struct {
	EnvironmentAllowlist []string `json:"environmentAllowlist,omitempty"`
	Provider             string   `json:"provider,omitempty"`
	ReasoningEffort      string   `json:"reasoningEffort,omitempty"`
	ServiceTier          string   `json:"serviceTier,omitempty"`
	MaxContextWindow     bool     `json:"maxContextWindow,omitempty"`
}

type NodeStatus string

const (
	NodePending   NodeStatus = "pending"
	NodeRunning   NodeStatus = "running"
	NodeCompleted NodeStatus = "completed"
	NodePaused    NodeStatus = "paused"
	NodeFailed    NodeStatus = "failed"
)

// NodeState is one step's live position in a run. Inbox holds messages the
// step has not yet acted on: rework feedback from a downstream step, guidance
// a human addressed to it, or a note from a resume. It is delivered on the
// step's next execution and cleared once the step reaches a verdict.
type NodeState struct {
	StepID     string     `json:"stepId"`
	Status     NodeStatus `json:"status"`
	Attempt    int        `json:"attempt"`
	Failures   int        `json:"failures,omitempty"`
	Verdict    string     `json:"verdict,omitempty"`
	Summary    string     `json:"summary,omitempty"`
	Error      string     `json:"error,omitempty"`
	Inbox      []Message  `json:"inbox,omitempty"`
	StartedAt  time.Time  `json:"startedAt,omitempty"`
	FinishedAt time.Time  `json:"finishedAt,omitempty"`
}

const (
	MessageFromHuman = "human"
)

// Message is one piece of input waiting for a step. From is a step ID for
// rework feedback, or MessageFromHuman.
type Message struct {
	From string    `json:"from"`
	Text string    `json:"text"`
	At   time.Time `json:"at"`
}

// Outcome is the verdict a step reports when it finishes. Agents choose a
// verdict, never a destination: a rework Target must name one of the step's
// declared rework edges, and the definition decides everything else.
type Outcome struct {
	Verdict  string `json:"verdict"`
	Summary  string `json:"summary"`
	Feedback string `json:"feedback,omitempty"`
	Target   string `json:"target,omitempty"`
}

type StepRunStatus string

const (
	StepRunRunning     StepRunStatus = "running"
	StepRunSucceeded   StepRunStatus = "succeeded"
	StepRunFailed      StepRunStatus = "failed"
	StepRunInterrupted StepRunStatus = "interrupted"
)

// StepRun is one attempt to execute one workflow step.
type StepRun struct {
	ID                       string        `json:"id"`
	RunID                    string        `json:"runId"`
	StepID                   string        `json:"stepId"`
	Attempt                  int           `json:"attempt"`
	LoopRound                int           `json:"loopRound,omitempty"`
	Runtime                  string        `json:"runtime,omitempty"`
	Model                    string        `json:"model,omitempty"`
	Sandbox                  string        `json:"sandbox,omitempty"`
	Status                   StepRunStatus `json:"status"`
	Verdict                  string        `json:"verdict,omitempty"`
	Summary                  string        `json:"summary,omitempty"`
	SessionIDBefore          string        `json:"sessionIdBefore,omitempty"`
	SessionIDAfter           string        `json:"sessionIdAfter,omitempty"`
	StartedAt                time.Time     `json:"startedAt"`
	FinishedAt               time.Time     `json:"finishedAt,omitempty"`
	InputTokens              int           `json:"inputTokens,omitempty"`
	CachedInputTokens        int           `json:"cachedInputTokens,omitempty"`
	CacheCreationInputTokens int           `json:"cacheCreationInputTokens,omitempty"`
	OutputTokens             int           `json:"outputTokens,omitempty"`
	ReasoningOutputTokens    int           `json:"reasoningOutputTokens,omitempty"`
	// ContextWindow and ContextTokens describe how full the model's context
	// was, which the token counters above cannot express: those accumulate
	// across every model call in the step, while the window only held the latest
	// active context. ContextTokens may fall when the harness compacts.
	ContextWindow int    `json:"contextWindow,omitempty"`
	ContextTokens int    `json:"contextTokens,omitempty"`
	DurationMS    int64  `json:"durationMs,omitempty"`
	Error         string `json:"error,omitempty"`
}

type InstructionStatus string

const (
	// InstructionReady is a direct send persisted before its stopped run resumes.
	// It belongs in the conversation immediately, never in the waiting queue.
	InstructionReady   InstructionStatus = "ready"
	InstructionPending InstructionStatus = "pending"
	InstructionApplied InstructionStatus = "applied"
	InstructionRemoved InstructionStatus = "removed"
)

// Instruction is durable human guidance queued while a run is active. It lives
// in a file separate from run.json so enqueueing cannot race the Run revision
// used by the workflow state machine. A non-empty StepID addresses one step;
// otherwise the guidance goes to whichever steps start next.
type Instruction struct {
	ID          string            `json:"id"`
	StepID      string            `json:"stepId,omitempty"`
	Content     string            `json:"content"`
	Attachments []string          `json:"attachments,omitempty"`
	Status      InstructionStatus `json:"status"`
	Priority    bool              `json:"priority,omitempty"`
	FollowUp    bool              `json:"followUp,omitempty"`
	CreatedAt   time.Time         `json:"createdAt"`
	AppliedAt   time.Time         `json:"appliedAt,omitempty"`
}

// WorkflowEvent is a durable, high-level state machine event. Runtime stream
// events are kept separately in per-step JSONL files.
type WorkflowEvent struct {
	RunID   string          `json:"runId"`
	Seq     int64           `json:"seq"`
	Type    string          `json:"type"`
	StepID  string          `json:"stepId,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
	At      time.Time       `json:"at"`
}

// RuntimeEvent wraps one normalized CLI event in its append-only stream.
type RuntimeEvent struct {
	Seq     int64           `json:"seq"`
	At      time.Time       `json:"at"`
	Payload json.RawMessage `json:"payload"`
}
