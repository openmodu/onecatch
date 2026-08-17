package desktop

import (
	"context"
	"errors"
	"os"
	"strings"
	"time"

	domainsettings "github.com/openmodu/onecatch/internal/domain/settings"
	domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"
	settingsrepo "github.com/openmodu/onecatch/internal/repo/settings"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
	workflowuc "github.com/openmodu/onecatch/internal/usecase/workflows"
)

type RuntimeDraftInput struct {
	Runtime  string                         `json:"runtime"`
	Settings domainsettings.RuntimeSettings `json:"settings"`
}

type RunStartPreview struct {
	RequiresFullSandboxConfirmation bool      `json:"requiresFullSandboxConfirmation"`
	ConfirmationToken               string    `json:"confirmationToken,omitempty"`
	ExpiresAt                       time.Time `json:"expiresAt,omitempty"`
}
type runConfirmation struct {
	TaskID, WorkspaceID, WorkflowID string
	WorkflowUpdatedAt               time.Time
	ExpiresAt                       time.Time
}

func (a *Service) InitializeSettings(ctx context.Context) error {
	settings, err := a.settings.Get(ctx)
	if err != nil {
		return mapSettingsError(err)
	}
	return a.applySettings(settings)
}

func (a *Service) SetSettingsReload(reload func(domainsettings.Settings) error) {
	a.settingsReload = reload
}

func (a *Service) GetSettings(ctx context.Context) (domainsettings.Settings, error) {
	value, err := a.settings.Get(ctx)
	if err != nil {
		return value, mapSettingsError(err)
	}
	return value, nil
}

func (a *Service) UpdateRuntimeSettings(ctx context.Context, value map[string]domainsettings.RuntimeSettings, expected int64) (domainsettings.Settings, error) {
	for id, item := range value {
		if strings.TrimSpace(item.Binary) == "" {
			continue
		}
		status, err := a.runtimes.CheckDraft(id, item)
		if err != nil || !status.Available {
			return domainsettings.Settings{}, coded("runtime_draft_unavailable", "binary is not executable")
		}
	}
	return a.updateSettings(ctx, expected, func(current *domainsettings.Settings) { current.Runtimes = value })
}
func (a *Service) UpdateTerminalSettings(ctx context.Context, value domainsettings.TerminalSettings, expected int64) (domainsettings.Settings, error) {
	return a.updateSettings(ctx, expected, func(current *domainsettings.Settings) { current.Terminal = value })
}
func (a *Service) UpdateExecutionSettings(ctx context.Context, value domainsettings.ExecutionSettings, expected int64) (domainsettings.Settings, error) {
	return a.updateSettings(ctx, expected, func(current *domainsettings.Settings) { current.Execution = value })
}
func (a *Service) UpdateSecuritySettings(ctx context.Context, value domainsettings.SecuritySettings, expected int64) (domainsettings.Settings, error) {
	return a.updateSettings(ctx, expected, func(current *domainsettings.Settings) { current.Security = value })
}
func (a *Service) UpdateStorageSettings(ctx context.Context, value domainsettings.StorageSettings, expected int64) (domainsettings.Settings, error) {
	return a.updateSettings(ctx, expected, func(current *domainsettings.Settings) { current.Storage = value })
}
func (a *Service) UpdateExperimentalSettings(ctx context.Context, value domainsettings.ExperimentalSettings, expected int64) (domainsettings.Settings, error) {
	return a.updateSettings(ctx, expected, func(current *domainsettings.Settings) { current.Experimental = value })
}

func (a *Service) ResetSettingsSection(ctx context.Context, section string, expected int64) (domainsettings.Settings, error) {
	if _, err := domainsettings.DefaultSection(section); err != nil {
		return domainsettings.Settings{}, coded("settings_invalid", err.Error())
	}
	return a.updateSettings(ctx, expected, func(current *domainsettings.Settings) {
		d := domainsettings.Defaults()
		switch section {
		case domainsettings.SectionRuntime:
			current.Runtimes = d.Runtimes
		case domainsettings.SectionTerminal:
			current.Terminal = d.Terminal
		case domainsettings.SectionExecution:
			current.Execution = d.Execution
		case domainsettings.SectionSecurity:
			current.Security = d.Security
		case domainsettings.SectionStorage:
			current.Storage = d.Storage
		case domainsettings.SectionExperimental:
			current.Experimental = d.Experimental
		}
	})
}

