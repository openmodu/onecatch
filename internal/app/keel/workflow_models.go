package keeldesktop

import (
	"fmt"
	"slices"

	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/theme"
	domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

type workflowModelKey struct {
	runtime, worker, path string
	revision              int64
}

type workflowModelCatalog struct {
	configuration agentrun.HarnessConfiguration
	loading       bool
	err           string
}

func (a *app) workflowModelKey(step domainworkflows.Step) workflowModelKey {
	worker := firstProfileValue(step.WorkerID, "local")
	workspace := a.workspaceForID(a.workspaceID)
	path := ""
	if worker == "local" && workspace.RemoteFS == nil {
		path = workspace.Path
	}
	return workflowModelKey{runtime: step.Runtime, worker: worker, path: path, revision: a.settings.Revision}
}

// A catalog belongs to its runtime, settings and workspace, never to the
// currently selected node. A late reply cannot overwrite another node's model.
func (a *app) loadWorkflowModels(key workflowModelKey) {
	if key.worker != "local" || key.runtime == "" {
		return
	}
	if a.ui.DAG.Models == nil {
		a.ui.DAG.Models = map[workflowModelKey]*workflowModelCatalog{}
	}
	if a.ui.DAG.Models[key] != nil {
		return
	}
	catalog := &workflowModelCatalog{loading: true}
	a.ui.DAG.Models[key] = catalog
	service, ctx, settings := a.backend.service, a.ctx, a.settings.Runtimes[key.runtime]
	post := a.postUpdate
	if post == nil {
		post = core.Update
	}
	go func() {
		configuration, _, err := inspectRuntimeConfiguration(ctx, service, key.runtime, settings, key.path)
		post(func() {
			if a.closed || a.ui.DAG.Models[key] != catalog {
				return
			}
			catalog.loading, catalog.configuration = false, configuration
			if err != nil {
				catalog.err = err.Error()
			}
		})
	}()
}

func workflowModelOptions(step domainworkflows.Step, catalog *workflowModelCatalog) []kit.SelectOption {
	options := []kit.SelectOption{{Value: "__default", Label: "使用运行环境默认模型"}}
	if catalog != nil && !catalog.loading && catalog.err == "" {
		for _, model := range catalog.configuration.Models {
			if model.Model == "" {
				continue
			}
			label := firstProfileValue(model.DisplayName, model.Model)
			if label != model.Model {
				label += " · " + model.Model
			}
			options = append(options, kit.SelectOption{Value: model.Model, Label: label})
		}
	}
	if step.Model != "" && !slices.ContainsFunc(options, func(o kit.SelectOption) bool { return o.Value == step.Model }) {
		// Keep the old value visible for correction, without offering it as a
		// valid choice or silently rewriting an existing workflow.
		label := step.Model + " · 当前配置"
		if catalog != nil && !catalog.loading && catalog.err == "" {
			label = step.Model + " · 不可用"
		}
		options = append(options, kit.SelectOption{Value: step.Model, Label: label, Disabled: true})
	}
	return options
}

func (a *app) workflowModelPicker(cx *el.Context, index int) el.Element {
	step := a.ui.WorkflowDraft.Steps[index]
	key := a.workflowModelKey(step)
	if a.window != nil {
		a.loadWorkflowModels(key)
	}
	catalog := a.ui.DAG.Models[key]
	value := firstProfileValue(step.Model, "__default")
	options := workflowModelOptions(step, catalog)
	defaultModel := ""
	if key.worker == "local" {
		defaultModel = a.settings.Runtimes[key.runtime].DefaultModel
		labelModel := defaultModel
		if labelModel == "" && catalog != nil {
			labelModel = catalog.configuration.Model
		}
		if labelModel != "" {
			options[0].Label += " · " + labelModel
		}
		options[0].Disabled = defaultModel != "" && catalog != nil && !catalog.loading && catalog.err == "" && !workflowCatalogContains(catalog, defaultModel)
	}
	box := el.Div().WFull().Gap(6).Child(a.workflowSelect(cx, "dag-model/"+step.ID, "模型", value, options, func(value string) {
		if value == "__default" {
			value = ""
		}
		a.setDAGModel(index, value)
	}))
	message, invalid := "", false
	switch {
	case key.worker != "local":
		message = "远端 Worker 暂不提供模型列表，可使用其默认模型。"
	case catalog == nil:
		message = "点击刷新，读取运行环境提供的模型。"
	case catalog.loading:
		message = "正在读取可用模型…"
	case catalog.err != "":
		message, invalid = "模型列表读取失败："+catalog.err, true
	case step.Model != "" && !workflowCatalogContains(catalog, step.Model):
		message, invalid = "当前模型不在可用列表中，请重新选择。", true
	case step.Model == "" && options[0].Disabled:
		message, invalid = "默认模型不在可用列表中，请选择一个可用模型。", true
	case len(catalog.configuration.Models) == 0:
		message = "运行环境未返回可选模型，可使用默认模型。"
	}
	if message != "" {
		fg := theme.Muted
		if invalid {
			fg = theme.DangerText
		}
		box.Child(el.Text(message).TextSize(11).TextColor(fg))
	}
	if key.worker == "local" {
		refresh := button("刷新模型", func() {
			delete(a.ui.DAG.Models, key)
			a.loadWorkflowModels(key)
		}, kit.ButtonGhost).ID("dag-model-refresh/" + step.ID).Size(28).Compact(true)
		refresh.SetDisabled(catalog != nil && catalog.loading)
		box.Child(el.Div().Row().Justify(el.End).Child(refresh.Render(cx)))
	}
	return box
}

func workflowCatalogContains(catalog *workflowModelCatalog, model string) bool {
	return slices.ContainsFunc(catalog.configuration.Models, func(m agentrun.HarnessModel) bool { return m.Model == model })
}

func (a *app) validateWorkflowModels(definition domainworkflows.Definition) error {
	a.prepareWorkflowModels(definition)
	for _, step := range definition.Steps {
		if step.Kind == domainworkflows.StepCommand {
			continue
		}
		key := a.workflowModelKey(step)
		// Do not validate remote credentials against the local account's list.
		if key.worker != "local" {
			continue
		}
		model := firstProfileValue(step.Model, a.settings.Runtimes[key.runtime].DefaultModel)
		effort := firstProfileValue(step.ReasoningEffort, a.settings.Runtimes[key.runtime].ReasoningEffort)
		if model == "" && effort == "" {
			continue
		}
		catalog := a.ui.DAG.Models[key]
		if catalog == nil || catalog.loading {
			return fmt.Errorf("正在读取步骤「%s」的可用模型，请稍后保存", step.Name)
		}
		if catalog.err != "" {
			return fmt.Errorf("无法校验步骤「%s」的模型，请刷新模型列表：%s", step.Name, catalog.err)
		}
		if model != "" && !workflowCatalogContains(catalog, model) {
			return fmt.Errorf("步骤「%s」的模型 %q 不在可用列表中，请重新选择", step.Name, model)
		}
		values := a.workflowModelEfforts(step, catalog)
		if effort != "" && (step.ReasoningEffort != "" || len(values) > 0) && !slices.Contains(values, effort) {
			return fmt.Errorf("步骤「%s」的推理强度 %q 不受所选模型支持，请重新选择", step.Name, effort)
		}
	}
	return nil
}

func (a *app) prepareWorkflowModels(definition domainworkflows.Definition) {
	for _, step := range definition.Steps {
		if step.Kind != domainworkflows.StepCommand {
			a.loadWorkflowModels(a.workflowModelKey(step))
		}
	}
}
