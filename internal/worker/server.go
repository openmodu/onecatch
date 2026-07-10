package worker

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/openmodu/oneshot/internal/agentrun"
)

type Engine interface {
	Available(agentrun.Runtime) bool
	Run(ctx context.Context, request agentrun.Request, sink agentrun.Sink) (agentrun.Result, error)
}

type Server struct {
	id         string
	name       string
	token      string
	workspaces map[string]string
	locks      map[string]*sync.RWMutex
	engine     Engine
}

func NewServer(id, name, token string, workspaces map[string]string, engine Engine) *Server {
	locks := make(map[string]*sync.RWMutex, len(workspaces))
	for workspaceID := range workspaces {
		locks[workspaceID] = &sync.RWMutex{}
	}
	return &Server{id: id, name: name, token: token, workspaces: workspaces, locks: locks, engine: engine}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", s.authorize(s.health))
	mux.HandleFunc("POST /v1/execute", s.authorize(s.execute))
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
	writeJSON(writer, http.StatusOK, Health{WorkerID: s.id, Name: s.name, Runtimes: map[string]bool{"codex": s.engine.Available(agentrun.RuntimeCodex), "claude": s.engine.Available(agentrun.RuntimeClaude)}})
}

func (s *Server) execute(writer http.ResponseWriter, request *http.Request) {
	var input ExecuteRequest
	decoder := json.NewDecoder(io.LimitReader(request.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeError(writer, http.StatusBadRequest, "worker_invalid_request", "invalid execute request")
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
	workspaceLock := s.locks[input.WorkspaceID]
	if input.Sandbox == agentrun.SandboxReadOnly {
		workspaceLock.RLock()
		defer workspaceLock.RUnlock()
	} else {
		workspaceLock.Lock()
		defer workspaceLock.Unlock()
	}
	events := make([]agentrun.Event, 0, 32)
	result, err := s.engine.Run(request.Context(), agentrun.Request{Runtime: input.Runtime, Workspace: workspace, Prompt: input.Prompt, Model: input.Model, Sandbox: input.Sandbox, ResumeSessionID: input.ResumeSessionID}, func(event agentrun.Event) { events = append(events, event) })
	if err != nil {
		if errors.Is(err, request.Context().Err()) {
			return
		}
		writeError(writer, http.StatusInternalServerError, "worker_execution_failed", "remote agent execution failed")
		return
	}
	writeJSON(writer, http.StatusOK, ExecuteResponse{Result: result, Events: events})
}

func writeError(writer http.ResponseWriter, status int, code, message string) {
	writeJSON(writer, status, RemoteError{Code: code, Message: message})
}
func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}
