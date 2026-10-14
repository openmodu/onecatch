package agentrun

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"testing"
	"time"
)

type codexQuestionHarness struct {
	conn   net.Conn
	scan   *bufio.Scanner
	events chan Event
	result chan error
	cancel context.CancelFunc
}

func newCodexQuestionHarness(t *testing.T, handler func(context.Context, UserInputRequest) (UserInputResponse, error)) *codexQuestionHarness {
	t.Helper()
	client, server := net.Pipe()
	_ = server.SetDeadline(time.Now().Add(10 * time.Second))
	ctx, cancel := context.WithCancel(context.Background())
	h := &codexQuestionHarness{conn: server, scan: bufio.NewScanner(server), events: make(chan Event, 100), result: make(chan error, 1), cancel: cancel}
	go func() {
		_, err := RunCodexStream(ctx, Request{UserInputHandler: handler}, client, func(e Event) { h.events <- e })
		h.result <- err
	}()
	t.Cleanup(func() { cancel(); client.Close(); server.Close() })
	for _, frame := range []string{`{"id":1,"result":{}}`, `{"id":2,"result":{"data":[]}}`, `{"id":3,"result":{"thread":{"id":"thread-1"}}}`, `{"id":4,"result":{"turn":{"id":"turn-1"}}}`} {
		h.read(t)
		h.send(t, frame)
		if strings.Contains(frame, `"id":1,`) {
			h.read(t)
		} // initialized notification
	}
	return h
}
func (h *codexQuestionHarness) send(t *testing.T, s string) {
	t.Helper()
	if _, err := h.conn.Write([]byte(s + "\n")); err != nil {
		t.Fatal(err)
	}
}
func (h *codexQuestionHarness) read(t *testing.T) map[string]json.RawMessage {
	t.Helper()
	if !h.scan.Scan() {
		t.Fatalf("missing protocol output: %v", h.scan.Err())
	}
	var out map[string]json.RawMessage
	if err := json.Unmarshal(h.scan.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}
func (h *codexQuestionHarness) event(t *testing.T, kind EventKind) Event {
	t.Helper()
	for {
		select {
		case e := <-h.events:
			if e.Kind == kind {
				return e
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("missing event %s", kind)
		}
	}
}
func (h *codexQuestionHarness) finish(t *testing.T) {
	t.Helper()
	h.send(t, `{"method":"turn/completed","params":{"threadId":"thread-1","turn":{"id":"turn-1","status":"completed"}}}`)
	select {
	case err := <-h.result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("turn did not finish")
	}
}

const codexTestQuestion = `{"id":21,"method":"item/tool/requestUserInput","params":{"threadId":"thread-1","turnId":"turn-1","itemId":"ask-1","isBlocking":true,"questions":[{"id":"scope","header":"Scope","question":"Which scope?","options":[{"label":"small","description":"one change"}]}]}}`

func TestCodexQuestionsReplyWithoutBlockingNotifications(t *testing.T) {
	for _, blocking := range []bool{true, false} {
		t.Run(map[bool]string{true: "blocking", false: "async"}[blocking], func(t *testing.T) {
			answers := make(chan UserInputResponse, 1)
			h := newCodexQuestionHarness(t, func(ctx context.Context, r UserInputRequest) (UserInputResponse, error) {
				select {
				case a := <-answers:
					return a, nil
				case <-ctx.Done():
					return UserInputResponse{}, ctx.Err()
				}
			})
			frame := codexTestQuestion
			if !blocking {
				frame = strings.Replace(frame, `"isBlocking":true`, `"isBlocking":false`, 1)
			}
			h.send(t, frame)
			e := h.event(t, KindUserInputRequest)
			if e.UserInput.Async == blocking || e.UserInput.Questions[0].Options[0].Description != "one change" {
				t.Fatalf("request: %+v", e.UserInput)
			}
			h.send(t, `{"method":"item/agentMessage/delta","params":{"threadId":"thread-1","turnId":"turn-1","itemId":"msg","delta":"still working"}}`)
			h.event(t, KindMessage) // implicit stream start
			if e := h.event(t, KindMessage); e.Text != "still working" {
				t.Fatalf("event: %+v", e)
			}
			answers <- UserInputResponse{Answers: map[string]string{"scope": "custom scope"}}
			out := h.read(t)
			if string(out["id"]) != "21" || string(out["result"]) != `{"answers":{"scope":{"answers":["custom scope"]}}}` {
				t.Fatalf("reply: %s", out)
			}
			e = h.event(t, KindUserInputResolved)
			if e.UserInputResponse.Answers["scope"] != "custom scope" {
				t.Fatalf("resolved: %+v", e)
			}
			h.finish(t)
		})
	}
}
func TestCodexQuestionServerCleanupCancelsWaiter(t *testing.T) {
	stopped := make(chan struct{})
	h := newCodexQuestionHarness(t, func(ctx context.Context, r UserInputRequest) (UserInputResponse, error) {
		<-ctx.Done()
		close(stopped)
		return UserInputResponse{}, ctx.Err()
	})
	h.send(t, codexTestQuestion)
	h.event(t, KindUserInputRequest)
	h.send(t, `{"method":"serverRequest/resolved","params":{"threadId":"another-thread","requestId":21}}`)
	h.send(t, `{"method":"serverRequest/resolved","params":{"threadId":"thread-1","requestId":21}}`)
	e := h.event(t, KindUserInputResolved)
	if !e.UserInputResponse.Closed || e.UserInputResponse.Cancelled {
		t.Fatalf("cleanup: %+v", e.UserInputResponse)
	}
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("waiter leaked")
	}
	h.finish(t)
}
func TestCodexAsyncMessageAnswersSteerAndWaitForAck(t *testing.T) {
	answers := make(chan UserInputResponse, 1)
	h := newCodexQuestionHarness(t, func(ctx context.Context, r UserInputRequest) (UserInputResponse, error) {
		select {
		case a := <-answers:
			return a, nil
		case <-ctx.Done():
			return UserInputResponse{}, ctx.Err()
		}
	})
	frame := `{"method":"item/completed","params":{"threadId":"thread-1","turnId":"turn-1","item":{"id":"async-1","type":"agentMessage","text":"Pick one","delivery":"async","questions":[{"title":"Which scope?","options":["small","large"]}]}}}`
	h.send(t, frame)
	e := h.event(t, KindUserInputRequest)
	if !e.UserInput.Async || e.UserInput.Questions[0].Question != "Which scope?" {
		t.Fatalf("async input: %+v", e.UserInput)
	}
	h.send(t, frame)                                                                  // completed snapshots may be repeated
	h.send(t, `{"method":"serverRequest/resolved","params":{"threadId":"thread-1"}}`) // malformed cleanup cannot close message questions
	delivered := make(chan error, 1)
	answers <- UserInputResponse{Answers: map[string]string{"question-1": "large"}, Delivered: delivered}
	out := h.read(t)
	if string(out["method"]) != `"turn/steer"` || !strings.Contains(string(out["params"]), `"expectedTurnId":"turn-1"`) || !strings.Contains(string(out["params"]), "large") {
		t.Fatalf("steer: %s", out)
	}
	select {
	case <-delivered:
		t.Fatal("answer acknowledged before server accepted it")
	default:
	}
	h.send(t, `{"id":`+string(out["id"])+`,"result":{"turnId":"turn-1"}}`)
	if err := <-delivered; err != nil {
		t.Fatal(err)
	}
	h.event(t, KindUserInputResolved)
	h.finish(t)
	for len(h.events) > 0 {
		if (<-h.events).Kind == KindUserInputRequest {
			t.Fatal("duplicate question card")
		}
	}
}
func TestCodexQuestionDismissalAndFailureReturnNoAnswers(t *testing.T) {
	for _, fail := range []bool{false, true} {
		h := newCodexQuestionHarness(t, func(context.Context, UserInputRequest) (UserInputResponse, error) {
			if fail {
				return UserInputResponse{Answers: map[string]string{"scope": "must not send"}}, errors.New("failed")
			}
			return UserInputResponse{Cancelled: true}, nil
		})
		h.send(t, codexTestQuestion)
		out := h.read(t)
		if string(out["result"]) != `{"answers":{}}` {
			t.Fatalf("dismissal: %s", out)
		}
		h.finish(t)
	}
}
func TestCodexTurnEndClosesUnansweredCards(t *testing.T) {
	h := newCodexQuestionHarness(t, func(ctx context.Context, r UserInputRequest) (UserInputResponse, error) {
		<-ctx.Done()
		return UserInputResponse{}, ctx.Err()
	})
	h.send(t, codexTestQuestion)
	h.event(t, KindUserInputRequest)
	h.finish(t)
	e := h.event(t, KindUserInputResolved)
	if !e.UserInputResponse.Closed {
		t.Fatal("pending card survived turn end")
	}
}

func TestCodexQuestionsRejectInvalidAndStaleRequests(t *testing.T) {
	for _, frame := range []string{
		strings.Replace(codexTestQuestion, `"turn-1"`, `"old-turn"`, 1),
		strings.Replace(codexTestQuestion, `"thread-1"`, `"other-thread"`, 1),
		strings.Replace(codexTestQuestion, `"id":"scope"`, `"id":""`, 1),
	} {
		h := newCodexQuestionHarness(t, func(context.Context, UserInputRequest) (UserInputResponse, error) {
			t.Error("invalid request reached host")
			return UserInputResponse{}, nil
		})
		h.send(t, frame)
		out := h.read(t)
		if string(out["id"]) != "21" {
			t.Fatalf("request id lost: %s", out)
		}
		if strings.Contains(frame, `"id":""`) {
			if out["error"] == nil {
				t.Fatal("malformed request accepted")
			}
		} else if string(out["result"]) != `{"answers":{}}` {
			t.Fatalf("stale request: %s", out)
		}
		h.finish(t)
	}
}
func TestCodexSecretAnswerIsSentButNotPersisted(t *testing.T) {
	h := newCodexQuestionHarness(t, func(context.Context, UserInputRequest) (UserInputResponse, error) {
		return UserInputResponse{Answers: map[string]string{"scope": "private-answer"}}, nil
	})
	h.send(t, strings.Replace(codexTestQuestion, `"header":"Scope"`, `"header":"Scope","isSecret":true`, 1))
	out := h.read(t)
	if !strings.Contains(string(out["result"]), "private-answer") {
		t.Fatal("answer not delivered")
	}
	e := h.event(t, KindUserInputResolved)
	if len(e.UserInputResponse.Answers) != 0 {
		t.Fatal("secret persisted")
	}
	h.finish(t)
}
func TestCodexAsyncSteerRejectionIsNotReportedAsAnswered(t *testing.T) {
	delivered := make(chan error, 1)
	h := newCodexQuestionHarness(t, func(context.Context, UserInputRequest) (UserInputResponse, error) {
		return UserInputResponse{Answers: map[string]string{"question-1": "answer"}, Delivered: delivered}, nil
	})
	h.send(t, `{"method":"item/completed","params":{"threadId":"thread-1","turnId":"turn-1","item":{"id":"async-1","type":"agentMessage","questions":[{"title":"Scope?"}]}}}`)
	out := h.read(t)
	h.send(t, `{"id":`+string(out["id"])+`,"error":{"code":-32600,"message":"turn changed"}}`)
	if err := <-delivered; err == nil {
		t.Fatal("rejected answer reported success")
	}
	if e := h.event(t, KindUserInputResolved); !e.UserInputResponse.Closed || len(e.UserInputResponse.Answers) > 0 {
		t.Fatalf("rejection: %+v", e)
	}
	h.finish(t)
}
