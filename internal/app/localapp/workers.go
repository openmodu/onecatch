package localapp

import (
	"context"

	"github.com/openmodu/oneshot/internal/agentrun"
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

type remoteExecutor struct {
	registry *worker.Registry
	client   *worker.Client
}

func (e remoteExecutor) RunRemote(ctx context.Context, workerID, workspaceID string, request agentrun.Request, sink agentrun.Sink) (agentrun.Result, error) {
	config, err := e.registry.Get(ctx, workerID)
	if err != nil || !config.Enabled {
		return agentrun.Result{}, worker.RemoteError{Code: "worker_not_found", Message: "worker is missing or disabled"}
	}
	response, err := e.client.Execute(ctx, config, worker.ExecuteRequest{WorkspaceID: workspaceID, Runtime: request.Runtime, Model: request.Model, Sandbox: request.Sandbox, Prompt: request.Prompt, ResumeSessionID: request.ResumeSessionID})
	if err != nil {
		return agentrun.Result{}, err
	}
	for _, event := range response.Events {
		if sink != nil {
			sink(event)
		}
	}
	return response.Result, nil
}
