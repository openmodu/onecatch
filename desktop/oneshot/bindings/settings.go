package bindings

import (
	"context"

	"github.com/openmodu/oneshot/internal/agentrun"
	"github.com/openmodu/oneshot/internal/app/localapp"
	domainsettings "github.com/openmodu/oneshot/internal/domain/settings"
)

type SettingsBinding struct{ app *localapp.App }

func NewSettingsBinding(app *localapp.App) *SettingsBinding { return &SettingsBinding{app: app} }
func (b *SettingsBinding) GetSettings() (domainsettings.Settings, error) {
	return b.app.GetSettings(context.Background())
}
func (b *SettingsBinding) UpdateRuntimeSettings(input map[string]domainsettings.RuntimeSettings, expectedRevision int64) (domainsettings.Settings, error) {
	return b.app.UpdateRuntimeSettings(context.Background(), input, expectedRevision)
}
func (b *SettingsBinding) UpdateExecutionSettings(input domainsettings.ExecutionSettings, expectedRevision int64) (domainsettings.Settings, error) {
	return b.app.UpdateExecutionSettings(context.Background(), input, expectedRevision)
}
func (b *SettingsBinding) UpdateSecuritySettings(input domainsettings.SecuritySettings, expectedRevision int64) (domainsettings.Settings, error) {
	return b.app.UpdateSecuritySettings(context.Background(), input, expectedRevision)
}
func (b *SettingsBinding) UpdateStorageSettings(input domainsettings.StorageSettings, expectedRevision int64) (domainsettings.Settings, error) {
	return b.app.UpdateStorageSettings(context.Background(), input, expectedRevision)
}
func (b *SettingsBinding) UpdateExperimentalSettings(input domainsettings.ExperimentalSettings, expectedRevision int64) (domainsettings.Settings, error) {
	return b.app.UpdateExperimentalSettings(context.Background(), input, expectedRevision)
}
func (b *SettingsBinding) ResetSettingsSection(section string, expectedRevision int64) (domainsettings.Settings, error) {
	return b.app.ResetSettingsSection(context.Background(), section, expectedRevision)
}
func (b *SettingsBinding) CheckRuntimeDraft(input localapp.RuntimeDraftInput) (localapp.RuntimeInfo, error) {
	return b.app.CheckRuntimeDraft(input)
}
func (b *SettingsBinding) InspectCodexConfiguration(input domainsettings.RuntimeSettings) (agentrun.CodexConfiguration, error) {
	return b.app.InspectCodexConfiguration(context.Background(), input)
}
func (b *SettingsBinding) InspectClaudeConfiguration(input domainsettings.RuntimeSettings) (agentrun.ClaudeConfiguration, error) {
	return b.app.InspectClaudeConfiguration(context.Background(), input)
}
func (b *SettingsBinding) GetStorageUsage() (localapp.StorageUsage, error) {
	return b.app.GetStorageUsage()
}
func (b *SettingsBinding) RevealDataRoot() error { return b.app.RevealDataRoot() }
func (b *SettingsBinding) PreviewCleanup(input localapp.CleanupPreviewInput) (localapp.CleanupPreview, error) {
	return b.app.PreviewCleanup(context.Background(), input)
}
func (b *SettingsBinding) ExecuteCleanup(previewToken string) (localapp.CleanupResult, error) {
	return b.app.ExecuteCleanup(context.Background(), previewToken)
}
func (b *SettingsBinding) ExportDiagnostics(input localapp.DiagnosticsExportInput) (localapp.DiagnosticsExport, error) {
	return b.app.ExportDiagnostics(context.Background(), input)
}
