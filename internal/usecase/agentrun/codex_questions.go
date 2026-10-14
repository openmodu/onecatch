package agentrun

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

func (*CodexRunner) SupportsInteractiveUserInput() bool { return true }

type codexRead struct {
	line string
	err  error
}

// One reader per connection, including warm turns. Only the turn loop writes
// protocol responses and emits events; waiting for a person never stops reads.
func (p *codexAppProcess) readLines() <-chan codexRead {
	p.readOnce.Do(func() {
		p.reads = make(chan codexRead, 16)
		p.readStop = make(chan struct{})
		go func() {
			defer close(p.reads)
			for p.scanner.Scan() {
				select {
				case p.reads <- codexRead{line: p.scanner.Text()}:
				case <-p.readStop:
					return
				}
			}
			if err := p.scanner.Err(); err != nil {
				select {
				case p.reads <- codexRead{err: err}:
				case <-p.readStop:
				}
			}
		}()
	})
	return p.reads
}

type codexAsyncQuestion struct {
	Title   string   `json:"title"`
	Options []string `json:"options"`
}
type codexInputQuestion struct {
	ID       string            `json:"id"`
	Header   string            `json:"header"`
	Question string            `json:"question"`
	IsSecret bool              `json:"isSecret"`
	Options  []UserInputOption `json:"options"`
}
type codexQuestionPending struct {
	request      UserInputRequest
	rpcID        json.RawMessage
	thread, turn string
	cancel       context.CancelFunc
	response     *UserInputResponse
}
type codexQuestionAnswer struct {
	id       string
	response UserInputResponse
	err      error
}
type codexQuestions struct {
	ctx     context.Context
	req     Request
	encoder *json.Encoder
	sink    Sink
	now     func() time.Time
	pending map[string]*codexQuestionPending
	seen    map[string]bool
	acks    map[string]string
	answers chan codexQuestionAnswer
	wg      sync.WaitGroup
}

