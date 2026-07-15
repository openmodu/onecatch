package bindings

import (
	"context"

	"github.com/openmodu/oneshot/internal/app/localapp"
	domainworkspaces "github.com/openmodu/oneshot/internal/domain/workspaces"
	"github.com/openmodu/oneshot/internal/worker"
)

type WorkerBinding struct{ app *localapp.App }

func NewWorkerBinding(app *localapp.App) *WorkerBinding { return &WorkerBinding{app: app} }
func (b *WorkerBinding) ListWorkers() ([]worker.Info, error) {
	return b.app.ListWorkers(context.Background())
}
func (b *WorkerBinding) SaveWorker(input worker.Input) (worker.Info, error) {
	return b.app.SaveWorker(context.Background(), input)
}
func (b *WorkerBinding) DeleteWorker(id string) error {
	return b.app.DeleteWorker(context.Background(), id)
}
func (b *WorkerBinding) CheckWorker(id string) (localapp.WorkerStatus, error) {
	return b.app.CheckWorker(context.Background(), id)
}
func (b *WorkerBinding) WorkerGitStatus(id, workspaceID string) (domainworkspaces.GitSnapshot, error) {
	return b.app.WorkerGitStatus(context.Background(), id, workspaceID)
}
