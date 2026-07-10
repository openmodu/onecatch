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
	StatusReady     Status = "ready"
	StatusRunning   Status = "running"
	StatusPaused    Status = "paused"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
)

type Task struct {
	ID          string    `json:"id"`
	WorkspaceID string    `json:"workspaceId"`
	Title       string    `json:"title"`
	Prompt      string    `json:"prompt"`
	WorkflowID  string    `json:"workflowId"`
	Status      Status    `json:"status"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

func Validate(task Task) error {
	if strings.TrimSpace(task.ID) == "" || strings.TrimSpace(task.WorkspaceID) == "" || strings.TrimSpace(task.Title) == "" || strings.TrimSpace(task.Prompt) == "" || strings.TrimSpace(task.WorkflowID) == "" {
		return ErrInvalid
	}
	switch task.Status {
	case StatusReady, StatusRunning, StatusPaused, StatusCompleted, StatusFailed, StatusCancelled:
		return nil
	default:
		return ErrInvalid
	}
}
