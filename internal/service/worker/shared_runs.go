package worker

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

// SharedRun is a projection of the host's durable task record. Clients cache
// it for offline viewing; only the host owns execution and history.
type SharedRun struct {
	ID             string           `json:"id"`
	ConversationID string           `json:"conversationId"`
	WorkerID       string           `json:"workerId"`
	WorkspaceID    string           `json:"workspaceId"`
	Runtime        agentrun.Runtime `json:"runtime"`
	Prompt         string           `json:"prompt"`
	Title          string           `json:"title,omitempty"`
	Status         string           `json:"status"`
	Shared         bool             `json:"shared,omitempty"`
	Events         []agentrun.Event `json:"events,omitempty"`
	Result         *agentrun.Result `json:"result,omitempty"`
	Error          string           `json:"error,omitempty"`
	StartedAt      time.Time        `json:"startedAt"`
	FinishedAt     *time.Time       `json:"finishedAt,omitempty"`
}

type SharedRunInput struct {
	WorkspaceID     string `json:"workspaceId"`
	ConversationID  string `json:"conversationId,omitempty"`
	Runtime         string `json:"runtime"`
	Prompt          string `json:"prompt"`
	Model           string `json:"model,omitempty"`
	ReasoningEffort string `json:"reasoningEffort,omitempty"`
	ServiceTier     string `json:"serviceTier,omitempty"`
}

type SharedRunPage struct {
	Items      []SharedRun `json:"items"`
	NextCursor string      `json:"nextCursor,omitempty"`
}

type SharedRuns interface {
	List(context.Context, string) (SharedRunPage, error)
	Get(context.Context, string) (SharedRun, error)
	Start(context.Context, SharedRunInput) (SharedRun, error)
	Import(context.Context, SharedRun) (SharedRun, error)
	Interrupt(context.Context, string) error
	RespondPermission(context.Context, string, string, string) error
}

func (s *Server) SetSharedRuns(runs SharedRuns) { s.sharedRuns = runs }

func (s *Server) sharedRunsHandler(w http.ResponseWriter, r *http.Request) {
	if s.sharedRuns == nil {
		writeError(w, http.StatusNotImplemented, "shared_runs_unavailable", "this worker does not share task history")
		return
	}
	var value any
	var err error
	id := r.PathValue("runID")
	switch {
	case r.URL.Path == "/v1/shared-runs/import":
		var input SharedRun
		if err = json.NewDecoder(io.LimitReader(r.Body, 16<<20)).Decode(&input); err == nil {
			value, err = s.sharedRuns.Import(r.Context(), input)
		}
	case r.Method == http.MethodGet && id == "":
		value, err = s.sharedRuns.List(r.Context(), r.URL.Query().Get("cursor"))
	case r.Method == http.MethodGet:
		value, err = s.sharedRuns.Get(r.Context(), id)
	case strings.HasSuffix(r.URL.Path, "/interrupt"):
		err = s.sharedRuns.Interrupt(r.Context(), id)
	case strings.Contains(r.URL.Path, "/permissions/"):
		var input PermissionResponse
		if err = json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input); err == nil {
			err = s.sharedRuns.RespondPermission(r.Context(), id, r.PathValue("requestID"), input.Decision)
		}
	default:
		var input SharedRunInput
		decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err = decoder.Decode(&input); err == nil {
			value, err = s.sharedRuns.Start(r.Context(), input)
		}
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, "shared_run_failed", err.Error())
		return
	}
	if value == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (c *Client) ListSharedRuns(ctx context.Context, config Config, cursor string) (SharedRunPage, error) {
	var page SharedRunPage
	err := c.do(ctx, config, http.MethodGet, "/v1/shared-runs?cursor="+url.QueryEscape(cursor), nil, &page)
	return page, err
}
func (c *Client) GetSharedRun(ctx context.Context, config Config, id string) (SharedRun, error) {
	var run SharedRun
	err := c.do(ctx, config, http.MethodGet, "/v1/shared-runs/"+url.PathEscape(id), nil, &run)
	return run, err
}
func (c *Client) StartSharedRun(ctx context.Context, config Config, input SharedRunInput) (SharedRun, error) {
	var run SharedRun
	err := c.do(ctx, config, http.MethodPost, "/v1/shared-runs", input, &run)
	return run, err
}
func (c *Client) InterruptSharedRun(ctx context.Context, config Config, id string) error {
	return c.do(ctx, config, http.MethodPost, "/v1/shared-runs/"+url.PathEscape(id)+"/interrupt", nil, nil)
}
func (c *Client) RespondSharedPermission(ctx context.Context, config Config, id, requestID, decision string) error {
	return c.do(ctx, config, http.MethodPost, "/v1/shared-runs/"+url.PathEscape(id)+"/permissions/"+url.PathEscape(requestID), PermissionResponse{Decision: decision}, nil)
}

func (c *Client) ImportSharedRun(ctx context.Context, config Config, input SharedRun) (SharedRun, error) {
	var run SharedRun
	err := c.do(ctx, config, http.MethodPost, "/v1/shared-runs/import", input, &run)
	return run, err
}