func newCodexQuestions(ctx context.Context, req Request, encoder *json.Encoder, sink Sink, now func() time.Time) *codexQuestions {
	return &codexQuestions{ctx: ctx, req: req, encoder: encoder, sink: sink, now: now, pending: map[string]*codexQuestionPending{}, seen: map[string]bool{}, acks: map[string]string{}, answers: make(chan codexQuestionAnswer, 16)}
}
func (q *codexQuestions) request(e codexAppEnvelope, thread, turn, raw string) error {
	var p struct {
		ThreadID         string               `json:"threadId"`
		TurnID           string               `json:"turnId"`
		ItemID           string               `json:"itemId"`
		Questions        []codexInputQuestion `json:"questions"`
		IsBlocking       *bool                `json:"isBlocking"`
		AutoResolutionMs *int64               `json:"autoResolutionMs"`
	}
	if json.Unmarshal(e.Params, &p) != nil || p.ThreadID == "" || p.TurnID == "" || p.ItemID == "" || len(p.Questions) == 0 {
		return q.encoder.Encode(map[string]any{"id": e.ID, "error": map[string]any{"code": -32602, "message": "invalid Codex user input request"}})
	}
	input := UserInputRequest{ID: fmt.Sprintf("codex-input/%s/%s/%s", thread, turn, e.ID)}
	// Old servers omitted isBlocking. Never infer a default answer from timeout.
	input.Async = p.IsBlocking != nil && !*p.IsBlocking
	ids := map[string]bool{}
	for _, question := range p.Questions {
		if question.ID == "" || strings.TrimSpace(question.Question) == "" || ids[question.ID] {
			return q.encoder.Encode(map[string]any{"id": e.ID, "error": map[string]any{"code": -32602, "message": "invalid Codex question"}})
		}
		ids[question.ID] = true
		input.Questions = append(input.Questions, UserInputQuestion{ID: question.ID, Header: question.Header, Question: question.Question, Options: question.Options, IsSecret: question.IsSecret})
	}
	if p.ThreadID != thread || p.TurnID != turn || q.req.UserInputHandler == nil || q.ctx.Err() != nil {
		return q.encoder.Encode(map[string]any{"id": e.ID, "result": map[string]any{"answers": map[string]any{}}})
	}
	q.start(input, e.ID, thread, turn, raw)
	return nil
}
func (q *codexQuestions) start(input UserInputRequest, rpcID json.RawMessage, thread, turn, raw string) {
	if q.seen[input.ID] {
		return
	}
	q.seen[input.ID] = true
	input.WaitForDelivery = true
	ctx, cancel := context.WithCancel(q.ctx)
	q.pending[input.ID] = &codexQuestionPending{request: input, rpcID: rpcID, thread: thread, turn: turn, cancel: cancel}
	q.sink(Event{Kind: KindUserInputRequest, UserInput: &input, Raw: raw, At: q.now()})
	q.wg.Add(1)
	go func() {
		defer q.wg.Done()
		response, err := q.req.UserInputHandler(ctx, input)
		select {
		case q.answers <- codexQuestionAnswer{id: input.ID, response: response, err: err}:
		case <-ctx.Done():
			deliverCodexAnswer(response, ctx.Err())
		}
	}()
}
func (q *codexQuestions) notification(e codexAppEnvelope, thread, turn, raw string) {
	if e.Method == "serverRequest/resolved" {
		var p struct {
			ThreadID  string          `json:"threadId"`
			RequestID json.RawMessage `json:"requestId"`
		}
		if json.Unmarshal(e.Params, &p) == nil && p.ThreadID == thread && len(p.RequestID) > 0 && string(p.RequestID) != "null" {
			for id, pending := range q.pending {
				if len(pending.rpcID) > 0 && string(pending.rpcID) == string(p.RequestID) {
					q.finish(id, UserInputResponse{Closed: true}, errors.New("Codex request was closed"))
				}
			}
		}
	}
	if e.Method != "item/completed" || q.req.UserInputHandler == nil {
		return
	}
	var p codexAppParams
	if json.Unmarshal(e.Params, &p) != nil || p.Item.Type != "agentMessage" || len(p.Item.Questions) == 0 || p.Item.ID == "" {
		return
	}
	input := UserInputRequest{ID: "codex-message-input/" + thread + "/" + turn + "/" + p.Item.ID, Async: true}
	for i, question := range p.Item.Questions {
		if strings.TrimSpace(question.Title) == "" {
			return
		}
		item := UserInputQuestion{ID: fmt.Sprintf("question-%d", i+1), Question: question.Title}
		for _, option := range question.Options {
			item.Options = append(item.Options, UserInputOption{Label: option})
		}
		input.Questions = append(input.Questions, item)
	}
	q.start(input, nil, thread, turn, raw)
}
func (q *codexQuestions) answer(a codexQuestionAnswer) error {
	p := q.pending[a.id]
	if p == nil {
		deliverCodexAnswer(a.response, errors.New("Codex request is no longer pending"))
		return nil
	}
	if a.err != nil || q.ctx.Err() != nil {
		a.response.Cancelled = true
		a.response.Answers = nil
	}
	if !a.response.Cancelled {
		for _, question := range p.request.Questions {
			if strings.TrimSpace(a.response.Answers[question.ID]) == "" {
				a.response.Cancelled = true
				a.response.Answers = nil
				break
			}
		}
	}
	p.response = &a.response
	if len(p.rpcID) > 0 {
		answers := map[string]any{}
		if !a.response.Cancelled {
			for _, question := range p.request.Questions {
				answers[question.ID] = map[string]any{"answers": []string{a.response.Answers[question.ID]}}
			}
		}
		err := q.encoder.Encode(map[string]any{"id": p.rpcID, "result": map[string]any{"answers": answers}})
		if err != nil {
			q.finish(a.id, UserInputResponse{Closed: true}, err)
		} else {
			q.finish(a.id, a.response, nil)
		}
		return err
	}
	if a.response.Cancelled {
		q.finish(a.id, a.response, nil)
		return nil
	}
	var text strings.Builder
	for _, question := range p.request.Questions {
		fmt.Fprintf(&text, "%s\n%s\n\n", question.Question, a.response.Answers[question.ID])
	}
	id := "onecatch-user-input/" + a.id
	q.acks[id] = a.id
	err := q.encoder.Encode(map[string]any{"id": id, "method": "turn/steer", "params": map[string]any{"threadId": p.thread, "expectedTurnId": p.turn, "input": []map[string]any{{"type": "text", "text": strings.TrimSpace(text.String())}}}})
	if err != nil {
		q.finish(a.id, UserInputResponse{Closed: true}, err)
	}
	return err
}
func (q *codexQuestions) ack(e codexAppEnvelope) bool {
	if e.Method != "" {
		return false
	}
	var id string
	if json.Unmarshal(e.ID, &id) != nil {
		return false
	}
	pendingID, ok := q.acks[id]
	if !ok {
		return false
	}
	delete(q.acks, id)
	if p := q.pending[pendingID]; p != nil && p.response != nil {
		var err error
		response := *p.response
		if len(e.Error) > 0 {
			err = fmt.Errorf("Codex rejected answer: %s", e.Error)
			response = UserInputResponse{Closed: true}
		}
		q.finish(pendingID, response, err)
	}
	return true
}
func deliverCodexAnswer(response UserInputResponse, err error) {
	if response.Delivered != nil {
		select {
		case response.Delivered <- err:
		default:
		}
	}
}
func (q *codexQuestions) finish(id string, response UserInputResponse, err error) {
	p := q.pending[id]
	if p == nil {
		return
	}
	delete(q.pending, id)
	if p.response != nil {
		deliverCodexAnswer(*p.response, err)
	}
	p.cancel()
	// Secret answers are delivered to Codex but not written to the transcript.
	response.Answers = cloneCodexAnswers(response.Answers)
	for _, question := range p.request.Questions {
		if question.IsSecret {
			delete(response.Answers, question.ID)
		}
	}
	q.sink(Event{Kind: KindUserInputResolved, UserInput: &p.request, UserInputResponse: &response, At: q.now()})
}
func cloneCodexAnswers(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
func (q *codexQuestions) close() {
	for id := range q.pending {
		q.finish(id, UserInputResponse{Closed: true}, errors.New("Codex turn ended"))
	}
	q.wg.Wait()
	for {
		select {
		case a := <-q.answers:
			deliverCodexAnswer(a.response, errors.New("Codex turn ended"))
		default:
			return
		}
	}
}
