// Package execution is the long-horizon worker: it picks up running orders and
// drives a real local agent (Codex / Claude Code) to completion in a per-order
// workspace, streaming the agent's progress and turning the files it produces
// into the order's deliverables.
package execution

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/openmodu/oneshot/internal/agentrun"
	domainagents "github.com/openmodu/oneshot/internal/domain/agents"
	domainartifacts "github.com/openmodu/oneshot/internal/domain/artifacts"
	domainorders "github.com/openmodu/oneshot/internal/domain/orders"
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

// ArtifactRecorder records the files an agent produced as order deliverables.
type ArtifactRecorder interface {
	RecordWorkspaceOutput(ctx context.Context, order domainorders.Order, workspaceDir string, finalMessage string) ([]domainartifacts.Artifact, error)
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
	// workspace (<root>/<orderID>). Defaults to ./workspaces.
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

// RunLog is the live, in-memory record of an order's agent run: the streamed
// events plus terminal state. It powers the "watch the agent work" view.
type RunLog struct {
	OrderID      string           `json:"orderId"`
	Runtime      string           `json:"runtime"`
	Status       RunStatus        `json:"status"`
	Events       []agentrun.Event `json:"events"`
	FinalMessage string           `json:"finalMessage,omitempty"`
	Error        string           `json:"error,omitempty"`
	StartedAt    time.Time        `json:"startedAt"`
	UpdatedAt    time.Time        `json:"updatedAt"`
}

type Usecase struct {
	orders    OrderRepository
	agents    AgentResolver
	artifacts ArtifactRecorder
	engine    Engine
	cfg       Config
	now       func() time.Time

	mu       sync.Mutex
	runs     map[string]*RunLog
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
		runs:      make(map[string]*RunLog),
		inflight:  make(map[string]bool),
	}
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

// Snapshot returns a copy of an order's run log, if the worker has one.
func (s *Usecase) Snapshot(orderID string) (RunLog, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	log, ok := s.runs[orderID]
	if !ok {
		return RunLog{}, false
	}
	cp := *log
	cp.Events = append([]agentrun.Event(nil), log.Events...)
	return cp, true
}

// runOrder executes one order end to end. It is synchronous so it can be unit
// tested directly; RunOnce wraps it in a goroutine.
func (s *Usecase) runOrder(ctx context.Context, order domainorders.Order) {
	workspace := filepath.Join(s.cfg.WorkspaceRoot, order.ID)
	s.beginRun(order)

	if err := os.MkdirAll(workspace, 0o755); err != nil {
		s.failOrder(ctx, order, "prepare workspace: "+err.Error())
		return
	}

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

	req := agentrun.Request{
		Runtime:   runtime,
		Workspace: workspace,
		Prompt:    composePrompt(agent, order.Requirement.Prompt),
		Model:     agent.Model,
		Sandbox:   agentrun.Sandbox(agent.Sandbox),
	}
	res, runErr := s.engine.Run(ctx, req, func(e agentrun.Event) {
		s.appendEvent(order.ID, e)
	})

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
	delivering.UpdatedAt = s.now()
	if err := s.orders.TransitionOrder(ctx, delivering, domainorders.StatusRunning); err != nil {
		s.markRun(order.ID, RunFailed, res.FinalMessage, "订单状态已变更："+err.Error())
		return
	}

	if _, err := s.artifacts.RecordWorkspaceOutput(ctx, delivering, workspace, res.FinalMessage); err != nil {
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
	failed.UpdatedAt = s.now()
	_ = s.orders.TransitionOrder(ctx, failed, domainorders.StatusRunning)
	s.markRun(order.ID, RunFailed, "", reason)
}

// --- run log bookkeeping (all guarded by s.mu) ---

func (s *Usecase) beginRun(order domainorders.Order) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	s.runs[order.ID] = &RunLog{
		OrderID:   order.ID,
		Status:    RunRunning,
		StartedAt: now,
		UpdatedAt: now,
	}
}

func (s *Usecase) setRuntime(orderID, runtime string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if log, ok := s.runs[orderID]; ok {
		log.Runtime = runtime
		log.UpdatedAt = s.now()
	}
}

func (s *Usecase) appendEvent(orderID string, e agentrun.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if log, ok := s.runs[orderID]; ok {
		log.Events = append(log.Events, e)
		log.UpdatedAt = s.now()
	}
}

func (s *Usecase) markRun(orderID string, status RunStatus, final, errText string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if log, ok := s.runs[orderID]; ok {
		log.Status = status
		log.FinalMessage = final
		log.Error = errText
		log.UpdatedAt = s.now()
	}
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

// composePrompt joins the agent's persona instruction with the user's task into
// the single prompt handed to the runtime.
func composePrompt(agent domainagents.Agent, task string) string {
	task = strings.TrimSpace(task)
	var b strings.Builder
	if sp := strings.TrimSpace(agent.SystemPrompt); sp != "" {
		b.WriteString(sp)
		b.WriteString("\n\n")
	}
	b.WriteString("用户任务：\n")
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
