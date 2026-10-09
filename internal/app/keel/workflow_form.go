package keeldesktop

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/theme"
	domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"
)

func (a *app) resetWorkflowForm() {
	a.ui.DAG = dagState{}
	a.ui.WorkflowSelects = map[string]*kit.SelectView{}
	a.ui.WorkflowStep = 0
	a.ui.WorkflowStepID, a.ui.WorkflowSignal = "", ""
	p := a.ui.WorkflowDraft.Policy
	a.ui.WorkflowPolicy = map[string]string{"transitions": strconv.Itoa(p.MaxTransitions), "failures": strconv.Itoa(p.MaxConsecutiveFailures), "timeout": strconv.Itoa(p.StepTimeoutSeconds)}
	if len(a.ui.WorkflowDraft.Steps) > 0 {
		a.ui.WorkflowStepID = a.ui.WorkflowDraft.Steps[0].ID
	}
}

func (a *app) syncWorkflowDocument() {
	data, err := json.MarshalIndent(a.ui.WorkflowDraft, "", "  ")
	if err != nil {
		a.fail(err)
		return
	}
	a.workflowEditor.SetValue(string(data))
}

func (a *app) toggleWorkflowRaw() {
	if a.ui.WorkflowRaw {
		var d domainworkflows.Definition
		if err := json.Unmarshal([]byte(a.workflowEditor.Value()), &d); err != nil {
			a.fail(fmt.Errorf("工作流 JSON 无效：%w", err))
			return
		}
		a.ui.WorkflowDraft = domainworkflows.Normalize(d)
		a.resetWorkflowForm()
	} else {
		a.syncWorkflowDocument()
	}
	a.ui.WorkflowRaw = !a.ui.WorkflowRaw
	// The JSON editor is the common snapshot used for history and persistence.
}

func workflowField(label string, content el.Element) el.Element {
	return el.Div().Gap(6).WFull().Child(el.Text(label).TextSize(12).TextColor(theme.Muted), content)
}

func (a *app) workflowText(id, hint string, value *string, multiline bool) el.Element {
	if multiline {
		rows := 3
		if strings.HasPrefix(id, "simple-instruction-") {
			rows = 5
		}
		return el.TextArea().ID(id).Name(hint).Bind(value).AutoGrow(rows, 8).WFull().Border(0, theme.Border).Bg(theme.Subtle).Rounded(8).P(10).TextSize(13).OnChange(func(string) { a.syncWorkflowDocument() })
	}
	return field(id, hint, value).WFull().OnChange(func(string) { a.syncWorkflowDocument() })
}

func (a *app) workflowSelect(cx *el.Context, id, label, value string, options []kit.SelectOption, change func(string)) el.Element {
	if a.ui.WorkflowSelects == nil {
		a.ui.WorkflowSelects = map[string]*kit.SelectView{}
	}
	view := a.ui.WorkflowSelects[id]
	if view == nil {
		view = kit.Select("").Hint(label).Appearance(false).MenuWidth(240).Size(32)
		if strings.HasPrefix(id, "dag-") {
			view.Size(36)
		}
		a.ui.WorkflowSelects[id] = view
	}
	options = uniqueSelectOptions(options)
	if !slices.Equal(view.Entries(), options) {
		view.SetEntries(options...)
	}
	view.SetValue(value)
	view.OnChange(func(value string) { change(value); a.syncWorkflowDocument() })
	return el.Div().WFull().MinH(el.Dp(32)).Px(8).Rounded(8).Bg(theme.Subtle).Child(view.Render(cx))
}

// Invalid drafts must remain editable instead of panicking in kit.Select.
func uniqueSelectOptions(options []kit.SelectOption) []kit.SelectOption {
	seen := map[string]bool{}
	result := make([]kit.SelectOption, 0, len(options))
	for _, option := range options {
		if option.Value != "" && !seen[option.Value] {
			seen[option.Value] = true
			result = append(result, option)
		}
	}
	return result
}

