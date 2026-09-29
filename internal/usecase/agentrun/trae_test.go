package agentrun

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/openmodu/onecatch/internal/usecase/agentrun/seam"
)

func TestTraeRunsAndResumesThroughAppServer(t *testing.T) {
	capture := filepath.Join(t.TempDir(), "requests.jsonl")
	t.Setenv("ONECATCH_TRAE_CAPTURE", capture)
	runner := NewTraeRunner(stubTraeAppServer(t))
	if runner.Runtime() != RuntimeTrae {
		t.Fatal("wrong runtime")
	}
	for _, resume := range []string{"", "thread-live"} {
		var events []Event
		result, err := runner.Run(context.Background(), Request{Workspace: t.TempDir(), Prompt: "hello", ResumeSessionID: resume, Model: "Doubao-Seed-2.1-Pro", ReasoningEffort: "high", Sandbox: SandboxReadOnly}, func(e Event) { events = append(events, e) })
		if err != nil {
			t.Fatal(err)
		}
		if !result.Succeeded || result.FinalMessage != "Hello" || result.SessionID != "thread-live" || result.Usage.InputTokens != 17 {
			t.Fatalf("unexpected result: %+v", result)
		}
		found := false
		for _, e := range events {
			if e.Kind == KindToolResult {
				found = true
			}
		}
		if !found {
			t.Fatal("missing tool result")
		}
	}
	data, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"method":"thread/start"`, `"method":"thread/resume"`, `"sandbox":"read-only"`, `"model":"Doubao-Seed-2.1-Pro"`, `"effort":"high"`} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("missing %s in requests", want)
		}
	}
}

func TestTraeModelCatalogUsesConfigurationNames(t *testing.T) {
	// Shape captured from traecli 0.207.1 model/list; configName differs from model.
	response := `{"id":2,"result":{"data":[{"id":"Doubao-Seed-2.1-Pro","model":"Seed-2.1-Pro-0915","configName":"Doubao-Seed-2.1-Pro","displayName":"Seed-2.1-Pro-0915","contextWindow":200000,"supportedReasoningEfforts":[],"defaultReasoningEffort":"high","isDefault":true},{"id":"gpt-5.2","model":"gpt-5.2","supportedReasoningEfforts":[{"reasoningEffort":"high"}]}],"nextCursor":null}}`
	t.Setenv("ONECATCH_TRAE_MODELS", response)
	runner := NewTraeRunner(stubTraeAppServer(t))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cfg, err := runner.InspectConfiguration(ctx, t.TempDir(), os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Model != "Doubao-Seed-2.1-Pro" || len(cfg.Models) != 2 || cfg.Models[0].Model != cfg.Model || cfg.Models[0].ContextWindow != 200000 || cfg.Models[1].Efforts[0] != "high" {
		t.Fatalf("catalog: %+v", cfg)
	}
}

func TestTraeRejectsUnsupportedOverrides(t *testing.T) {
	runner := NewTraeRunner("onecatch-absent-trae")
	for _, req := range []Request{{Sandbox: "invalid"}, {Remote: &seam.Target{Root: "/remote"}}, {ServiceTier: "fast"}, {MaxContextWindow: true}} {
		if _, err := runner.Run(context.Background(), req, nil); err == nil {
			t.Fatalf("accepted unsupported request: %+v", req)
		}
	}
}

func TestTraeRealAppServerCatalog(t *testing.T) {
	runner := NewTraeRunner("")
	if !runner.Available() {
		t.Skip("traecli not installed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cfg, err := runner.InspectConfiguration(ctx, t.TempDir(), os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Models) == 0 {
		t.Fatal("empty model catalog")
	}
}

func TestLiveTraeAppServer(t *testing.T) {
	if os.Getenv("ONECATCH_LIVE") != "1" {
		t.Skip("set ONECATCH_LIVE=1 to spend model quota")
	}
	runner := NewTraeRunner("")
	if !runner.Available() {
		t.Skip("traecli not installed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	workspace := t.TempDir()
	result, err := runner.Run(ctx, Request{Workspace: workspace, Prompt: "Reply with exactly OK. Do not use tools.", Model: os.Getenv("ONECATCH_TRAE_MODEL"), Sandbox: SandboxReadOnly}, func(event Event) { t.Logf("%s: %s", event.Kind, event.Text) })
	if err != nil {
		t.Fatal(err)
	}
	if !result.Succeeded || result.SessionID == "" || strings.TrimSpace(result.FinalMessage) == "" {
		t.Fatalf("result: %+v", result)
	}
	resumed, err := runner.Run(ctx, Request{Workspace: workspace, ResumeSessionID: result.SessionID, Prompt: "Repeat your previous answer exactly. Do not use tools.", Model: os.Getenv("ONECATCH_TRAE_MODEL"), Sandbox: SandboxReadOnly}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !resumed.Succeeded || resumed.SessionID != result.SessionID || strings.TrimSpace(resumed.FinalMessage) != "OK" {
		t.Fatalf("resumed result: %+v", resumed)
	}
}

// TRAE protocol fixture, independent from every other harness's stubs.
func stubTraeAppServer(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell fixture")
	}
	path := filepath.Join(t.TempDir(), "traecli")
	script := `#!/bin/sh
[ "$1" = "app-server" ] || exit 9
while IFS= read -r line; do
 [ -n "$ONECATCH_TRAE_CAPTURE" ] && printf '%s\n' "$line" >> "$ONECATCH_TRAE_CAPTURE"
 case "$line" in
  *'"method":"initialize"'*) printf '%s\n' '{"id":1,"result":{"userAgent":"TraeCode CLI/0.207.1"}}' ;;
  *'"method":"model/internalUsage/read"'*) printf '%s\n' "$ONECATCH_TRAE_USAGE" ;;
  *'"method":"model/list"'*) printf '%s\n' "$ONECATCH_TRAE_MODELS" ;;
  *'"method":"skills/list"'*) printf '%s\n' '{"id":2,"result":{"data":[]}}' ;;
  *'"method":"thread/start"'*|*'"method":"thread/resume"'*) printf '%s\n' '{"id":3,"result":{"thread":{"id":"thread-live"}}}' ;;
  *'"method":"turn/start"'*)
   printf '%s\n' '{"id":4,"result":{"turn":{"id":"turn-live","status":"inProgress"}}}'
   printf '%s\n' '{"method":"item/agentMessage/delta","params":{"threadId":"thread-live","turnId":"turn-live","itemId":"message-1","delta":"Hello"}}'
   printf '%s\n' '{"method":"item/completed","params":{"threadId":"thread-live","turnId":"turn-live","item":{"id":"message-1","type":"agentMessage","text":"Hello"}}}'
   printf '%s\n' '{"method":"item/completed","params":{"threadId":"thread-live","turnId":"turn-live","item":{"id":"command-1","type":"commandExecution","status":"completed","aggregatedOutput":"ok","exitCode":0}}}'
   printf '%s\n' '{"method":"thread/tokenUsage/updated","params":{"threadId":"thread-live","turnId":"turn-live","tokenUsage":{"modelContextWindow":200000,"last":{"inputTokens":17,"outputTokens":5,"totalTokens":22},"total":{"inputTokens":17,"outputTokens":5,"totalTokens":22}}}}'
   printf '%s\n' '{"method":"turn/completed","params":{"threadId":"thread-live","turn":{"id":"turn-live","status":"completed"}}}' ;;
 esac
