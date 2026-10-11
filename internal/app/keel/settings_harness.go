package keeldesktop

import (
	"fmt"
	"slices"
	"strings"

	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/theme"
	domainsettings "github.com/openmodu/onecatch/internal/domain/settings"
	desktopservice "github.com/openmodu/onecatch/internal/service/desktop"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

type settingsHarness struct {
	configuration agentrun.HarnessConfiguration
	codex         agentrun.CodexConfiguration
	info          desktopservice.RuntimeInfo
	err           string
	models        *kit.SelectView
}

func (p *settingsPage) runtimeDraft(id string) domainsettings.RuntimeSettings {
	r := p.draft.Runtimes[id]
	if id == p.currentRuntime {
		r.Enabled = p.enabled.Value()
		r.Binary = strings.TrimSpace(p.binary)
		r.DefaultModel = strings.TrimSpace(p.model)
	}
	r.Binary = p.value("runtime-binary/"+id, r.Binary)
	r.DefaultModel = p.value("runtime-model/"+id, r.DefaultModel)
	r.Enabled = p.on("runtime-enabled/"+id, r.Enabled)
	r.ReasoningEffort = p.selected("runtime-effort/"+id, r.ReasoningEffort)
	r.Provider = p.selected("runtime-provider/"+id, r.Provider)
	r.ServiceTier = p.selected("runtime-tier/"+id, r.ServiceTier)
	r.Integration = p.selected("runtime-integration/"+id, r.Integration)
	r.ConfigSource = p.selected("runtime-source/"+id, r.ConfigSource)
	r.ConfigPath = p.value("runtime-config/"+id, r.ConfigPath)
	r.RemoteFSEnabled = p.on("runtime-remotefs/"+id, r.RemoteFSEnabled)
	r.MaxContextWindow = p.on("runtime-context/"+id, r.MaxContextWindow)
	if v := p.fields["runtime-env/"+id]; v != nil {
		r.EnvironmentAllowlist = strings.FieldsFunc(*v, func(c rune) bool { return c == ',' || c == '\n' || c == ' ' || c == '\t' })
	}
	return r
}

func (p *settingsPage) checkHarnessID(id string) {
	input, ctx := p.runtimeDraft(id), p.workerContext()
	p.settingsAction("harness", func() (func(), error) {
		result := &settingsHarness{}
		service := p.app.backend.service
		info, err := service.CheckRuntimeDraft(desktopservice.RuntimeDraftInput{Runtime: id, Settings: input})
		result.info = info
		if err == nil {
			switch id {
			case "codex":
				result.codex, err = service.InspectCodexConfiguration(ctx, input, "")
				result.configuration.Model = result.codex.Model
				for _, m := range result.codex.Models {
					result.configuration.Models = append(result.configuration.Models, agentrun.HarnessModel{Model: m.Model, DisplayName: m.DisplayName, Efforts: m.ReasoningEfforts})
				}
			case "claude":
				var source agentrun.ClaudeConfiguration
				source, err = service.InspectClaudeConfiguration(ctx, input, "")
				result.configuration.Model, result.configuration.Efforts = source.Model, source.Efforts
				for _, m := range source.Models {
					result.configuration.Models = append(result.configuration.Models, agentrun.HarnessModel{Model: m.Model, DisplayName: m.DisplayName})
				}
			default:
				result.configuration, err = service.InspectHarnessConfiguration(ctx, id, input)
			}
		}
		if err != nil {
			result.err = err.Error()
		}
		return func() {
			if p.harnesses == nil {
				p.harnesses = map[string]*settingsHarness{}
			}
			p.harnesses[id] = result
			p.viewModel = ""
		}, nil
	})
}
func (p *settingsPage) harnessModel(cx *el.Context, id string) el.Element {
	root := el.Div().Gap(6).Child(p.text("runtime-model/"+id, "留空使用 Agent 的默认模型", p.draft.Runtimes[id].DefaultModel).(*el.InputEl).Name(uiText("默认模型")))
	result := p.harnesses[id]
	if result == nil || len(result.configuration.Models) == 0 {
		return root
	}
	if result.models == nil {
		result.models = kit.Select("").Appearance(false).Size(32).OnChange(func(value string) {
			if value == "__default" {
				value = ""
			}
			*p.fields["runtime-model/"+id] = value
			efforts := p.harnessEfforts(id, nil)
			if choice := p.selects["runtime-effort/"+id]; choice != nil && !slices.Contains(efforts, choice.Value()) {
				choice.SetValue("__default")
			}
			p.draft.Runtimes[id] = p.runtimeDraft(id)
			delete(p.selects, "runtime-effort/"+id)
			p.viewModel = ""
		})
	}
	options := []kit.SelectOption{{Value: "__default", Label: uiText("使用 Agent 默认配置")}}
	for _, m := range result.configuration.Models {
		label := m.DisplayName
		if label == "" {
			label = m.Model
		}
		options = append(options, kit.SelectOption{Value: m.Model, Label: label})
	}
	result.models.SetEntries(options...)
	value := p.value("runtime-model/"+id, "")
	if value == "" {
		value = "__default"
	}
	result.models.SetValue(value)
	return root.Child(result.models.Render(cx))
}
func (p *settingsPage) harnessEfforts(id string, fallback []string) []string {
	values := fallback
	if result := p.harnesses[id]; result != nil {
		if len(result.configuration.Efforts) > 0 {
			values = result.configuration.Efforts
		}
		model := p.value("runtime-model/"+id, p.draft.Runtimes[id].DefaultModel)
		if model == "" {
			model = result.configuration.Model
		}
		for _, m := range result.configuration.Models {
			if m.Model == model && len(m.Efforts) > 0 {
				values = m.Efforts
				break
			}
		}
	}
	return append([]string{""}, values...)
}
func (p *settingsPage) harnessStatus(cx *el.Context, id string) el.Element {
	text := ""
	if result := p.harnesses[id]; result != nil {
		if result.info.Available {
			text = fmt.Sprintf("已连接 · %s · %d 个模型", result.info.Version, len(result.configuration.Models))
		}
		if result.err != "" {
			text += "\n" + result.err
		}
	}
	b := button("刷新模型", func() { p.checkHarnessID(id) }, kit.ButtonGhost).Loading(p.actions.busy == "harness")
	b.SetDisabled(p.actions.busy != "" || p.saving)
	root := el.Div().Gap(8).Items(el.Stretch)
	if text != "" {
		root.Child(el.Text(text).TextSize(12).TextColor(theme.Muted))
	}
	return root.Child(el.Div().Row().Justify(el.End).Child(b.Render(cx)))
}
