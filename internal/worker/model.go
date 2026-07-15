// Package worker contains the trusted-network remote Agent worker protocol and
// the coordinator-side worker registry.
package worker

import (
	"time"

	"github.com/openmodu/oneshot/internal/agentrun"
)

type Config struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	BaseURL   string    `json:"baseUrl"`
	Token     string    `json:"token"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Input struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	BaseURL string `json:"baseUrl"`
	Token   string `json:"token,omitempty"`
	Enabled bool   `json:"enabled"`
}

type Info struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	BaseURL   string    `json:"baseUrl"`
	Enabled   bool      `json:"enabled"`
	HasToken  bool      `json:"hasToken"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Health struct {
	WorkerID string          `json:"workerId"`
	Name     string          `json:"name"`
	Runtimes map[string]bool `json:"runtimes"`
}

type ExecuteRequest struct {
	// RunID correlates this execution with an interrupt call. The coordinator
	// generates it; the worker keys its in-flight run registry on it so
	// POST /v1/runs/{runID}/interrupt can find and stop the right run.
	RunID                 string           `json:"runId"`
	WorkspaceID           string           `json:"workspaceId"`
	Runtime               agentrun.Runtime `json:"runtime"`
	Model                 string           `json:"model,omitempty"`
	Sandbox               agentrun.Sandbox `json:"sandbox"`
	Prompt                string           `json:"prompt"`
	ResumeSessionID       string           `json:"resumeSessionId,omitempty"`
	InterruptGraceSeconds int              `json:"interruptGraceSeconds,omitempty"`
}

// Frame is one line of the NDJSON execute stream. Exactly one field is set: a
// run emits many Event frames as work happens, then a single terminal frame —
// Result on completion (or graceful interrupt) or Error on failure. Streaming
// the events as they occur, rather than buffering them into one response, is
// what lets the coordinator's UI show a remote run live, the same as a local one.
type Frame struct {
	Event  *agentrun.Event  `json:"event,omitempty"`
	Result *agentrun.Result `json:"result,omitempty"`
	Error  *RemoteError     `json:"error,omitempty"`
}

type RemoteError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e RemoteError) Error() string { return e.Code + ": " + e.Message }
