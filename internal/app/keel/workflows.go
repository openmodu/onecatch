package keeldesktop

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"runtime"
	"sort"
	"strings"

	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/theme"
	"github.com/dyike/keel/ui/window"
	domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"
)

// Source: frontend/src/app/templates.js. Embedded data keeps the native binary
// independent of the JavaScript build and preserves the existing starter flows.
//
//go:embed workflow_templates.json
var workflowTemplateData []byte

func starterWorkflow(kind string) domainworkflows.Definition {
	var templates map[string]domainworkflows.Definition
	if err := json.Unmarshal(workflowTemplateData, &templates); err != nil {
		panic(err)
	}
	d, ok := templates[kind]
	if !ok {
		d = templates["loop"]
	}
	return domainworkflows.Normalize(d)
}

func (a *app) newWorkflowMenu(cx *el.Context) el.Element {
	if a.ui.WorkflowNew == nil {
		menu := kit.Menu().Width(210).Placement(el.Bottom, el.End)
		menu.ContentItem("实现与审查 Loop", "", menuIcon("workflow", "实现与审查 Loop"), func() { a.newWorkflowFrom("loop") })
		menu.ContentItem("并行 DAG", "", menuIcon("git-branch", "并行 DAG"), func() { a.newWorkflowFrom("dag") })
		menu.Trigger(el.ViewFunc(func(cx *el.Context) el.Element {
			return iconAction(cx, "new-workflow", "新建工作流", "plus", menu.Toggle, menu.Value())
		}))
		a.ui.WorkflowNew = menu
	}
	return a.ui.WorkflowNew.Render(cx)
}

