package worker

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

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
	TurnCount      int              `json:"turnCount,omitempty"`
	Status         string           `json:"status"`
	Shared         bool             `json:"shared,omitempty"`
	Events         []agentrun.Event `json:"events,omitempty"`
	// EventsTotal is the length of the whole transcript and EventsOffset the
	// index Events starts at, so a phone that was handed the newest page can
	// tell how much history is still on the host and ask for the page before.
	EventsTotal  int              `json:"eventsTotal,omitempty"`
	EventsOffset int              `json:"eventsOffset,omitempty"`
	Result       *agentrun.Result `json:"result,omitempty"`
	Error        string           `json:"error,omitempty"`
	StartedAt    time.Time        `json:"startedAt"`
	FinishedAt   *time.Time       `json:"finishedAt,omitempty"`
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

// TranscriptWindow asks for one page of a run's transcript, counted from the
// start of the transcript so offsets stay valid while a run keeps appending.
type TranscriptWindow struct {
	// Limit bounds the page; 0 asks for the whole transcript.
	Limit int
	// Before is the exclusive end of the page. Zero or less asks for the
	// newest page, which is what opening a conversation wants.
	Before int
}

// MaxSharedEventText caps one tool event's text on its way to a remote client.
// A file edit's result can be 150 KB — the whole file, echoed back — and a page
// of two hundred of those is megabytes that a phone spends seconds receiving,
// decoding and laying out, to draw a line it keeps collapsed anyway.
const MaxSharedEventText = 4096

// Apply slices the assembled transcript to the requested page, trims tool
// output no remote client will read in full, and reports where the page starts.
func (w TranscriptWindow) Apply(events []agentrun.Event) ([]agentrun.Event, int) {
	end := len(events)
	if w.Before > 0 && w.Before < end {
		end = w.Before
	}
	start := 0
	if w.Limit > 0 && end-w.Limit > 0 {
		start = end - w.Limit
	}
	return trimSharedEvents(events[start:end]), start
}

// trimSharedEvents shortens oversized tool bodies. Prose — the prompt, the
// reply, the reasoning — is what the reader opened the conversation for and is
// never cut. The page is copied only when something actually needs trimming, so
// an ordinary one is passed through untouched.
func trimSharedEvents(events []agentrun.Event) []agentrun.Event {
	trimmed := events
	for i, event := range events {
		if event.Kind != agentrun.KindToolUse && event.Kind != agentrun.KindToolResult {
			continue
		}
		if len(event.Text) <= MaxSharedEventText {
			continue
		}
		if &trimmed[0] == &events[0] {
			trimmed = append([]agentrun.Event{}, events...)
		}
		trimmed[i].Text = truncateEventText(event.Text)
	}
	return trimmed
}

// truncateEventText cuts on a rune boundary and says so, so the reader can tell
// a trimmed tool result from one that really ended there.
func truncateEventText(text string) string {
	cut := MaxSharedEventText
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return text[:cut] + "\n…（输出过长，完整内容在桌面端）"
}

type SharedRuns interface {
	List(context.Context, string) (SharedRunPage, error)
	Get(context.Context, string, TranscriptWindow) (SharedRun, error)
	Start(context.Context, SharedRunInput) (SharedRun, error)
	Import(context.Context, SharedRun) (SharedRun, error)
	Interrupt(context.Context, string) error
	RespondPermission(context.Context, string, string, string) error
	// Rename and Remove act on a conversation, not one of its runs: that is
	// what a phone shows in its list and what the host stores as a task.
	Rename(ctx context.Context, conversationID, title string) error
	Remove(ctx context.Context, conversationID string) error
	// Usage reports each runtime's account quota and recent daily activity, so
	// the phone can show the same board the desktop does. A runtime the host
	// cannot report is left out rather than failing the request.
	Usage(ctx context.Context, refresh bool) ([]agentrun.AccountUsage, error)
}

type ConversationTitle struct {
	Title string `json:"title"`
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
		query := r.URL.Query()
		limit, _ := strconv.Atoi(query.Get("limit"))
		before, _ := strconv.Atoi(query.Get("before"))
		value, err = s.sharedRuns.Get(r.Context(), id, TranscriptWindow{Limit: limit, Before: before})
	case r.Method == http.MethodDelete:
		err = s.sharedRuns.Remove(r.Context(), r.PathValue("conversationID"))
	case strings.HasSuffix(r.URL.Path, "/rename"):
		var input ConversationTitle
		if err = json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input); err == nil {
			err = s.sharedRuns.Rename(r.Context(), r.PathValue("conversationID"), input.Title)
		}
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
func (c *Client) GetSharedRun(ctx context.Context, config Config, id string, window TranscriptWindow) (SharedRun, error) {
	var run SharedRun
	path := "/v1/shared-runs/" + url.PathEscape(id)
	query := url.Values{}
	if window.Limit > 0 {
		query.Set("limit", strconv.Itoa(window.Limit))
	}
	if window.Before > 0 {
		query.Set("before", strconv.Itoa(window.Before))
	}
	if len(query) > 0 {
		path += "?" + query.Encode()
	}
	err := c.do(ctx, config, http.MethodGet, path, nil, &run)
	return run, err
}
func (c *Client) StartSharedRun(ctx context.Context, config Config, input SharedRunInput) (SharedRun, error) {
	var run SharedRun
	err := c.do(ctx, config, http.MethodPost, "/v1/shared-runs", input, &run)
	return run, err
}

// usage answers the phone's usage board. A worker that shares no history has
// nothing to report either, so it answers the same "unavailable" as the rest.
func (s *Server) usage(w http.ResponseWriter, r *http.Request) {
	if s.sharedRuns == nil {
		writeError(w, http.StatusNotImplemented, "shared_runs_unavailable", "this worker does not share task history")
		return
	}
	items, err := s.sharedRuns.Usage(r.Context(), r.URL.Query().Get("refresh") != "")
	if err != nil {
		writeError(w, http.StatusBadRequest, "usage_failed", err.Error())
		return
	}
	if items == nil {
		items = []agentrun.AccountUsage{}
	}
	writeJSON(w, http.StatusOK, items)
}

func (c *Client) SharedUsage(ctx context.Context, config Config, refresh bool) ([]agentrun.AccountUsage, error) {
	var usage []agentrun.AccountUsage
	path := "/v1/usage"
	if refresh {
		path += "?refresh=1"
	}
	err := c.do(ctx, config, http.MethodGet, path, nil, &usage)
	return usage, err
}

func (c *Client) RenameSharedConversation(ctx context.Context, config Config, id, title string) error {
	return c.do(ctx, config, http.MethodPost, "/v1/shared-conversations/"+url.PathEscape(id)+"/rename", ConversationTitle{Title: title}, nil)
}

func (c *Client) RemoveSharedConversation(ctx context.Context, config Config, id string) error {
	return c.do(ctx, config, http.MethodDelete, "/v1/shared-conversations/"+url.PathEscape(id), nil, nil)
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
