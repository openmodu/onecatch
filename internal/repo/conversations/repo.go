package conversations

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	domainconversations "github.com/openmodu/oneshot/internal/domain/conversations"
	pkgsql "github.com/openmodu/oneshot/pkg/sql"
	"gorm.io/gorm"
)

type ConversationsRepo interface {
	NextConversationID(context.Context) (string, error)
	NextMessageID(context.Context) (string, error)
	Create(context.Context, domainconversations.Conversation) error
	Get(ctx context.Context, userID string, id string) (domainconversations.Conversation, error)
	AppendMessage(context.Context, domainconversations.Message) error
	UpdateMeta(context.Context, domainconversations.Conversation) error
}

type conversationsImpl struct {
	sql *pkgsql.Sql
	mu  sync.RWMutex

	conversations map[string]domainconversations.Conversation
}

type conversationRecord struct {
	ID                 string `gorm:"primaryKey;size:64"`
	UserID             string `gorm:"size:64;not null;index"`
	AgentID            string `gorm:"size:64;not null"`
	AgentName          string `gorm:"size:255"`
	Status             string `gorm:"size:32;not null"`
	OrderID            string `gorm:"size:64;index"`
	PendingRequirement string `gorm:"type:text"`
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type messageRecord struct {
	Seq            uint   `gorm:"primaryKey;autoIncrement"`
	ID             string `gorm:"size:64;uniqueIndex;not null"`
	ConversationID string `gorm:"size:64;not null;index"`
	Role           string `gorm:"size:16;not null"`
	Kind           string `gorm:"size:16;not null"`
	Text           string `gorm:"type:text"`
	CreatedAt      time.Time
}

func (conversationRecord) TableName() string { return "conversations" }
func (messageRecord) TableName() string      { return "conversation_messages" }

func NewConversationsRepo(sql *pkgsql.Sql) ConversationsRepo {
	return &conversationsImpl{
		sql:           sql,
		conversations: make(map[string]domainconversations.Conversation),
	}
}

// Migrate creates or updates this repo's tables. Called once at startup.
func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(&conversationRecord{}, &messageRecord{})
}

func (r *conversationsImpl) NextConversationID(context.Context) (string, error) {
	return randomID("conv_")
}

func (r *conversationsImpl) NextMessageID(context.Context) (string, error) {
	return randomID("msg_")
}

func (r *conversationsImpl) Create(ctx context.Context, conv domainconversations.Conversation) error {
	if r.sql != nil && r.sql.Gorm() != nil {
		return r.sql.Gorm().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Create(fromDomainConversation(conv)).Error; err != nil {
				return err
			}
			for _, msg := range conv.Messages {
				if err := tx.Create(fromDomainMessage(msg)).Error; err != nil {
					return err
				}
			}
			return nil
		})
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.conversations[conv.ID] = conv
	return nil
}

func (r *conversationsImpl) Get(ctx context.Context, userID string, id string) (domainconversations.Conversation, error) {
	if r.sql != nil && r.sql.Gorm() != nil {
		var record conversationRecord
		err := r.sql.Gorm().WithContext(ctx).Where("id = ? AND user_id = ?", id, userID).First(&record).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domainconversations.Conversation{}, domainconversations.ErrNotFound
		}
		if err != nil {
			return domainconversations.Conversation{}, err
		}
		var messages []messageRecord
		if err := r.sql.Gorm().WithContext(ctx).Where("conversation_id = ?", id).Order("seq ASC").Find(&messages).Error; err != nil {
			return domainconversations.Conversation{}, err
		}
		return toDomainConversation(record, messages), nil
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	conv, ok := r.conversations[id]
	if !ok || conv.UserID != userID {
		return domainconversations.Conversation{}, domainconversations.ErrNotFound
	}
	return conv, nil
}

func (r *conversationsImpl) AppendMessage(ctx context.Context, msg domainconversations.Message) error {
	if r.sql != nil && r.sql.Gorm() != nil {
		return r.sql.Gorm().WithContext(ctx).Create(fromDomainMessage(msg)).Error
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	conv, ok := r.conversations[msg.ConversationID]
	if !ok {
		return domainconversations.ErrNotFound
	}
	conv.Messages = append(conv.Messages, msg)
	r.conversations[msg.ConversationID] = conv
	return nil
}

func (r *conversationsImpl) UpdateMeta(ctx context.Context, conv domainconversations.Conversation) error {
	if r.sql != nil && r.sql.Gorm() != nil {
		return r.sql.Gorm().WithContext(ctx).Model(&conversationRecord{}).
			Where("id = ?", conv.ID).
			UpdateColumns(map[string]any{
				"status":              string(conv.Status),
				"order_id":            conv.OrderID,
				"pending_requirement": conv.PendingRequirement,
				"updated_at":          conv.UpdatedAt,
			}).Error
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	current, ok := r.conversations[conv.ID]
	if !ok {
		return domainconversations.ErrNotFound
	}
	current.Status = conv.Status
	current.OrderID = conv.OrderID
	current.PendingRequirement = conv.PendingRequirement
	current.UpdatedAt = conv.UpdatedAt
	r.conversations[conv.ID] = current
	return nil
}

func fromDomainConversation(conv domainconversations.Conversation) *conversationRecord {
	return &conversationRecord{
		ID:                 conv.ID,
		UserID:             conv.UserID,
		AgentID:            conv.AgentID,
		AgentName:          conv.AgentName,
		Status:             string(conv.Status),
		OrderID:            conv.OrderID,
		PendingRequirement: conv.PendingRequirement,
		CreatedAt:          conv.CreatedAt,
		UpdatedAt:          conv.UpdatedAt,
	}
}

func fromDomainMessage(msg domainconversations.Message) *messageRecord {
	return &messageRecord{
		ID:             msg.ID,
		ConversationID: msg.ConversationID,
		Role:           string(msg.Role),
		Kind:           string(msg.Kind),
		Text:           msg.Text,
		CreatedAt:      msg.CreatedAt,
	}
}

func toDomainConversation(record conversationRecord, messages []messageRecord) domainconversations.Conversation {
	out := domainconversations.Conversation{
		ID:                 record.ID,
		UserID:             record.UserID,
		AgentID:            record.AgentID,
		AgentName:          record.AgentName,
		Status:             domainconversations.Status(record.Status),
		OrderID:            record.OrderID,
		PendingRequirement: record.PendingRequirement,
		CreatedAt:          record.CreatedAt,
		UpdatedAt:          record.UpdatedAt,
		Messages:           make([]domainconversations.Message, 0, len(messages)),
	}
	for _, m := range messages {
		out.Messages = append(out.Messages, domainconversations.Message{
			ID:             m.ID,
			ConversationID: m.ConversationID,
			Role:           domainconversations.Role(m.Role),
			Kind:           domainconversations.MessageKind(m.Kind),
			Text:           m.Text,
			CreatedAt:      m.CreatedAt,
		})
	}
	return out
}

func randomID(prefix string) (string, error) {
	buf := make([]byte, 10)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}
	return prefix + hex.EncodeToString(buf), nil
}