func workflowPath(d domainworkflows.Definition) string {
	var names []string
	for _, s := range d.Steps {
		names = append(names, s.Name)
	}
	if d.Mode == domainworkflows.ModeDAG {
		var roots, joins []string
		for _, s := range d.Steps {
			if len(s.DependsOn) == 0 {
				roots = append(roots, s.Name)
			} else {
				joins = append(joins, s.Name)
			}
		}
		path := strings.Join(roots, " ∥ ")
		if len(joins) > 0 {
			path += " → " + strings.Join(joins, " → ")
		}
		return path
	}
	if len(names) > 1 {
		return strings.Join(names, " → ") + " ↺"
	}
	return strings.Join(names, " → ") + " → $done"
}
func (a *app) workflowPage(cx *el.Context) el.Element {
	return a.workflowPageLayout(cx, a.ui.WorkflowWindow != nil)
}
func (a *app) workflowPageLayout(cx *el.Context, auxiliary bool) el.Element {
	editable := make([]domainworkflows.Definition, 0, len(a.definitions))
	var selected *domainworkflows.Definition
	for _, d := range a.definitions {
		if d.ID != "single_agent" {
			editable = append(editable, d)
			if d.ID == a.ui.SelectedWorkflow {
				copy := d
				selected = &copy
			}
		}
	}
	if selected == nil && len(editable) > 0 {
		selected = &editable[0]
	}
	// The source's 240px rail includes its right-hand divider.
	left := el.Div().W(el.Dp(239)).NoShrink().Bg(sidebarColor).HFull()
	back := iconButton("", func() { a.goWorkflowHistory(-1) }, kit.ButtonGhost, kit.IconChevronLeft).ID("workflow-back").Name("返回").Size(26).Compact(true)
	forward := iconButton("", func() { a.goWorkflowHistory(1) }, kit.ButtonGhost, kit.IconChevronRight).ID("workflow-forward").Name("前进").Size(26).Compact(true)
	back.SetDisabled(len(a.ui.WorkflowHistory.Entries) == 0 || a.ui.WorkflowHistory.Index == 0)
	forward.SetDisabled(a.ui.WorkflowHistory.Index+1 >= len(a.ui.WorkflowHistory.Entries))
	top := el.Div().Row().Items(el.Center).Px(16).H(el.Dp(52))
	if runtime.GOOS == "darwin" && auxiliary {
		top.Pl(94)
	}
	left.Child(top.Child(dragArea(cx, el.Div().Grow().HFull()), back.Render(cx), forward.Render(cx)),
		el.Div().Row().Items(el.Center).Px(20).Pt(8).Pb(12).Child(el.Div().Grow().Gap(3).Child(el.Text("工作流").Bold().TextSize(14), el.Text(fmt.Sprint(len(editable))).TextSize(11).TextColor(theme.Muted)), a.newWorkflowMenu(cx)))
	list := el.Div().ID("workflow-list").Grow().H(el.Dp(0)).ScrollY().Px(12).Pb(16).Gap(1)
	for _, definition := range editable {
		d := definition
		active := selected != nil && selected.ID == d.ID
		row := el.Div().Row().Items(el.Center).Rounded(8).Px(4).Py(4).Hover(func(s *el.Style) { s.Bg(theme.Highlight) })
		if active {
			row.Bg(theme.Highlight)
		}
		summary := fmt.Sprintf("%d 个步骤 · 最多 %d 次转换", len(d.Steps), d.Policy.MaxTransitions)
		if d.Mode == domainworkflows.ModeDAG {
			summary = fmt.Sprintf("%d 个节点 · 全部汇合", len(d.Steps))
		}
		row.Child(el.Div().ID("workflow/"+d.ID).Name(d.Name).Role("button").Selected(active).Focusable(true).Grow().W(el.Dp(0)).Px(8).Py(8).CursorPointer().OnClick(func() { a.selectWorkflow(d.ID, false) }).FocusStyle(neutralFocus).Gap(3).Child(el.Text(d.Name).MaxLines(1).Medium().TextSize(13), el.Text(summary).MaxLines(1).TextSize(11).TextColor(theme.Muted)))
		row.Child(a.workflowRowMenu(d).Render(cx))
		list.Child(row)
	}
	left.Child(list)
	right := el.Div().Grow().W(el.Dp(0)).HFull()
	if a.ui.WorkflowSource && !a.ui.WorkflowRaw && a.ui.WorkflowDraft.Mode == domainworkflows.ModeDAG {
		right.Child(dragArea(cx, el.Div().H(el.Dp(52)).WFull()), a.dagEditor(cx))
	} else if a.ui.WorkflowSource {
		save := button("保存", a.saveWorkflow, kit.ButtonPrimary).Loading(a.busy)
		save.SetDisabled(a.editingWorkflow == "single_agent")
		_, simple := simpleReviewWorkflow(a.ui.WorkflowDraft)
		guided := simple && !a.ui.WorkflowAdvanced && !a.ui.WorkflowRaw
		title := "编辑工作流"
		if guided {
			title = "配置流程"
		}
		header := el.Div().Row().Items(el.Center).Px(20).H(el.Dp(64)).Child(el.Text(title).Medium().Grow())
		if simple && !a.ui.WorkflowRaw {
			label := "高级编辑"
			if a.ui.WorkflowAdvanced {
				label = "简化配置"
			}
			header.Child(iconButton(label, func() { a.ui.WorkflowAdvanced = !a.ui.WorkflowAdvanced }, kit.ButtonGhost, kit.IconSettings).Render(cx))
		}
		if !guided {
			header.Child(button(map[bool]string{true: "表单", false: "源码"}[a.ui.WorkflowRaw], a.toggleWorkflowRaw, kit.ButtonGhost).Render(cx), save.Render(cx))
		}
		right.Child(dragArea(cx, el.Div().H(el.Dp(52)).WFull()), header, el.Div().Px(24).Child(a.errorBanner(cx)))
		if a.ui.WorkflowRaw {
			right.Child(a.workflowEditor.Render(cx))
		} else if guided {
			right.Child(a.simpleWorkflowForm(cx), el.Div().Row().Items(el.Center).Px(24).Py(16).Child(el.Text("保存后，可在新任务中选择这个流程。").TextSize(11).TextColor(theme.Muted).Grow(), save.Render(cx)))
		} else {
			right.Child(a.workflowForm(cx))
		}
	} else if selected != nil {
		d := *selected
		body := el.Div().ID("workflow-detail").ScrollY().Grow().H(el.Dp(0)).Px(48).Pt(16).Pb(32).Gap(32)
		mode := "串行"
		if d.Mode == domainworkflows.ModeDAG {
			mode = "并行 DAG"
		}
		body.Child(el.Div().Row().Items(el.Start).Gap(12).Child(el.Div().Grow().W(el.Dp(0)).Gap(8).Child(el.Text(mode+"  ·  "+d.ID).TextSize(11).TextColor(theme.Muted), el.Text(d.Name).TextSize(24).Bold(), el.Text(workflowPath(d)).TextSize(13).TextColor(theme.Muted)), iconButton("编辑", func() { a.selectWorkflow(d.ID, true) }, kit.ButtonGhost, kit.IconEdit).Size(30).Render(cx)))
		preview := el.Div().Gap(8).Child(el.Div().Row().Items(el.Center).Gap(8).Child(lucide(cx, "workflow", 16, theme.Muted), el.Text("流程预览").TextSize(15).Medium()))
		for i, step := range d.Steps {
			node := el.Div().Row().Items(el.Start).Gap(12).Px(16).Py(14).Rounded(12).Bg(theme.Surface)
			node.Child(el.Div().Size(el.Dp(28)).Rounded(14).Bg(theme.Subtle).Center().Child(el.Text(fmt.Sprint(i + 1)).TextSize(12).Medium().TextColor(theme.Muted)))
			content := el.Div().Grow().W(el.Dp(0)).Gap(8).Child(el.Div().Row().Items(el.Center).Gap(8).Child(el.Text(step.Name).Medium().TextSize(14), el.Text(step.Runtime).Mono().TextSize(11).TextColor(theme.Muted)))
			if d.Mode == domainworkflows.ModeDAG {
				content.Child(el.Text("依赖：" + strings.Join(step.DependsOn, ", ")).TextSize(12).TextColor(theme.Muted))
			} else {
				keys := make([]string, 0, len(step.Transitions))
				for key := range step.Transitions {
					keys = append(keys, key)
				}
				sort.Strings(keys)
				transitions := el.Div().Row().Wrap().Gap(6)
				for _, signal := range keys {
					transitions.Child(el.Text(signal + " → " + step.Transitions[signal]).Mono().TextSize(11).TextColor(theme.Muted).Bg(theme.Subtle).Rounded(5).Px(8).Py(4))
				}
				content.Child(transitions)
			}
			if step.Instruction != "" {
				content.Child(el.Text(step.Instruction).TextSize(12).TextColor(theme.Muted).MaxLines(3))
			}
			preview.Child(node.Child(content))
		}
		policy := el.Div().P(16).Rounded(12).Bg(theme.Surface).Gap(12).Child(el.Text("策略").TextSize(14).Medium())
		for _, item := range []struct{ Label, Value string }{{"最大转移次数", fmt.Sprint(d.Policy.MaxTransitions)}, {"连续失败上限", fmt.Sprint(d.Policy.MaxConsecutiveFailures)}, {"单步超时", fmt.Sprintf("%d s", d.Policy.StepTimeoutSeconds)}} {
			policy.Child(el.Div().Row().Child(el.Text(item.Label).Grow().TextSize(12).TextColor(theme.Muted), el.Text(item.Value).Mono().TextSize(12)))
		}
		body.Child(preview, policy, el.Text("工作流不会自动替换缺失的运行环境，以免破坏角色语义。").P(14).Rounded(10).Bg(theme.Subtle).TextSize(12).TextColor(theme.Muted))
		right.Child(dragArea(cx, el.Div().H(el.Dp(52)).WFull()), body)
	} else {
		right.Center().Child(el.Text("创建工作流，编排 Agent 的执行步骤。").TextSize(13).TextColor(theme.Muted))
	}
	return el.Div().Row().Grow().H(el.Dp(0)).Child(left, el.Div().W(el.Dp(1)).Bg(seamColor).NoShrink(), right, el.Div().Absolute().Top(0).Left(0).Right(0).H(el.Dp(52)).Center().Child(el.Text("工作流").TextSize(14).Medium()))
}
func (a *app) newWorkflow() {
	a.newWorkflowFrom("loop")
}
func (a *app) newWorkflowFrom(kind string) {
	before := a.currentLocation()
	a.ui.WorkflowSource = true
	a.editingWorkflow = ""
	a.ui.SelectedWorkflow = ""
	d := starterWorkflow(kind)
	base := d.ID
	for suffix := 2; ; suffix++ {
		occupied := false
		for _, existing := range a.definitions {
			if existing.ID == d.ID {
				occupied = true
				break
			}
		}
		if !occupied {
			break
		}
		d.ID = fmt.Sprintf("%s_%d", base, suffix)
	}
	if a.settings.SchemaVersion > 0 {
		e := a.settings.Execution
		d.Policy = domainworkflows.Policy{MaxTransitions: e.MaxTransitions, MaxConsecutiveFailures: e.MaxConsecutiveFailures, StepTimeoutSeconds: e.StepTimeoutSeconds}
	}
	a.ui.WorkflowDraft = domainworkflows.Normalize(d)
	a.ui.WorkflowRaw, a.ui.WorkflowStep = false, 0
	a.ui.WorkflowAdvanced = false
	a.resetWorkflowForm()
	data, _ := json.MarshalIndent(d, "", "  ")
	a.workflowEditor.SetValue(string(data))
	a.workflowEditor.SetReadOnly(false)
	a.ui.WorkflowHistory.push(before, a.currentLocation())
}
func (a *app) deleteWorkflow(id string) {
	if id == "single_agent" {
		return
	}
	a.ui.WorkflowConfirm.Confirm("删除工作流？", "删除后无法创建使用此工作流的新任务。已有执行记录会保留。", func() {
		go func() {
			err := a.backend.service.DeleteDefinition(a.ctx, id)
			core.Update(func() {
				if a.closed {
					return
				}
				a.fail(err)
				if err == nil {
					a.ui.SelectedWorkflow = ""
					a.ui.WorkflowSource = false
					a.refresh()
				}
			})
		}()
	})
}

