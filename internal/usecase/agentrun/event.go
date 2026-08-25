// Package agentrun runs local coding-agent harnesses for long-horizon tasks,
// normalizing their heterogeneous SDK and process event streams
// streaming output into a single event model the rest of the product consumes.
//
// Each supported runtime speaks a different wire format on stdout:
//
//   - Codex app-server emits JSON-RPC notifications including agent-message and
//     command-output deltas.
//   - Claude Code (`claude -p --output-format stream-json`) emits JSONL:
//     system{init}, assistant{message.content[]}, user{tool_result},
//     result{success|error}.
//   - Modu Code can use its in-process Go SDK or the legacy print-mode NDJSON
//     adapter (`modu_code -p ... -json`).
//   - Pi (`pi -p --mode json`) emits JSONL: a session header followed by the
//     agent's own event union (agent_start, message_update, tool_execution_*).
//   - Grok Build speaks the Agent Client Protocol over stdio
//     (`grok agent stdio`), so it is driven by the shared ACP client rather
//     than by a bespoke parser.
//   - DeepSeek Harness has no machine-readable stdout, so its adapter pins the
//     harness's own JSONL session log to a per-run directory and reads that.
//
// Adapters translate all of them into the [Event] stream below so callers never
// have to branch on the underlying runtime.
package agentrun

import (
	"time"

	domainharnesses "github.com/openmodu/onecatch/internal/domain/harnesses"
)

// Runtime identifies a local agent CLI the engine knows how to drive.
type Runtime string

const (
	// RuntimeCodex drives the OpenAI Codex CLI via app-server.
	RuntimeCodex Runtime = "codex"
	// RuntimeClaude drives Anthropic's Claude Code via `claude -p`.
	RuntimeClaude Runtime = "claude"
	// RuntimeModu drives Modu Code via its native SDK or print mode.
	RuntimeModu Runtime = "modu"
	// RuntimePi drives the Pi coding agent via `pi -p --mode json`.
	RuntimePi Runtime = "pi"
	// RuntimeGrok drives xAI's Grok Build via its ACP server (`grok agent stdio`).
	RuntimeGrok Runtime = "grok"
	// RuntimeDsh drives DeepSeek Harness via its one-shot headless profile.
	RuntimeDsh Runtime = "dsh"
)

// Valid reports whether r names a harness in the catalog. The constants above
// are spelling aids for call sites; the catalog decides what exists, so a
// harness cannot be half-added by declaring a constant and nothing else.
func (r Runtime) Valid() bool {
	return domainharnesses.IsKnown(string(r))
}

// EventKind is the normalized category of a streamed run event. It is the
// common denominator across every supported runtime.
type EventKind string

const (
	// KindStarted marks the beginning of a run (session/thread established).
	KindStarted EventKind = "started"
	// KindReasoning carries the agent's private thinking, when the runtime
	// surfaces it. Useful for live progress but not part of the deliverable.
	KindReasoning EventKind = "reasoning"
	// KindMessage carries assistant prose addressed to the user.
	KindMessage EventKind = "message"
	// KindToolUse marks the agent invoking a tool (shell command, file edit).
	KindToolUse EventKind = "tool_use"
	// KindToolResult carries the result of a prior tool invocation.
	KindToolResult EventKind = "tool_result"
	// KindFileChange marks the agent creating or modifying a file.
	KindFileChange EventKind = "file_change"
	// KindUsage carries token/cost accounting for the turn.
	KindUsage EventKind = "usage"
	// KindResult marks the terminal outcome of the run.
	KindResult EventKind = "result"
	// KindError carries a runtime or parse error encountered mid-stream.
	KindError EventKind = "error"
	// KindPermissionRequest asks the host application to approve or deny a
	// tool call while the runtime remains blocked on that decision.
	KindPermissionRequest EventKind = "permission_request"
	// KindPermissionResolved records the decision returned to the runtime.
	KindPermissionResolved EventKind = "permission_resolved"
)

// PermissionUpdate is a provider-authored rule update. Claude supplies these
// suggestions for an "always allow" decision; keeping the structure opaque
// lets newer CLI versions add rule variants without requiring an app release.
type PermissionUpdate map[string]any

// PermissionRequest is the runtime-neutral payload shown by the desktop
// approval card. ID identifies the blocking control request, while ToolUseID
// identifies the eventual tool call in the provider transcript.
type PermissionRequest struct {
	ID                      string             `json:"id"`
	ToolUseID               string             `json:"toolUseId,omitempty"`
	ToolName                string             `json:"toolName"`
	Input                   map[string]any     `json:"input,omitempty"`
	Suggestions             []PermissionUpdate `json:"suggestions,omitempty"`
	Title                   string             `json:"title,omitempty"`
	DisplayName             string             `json:"displayName,omitempty"`
	Description             string             `json:"description,omitempty"`
	DecisionReason          string             `json:"decisionReason,omitempty"`
	SuppressAlwaysAllow     bool               `json:"suppressAlwaysAllow,omitempty"`
	RequiresUserInteraction bool               `json:"requiresUserInteraction,omitempty"`
}

