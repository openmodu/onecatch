package desktop

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/openmodu/oneshot/internal/usecase/agentrun"
)

const (
	maxWhiteboardCanvasBytes = 512 * 1024
	maxWhiteboardChanges     = 12
)

// WhiteboardProposalInput is the complete, read-only context for one Agent
// turn on the collaborative canvas. The Agent can inspect the workspace and
// canvas, but it can only return proposals; accepting a proposal remains a
// separate human action in the UI.
type WhiteboardProposalInput struct {
	WorkspaceID string `json:"workspaceId"`
	Runtime     string `json:"runtime"`
	Instruction string `json:"instruction"`
	CanvasJSON  string `json:"canvasJson"`
	RequestID   string `json:"requestId,omitempty"`
	SessionID   string `json:"sessionId,omitempty"`
}

type WhiteboardChange struct {
	ID                   string  `json:"id"`
	Action               string  `json:"action"`
	ObjectType           string  `json:"objectType"`
	Category             string  `json:"category"`
	Title                string  `json:"title"`
	Content              string  `json:"content"`
	TargetID             string  `json:"targetId,omitempty"`
	X                    float64 `json:"x"`
	Y                    float64 `json:"y"`
	Width                float64 `json:"width"`
	Height               float64 `json:"height"`
	RequiresConfirmation bool    `json:"requiresConfirmation,omitempty"`
}

type WhiteboardProposal struct {
	Runtime   string             `json:"runtime"`
	SessionID string             `json:"sessionId,omitempty"`
	Summary   string             `json:"summary"`
	Changes   []WhiteboardChange `json:"changes"`
	Usage     agentrun.Usage     `json:"usage"`
}

// ProposeWhiteboardChanges runs a real local Agent in read-only mode. The
// structured result is deliberately proposal-only: no file or canvas mutation
// happens until the user accepts individual changes in the frontend.
func (a *Service) ProposeWhiteboardChanges(_ context.Context, input WhiteboardProposalInput) (WhiteboardProposal, error) {
	return a.ProposeWhiteboardChangesWithSink(input, nil)
}

// ProposeWhiteboardChangesWithSink exposes the runtime's normalized progress
// stream to the desktop transport. The stream is observational only: actual
// canvas mutations still come from the validated proposal returned at the end.
func (a *Service) ProposeWhiteboardChangesWithSink(input WhiteboardProposalInput, sink agentrun.Sink) (WhiteboardProposal, error) {
	workspaceID := strings.TrimSpace(input.WorkspaceID)
	workspace, err := a.GetWorkspace(a.rootCtx, workspaceID)
	if err != nil {
		return WhiteboardProposal{}, err
	}
	runtime := agentrun.Runtime(strings.TrimSpace(input.Runtime))
	if runtime == "" {
		runtime = agentrun.RuntimeCodex
	}
	if !runtime.Valid() {
		return WhiteboardProposal{}, coded("whiteboard_runtime_invalid", "Agent runtime is invalid")
	}
	if !a.runtimes.Available(runtime) {
		return WhiteboardProposal{}, coded("whiteboard_runtime_unavailable", fmt.Sprintf("runtime %q is unavailable", runtime))
	}
	instruction := strings.TrimSpace(input.Instruction)
	if instruction == "" {
		return WhiteboardProposal{}, coded("whiteboard_instruction_required", "instruction is required")
	}
	canvas := strings.TrimSpace(input.CanvasJSON)
	if canvas == "" || len(canvas) > maxWhiteboardCanvasBytes || !json.Valid([]byte(canvas)) {
		return WhiteboardProposal{}, coded("whiteboard_canvas_invalid", "canvas context must be valid JSON within 512 KiB")
	}

	runCtx, cancel := context.WithTimeout(a.rootCtx, 10*time.Minute)
	requestID := strings.TrimSpace(input.RequestID)
	if requestID != "" {
		a.whiteboardMu.Lock()
		if previous := a.whiteboardRuns[requestID]; previous != nil {
			previous()
		}
		a.whiteboardRuns[requestID] = cancel
		a.whiteboardMu.Unlock()
		defer func() {
			a.whiteboardMu.Lock()
			delete(a.whiteboardRuns, requestID)
			a.whiteboardMu.Unlock()
		}()
	}
	defer cancel()
	result, err := a.runtimes.Run(runCtx, agentrun.Request{
		Runtime:         runtime,
		Workspace:       workspace.Path,
		Prompt:          whiteboardAgentPrompt(instruction, canvas),
		Sandbox:         agentrun.SandboxReadOnly,
		ResumeSessionID: strings.TrimSpace(input.SessionID),
	}, sink)
	if err != nil {
		return WhiteboardProposal{}, coded("whiteboard_agent_failed", err.Error())
	}
	if !result.Succeeded {
		return WhiteboardProposal{}, coded("whiteboard_agent_failed", "Agent did not complete successfully")
	}
	proposal, err := parseWhiteboardProposal(result.FinalMessage)
	if err != nil {
		return WhiteboardProposal{}, coded("whiteboard_proposal_invalid", err.Error())
	}
	proposal.Runtime = string(runtime)
	proposal.SessionID = result.SessionID
	proposal.Usage = result.Usage
	return proposal, nil
}

