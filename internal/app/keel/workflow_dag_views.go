package keeldesktop

import (
	"fmt"
	"image/color"
	"slices"
	"sort"
	"strings"

	"gioui.org/io/pointer"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/theme"
)

func dagAction(cx *el.Context, id, label, icon string, primary bool, disabled bool, action func()) el.Element {
	variant, fg := kit.ButtonSecondary, theme.Text
	if primary {
		variant, fg = kit.ButtonPrimary, theme.OnColor
	}
	b := button(label, action, variant).ID(id).Name(label).Size(32).Compact(true)
	b.SetDisabled(disabled)
	b.Content(el.ViewFunc(func(cx *el.Context) el.Element {
		return el.Div().Row().Items(el.Center).Gap(6).Child(lucide(cx, icon, 14, fg), el.Text(label).TextSize(12).MaxLines(1))
	}))
	return b.Render(cx)
}

func (a *app) dagEditor(cx *el.Context) el.Element {
	d := &a.ui.WorkflowDraft
	top := el.Div().Row().Items(el.Center).H(el.Dp(32)).Gap(8).Child(el.Div().Row().Items(el.Center).Gap(6).Px(8).Py(5).Rounded(6).Bg(theme.Subtle).Child(lucide(cx, "git-branch", 12, theme.Muted), el.Text("DAG · 全部汇合").TextSize(11).Medium()), el.Div().Grow(),
		dagAction(cx, "dag-layout", "自动布局", "layout-grid", false, a.busy, a.layoutDAG),
		dagAction(cx, "dag-add", "节点", "plus", false, a.busy, a.addDAGNode),
		dagAction(cx, "dag-save", map[bool]string{true: "保存中…", false: "保存 DAG"}[a.busy], "save", true, a.busy || a.ui.DAG.IDError != "" || a.ui.DAG.SignalError != "", a.saveWorkflow))
	identity := el.Div().Row().Gap(16).P(16).H(el.Dp(94)).NoShrink().Rounded(12).Bg(theme.Surface).Child(
		el.Div().Grow().W(el.Dp(0)).Child(dagField("名称", a.dagText("workflow-name", "工作流名称", &d.Name))),
		el.Div().Grow().W(el.Dp(0)).Child(dagField("ID", field("workflow-id", "工作流 ID", &d.ID).Mono().TextSize(14).H(el.Dp(36)).WFull().OnChange(func(string) { a.syncWorkflowDocument() }))))
	header := el.Div().Px(20).Py(16).Gap(16).NoShrink().Child(top, identity, el.Div().Absolute().Left(0).Right(0).Bottom(0).H(el.Dp(1)).Bg(seamColor))
	hint := "拖动节点调整布局 · 点击边上的 × 删除依赖"
	if a.ui.DAG.ConnectFrom != "" {
		hint = "正在从 " + a.ui.DAG.ConnectFrom + " 连接，点击目标节点连接点"
	}
	canvas := el.Div().ID("dag-canvas").Grow().W(el.Dp(0)).HFull().Child(el.Div().ID("dag-scroll").WFull().HFull().ScrollX().ScrollY().Child(a.dagCanvas(cx)), el.Div().Absolute().Left(12).Bottom(12).MaxW(el.Frac(.94)).Px(9).Py(6).Rounded(7).Bg(theme.Surface).Child(el.Text(hint).TextSize(11).TextColor(theme.Muted).MaxLines(1)))
	workspace := el.Div().Row().Grow().H(el.Dp(0)).Child(canvas, el.Div().W(el.Dp(1)).HFull().NoShrink().Bg(seamColor), a.dagInspector(cx))
	footer := el.Div().ID("dag-validation").Row().Items(el.Center).NoShrink().MinH(el.Dp(44)).Px(16).Py(6).Gap(12).Bg(theme.Subtle).Child(dagAction(cx, "dag-validate", "校验 DAG", "circle-check", false, false, a.validateDAG))
	notes := el.Div().Row().Grow().W(el.Dp(0)).ScrollX().Gap(12)
	if len(a.ui.DAG.Validation) == 0 {
		notes.Child(el.Text("保存前会检查环、未知依赖和并行写冲突。").TextSize(12).TextColor(theme.Muted).MaxLines(1))
	} else {
		for _, issue := range a.ui.DAG.Validation {
			notes.Child(el.Text(issue.Path + " · " + issue.Message).TextSize(12).TextColor(theme.DangerText).MaxLines(1))
		}
	}
	footer.Child(notes)
	return el.Div().ID("dag-editor").Grow().H(el.Dp(0)).Disabled(a.busy).Child(header, el.Div().Px(20).Child(a.errorBanner(cx)), workspace, footer)
}

