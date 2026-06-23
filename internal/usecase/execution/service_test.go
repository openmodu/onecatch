package execution

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/openmodu/oneshot/internal/agentrun"
	domainorders "github.com/openmodu/oneshot/internal/domain/orders"
	"github.com/openmodu/oneshot/internal/domain/users"
	repoagents "github.com/openmodu/oneshot/internal/repo/agents"
	repoartifacts "github.com/openmodu/oneshot/internal/repo/artifacts"
	repoorders "github.com/openmodu/oneshot/internal/repo/orders"
	usecaseartifacts "github.com/openmodu/oneshot/internal/usecase/artifacts"
)

// stubEngine simulates a local agent: on Run it writes a deliverable into the
// workspace, streams a couple of events, and reports success (or a configured
// failure). It lets the worker be tested without spawning a real CLI. On a
// resume it writes a distinct file and records the session id it was given.
type stubEngine struct {
	available []agentrun.Runtime
	writeFile string
	result    agentrun.Result
	runErr    error

	mu          sync.Mutex
	lastResume  string
	resumeFile  string
	resumeCalls int
}

func (e *stubEngine) Run(ctx context.Context, req agentrun.Request, sink agentrun.Sink) (agentrun.Result, error) {
	sink(agentrun.Event{Kind: agentrun.KindStarted, At: time.Now()})
	resume := req.ResumeSessionID != ""
	file := e.writeFile
	if resume {
		e.mu.Lock()
		e.lastResume = req.ResumeSessionID
		e.resumeCalls++
		e.mu.Unlock()
		if e.resumeFile != "" {
			file = e.resumeFile
		}
	}
	if file != "" {
		_ = os.WriteFile(filepath.Join(req.Workspace, file), []byte("# 报告\n内容"), 0o644)
		sink(agentrun.Event{Kind: agentrun.KindFileChange, Text: file, At: time.Now()})
	}
	sink(agentrun.Event{Kind: agentrun.KindMessage, Text: e.result.FinalMessage, At: time.Now()})
	return e.result, e.runErr
}

func (e *stubEngine) lastResumeID() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.lastResume
}

func (e *stubEngine) Available(rt agentrun.Runtime) bool {
	for _, r := range e.available {
		if r == rt {
			return true
		}
	}
	return false
}

func (e *stubEngine) AvailableRuntimes() []agentrun.Runtime { return e.available }

func newWorker(t *testing.T, engine Engine) (*Usecase, repoorders.OrdersRepo, *usecaseartifacts.Usecase) {
	t.Helper()
	orderRepo := repoorders.NewOrdersRepo(nil)
	artifactRepo := repoartifacts.NewArtifactsRepo(nil)
	agentRepo := repoagents.NewAgentsRepo(nil)
	artifactUsecase := usecaseartifacts.NewUsecase(artifactRepo, orderRepo)
	worker := NewUsecase(orderRepo, agentRepo, artifactUsecase, engine, Config{WorkspaceRoot: t.TempDir()})
	return worker, orderRepo, artifactUsecase
}

