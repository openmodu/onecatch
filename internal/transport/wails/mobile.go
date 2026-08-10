package wailstransport

import (
	"context"

	domainworkspaces "github.com/openmodu/oneshot/internal/domain/workspaces"
	mobileservice "github.com/openmodu/oneshot/internal/service/mobile"
	"github.com/openmodu/oneshot/internal/service/worker"
)

type MobileBinding struct{ service *mobileservice.Service }

func NewMobileBinding(service *mobileservice.Service) *MobileBinding {
	return &MobileBinding{service: service}
}

func (b *MobileBinding) ListWorkers() ([]worker.Info, error) {
	return b.service.ListWorkers(context.Background())
}

func (b *MobileBinding) PairWorker(baseURL, code string) (worker.Info, error) {
	return b.service.PairWorker(context.Background(), baseURL, code)
}

func (b *MobileBinding) DeleteWorker(id string) error {
	return b.service.DeleteWorker(context.Background(), id)
}

func (b *MobileBinding) CheckWorker(id string) (mobileservice.WorkerStatus, error) {
	return b.service.CheckWorker(context.Background(), id)
}

func (b *MobileBinding) ListWorkspaces(workerID string) ([]worker.WorkspaceMapping, error) {
	return b.service.ListWorkspaces(context.Background(), workerID)
}

func (b *MobileBinding) WorkspaceGitStatus(workerID, workspaceID string) (domainworkspaces.GitSnapshot, error) {
	return b.service.WorkspaceGitStatus(context.Background(), workerID, workspaceID)
}

func (b *MobileBinding) StartRun(input mobileservice.StartRunInput) (mobileservice.RunView, error) {
	return b.service.StartRun(context.Background(), input)
}

func (b *MobileBinding) GetRun(id string) (mobileservice.RunView, error) {
	return b.service.GetRun(id)
}

func (b *MobileBinding) ListRuns() []mobileservice.RunView {
	return b.service.ListRuns()
}

func (b *MobileBinding) InterruptRun(id string) error {
	return b.service.InterruptRun(context.Background(), id)
}

func (b *MobileBinding) RespondPermission(input mobileservice.PermissionDecisionInput) error {
	return b.service.RespondPermission(context.Background(), input)
}
