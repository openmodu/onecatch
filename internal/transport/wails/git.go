package wailstransport

import (
	"context"

	domainworkspaces "github.com/openmodu/onecatch/internal/domain/workspaces"
	desktopservice "github.com/openmodu/onecatch/internal/service/desktop"
)

type GitBinding struct{ service *desktopservice.Service }

func NewGitBinding(service *desktopservice.Service) *GitBinding { return &GitBinding{service: service} }

func (b *GitBinding) Status(workspaceID string) (domainworkspaces.GitSnapshot, error) {
	return b.service.GitStatus(context.Background(), workspaceID)
}

func (b *GitBinding) Diff(workspaceID string, staged bool) (string, error) {
	return b.service.GitDiff(context.Background(), workspaceID, staged)
}

func (b *GitBinding) StageAll(workspaceID string) error {
	return b.service.GitStageAll(context.Background(), workspaceID)
}

func (b *GitBinding) ListBranches(workspaceID string) ([]domainworkspaces.GitBranch, error) {
	return b.service.GitListBranches(context.Background(), workspaceID)
}

func (b *GitBinding) SwitchBranch(workspaceID, name string) (domainworkspaces.GitSnapshot, error) {
	return b.service.GitSwitchBranch(context.Background(), workspaceID, name)
}

func (b *GitBinding) CreateBranch(workspaceID, name string) (domainworkspaces.GitSnapshot, error) {
	return b.service.GitCreateBranch(context.Background(), workspaceID, name)
}

func (b *GitBinding) GenerateCommitMessage(workspaceID, runtime string) (string, error) {
	return b.service.GenerateCommitMessage(context.Background(), workspaceID, runtime)
}

func (b *GitBinding) CommitAndPush(input desktopservice.GitCommitInput) (desktopservice.GitCommitResult, error) {
	return b.service.GitCommitAndPush(context.Background(), input)
}
