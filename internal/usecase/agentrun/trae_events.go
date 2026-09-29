package agentrun

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type traeTurnState struct {
	result  Result
	streams map[string]bool
}
type traeTokens struct {
	Input     int `json:"inputTokens"`
	Cached    int `json:"cachedInputTokens"`
	Created   int `json:"cacheCreationInputTokens"`
	Output    int `json:"outputTokens"`
	Reasoning int `json:"reasoningOutputTokens"`
	Total     int `json:"totalTokens"`
}

func (s *traeTurnState) update(f traeFrame, turnID string, sink Sink) (bool, error) {
	var n struct {
		ThreadID  string          `json:"threadId"`
		TurnID    string          `json:"turnId"`
		ItemID    string          `json:"itemId"`
		Delta     string          `json:"delta"`
		WillRetry bool            `json:"willRetry"`
		Error     json.RawMessage `json:"error"`
		Turn      struct {
			ID     string          `json:"id"`
			Status string          `json:"status"`
			Error  json.RawMessage `json:"error"`
		} `json:"turn"`
		Item struct {
			ID               string          `json:"id"`
			Type             string          `json:"type"`
			Text             string          `json:"text"`
			Command          string          `json:"command"`
			Tool             string          `json:"tool"`
			Status           string          `json:"status"`
			AggregatedOutput string          `json:"aggregatedOutput"`
			ExitCode         *int            `json:"exitCode"`
			Summary          []string        `json:"summary"`
			Content          json.RawMessage `json:"content"`
			Result           json.RawMessage `json:"result"`
			Error            json.RawMessage `json:"error"`
			Changes          []struct {
				Path string `json:"path"`
			} `json:"changes"`
		} `json:"item"`
		TokenUsage struct {
			Total  traeTokens `json:"total"`
			Last   traeTokens `json:"last"`
			Window int        `json:"modelContextWindow"`
		} `json:"tokenUsage"`
	}
	if f.Method == "" {
		return false, nil
	}
	if err := json.Unmarshal(f.Params, &n); err != nil {
		return false, fmt.Errorf("decode TRAE %s: %w", f.Method, err)
	}
	if n.ThreadID != s.result.SessionID || (n.TurnID != "" && n.TurnID != turnID) || (n.Turn.ID != "" && n.Turn.ID != turnID) {
		return false, nil
	}
	raw, _ := json.Marshal(f)
	emit := func(kind EventKind, text string, phase StreamPhase, id string, failed bool) {
		sink(Event{Kind: kind, Text: text, StreamID: id, Phase: phase, Failed: failed, Raw: string(raw), At: time.Now()})
	}
	stream := func(kind EventKind, id, text string, phase StreamPhase) {
		key := "trae-" + string(kind) + "-" + id
		if phase == StreamStart {
			if s.streams[key] {
				return
			}
			s.streams[key] = true
		} else if phase == StreamDelta && !s.streams[key] {
			emit(kind, "", StreamStart, key, false)
			s.streams[key] = true
		}
		emit(kind, text, phase, key, false)
		if phase == StreamEnd {
			delete(s.streams, key)
		}
	}
	switch f.Method {
	case "queue/status":
		var queue struct {
			State     string  `json:"state"`
			Position  *uint32 `json:"position,omitempty"`
			Message   string  `json:"message,omitempty"`
			Operation string  `json:"operation,omitempty"`
		}
		if err := json.Unmarshal(f.Params, &queue); err != nil {
			return false, fmt.Errorf("decode TRAE queue status: %w", err)
		}
		if queue.State == "queued" || queue.State == "waiting" || queue.State == "ready" {
			text, _ := json.Marshal(queue)
			emit(KindQueueStatus, string(text), "", "", false)
		}
	case "item/agentMessage/delta":
		stream(KindMessage, n.ItemID, n.Delta, StreamDelta)
	case "item/reasoning/textDelta", "item/reasoning/summaryTextDelta":
		stream(KindReasoning, n.ItemID, n.Delta, StreamDelta)
	case "item/commandExecution/outputDelta":
		stream(KindToolResult, n.ItemID, n.Delta, StreamDelta)
	case "item/started":
		switch n.Item.Type {
		case "agentMessage":
			stream(KindMessage, n.Item.ID, "", StreamStart)
		case "reasoning":
			stream(KindReasoning, n.Item.ID, "", StreamStart)
		case "commandExecution":
			emit(KindToolUse, n.Item.Command, "", "", false)
		case "mcpToolCall", "dynamicToolCall":
			emit(KindToolUse, n.Item.Tool, "", "", false)
		}
	case "item/completed":
		failed := n.Item.Status == "failed" || n.Item.Status == "declined"
		switch n.Item.Type {
		case "agentMessage":
			s.result.FinalMessage = n.Item.Text
			stream(KindMessage, n.Item.ID, n.Item.Text, StreamEnd)
		case "reasoning":
			text := strings.Join(n.Item.Summary, "\n")
			if text == "" {
				text = traeContentText(n.Item.Content)
			}
			stream(KindReasoning, n.Item.ID, text, StreamEnd)
		case "commandExecution":
			if n.Item.ExitCode != nil && *n.Item.ExitCode != 0 {
				failed = true
			}
			id := "trae-" + string(KindToolResult) + "-" + n.Item.ID
			emit(KindToolResult, n.Item.AggregatedOutput, StreamEnd, id, failed)
			delete(s.streams, id)
		case "fileChange":
			var paths []string
			for _, change := range n.Item.Changes {
				paths = append(paths, change.Path)
			}
			emit(KindFileChange, strings.Join(paths, "\n"), "", "", failed)
		case "mcpToolCall", "dynamicToolCall":
			text := string(n.Item.Result)
			if text == "" || text == "null" {
				text = string(n.Item.Error)
			}
			emit(KindToolResult, text, "", "", failed)
		case "contextCompaction":
			emit(KindContextCompaction, "", "", "", false)
		}
	case "thread/tokenUsage/updated":
		tokens := n.TokenUsage.Total
		if tokens == (traeTokens{}) {
			tokens = n.TokenUsage.Last
		}
		s.result.Usage = Usage{InputTokens: tokens.Input, CachedInputTokens: tokens.Cached, CacheCreationInputTokens: tokens.Created, OutputTokens: tokens.Output, ReasoningOutputTokens: tokens.Reasoning}
		s.result.Context = ContextUsage{Tokens: n.TokenUsage.Last.Total, Window: n.TokenUsage.Window}
		usage, contextUsage := s.result.Usage, s.result.Context
		sink(Event{Kind: KindUsage, Usage: &usage, Context: &contextUsage, Raw: string(raw), At: time.Now()})
	case "turn/completed":
		s.result.Succeeded = n.Turn.Status == "completed"
		if !s.result.Succeeded {
			emit(KindError, string(n.Turn.Error), "", "", true)
		}
		return true, nil
	case "error":
		emit(KindError, string(n.Error), "", "", true)
		if !n.WillRetry {
			return false, fmt.Errorf("TRAE turn failed: %s", n.Error)
		}
	}
	return false, nil
}

