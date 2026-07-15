package worker

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/openmodu/oneshot/internal/agentrun"
)

type Engine interface {
	Available(agentrun.Runtime) bool
	Run(ctx context.Context, request agentrun.Request, sink agentrun.Sink) (agentrun.Result, error)
}

// defaultMaxConcurrency caps simultaneous runs when the operator does not set
// one. A remote worker is usually a single dev machine; a small cap keeps a
// burst of dispatches from spawning unbounded long-lived agent processes.
const defaultMaxConcurrency = 4

var runIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

type Server struct {
	id         string
	name       string
	token      string
	workspaces map[string]string
	locks      map[string]*sync.RWMutex
	engine     Engine
	slots      chan struct{}

	mu      sync.Mutex
	running map[string]context.CancelFunc
}

// NewServer builds a worker HTTP server. maxConcurrency <= 0 uses
// defaultMaxConcurrency.
func NewServer(id, name, token string, workspaces map[string]string, engine Engine, maxConcurrency int) *Server {
	if maxConcurrency <= 0 {
		maxConcurrency = defaultMaxConcurrency
	}
	locks := make(map[string]*sync.RWMutex, len(workspaces))
	for workspaceID := range workspaces {
		locks[workspaceID] = &sync.RWMutex{}
	}
	return &Server{
		id:         id,
		name:       name,
		token:      token,
		workspaces: workspaces,
		locks:      locks,
		engine:     engine,
		slots:      make(chan struct{}, maxConcurrency),
		running:    make(map[string]context.CancelFunc),
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", s.authorize(s.health))
	mux.HandleFunc("POST /v1/execute", s.authorize(s.execute))
	mux.HandleFunc("POST /v1/runs/{runID}/interrupt", s.authorize(s.interrupt))
	return mux
}

func (s *Server) authorize(next http.HandlerFunc) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		provided := strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer ")
		if provided == "" || len(provided) != len(s.token) || subtle.ConstantTimeCompare([]byte(provided), []byte(s.token)) != 1 {
			writeError(writer, http.StatusUnauthorized, "worker_unauthorized", "invalid worker token")
			return
		}
		next(writer, request)
	}
}

func (s *Server) health(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, http.StatusOK, Health{WorkerID: s.id, Name: s.name, Runtimes: map[string]bool{"codex": s.engine.Available(agentrun.RuntimeCodex), "claude": s.engine.Available(agentrun.RuntimeClaude), "modu": s.engine.Available(agentrun.RuntimeModu)}})
}

// execute streams the run as NDJSON: one Frame per line. Pre-flight failures
// (bad request, unknown workspace/runtime, no free slot) are ordinary HTTP
// errors sent before the stream opens. Once the run starts, the response is a
// committed 200 stream, so a mid-run failure arrives as a terminal Error frame
// rather than an HTTP status.
func (s *Server) execute(writer http.ResponseWriter, request *http.Request) {
	var input ExecuteRequest
	decoder := json.NewDecoder(io.LimitReader(request.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeError(writer, http.StatusBadRequest, "worker_invalid_request", "invalid execute request")
		return
	}
	if !runIDPattern.MatchString(input.RunID) {
		writeError(writer, http.StatusBadRequest, "worker_invalid_request", "run id is missing or malformed")
		return
	}
	workspace, ok := s.workspaces[input.WorkspaceID]
	if !ok {
		writeError(writer, http.StatusConflict, "worker_workspace_unmapped", "workspace is not mapped on this worker")
		return
	}
	if !s.engine.Available(input.Runtime) {
		writeError(writer, http.StatusConflict, "worker_runtime_unavailable", "runtime is unavailable on this worker")
		return
	}
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		writeError(writer, http.StatusTooManyRequests, "worker_busy", "worker is at capacity")
		return
	}

	flusher, ok := writer.(http.Flusher)
	if !ok {
		writeError(writer, http.StatusInternalServerError, "worker_execution_failed", "streaming is unsupported")
		return
	}

	// Register a cancellable context so an interrupt call can stop this run.
	runCtx, cancel := context.WithCancel(request.Context())
	defer cancel()
	s.track(input.RunID, cancel)
	defer s.untrack(input.RunID)

	workspaceLock := s.locks[input.WorkspaceID]
	if input.Sandbox == agentrun.SandboxReadOnly {
		workspaceLock.RLock()
		defer workspaceLock.RUnlock()
	} else {
		workspaceLock.Lock()
		defer workspaceLock.Unlock()
	}

	writer.Header().Set("Content-Type", "application/x-ndjson")
	writer.WriteHeader(http.StatusOK)
	flusher.Flush()

	encoder := json.NewEncoder(writer)
	var writeErr error
	send := func(frame Frame) {
		if writeErr != nil {
			return
		}
		if writeErr = encoder.Encode(frame); writeErr == nil {
			flusher.Flush()
		}
	}

	result, runErr := s.engine.Run(runCtx, agentrun.Request{
		Runtime:         input.Runtime,
		Workspace:       workspace,
		Prompt:          input.Prompt,
		Model:           input.Model,
		Sandbox:         input.Sandbox,
		ResumeSessionID: input.ResumeSessionID,
		InterruptGrace:  time.Duration(input.InterruptGraceSeconds) * time.Second,
	}, func(event agentrun.Event) {
		e := event
		send(Frame{Event: &e})
	})

	switch {
	case request.Context().Err() != nil:
		// The client hung up; nothing left to tell it.
		return
	case runErr != nil && errors.Is(runErr, context.Canceled):
		// Graceful interrupt: hand back whatever the agent produced before it stopped.
		send(Frame{Result: &result})
	case runErr != nil:
		send(Frame{Error: &RemoteError{Code: "worker_execution_failed", Message: "remote agent execution failed"}})
	default:
		send(Frame{Result: &result})
	}
}

func (s *Server) interrupt(writer http.ResponseWriter, request *http.Request) {
	runID := request.PathValue("runID")
	s.mu.Lock()
	cancel := s.running[runID]
	s.mu.Unlock()
	if cancel == nil {
		writeError(writer, http.StatusNotFound, "worker_run_not_found", "no such in-flight run")
		return
	}
	cancel()
	writer.WriteHeader(http.StatusNoContent)
}

func (s *Server) track(runID string, cancel context.CancelFunc) {
	s.mu.Lock()
	s.running[runID] = cancel
	s.mu.Unlock()
}

func (s *Server) untrack(runID string) {
	s.mu.Lock()
	delete(s.running, runID)
	s.mu.Unlock()
}

func writeError(writer http.ResponseWriter, status int, code, message string) {
	writeJSON(writer, status, RemoteError{Code: code, Message: message})
}
func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}
