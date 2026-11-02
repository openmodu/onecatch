package agentrun

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestCodexOccupiedThreadContinuation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stub uses a POSIX shell")
	}
	for _, scenario := range []string{"fork", "fork-error", "other-error", "new-thread"} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			requests := filepath.Join(dir, "requests")
			binary := filepath.Join(dir, "codex")
			script := `#!/bin/sh
while IFS= read -r line; do
  printf '%s\n' "$line" >> "$TEST_REQUESTS"
  case "$line" in
    *'"method":"initialize"'*) echo '{"id":1,"result":{}}' ;;
    *'"method":"skills/list"'*) echo '{"id":2,"result":{"data":[]}}' ;;
    *'"method":"thread/fork"'*)
      if [ "$TEST_SCENARIO" = fork-error ]; then
        echo '{"id":3,"error":{"code":-32600,"message":"thread original already has an active writer"}}'
      else
        echo '{"id":3,"result":{"thread":{"id":"branched"}}}'
      fi ;;
    *'"id":3'*)
      if [ "$TEST_SCENARIO" = other-error ]; then
        echo '{"id":3,"error":{"code":-32600,"message":"thread not found"}}'
      else
        echo '{"id":3,"error":{"code":-32600,"message":"thread original already has an active writer"}}'
      fi ;;
    *'"method":"turn/start"'*)
      echo '{"id":4,"result":{"turn":{"id":"turn"}}}'
      echo '{"method":"turn/completed","params":{"threadId":"branched","turn":{"id":"turn","status":"completed"}}}' ;;
  esac
done
`
			if err := os.WriteFile(binary, []byte(script), 0o755); err != nil {
				t.Fatal(err)
			}
			runner := NewCodexRunner(binary)
			t.Cleanup(func() { _ = runner.Close() })
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			req := Request{RunID: "run", Workspace: dir, ResumeSessionID: "original", Prompt: "continue", Model: "model", Sandbox: SandboxReadOnly,
				Environment: append(os.Environ(), "TEST_REQUESTS="+requests, "TEST_SCENARIO="+scenario)}
			if scenario == "new-thread" {
				req.ResumeSessionID = ""
			}
			result, err := runner.Run(ctx, req, nil)
			if scenario == "fork" {
				if err != nil || !result.Succeeded || result.SessionID != "branched" {
					t.Fatalf("continuation = %+v, %v", result, err)
				}
				req.ResumeSessionID = result.SessionID
				if _, err := runner.Run(ctx, req, nil); err != nil {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatal("expected error")
			}
			payload, err := os.ReadFile(requests)
			if err != nil {
				t.Fatal(err)
			}
			calls := string(payload)
			wantForks := 0
			if scenario == "fork" || scenario == "fork-error" {
				wantForks = 1
			}
			if got := strings.Count(calls, `"method":"thread/fork"`); got != wantForks {
				t.Fatalf("forks=%d, want %d: %s", got, wantForks, calls)
			}
			if scenario == "fork" {
				if strings.Count(calls, `"method":"turn/start"`) != 2 || strings.Count(calls, `"method":"thread/resume"`) != 1 {
					t.Fatalf("follow-up did not reuse branch: %s", calls)
				}
				for _, line := range strings.Split(calls, "\n") {
					if strings.Contains(line, `"method":"thread/fork"`) {
						for _, field := range []string{`"threadId":"original"`, `"excludeTurns":true`, `"sandbox":"read-only"`, `"approvalPolicy":"never"`, `"model":"model"`} {
							if !strings.Contains(line, field) {
								t.Fatalf("fork missing %s: %s", field, line)
							}
						}
					}
					if strings.Contains(line, `"method":"turn/start"`) && !strings.Contains(line, `"threadId":"branched"`) {
						t.Fatalf("turn used occupied thread: %s", line)
					}
				}
			} else if strings.Contains(calls, `"method":"turn/start"`) {
				t.Fatalf("started turn after failure: %s", calls)
			}
		})
	}
}
