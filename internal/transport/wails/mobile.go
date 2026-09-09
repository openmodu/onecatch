package wailstransport

import (
	"context"

	domainworkspaces "github.com/openmodu/onecatch/internal/domain/workspaces"
	mobileservice "github.com/openmodu/onecatch/internal/service/mobile"
	"github.com/openmodu/onecatch/internal/service/worker"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
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

func (b *MobileBinding) PrepareWorkspace(workerID, workspaceID string, input worker.WorkspacePrepareRequest) (worker.WorkspacePrepareResult, error) {
	return b.service.PrepareWorkspace(context.Background(), workerID, workspaceID, input)
}

func (b *MobileBinding) RemoveWorkspace(workerID, workspaceID string, deleteFiles bool) error {
	return b.service.RemoveWorkspace(context.Background(), workerID, workspaceID, deleteFiles)
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

// LoadEarlierRun extends an open conversation one transcript page further back.
func (b *MobileBinding) LoadEarlierRun(id string) (mobileservice.RunView, error) {
	return b.service.LoadEarlierRun(id)
}

func (b *MobileBinding) ListRuns() []mobileservice.RunView {
	return b.service.ListRuns()
}

// AccountUsage reports one worker's quota and recent daily token activity.
func (b *MobileBinding) AccountUsage(workerID string, refresh bool) ([]agentrun.AccountUsage, error) {
	return b.service.AccountUsage(workerID, refresh)
}

// RenameConversation retitles a session everywhere it is stored.
func (b *MobileBinding) RenameConversation(id, title string) error {
	return b.service.RenameConversation(context.Background(), id, title)
}

// DeleteConversation removes a session and every run in it.
func (b *MobileBinding) DeleteConversation(id string) error {
	return b.service.DeleteConversation(context.Background(), id)
}

// RefreshRuns re-reads the host's history and waits for it, for pull to refresh.
func (b *MobileBinding) RefreshRuns() []mobileservice.RunView {
	return b.service.RefreshRuns()
}

func (b *MobileBinding) ListRunSummaries() []mobileservice.RunView {
	return b.service.ListRunSummaries()
}

func (b *MobileBinding) InterruptRun(id string) error {
	return b.service.InterruptRun(context.Background(), id)
}

func (b *MobileBinding) RespondPermission(input mobileservice.PermissionDecisionInput) error {
	return b.service.RespondPermission(context.Background(), input)
}
