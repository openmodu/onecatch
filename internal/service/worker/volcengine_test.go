package worker

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

func TestSandboxRegistrySeparatesCredentials(t *testing.T) {
	r := NewRegistry(filepath.Join(t.TempDir(), "workers.json"))
	in := Input{ID: "sandbox-test", Name: "Sandbox", Provider: ProviderVolcengineSandbox, RemotePath: "/home/gem/project", BaseURL: "https://test.volceapi.com/?faasInstanceName=instance&Authorization=secret%2Btoken&session_id=someone-elses-terminal", Enabled: true}
	info, err := r.Save(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(info)
	if strings.Contains(string(raw), "secret") || strings.Contains(info.BaseURL, "session_id") || strings.Contains(info.BaseURL, "Authorization") {
		t.Fatalf("credentials leaked in public worker")
	}
	config, err := r.Get(context.Background(), in.ID)
	if err != nil || config.Token != "secret+token" {
		t.Fatal("token was not persisted separately")
	}
	_, err = r.Update(context.Background(), UpdateInput{ID: info.ID, Name: info.Name, BaseURL: info.BaseURL, Provider: info.Provider, RemotePath: "/home/gem/other", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	config, _ = r.Get(context.Background(), in.ID)
	if config.Token != "secret+token" || config.RemotePath != "/home/gem/other" {
		t.Fatal("update lost credentials or path")
	}
	stat, _ := os.Stat(r.path)
	if stat.Mode().Perm() != 0600 {
		t.Fatalf("credential file permissions: %v", stat.Mode())
	}
	for _, p := range []string{"relative", "/tmp/a\nwhoami", "/tmp/\x00"} {
		in.RemotePath = p
		if _, err := r.Save(context.Background(), in); err == nil {
			t.Fatalf("accepted invalid remote path %q", p)
		}
	}
}

// This server models the observed shell protocol, including bootstrap echo,
// fragmented output and long JSON inputs past PTY limits.
func mockSandbox(t *testing.T, hang bool) (*httptest.Server, *atomic.Bool, *atomic.Bool) {
	t.Helper()
	deleted, initialized := &atomic.Bool{}, &atomic.Bool{}
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("Authorization") != "secret" || r.URL.Query().Get("faasInstanceName") != "instance" {
			t.Error("gateway query parameters missing")
			w.WriteHeader(401)
			return
		}
		if r.Method == http.MethodDelete && r.URL.Path == "/v1/shell/sessions/test-session" {
			deleted.Store(true)
			return
		}
		if r.URL.Path != "/v1/shell/ws" {
			w.WriteHeader(404)
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		send := func(kind string, data any) { _ = conn.WriteJSON(map[string]any{"type": kind, "data": data}) }
		send("session_id", "test-session")
		send("ready", nil)
		var frame struct {
			Type string `json:"type"`
			Data string `json:"data"`
		}
		if err := conn.ReadJSON(&frame); err != nil {
			return
		}
		if !strings.Contains(frame.Data, "stty -echo -icanon -opost") || !strings.Contains(frame.Data, "cd '/remote/project'") {
			t.Error("unsafe/missing PTY bootstrap")
		}
		marker := regexp.MustCompile(`ONECATCH_[0-9a-f]+`).FindString(frame.Data)
		send("output", frame.Data) // command echo must not be mistaken for readiness
		send("output", "\n"+marker[:12])
		send("output", marker[12:]+"\n")
		rpc := func(v any) { b, _ := json.Marshal(v); send("output", string(b)+"\n") }
		for {
			if err := conn.ReadJSON(&frame); err != nil {
				return
			}
			if frame.Type == "pong" {
				continue
			}
			var message struct {
				ID     int             `json:"id"`
				Method string          `json:"method"`
				Params json.RawMessage `json:"params"`
			}
			if err := json.Unmarshal([]byte(frame.Data), &message); err != nil {
				t.Error(err)
				return
			}
			switch message.Method {
			case "initialize":
				initialized.Store(true)
				if hang {
					continue
				}
				rpc(map[string]any{"id": message.ID, "result": map[string]any{}})
			case "skills/list":
				rpc(map[string]any{"id": message.ID, "result": map[string]any{"data": []any{}}})
			case "thread/start", "thread/resume":
				if !strings.Contains(string(message.Params), `"cwd":"/remote/project"`) {
					t.Error("local path reached remote Codex")
				}
				rpc(map[string]any{"id": message.ID, "result": map[string]any{"thread": map[string]string{"id": "remote-thread"}}})
			case "turn/start":
				rpc(map[string]any{"id": message.ID, "result": map[string]any{"turn": map[string]string{"id": "turn"}}})
				rpc(map[string]any{"method": "item/agentMessage/delta", "params": map[string]string{"threadId": "remote-thread", "turnId": "turn", "itemId": "message", "delta": "remote result"}})
				rpc(map[string]any{"method": "turn/completed", "params": map[string]any{"threadId": "remote-thread", "turn": map[string]string{"id": "turn", "status": "completed"}}})
			}
		}
	}))
	return server, deleted, initialized
}

func TestSandboxRunStreamsRemoteEventsAndCleansSession(t *testing.T) {
	server, deleted, _ := mockSandbox(t, false)
	defer server.Close()
	config := Config{Provider: ProviderVolcengineSandbox, BaseURL: server.URL + "/?faasInstanceName=instance", Token: "secret", RemotePath: "/remote/project"}
	var events []agentrun.Event
	result, err := NewClient().RunSandbox(context.Background(), config, agentrun.Request{Runtime: agentrun.RuntimeCodex, Workspace: "/local/never-use", Prompt: strings.Repeat("long prompt ", 1000)}, func(event agentrun.Event) { events = append(events, event) })
	if err != nil {
		t.Fatal(err)
	}
	if !result.Succeeded || result.SessionID != "remote-thread" || len(events) == 0 {
		t.Fatalf("missing remote result/events: %+v", result)
	}
	if !deleted.Load() {
		t.Fatal("remote process session was not cleaned up")
	}
}

func TestSandboxCancellationUnblocksAndCleansSession(t *testing.T) {
	server, deleted, initialized := mockSandbox(t, true)
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := NewClient().RunSandbox(ctx, Config{BaseURL: server.URL + "/?faasInstanceName=instance", Token: "secret", RemotePath: "/remote/project"}, agentrun.Request{Runtime: agentrun.RuntimeCodex}, nil)
		done <- err
	}()
	deadline := time.After(5 * time.Second)
	for !initialized.Load() {
		select {
		case <-deadline:
			t.Fatal("initialize timed out")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancelled run succeeded")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancel blocked")
	}
	if !deleted.Load() {
		t.Fatal("cancel left remote session alive")
	}
}

