package desktop

import (
	"context"
	tasks "github.com/openmodu/onecatch/internal/domain/tasks"
	workflows "github.com/openmodu/onecatch/internal/domain/workflows"
)

func (a *Service) resolveExecutionSettings(ctx context.Context, task tasks.Task, definition workflows.Definition) (workflows.Definition, map[string]workflows.ResolvedRuntimeSettings, error) {
	settings, err := a.settings.Get(ctx)
	if err != nil {
		return definition, nil, err
	}
	for i := range definition.Steps {
		if definition.Steps[i].Model == "" {
			definition.Steps[i].Model = settings.Runtimes[definition.Steps[i].Runtime].DefaultModel
		}
	}
	if err := validateDefinitionSettings(definition, settings); err != nil {
		return definition, nil, err
	}
	workspace, err := a.store.Repos.Tasks.GetWorkspace(ctx, task.WorkspaceID)
	if err != nil {
		return definition, nil, err
	}
	if err := validateHarnessSettings(definition, settings, workspace.RemoteFS != nil); err != nil {
		return definition, nil, err
	}
	resolved := map[string]workflows.ResolvedRuntimeSettings{}
	for _, step := range definition.Steps {
		if step.Kind != workflows.StepCommand {
			resolved[step.Runtime] = resolvedRuntimeSetting(step.Runtime, settings.Runtimes[step.Runtime])
		}
	}
	return definition, resolved, nil
}
