package desktop

import (
	"context"
	"strings"
	"testing"
)

func TestParseWhiteboardProposalNormalizesAgentOutput(t *testing.T) {
	proposal, err := parseWhiteboardProposal("```json\n" + `{"summary":"梳理完成","changes":[{"id":"task-order","action":"invented","objectType":"checklist","category":"confirm","title":"任务序列","content":"先恢复，再优化","x":9999,"y":0,"width":10,"height":999}]}` + "\n```")
	if err != nil {
		t.Fatal(err)
	}
	if proposal.Summary != "梳理完成" || len(proposal.Changes) != 1 {
		t.Fatalf("proposal = %+v", proposal)
	}
	change := proposal.Changes[0]
	if change.Action != "add" || change.X != 980 || change.Y != 100 || change.Width != 180 || change.Height != 280 {
		t.Fatalf("normalized change = %+v", change)
	}
	if !change.RequiresConfirmation {
		t.Fatal("confirm category must require confirmation")
	}
}

func TestParseWhiteboardProposalRejectsEmptyChanges(t *testing.T) {
	if _, err := parseWhiteboardProposal(`{"summary":"nothing","changes":[]}`); err == nil {
		t.Fatal("expected empty proposal to fail")
	}
}

func TestWhiteboardPromptKeepsCanvasAndGuardrail(t *testing.T) {
	prompt := whiteboardAgentPrompt("继续优化", `{"objects":[{"id":"goal"}]}`)
	for _, want := range []string{"继续优化", `"id":"goal"`, "not to modify workspace files", "review.pendingChanges", "action=update", "Return ONLY one JSON object"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q", want)
		}
	}
}

func TestCancelWhiteboardRequestIsIdempotent(t *testing.T) {
	cancelled := false
	service := &Service{whiteboardRuns: map[string]context.CancelFunc{
		"request-1": func() { cancelled = true },
	}}
	service.CancelWhiteboardRequest(" request-1 ")
	service.CancelWhiteboardRequest("missing")
	if !cancelled {
		t.Fatal("active whiteboard request was not cancelled")
	}
}