func (a *app) workflowForm(cx *el.Context) el.Element {
	d := &a.ui.WorkflowDraft
	body := el.Div().ID("workflow-form").Disabled(a.busy).Grow().H(el.Dp(0)).ScrollY().Px(24).Pb(24).Gap(22)
	identity := el.Div().Gap(14).Child(workflowField("名称", a.workflowText("workflow-name", "工作流名称", &d.Name, false)),
		workflowField("描述", a.workflowText("workflow-description", "工作流描述", &d.Description, true)))
	identity.Child(workflowField("工作流 ID", field("workflow-id", "工作流 ID", &d.ID).WFull().ReadOnly(a.editingWorkflow != "").OnChange(func(string) { a.syncWorkflowDocument() })))
	entries := make([]kit.SelectOption, 0, len(d.Steps))
	for _, step := range d.Steps {
		entries = append(entries, kit.SelectOption{Value: step.ID, Label: step.Name + " · " + step.ID})
	}
	if len(entries) > 0 {
		identity.Child(workflowField("入口步骤", a.workflowSelect(cx, "entry", "入口步骤", d.EntryStepID, entries, func(s string) { d.EntryStepID = s })))
	}
	body.Child(identity)
	heading := el.Div().Row().Items(el.Center).Child(el.Text(map[bool]string{true: "节点", false: "步骤"}[d.Mode == domainworkflows.ModeDAG]).TextSize(15).Medium().Grow(), iconAction(cx, "workflow-add-step", "添加步骤", "plus", a.addWorkflowStep, false))
	body.Child(heading)
	choices := el.Div().Row().Wrap().Gap(6)
	for i, step := range d.Steps {
		index := i
		choices.Child(button(fmt.Sprintf("%d · %s", i+1, step.Name), func() { a.ui.WorkflowStep = index; a.ui.WorkflowStepID = d.Steps[index].ID; a.ui.WorkflowSignal = "" }, kit.ButtonGhost).Selected(a.ui.WorkflowStep == i).Render(cx))
	}
	body.Child(choices)
	if len(d.Steps) == 0 {
		body.Child(el.Text("添加一个步骤开始编排。").TextSize(13).TextColor(theme.Muted))
		return body
	}
	a.ui.WorkflowStep = min(max(0, a.ui.WorkflowStep), len(d.Steps)-1)
	index := a.ui.WorkflowStep
	step := &d.Steps[index]
	section := el.Div().Gap(14).P(18).Rounded(12).Bg(theme.Surface)
	remove := iconButton("删除步骤", func() { a.removeWorkflowStep(index) }, kit.ButtonGhost, kit.IconTrash)
	remove.SetDisabled(len(d.Steps) <= 1)
	section.Child(el.Div().Row().Items(el.Center).Child(el.Text(step.Name).Medium().TextSize(15).Grow(), remove.Render(cx)),
		workflowField("名称", a.workflowText(fmt.Sprintf("step-%d-name", index), "步骤名称", &step.Name, false)))
	section.Child(workflowField("步骤 ID", el.Div().Row().Gap(8).Child(field("step-id", "步骤 ID", &a.ui.WorkflowStepID).Grow().W(el.Dp(0)), button("修改", func() {
		if err := renameWorkflowStep(d, index, strings.TrimSpace(a.ui.WorkflowStepID)); err != nil {
			a.fail(err)
			return
		}
		a.resetWorkflowSelects()
		a.syncWorkflowDocument()
	}, kit.ButtonGhost).Render(cx))))
	var runtimes []kit.SelectOption
	for _, r := range a.runtimes {
		runtimes = append(runtimes, kit.SelectOption{Value: r.ID, Label: r.Name, Disabled: !r.Enabled && r.ID != step.Runtime})
	}
	if !slices.ContainsFunc(runtimes, func(r kit.SelectOption) bool { return r.Value == step.Runtime }) {
		runtimes = append(runtimes, kit.SelectOption{Value: step.Runtime, Label: step.Runtime})
	}
	section.Child(workflowField("Agent", a.workflowSelect(cx, fmt.Sprintf("runtime-%d", index), "Agent", step.Runtime, runtimes, func(s string) { step.Runtime = s })),
		workflowField("模型", a.workflowText(fmt.Sprintf("model-%d", index), "使用 Agent 默认模型", &step.Model, false)),
		workflowField("Worker", a.workflowText(fmt.Sprintf("worker-%d", index), "local 或已配置的 Worker ID", &step.WorkerID, false)),
		workflowField("工作区访问", a.workflowSelect(cx, fmt.Sprintf("sandbox-%d", index), "工作区访问", step.Sandbox, []kit.SelectOption{{Value: "read-only", Label: "只读访问"}, {Value: "workspace-write", Label: "工作区访问"}, {Value: "full", Label: "完全访问", Disabled: step.WorkerID != "local" || !a.settings.Security.AllowFullSandbox && step.Sandbox != "full"}}, func(s string) { step.Sandbox = s })),
		workflowField("角色提示词", a.workflowText(fmt.Sprintf("role-%d", index), "角色提示词", &step.RolePrompt, true)),
		workflowField("执行指令", a.workflowText(fmt.Sprintf("instruction-%d", index), "执行指令", &step.Instruction, true)))
	if d.Mode == domainworkflows.ModeDAG {
		dependencies := el.Div().Gap(6)
		for _, dep := range d.Steps {
			if dep.ID == step.ID {
				continue
			}
			id := dep.ID
			selected := slices.Contains(step.DependsOn, id)
			dependencies.Child(button(dep.Name, func() {
				if slices.Contains(step.DependsOn, id) {
					step.DependsOn = slices.DeleteFunc(step.DependsOn, func(s string) bool { return s == id })
				} else {
					step.DependsOn = append(step.DependsOn, id)
				}
				a.syncWorkflowDocument()
			}, kit.ButtonGhost).Selected(selected).Render(cx))
		}
		section.Child(workflowField("等待所有依赖完成", dependencies))
	}
	targets := []kit.SelectOption{{Value: "$done", Label: "完成 · $done"}, {Value: "$pause", Label: "暂停 · $pause"}, {Value: "$fail", Label: "失败 · $fail"}}
	if d.Mode != domainworkflows.ModeDAG {
		targets = append(targets, entries...)
	}
	signals := make([]string, 0, len(step.Transitions))
	for signal := range step.Transitions {
		signals = append(signals, signal)
	}
	sort.Strings(signals)
	transitions := el.Div().Gap(8)
	for _, signal := range signals {
		key := signal
		transitions.Child(el.Div().Row().Items(el.Center).Gap(8).Child(el.Text(key).Mono().TextSize(12).W(el.Dp(130)).MaxLines(1), el.Div().Grow().W(el.Dp(0)).Child(a.workflowSelect(cx, fmt.Sprintf("target-%d-%s", index, key), key+" 的目标", step.Transitions[key], targets, func(s string) { step.Transitions[key] = s })), iconAction(cx, "remove-signal/"+key, "移除 "+key, "trash-2", func() { delete(step.Transitions, key); a.syncWorkflowDocument() }, false)))
	}
	transitions.Child(el.Div().Row().Gap(8).Child(field("new-signal", "添加信号，例如 completed", &a.ui.WorkflowSignal).Grow().W(el.Dp(0)), button("添加", func() {
		signal := strings.TrimSpace(a.ui.WorkflowSignal)
		if signal == "" {
			return
		}
		if _, exists := step.Transitions[signal]; exists {
			a.fail(fmt.Errorf("信号 %s 已存在", signal))
			return
		}
		if step.Transitions == nil {
			step.Transitions = map[string]string{}
		}
		step.Transitions[signal] = "$pause"
		a.ui.WorkflowSignal = ""
		a.syncWorkflowDocument()
	}, kit.ButtonGhost).Render(cx)))
	section.Child(workflowField("信号与转换目标", transitions))
	body.Child(section)
	policy := el.Div().Gap(14).P(18).Rounded(12).Bg(theme.Surface).Child(el.Text("执行策略").Medium().TextSize(15))
	for _, item := range []struct {
		ID, Label string
		Value     *int
	}{{"transitions", "最大转换次数", &d.Policy.MaxTransitions}, {"failures", "最大连续失败次数", &d.Policy.MaxConsecutiveFailures}, {"timeout", "步骤超时（秒）", &d.Policy.StepTimeoutSeconds}} {
		it := item
		value := a.ui.WorkflowPolicy[it.ID]
		policy.Child(workflowField(it.Label, field("policy/"+it.ID, it.Label, &value).Filter("0123456789").MaxLen(7).OnChange(func(s string) {
			a.ui.WorkflowPolicy[it.ID] = s
			n, err := strconv.Atoi(s)
			if err != nil {
				n = -1
			}
			*it.Value = n
			a.syncWorkflowDocument()
		})))
	}
	body.Child(policy)
	if err := domainworkflows.Validate(*d); err != nil {
		body.Child(el.Text(err.Error()).TextSize(12).TextColor(theme.DangerText))
	}
	return body
}

