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
	WorkspaceID     string           `json:"workspaceId"`
	Runtime         agentrun.Runtime `json:"runtime"`
	Model           string           `json:"model,omitempty"`
	Sandbox         agentrun.Sandbox `json:"sandbox"`
	Prompt          string           `json:"prompt"`
	ResumeSessionID string           `json:"resumeSessionId,omitempty"`
}

type ExecuteResponse struct {
	Result agentrun.Result  `json:"result"`
	Events []agentrun.Event `json:"events"`
}

type RemoteError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e RemoteError) Error() string { return e.Code + ": " + e.Message }
