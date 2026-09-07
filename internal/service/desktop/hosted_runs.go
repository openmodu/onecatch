package desktop

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"time"

	domaintasks "github.com/openmodu/onecatch/internal/domain/tasks"
	domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"
	"github.com/openmodu/onecatch/internal/service/worker"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

// hostedRuns uses the same repositories and orchestrator as the desktop UI.
// The worker's execute/patch protocol is for separate clones, not these tasks.
type hostedRuns struct {
	app      *Service
	importMu sync.Mutex
}

func (h *hostedRuns) workspace(ctx context.Context, id string) error {
	items, err := h.app.hostedWorkspaces(ctx)
	if err != nil {
		return err
	}
	for _, item := range items {
		if item.Mapping.ID == id {
			return nil
		}
	}
	return coded("workspace_not_found", "workspace is not shared by this desktop")
}

func (h *hostedRuns) List(ctx context.Context, cursor string) (worker.SharedRunPage, error) {
	workspaces, err := h.app.hostedWorkspaces(ctx)
	if err != nil {
		return worker.SharedRunPage{}, err
	}
	ids := []string{}
	tasks := map[string]domaintasks.Task{}
	for _, workspace := range workspaces {
		items, err := h.app.ListTasks(ctx, workspace.Mapping.ID)
		if err != nil {
			return worker.SharedRunPage{}, err
		}
		for _, task := range items {
			ids = append(ids, task.ID)
			tasks[task.ID] = task
		}
	}
	page, err := h.app.store.Repos.Workflows.ListRuns(ctx, domainworkflows.RunListQuery{TaskIDs: ids, Cursor: cursor, Limit: 100})
	if err != nil {
		return worker.SharedRunPage{}, err
	}
	result := worker.SharedRunPage{Items: []worker.SharedRun{}, NextCursor: page.NextCursor}
	for _, run := range page.Items {
		view := hostedRunView(run, tasks[run.TaskID], h.app.isActive(run.ID))
		if view.Runtime == "" {
			definition, err := h.app.store.Repos.Workflows.GetRunDefinition(ctx, run.ID)
			if err != nil {
				return worker.SharedRunPage{}, err
			}
			for _, step := range definition.Steps {
				if step.ID == run.CurrentStepID {
					view.Runtime = agentrun.Runtime(step.Runtime)
				}
			}
		}
		result.Items = append(result.Items, view)
	}
	return result, nil
}

func hostedRunView(run domainworkflows.Run, task domaintasks.Task, active bool) worker.SharedRun {
	status := string(run.Status)
	if active || run.Status == domainworkflows.RunReady {
		status = "running"
	} else if run.Status == domainworkflows.RunCompleted {
		status = "succeeded"
	}
	view := worker.SharedRun{ID: run.ID, ConversationID: task.ID, WorkerID: hostWorkerID(), WorkspaceID: task.WorkspaceID,
		Runtime: agentrun.Runtime(task.Harness), Prompt: task.Prompt, Title: task.Title,
		Status: status, Shared: true, StartedAt: run.StartedAt, Error: run.LastError}
	if !active && !run.CompletedAt.IsZero() {
		at := run.CompletedAt
		view.FinishedAt = &at
	}
	return view
}