func (a *app) resetWorkflowSelects() { a.ui.WorkflowSelects = map[string]*kit.SelectView{} }

// Renaming a node updates every reference, including canvas positions. Reject
// collisions instead of silently redirecting another node's incoming edges.
func renameWorkflowStep(d *domainworkflows.Definition, index int, id string) error {
	if index < 0 || index >= len(d.Steps) || id == "" || strings.HasPrefix(id, "$") {
		return fmt.Errorf("步骤 ID 无效")
	}
	for i, s := range d.Steps {
		if i != index && s.ID == id {
			return fmt.Errorf("步骤 ID %s 已存在", id)
		}
	}
	old := d.Steps[index].ID
	d.Steps[index].ID = id
	if d.EntryStepID == old {
		d.EntryStepID = id
	}
	for i := range d.Steps {
		for j, dep := range d.Steps[i].DependsOn {
			if dep == old {
				d.Steps[i].DependsOn[j] = id
			}
		}
		for signal, target := range d.Steps[i].Transitions {
			if target == old {
				d.Steps[i].Transitions[signal] = id
			}
		}
	}
	if point, ok := d.Layout.Nodes[old]; ok {
		delete(d.Layout.Nodes, old)
		d.Layout.Nodes[id] = point
	}
	return nil
}

func (a *app) addWorkflowStep() {
	d := &a.ui.WorkflowDraft
	index := len(d.Steps) + 1
	id := fmt.Sprintf("step_%d", index)
	for slices.ContainsFunc(d.Steps, func(s domainworkflows.Step) bool { return s.ID == id }) {
		index++
		id = fmt.Sprintf("step_%d", index)
	}
	runtime := "codex"
	for _, r := range a.runtimes {
		if r.Enabled && r.Available {
			runtime = r.ID
			break
		}
	}
	d.Steps = append(d.Steps, domainworkflows.Step{ID: id, Name: fmt.Sprintf("步骤 %d", index), Runtime: runtime, WorkerID: "local", Sandbox: "workspace-write", RolePrompt: "你是负责此步骤的工程师。", Instruction: "完成此步骤并验证结果。", Transitions: map[string]string{"completed": "$done", "need_human": "$pause"}})
	if d.EntryStepID == "" {
		d.EntryStepID = id
	}
	a.ui.WorkflowStep = len(d.Steps) - 1
	a.ui.WorkflowStepID = id
	a.ui.WorkflowSignal = ""
	a.resetWorkflowSelects()
	a.syncWorkflowDocument()
}

func removeWorkflowNode(d *domainworkflows.Definition, index int) bool {
	if index < 0 || index >= len(d.Steps) || len(d.Steps) <= 1 {
		return false
	}
	id := d.Steps[index].ID
	d.Steps = slices.Delete(d.Steps, index, index+1)
	if d.EntryStepID == id {
		d.EntryStepID = d.Steps[0].ID
	}
	for i := range d.Steps {
		d.Steps[i].DependsOn = slices.DeleteFunc(d.Steps[i].DependsOn, func(dep string) bool { return dep == id })
		for signal, target := range d.Steps[i].Transitions {
			if target == id {
				d.Steps[i].Transitions[signal] = "$pause"
			}
		}
	}
	delete(d.Layout.Nodes, id)
	return true
}

func (a *app) removeWorkflowStep(index int) {
	if !removeWorkflowNode(&a.ui.WorkflowDraft, index) {
		return
	}
	a.resetWorkflowForm()
	a.syncWorkflowDocument()
}
