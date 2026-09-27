package agentrun

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestCodexApprovalDecisions(t *testing.T) {
	for _, method := range []string{"item/commandExecution/requestApproval", "item/fileChange/requestApproval", "item/permissions/requestApproval"} {
		for _, scenario := range []string{"allow", "deny", "handler error", "missing handler", "cancelled", "cancel during approval", "stale turn", "other thread", "full", "invalid decision", "unavailable accept"} {
			t.Run(method+"/"+scenario, func(t *testing.T) {
				params := map[string]any{
					"threadId": "thread-1", "turnId": "turn-1", "itemId": "git-1",
					"command": "git add main.go && git commit -m fix", "cwd": "/workspace",
					"reason":      "Git needs to update its index",
					"permissions": map[string]any{"fileSystem": map[string]any{"write": []string{"/workspace/.git"}}},
				}
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				called := false
				req := Request{Sandbox: SandboxWorkspaceWrite, PermissionHandler: func(_ context.Context, p PermissionRequest) (PermissionDecision, error) {
					called = true
					if !p.SuppressAlwaysAllow || p.ToolUseID != "git-1" || p.Input["cwd"] != "/workspace" || p.Description != params["reason"] {
						t.Fatalf("incomplete approval card: %+v", p)
					}
					if scenario == "handler error" {
						return PermissionDecision{Behavior: "allow"}, errors.New("closed")
					}
					if scenario == "cancel during approval" {
						cancel()
					}
					behavior := "allow"
					if scenario == "deny" || scenario == "invalid decision" {
						behavior = scenario
					}
					return PermissionDecision{Behavior: behavior, UpdatedPermissions: []PermissionUpdate{{"write": []string{"/"}}}}, nil
				}}
				switch scenario {
				case "missing handler":
					req.PermissionHandler = nil
				case "cancelled":
					cancel()
				case "stale turn":
					params["turnId"] = "old-turn"
				case "other thread":
					params["threadId"] = "other-thread"
				case "full":
					req.Sandbox = SandboxFull
				case "unavailable accept":
					params["availableDecisions"] = []string{"cancel"}
				}
				data, _ := json.Marshal(params)
				envelope := codexAppEnvelope{ID: json.RawMessage(`"approval-1"`), Method: method, Params: data}
				var output bytes.Buffer
				var events []Event
				runner := &CodexRunner{now: fixedClock()}
				if err := runner.handleCodexApproval(ctx, json.NewEncoder(&output), envelope, req, "thread-1", "turn-1", string(data), collectSink(&events)); err != nil {
					t.Fatal(err)
				}
				var response struct {
					ID     string         `json:"id"`
					Result map[string]any `json:"result"`
				}
				if err := json.Unmarshal(output.Bytes(), &response); err != nil || response.ID != "approval-1" {
					t.Fatalf("invalid response: %s (%v)", output.String(), err)
				}
				wantAllow := scenario == "allow" || (scenario == "unavailable accept" && method != "item/commandExecution/requestApproval")
				if method == "item/permissions/requestApproval" {
					grant, _ := json.Marshal(response.Result["permissions"])
					want := `{}`
					if wantAllow {
						want = `{"fileSystem":{"write":["/workspace/.git"]}}`
					}
					if string(grant) != want || response.Result["scope"] != "turn" {
						t.Fatalf("unexpected grant: %s", output.String())
					}
				} else {
					want := "decline"
					if wantAllow {
						want = "accept"
					} else if scenario == "unavailable accept" {
						want = "cancel"
					}
					if response.Result["decision"] != want {
						t.Fatalf("unexpected decision: %s", output.String())
					}
				}
				if called {
					want := "deny"
					if wantAllow {
						want = "allow"
					}
					if len(events) != 2 || events[0].Kind != KindPermissionRequest || events[1].Kind != KindPermissionResolved || events[1].PermissionDecision != want {
						t.Fatalf("approval events = %+v", events)
					}
				} else if len(events) != 0 {
					t.Fatalf("non-interactive request emitted a pending card: %+v", events)
				}
			})
		}
	}
}

func TestCodexApprovalPolicyRequiresAHost(t *testing.T) {
	for _, sandbox := range []Sandbox{"", SandboxReadOnly, SandboxWorkspaceWrite, SandboxFull} {
		for _, interactive := range []bool{false, true} {
			for _, resume := range []string{"", "existing-thread"} {
				t.Run(fmt.Sprintf("%s/%t/%s", sandbox, interactive, resume), func(t *testing.T) {
					req := Request{Sandbox: sandbox, ResumeSessionID: resume}
					if interactive {
						req.PermissionHandler = func(context.Context, PermissionRequest) (PermissionDecision, error) { return PermissionDecision{}, nil }
					}
					var output bytes.Buffer
					if err := sendCodexThreadStart(json.NewEncoder(&output), req); err != nil {
						t.Fatal(err)
					}
					var call struct {
						Method string         `json:"method"`
						Params map[string]any `json:"params"`
					}
					if err := json.Unmarshal(output.Bytes(), &call); err != nil {
						t.Fatal(err)
					}
					want := "never"
					if interactive && sandbox != SandboxFull {
						want = "on-request"
					}
					if call.Params["approvalPolicy"] != want || call.Params["sandbox"] != codexSandbox(sandbox) {
						t.Fatalf("thread permissions: %s", output.String())
					}
					if resume != "" && call.Method != "thread/resume" {
						t.Fatal("existing threads must receive the updated policy on resume")
					}
				})
			}
		}
	}
}

// A recorded app-server stream exercises request routing through the same
// turn loop used by local processes and remote streams.
func TestCodexStreamRoutesApprovalAndContinues(t *testing.T) {
	stream := &codexApprovalTestStream{Reader: strings.NewReader(`
{"id":1,"result":{}}
{"id":2,"result":{"data":[]}}
{"id":3,"result":{"thread":{"id":"thread-1"}}}
{"id":4,"result":{"turn":{"id":"turn-1"}}}
{"id":"approval-1","method":"item/commandExecution/requestApproval","params":{"threadId":"thread-1","turnId":"turn-1","itemId":"git-1","command":"git commit -m fix"}}
{"method":"turn/completed","params":{"threadId":"thread-1","turn":{"id":"turn-1","status":"completed"}}}
`)}
	var events []Event
	result, err := RunCodexStream(context.Background(), Request{
		Sandbox: SandboxWorkspaceWrite,
		PermissionHandler: func(context.Context, PermissionRequest) (PermissionDecision, error) {
			return PermissionDecision{Behavior: "allow"}, nil
		},
	}, stream, collectSink(&events))
	if err != nil || !result.Succeeded || countKind(events, KindPermissionRequest) != 1 || countKind(events, KindPermissionResolved) != 1 {
		t.Fatalf("run = %+v, %v; events = %+v", result, err, events)
	}
	if !strings.Contains(stream.output.String(), `"result":{"decision":"accept"}`) || !strings.Contains(stream.output.String(), `"approvalPolicy":"on-request"`) {
		t.Fatalf("approval was not returned to the app-server: %s", stream.output.String())
	}
}

type codexApprovalTestStream struct {
	*strings.Reader
	output bytes.Buffer
}

func (s *codexApprovalTestStream) Write(p []byte) (int, error) { return s.output.Write(p) }
func (s *codexApprovalTestStream) Close() error                { return nil }
