package wailstransport

import (
	"context"

	domainworkspaces "github.com/openmodu/onecatch/internal/domain/workspaces"
	desktopservice "github.com/openmodu/onecatch/internal/service/desktop"
	"github.com/openmodu/onecatch/internal/service/worker"
)

type WorkerBinding struct{ service *desktopservice.Service }

func NewWorkerBinding(service *desktopservice.Service) *WorkerBinding {
	return &WorkerBinding{service: service}
}
func (b *WorkerBinding) ListWorkers() ([]worker.Info, error) {
	return b.service.ListWorkers(context.Background())
}
func (b *WorkerBinding) UpdateWorker(input worker.UpdateInput) (worker.Info, error) {
	return b.service.UpdateWorker(context.Background(), input)
}
func (b *WorkerBinding) DeleteWorker(id string) error {
	return b.service.DeleteWorker(context.Background(), id)
}
func (b *WorkerBinding) PairWorker(baseURL, code string) (worker.Info, error) {
	return b.service.PairWorker(context.Background(), baseURL, code)
}
func (b *WorkerBinding) CheckWorker(id string) (desktopservice.WorkerStatus, error) {
	return b.service.CheckWorker(context.Background(), id)
}
func (b *WorkerBinding) WorkerGitStatus(id, workspaceID string) (domainworkspaces.GitSnapshot, error) {
	return b.service.WorkerGitStatus(context.Background(), id, workspaceID)
}
func (b *WorkerBinding) PrepareWorkerWorkspace(id, workspaceID string) (desktopservice.WorkerWorkspaceSetup, error) {
	return b.service.PrepareWorkerWorkspace(context.Background(), id, workspaceID)
}