func (a *app) dagCanvas(cx *el.Context) el.Element {
	d := a.ui.WorkflowDraft
	w, h := dagExtent(d)
	content := el.Div().W(el.Dp(w)).H(el.Dp(h)).NoShrink().Child(el.Widget(dagDrawing{App: a}).WFull().HFull())
	for _, edge := range dagEdges(d) {
		e := edge
		m := e.midpoint()
		id := "dag-edge/" + e.Source + "/" + e.Target
		alpha := float32(0)
		if cx.Hovered(id) || cx.FocusVisible(id) || a.ui.DAG.Edges.Hover != nil && a.ui.DAG.Edges.Hover.Source == e.Source && a.ui.DAG.Edges.Hover.Target == e.Target {
			alpha = 1
		}
		content.Child(el.Div().Absolute().Left(m.X - 11).Top(m.Y - 23).Size(el.Dp(22)).Center().ID(id).Name("删除依赖 " + e.Source + " → " + e.Target).Role("button").Focusable(true).CursorPointer().OnClick(func() { a.removeDAGEdge(e.Source, e.Target) }).Hover(func(st *el.Style) { st.Bg(theme.Highlight) }).FocusStyle(func(st *el.Style) { st.BorderColor(color.NRGBA{}).Bg(theme.Highlight) }).Rounded(5).Child(el.Div().Opacity(alpha).Child(lucide(cx, "x", 12, theme.DangerText))))
	}
	for _, step := range d.Steps {
		s := step
		p := dagPosition(d, s.ID)
		active := a.ui.WorkflowStepID == s.ID || a.ui.DAG.ConnectFrom == s.ID
		fill := theme.Surface
		if active {
			fill = theme.Highlight
		}
		wrapper := el.Div().Absolute().Left(float32(p.X)).Top(float32(p.Y)).W(el.Dp(dagNodeWidth)).H(el.Dp(dagNodeHeight))
		node := el.Div().WFull().HFull().Rounded(10).Bg(fill).Px(14).Py(12).
			ID("dag-node/" + s.ID).Role("button").Name("节点 " + s.Name).Selected(active).Focusable(true).Cursor(pointer.CursorGrab).
			FocusStyle(func(st *el.Style) { st.BorderColor(color.NRGBA{}).Bg(theme.Highlight) }).
			OnClick(func() { a.selectDAGNode(s.ID) }).DragAccept(func(float32, float32) bool { return true }).OnDrag(func(e el.DragEvent) {
			switch e.Kind {
			case el.DragStart:
				a.selectDAGNode(s.ID)
				a.ui.DAG.DragID = s.ID
				a.ui.DAG.DragX, a.ui.DAG.DragY = e.X, e.Y
			case el.DragMove:
				if a.ui.DAG.DragID == s.ID {
					current := dagPosition(a.ui.WorkflowDraft, s.ID)
					a.moveDAGNode(s.ID, current.X+float64(e.X-a.ui.DAG.DragX), current.Y+float64(e.Y-a.ui.DAG.DragY))
				}
			case el.DragEnd:
				a.ui.DAG.DragID = ""
			}
		}).OnKey(func(e el.KeyEvent) bool {
			if e.State != el.KeyPress {
				return false
			}
			p := dagPosition(a.ui.WorkflowDraft, s.ID)
			switch e.Name {
			case "←":
				p.X -= 8
			case "→":
				p.X += 8
			case "↑":
				p.Y -= 8
			case "↓":
				p.Y += 8
			case "⎋":
				a.ui.DAG.ConnectFrom = ""
				return true
			default:
				return false
			}
			a.moveDAGNode(s.ID, p.X, p.Y)
			return true
		})
		head := el.Div().Row().Items(el.Center).Gap(6).Mb(7).H(el.Dp(19)).Child(el.Text(s.Runtime).Mono().TextSize(10).MaxLines(1).Px(8).Py(2).H(el.Dp(19)).MaxW(el.Dp(118)), el.Div().Grow(), el.Text(s.WorkerID).Mono().TextSize(12).TextColor(theme.Muted).MaxLines(1))
		hint := "根节点 · 可并行"
		if len(s.DependsOn) > 0 {
			hint = fmt.Sprintf("等待 %d 个依赖", len(s.DependsOn))
		}
		node.Child(head, el.Text(s.Name).Bold().TextSize(14).H(el.Dp(21)).MaxLines(1), el.Text(hint).TextSize(12).H(el.Dp(18)).TextColor(theme.Muted).Mt(5).MaxLines(1))
		wrapper.Child(node)
		content.Child(wrapper)
		for _, side := range []string{"input", "output"} {
			name := "连接到 " + s.Name
			x := float32(-6)
			if side == "output" {
				name = "从 " + s.Name + " 连接"
				x = dagNodeWidth - 4
			}
			port := el.Div().Absolute().Left(float32(p.X) + x - 4).Top(float32(p.Y) + 38).Size(el.Dp(18)).Center().ID("dag-port/" + side + "/" + s.ID).Role("button").Name(name).Focusable(true).Cursor(pointer.CursorCrosshair).
				OnClick(func() { a.connectDAGNode(s.ID) }).FocusStyle(func(st *el.Style) { st.BorderColor(color.NRGBA{}).Bg(theme.Highlight) })
			port.Child(el.Div().Size(el.Dp(10)).Rounded(5).Bg(theme.Info))
			content.Child(port)
		}
	}
	return content
}

