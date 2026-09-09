package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	domaintasks "github.com/openmodu/onecatch/internal/domain/tasks"
	domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"
	repoworkflows "github.com/openmodu/onecatch/internal/repo/workflows"
	"github.com/openmodu/onecatch/internal/service/worker"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
	"github.com/openmodu/onecatch/pkg/localfile"
)

// Import adopts the old phone cache without re-executing anything. Stable IDs
// and an import-complete marker make reconnects and interrupted uploads safe.
func (h *hostedRuns) Import(ctx context.Context, input worker.SharedRun) (worker.SharedRun, error) {
	h.importMu.Lock()
	defer h.importMu.Unlock()
	if !strings.HasPrefix(input.ID, "mobile_") || !localfile.ValidID(input.ID) || !strings.HasPrefix(input.ConversationID, "mobile_") || !localfile.ValidID(input.ConversationID) || !input.Runtime.Valid() || strings.TrimSpace(input.Prompt) == "" || (input.Status != "succeeded" && input.Status != "failed") || input.StartedAt.IsZero() {
		return worker.SharedRun{}, coded("import_invalid", "only finished legacy mobile runs can be imported")
	}
	if err := h.workspace(ctx, input.WorkspaceID); err != nil {
		return worker.SharedRun{}, err
	}
	repo := h.app.store.Repos.Workflows
	existing, err := repo.GetRun(ctx, input.ID)
	if err != nil && !errors.Is(err, repoworkflows.ErrRunNotFound) {
		return worker.SharedRun{}, err
	}
	if err == nil {
		if existing.TaskID != input.ConversationID {
			return worker.SharedRun{}, coded("import_conflict", "run belongs to another conversation")
		}
		events, err := repo.ListEvents(ctx, input.ID, 0, 10000)
		if err != nil {
			return worker.SharedRun{}, err
		}
		for _, event := range events {
			if event.Type == "mobile.imported" {
				return h.Get(ctx, input.ID, worker.TranscriptWindow{})
			}
		}
	}
	definition, err := h.app.GetDefinition(ctx, directAgentWorkflowID)
	if err != nil {
		return worker.SharedRun{}, err
	}
	definition.Steps = append([]domainworkflows.Step(nil), definition.Steps...)
	definition.Steps[0].Runtime = string(input.Runtime)
	// Imported history now belongs to the regular coding workflow. Any next
	// turn uses the same workspace policy as a newly created shared task.
	definition.Steps[0].Sandbox = string(agentrun.SandboxWorkspaceWrite)
	finished := input.StartedAt
	if input.FinishedAt != nil {
		finished = *input.FinishedAt
	}
	task, taskErr := h.app.store.Repos.Tasks.GetTask(ctx, input.ConversationID)
	if taskErr != nil && !errors.Is(taskErr, domaintasks.ErrNotFound) {
		return worker.SharedRun{}, taskErr
	}
	if taskErr == nil && (task.WorkspaceID != input.WorkspaceID || task.Harness != string(input.Runtime)) {
		return worker.SharedRun{}, coded("import_conflict", "conversation belongs to another workspace or runtime")
	}
	if taskErr != nil {
		task = domaintasks.Task{ID: input.ConversationID, WorkspaceID: input.WorkspaceID, WorkflowID: definition.ID,
			Title: taskTitleFromPrompt(input.Prompt, "移动端任务"), Prompt: input.Prompt, Harness: string(input.Runtime),
			Sandbox: string(agentrun.SandboxWorkspaceWrite), Status: domaintasks.StatusCompleted, ExecutionMode: domaintasks.ExecutionImmediate,
			CreatedAt: input.StartedAt, UpdatedAt: finished}
		if input.Status == "failed" {
			task.Status = domaintasks.StatusPaused
		}
		if err := h.app.store.Repos.Tasks.SaveTask(ctx, task); err != nil {
			return worker.SharedRun{}, err
		}
	}
	step := domainworkflows.StepRun{ID: input.ID + "_import", RunID: input.ID, StepID: definition.EntryStepID, Attempt: 1,
		Status: domainworkflows.StepRunSucceeded, StartedAt: input.StartedAt, FinishedAt: finished, Error: input.Error}
	run := domainworkflows.Run{ID: input.ID, TaskID: task.ID, WorkflowID: definition.ID, CurrentStepID: definition.EntryStepID, Revision: 1,
		Status: domainworkflows.RunCompleted, StartedAt: input.StartedAt, UpdatedAt: finished, CompletedAt: finished, LastError: input.Error, Sessions: map[string]string{}}
	if input.Status == "failed" {
		run.Status = domainworkflows.RunPaused
		run.PauseReason = "mobile_import_failed"
		step.Status = domainworkflows.StepRunFailed
	}
	if input.Result != nil {
		result := input.Result
		step.Content, step.SessionIDAfter = result.FinalMessage, result.SessionID
		step.InputTokens, step.OutputTokens = result.Usage.InputTokens, result.Usage.OutputTokens
		step.CachedInputTokens, step.CacheCreationInputTokens, step.ReasoningOutputTokens = result.Usage.CachedInputTokens, result.Usage.CacheCreationInputTokens, result.Usage.ReasoningOutputTokens
		run.Sessions[definition.EntryStepID] = result.SessionID
	}
	if existing.ID == "" {
		if err := repo.SaveRun(ctx, run, definition); err != nil {
			return worker.SharedRun{}, err
		}
	}
	if err := repo.SaveStepRun(ctx, step); err != nil {
		return worker.SharedRun{}, err
	}
	// A retry appends only the suffix not persisted by the previous attempt.
	events, err := repo.ListRuntimeEvents(ctx, input.ID, step.ID, 0, 10000)
	if err != nil {
		return worker.SharedRun{}, err
	}
	for i := len(events); i < len(input.Events); i++ {
		raw, err := json.Marshal(input.Events[i])
		if err != nil {
			return worker.SharedRun{}, err
		}
		if _, err := repo.AppendRuntimeEvent(ctx, input.ID, step.ID, raw); err != nil {
			return worker.SharedRun{}, err
		}
	}
	if _, err := repo.AppendEvent(ctx, domainworkflows.WorkflowEvent{RunID: input.ID, Type: "mobile.imported", Payload: localEventPayload(map[string]any{"prompt": input.Prompt, "originalSandbox": "read-only", "originalStatus": input.Status}), At: time.Now().UTC()}); err != nil {
		return worker.SharedRun{}, err
	}
	return h.Get(ctx, input.ID, worker.TranscriptWindow{})
}