// CancelWhiteboardRequest stops an in-flight Agent turn. It is intentionally
// idempotent because the runtime may finish between the user's click and the
// transport call reaching the service.
func (a *Service) CancelWhiteboardRequest(requestID string) {
	a.whiteboardMu.Lock()
	cancel := a.whiteboardRuns[strings.TrimSpace(requestID)]
	a.whiteboardMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func whiteboardAgentPrompt(instruction, canvas string) string {
	return strings.Join([]string{
		"You are operating inside Oneshot's collaborative infinite canvas.",
		"Your job is to propose useful canvas changes, not to modify workspace files or the canvas directly.",
		"Read the canvas context, infer relationships, and return a small reviewable patch for the human.",
		"The canvas context includes selectedObjectId and review.focusedChangeId. Treat them as the human's current focus.",
		"review.pendingChanges is your existing working layer. Reuse its ids with action=update or action=link when refining it; do not create duplicate cards.",
		"Coordinates use a 1120 by 800 canvas. Keep proposals between x=420..980 and y=70..700.",
		"Return ONLY one JSON object. Do not use Markdown fences or add prose before or after it.",
		`Schema: {"summary":"short Chinese summary","changes":[{"id":"stable-kebab-id","action":"add|update|link","objectType":"card|checklist|risk|file|test|note","category":"new|linked|confirm","title":"short Chinese title","content":"plain text; use newline between items","targetId":"existing object id when updating/linking","x":700,"y":120,"width":240,"height":130,"requiresConfirmation":false}]}`,
		"Return 1 to 6 changes. Each change must stand alone and be safe to accept independently.",
		"Use category=confirm and requiresConfirmation=true for decisions the Agent must not make alone.",
		"Prefer concrete tests, task ordering, risk notes, and file relationships over generic advice.",
		"",
		"## Human instruction",
		instruction,
		"",
		"## Canvas context JSON",
		canvas,
	}, "\n")
}

func parseWhiteboardProposal(message string) (WhiteboardProposal, error) {
	trimmed := strings.TrimSpace(message)
	if start, end := strings.Index(trimmed, "{"), strings.LastIndex(trimmed, "}"); start >= 0 && end >= start {
		trimmed = trimmed[start : end+1]
	}
	var proposal WhiteboardProposal
	if err := json.Unmarshal([]byte(trimmed), &proposal); err != nil {
		return WhiteboardProposal{}, fmt.Errorf("Agent returned invalid proposal JSON: %w", err)
	}
	proposal.Summary = strings.TrimSpace(proposal.Summary)
	if proposal.Summary == "" {
		proposal.Summary = "Agent 已生成画布提案"
	}
	if len(proposal.Changes) == 0 {
		return WhiteboardProposal{}, fmt.Errorf("Agent returned no canvas changes")
	}
	if len(proposal.Changes) > maxWhiteboardChanges {
		proposal.Changes = proposal.Changes[:maxWhiteboardChanges]
	}
	seen := make(map[string]bool, len(proposal.Changes))
	for index := range proposal.Changes {
		proposal.Changes[index] = NormalizeWhiteboardChange(proposal.Changes[index], index)
		change := &proposal.Changes[index]
		if change.ID == "" || seen[change.ID] {
			change.ID = fmt.Sprintf("agent-change-%d", index+1)
		}
		seen[change.ID] = true
	}
	return proposal, nil
}

// NormalizeWhiteboardChange applies the same validation to a change discovered
// in the live assistant stream as to the completed proposal. This lets the UI
// render an operation as soon as its JSON object is complete without trusting
// partially generated coordinates or enum values.
func NormalizeWhiteboardChange(change WhiteboardChange, index int) WhiteboardChange {
	change.ID = strings.TrimSpace(change.ID)
	if change.ID == "" {
		change.ID = fmt.Sprintf("agent-change-%d", index+1)
	}
	change.Action = normalizeEnum(change.Action, []string{"add", "update", "link"}, "add")
	change.ObjectType = normalizeEnum(change.ObjectType, []string{"card", "checklist", "risk", "file", "test", "note"}, "card")
	change.Category = normalizeEnum(change.Category, []string{"new", "linked", "confirm"}, "new")
	change.Title = strings.TrimSpace(change.Title)
	change.Content = strings.TrimSpace(change.Content)
	change.TargetID = strings.TrimSpace(change.TargetID)
	if change.Title == "" {
		change.Title = fmt.Sprintf("Agent 提案 %d", index+1)
	}
	change.X = clampFloat(change.X, 420, 980, 520+float64((index%2)*220))
	change.Y = clampFloat(change.Y, 70, 700, 100+float64(index*118))
	change.Width = clampFloat(change.Width, 180, 360, 240)
	change.Height = clampFloat(change.Height, 82, 280, 120)
	if change.Category == "confirm" {
		change.RequiresConfirmation = true
	}
	return change
}

func normalizeEnum(value string, allowed []string, fallback string) string {
	value = strings.TrimSpace(value)
	for _, candidate := range allowed {
		if value == candidate {
			return value
		}
	}
	return fallback
}

func clampFloat(value, min, max, fallback float64) float64 {
	if value == 0 {
		return fallback
	}
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
