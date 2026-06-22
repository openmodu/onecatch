package agentrun

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"time"
)

// codexBinaryDefault is the CLI invoked when no override is configured.
const codexBinaryDefault = "codex"

// CodexRunner drives the Codex CLI through its non-interactive `codex exec`
// mode with `--json`, parsing the JSONL event stream documented in event.go.
type CodexRunner struct {
	// binary is the codex executable; overridable for tests.
	binary string
	now    nowFunc
}

// NewCodexRunner builds a runner driving the given codex binary. An empty
// binary falls back to "codex" resolved from PATH.
func NewCodexRunner(binary string) *CodexRunner {
	if binary == "" {
		binary = codexBinaryDefault
	}
	return &CodexRunner{binary: binary, now: time.Now}
}

func (r *CodexRunner) Runtime() Runtime { return RuntimeCodex }

func (r *CodexRunner) Available() bool {
	_, err := exec.LookPath(r.binary)
	return err == nil
}

func (r *CodexRunner) Run(ctx context.Context, req Request, sink Sink) (Result, error) {
	args := []string{
		"exec", "--json",
		// Workspaces are bare task directories, not git repos.
		"--skip-git-repo-check",
		"--sandbox", string(codexSandbox(req.Sandbox)),
		"-C", req.Workspace,
	}
	if req.Model != "" {
		args = append(args, "-m", req.Model)
	}
	args = append(args, req.Prompt)

	cmd := exec.CommandContext(ctx, r.binary, args...)
	cmd.Dir = req.Workspace
	// Closing stdin avoids codex blocking on "reading additional input".
	cmd.Stdin = nil
	return streamProcess(ctx, cmd, &codexParser{}, r.now, sink)
}

// codexSandbox maps the engine's sandbox vocabulary onto codex's flag values.
func codexSandbox(s Sandbox) string {
	switch s {
	case SandboxReadOnly:
		return "read-only"
	case SandboxFull:
		return "danger-full-access"
	default:
		return "workspace-write"
	}
}

// codexParser accumulates terminal state from the codex JSONL stream.
type codexParser struct {
	final     string
	sessionID string
	usage     Usage
	completed bool
}

type codexEnvelope struct {
	Type     string          `json:"type"`
	ThreadID string          `json:"thread_id"`
	Item     codexItem       `json:"item"`
	Usage    codexUsage      `json:"usage"`
	Error    json.RawMessage `json:"error"`
}

type codexItem struct {
	Type    string `json:"type"`
	Text    string `json:"text"`
	Command string `json:"command"`
	Path    string `json:"path"`
}

type codexUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

func (p *codexParser) parse(line string, at time.Time, sink Sink) {
	var env codexEnvelope
	if err := json.Unmarshal([]byte(line), &env); err != nil {
		// Codex occasionally prints non-JSON banner lines; preserve them as
		// raw reasoning rather than failing the run.
		sink(Event{Kind: KindReasoning, Text: strings.TrimSpace(line), Raw: line, At: at})
		return
	}

	switch env.Type {
	case "thread.started":
		p.sessionID = env.ThreadID
		sink(Event{Kind: KindStarted, Text: env.ThreadID, Raw: line, At: at})
	case "turn.started":
		// Turn boundaries carry no user-facing payload.
	case "item.completed", "item.updated":
		p.handleItem(env.Item, line, at, sink)
	case "turn.completed":
		p.usage = Usage{InputTokens: env.Usage.InputTokens, OutputTokens: env.Usage.OutputTokens}
		p.completed = true
		sink(Event{Kind: KindUsage, Raw: line, At: at})
	case "error", "turn.failed", "thread.error":
		p.completed = false
		sink(Event{Kind: KindError, Text: codexErrorText(env, line), Raw: line, At: at})
	default:
		// Unknown event types are retained raw so nothing is silently lost.
		sink(Event{Kind: KindReasoning, Raw: line, At: at})
	}
}

func (p *codexParser) handleItem(item codexItem, line string, at time.Time, sink Sink) {
	switch item.Type {
	case "agent_message", "assistant_message":
		p.final = item.Text
		sink(Event{Kind: KindMessage, Text: item.Text, Raw: line, At: at})
	case "reasoning":
		sink(Event{Kind: KindReasoning, Text: item.Text, Raw: line, At: at})
	case "command_execution", "local_shell_call":
		sink(Event{Kind: KindToolUse, Text: item.Command, Raw: line, At: at})
	case "file_change", "patch_apply":
		sink(Event{Kind: KindFileChange, Text: item.Path, Raw: line, At: at})
	default:
		sink(Event{Kind: KindToolResult, Text: item.Text, Raw: line, At: at})
	}
}

func codexErrorText(env codexEnvelope, line string) string {
	if len(env.Error) > 0 {
		return strings.Trim(string(env.Error), `"`)
	}
	return line
}

func (p *codexParser) result() Result {
	return Result{
		FinalMessage: p.final,
		Usage:        p.usage,
		SessionID:    p.sessionID,
		Succeeded:    p.completed,
	}
}
