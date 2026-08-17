// Package execution is the long-horizon worker: it picks up running orders and
// drives a real local agent (Codex / Claude Code) to completion, streaming the
// agent's progress and turning the files it produces into the order's
// deliverables. Tasks can run in a managed per-order workspace or in a
// user-supplied directory, and can be resumed for multi-turn continuations.
package execution

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	domainagents "github.com/openmodu/onecatch/internal/domain/agents"
	domainartifacts "github.com/openmodu/onecatch/internal/domain/artifacts"
	domainorders "github.com/openmodu/onecatch/internal/domain/orders"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

// OrderRepository is the slice of order persistence the worker needs.
type OrderRepository interface {
	ListOrdersByStatus(context.Context, domainorders.Status, int) ([]domainorders.Order, error)
	TransitionOrder(ctx context.Context, order domainorders.Order, from domainorders.Status) error
}

// AgentResolver resolves the agent definition (runtime, persona, sandbox) for
// an order.
type AgentResolver interface {
	GetAgent(context.Context, string) (domainagents.Agent, error)
}

// ArtifactRecorder records what an agent produced as order deliverables.
// metaDir is the managed directory where the run summary is written; when
// collectFiles is true the agent's workspace is also scanned for real files.
type ArtifactRecorder interface {
	RecordRunOutput(ctx context.Context, order domainorders.Order, metaDir string, workspaceDir string, collectFiles bool, finalMessage string) ([]domainartifacts.Artifact, error)
}

// Engine runs a request against a local agent runtime. *agentrun.Engine
// satisfies it; tests inject a stub.
type Engine interface {
	Run(ctx context.Context, req agentrun.Request, sink agentrun.Sink) (agentrun.Result, error)
	Available(agentrun.Runtime) bool
	AvailableRuntimes() []agentrun.Runtime
}

// Config tunes the worker.
type Config struct {
	// WorkspaceRoot is the directory under which each order gets its own
	// managed workspace and run metadata (<root>/<orderID>). Defaults to
	// ./workspaces.
	WorkspaceRoot string
	// MaxConcurrent caps how many orders run at once. Defaults to 2.
	MaxConcurrent int
}

// RunStatus is the lifecycle of a single agent run as the worker observes it.
type RunStatus string

const (
	RunRunning   RunStatus = "running"
	RunSucceeded RunStatus = "succeeded"
	RunFailed    RunStatus = "failed"
)

