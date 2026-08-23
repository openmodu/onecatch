package wailstransport

import (
	"context"

	domainsettings "github.com/openmodu/onecatch/internal/domain/settings"
	desktopservice "github.com/openmodu/onecatch/internal/service/desktop"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

type SettingsBinding struct{ service *desktopservice.Service }

func NewSettingsBinding(service *desktopservice.Service) *SettingsBinding {
	return &SettingsBinding{service: service}
}
func (b *SettingsBinding) GetSettings() (domainsettings.Settings, error) {
	return b.service.GetSettings(context.Background())
}
func (b *SettingsBinding) UpdateRuntimeSettings(input map[string]domainsettings.RuntimeSettings, expectedRevision int64) (domainsettings.Settings, error) {
	return b.service.UpdateRuntimeSettings(context.Background(), input, expectedRevision)
}
func (b *SettingsBinding) UpdateTerminalSettings(input domainsettings.TerminalSettings, expectedRevision int64) (domainsettings.Settings, error) {
	return b.service.UpdateTerminalSettings(context.Background(), input, expectedRevision)
}
func (b *SettingsBinding) UpdateExecutionSettings(input domainsettings.ExecutionSettings, expectedRevision int64) (domainsettings.Settings, error) {
	return b.service.UpdateExecutionSettings(context.Background(), input, expectedRevision)
}
func (b *SettingsBinding) UpdateSecuritySettings(input domainsettings.SecuritySettings, expectedRevision int64) (domainsettings.Settings, error) {
	return b.service.UpdateSecuritySettings(context.Background(), input, expectedRevision)
}
func (b *SettingsBinding) UpdateStorageSettings(input domainsettings.StorageSettings, expectedRevision int64) (domainsettings.Settings, error) {
	return b.service.UpdateStorageSettings(context.Background(), input, expectedRevision)
}
func (b *SettingsBinding) UpdateExperimentalSettings(input domainsettings.ExperimentalSettings, expectedRevision int64) (domainsettings.Settings, error) {
	return b.service.UpdateExperimentalSettings(context.Background(), input, expectedRevision)
}
func (b *SettingsBinding) ResetSettingsSection(section string, expectedRevision int64) (domainsettings.Settings, error) {
	return b.service.ResetSettingsSection(context.Background(), section, expectedRevision)
}
func (b *SettingsBinding) CheckRuntimeDraft(input desktopservice.RuntimeDraftInput) (desktopservice.RuntimeInfo, error) {
	return b.service.CheckRuntimeDraft(input)
}
func (b *SettingsBinding) InspectCodexConfiguration(input domainsettings.RuntimeSettings) (agentrun.CodexConfiguration, error) {
	return b.service.InspectCodexConfiguration(context.Background(), input)
}
func (b *SettingsBinding) InspectClaudeConfiguration(input domainsettings.RuntimeSettings) (agentrun.ClaudeConfiguration, error) {
	return b.service.InspectClaudeConfiguration(context.Background(), input)
}

// InspectHarnessConfiguration serves every harness that reports through the
// shared configuration shape, so a new adapter needs no binding of its own.
func (b *SettingsBinding) InspectHarnessConfiguration(runtime string, input domainsettings.RuntimeSettings) (agentrun.HarnessConfiguration, error) {
	return b.service.InspectHarnessConfiguration(context.Background(), runtime, input)
}
func (b *SettingsBinding) GetStorageUsage() (desktopservice.StorageUsage, error) {
	return b.service.GetStorageUsage()
}
func (b *SettingsBinding) RevealDataRoot() error { return b.service.RevealDataRoot() }
func (b *SettingsBinding) PreviewCleanup(input desktopservice.CleanupPreviewInput) (desktopservice.CleanupPreview, error) {
	return b.service.PreviewCleanup(context.Background(), input)
}
func (b *SettingsBinding) ExecuteCleanup(previewToken string) (desktopservice.CleanupResult, error) {
	return b.service.ExecuteCleanup(context.Background(), previewToken)
}
func (b *SettingsBinding) ExportDiagnostics(input desktopservice.DiagnosticsExportInput) (desktopservice.DiagnosticsExport, error) {
	return b.service.ExportDiagnostics(context.Background(), input)
}
