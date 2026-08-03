// Package worker contains the trusted-network remote Agent worker protocol and
// the coordinator-side worker registry.
package worker

import (
	"time"

	domainworkspaces "github.com/openmodu/oneshot/internal/domain/workspaces"
	"github.com/openmodu/oneshot/internal/usecase/agentrun"
)

const MaxRunDuration = 24 * time.Hour

type Config struct {
	ID                      string    `json:"id"`
	Name                    string    `json:"name"`
	BaseURL                 string    `json:"baseUrl"`
	Token                   string    `json:"token"`
	CAFile                  string    `json:"caFile,omitempty"`
	ClientCertFile          string    `json:"clientCertFile,omitempty"`
	ClientKeyFile           string    `json:"clientKeyFile,omitempty"`
	ServerName              string    `json:"serverName,omitempty"`
	ServerCertificateSHA256 string    `json:"serverCertificateSha256,omitempty"`
	Enabled                 bool      `json:"enabled"`
	CreatedAt               time.Time `json:"createdAt"`
	UpdatedAt               time.Time `json:"updatedAt"`
}

type Input struct {
	ID                      string `json:"id"`
	Name                    string `json:"name"`
	BaseURL                 string `json:"baseUrl"`
	Token                   string `json:"token,omitempty"`
	CAFile                  string `json:"caFile,omitempty"`
	ClientCertFile          string `json:"clientCertFile,omitempty"`
	ClientKeyFile           string `json:"clientKeyFile,omitempty"`
	ServerName              string `json:"serverName,omitempty"`
	ServerCertificateSHA256 string `json:"serverCertificateSha256,omitempty"`
	Enabled                 bool   `json:"enabled"`
}

type UpdateInput struct {
	ID                      string `json:"id"`
	Name                    string `json:"name"`
	BaseURL                 string `json:"baseUrl"`
	CAFile                  string `json:"caFile,omitempty"`
	ClientCertFile          string `json:"clientCertFile,omitempty"`
	ClientKeyFile           string `json:"clientKeyFile,omitempty"`
	ServerName              string `json:"serverName,omitempty"`
	ServerCertificateSHA256 string `json:"serverCertificateSha256,omitempty"`
	Enabled                 bool   `json:"enabled"`
}

type Info struct {
	ID                      string    `json:"id"`
	Name                    string    `json:"name"`
	BaseURL                 string    `json:"baseUrl"`
	CAFile                  string    `json:"caFile,omitempty"`
	ClientCertFile          string    `json:"clientCertFile,omitempty"`
	ClientKeyFile           string    `json:"clientKeyFile,omitempty"`
	ServerName              string    `json:"serverName,omitempty"`
	ServerCertificateSHA256 string    `json:"serverCertificateSha256,omitempty"`
	Enabled                 bool      `json:"enabled"`
	HasToken                bool      `json:"hasToken"`
	CreatedAt               time.Time `json:"createdAt"`
	UpdatedAt               time.Time `json:"updatedAt"`
}

type Health struct {
	WorkerID        string          `json:"workerId"`
	Name            string          `json:"name"`
	ProtocolVersion int             `json:"protocolVersion"`
	Runtimes        map[string]bool `json:"runtimes"`
	Capabilities    map[string]bool `json:"capabilities"`
}

type PairRequest struct {
	Code string `json:"code"`
}

type PairResult struct {
	WorkerID                string          `json:"workerId"`
	Name                    string          `json:"name"`
	Token                   string          `json:"token"`
	ProtocolVersion         int             `json:"protocolVersion"`
	Runtimes                map[string]bool `json:"runtimes"`
	Capabilities            map[string]bool `json:"capabilities"`
	ServerCertificateSHA256 string          `json:"serverCertificateSha256,omitempty"`
}

type PermissionResponse struct {
	Decision string `json:"decision"`
}

type PatchAckRequest struct {
	Digest string `json:"digest"`
}

type WorkspaceMapping struct {
	ID        string    `json:"id"`
	Path      string    `json:"path"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type WorkspacePrepareRequest struct {
	RemoteURL string `json:"remoteUrl"`
	Revision  string `json:"revision"`
}

type WorkspacePrepareResult struct {
	Mapping WorkspaceMapping             `json:"mapping"`
	Git     domainworkspaces.GitSnapshot `json:"git"`
}

// WorkspacePatch is the complete Git worktree delta produced by a writable
// remote run. Data carries a binary-capable git patch using Encoding;
// BaseRevision and Digest keep application and cleanup fail-closed when either
// worktree moved.
type WorkspacePatch struct {
	BaseRevision   string   `json:"baseRevision"`
	Digest         string   `json:"digest"`
	Encoding       string   `json:"encoding,omitempty"`
	Data           string   `json:"data"`
	UntrackedPaths []string `json:"untrackedPaths,omitempty"`
}

type ExecuteRequest struct {
	// RunID correlates this execution with an interrupt call. The coordinator
	// generates it; the worker keys its in-flight run registry on it so
	// POST /v1/runs/{runID}/interrupt can find and stop the right run.
	RunID                 string           `json:"runId"`
	WorkspaceID           string           `json:"workspaceId"`
	Runtime               agentrun.Runtime `json:"runtime"`
	Model                 string           `json:"model,omitempty"`
	ReasoningEffort       string           `json:"reasoningEffort,omitempty"`
	ServiceTier           string           `json:"serviceTier,omitempty"`
	Provider              string           `json:"provider,omitempty"`
	Sandbox               agentrun.Sandbox `json:"sandbox"`
	Prompt                string           `json:"prompt"`
	ResumeSessionID       string           `json:"resumeSessionId,omitempty"`
	EnvironmentAllowlist  []string         `json:"environmentAllowlist"`
	TimeoutSeconds        int              `json:"timeoutSeconds,omitempty"`
	InterruptGraceSeconds int              `json:"interruptGraceSeconds,omitempty"`
	BaseRevision          string           `json:"baseRevision,omitempty"`
	SyncChanges           bool             `json:"syncChanges,omitempty"`
}

// Frame is one line of the NDJSON execute stream. Exactly one field is set: a
// run emits many Event frames as work happens, then a single terminal frame —
// Result on completion (or graceful interrupt) or Error on failure. Streaming
// the events as they occur, rather than buffering them into one response, is
// what lets the coordinator's UI show a remote run live, the same as a local one.
type Frame struct {
	Event  *agentrun.Event  `json:"event,omitempty"`
	Patch  *WorkspacePatch  `json:"patch,omitempty"`
	Result *agentrun.Result `json:"result,omitempty"`
	Error  *RemoteError     `json:"error,omitempty"`
}

type RemoteError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e RemoteError) Error() string { return e.Code + ": " + e.Message }