// RunLog is the live record of an order's agent run: streamed events plus
// terminal state. It powers the "watch the agent work" view and is persisted so
// history and the resume session id survive a restart.
type RunLog struct {
	OrderID      string           `json:"orderId"`
	Runtime      string           `json:"runtime"`
	Status       RunStatus        `json:"status"`
	Events       []agentrun.Event `json:"events"`
	FinalMessage string           `json:"finalMessage,omitempty"`
	Error        string           `json:"error,omitempty"`
	// SessionID is the runtime's session id from the latest turn, used to
	// resume the task.
	SessionID string `json:"sessionId,omitempty"`
	// Turns counts how many times the task has run (1 = first run, 2+ resumed).
	Turns     int       `json:"turns"`
	StartedAt time.Time `json:"startedAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Usecase struct {
	orders    OrderRepository
	agents    AgentResolver
	artifacts ArtifactRecorder
	engine    Engine
	cfg       Config
	now       func() time.Time

	runs     *runStore
	mu       sync.Mutex
	inflight map[string]bool
	wg       sync.WaitGroup
}

func NewUsecase(orderRepo OrderRepository, agents AgentResolver, artifacts ArtifactRecorder, engine Engine, cfg Config) *Usecase {
	if cfg.WorkspaceRoot == "" {
		cfg.WorkspaceRoot = "workspaces"
	}
	if cfg.MaxConcurrent <= 0 {
		cfg.MaxConcurrent = 2
	}
	return &Usecase{
		orders:    orderRepo,
		agents:    agents,
		artifacts: artifacts,
		engine:    engine,
		cfg:       cfg,
		now:       time.Now,
		runs:      newRunStore(cfg.WorkspaceRoot),
		inflight:  make(map[string]bool),
	}
}

// AvailableRuntimes lists the local runtimes that are installed, so callers can
// show the user which agents they can actually run.
func (s *Usecase) AvailableRuntimes() []agentrun.Runtime {
	return s.engine.AvailableRuntimes()
}

// RunOnce picks up running orders that are not already in flight and dispatches
// each to its own goroutine. It returns immediately; long-horizon runs continue
// in the background. The in-flight guard ensures the polling loop never starts
// the same order twice.
func (s *Usecase) RunOnce(ctx context.Context) error {
	items, err := s.orders.ListOrdersByStatus(ctx, domainorders.StatusRunning, s.cfg.MaxConcurrent*2)
	if err != nil {
		return err
	}
	for _, order := range items {
		if s.inflightCount() >= s.cfg.MaxConcurrent {
			break
		}
		if !s.claim(order.ID) {
			continue
		}
		order := order
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			defer s.release(order.ID)
			s.runOrder(ctx, order)
		}()
	}
	return nil
}

// Start runs the worker loop until ctx is cancelled.
func (s *Usecase) Start(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = time.Second
	}
	go func() {
		_ = s.RunOnce(ctx)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = s.RunOnce(ctx)
			}
		}
	}()
}

// Wait blocks until all dispatched runs finish; used by tests and graceful
// shutdown.
func (s *Usecase) Wait() { s.wg.Wait() }

// Snapshot returns a copy of an order's run log, loading it from disk if the
// worker has not run it this process (e.g. after a restart).
func (s *Usecase) Snapshot(orderID string) (RunLog, bool) {
	return s.runs.get(orderID)
}

// SessionID returns the resumable runtime session id from an order's latest
// run, hydrating from disk after a restart. It satisfies the orders package's
// RunSessionReader so a task can be resumed for a multi-turn continuation.
func (s *Usecase) SessionID(orderID string) (string, bool) {
	log, ok := s.runs.get(orderID)
	if !ok || strings.TrimSpace(log.SessionID) == "" {
		return "", false
	}
	return log.SessionID, true
}

// runOrder executes one order end to end. It is synchronous so it can be unit
// tested directly; RunOnce wraps it in a goroutine.
func (s *Usecase) runOrder(ctx context.Context, order domainorders.Order) {
	resume := order.ResumeSessionID != ""

	// The managed metadata dir always lives under the workspace root; it holds
	// the run log and summary even when the agent works in a user directory.
	metaDir := filepath.Join(s.cfg.WorkspaceRoot, order.ID)
	if err := os.MkdirAll(metaDir, 0o755); err != nil {
		s.failOrder(ctx, order, "prepare workspace: "+err.Error())
		return
	}

	// Where the agent actually works: the user's directory if supplied,
	// otherwise the managed workspace.
	workDir := metaDir
	collectFiles := true
	if order.Workspace != "" {
		workDir = order.Workspace
		collectFiles = false // never scan a user's whole project for artifacts
		if info, err := os.Stat(workDir); err != nil || !info.IsDir() {
			s.failOrder(ctx, order, "工作目录不存在或不可用："+workDir)
			return
		}
	}

	s.beginRun(order, resume)

	agent, err := s.agents.GetAgent(ctx, order.AgentID)
	if err != nil {
		s.failOrder(ctx, order, "resolve agent: "+err.Error())
		return
	}

	runtime := s.pickRuntime(agent.Runtime)
	if runtime == "" {
		s.failOrder(ctx, order, "没有可用的本地 agent 运行时，请安装 codex 或 claude CLI")
		return
	}
	s.setRuntime(order.ID, string(runtime))

	prompt := composePrompt(agent, order.Requirement.Prompt, resume)
	req := agentrun.Request{
		Runtime:         runtime,
		Workspace:       workDir,
		Prompt:          prompt,
		Model:           agent.Model,
		Sandbox:         agentrun.Sandbox(agent.Sandbox),
		ResumeSessionID: order.ResumeSessionID,
	}
	res, runErr := s.engine.Run(ctx, req, func(e agentrun.Event) {
		s.appendEvent(order.ID, e)
	})
	if res.SessionID != "" {
		s.setSession(order.ID, res.SessionID)
	}

	if ctx.Err() != nil {
		// Deliberate shutdown — leave the order running for a future restart.
		s.markRun(order.ID, RunFailed, res.FinalMessage, "已中断（服务停止）")
		return
	}
	if runErr != nil {
		s.failOrder(ctx, order, runErr.Error())
		return
	}
	if !res.Succeeded {
		s.failOrder(ctx, order, firstNonEmpty(res.FinalMessage, "agent 未能正常完成任务"))
		return
	}

	// running -> delivering. A conflict here means the user cancelled mid-run.
	delivering := order
	delivering.Status = domainorders.StatusDelivering
	delivering.ResumeSessionID = "" // consumed
	delivering.UpdatedAt = s.now()
	if err := s.orders.TransitionOrder(ctx, delivering, domainorders.StatusRunning); err != nil {
		s.markRun(order.ID, RunFailed, res.FinalMessage, "订单状态已变更："+err.Error())
		return
	}

	if _, err := s.artifacts.RecordRunOutput(ctx, delivering, metaDir, workDir, collectFiles, res.FinalMessage); err != nil {
		failed := delivering
		failed.Status = domainorders.StatusFailed
		failed.FailureReason = "record output: " + err.Error()
		failed.UpdatedAt = s.now()
		_ = s.orders.TransitionOrder(ctx, failed, domainorders.StatusDelivering)
		s.markRun(order.ID, RunFailed, res.FinalMessage, err.Error())
		return
	}

	delivered := delivering
	delivered.Status = domainorders.StatusDelivered
	delivered.UpdatedAt = s.now()
	if err := s.orders.TransitionOrder(ctx, delivered, domainorders.StatusDelivering); err != nil {
		s.markRun(order.ID, RunFailed, res.FinalMessage, err.Error())
		return
	}
	s.markRun(order.ID, RunSucceeded, res.FinalMessage, "")
}

// pickRuntime honors the agent's configured runtime when it is installed, and
// otherwise falls back to any available runtime so a task still runs when the
// preferred CLI is missing. Returns "" when nothing is installed.
func (s *Usecase) pickRuntime(preferred string) agentrun.Runtime {
	rt := agentrun.Runtime(preferred)
	if rt.Valid() && s.engine.Available(rt) {
		return rt
	}
	if avail := s.engine.AvailableRuntimes(); len(avail) > 0 {
		return avail[0]
	}
	return ""
}

func (s *Usecase) failOrder(ctx context.Context, order domainorders.Order, reason string) {
	failed := order
	failed.Status = domainorders.StatusFailed
	failed.FailureReason = reason
	failed.ResumeSessionID = ""
	failed.UpdatedAt = s.now()
	_ = s.orders.TransitionOrder(ctx, failed, domainorders.StatusRunning)
	s.markRun(order.ID, RunFailed, "", reason)
}

// --- run log bookkeeping (delegated to the persistent run store) ---

// beginRun marks a run as started. A fresh run resets the event stream; a
// resume keeps the prior events and bumps the turn counter so the continuation
// appends to the same transcript.
func (s *Usecase) beginRun(order domainorders.Order, resume bool) {
	s.runs.update(order.ID, true, func(log *RunLog) {
		now := s.now()
		log.OrderID = order.ID
		log.Status = RunRunning
		log.Error = ""
		log.UpdatedAt = now
		if resume && log.Turns > 0 {
			log.Turns++
		} else {
			log.Events = nil
			log.FinalMessage = ""
			log.Turns = 1
			log.StartedAt = now
		}
	})
}

func (s *Usecase) setRuntime(orderID, runtime string) {
	s.runs.update(orderID, true, func(log *RunLog) {
		log.Runtime = runtime
		log.UpdatedAt = s.now()
	})
}

func (s *Usecase) setSession(orderID, sessionID string) {
	s.runs.update(orderID, true, func(log *RunLog) {
		log.SessionID = sessionID
		log.UpdatedAt = s.now()
	})
}

// appendEvent records a streamed event in memory (no disk write per line).
func (s *Usecase) appendEvent(orderID string, e agentrun.Event) {
	s.runs.update(orderID, false, func(log *RunLog) {
		log.Events = append(log.Events, e)
		log.UpdatedAt = s.now()
	})
}

// markRun records terminal state and flushes the run log to disk.
func (s *Usecase) markRun(orderID string, status RunStatus, final, errText string) {
	s.runs.update(orderID, true, func(log *RunLog) {
		log.Status = status
		if final != "" {
			log.FinalMessage = final
		}
		log.Error = errText
		log.UpdatedAt = s.now()
	})
}

// --- in-flight guard ---

func (s *Usecase) claim(orderID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.inflight[orderID] {
		return false
	}
	s.inflight[orderID] = true
	return true
}

func (s *Usecase) release(orderID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.inflight, orderID)
}

func (s *Usecase) inflightCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.inflight)
}

// composePrompt joins the agent's persona instruction with the user's task. On
// a resume the persona is already in the session context, so only the follow-up
// task is sent.
func composePrompt(agent domainagents.Agent, task string, resume bool) string {
	task = strings.TrimSpace(task)
	var b strings.Builder
	if !resume {
		if sp := strings.TrimSpace(agent.SystemPrompt); sp != "" {
			b.WriteString(sp)
			b.WriteString("\n\n")
		}
		b.WriteString("用户任务：\n")
	} else {
		b.WriteString("用户补充/后续任务：\n")
	}
	b.WriteString(task)
	b.WriteString("\n\n请在当前工作目录中完成任务，并把交付物保存为文件。完成后用一段话总结你做了什么。")
	return b.String()
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
