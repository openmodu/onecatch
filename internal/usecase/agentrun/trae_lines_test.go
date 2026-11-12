package agentrun

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTraeResumesLargeConversation(t *testing.T) {
	response := `{"id":3,"result":{"thread":{"id":"thread-live","turns":[{"items":[{"type":"agentMessage","text":"` + strings.Repeat("x", 9*1024*1024) + `"}]}]}}}` + "\n"
	path := filepath.Join(t.TempDir(), "resume.jsonl")
	if err := os.WriteFile(path, []byte(response), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ONECATCH_TRAE_THREAD_RESPONSE", path)
	runner := NewTraeRunner(stubTraeAppServer(t))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result, err := runner.Run(ctx, Request{Workspace: t.TempDir(), ResumeSessionID: "thread-live", Prompt: "continue", Sandbox: SandboxReadOnly}, nil)
	if err != nil || !result.Succeeded || result.SessionID != "thread-live" || result.FinalMessage != "Hello" {
		t.Fatalf("large conversation continuation: %+v, %v", result, err)
	}
}

func TestTraeReadsLargeNotificationAndFinalFrame(t *testing.T) {
	text := strings.Repeat("x", 9*1024*1024)
	wire := "\r\n" + `{"method":"item/agentMessage/delta","params":{"delta":"` + text + `"}}` + "\r\n" + `{"id":4,"result":{}}`
	p := &traeProcess{ctx: context.Background(), reader: bufio.NewReader(strings.NewReader(wire))}
	frame, err := p.readWire()
	if err != nil {
		t.Fatal(err)
	}
	var params struct {
		Delta string `json:"delta"`
	}
	if err := json.Unmarshal(frame.Params, &params); err != nil || params.Delta != text {
		t.Fatalf("large notification lost: length=%d, err=%v", len(params.Delta), err)
	}
	frame, err = p.readWire()
	if err != nil || string(frame.ID) != "4" {
		t.Fatalf("final frame without newline: %+v, %v", frame, err)
	}
	if _, err := p.readWire(); err == nil || !strings.Contains(err.Error(), "closed before response") {
		t.Fatalf("EOF: %v", err)
	}
}

func TestTraeRejectsTruncatedFrame(t *testing.T) {
	p := &traeProcess{ctx: context.Background(), reader: bufio.NewReader(strings.NewReader(`{"id":4`))}
	if _, err := p.readWire(); err == nil || !strings.Contains(err.Error(), "invalid TRAE app-server frame") {
		t.Fatalf("truncated JSON: %v", err)
	}
}