func (h *hostedRuns) Get(ctx context.Context, id string) (worker.SharedRun, error) {
	detail, err := h.app.GetRunDetail(ctx, id)
	if err != nil {
		return worker.SharedRun{}, err
	}
	if err := h.workspace(ctx, detail.Task.WorkspaceID); err != nil {
		return worker.SharedRun{}, err
	}
	view := hostedRunView(detail.Run, detail.Task, detail.Active)
	if view.Runtime == "" {
		for _, step := range detail.Workflow.Steps {
			if step.ID == detail.Run.CurrentStepID {
				view.Runtime = agentrun.Runtime(step.Runtime)
			}
		}
	}
	transcript, err := h.app.GetRunTranscript(ctx, id)
	if err != nil {
		return worker.SharedRun{}, err
	}
	view.Events = []agentrun.Event{}
	indexes := map[string]int{}
	for _, item := range transcript {
		at, _ := time.Parse(time.RFC3339Nano, item.At)
		event := agentrun.Event{Kind: agentrun.EventKind(item.Kind), Text: item.Text, Failed: item.Failed, At: at, Revision: item.Revision,
			Permission: item.Permission, PermissionDecision: item.PermissionDecision, UserInput: item.UserInput, UserInputResponse: item.UserInputResponse}
		if item.StreamID != "" {
			event.StreamID = item.StepRunID + ":" + item.StreamID
			event.Phase = agentrun.StreamEnd
			if item.Streaming {
				event.Phase = agentrun.StreamSnapshot
			}
			if event.Kind == agentrun.KindToolUse {
				event.Phase = ""
			}
			indexes[event.StreamID+":"+item.Kind] = len(view.Events)
		}
		view.Events = append(view.Events, event)
	}
	// Include live snapshots that have not reached the durable event log yet.
	for _, frame := range h.app.GetRunStreamSnapshot(id) {
		key := frame.StepRunID + ":" + frame.StreamID
		event := agentrun.Event{Kind: frame.Kind, StreamID: key, Phase: frame.Phase, Text: frame.Text, At: frame.At, Failed: frame.Failed}
		if index, ok := indexes[key+":"+string(frame.Kind)]; ok {
			if frame.Revision > view.Events[index].Revision {
				view.Events[index] = event
			}
		} else {
			view.Events = append(view.Events, event)
		}
	}
	// Human follow-ups are workflow records, not runtime messages.
	for _, item := range detail.Events {
		if item.Type == "mobile.imported" {
			var payload struct {
				Prompt string `json:"prompt"`
			}
			if json.Unmarshal([]byte(item.Payload), &payload) == nil && payload.Prompt != "" {
				view.Prompt = payload.Prompt
			}
		}
		if item.Type != "run.resumed" {
			continue
		}
		var payload struct {
			Instruction string `json:"instruction"`
		}
		if json.Unmarshal([]byte(item.Payload), &payload) == nil && payload.Instruction != "" {
			at, _ := time.Parse(time.RFC3339Nano, item.At)
			view.Events = append(view.Events, agentrun.Event{Kind: "user_message", Text: payload.Instruction, At: at})
		}
	}
	for _, item := range detail.Instructions {
		if item.Status == domainworkflows.InstructionRemoved {
			continue
		}
		view.Events = append(view.Events, agentrun.Event{Kind: "user_message", Text: item.Content, At: item.CreatedAt})
	}
	sort.SliceStable(view.Events, func(i, j int) bool { return view.Events[i].At.Before(view.Events[j].At) })
	result := agentrun.Result{Succeeded: view.Status == "succeeded"}
	for _, step := range detail.StepRuns {
		result.Usage.InputTokens += step.InputTokens
		result.Usage.OutputTokens += step.OutputTokens
		result.Usage.CachedInputTokens += step.CachedInputTokens
		result.Usage.CacheCreationInputTokens += step.CacheCreationInputTokens
		result.Usage.ReasoningOutputTokens += step.ReasoningOutputTokens
		result.FinalMessage = step.Content
		result.SessionID = step.SessionIDAfter
	}
	view.Result = &result
	if detail.LastError != "" {
		view.Error = detail.LastError
	}
	return view, nil
}

func (h *hostedRuns) Start(ctx context.Context, input worker.SharedRunInput) (worker.SharedRun, error) {
	input.WorkspaceID = strings.TrimSpace(input.WorkspaceID)
	input.Prompt = strings.TrimSpace(input.Prompt)
	input.ConversationID = strings.TrimSpace(input.ConversationID)
	if input.Prompt == "" || !agentrun.Runtime(input.Runtime).Valid() {
		return worker.SharedRun{}, coded("task_invalid", "a prompt and valid runtime are required")
	}
	if err := h.workspace(ctx, input.WorkspaceID); err != nil {
		return worker.SharedRun{}, err
	}
	if input.ConversationID != "" {
		task, err := h.app.store.Repos.Tasks.GetTask(ctx, input.ConversationID)
		if err != nil || task.WorkspaceID != input.WorkspaceID {
			return worker.SharedRun{}, coded("task_not_found", "conversation was not found in this workspace")
		}
		runs, err := h.app.ListRunsByTask(ctx, task.ID)
		if err != nil {
			return worker.SharedRun{}, err
		}
		if len(runs) == 0 {
			return worker.SharedRun{}, coded("run_not_found", "conversation has no run")
		}
		sort.Slice(runs, func(i, j int) bool { return runs[i].StartedAt.After(runs[j].StartedAt) })
		run, err := h.app.ResumeRunConfigured(ctx, runs[0].ID, ResumeRunInput{Instruction: input.Prompt, Harness: input.Runtime, Model: input.Model, ReasoningEffort: input.ReasoningEffort, ServiceTier: input.ServiceTier})
		if err != nil {
			return worker.SharedRun{}, err
		}
		return h.Get(ctx, run.ID)
	}
	task, err := h.app.CreateTask(ctx, CreateTaskInput{WorkspaceID: input.WorkspaceID, Title: taskTitleFromPrompt(input.Prompt, "新建任务"), Prompt: input.Prompt,
		WorkflowID: directAgentWorkflowID, Sandbox: string(agentrun.SandboxWorkspaceWrite), Harness: input.Runtime,
		Model: input.Model, ReasoningEffort: input.ReasoningEffort, ServiceTier: input.ServiceTier})
	if err != nil {
		return worker.SharedRun{}, err
	}
	run, err := h.app.StartRun(ctx, task.ID)
	if err != nil {
		return worker.SharedRun{}, err
	}
	return h.Get(ctx, run.ID)
}

func (h *hostedRuns) Interrupt(ctx context.Context, id string) error {
	if _, err := h.Get(ctx, id); err != nil {
		return err
	}
	_, err := h.app.InterruptRun(ctx, id)
	return err
}
func (h *hostedRuns) RespondPermission(ctx context.Context, id, requestID, decision string) error {
	if _, err := h.Get(ctx, id); err != nil {
		return err
	}
	return h.app.RespondPermission(PermissionDecisionInput{RunID: id, RequestID: requestID, Decision: decision})
}