func (a *app) dagInspector(cx *el.Context) el.Element {
	d := &a.ui.WorkflowDraft
	panel := el.Div().ID("dag-inspector").W(el.Dp(320)).NoShrink().HFull().Bg(theme.Bg)
	if len(d.Steps) == 0 {
		return panel
	}
	a.ui.WorkflowStep = min(max(0, a.ui.WorkflowStep), len(d.Steps)-1)
	i := a.ui.WorkflowStep
	s := &d.Steps[i]
	remove := iconButton("", func() { a.removeWorkflowStep(i) }, kit.ButtonGhost, kit.IconTrash).ID("dag-delete").Name("删除节点").Size(28).Compact(true)
	remove.SetDisabled(len(d.Steps) <= 1)
	remove.Content(el.ViewFunc(func(cx *el.Context) el.Element { return lucide(cx, "trash-2", 14, theme.DangerText) }))
	panel.Child(el.Div().Row().Items(el.Start).Px(16).Py(14).H(el.Dp(77)).NoShrink().Gap(12).Child(el.Div().Grow().W(el.Dp(0)).Gap(4).Child(el.Text("节点检查器").TextSize(11).TextColor(theme.Muted).Medium(), el.Text(s.Name).TextSize(14).Bold().MaxLines(1)), remove.Render(cx), el.Div().Absolute().Left(0).Right(0).Bottom(0).H(el.Dp(1)).Bg(seamColor)))
	fields := el.Div().ID("dag-fields").Grow().H(el.Dp(0)).ScrollY().P(16).Gap(16)
	fields.Child(dagField("节点 ID", field("dag-node-id", "节点 ID", &a.ui.WorkflowStepID).Mono().TextSize(14).H(el.Dp(36)).WFull().OnChange(func(id string) {
		old := s.ID
		if err := renameWorkflowStep(d, i, strings.TrimSpace(id)); err != nil {
			a.ui.DAG.IDError = err.Error()
			return
		}
		if a.ui.DAG.ConnectFrom == old {
			a.ui.DAG.ConnectFrom = d.Steps[i].ID
		}
		if order, ok := a.ui.DAG.SignalOrder[old]; ok {
			delete(a.ui.DAG.SignalOrder, old)
			a.ui.DAG.SignalOrder[d.Steps[i].ID] = order
		}
		a.ui.DAG.IDError = ""
		a.resetWorkflowSelects()
		a.syncWorkflowDocument()
	})), dagField("名称", a.dagText("dag-node-name", "节点名称", &s.Name)))
	if a.ui.DAG.IDError != "" {
		fields.Child(el.Text(a.ui.DAG.IDError).TextSize(11).TextColor(theme.DangerText))
	}
	var runtimes []kit.SelectOption
	for _, r := range a.runtimes {
		runtimes = append(runtimes, kit.SelectOption{Value: r.ID, Label: r.Name, Disabled: !r.Enabled && r.ID != s.Runtime})
	}
	if !slices.ContainsFunc(runtimes, func(o kit.SelectOption) bool { return o.Value == s.Runtime }) {
		runtimes = append(runtimes, kit.SelectOption{Value: s.Runtime, Label: s.Runtime})
	}
	workers := []kit.SelectOption{{Value: "local", Label: "本地"}}
	for _, w := range a.workers {
		if w.Enabled {
			workers = append(workers, kit.SelectOption{Value: w.ID, Label: w.Name})
		}
	}
	if !slices.ContainsFunc(workers, func(o kit.SelectOption) bool { return o.Value == s.WorkerID }) {
		workers = append(workers, kit.SelectOption{Value: s.WorkerID, Label: s.WorkerID + " · 未配置"})
	}
	fields.Child(dagField("运行环境", a.workflowSelect(cx, "dag-runtime/"+s.ID, "运行环境", s.Runtime, runtimes, func(value string) { s.Runtime = value })),
		dagField("Worker", a.workflowSelect(cx, "dag-worker/"+s.ID, "Worker", s.WorkerID, workers, func(value string) { a.setDAGWorker(i, value) })),
		dagField("沙箱", a.workflowSelect(cx, "dag-sandbox/"+s.ID, "沙箱", s.Sandbox, []kit.SelectOption{{Value: "read-only", Label: "只读"}, {Value: "workspace-write", Label: "工作区访问"}, {Value: "full", Label: "完全访问", Disabled: s.WorkerID != "local" || !a.settings.Security.AllowFullSandbox && s.Sandbox != "full"}}, func(value string) { s.Sandbox = value })),
		dagField("角色提示", el.TextArea().ID("dag-role/"+s.ID).Name("角色提示").Bind(&s.RolePrompt).AutoGrow(4, 8).WFull().Border(0, theme.Border).Bg(theme.Subtle).Rounded(8).P(10).TextSize(13).OnChange(func(string) { a.syncWorkflowDocument() })),
		dagField("节点指令", el.TextArea().ID("dag-instruction/"+s.ID).Name("节点指令").Bind(&s.Instruction).AutoGrow(5, 10).WFull().Border(0, theme.Border).Bg(theme.Subtle).Rounded(8).P(10).TextSize(13).OnChange(func(string) { a.syncWorkflowDocument() })))
	transitions := el.Div().P(14).Rounded(12).Bg(theme.Subtle).Gap(12).Child(el.Text("终点信号").TextSize(12).Bold())
	if a.ui.DAG.SignalOrder == nil {
		a.ui.DAG.SignalOrder = map[string][]string{}
	}
	keys := slices.DeleteFunc(a.ui.DAG.SignalOrder[s.ID], func(key string) bool { _, ok := s.Transitions[key]; return !ok })
	var extra []string
	for signal := range s.Transitions {
		if !slices.Contains(keys, signal) {
			extra = append(extra, signal)
		}
	}
	sort.Strings(extra)
	keys = append(keys, extra...)
	a.ui.DAG.SignalOrder[s.ID] = keys
	for index, key := range keys {
		signal := key
		value := key
		transitions.Child(el.Div().P(12).Rounded(8).Bg(theme.Surface).Gap(12).Child(el.Text(fmt.Sprintf("转移 %d", index+1)).TextSize(11).TextColor(theme.Muted),
			dagField("信号", field(fmt.Sprintf("dag-signal/%s/%d", s.ID, index), "信号", &value).Mono().TextSize(14).H(el.Dp(36)).WFull().OnChange(func(next string) {
				if next == signal {
					a.ui.DAG.SignalError = ""
					return
				}
				if _, exists := s.Transitions[next]; exists {
					a.ui.DAG.SignalError = fmt.Sprintf("信号 %s 已存在", next)
					return
				}
				target := s.Transitions[signal]
				a.ui.DAG.SignalError = ""
				delete(s.Transitions, signal)
				s.Transitions[next] = target
				a.ui.DAG.SignalOrder[s.ID][index] = next
				signal = next
				a.syncWorkflowDocument()
			})), dagField("目标", a.workflowSelect(cx, fmt.Sprintf("dag-target/%s/%d", s.ID, index), key+" target", s.Transitions[key], []kit.SelectOption{{Value: "$done", Label: "$done"}, {Value: "$pause", Label: "$pause"}, {Value: "$fail", Label: "$fail"}}, func(next string) { s.Transitions[signal] = next }))))
	}
	if a.ui.DAG.SignalError != "" {
		transitions.Child(el.Text(a.ui.DAG.SignalError).TextSize(11).TextColor(theme.DangerText))
	}
	fields.Child(transitions, button("源码", a.toggleWorkflowRaw, kit.ButtonGhost).ID("dag-source").Render(cx))
	panel.Child(fields)
	return panel
}

func dagField(label string, content el.Element) el.Element {
	return el.Div().Gap(8).WFull().Child(el.Text(label).TextSize(12).H(el.Dp(18)).TextColor(theme.Muted), content)
}
func (a *app) dagText(id, label string, value *string) el.Element {
	return field(id, label, value).TextSize(14).H(el.Dp(36)).WFull().OnChange(func(string) { a.syncWorkflowDocument() })
}
