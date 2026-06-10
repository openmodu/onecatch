package bindings

import (
	"context"
	"os/exec"
	"runtime"

	oneshot "github.com/openmodu/oneshot/clients/oneshot"
)

type ArtifactBinding struct {
	client oneshot.Client
}

func NewArtifactBinding(client oneshot.Client) *ArtifactBinding {
	return &ArtifactBinding{client: client}
}

func (b *ArtifactBinding) ListArtifacts(orderID string) ([]oneshot.Artifact, error) {
	return b.client.ListArtifacts(context.Background(), orderID)
}

func (b *ArtifactBinding) DownloadArtifact(artifactID string) (oneshot.ArtifactDownload, error) {
	return b.client.DownloadArtifact(context.Background(), artifactID)
}

func (b *ArtifactBinding) ShareArtifact(artifactID string) (oneshot.ArtifactShare, error) {
	return b.client.ShareArtifact(context.Background(), artifactID)
}

func (b *ArtifactBinding) ShowInFolder(path string) error {
	if runtime.GOOS == "darwin" {
		return exec.Command("open", "-R", path).Run()
	}
	return exec.Command("xdg-open", path).Run()
}
