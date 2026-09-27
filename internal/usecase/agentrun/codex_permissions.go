package agentrun

import (
	"context"
	"encoding/json"
	"fmt"
)

// Codex protects .git even in workspace-write mode. Its app-server approval
// channel lets the host authorize a blocked operation without disabling the
// sandbox for the rest of the run.
func (r *CodexRunner) SupportsInteractivePermissions(sandbox Sandbox) bool {
	return sandbox == "" || sandbox == SandboxReadOnly || sandbox == SandboxWorkspaceWrite
}

func codexApprovalPolicy(req Request) string {
	if req.PermissionHandler != nil && (&CodexRunner{}).SupportsInteractivePermissions(req.Sandbox) {
		return "on-request"
	}
	return "never"
}

func (r *CodexRunner) handleCodexApproval(ctx context.Context, encoder *json.Encoder, envelope codexAppEnvelope, req Request, threadID, turnID, raw string, sink Sink) error {
	switch envelope.Method {
	case "item/commandExecution/requestApproval", "item/fileChange/requestApproval", "item/permissions/requestApproval":
	default:
		return respondUnsupportedCodexRequest(encoder, envelope)
	}
	var params struct {
		ThreadID               string         `json:"threadId"`
		TurnID                 string         `json:"turnId"`
		ItemID                 string         `json:"itemId"`
		Reason                 string         `json:"reason"`
		Command                string         `json:"command"`
		Permissions            map[string]any `json:"permissions"`
		NetworkApprovalContext *struct {
			Host     string `json:"host"`
			Protocol string `json:"protocol"`
		} `json:"networkApprovalContext"`
		AvailableDecisions []json.RawMessage `json:"availableDecisions"`
	}
	if err := json.Unmarshal(envelope.Params, &params); err != nil || params.ThreadID == "" || params.TurnID == "" || params.ItemID == "" ||
		(envelope.Method == "item/permissions/requestApproval" && params.Permissions == nil) {
		return encoder.Encode(map[string]any{
			"id": envelope.ID, "error": map[string]any{"code": -32602, "message": "invalid Codex approval request"},
		})
	}

	request := PermissionRequest{
		// Request IDs are scoped to an app-server connection. Include the
		// thread and turn so concurrent tasks cannot resolve each other's card.
		ID:        fmt.Sprintf("codex:%s:%s:%s", params.ThreadID, params.TurnID, envelope.ID),
		ToolUseID: params.ItemID, Description: params.Reason,
		// OneCatch's persistent option means an enduring rule. Codex's
		// acceptForSession and turn-scoped permission grants do not mean that.
		SuppressAlwaysAllow: true,
	}
	_ = json.Unmarshal(envelope.Params, &request.Input)
	switch envelope.Method {
	case "item/commandExecution/requestApproval":
		request.ToolName, request.Title = "commandExecution", params.Command
		if params.NetworkApprovalContext != nil {
			request.ToolName = "networkAccess"
			request.Title = fmt.Sprintf("%s: %s", params.NetworkApprovalContext.Protocol, params.NetworkApprovalContext.Host)
		}
	case "item/fileChange/requestApproval":
		request.ToolName = "fileChange"
	case "item/permissions/requestApproval":
		request.ToolName = "request_permissions"
	}
	if request.Title == "" {
		request.Title = request.ToolName
	}

	// Stale callbacks, helpers without an approval UI, and cancelled runs must
	// never gain access. The decision remains denied unless the host allows it.
	allowed := false
	interactive := codexApprovalPolicy(req) == "on-request" && ctx.Err() == nil && params.ThreadID == threadID && params.TurnID == turnID
	if interactive {
		sink(Event{Kind: KindPermissionRequest, Text: request.Title, Permission: &request, Raw: raw, At: r.now()})
		decision, err := req.PermissionHandler(ctx, request)
		allowed = err == nil && ctx.Err() == nil && decision.Behavior == "allow"
	}
	result := map[string]any{"decision": "decline"}
	if envelope.Method == "item/permissions/requestApproval" {
		result = map[string]any{"permissions": map[string]any{}, "scope": "turn"}
		if allowed {
			// Return exactly the requested grant, never handler-supplied extras.
			result["permissions"] = params.Permissions
		}
	} else {
		if envelope.Method == "item/commandExecution/requestApproval" && len(params.AvailableDecisions) > 0 {
			allowed = allowed && codexHasApprovalDecision(params.AvailableDecisions, "accept")
			if !codexHasApprovalDecision(params.AvailableDecisions, "decline") {
				result["decision"] = "cancel"
			}
		}
		if allowed {
			result["decision"] = "accept"
		}
	}
	if interactive {
		decision := "deny"
		if allowed {
			decision = "allow"
		}
		sink(Event{Kind: KindPermissionResolved, Text: request.Title, Permission: &request, PermissionDecision: decision, At: r.now()})
	}
	if err := encoder.Encode(map[string]any{"id": envelope.ID, "result": result}); err != nil {
		return fmt.Errorf("respond to Codex approval: %w", err)
	}
	return nil
}

func codexHasApprovalDecision(decisions []json.RawMessage, want string) bool {
	for _, raw := range decisions {
		var decision string
		if json.Unmarshal(raw, &decision) == nil && decision == want {
			return true
		}
	}
	return false
}
