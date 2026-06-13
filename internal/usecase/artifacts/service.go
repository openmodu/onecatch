package artifacts

import (
	"context"
	"fmt"
	"time"

	domainartifacts "github.com/openmodu/oneshot/internal/domain/artifacts"
	domainorders "github.com/openmodu/oneshot/internal/domain/orders"
)

type Repository interface {
	NextArtifactID(context.Context) (string, error)
	NextShareToken(context.Context) (string, error)
	SaveArtifact(context.Context, domainartifacts.Artifact) error
	ListArtifacts(context.Context, string, string) ([]domainartifacts.Artifact, error)
	GetArtifact(context.Context, string, string) (domainartifacts.Artifact, error)
	SaveShare(context.Context, domainartifacts.Share) (domainartifacts.Share, error)
}

type OrderRepository interface {
	GetOrder(context.Context, string, string) (domainorders.Order, error)
}

type Usecase struct {
	repo   Repository
	orders OrderRepository
	now    func() time.Time
}

func NewUsecase(repo Repository, orders OrderRepository) *Usecase {
	return &Usecase{
		repo:   repo,
		orders: orders,
		now:    time.Now,
	}
}

func (s *Usecase) CreateForOrder(ctx context.Context, order domainorders.Order) (domainartifacts.Artifact, error) {
	// The worker generates the artifact while the order is still delivering,
	// before the final transition to delivered.
	if order.Status != domainorders.StatusDelivering && order.Status != domainorders.StatusDelivered {
		return domainartifacts.Artifact{}, domainartifacts.ErrNotReady
	}
	existing, err := s.repo.ListArtifacts(ctx, order.UserID, order.ID)
	if err != nil {
		return domainartifacts.Artifact{}, err
	}
	if len(existing) > 0 {
		return existing[0], nil
	}

	id, err := s.repo.NextArtifactID(ctx)
	if err != nil {
		return domainartifacts.Artifact{}, err
	}
	fileName := fmt.Sprintf("%s-report.pdf", order.ID)
	artifact := domainartifacts.Artifact{
		ID:        id,
		OrderID:   order.ID,
		UserID:    order.UserID,
		FileName:  fileName,
		FileType:  "PDF",
		SizeBytes: int64(len(renderReport(order))),
		Preview:   "report-preview",
		CreatedAt: s.now(),
	}
	return artifact, s.repo.SaveArtifact(ctx, artifact)
}

func (s *Usecase) ListForOrder(ctx context.Context, userID string, orderID string) ([]domainartifacts.Artifact, error) {
	order, err := s.orders.GetOrder(ctx, userID, orderID)
	if err != nil {
		return nil, err
	}
	if order.Status != domainorders.StatusDelivered {
		return nil, domainartifacts.ErrNotReady
	}
	artifact, err := s.CreateForOrder(ctx, order)
	if err != nil {
		return nil, err
	}
	artifacts, err := s.repo.ListArtifacts(ctx, userID, orderID)
	if err != nil {
		return nil, err
	}
	if len(artifacts) == 0 {
		return []domainartifacts.Artifact{artifact}, nil
	}
	return artifacts, nil
}

func (s *Usecase) Download(ctx context.Context, userID string, artifactID string) (domainartifacts.Download, error) {
	artifact, err := s.repo.GetArtifact(ctx, userID, artifactID)
	if err != nil {
		return domainartifacts.Download{}, err
	}
	order, err := s.orders.GetOrder(ctx, userID, artifact.OrderID)
	if err != nil {
		return domainartifacts.Download{}, err
	}
	if order.Status != domainorders.StatusDelivered {
		return domainartifacts.Download{}, domainartifacts.ErrNotReady
	}
	return domainartifacts.Download{
		Artifact:    artifact,
		ContentType: "application/pdf",
		Content:     renderReport(order),
	}, nil
}

func (s *Usecase) Share(ctx context.Context, userID string, artifactID string) (domainartifacts.Share, error) {
	artifact, err := s.repo.GetArtifact(ctx, userID, artifactID)
	if err != nil {
		return domainartifacts.Share{}, err
	}
	order, err := s.orders.GetOrder(ctx, userID, artifact.OrderID)
	if err != nil {
		return domainartifacts.Share{}, err
	}
	if order.Status != domainorders.StatusDelivered {
		return domainartifacts.Share{}, domainartifacts.ErrNotReady
	}
	token, err := s.repo.NextShareToken(ctx)
	if err != nil {
		return domainartifacts.Share{}, err
	}
	return s.repo.SaveShare(ctx, domainartifacts.Share{
		ArtifactID: artifact.ID,
		Token:      token,
		URL:        "https://oneshot.local/share/" + token,
		CreatedAt:  s.now(),
	})
}