// PermissionDecision is returned by the host after the user responds.
type PermissionDecision struct {
	Behavior               string             `json:"behavior"`
	Message                string             `json:"message,omitempty"`
	UpdatedPermissions     []PermissionUpdate `json:"updatedPermissions,omitempty"`
	DecisionClassification string             `json:"decisionClassification,omitempty"`
	ToolUseID              string             `json:"toolUseID,omitempty"`
}

// StreamPhase describes how an event contributes to one logical, growing UI
// entry. Empty means the event is atomic and remains backwards compatible with
// events produced before streaming was introduced.
type StreamPhase string

const (
	StreamStart    StreamPhase = "start"
	StreamDelta    StreamPhase = "delta"
	StreamSnapshot StreamPhase = "snapshot"
	StreamEnd      StreamPhase = "end"
)

// Event is a single normalized step emitted while an agent runs. Raw preserves
// the original JSON line so nothing is lost, while Kind/Text give callers a
// runtime-agnostic view suitable for display and persistence.
type Event struct {
	Kind EventKind `json:"kind"`
	// Usage is the latest cumulative token accounting for this step. Runtimes
	// that report usage before completion attach it to KindUsage so the desktop
	// can update its metrics while the agent is still running.
	Usage *Usage `json:"usage,omitempty"`
	// StreamID is stable for all chunks belonging to one logical message,
	// reasoning block, or tool output. Empty identifies an atomic event.
	StreamID string `json:"streamId,omitempty"`
	// Phase controls whether Text starts, appends to, replaces, or completes a
	// logical stream. Revision is assigned by the orchestration collector after
	// it batches provider token deltas.
	Phase    StreamPhase `json:"phase,omitempty"`
	Revision uint64      `json:"revision,omitempty"`
	// Text is the human-meaningful payload: message prose, command, file path,
	// or error description, depending on Kind.
	Text string `json:"text,omitempty"`
	// Raw is the original JSONL line from the runtime, retained for debugging
	// and forward compatibility with fields we do not yet model.
	Raw string `json:"raw,omitempty"`
	// Failed reports that this individual event is an error: a tool that
	// returned an error, not a run that ended badly. A step can fail while most
	// of its tool calls succeeded, so callers must not infer one from the other.
	Failed bool `json:"failed,omitempty"`
	// Context is the context-window occupancy observed at this step, attached
	// to KindUsage beside Usage. It is a separate field precisely because it
	// is not a subset of Usage — see [ContextUsage].
	Context *ContextUsage `json:"context,omitempty"`
	// Permission is populated for permission_request and permission_resolved
	// events. PermissionDecision is "allow" or "deny" on the resolved event.
	Permission         *PermissionRequest `json:"permission,omitempty"`
	PermissionDecision string             `json:"permissionDecision,omitempty"`
	// At is when the engine observed the event.
	At time.Time `json:"at"`
}

// ContextUsage reports how full the model's context window is. It is a
// different quantity from [Usage] and the two must not be substituted for one
// another: Usage accumulates every model call in a step, so it only ever grows
// and routinely exceeds the window a tool-heavy step never came close to
// filling. Occupancy is the size of a single prompt, and it *falls* whenever
// the harness compacts. Charting the cumulative total against the window shows
// a step at 300% of a context it never overflowed.
type ContextUsage struct {
	// Window is the model's context window in tokens. Zero means the runtime
	// did not report one — render that as unknown rather than as a full bar.
	Window int `json:"window,omitempty"`
	// Tokens is the prompt size of the most recent model call: everything the
	// model read this turn, cached prefix included, because a cache hit still
	// occupies the window. May decrease after a compaction.
	Tokens int `json:"tokens,omitempty"`
}

// Known reports whether the runtime supplied enough to draw an occupancy
// figure. A window with no sample, or a sample with no window, is not a ratio.
func (c ContextUsage) Known() bool { return c.Window > 0 && c.Tokens > 0 }

// Usage captures cumulative token accounting for one workflow step. InputTokens
// includes cache reads and cache creation when a provider reports them
// separately; the detailed fields are subsets used to explain that total.
type Usage struct {
	InputTokens              int `json:"inputTokens"`
	CachedInputTokens        int `json:"cachedInputTokens,omitempty"`
	CacheCreationInputTokens int `json:"cacheCreationInputTokens,omitempty"`
	OutputTokens             int `json:"outputTokens"`
	ReasoningOutputTokens    int `json:"reasoningOutputTokens,omitempty"`
}

// Result is the terminal outcome of a completed run.
type Result struct {
	// FinalMessage is the agent's last assistant message — the natural-language
	// summary of what it did.
	FinalMessage string `json:"finalMessage"`
	// Usage is best-effort token accounting; zero when the runtime omits it.
	Usage Usage `json:"usage"`
	// Context is the final context-window occupancy; zero when unreported.
	Context ContextUsage `json:"context"`
	// SessionID is the runtime's own session/thread id, for resuming.
	SessionID string `json:"sessionId,omitempty"`
	// Succeeded is true when the runtime reported a clean terminal state.
	Succeeded bool `json:"succeeded"`
}