func (a *Service) CheckRuntimeDraft(input RuntimeDraftInput) (RuntimeInfo, error) {
	normalized, err := domainsettings.Normalize(domainsettings.Settings{SchemaVersion: 1, Revision: 1, Runtimes: map[string]domainsettings.RuntimeSettings{input.Runtime: input.Settings}})
	if err != nil {
		return RuntimeInfo{}, coded("settings_invalid", err.Error())
	}
	if err := domainsettings.Validate(normalized); err != nil {
		return RuntimeInfo{}, coded("settings_invalid", err.Error())
	}
	status, err := a.runtimes.CheckDraft(strings.TrimSpace(input.Runtime), input.Settings)
	if err != nil {
		return status, err
	}
	if !status.Available {
		return status, coded("runtime_draft_unavailable", "binary is not executable")
	}
	return status, nil
}

func (a *Service) InspectCodexConfiguration(ctx context.Context, input domainsettings.RuntimeSettings) (agentrun.CodexConfiguration, error) {
	settings := domainsettings.Defaults()
	settings.Runtimes["codex"] = input
	normalized, err := domainsettings.Normalize(settings)
	if err != nil {
		return agentrun.CodexConfiguration{}, coded("settings_invalid", err.Error())
	}
	if err := domainsettings.Validate(normalized); err != nil {
		return agentrun.CodexConfiguration{}, coded("settings_invalid", err.Error())
	}
	input = normalized.Runtimes["codex"]
	status, err := a.runtimes.CheckDraft("codex", input)
	if err != nil || !status.Available {
		return agentrun.CodexConfiguration{}, coded("runtime_draft_unavailable", "Codex binary is not executable")
	}
	home, _ := os.UserHomeDir()
	inspectCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	return agentrun.NewCodexRunner(input.Binary).InspectConfiguration(inspectCtx, home, allowedEnvironment(input.EnvironmentAllowlist))
}

func (a *Service) InspectClaudeConfiguration(ctx context.Context, input domainsettings.RuntimeSettings) (agentrun.ClaudeConfiguration, error) {
	settings := domainsettings.Defaults()
	settings.Runtimes["claude"] = input
	normalized, err := domainsettings.Normalize(settings)
	if err != nil {
		return agentrun.ClaudeConfiguration{}, coded("settings_invalid", err.Error())
	}
	if err := domainsettings.Validate(normalized); err != nil {
		return agentrun.ClaudeConfiguration{}, coded("settings_invalid", err.Error())
	}
	input = normalized.Runtimes["claude"]
	status, err := a.runtimes.CheckDraft("claude", input)
	if err != nil || !status.Available {
		return agentrun.ClaudeConfiguration{}, coded("runtime_draft_unavailable", "Claude Code binary is not executable")
	}
	home, _ := os.UserHomeDir()
	inspectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return agentrun.NewClaudeRunner(input.Binary).InspectConfiguration(inspectCtx, home, allowedEnvironment(input.EnvironmentAllowlist))
}

func (a *Service) updateSettings(ctx context.Context, expected int64, mutate func(*domainsettings.Settings)) (domainsettings.Settings, error) {
	current, err := a.settings.Get(ctx)
	if err != nil {
		return current, mapSettingsError(err)
	}
	if current.Revision != expected {
		return current, coded("settings_state_conflict", "settings were changed by another window")
	}
	mutate(&current)
	current, err = domainsettings.Normalize(current)
	if err != nil {
		return current, coded("settings_invalid", err.Error())
	}
	if err := domainsettings.Validate(current); err != nil {
		return current, coded("settings_invalid", err.Error())
	}
	saved, err := a.settings.Save(ctx, current, expected)
	if err != nil {
		return saved, mapSettingsError(err)
	}
	if err := a.applySettings(saved); err != nil {
		return saved, coded("settings_reload_failed", err.Error())
	}
	return saved, nil
}

func (a *Service) applySettings(value domainsettings.Settings) error {
	a.runtimes.ApplySettings(value.Runtimes, value.Execution.InterruptGraceSeconds)
	a.orchestrator.SetMaxDAGConcurrency(value.Execution.MaxLocalDAGConcurrency)
	if a.settingsReload != nil {
		return a.settingsReload(value)
	}
	return nil
}

