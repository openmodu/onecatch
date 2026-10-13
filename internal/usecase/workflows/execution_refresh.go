package workflows

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"

	domaintasks "github.com/openmodu/onecatch/internal/domain/tasks"
	workflows "github.com/openmodu/onecatch/internal/domain/workflows"
)

var errExecutionSettings = errors.New("execution settings are invalid")

func (s *Usecase) SetExecutionSettingsResolver(fn func(context.Context, domaintasks.Task, workflows.Definition) (workflows.Definition, map[string]workflows.ResolvedRuntimeSettings, error)) {
	s.resolveExecutionSettings = fn
}

// Refresh only nodes about to start. In-flight calls retain their immutable
// snapshots; graph, prompt and rework edits never leak into an existing run.
func (s *Usecase) refreshStartingSettings(ctx context.Context, task domaintasks.Task, definition workflows.Definition, run *workflows.Run, ready []workflows.Step, preserve []string) (workflows.Definition, error) {
	if definition.ID == directAgentWorkflowID {
		return definition, nil
	}
	resolve := s.resolveDefinition
	if resolve == nil {
		resolve = func(ctx context.Context, task domaintasks.Task) (workflows.Definition, error) {
			return s.workflows.GetDefinition(ctx, task.WorkflowID)
		}
	}
	latest, err := resolve(ctx, task)
	if err != nil {
		return definition, nil
	} // Removed templates stay resumable.
	candidate := RefreshStepModels(definition, latest)
	for i, step := range candidate.Steps {
		starting := false
		for _, r := range ready {
			if r.ID == step.ID {
				starting = true
				break
			}
		}
		if !starting || slices.Contains(preserve, step.ID) {
			candidate.Steps[i] = definition.Steps[i]
		}
	}
	var settings map[string]workflows.ResolvedRuntimeSettings
	if s.resolveExecutionSettings != nil {
		candidate, settings, err = s.resolveExecutionSettings(ctx, task, candidate)
		if err != nil {
			return definition, fmt.Errorf("%w: %v", errExecutionSettings, err)
		}
	}
	if err := workflows.Validate(candidate); err != nil {
		return definition, fmt.Errorf("%w: %v", errExecutionSettings, err)
	}
	changedSession := false
	for _, step := range candidate.Steps {
		old, _ := workflows.FindStep(definition, step.ID)
		if old.Runtime != step.Runtime || old.WorkerID != step.WorkerID {
			delete(run.Sessions, step.ID)
			changedSession = true
		}
	}
	if len(settings) > 0 {
		if run.RuntimeSettings == nil {
			run.RuntimeSettings = map[string]workflows.ResolvedRuntimeSettings{}
		}
		for id, value := range settings {
			run.RuntimeSettings[id] = value
		}
	}
	if changedSession {
		saved, err := s.workflows.UpdateRun(ctx, *run, run.Revision)
		if err != nil {
			return definition, err
		}
		*run = saved
	}
	if !reflect.DeepEqual(definition, candidate) {
		if err := s.workflows.UpdateRunDefinition(ctx, run.ID, candidate); err != nil {
			return definition, err
		}
	}
	return candidate, nil
}

// A completed outer round can safely adopt additions/removals and new edges.
func (s *Usecase) refreshLoopDefinition(ctx context.Context, task domaintasks.Task, definition workflows.Definition, run *workflows.Run) (workflows.Definition, error) {
	resolve := s.resolveDefinition
	if resolve == nil {
		resolve = func(ctx context.Context, task domaintasks.Task) (workflows.Definition, error) {
			return s.workflows.GetDefinition(ctx, task.WorkflowID)
		}
	}
	latest, err := resolve(ctx, task)
	if err != nil {
		return definition, nil
	}
	candidate := RefreshRunDefinition(definition, latest)
	if s.resolveExecutionSettings != nil {
		var settings map[string]workflows.ResolvedRuntimeSettings
		candidate, settings, err = s.resolveExecutionSettings(ctx, task, candidate)
		if err != nil {
			return definition, err
		}
		if run.RuntimeSettings == nil {
			run.RuntimeSettings = map[string]workflows.ResolvedRuntimeSettings{}
		}
		for id, value := range settings {
			run.RuntimeSettings[id] = value
		}
	}
	for _, old := range definition.Steps {
		next, ok := workflows.FindStep(candidate, old.ID)
		if !ok || old.Runtime != next.Runtime || old.WorkerID != next.WorkerID || old.Kind != next.Kind {
			delete(run.Sessions, old.ID)
		}
	}
	if err := workflows.Validate(candidate); err != nil {
		return definition, err
	}
	saved, err := s.workflows.UpdateRun(context.Background(), *run, run.Revision)
	if err != nil {
		return definition, err
	}
	*run = saved
	if !reflect.DeepEqual(definition, candidate) {
		if err := s.workflows.UpdateRunDefinition(context.Background(), run.ID, candidate); err != nil {
			return definition, err
		}
	}
	return candidate, nil
}
