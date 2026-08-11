package terminal

import "time"

const (
	OutputEvent = "oneshot:terminal-output"
	ExitEvent   = "oneshot:terminal-exit"
)

type CreateInput struct {
	Workspace string   `json:"workspace"`
	Shell     string   `json:"shell,omitempty"`
	Arguments []string `json:"arguments,omitempty"`
	Rows      uint16   `json:"rows,omitempty"`
	Cols      uint16   `json:"cols,omitempty"`
}

type Session struct {
	ID        string    `json:"id"`
	Workspace string    `json:"workspace"`
	Shell     string    `json:"shell"`
	PID       int       `json:"pid"`
	Rows      uint16    `json:"rows"`
	Cols      uint16    `json:"cols"`
	StartedAt time.Time `json:"startedAt"`
}

// OutputFrame transports raw PTY bytes as base64 so ANSI escape sequences and
// split UTF-8 code points survive the JSON bridge unchanged.
type OutputFrame struct {
	SessionID string `json:"sessionId"`
	Data      string `json:"data"`
}

type ExitFrame struct {
	SessionID string `json:"sessionId"`
	ExitCode  int    `json:"exitCode"`
	Error     string `json:"error,omitempty"`
}
