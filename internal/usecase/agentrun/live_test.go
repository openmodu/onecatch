package agentrun

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestLiveCodexReadsAccountUsage verifies the quota-only app-server request
// against the installed CLI. It does not start a model turn or spend quota.
//
//	ONECATCH_LIVE_USAGE=1 go test ./internal/usecase/agentrun -run TestLiveCodexReadsAccountUsage -v
func TestLiveCodexReadsAccountUsage(t *testing.T) {
	if os.Getenv("ONECATCH_LIVE_USAGE") != "1" {
		t.Skip("set ONECATCH_LIVE_USAGE=1 to run the live Codex usage check")
	}
	runner := NewCodexRunner("")
	if !runner.Available() {
		t.Skip("codex CLI not installed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	usage, err := runner.ReadAccountUsage(ctx, t.TempDir(), os.Environ())
	if err != nil {
		t.Fatalf("read live Codex usage: %v", err)
	}
	if usage.Runtime != RuntimeCodex || len(usage.RateLimits) == 0 || len(usage.DailyUsage) == 0 {
		t.Fatalf("incomplete live Codex usage: %+v", usage)
	}
	t.Logf("Codex account reported %d rate-limit buckets and %d active days", len(usage.RateLimits), len(usage.DailyUsage))
}

// TestLiveLocalHarnessesReadAccountUsage checks the real session layouts
// without starting a model turn. Empty histories are valid; a successfully
// decoded device-scoped snapshot is the contract under test.
//
//	ONECATCH_LIVE_USAGE=1 go test ./internal/usecase/agentrun -run TestLiveLocalHarnessesReadAccountUsage -v
func TestLiveLocalHarnessesReadAccountUsage(t *testing.T) {
	if os.Getenv("ONECATCH_LIVE_USAGE") != "1" {
		t.Skip("set ONECATCH_LIVE_USAGE=1 to run local harness usage checks")
	}
	tests := []struct {
		runtime Runtime
		runner  Runner
	}{
		{RuntimeClaude, NewClaudeRunner("")},
		{RuntimePi, NewPiRunner("")},
		{RuntimeGrok, NewGrokRunner("")},
		{RuntimeModu, NewModuRunner("")},
	}
	for _, test := range tests {
		t.Run(string(test.runtime), func(t *testing.T) {
			if !test.runner.Available() {
				t.Skip("runtime is not installed")
			}
			reader, ok := test.runner.(AccountUsageReader)
			if !ok {
				t.Fatal("runner does not implement AccountUsageReader")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			usage, err := reader.ReadAccountUsage(ctx, "", os.Environ())
			if err != nil {
				t.Fatal(err)
			}
			if usage.Runtime != test.runtime || usage.Scope != AccountUsageScopeDevice {
				t.Fatalf("unexpected snapshot identity: %+v", usage)
			}
			var total int64
			for _, bucket := range usage.DailyUsage {
				total += bucket.Tokens
			}
			t.Logf("%s reported %d active days and %d local tokens", test.runtime, len(usage.DailyUsage), total)
		})
	}
}

// TestLiveCodexProducesFile drives the real codex CLI end to end: it asks the
// agent to write a file into a fresh workspace and asserts both that the file
// lands on disk and that the runner captured a final message. It is skipped
// unless ONECATCH_LIVE=1, so the normal suite never spends model credits.
//
//	ONECATCH_LIVE=1 go test ./internal/usecase/agentrun -run TestLiveCodex -v
func TestLiveCodexProducesFile(t *testing.T) {
	if os.Getenv("ONECATCH_LIVE") != "1" {
		t.Skip("set ONECATCH_LIVE=1 to run the live codex smoke test")
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
// test. Skipped unless ONECATCH_LIVE=1.
func TestLiveClaudeProducesFile(t *testing.T) {
	if os.Getenv("ONECATCH_LIVE") != "1" {
		t.Skip("set ONECATCH_LIVE=1 to run the live claude smoke test")
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