type workflowWindowView struct{ app *app }

func (v workflowWindowView) Render(cx *el.Context) el.Element {
	a := v.app
	return el.Div().WFull().HFull().Bg(theme.Bg).Child(a.ui.WorkflowConfirm.Render(cx), a.workflowPageLayout(cx, true))
}
func (a *app) openWorkflows() {
	if a.ui.WorkflowWindow != nil && !a.ui.WorkflowWindow.Closed() {
		a.ui.WorkflowWindow.Raise()
		return
	}
	if a.ui.SelectedWorkflow == "" {
		for _, d := range a.definitions {
			if d.ID != "single_agent" {
				a.ui.SelectedWorkflow = d.ID
				break
			}
		}
	}
	a.ui.WorkflowWindow = window.Open(window.Options{Title: "OneCatch · 工作流", Width: 1040, Height: 760, MinWidth: 860, MinHeight: 600, Frameless: runtime.GOOS == "darwin", NativeTrafficLights: runtime.GOOS == "darwin", TrafficLightLayout: &window.TrafficLightLayout{Height: 52, Left: 20}, Content: el.Root(workflowWindowView{a}), OnClose: func() { a.ui.WorkflowWindow = nil }, Shortcuts: map[string]func(){"mod+[": func() { a.goWorkflowHistory(-1) }, "mod+]": func() { a.goWorkflowHistory(1) }}})
}
func (a *app) goWorkflowHistory(delta int) {
	if h := &a.ui.WorkflowHistory; len(h.Entries) > 0 {
		h.Entries[h.Index] = a.currentLocation()
	}
	target, ok := a.ui.WorkflowHistory.move(delta)
	if !ok {
		return
	}
	a.restoreWorkflowLocation(target)
}