done
`
	if err := os.WriteFile(path, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestTraeStructuredContentAndTurnIsolation(t *testing.T) {
	state := traeTurnState{result: Result{SessionID: "thread"}, streams: map[string]bool{}}
	frames := []string{
		`{"method":"item/started","params":{"threadId":"thread","turnId":"turn","item":{"id":"user","type":"userMessage","content":[{"type":"text","text":"hi"}]}}}`,
		`{"method":"item/completed","params":{"threadId":"other","turnId":"turn","item":{"id":"message","type":"agentMessage","text":"wrong"}}}`,
		`{"method":"turn/completed","params":{"threadId":"thread","turn":{"id":"old-turn","status":"completed"}}}`,
	}
	for _, raw := range frames {
		var frame traeFrame
		if err := json.Unmarshal([]byte(raw), &frame); err != nil {
			t.Fatal(err)
		}
		done, err := state.update(frame, "turn", func(Event) { t.Fatal("unrelated/user frame emitted an event") })
		if err != nil || done {
			t.Fatalf("update = %v, %v", done, err)
		}
	}
}

func TestTraeApprovalDeniesStaleRequests(t *testing.T) {
	for _, stale := range []bool{false, true} {
		var out bytes.Buffer
		p := &traeProcess{encoder: json.NewEncoder(&out)}
		frame := traeFrame{ID: json.RawMessage(`7`), Method: "item/commandExecution/requestApproval", Params: json.RawMessage(`{"threadId":"thread","turnId":"turn","itemId":"command","command":"echo test","availableDecisions":["accept","decline"]}`)}
		asked := false
		req := Request{Sandbox: SandboxWorkspaceWrite, PermissionHandler: func(_ context.Context, card PermissionRequest) (PermissionDecision, error) {
			asked = true
			if !strings.HasPrefix(card.ID, "trae:") {
				t.Fatal(card.ID)
			}
			return PermissionDecision{Behavior: "allow"}, nil
		}}
		turn := "turn"
		if stale {
			turn = "different"
		}
		if err := NewTraeRunner("").permission(context.Background(), p, frame, req, "thread", turn, func(Event) {}); err != nil {
			t.Fatal(err)
		}
		want := `"decision":"accept"`
		if stale {
			want = `"decision":"decline"`
		}
		if asked == stale || !strings.Contains(out.String(), want) {
			t.Fatalf("stale=%v asked=%v response=%s", stale, asked, out.String())
		}
	}
}

func TestTraeCancellationStopsTheServer(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell fixture")
	}
	path := filepath.Join(t.TempDir(), "traecli")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nwhile IFS= read -r line; do :; done\n"), 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := NewTraeRunner(path).InspectConfiguration(ctx, t.TempDir(), nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected cancellation, got %v", err)
	}
}

func TestTraeModelLoadPreservesUnknownAndZero(t *testing.T) {
	t.Setenv("ONECATCH_TRAE_MODELS", `{"id":2,"result":{"data":[{"id":"busy","businessMetadata":{"load":{"load_percent":264,"queue_size":null}}},{"id":"queued","businessMetadata":{"load":{"load_percent":120,"queue_size":12}}},{"id":"idle","businessMetadata":{"load":{"load_percent":0,"queue_size":0}}},{"id":"unknown","businessMetadata":{"load":null}}]}}`)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cfg, err := NewTraeRunner(stubTraeAppServer(t)).InspectConfiguration(ctx, t.TempDir(), os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Models) != 4 {
		t.Fatal(cfg.Models)
	}
	busy, queued, idle, unknown := cfg.Models[0].Load, cfg.Models[1].Load, cfg.Models[2].Load, cfg.Models[3].Load
	if busy == nil || busy.Percent == nil || *busy.Percent != 264 || busy.QueueSize != nil {
		t.Fatalf("busy load: %+v", busy)
	}
	if queued == nil || queued.QueueSize == nil || *queued.QueueSize != 12 {
		t.Fatalf("queue: %+v", queued)
	}
	if idle == nil || idle.Percent == nil || *idle.Percent != 0 || idle.QueueSize == nil || *idle.QueueSize != 0 {
		t.Fatalf("zero load: %+v", idle)
	}
	if unknown != nil {
		t.Fatalf("unknown load must stay absent: %+v", unknown)
	}
}

func TestTraeQueueStatus(t *testing.T) {
	state := traeTurnState{result: Result{SessionID: "thread"}, streams: map[string]bool{}}
	var events []Event
	sink := func(e Event) { events = append(events, e) }
	for _, params := range []string{
		`{"threadId":"other","turnId":"turn","state":"queued","position":9}`,
		`{"threadId":"thread","turnId":"old","state":"waiting"}`,
		`{"threadId":"thread","turnId":"turn","state":"unknown"}`,
		`{"threadId":"thread","turnId":"turn","state":"queued","position":0,"message":"Capacity busy","operation":"contextCompaction"}`,
		`{"threadId":"thread","turnId":"turn","state":"waiting","position":null}`,
		`{"threadId":"thread","turnId":"turn","state":"ready"}`,
	} {
		done, err := state.update(traeFrame{Method: "queue/status", Params: json.RawMessage(params)}, "turn", sink)
		if done || err != nil {
			t.Fatalf("queue must not complete the turn: %v %v", done, err)
		}
	}
	if len(events) != 3 {
		t.Fatalf("events: %+v", events)
	}
	for i, want := range []string{"queued", "waiting", "ready"} {
		var payload map[string]any
		if err := json.Unmarshal([]byte(events[i].Text), &payload); err != nil {
			t.Fatal(err)
		}
		if events[i].Kind != KindQueueStatus || payload["state"] != want {
			t.Fatalf("event: %+v", events[i])
		}
		if i == 0 && (payload["position"] != float64(0) || payload["message"] != "Capacity busy" || payload["operation"] != "contextCompaction") {
			t.Fatalf("queue details lost: %+v", payload)
		}
		if i > 0 {
			if _, ok := payload["position"]; ok {
				t.Fatalf("missing position became known: %+v", payload)
			}
		}
	}
}

func TestTraeQueuesNotificationsBeforeTurnResponse(t *testing.T) {
	var output bytes.Buffer
	p := &traeProcess{ctx: context.Background(), encoder: json.NewEncoder(&output), scanner: newJSONLineScanner(strings.NewReader(
		"{\"method\":\"queue/status\",\"params\":{\"state\":\"queued\"}}\n{\"id\":1,\"result\":{\"turn\":{\"id\":\"turn\"}}}\n{\"method\":\"queue/status\",\"params\":{\"state\":\"ready\"}}\n"))}
	if _, err := p.call("turn/start", nil); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"queued", "ready"} {
		f, err := p.read()
		if err != nil || f.Method != "queue/status" || !strings.Contains(string(f.Params), want) {
			t.Fatalf("lost notification %s: %+v %v", want, f, err)
		}
	}
}

func TestTraePreservesCacheReadAndCreationSeparately(t *testing.T) {
	state := traeTurnState{result: Result{SessionID: "thread"}, streams: map[string]bool{}}
	for _, sample := range []struct {
		raw             string
		cached, created int
	}{
		{`{"inputTokens":36935,"cachedInputTokens":0,"cacheCreationInputTokens":33968,"outputTokens":39,"totalTokens":36974}`, 0, 33968},
		{`{"inputTokens":36935,"cachedInputTokens":33968,"cacheCreationInputTokens":0,"outputTokens":39,"totalTokens":36974}`, 33968, 0},
	} {
		params := json.RawMessage(`{"threadId":"thread","turnId":"turn","tokenUsage":{"total":` + sample.raw + `,"last":` + sample.raw + `}}`)
		var received Usage
		_, err := state.update(traeFrame{Method: "thread/tokenUsage/updated", Params: params}, "turn", func(e Event) {
			if e.Usage != nil {
				received = *e.Usage
			}
		})
		if err != nil {
			t.Fatal(err)
		}
		if received.InputTokens != 36935 || received.CachedInputTokens != sample.cached || received.CacheCreationInputTokens != sample.created || state.result.Usage != received {
			t.Fatalf("cache accounting: %+v", received)
		}
	}
}