func (a *Service) PreviewRun(ctx context.Context, taskID string) (RunStartPreview, error) {
	task, err := a.store.Repos.Tasks.GetTask(ctx, taskID)
	if err != nil {
		return RunStartPreview{}, mapError(err)
	}
	definition, _, err := a.resolveRunSettings(ctx, task.ID)
	if err != nil {
		return RunStartPreview{}, err
	}
	settings, err := a.settings.Get(ctx)
	if err != nil {
		return RunStartPreview{}, mapSettingsError(err)
	}
	if err := validateDefinitionSettings(definition, settings); err != nil {
		return RunStartPreview{}, err
	}
	usesFull := definitionUsesFullSandbox(definition, "")
	if usesFull && !settings.Security.AllowFullSandbox {
		return RunStartPreview{}, coded("security_full_sandbox_disabled", "Full access sandbox is disabled in Settings")
	}
	requires := settings.Security.ConfirmFullSandboxEveryRun && usesFull
	preview := RunStartPreview{RequiresFullSandboxConfirmation: requires}
	if requires {
		preview.ConfirmationToken, preview.ExpiresAt = randomToken(), time.Now().UTC().Add(2*time.Minute)
		a.cleanupMu.Lock()
		a.confirmations[preview.ConfirmationToken] = runConfirmation{TaskID: task.ID, WorkspaceID: task.WorkspaceID, WorkflowID: definition.ID, WorkflowUpdatedAt: definition.UpdatedAt, ExpiresAt: preview.ExpiresAt}
		a.cleanupMu.Unlock()
	}
	return preview, nil
}

func (a *Service) validateTaskSecurity(ctx context.Context, taskID, confirmationToken string) error {
	task, err := a.store.Repos.Tasks.GetTask(ctx, taskID)
	if err != nil {
		return mapError(err)
	}
	definition, _, err := a.resolveRunSettings(ctx, task.ID)
	if err != nil {
		return err
	}
	settings, err := a.settings.Get(ctx)
	if err != nil {
		return mapSettingsError(err)
	}
	if err := validateDefinitionSettings(definition, settings); err != nil {
		return err
	}
	usesFull := definitionUsesFullSandbox(definition, "")
	if usesFull && !settings.Security.AllowFullSandbox {
		return coded("security_full_sandbox_disabled", "Full access sandbox is disabled in Settings")
	}
	if settings.Security.ConfirmFullSandboxEveryRun && usesFull {
		a.cleanupMu.Lock()
		confirmation, ok := a.confirmations[confirmationToken]
		delete(a.confirmations, confirmationToken)
		a.cleanupMu.Unlock()
		if !ok || time.Now().UTC().After(confirmation.ExpiresAt) || confirmation.TaskID != task.ID || confirmation.WorkspaceID != task.WorkspaceID || confirmation.WorkflowID != definition.ID || !confirmation.WorkflowUpdatedAt.Equal(definition.UpdatedAt) {
			return coded("security_full_sandbox_confirmation_required", "Full access must be confirmed again before starting")
		}
	}
	return nil
}

func validateDefinitionSettings(definition domainworkflows.Definition, settings domainsettings.Settings) error {
	for _, step := range definition.Steps {
		if step.Sandbox == "full" && !settings.Security.AllowFullSandbox {
			return coded("security_full_sandbox_disabled", "Full access sandbox is disabled in Settings")
		}
		if step.WorkerID != "" && step.WorkerID != "local" && !settings.Experimental.RemoteWorkersEnabled {
			return coded("experimental_remote_workers_disabled", "remote workers are disabled in Settings")
		}
	}
	return nil
}

func definitionUsesFullSandbox(definition domainworkflows.Definition, workspaceDefault string) bool {
	for _, step := range definition.Steps {
		if step.Sandbox == "full" || (step.Sandbox == "" && workspaceDefault == "full") {
			return true
		}
	}
	return false
}

