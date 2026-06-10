package agents

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	domainagents "github.com/openmodu/oneshot/internal/domain/agents"
	pkgsql "github.com/openmodu/oneshot/pkg/sql"
	"gorm.io/gorm"
)

type AgentsRepo interface {
	ListAgents(context.Context) ([]domainagents.Agent, error)
	GetAgent(context.Context, string) (domainagents.Agent, error)
}

type agentsImpl struct {
	sql *pkgsql.Sql
	mu  sync.RWMutex

	agents []domainagents.Agent

	migrateOnce sync.Once
	migrateErr  error
}

type agentRecord struct {
	ID                string `gorm:"primaryKey;size:64"`
	Name              string `gorm:"size:255;not null"`
	Category          string `gorm:"size:64;not null;index"`
	Tags              string `gorm:"type:text"`
	Description       string `gorm:"type:text"`
	PriceUses         int
	PriceCents        int
	Rating            string `gorm:"size:32"`
	DealCount         int
	EstimatedDuration string `gorm:"size:64"`
	Deliverable       string `gorm:"type:text"`
	ArtifactTypes     string `gorm:"type:text"`
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func NewAgentsRepo(sql *pkgsql.Sql) AgentsRepo {
	return &agentsImpl{
		sql:    sql,
		agents: domainagents.SeedCatalog(),
	}
}

func (agentRecord) TableName() string {
	return "agents"
}

func (r *agentsImpl) ListAgents(ctx context.Context) ([]domainagents.Agent, error) {
	if r.sql != nil && r.sql.Gorm() != nil {
		return r.listAgentsSQL(ctx)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]domainagents.Agent, len(r.agents))
	copy(out, r.agents)
	return out, nil
}

func (r *agentsImpl) GetAgent(ctx context.Context, id string) (domainagents.Agent, error) {
	if r.sql != nil && r.sql.Gorm() != nil {
		return r.getAgentSQL(ctx, id)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, agent := range r.agents {
		if agent.ID == id {
			return agent, nil
		}
	}
	return domainagents.Agent{}, domainagents.ErrNotFound
}

func (r *agentsImpl) listAgentsSQL(ctx context.Context) ([]domainagents.Agent, error) {
	if err := r.ensureSchema(ctx); err != nil {
		return nil, err
	}

	var records []agentRecord
	if err := r.sql.Gorm().WithContext(ctx).Order("id ASC").Find(&records).Error; err != nil {
		return nil, err
	}
	out := make([]domainagents.Agent, 0, len(records))
	for _, record := range records {
		out = append(out, toDomainAgent(record))
	}
	return out, nil
}

func (r *agentsImpl) getAgentSQL(ctx context.Context, id string) (domainagents.Agent, error) {
	if err := r.ensureSchema(ctx); err != nil {
		return domainagents.Agent{}, err
	}

	var record agentRecord
	err := r.sql.Gorm().WithContext(ctx).Where("id = ?", id).First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domainagents.Agent{}, domainagents.ErrNotFound
	}
	if err != nil {
		return domainagents.Agent{}, err
	}
	return toDomainAgent(record), nil
}

func (r *agentsImpl) ensureSchema(ctx context.Context) error {
	r.migrateOnce.Do(func() {
		db := r.sql.Gorm().WithContext(ctx)
		if err := db.AutoMigrate(&agentRecord{}); err != nil {
			r.migrateErr = err
			return
		}
		var count int64
		if err := db.Model(&agentRecord{}).Count(&count).Error; err != nil {
			r.migrateErr = err
			return
		}
		if count > 0 {
			return
		}
		records := make([]agentRecord, 0, len(domainagents.SeedCatalog()))
		now := time.Now()
		for _, agent := range domainagents.SeedCatalog() {
			record := fromDomainAgent(agent)
			record.CreatedAt = now
			record.UpdatedAt = now
			records = append(records, record)
		}
		r.migrateErr = db.Create(&records).Error
	})
	return r.migrateErr
}

func toDomainAgent(record agentRecord) domainagents.Agent {
	return domainagents.Agent{
		ID:                record.ID,
		Name:              record.Name,
		Category:          record.Category,
		Tags:              decodeStrings(record.Tags),
		Description:       record.Description,
		PriceUses:         record.PriceUses,
		PriceCents:        record.PriceCents,
		Rating:            record.Rating,
		DealCount:         record.DealCount,
		EstimatedDuration: record.EstimatedDuration,
		Deliverable:       record.Deliverable,
		ArtifactTypes:     decodeStrings(record.ArtifactTypes),
	}
}

func fromDomainAgent(agent domainagents.Agent) agentRecord {
	return agentRecord{
		ID:                agent.ID,
		Name:              agent.Name,
		Category:          agent.Category,
		Tags:              encodeStrings(agent.Tags),
		Description:       agent.Description,
		PriceUses:         agent.PriceUses,
		PriceCents:        agent.PriceCents,
		Rating:            agent.Rating,
		DealCount:         agent.DealCount,
		EstimatedDuration: agent.EstimatedDuration,
		Deliverable:       agent.Deliverable,
		ArtifactTypes:     encodeStrings(agent.ArtifactTypes),
	}
}

func encodeStrings(values []string) string {
	payload, _ := json.Marshal(values)
	return string(payload)
}

func decodeStrings(value string) []string {
	if value == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(value), &out); err != nil {
		return nil
	}
	return out
}