func (a *app) restoreWorkflowLocation(target location) {
	a.ui.SelectedWorkflow, a.ui.WorkflowSource = target.Workflow, target.Source
	if !target.Source {
		return
	}
	if target.Editor != "" {
		a.editingWorkflow = target.Workflow
		a.workflowEditor.SetValue(target.Editor)
		a.workflowEditor.SetReadOnly(target.Workflow == "single_agent")
		a.ui.WorkflowRaw = true
		if !target.Raw {
			a.toggleWorkflowRaw()
		}
		a.ui.WorkflowAdvanced = target.Advanced
		return
	}
	for _, d := range a.definitions {
		if d.ID == target.Workflow {
			a.editWorkflow(d)
			return
		}
	}
}

func (a *app) workflowRowMenu(d domainworkflows.Definition) *kit.MenuView {
	if menu := a.ui.WorkflowRowMenus[d.ID]; menu != nil {
		return menu
	}
	menu := kit.Menu().Width(150).Placement(el.Right, el.Start)
	menu.ContentItem("编辑", "", menuIcon("square-pen", "编辑"), func() { a.selectWorkflow(d.ID, true) }).ContentItem("删除", "", menuIcon("trash-2", "删除"), func() { a.deleteWorkflow(d.ID) })
	menu.Trigger(el.ViewFunc(func(cx *el.Context) el.Element {
		return iconAction(cx, "workflow-actions/"+d.ID, "编辑 "+d.Name, "ellipsis", menu.Toggle, menu.Value())
	}))
	a.ui.WorkflowRowMenus[d.ID] = menu
	return menu
}
