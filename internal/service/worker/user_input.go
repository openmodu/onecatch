package worker

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

type pendingUserInput struct {
	request  agentrun.UserInputRequest
	response chan agentrun.UserInputResponse
	closed   chan struct{}
	once     sync.Once
	answered bool
}

func (s *Server) registerUserInput(state *runState, request agentrun.UserInputRequest) *pendingUserInput {
	s.mu.Lock()
	defer s.mu.Unlock()
	if state.questions == nil {
		state.questions = map[string]*pendingUserInput{}
	}
	if p := state.questions[request.ID]; p != nil {
		return p
	}
	p := &pendingUserInput{request: request, response: make(chan agentrun.UserInputResponse, 1), closed: make(chan struct{})}
	state.questions[request.ID] = p
	return p
}
func (s *Server) closeUserInput(state *runState, id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p := state.questions[id]; p != nil {
		p.once.Do(func() { close(p.closed) })
		delete(state.questions, id)
	}
}
func (s *Server) awaitUserInput(ctx context.Context, state *runState, request agentrun.UserInputRequest) (agentrun.UserInputResponse, error) {
	if err := ctx.Err(); err != nil {
		return agentrun.UserInputResponse{}, err
	}
	p := s.registerUserInput(state, request)
	select {
	case response := <-p.response:
		return response, nil
	case <-ctx.Done():
		s.closeUserInput(state, request.ID)
		return agentrun.UserInputResponse{}, ctx.Err()
	}
}
func (s *Server) respondUserInput(w http.ResponseWriter, r *http.Request) {
	var response agentrun.UserInputResponse
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&response); err != nil || response.Closed {
		writeError(w, http.StatusBadRequest, "user_input_invalid", "invalid user input response")
		return
	}
	s.mu.Lock()
	var p *pendingUserInput
	if state := s.running[r.PathValue("runID")]; state != nil {
		p = state.questions[r.PathValue("requestID")]
	}
	if p == nil || p.answered {
		s.mu.Unlock()
		writeError(w, http.StatusConflict, "user_input_not_pending", "user input request is no longer pending")
		return
	}
	if response.Cancelled {
		response.Answers = nil
	} else {
		answers := map[string]string{}
		for _, q := range p.request.Questions {
			answer := strings.TrimSpace(response.Answers[q.ID])
			if answer == "" {
				s.mu.Unlock()
				writeError(w, http.StatusBadRequest, "user_input_incomplete", "every question requires an answer")
				return
			}
			answers[q.ID] = answer
		}
		response.Answers = answers
	}
	if p.request.WaitForDelivery {
		response.Delivered = make(chan error, 1)
	}
	p.answered = true
	p.response <- response
	s.mu.Unlock()
	if response.Delivered != nil {
		select {
		case err := <-response.Delivered:
			if err != nil {
				writeError(w, http.StatusConflict, "user_input_delivery_failed", err.Error())
				return
			}
		case <-p.closed:
			select {
			case err := <-response.Delivered:
				if err != nil {
					writeError(w, http.StatusConflict, "user_input_delivery_failed", err.Error())
					return
				}
			default:
				writeError(w, http.StatusConflict, "user_input_not_pending", "user input request was closed")
				return
			}
		case <-r.Context().Done():
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}
func (c *Client) RespondUserInput(ctx context.Context, config Config, runID, requestID string, response agentrun.UserInputResponse) error {
	return c.do(ctx, config, http.MethodPost, "/v1/runs/"+url.PathEscape(runID)+"/user-input/"+url.PathEscape(requestID), response, nil)
}