func runningOrder() domainorders.Order {
	return domainorders.Order{
		ID:          "order_worker_1",
		UserID:      users.DevUserID,
		AgentID:     "research-analyst",
		AgentName:   "行业研究分析师",
		Requirement: domainorders.Requirement{Prompt: "测试 worker"},
		Status:      domainorders.StatusRunning,
		UsageCost:   1,
		AmountCents: 1990,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
}

func TestRunOnceDeliversWithRealArtifacts(t *testing.T) {
	ctx := context.Background()
	engine := &stubEngine{
		available: []agentrun.Runtime{agentrun.RuntimeCodex},
		writeFile: "report.md",
		result:    agentrun.Result{FinalMessage: "已完成报告", Succeeded: true},
	}
	worker, orderRepo, artifactUsecase := newWorker(t, engine)

	if err := orderRepo.SaveOrder(ctx, runningOrder()); err != nil {
		t.Fatalf("save order: %v", err)
	}
	if err := worker.RunOnce(ctx); err != nil {
		t.Fatalf("run once: %v", err)
	}
	worker.Wait()

	delivered, err := orderRepo.GetOrder(ctx, users.DevUserID, "order_worker_1")
	if err != nil {
		t.Fatalf("get order: %v", err)
	}
	if delivered.Status != domainorders.StatusDelivered {
		t.Fatalf("status = %s, want delivered", delivered.Status)
	}

	artifacts, err := artifactUsecase.ListForOrder(ctx, users.DevUserID, "order_worker_1")
	if err != nil {
		t.Fatalf("list artifacts: %v", err)
	}
	// Expect the agent's report.md plus the always-written SUMMARY.md.
	var names []string
	for _, a := range artifacts {
		names = append(names, a.FileName)
	}
	if !contains(names, "report.md") || !contains(names, "SUMMARY.md") {
		t.Fatalf("artifacts = %v, want report.md and SUMMARY.md", names)
	}

	// The run log should be retrievable and marked succeeded.
	log, ok := worker.Snapshot("order_worker_1")
	if !ok || log.Status != RunSucceeded {
		t.Fatalf("run log = %+v ok=%v, want succeeded", log, ok)
	}
	if len(log.Events) == 0 {
		t.Fatal("expected streamed events in run log")
	}
}

func TestRunOnceFailsOrderOnAgentFailure(t *testing.T) {
	ctx := context.Background()
	engine := &stubEngine{
		available: []agentrun.Runtime{agentrun.RuntimeCodex},
		result:    agentrun.Result{FinalMessage: "出错了", Succeeded: false},
	}
	worker, orderRepo, _ := newWorker(t, engine)

	if err := orderRepo.SaveOrder(ctx, runningOrder()); err != nil {
		t.Fatalf("save order: %v", err)
	}
	if err := worker.RunOnce(ctx); err != nil {
		t.Fatalf("run once: %v", err)
	}
	worker.Wait()

	order, err := orderRepo.GetOrder(ctx, users.DevUserID, "order_worker_1")
	if err != nil {
		t.Fatalf("get order: %v", err)
	}
	if order.Status != domainorders.StatusFailed {
		t.Fatalf("status = %s, want failed", order.Status)
	}
	if order.FailureReason == "" {
		t.Fatal("expected a failure reason")
	}
}

func TestRunOnceFailsWhenNoRuntimeAvailable(t *testing.T) {
	ctx := context.Background()
	engine := &stubEngine{available: nil} // nothing installed
	worker, orderRepo, _ := newWorker(t, engine)

	if err := orderRepo.SaveOrder(ctx, runningOrder()); err != nil {
		t.Fatalf("save order: %v", err)
	}
	if err := worker.RunOnce(ctx); err != nil {
		t.Fatalf("run once: %v", err)
	}
	worker.Wait()

	order, err := orderRepo.GetOrder(ctx, users.DevUserID, "order_worker_1")
	if err != nil {
		t.Fatalf("get order: %v", err)
	}
	if order.Status != domainorders.StatusFailed {
		t.Fatalf("status = %s, want failed when no runtime", order.Status)
	}
}

func TestRunOnceDoesNotDoubleDispatch(t *testing.T) {
	ctx := context.Background()
	engine := &stubEngine{
		available: []agentrun.Runtime{agentrun.RuntimeCodex},
		writeFile: "report.md",
		result:    agentrun.Result{FinalMessage: "ok", Succeeded: true},
	}
	worker, orderRepo, _ := newWorker(t, engine)
	if err := orderRepo.SaveOrder(ctx, runningOrder()); err != nil {
		t.Fatalf("save order: %v", err)
	}

	// Two quick polls before the first run finishes must not start the order
	// twice; the in-flight guard plus the terminal transition prevent it.
	_ = worker.RunOnce(ctx)
	_ = worker.RunOnce(ctx)
	worker.Wait()

	order, _ := orderRepo.GetOrder(ctx, users.DevUserID, "order_worker_1")
	if order.Status != domainorders.StatusDelivered {
		t.Fatalf("status = %s, want delivered", order.Status)
	}
}

func TestWorkerResumesTaskAcrossTurns(t *testing.T) {
	ctx := context.Background()
	engine := &stubEngine{
		available:  []agentrun.Runtime{agentrun.RuntimeCodex},
		writeFile:  "report.md",
		resumeFile: "turn2.md",
		result:     agentrun.Result{FinalMessage: "ok", Succeeded: true, SessionID: "sess-A"},
	}
	worker, orderRepo, artifactUsecase := newWorker(t, engine)

	if err := orderRepo.SaveOrder(ctx, runningOrder()); err != nil {
		t.Fatalf("save order: %v", err)
	}
	// Turn 1.
	_ = worker.RunOnce(ctx)
	worker.Wait()
	if id, ok := worker.SessionID("order_worker_1"); !ok || id != "sess-A" {
		t.Fatalf("session id = %q ok=%v, want sess-A", id, ok)
	}

	// Re-queue the same order as a resume (what orders.Continue does).
	delivered, _ := orderRepo.GetOrder(ctx, users.DevUserID, "order_worker_1")
	delivered.Status = domainorders.StatusRunning
	delivered.ResumeSessionID = "sess-A"
	delivered.Requirement = domainorders.Requirement{Prompt: "继续"}
	if err := orderRepo.TransitionOrder(ctx, delivered, domainorders.StatusDelivered); err != nil {
		t.Fatalf("requeue: %v", err)
	}
	// Turn 2.
	_ = worker.RunOnce(ctx)
	worker.Wait()

	if got := engine.lastResumeID(); got != "sess-A" {
		t.Fatalf("engine resume id = %q, want sess-A", got)
	}
	final, _ := orderRepo.GetOrder(ctx, users.DevUserID, "order_worker_1")
	if final.Status != domainorders.StatusDelivered {
		t.Fatalf("status = %s, want delivered", final.Status)
	}
	if final.ResumeSessionID != "" {
		t.Fatalf("resume id should be cleared after run, got %q", final.ResumeSessionID)
	}

	// Run log accumulated a second turn; artifacts gained the new file.
	log, _ := worker.Snapshot("order_worker_1")
	if log.Turns != 2 {
		t.Fatalf("turns = %d, want 2", log.Turns)
	}
	arts, _ := artifactUsecase.ListForOrder(ctx, users.DevUserID, "order_worker_1")
	var names []string
	for _, a := range arts {
		names = append(names, a.FileName)
	}
	if !contains(names, "report.md") || !contains(names, "turn2.md") {
		t.Fatalf("artifacts = %v, want both turn files", names)
	}
}

func TestRunLogPersistsAcrossWorkers(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	orderRepo := repoorders.NewOrdersRepo(nil)
	artifactUsecase := usecaseartifacts.NewUsecase(repoartifacts.NewArtifactsRepo(nil), orderRepo)
	agentRepo := repoagents.NewAgentsRepo(nil)
	engine := &stubEngine{
		available: []agentrun.Runtime{agentrun.RuntimeCodex},
		writeFile: "report.md",
		result:    agentrun.Result{FinalMessage: "done", Succeeded: true, SessionID: "sess-P"},
	}

	w1 := NewUsecase(orderRepo, agentRepo, artifactUsecase, engine, Config{WorkspaceRoot: root})
	if err := orderRepo.SaveOrder(ctx, runningOrder()); err != nil {
		t.Fatalf("save: %v", err)
	}
	_ = w1.RunOnce(ctx)
	w1.Wait()

	// A brand-new worker (simulating a restart) sharing the same root must see
	// the prior run log — including the session id — loaded from disk.
	w2 := NewUsecase(orderRepo, agentRepo, artifactUsecase, engine, Config{WorkspaceRoot: root})
	log, ok := w2.Snapshot("order_worker_1")
	if !ok {
		t.Fatal("expected run log loaded from disk after restart")
	}
	if log.Status != RunSucceeded || log.SessionID != "sess-P" {
		t.Fatalf("hydrated log = %+v, want succeeded/sess-P", log)
	}
	if id, ok := w2.SessionID("order_worker_1"); !ok || id != "sess-P" {
		t.Fatalf("session after restart = %q ok=%v", id, ok)
	}
}

func contains(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}
