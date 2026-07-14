package tasks

import (
	"errors"
	"strings"
	"time"
)

var (
	ErrNotFound = errors.New("task not found")
	ErrInvalid  = errors.New("task is invalid")
)

type Status string

const (
	StatusQueued    Status = "queued"
	StatusReady     Status = "ready"
	StatusRunning   Status = "running"
	StatusPaused    Status = "paused"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
)

type ExecutionMode string

const (
	ExecutionImmediate ExecutionMode = "immediate"
	ExecutionQueued    ExecutionMode = "queued"
)

type QueueState string

const (
	QueueWaiting    QueueState = "waiting"
	QueueActive     QueueState = "active"
	QueueSuperseded QueueState = "superseded"
)

type QueueInfo struct {
	State            QueueState `json:"state"`
	EnqueuedAt       time.Time  `json:"enqueuedAt"`
	ActivatedAt      time.Time  `json:"activatedAt,omitempty"`
	ActivationSource string     `json:"activationSource,omitempty"`
	Authorized       bool       `json:"authorized,omitempty"`
}

type Attachment struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	OriginalPath string    `json:"originalPath,omitempty"`
	StoredPath   string    `json:"storedPath"`
	MIMEType     string    `json:"mimeType,omitempty"`
	Size         int64     `json:"size"`
	CreatedAt    time.Time `json:"createdAt"`
}

type Task struct {
	ID            string        `json:"id"`
	WorkspaceID   string        `json:"workspaceId"`
	Title         string        `json:"title"`
	Prompt        string        `json:"prompt"`
	WorkflowID    string        `json:"workflowId"`
	Status        Status        `json:"status"`
	ExecutionMode ExecutionMode `json:"executionMode,omitempty"`
	Queue         *QueueInfo    `json:"queue,omitempty"`
	Attachments   []Attachment  `json:"attachments,omitempty"`
	DeletedAt     time.Time     `json:"deletedAt,omitempty"`
	CreatedAt     time.Time     `json:"createdAt"`
	UpdatedAt     time.Time     `json:"updatedAt"`
}

func Validate(task Task) error {
	if strings.TrimSpace(task.ID) == "" || strings.TrimSpace(task.WorkspaceID) == "" || strings.TrimSpace(task.Title) == "" || strings.TrimSpace(task.Prompt) == "" || strings.TrimSpace(task.WorkflowID) == "" {
		return ErrInvalid
	}
	if task.ExecutionMode != "" && task.ExecutionMode != ExecutionImmediate && task.ExecutionMode != ExecutionQueued {
		return ErrInvalid
	}
	if task.Status == StatusQueued {
		if task.ExecutionMode != ExecutionQueued || task.Queue == nil || task.Queue.EnqueuedAt.IsZero() {
			return ErrInvalid
		}
		return nil
	}
	switch task.Status {
	case StatusReady, StatusRunning, StatusPaused, StatusCompleted, StatusFailed, StatusCancelled:
		return nil
	default:
		return ErrInvalid
	}
}
