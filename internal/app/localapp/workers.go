package localapp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/openmodu/oneshot/internal/agentrun"
	domainworkspaces "github.com/openmodu/oneshot/internal/domain/workspaces"
	"github.com/openmodu/oneshot/internal/worker"
)

type WorkerStatus struct {
	Worker worker.Info   `json:"worker"`
	Health worker.Health `json:"health"`
}

func (a *App) ListWorkers(ctx context.Context) ([]worker.Info, error) { return a.workers.List(ctx) }
func (a *App) SaveWorker(ctx context.Context, input worker.Input) (worker.Info, error) {
	return a.workers.Save(ctx, input)
}
func (a *App) DeleteWorker(ctx context.Context, id string) error { return a.workers.Delete(ctx, id) }
func (a *App) CheckWorker(ctx context.Context, id string) (WorkerStatus, error) {
	config, err := a.workers.Get(ctx, id)
	if err != nil {
		return WorkerStatus{}, coded("worker_not_found", "worker was not found")
	}
	health, err := a.workerClient.Health(ctx, config)
	if err != nil {
		return WorkerStatus{}, err
	}
	return WorkerStatus{Worker: worker.Info{ID: config.ID, Name: config.Name, BaseURL: config.BaseURL, Enabled: config.Enabled, HasToken: config.Token != "", CreatedAt: config.CreatedAt, UpdatedAt: config.UpdatedAt}, Health: health}, nil
}

// WorkerGitStatus reads the git state of a workspace on a remote worker, so the
// desktop Git panel can show what a remotely executed step changed.
func (a *App) WorkerGitStatus(ctx context.Context, workerID, workspaceID string) (domainworkspaces.GitSnapshot, error) {
	config, err := a.workers.Get(ctx, workerID)
	if err != nil {
		return domainworkspaces.GitSnapshot{}, coded("worker_not_found", "worker was not found")
	}
	return a.workerClient.GitStatus(ctx, config, workspaceID)
}

type remoteExecutor struct {
	registry *worker.Registry
	client   *worker.Client
}

func (e remoteExecutor) RunRemote(ctx context.Context, workerID, workspaceID string, request agentrun.Request, sink agentrun.Sink) (agentrun.Result, error) {
	config, err := e.registry.Get(ctx, workerID)
	if err != nil || !config.Enabled {
		return agentrun.Result{}, worker.RemoteError{Code: "worker_not_found", Message: "worker is missing or disabled"}
	}
	runID, err := newRunID()
	if err != nil {
		return agentrun.Result{}, err
	}

	// The streaming POST is deliberately detached from ctx. A cancel must become
	// a graceful interrupt — SIGINT plus grace on the remote agent, so it can
	// flush its final events down the open stream — not a hard connection reset
	// that discards them. A backstop hard-cancels if the worker never closes.
	streamCtx, cancelStream := context.WithCancel(context.Background())
	defer cancelStream()
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-done:
			return
		case <-ctx.Done():
		}
		interruptCtx, cancelInterrupt := context.WithTimeout(context.Background(), 8*time.Second)
		_ = e.client.Interrupt(interruptCtx, config, runID)
		cancelInterrupt()
		grace := request.InterruptGrace + 5*time.Second
		timer := time.NewTimer(grace)
		defer timer.Stop()
		select {
		case <-done:
		case <-timer.C:
			cancelStream()
		}
	}()

	return e.client.Execute(streamCtx, config, worker.ExecuteRequest{
		RunID:                 runID,
		WorkspaceID:           workspaceID,
		Runtime:               request.Runtime,
		Model:                 request.Model,
		ReasoningEffort:       request.ReasoningEffort,
		ServiceTier:           request.ServiceTier,
		Sandbox:               request.Sandbox,
		Prompt:                request.Prompt,
		ResumeSessionID:       request.ResumeSessionID,
		InterruptGraceSeconds: int(request.InterruptGrace / time.Second),
	}, sink)
}

func newRunID() (string, error) {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}