func (r *TraeRunner) permission(ctx context.Context, p *traeProcess, f traeFrame, req Request, threadID, turnID string, sink Sink) error {
	switch f.Method {
	case "item/commandExecution/requestApproval", "item/fileChange/requestApproval", "item/permissions/requestApproval":
	default:
		return p.unsupported(f)
	}
	var args struct {
		ThreadID           string            `json:"threadId"`
		TurnID             string            `json:"turnId"`
		ItemID             string            `json:"itemId"`
		Command            string            `json:"command"`
		Reason             string            `json:"reason"`
		Permissions        map[string]any    `json:"permissions"`
		AvailableDecisions []json.RawMessage `json:"availableDecisions"`
	}
	if json.Unmarshal(f.Params, &args) != nil || args.ThreadID == "" || args.TurnID == "" || args.ItemID == "" || (f.Method == "item/permissions/requestApproval" && args.Permissions == nil) {
		return p.encoder.Encode(map[string]any{"id": f.ID, "error": map[string]any{"code": -32602, "message": "Invalid TRAE approval request"}})
	}
	card := PermissionRequest{ID: fmt.Sprintf("trae:%s:%s:%s", args.ThreadID, args.TurnID, f.ID), ToolUseID: args.ItemID, ToolName: strings.Split(f.Method, "/")[1], Title: args.Command, Description: args.Reason, SuppressAlwaysAllow: true}
	if card.Title == "" {
		card.Title = card.ToolName
	}
	_ = json.Unmarshal(f.Params, &card.Input)
	interactive := ctx.Err() == nil && req.PermissionHandler != nil && r.SupportsInteractivePermissions(req.Sandbox) && args.ThreadID == threadID && args.TurnID == turnID
	allowed := false
	if interactive {
		sink(Event{Kind: KindPermissionRequest, Text: card.Title, Permission: &card, At: time.Now()})
		choice, err := req.PermissionHandler(ctx, card)
		allowed = err == nil && ctx.Err() == nil && choice.Behavior == "allow"
	}
	result := map[string]any{"decision": "decline"}
	if len(args.AvailableDecisions) > 0 {
		accept, decline := false, false
		for _, raw := range args.AvailableDecisions {
			var value string
			_ = json.Unmarshal(raw, &value)
			accept = accept || value == "accept"
			decline = decline || value == "decline"
		}
		allowed = allowed && accept
		if !decline {
			result["decision"] = "cancel"
		}
	}
	if allowed {
		result["decision"] = "accept"
	}
	if f.Method == "item/permissions/requestApproval" {
		result = map[string]any{"permissions": map[string]any{}, "scope": "turn"}
		if allowed {
			result["permissions"] = args.Permissions
		}
	}
	if interactive {
		decision := "deny"
		if allowed {
			decision = "allow"
		}
		sink(Event{Kind: KindPermissionResolved, Text: card.Title, Permission: &card, PermissionDecision: decision, At: time.Now()})
	}
	return p.encoder.Encode(map[string]any{"id": f.ID, "result": result})
}

// TRAE item.content is a union: user messages contain structured blocks,
// whereas reasoning items can carry strings. Never decode it as []string at
// the envelope level, which would reject otherwise valid user/tool items.
func traeContentText(raw json.RawMessage) string {
	var parts []json.RawMessage
	if json.Unmarshal(raw, &parts) != nil {
		return ""
	}
	var text []string
	for _, part := range parts {
		var value string
		if json.Unmarshal(part, &value) == nil {
			text = append(text, value)
			continue
		}
		var block struct {
			Text string `json:"text"`
		}
		if json.Unmarshal(part, &block) == nil && block.Text != "" {
			text = append(text, block.Text)
		}
	}
	return strings.Join(text, "\n")
}
