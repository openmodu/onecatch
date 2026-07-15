// Package agentrun runs local CLI coding agents (Codex, Claude Code, Modu Code) as
// subprocesses for long-horizon tasks, normalizing their heterogeneous
// streaming output into a single event model the rest of the product consumes.
//
// Each supported runtime speaks a different wire format on stdout:
//
//   - Codex app-server emits JSON-RPC notifications including agent-message and
//     command-output deltas. Older CLIs fall back to `codex exec --json` JSONL.
//   - Claude Code (`claude -p --output-format stream-json`) emits JSONL:
//     system{init}, assistant{message.content[]}, user{tool_result},
//     result{success|error}.
//   - Modu Code (`modu_code -p ... -json`) emits NDJSON lifecycle, message,
//     tool execution, and session completion events.
//
// Adapters translate all three into the [Event] stream below so callers never have
// to branch on the underlying runtime.
package agentrun

import "time"

// Runtime identifies a local agent CLI the engine knows how to drive.
type Runtime string

const (
	// RuntimeCodex drives the OpenAI Codex CLI via app-server, with exec fallback.
	RuntimeCodex Runtime = "codex"
	// RuntimeClaude drives Anthropic's Claude Code via `claude -p`.
	RuntimeClaude Runtime = "claude"
	// RuntimeModu drives Modu Code via its non-interactive print mode.
	RuntimeModu Runtime = "modu"
)

// Valid reports whether r is a runtime the engine can drive.
func (r Runtime) Valid() bool {
	switch r {
	case RuntimeCodex, RuntimeClaude, RuntimeModu:
		return true
	default:
		return false
	}
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
)

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
	// At is when the engine observed the event.
	At time.Time `json:"at"`
}

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
	// SessionID is the runtime's own session/thread id, for resuming.
	SessionID string `json:"sessionId,omitempty"`
	// Succeeded is true when the runtime reported a clean terminal state.
	Succeeded bool `json:"succeeded"`
}
