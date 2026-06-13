package artifacts

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	domainartifacts "github.com/openmodu/oneshot/internal/domain/artifacts"
	pkgsql "github.com/openmodu/oneshot/pkg/sql"
	"gorm.io/gorm"
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

type artifactRecord struct {
	ID        string `gorm:"primaryKey;size:64"`
	OrderID   string `gorm:"size:64;not null;index"`
	UserID    string `gorm:"size:64;not null;index"`
	FileName  string `gorm:"size:255;not null"`
	FileType  string `gorm:"size:32;not null"`
	SizeBytes int64
	Preview   string `gorm:"type:text"`
	CreatedAt time.Time
}

type shareRecord struct {
	ArtifactID string `gorm:"primaryKey;size:64"`
	Token      string `gorm:"size:128;not null;uniqueIndex"`
	URL        string `gorm:"size:1024;not null"`
	CreatedAt  time.Time
}

func NewArtifactsRepo(sql *pkgsql.Sql) ArtifactsRepo {
	return &artifactsImpl{
		sql:       sql,
		artifacts: make(map[string]domainartifacts.Artifact),
		shares:    make(map[string]domainartifacts.Share),
	}
}

func (artifactRecord) TableName() string {
	return "artifacts"
}

func (shareRecord) TableName() string {
	return "artifact_shares"
}

func (r *artifactsImpl) NextArtifactID(context.Context) (string, error) {
	if r.sql != nil && r.sql.Gorm() != nil {
		return fmt.Sprintf("artifact_%d", time.Now().UnixNano()), nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.nextArtifact++
	return fmt.Sprintf("artifact_%06d", r.nextArtifact), nil
}

func (r *artifactsImpl) NextShareToken(context.Context) (string, error) {
	if r.sql != nil && r.sql.Gorm() != nil {
		return fmt.Sprintf("share_%d", time.Now().UnixNano()), nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.nextShare++
	return fmt.Sprintf("share_%06d", r.nextShare), nil
}

func (r *artifactsImpl) SaveArtifact(ctx context.Context, artifact domainartifacts.Artifact) error {
	if r.sql != nil && r.sql.Gorm() != nil {
		return r.saveArtifactSQL(ctx, artifact)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.artifacts[artifact.ID] = artifact
	return nil
}

func (r *artifactsImpl) ListArtifacts(ctx context.Context, userID string, orderID string) ([]domainartifacts.Artifact, error) {
	if r.sql != nil && r.sql.Gorm() != nil {
		return r.listArtifactsSQL(ctx, userID, orderID)
	}

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

func (r *artifactsImpl) GetArtifact(ctx context.Context, userID string, artifactID string) (domainartifacts.Artifact, error) {
	if r.sql != nil && r.sql.Gorm() != nil {
		return r.getArtifactSQL(ctx, userID, artifactID)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	artifact, ok := r.artifacts[artifactID]
	if !ok || artifact.UserID != userID {
		return domainartifacts.Artifact{}, domainartifacts.ErrNotFound
	}
	return artifact, nil
}

func (r *artifactsImpl) SaveShare(ctx context.Context, share domainartifacts.Share) (domainartifacts.Share, error) {
	if r.sql != nil && r.sql.Gorm() != nil {
		return r.saveShareSQL(ctx, share)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.shares[share.ArtifactID] = share
	return share, nil
}

func (r *artifactsImpl) saveArtifactSQL(ctx context.Context, artifact domainartifacts.Artifact) error {
	record := fromDomainArtifact(artifact)
	return r.sql.Gorm().WithContext(ctx).Save(&record).Error
}

func (r *artifactsImpl) listArtifactsSQL(ctx context.Context, userID string, orderID string) ([]domainartifacts.Artifact, error) {
	var records []artifactRecord
	if err := r.sql.Gorm().WithContext(ctx).
		Where("user_id = ? AND order_id = ?", userID, orderID).
		Order("created_at ASC").
		Find(&records).Error; err != nil {
		return nil, err
	}
	out := make([]domainartifacts.Artifact, 0, len(records))
	for _, record := range records {
		out = append(out, toDomainArtifact(record))
	}
	return out, nil
}

func (r *artifactsImpl) getArtifactSQL(ctx context.Context, userID string, artifactID string) (domainartifacts.Artifact, error) {
	var record artifactRecord
	err := r.sql.Gorm().WithContext(ctx).
		Where("id = ? AND user_id = ?", artifactID, userID).
		First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domainartifacts.Artifact{}, domainartifacts.ErrNotFound
	}
	if err != nil {
		return domainartifacts.Artifact{}, err
	}
	return toDomainArtifact(record), nil
}

func (r *artifactsImpl) saveShareSQL(ctx context.Context, share domainartifacts.Share) (domainartifacts.Share, error) {
	record := shareRecord{
		ArtifactID: share.ArtifactID,
		Token:      share.Token,
		URL:        share.URL,
		CreatedAt:  share.CreatedAt,
	}
	if err := r.sql.Gorm().WithContext(ctx).Save(&record).Error; err != nil {
		return domainartifacts.Share{}, err
	}
	return share, nil
}

// Migrate creates or updates this repo's tables. Called once at startup.
func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(&artifactRecord{}, &shareRecord{})
}
func fromDomainArtifact(artifact domainartifacts.Artifact) artifactRecord {
	return artifactRecord{
		ID:        artifact.ID,
		OrderID:   artifact.OrderID,
		UserID:    artifact.UserID,
		FileName:  artifact.FileName,
		FileType:  artifact.FileType,
		SizeBytes: artifact.SizeBytes,
		Preview:   artifact.Preview,
		CreatedAt: artifact.CreatedAt,
	}
}

func toDomainArtifact(record artifactRecord) domainartifacts.Artifact {
	return domainartifacts.Artifact{
		ID:        record.ID,
		OrderID:   record.OrderID,
		UserID:    record.UserID,
		FileName:  record.FileName,
		FileType:  record.FileType,
		SizeBytes: record.SizeBytes,
		Preview:   record.Preview,
		CreatedAt: record.CreatedAt,
	}
}
