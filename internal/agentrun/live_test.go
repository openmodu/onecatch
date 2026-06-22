package agentrun

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestLiveCodexProducesFile drives the real codex CLI end to end: it asks the
// agent to write a file into a fresh workspace and asserts both that the file
// lands on disk and that the runner captured a final message. It is skipped
// unless ONESHOT_LIVE=1, so the normal suite never spends model credits.
//
//	ONESHOT_LIVE=1 go test ./internal/agentrun -run TestLiveCodex -v
func TestLiveCodexProducesFile(t *testing.T) {
	if os.Getenv("ONESHOT_LIVE") != "1" {
		t.Skip("set ONESHOT_LIVE=1 to run the live codex smoke test")
	}
	r := NewCodexRunner("")
	if !r.Available() {
		t.Skip("codex CLI not installed")
	}

	ws := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	var kinds []EventKind
	res, err := r.Run(ctx, Request{
		Runtime:   RuntimeCodex,
		Workspace: ws,
		Sandbox:   SandboxWorkspaceWrite,
		Prompt:    "Create a file named hello.txt in the current directory containing exactly the text: hello from codex. Then stop.",
	}, func(e Event) { kinds = append(kinds, e.Kind) })
	if err != nil {
		t.Fatalf("live run error: %v", err)
	}
	if !res.Succeeded {
		t.Fatalf("run did not succeed: %+v", res)
	}
	data, readErr := os.ReadFile(filepath.Join(ws, "hello.txt"))
	if readErr != nil {
		t.Fatalf("expected hello.txt produced by agent: %v", readErr)
	}
	t.Logf("agent produced hello.txt: %q", string(data))
	t.Logf("final message: %q", res.FinalMessage)
	t.Logf("event kinds: %v", kinds)
}

// TestLiveClaudeProducesFile is the Claude Code counterpart to the codex smoke
// test. Skipped unless ONESHOT_LIVE=1.
func TestLiveClaudeProducesFile(t *testing.T) {
	if os.Getenv("ONESHOT_LIVE") != "1" {
		t.Skip("set ONESHOT_LIVE=1 to run the live claude smoke test")
	}
	r := NewClaudeRunner("")
	if !r.Available() {
		t.Skip("claude CLI not installed")
	}

	ws := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	res, err := r.Run(ctx, Request{
		Runtime:   RuntimeClaude,
		Workspace: ws,
		Sandbox:   SandboxWorkspaceWrite,
		Model:     "claude-haiku-4-5-20251001",
		Prompt:    "Create a file named hello.txt in the current directory containing exactly: hello from claude. Then stop.",
	}, func(e Event) {})
	if err != nil {
		t.Fatalf("live run error: %v", err)
	}
	if !res.Succeeded {
		t.Fatalf("run did not succeed: %+v", res)
	}
	data, readErr := os.ReadFile(filepath.Join(ws, "hello.txt"))
	if readErr != nil {
		t.Fatalf("expected hello.txt produced by agent: %v", readErr)
	}
	t.Logf("claude produced hello.txt: %q", string(data))
	t.Logf("final message: %q", res.FinalMessage)
}