func (a *Service) resolveRunSettings(ctx context.Context, taskID string) (domainworkflows.Definition, workflowuc.RunResolution, error) {
	task, err := a.store.Repos.Tasks.GetTask(ctx, taskID)
	if err != nil {
		return domainworkflows.Definition{}, workflowuc.RunResolution{}, mapError(err)
	}
	workspace, err := a.store.Repos.Tasks.GetWorkspace(ctx, task.WorkspaceID)
	if err != nil {
		return domainworkflows.Definition{}, workflowuc.RunResolution{}, mapError(err)
	}
	definition, err := a.store.Repos.Workflows.GetDefinition(ctx, task.WorkflowID)
	if err != nil {
		return domainworkflows.Definition{}, workflowuc.RunResolution{}, mapError(err)
	}
	settings, err := a.settings.Get(ctx)
	if err != nil {
		return domainworkflows.Definition{}, workflowuc.RunResolution{}, mapSettingsError(err)
	}
	definition.Steps = append([]domainworkflows.Step(nil), definition.Steps...)
	directAgent := task.Harness != "" && definition.ID == "single_agent" && len(definition.Steps) == 1
	for index := range definition.Steps {
		step := &definition.Steps[index]
		if directAgent {
			step.Runtime = task.Harness
			step.Model = task.Model
			if step.Model == "" {
				step.Model = settings.Runtimes[step.Runtime].DefaultModel
			}
		} else if task.Harness != "" && step.Runtime == task.Harness && task.Model != "" {
			step.Model = task.Model
		} else if step.Model == "" {
			step.Model = settings.Runtimes[step.Runtime].DefaultModel
		}
		if directAgent && task.Sandbox != "" {
			step.Sandbox = task.Sandbox
		}
		step.Sandbox = resolveSandbox(step.Sandbox, settings.Execution.DefaultSandbox, workspace.DefaultSandbox)
		if !directAgent && task.Sandbox != "" {
			step.Sandbox = capSandbox(step.Sandbox, task.Sandbox)
		}
	}
	runtimeSettings := make(map[string]domainworkflows.ResolvedRuntimeSettings, len(settings.Runtimes))
	for id, item := range settings.Runtimes {
		runtimeSettings[id] = resolvedRuntimeSetting(id, item)
	}
	if task.Harness != "" {
		resolved := runtimeSettings[task.Harness]
		if task.ReasoningEffort != "" {
			resolved.ReasoningEffort = task.ReasoningEffort
		}
		if task.ServiceTier != "" {
			resolved.ServiceTier = task.ServiceTier
		}
		runtimeSettings[task.Harness] = resolved
	}
	return definition, workflowuc.RunResolution{MaxLocalDAGConcurrency: settings.Execution.MaxLocalDAGConcurrency, InterruptGraceSeconds: settings.Execution.InterruptGraceSeconds, RuntimeSettings: runtimeSettings}, nil
}

func resolvedRuntimeSetting(id string, item domainsettings.RuntimeSettings) domainworkflows.ResolvedRuntimeSettings {
	provider := item.Provider
	if id == "modu" && provider == "" {
		provider = "auto"
	}
	return domainworkflows.ResolvedRuntimeSettings{
		EnvironmentAllowlist: append([]string{}, item.EnvironmentAllowlist...),
		Provider:             provider,
		ReasoningEffort:      item.ReasoningEffort,
		ServiceTier:          item.ServiceTier,
	}
}

func resolveSandbox(explicit, globalDefault, workspaceMaximum string) string {
	value := explicit
	if value == "" {
		value = globalDefault
	}
	if value == "" {
		value = "workspace-write"
	}
	maximum := workspaceMaximum
	if maximum == "" {
		maximum = "workspace-write"
	}
	rank := map[string]int{"read-only": 0, "workspace-write": 1, "full": 2}
	if rank[value] > rank[maximum] {
		return maximum
	}
	return value
}

func capSandbox(value, maximum string) string {
	rank := map[string]int{"read-only": 0, "workspace-write": 1, "full": 2}
	if rank[value] > rank[maximum] {
		return maximum
	}
	return value
}

func mapSettingsError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, settingsrepo.ErrStateConflict) {
		return coded("settings_state_conflict", "settings were changed by another window")
	}
	message := err.Error()
	if strings.Contains(message, "migration failed") {
		return coded("settings_migration_failed", message)
	}
	if strings.Contains(message, "validate") || strings.Contains(message, "must be") || strings.Contains(message, "not allowed") {
		return coded("settings_invalid", message)
	}
	return err
}
