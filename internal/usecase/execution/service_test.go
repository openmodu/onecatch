package execution

import (
	"context"
	"os"
	"path/filepath"
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
// failure). It lets the worker be tested without spawning a real CLI.
type stubEngine struct {
	available []agentrun.Runtime
	writeFile string
	result    agentrun.Result
	runErr    error
}

func (e *stubEngine) Run(ctx context.Context, req agentrun.Request, sink agentrun.Sink) (agentrun.Result, error) {
	sink(agentrun.Event{Kind: agentrun.KindStarted, At: time.Now()})
	if e.writeFile != "" {
		_ = os.WriteFile(filepath.Join(req.Workspace, e.writeFile), []byte("# 报告\n内容"), 0o644)
		sink(agentrun.Event{Kind: agentrun.KindFileChange, Text: e.writeFile, At: time.Now()})
	}
	sink(agentrun.Event{Kind: agentrun.KindMessage, Text: e.result.FinalMessage, At: time.Now()})
	return e.result, e.runErr
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

func contains(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}