// Opt-in smoke test; the endpoint and its credential never enter fixtures.
func TestSandboxLiveHandshake(t *testing.T) {
	endpoint := os.Getenv("ONECATCH_TEST_SANDBOX_URL")
	if endpoint == "" {
		t.Skip("set ONECATCH_TEST_SANDBOX_URL to probe a live sandbox")
	}
	input, err := NewClient().ConnectSandbox(context.Background(), endpoint, "/home/gem")
	if err != nil {
		t.Fatal(err)
	}
	if input.Token == "" {
		t.Fatal("missing token")
	}
	// Ensure public serialization cannot include the live credential.
	b, _ := json.Marshal(publicInfo(Config{ID: input.ID, BaseURL: input.BaseURL}))
	if strings.Contains(string(b), input.Token) {
		t.Fatal("credential disclosure")
	}
}

var _ io.ReadWriteCloser = (*sandboxStream)(nil)

func TestSandboxLiveTurn(t *testing.T) {
	endpoint := os.Getenv("ONECATCH_TEST_SANDBOX_URL")
	if endpoint == "" || os.Getenv("ONECATCH_TEST_SANDBOX_TURN") != "1" {
		t.Skip("live model turn is opt-in")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	client := NewClient()
	input, err := client.ConnectSandbox(ctx, endpoint, "/home/gem")
	if err != nil {
		t.Fatal(err)
	}
	config := Config{Provider: input.Provider, BaseURL: input.BaseURL, Token: input.Token, RemotePath: input.RemotePath}
	events := 0
	result, err := client.RunSandbox(ctx, config, agentrun.Request{Runtime: agentrun.RuntimeCodex, Workspace: "/this-local-path-must-not-be-used", Sandbox: agentrun.SandboxReadOnly, Prompt: "Run pwd once, then reply ONECATCH_SANDBOX_OK and the working directory. Do not modify any files."}, func(event agentrun.Event) { events++ })
	if err != nil {
		t.Fatal(err)
	}
	if !result.Succeeded || result.SessionID == "" || events == 0 {
		t.Fatalf("turn failed: %+v", result)
	}
	t.Logf("remote turn succeeded, events=%d", events)
	resumed, err := client.RunSandbox(ctx, config, agentrun.Request{Runtime: agentrun.RuntimeCodex, Sandbox: agentrun.SandboxReadOnly, ResumeSessionID: result.SessionID, Prompt: "Reply with the working directory from your previous turn. Do not run any tools."}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !resumed.Succeeded || resumed.SessionID != result.SessionID {
		t.Fatal("remote thread resume failed")
	}
}
