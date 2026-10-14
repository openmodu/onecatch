package desktop

import (
	"context"
	tasks "github.com/openmodu/onecatch/internal/domain/tasks"
	workflows "github.com/openmodu/onecatch/internal/domain/workflows"
	workflowuc "github.com/openmodu/onecatch/internal/usecase/workflows"
)

func (a *Service) resolveExecutionSettings(ctx context.Context, task tasks.Task, definition workflows.Definition) (workflows.Definition, workflowuc.RunResolution, error) {
	settings, err := a.settings.Get(ctx)
	if err != nil {
		return definition, workflowuc.RunResolution{}, err
	}
	workspace, err := a.resolveTaskWorkspace(ctx, task)
	if err != nil {
		return definition, workflowuc.RunResolution{}, err
	}
	definition.Steps = append([]workflows.Step(nil), definition.Steps...)
	direct := definition.ID == "single_agent"
	if direct {
		definition.Policy = workflows.Policy{MaxReworkRounds: settings.Execution.MaxReworkRounds, MaxConsecutiveFailures: settings.Execution.MaxConsecutiveFailures, StepTimeoutSeconds: settings.Execution.StepTimeoutSeconds}
	}
	for i := range definition.Steps {
		step := &definition.Steps[i]
		if step.Kind == workflows.StepCommand {
			continue
		}
		if step.Model == "" && !(direct && step.WorkerID != "" && step.WorkerID != "local") {
			step.Model = settings.Runtimes[step.Runtime].DefaultModel
		}
		step.Sandbox = resolveSandbox(step.Sandbox, settings.Execution.DefaultSandbox, workspace.DefaultSandbox)
		if !direct && task.Sandbox != "" {
			step.Sandbox = capSandbox(step.Sandbox, task.Sandbox)
		}
	}
	if err := validateDefinitionSettings(definition, settings); err != nil {
		return definition, workflowuc.RunResolution{}, err
	}
	if err := validateHarnessSettings(definition, settings, workspace.RemoteFS != nil); err != nil {
		return definition, workflowuc.RunResolution{}, err
	}
	resolved := map[string]workflows.ResolvedRuntimeSettings{}
	for _, step := range definition.Steps {
		if step.Kind != workflows.StepCommand {
			value := resolvedRuntimeSetting(step.Runtime, settings.Runtimes[step.Runtime])
			if direct && step.WorkerID != "" && step.WorkerID != "local" {
				value = workflows.ResolvedRuntimeSettings{}
			}
			if task.Harness == step.Runtime {
				if task.ReasoningEffort != "" {
					value.ReasoningEffort = task.ReasoningEffort
				}
				if task.ServiceTier != "" {
					value.ServiceTier = task.ServiceTier
				}
			}
			resolved[step.Runtime] = value
		}
	}
	return definition, workflowuc.RunResolution{MaxConcurrency: settings.Execution.MaxConcurrency, InterruptGraceSeconds: settings.Execution.InterruptGraceSeconds, RuntimeSettings: resolved}, nil
}
