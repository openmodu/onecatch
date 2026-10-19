package keeldesktop

import (
	"slices"

	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/theme"
	"github.com/openmodu/onecatch/internal/domain/harnesses"
	domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"
)

func (a *app) workflowModelEfforts(step domainworkflows.Step, catalog *workflowModelCatalog) []string {
	if catalog == nil || catalog.loading || catalog.err != "" {
		return nil
	}
	model := firstProfileValue(step.Model, a.settings.Runtimes[step.Runtime].DefaultModel, catalog.configuration.Model)
	values := catalog.configuration.Efforts
	for _, item := range catalog.configuration.Models {
		if item.Model == model {
			if len(item.Efforts) > 0 {
				values = item.Efforts
			}
			break
		}
	}
	harness, _ := harnesses.Find(step.Runtime)
	var efforts []string
	for _, value := range uniqueProfileValues(values) {
		if value != "" && harness.SupportsEffort(value) {
			efforts = append(efforts, value)
		}
	}
	return efforts
}

func (a *app) setDAGModel(index int, model string) {
	step := &a.ui.WorkflowDraft.Steps[index]
	if step.Model == model {
		return
	}
	step.Model = model
	catalog := a.ui.DAG.Models[a.workflowModelKey(*step)]
	if !slices.Contains(a.workflowModelEfforts(*step, catalog), step.ReasoningEffort) {
		step.ReasoningEffort = ""
	}
}

func (a *app) workflowEffortPicker(cx *el.Context, index int) el.Element {
	step := a.ui.WorkflowDraft.Steps[index]
	catalog := a.ui.DAG.Models[a.workflowModelKey(step)]
	values := a.workflowModelEfforts(step, catalog)
	options := []kit.SelectOption{{Value: "__default", Label: "默认强度"}}
	inherited := a.settings.Runtimes[step.Runtime].ReasoningEffort
	if a.workflowModelKey(step).worker == "local" && inherited != "" {
		options[0].Label += " · " + profileEffortLabel(inherited)
		options[0].Disabled = len(values) > 0 && !slices.Contains(values, inherited)
	}
	for _, value := range values {
		options = append(options, kit.SelectOption{Value: value, Label: profileEffortLabel(value) + " · " + value})
	}
	invalid := step.ReasoningEffort != "" && !slices.Contains(values, step.ReasoningEffort)
	if invalid {
		options = append(options, kit.SelectOption{Value: step.ReasoningEffort, Label: step.ReasoningEffort + " · 待重新选择", Disabled: true})
	}
	box := el.Div().WFull().Gap(6).Child(a.workflowSelect(cx, "dag-effort/"+step.ID, "推理强度", firstProfileValue(step.ReasoningEffort, "__default"), options, func(value string) {
		if value == "__default" {
			value = ""
		}
		a.ui.WorkflowDraft.Steps[index].ReasoningEffort = value
	}))
	switch {
	case a.workflowModelKey(step).worker != "local":
		box.Child(el.Text("远端 Worker 暂不提供强度列表，可使用默认设置。").TextSize(11).TextColor(theme.Muted))
	case catalog == nil || catalog.loading || catalog.err != "":
		box.Child(el.Text("读取模型列表后可选择推理强度。").TextSize(11).TextColor(theme.Muted))
	case invalid:
		box.Child(el.Text("当前强度不受所选模型支持，请重新选择。").TextSize(11).TextColor(theme.DangerText))
	case step.ReasoningEffort == "" && options[0].Disabled:
		box.Child(el.Text("默认强度不受所选模型支持，请选择可用档位。").TextSize(11).TextColor(theme.DangerText))
	case len(values) == 0:
		box.Child(el.Text("当前模型未提供可选强度，使用默认设置。").TextSize(11).TextColor(theme.Muted))
	}
	return box
}
