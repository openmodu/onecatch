package bindings

import (
	"context"

	"github.com/openmodu/oneshot/internal/app/localapp"
	domainworkspaces "github.com/openmodu/oneshot/internal/domain/workspaces"
)

type GitBinding struct{ app *localapp.App }

func NewGitBinding(app *localapp.App) *GitBinding { return &GitBinding{app: app} }

func (b *GitBinding) Status(workspaceID string) (domainworkspaces.GitSnapshot, error) {
	return b.app.GitStatus(context.Background(), workspaceID)
}

func (b *GitBinding) Diff(workspaceID string, staged bool) (string, error) {
	return b.app.GitDiff(context.Background(), workspaceID, staged)
}

func (b *GitBinding) StageAll(workspaceID string) error {
	return b.app.GitStageAll(context.Background(), workspaceID)
}

func (b *GitBinding) GenerateCommitMessage(workspaceID, runtime string) (string, error) {
	return b.app.GenerateCommitMessage(context.Background(), workspaceID, runtime)
}

func (b *GitBinding) CommitAndPush(input localapp.GitCommitInput) (localapp.GitCommitResult, error) {
	return b.app.GitCommitAndPush(context.Background(), input)
}
