// Package workflows contains the runtime-agnostic domain model and state
// machine for user-defined, signal-driven Agent workflows.
package workflows

import (
	"encoding/json"
	"time"
)

const (
	ModeSerial = "serial"
	ModeDAG    = "dag"

	TargetDone  = "$done"
	TargetPause = "$pause"
	TargetFail  = "$fail"

	DefaultMaxTransitions         = 20
	DefaultMaxConsecutiveFailures = 3
	DefaultStepTimeoutSeconds     = 30 * 60
)

// Definition is an editable workflow template. Each Run persists a complete
// snapshot so later template edits cannot alter active or historical runs.
type Definition struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Mode        string    `json:"mode,omitempty"`
	EntryStepID string    `json:"entryStepId"`
	Steps       []Step    `json:"steps"`
	Policy      Policy    `json:"policy"`
	Layout      Layout    `json:"layout,omitempty"`
	CreatedAt   time.Time `json:"createdAt,omitempty"`
	UpdatedAt   time.Time `json:"updatedAt,omitempty"`
}

// Step is one serial Agent turn in a workflow. Runtime is resolved by the
// local runtime registry; the domain layer deliberately does not depend on CLI
// adapters.
type Step struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Runtime     string            `json:"runtime"`
	Model       string            `json:"model,omitempty"`
	Sandbox     string            `json:"sandbox,omitempty"`
	WorkerID    string            `json:"workerId,omitempty"`
	DependsOn   []string          `json:"dependsOn,omitempty"`
	RolePrompt  string            `json:"rolePrompt"`
	Instruction string            `json:"instruction"`
	Transitions map[string]string `json:"transitions"`
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
	MaxTransitions         int `json:"maxTransitions"`
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
// are keyed by step ID so revisiting a role resumes only that role's context.
type Run struct {
	ID                     string                             `json:"id"`
	TaskID                 string                             `json:"taskId"`
	WorkflowID             string                             `json:"workflowId"`
	Revision               int64                              `json:"revision"`
	Status                 RunStatus                          `json:"status"`
	CurrentStepID          string                             `json:"currentStepId"`
	TransitionCount        int                                `json:"transitionCount"`
	ConsecutiveFailures    int                                `json:"consecutiveFailures"`
	Sessions               map[string]string                  `json:"sessions,omitempty"`
	History                []TransitionRecord                 `json:"history,omitempty"`
	Nodes                  map[string]NodeState               `json:"nodes,omitempty"`
	PauseReason            string                             `json:"pauseReason,omitempty"`
	LastError              string                             `json:"lastError,omitempty"`
	StartedAt              time.Time                          `json:"startedAt"`
	UpdatedAt              time.Time                          `json:"updatedAt"`
	CompletedAt            time.Time                          `json:"completedAt,omitempty"`
	MaxLocalDAGConcurrency int                                `json:"maxLocalDAGConcurrency,omitempty"`
	InterruptGraceSeconds  int                                `json:"interruptGraceSeconds,omitempty"`
	RuntimeSettings        map[string]ResolvedRuntimeSettings `json:"runtimeSettings,omitempty"`
}

type ResolvedRuntimeSettings struct {
	EnvironmentAllowlist []string `json:"environmentAllowlist,omitempty"`
	Provider             string   `json:"provider,omitempty"`
}

type NodeStatus string

const (
	NodePending   NodeStatus = "pending"
	NodeRunning   NodeStatus = "running"
	NodeCompleted NodeStatus = "completed"
	NodePaused    NodeStatus = "paused"
	NodeFailed    NodeStatus = "failed"
)

type NodeState struct {
	StepID     string     `json:"stepId"`
	Status     NodeStatus `json:"status"`
	Attempt    int        `json:"attempt"`
	Signal     string     `json:"signal,omitempty"`
	Content    string     `json:"content,omitempty"`
	Error      string     `json:"error,omitempty"`
	StartedAt  time.Time  `json:"startedAt,omitempty"`
	FinishedAt time.Time  `json:"finishedAt,omitempty"`
}

// Outcome is the only control message an Agent can return. It names a signal,
// not a target; the server-owned definition decides where that signal leads.
type Outcome struct {
	Signal  string `json:"signal"`
	Content string `json:"content"`
}

type TransitionRecord struct {
	FromStepID string    `json:"fromStepId"`
	Signal     string    `json:"signal"`
	Target     string    `json:"target"`
	Content    string    `json:"content"`
	At         time.Time `json:"at"`
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
	ID              string        `json:"id"`
	RunID           string        `json:"runId"`
	StepID          string        `json:"stepId"`
	Attempt         int           `json:"attempt"`
	Status          StepRunStatus `json:"status"`
	Signal          string        `json:"signal,omitempty"`
	Content         string        `json:"content,omitempty"`
	SessionIDBefore string        `json:"sessionIdBefore,omitempty"`
	SessionIDAfter  string        `json:"sessionIdAfter,omitempty"`
	StartedAt       time.Time     `json:"startedAt"`
	FinishedAt      time.Time     `json:"finishedAt,omitempty"`
	Error           string        `json:"error,omitempty"`
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

func isTerminalTarget(target string) bool {
	switch target {
	case TargetDone, TargetPause, TargetFail:
		return true
	default:
		return false
	}
}
