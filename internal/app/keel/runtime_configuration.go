package keeldesktop

import (
	"context"

	domainsettings "github.com/openmodu/onecatch/internal/domain/settings"
	desktopservice "github.com/openmodu/onecatch/internal/service/desktop"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

// Share the runtime's advertised IDs between the composer and workflow editor.
func inspectRuntimeConfiguration(ctx context.Context, service *desktopservice.Service, runtime string, settings domainsettings.RuntimeSettings, path string) (agentrun.HarnessConfiguration, agentrun.CodexConfiguration, error) {
	var configuration agentrun.HarnessConfiguration
	var codex agentrun.CodexConfiguration
	var err error
	switch runtime {
	case "codex":
		var source agentrun.CodexConfiguration
		source, err = service.InspectCodexConfiguration(ctx, settings, path)
		codex = source
		configuration.Model = source.Model
		for _, model := range source.Models {
			if configuration.Model == "" && model.IsDefault {
				configuration.Model = model.Model
			}
			configuration.Models = append(configuration.Models, agentrun.HarnessModel{Model: model.Model, DisplayName: model.DisplayName, Description: model.Description, DefaultEffort: model.DefaultReasoningEffort, Efforts: model.ReasoningEfforts})
		}
	case "claude":
		var source agentrun.ClaudeConfiguration
		source, err = service.InspectClaudeConfiguration(ctx, settings, path)
		configuration.Model, configuration.Efforts = source.Model, source.Efforts
		for _, model := range source.Models {
			configuration.Models = append(configuration.Models, agentrun.HarnessModel{Model: model.Model, DisplayName: model.DisplayName})
		}
	default:
		configuration, err = service.InspectHarnessConfiguration(ctx, runtime, settings)
	}
	return configuration, codex, err
}
