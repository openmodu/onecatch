package artifacts

import (
	"context"
	"fmt"
	"sync"

	domainartifacts "github.com/openmodu/oneshot/internal/domain/artifacts"
	pkgsql "github.com/openmodu/oneshot/pkg/sql"
)

type ArtifactsRepo interface {
	NextArtifactID(context.Context) (string, error)
	NextShareToken(context.Context) (string, error)
	SaveArtifact(context.Context, domainartifacts.Artifact) error
	ListArtifacts(context.Context, string, string) ([]domainartifacts.Artifact, error)
	GetArtifact(context.Context, string, string) (domainartifacts.Artifact, error)
	SaveShare(context.Context, domainartifacts.Share) (domainartifacts.Share, error)
}

type artifactsImpl struct {
	sql *pkgsql.Sql
	mu  sync.RWMutex

	artifacts    map[string]domainartifacts.Artifact
	shares       map[string]domainartifacts.Share
	nextArtifact int
	nextShare    int
}

func NewArtifactsRepo(sql *pkgsql.Sql) ArtifactsRepo {
	return &artifactsImpl{
		sql:       sql,
		artifacts: make(map[string]domainartifacts.Artifact),
		shares:    make(map[string]domainartifacts.Share),
	}
}

func (r *artifactsImpl) NextArtifactID(context.Context) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.nextArtifact++
	return fmt.Sprintf("artifact_%06d", r.nextArtifact), nil
}

func (r *artifactsImpl) NextShareToken(context.Context) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.nextShare++
	return fmt.Sprintf("share_%06d", r.nextShare), nil
}

func (r *artifactsImpl) SaveArtifact(_ context.Context, artifact domainartifacts.Artifact) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.artifacts[artifact.ID] = artifact
	return nil
}

func (r *artifactsImpl) ListArtifacts(_ context.Context, userID string, orderID string) ([]domainartifacts.Artifact, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]domainartifacts.Artifact, 0)
	for _, artifact := range r.artifacts {
		if artifact.UserID == userID && artifact.OrderID == orderID {
			out = append(out, artifact)
		}
	}
	return out, nil
}

func (r *artifactsImpl) GetArtifact(_ context.Context, userID string, artifactID string) (domainartifacts.Artifact, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	artifact, ok := r.artifacts[artifactID]
	if !ok || artifact.UserID != userID {
		return domainartifacts.Artifact{}, domainartifacts.ErrNotFound
	}
	return artifact, nil
}

func (r *artifactsImpl) SaveShare(_ context.Context, share domainartifacts.Share) (domainartifacts.Share, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.shares[share.ArtifactID] = share
	return share, nil
}
