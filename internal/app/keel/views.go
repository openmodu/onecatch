package keeldesktop

import (
	"encoding/json"
	"image"
	"image/color"
	"runtime"
	"strings"

	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/io/system"
	"gioui.org/op/clip"
	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/theme"
	domaintasks "github.com/openmodu/onecatch/internal/domain/tasks"
	domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

var (
	sidebarColor = theme.RGB(0xfafaf9)
	seamColor    = theme.RGB(0xe4e4e1)
)

func accessWarningColor() color.NRGBA {
	if theme.Text.R > 128 {
		return theme.RGB(0xf28b82)
	}
	return theme.RGB(0xb7443e)
}

func applyPalette(dark bool) { applyAppPalette(dark, "forest") }

func applyAppPalette(dark bool, accent string) {
	p := theme.Light()
	if dark {
		p = theme.Dark()
		p.Bg, p.Surface, p.Subtle, p.SubtleHover = theme.RGB(0x222222), theme.RGB(0x2c2c2c), theme.RGB(0x333333), theme.RGB(0x3e3e3e)
		sidebarColor, seamColor = theme.RGB(0x272727), theme.RGB(0x393939)
	} else {
		p.Bg, p.Surface = theme.RGB(0xfcfcfb), theme.RGB(0xffffff)
		p.Text, p.Muted = theme.RGB(0x1a1a1a), theme.RGB(0x73726e)
		p.Subtle, p.SubtleHover = theme.RGB(0xf2f2f1), theme.RGB(0xedece8)
		sidebarColor, seamColor = theme.RGB(0xfafaf9), theme.RGB(0xe4e4e1)
	}
	p.OnColor = theme.RGB(0xffffff)
	if dark {
		p.OnColor = p.Bg
		p.Text = theme.RGB(0xefefef)
		p.Muted = theme.RGB(0xb6b6b6)
	}
	p.Primary = p.Text
	p.Frameless = true
	p.Info = theme.RGB(0x216e78)
	if dark {
		p.Info = theme.RGB(0x73e0eb)
	}
	p.PrimaryHover, p.PrimaryText = p.Primary, p.Primary
	p.Highlight = p.SubtleHover
	p.CodeBg = p.Subtle
	// All controls use fills for selection and focus. Only pane seams have a
	// stroke; they are drawn explicitly rather than as a component's outer box.
	p.Border = color.NRGBA{}
	// Gio composites its scrim in linear light; this matches the source's
	// half-black CSS overlay after conversion to display pixels.
	p.Scrim = color.NRGBA{A: 0xc8}
	theme.Apply(p)
	theme.Material.Face = platformUIFace(runtime.GOOS)
}

func button(label string, action func(), variant kit.ButtonVariant) *kit.ButtonView {
	return kit.Button(uiText(label), action).Variant(variant).Appearance(func(p kit.ButtonAppearance) kit.ButtonAppearance {
		p.Border, p.Focus = color.NRGBA{}, color.NRGBA{}
		return p
	})
}

// Picker entries share a leading icon column and fill the clickable row.
// Ordinary action buttons keep Keel's centered label layout.
func listOption(cx *el.Context, id, label string, mark el.Element, callback func(), selected, disabled bool) el.Element {
	v := button(label, callback, kit.ButtonGhost).ID(id).Name(label).Selected(selected).Compact(true).Size(32).
		Content(el.ViewFunc(func(cx *el.Context) el.Element {
			icon := el.Div().ID(id + "/icon").Role("image").Name(label + "图标").Size(el.Dp(16)).NoShrink().Center().Child(mark)
			fg := theme.Text
			if disabled {
				fg = theme.Muted
				icon.Opacity(.35)
			}
			return el.Div().WFull().Row().Items(el.Center).Gap(8).Child(icon, el.Text(label).TextSize(13).Weight(font.Normal).TextColor(fg).MaxLines(1).Grow().W(el.Dp(0)))
		}))
	v.SetDisabled(disabled)
	return v.Render(cx).(*el.DivEl).WFull().Px(8).Justify(el.Start).FocusStyle(neutralFocus)
}

// Keel's default keyboard focus adds a stroke even when the base border is
// transparent. Match the original frameless controls with a visible fill.
func neutralFocus(s *el.Style)      { s.BorderColor(color.NRGBA{}).Bg(theme.Highlight) }
func neutralInputFocus(s *el.Style) { s.BorderColor(color.NRGBA{}).Bg(theme.SubtleHover) }

func iconButton(label string, action func(), variant kit.ButtonVariant, name kit.IconName) *kit.ButtonView {
	return button(label, action, variant).Icon(name).Content(el.ViewFunc(func(cx *el.Context) el.Element {
		fg := theme.Text
		if variant == kit.ButtonPrimary {
			fg = theme.OnColor
		}
		row := el.Div().Row().Items(el.Center).Gap(6).Child(outlineIcon(cx, name, 16, fg))
		if label != "" {
			row.Child(el.Text(label).TextSize(12).MaxLines(1))
		}
		return row
	}))
}

func action(cx *el.Context, id, label string, icon kit.IconName, callback func()) el.Element {
	return iconButton(label, callback, kit.ButtonGhost, icon).ID(id).Name(label).Size(28).Compact(true).Render(cx)
}

func field(id, hint string, value *string) *el.InputEl {
	return el.Input().ID(id).Name(uiText(hint)).Placeholder(uiText(hint)).Bind(value).Border(0, color.NRGBA{}).Bg(theme.Subtle).Rounded(8).Px(12).TextSize(13).
		FocusStyle(neutralInputFocus)
}

func dragArea(cx *el.Context, d *el.DivEl) *el.DivEl {
	if runtime.GOOS != "darwin" && !usesDesktopHeader(runtime.GOOS) {
		return d
	}
	if usesDesktopHeader(runtime.GOOS) {
		return windowsDragArea(d)
	}
	return d.Decorate(func(gtx core.C, draw func()) {
		if w := core.CurrentWindow(); w != nil {
			origin, visible := cx.PaintGeometry()
			rect := image.Rectangle{Min: origin, Max: origin.Add(gtx.Constraints.Max)}.Intersect(visible)
			scale := gtx.Metric.PxPerDp
			if scale <= 0 {
				scale = 1
			}
			w.TitleBarArea(float32(rect.Min.X)/scale, float32(rect.Min.Y)/scale, float32(rect.Dx())/scale, float32(rect.Dy())/scale)
			area := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
			system.ActionInputOp(system.ActionMove).Add(gtx.Ops)
			area.Pop()
		}
		draw()
	})
}

func (a *app) Render(cx *el.Context) el.Element {
	root := el.Div().Row().WFull().HFull().Bg(theme.Bg).TextSize(14)
	peekTrigger := a.sidebarPeek(cx)
	if a.settingsPanel != nil {
		root.Child(el.Div().ID("settings-modal").Absolute().Size(el.Dp(0)))
		a.settingsOverlay(cx)
	}
	if a.ui.SearchPalette != nil {
		root.Child(a.renderSearchPalette(cx))
	}
	root.Child(a.projectDialog.Render(cx), a.ui.Customize.Render(cx), a.confirmation.Render(cx), a.ui.TaskDialog.Render(cx))
	if a.page == "skills" {
		root.Child(a.skills.Create.Render(cx), a.skills.Confirm.Render(cx))
	}
	if !a.sidebarCollapsed(cx) {
		root.Child(a.sidebar(cx), a.sidebarSeam(cx))
	}
	if !((a.workbench.Visible && a.workbench.Maximized || a.terminal.Open && a.terminal.Maximized) && a.page == "" || a.page == "skills" && a.skills.Visible && a.skills.Maximized) {
		center := el.Div().Grow().W(el.Dp(0)).HFull().Decorate(func(gtx core.C, draw func()) {
			defer clip.Rect(image.Rectangle{Max: gtx.Constraints.Max}).Push(gtx.Ops).Pop()
			draw()
		})
		if !usesDesktopHeader(runtime.GOOS) {
			center.Child(a.header(cx))
		}
		if a.page != "" && a.page != "skills" || !a.terminal.Open || !a.terminal.Maximized {
			center.Child(a.content(cx))
		}
		if a.page == "skills" && a.terminal.Open {
			center.Child(a.terminalDock(cx))
		}
		root.Child(center)
	}
	if a.page == "" {
		if a.terminal.Open {
			root.Child(a.inspectorSeam(cx), a.sidePanelCard(cx, a.terminalDock(cx)))
		} else if a.workbench.Visible {
			root.Child(a.inspectorSeam(cx), a.sidePanelCard(cx, a.inspector(cx)))
		}
	}
	if a.page == "skills" && a.skills.Visible {
		root.Child(a.skillResizeSeam("skill-inspector-resize", "调整 Skills 检查面板宽度", &a.skills.Width, 280, 820, true), a.skillInspector(cx))
	}
	root.Child(peekTrigger)
	if usesDesktopHeader(runtime.GOOS) {
		return el.Div().WFull().HFull().Bg(theme.Bg).Child(a.header(cx), root.Grow().H(el.Dp(0)))
	}
	return root
}

func (a *app) header(cx *el.Context) el.Element { return a.headerForPlatform(cx, runtime.GOOS) }

func (a *app) headerForPlatform(cx *el.Context, platform string) el.Element {
	title := "新建任务"
	if a.page != "" {
		for _, item := range sidebarItems {
			if item.ID == a.page {
				title = item.Label
			}
		}
	} else if a.task != nil {
		title = a.task.Title
	}
	if a.page == "skills" {
		title = "Skills 管理"
	}
	left := el.Div().Row().Items(el.Center).Gap(10).Grow().HFull().Child(outlineIcon(cx, kit.IconFolder, 18, theme.Muted))
	if a.task != nil && a.page == "" {
		category := a.task.Category
		if category == "" || category == "other" {
			category = "其他"
		}
		left.Child(el.Div().Px(5).Py(2).Rounded(4).Bg(theme.Subtle).Child(el.Text(category).TextSize(10).TextColor(theme.Muted)))
	}
	if usesDesktopHeader(platform) {
		left = windowsDragArea(left)
	} else {
		left = dragArea(cx, left)
	}
	left.Child(el.Text(title).Grow().W(el.Dp(0)).MaxLines(1).Medium().TextSize(13))
	bar := el.Div().Row().Items(el.Center).H(el.Dp(mainTitleBarHeight)).Px(18).Gap(4)
	if !usesDesktopHeader(platform) && a.sidebarCollapsed(cx) {
		if platform == "darwin" {
			bar.Pl(macSidebarToggleLeft)
		}
		bar.Child(iconAction(cx, "show-sidebar", "展开侧栏", "panel-left", a.toggleSidebar, false))
	}
	if usesDesktopHeader(platform) {
		bar.Px(0).Gap(0).NoShrink()
		leading := el.Div().ID("windows-sidebar-header").Row().Items(el.Center).HFull().Px(18).Gap(4).NoShrink()
		id, label := "hide-sidebar", "收起侧栏"
		if a.sidebarCollapsed(cx) {
			id, label = "show-sidebar", "展开侧栏"
		} else {
			leading.W(el.Dp(a.sidebarWidth(cx))).Bg(sidebarColor)
		}
		leading.Child(a.windowsApplicationMenu(cx), iconAction(cx, id, label, "panel-left", a.toggleSidebar, false))
		bar.Child(leading)
		if !a.sidebarCollapsed(cx) {
			bar.Child(el.Div().ID("windows-sidebar-header-seam").W(el.Dp(1)).HFull().Bg(seamColor).NoShrink())
		}
		left.Pl(18)
	}
	bar.Child(left)
	if a.page == "" || a.page == "skills" {
		bar.Child(iconAction(cx, "toggle-terminal", "Terminal", "terminal", a.toggleTerminal, a.terminal.Open))
	}
	if a.page == "" {
		bar.Child(
			iconAction(cx, "show-changes", "Changes", "square-diff", func() { a.toggleInspector("review") }, a.workbench.Visible && a.workbench.Tab == "review"),
			iconAction(cx, "show-files", "Files", "folder", func() { a.toggleInspector("files") }, a.workbench.Visible && a.workbench.Tab == "files"),
			a.headerMore(cx),
		)
	}
	if a.page == "skills" && !a.skills.Visible {
		bar.Child(iconAction(cx, "show-skill-inspector", "展开检查面板", "panel-right-open", func() { a.openSkillInspector(a.skills.Tab) }, false))
	}
	if usesDesktopHeader(platform) {
		bar.H(el.Dp(44)).Pr(0).Child(desktopWindowButtons(cx))
	}
	return bar
}

func (a *app) content(cx *el.Context) el.Element {
	switch a.page {
	case "templates":
		return a.templatesPage(cx)
	case "skills":
		return a.skillsPage(cx)
	case "usage":
		return a.usagePage(cx)
	}
	if a.task == nil {
		return el.Div().Grow().Center().Px(24).Pb(46).Child(el.Div().WFull().MaxW(el.Dp(920)).Gap(18).Child(
			el.Div().WFull().Items(el.Center).Gap(8).Child(el.Text(uiText("你想推进什么？")).TextSize(28).Bold(), el.Text(uiText("描述目标和约束，任务标题会由 AI 自动生成。")).TextSize(13).TextColor(theme.Muted)),
			a.composer(cx), a.workspaceCaption(cx), a.errorBanner(cx),
		))
	}
	column := el.Div().Grow().H(el.Dp(0))
	// Only when nothing is on screen yet: above a shown conversation the
	// notice pushed every message down while a run reloaded.
	if a.loading && len(a.entries) == 0 {
		column.Child(el.Div().Px(32).Py(8).Child(el.Text("正在读取对话…").TextColor(theme.Muted).TextSize(12)))
	}

	if a.detail != nil && a.detail.Workflow.ID != "single_agent" && len(a.detail.Workflow.Steps) > 1 {
		column.Child(el.Div().Items(el.Center).Px(24).Py(8).Child(a.workflowLoopBar(cx)))
	}
	column.Child(a.conversationScroll(cx), el.Div().Center().Px(24).Pt(8).Pb(14).Child(el.Div().WFull().MaxW(el.Dp(920)).Gap(10).Child(a.errorBanner(cx), a.composerContext(cx), a.composer(cx), a.workspaceCaption(cx))))
	return column
}

func (a *app) composer(cx *el.Context) el.Element {
	if a.promptDocument.Content().Text() != a.prompt {
		_ = a.promptDocument.SetText(a.prompt)
	}
	a.composerSkills(cx)
	a.composerMentions(cx)
	hint := "随心输入，@ 选择工作流"
	if a.detail != nil {
		hint = "继续发送消息…"
		if a.detail.Run.Status == domainworkflows.RunPaused && !a.detail.Active {
			hint = "补充指令并恢复运行…"
		}
		if _, ok := a.mentionWorkflow(); ok {
			hint = strings.TrimSuffix(hint, "…") + "，@ 某个步骤只发给它…"
		}
	}
	input := el.TextArea().PasteReader(readClipboard).OnPaste(a.pasteTaskAttachments).OnPasteError(a.fail).ID("prompt").Name("任务指令").Document(&a.promptDocument).OnChange(func(text string) { a.prompt = text }).Placeholder(hint).AutoGrow(4, 10).Scrollbars(el.ScrollbarAlways).Border(0, color.NRGBA{}).Bg(color.NRGBA{}).Px(2).Py(2).TextSize(14).Weight(font.Normal).LineHeight(1.6).WFull()
	// As in the Wails client: Enter sends, Shift+Enter breaks the line, and
	// an input method composing text keeps Enter to confirm its candidate.
	input.CaptureKeys(string(key.NameReturn), string(key.NameEnter)).OnKey(func(e el.KeyEvent) bool {
		if a.composerSkillKey(cx, e) || a.composerMentionKey(cx, e) {
			return true
		}
		if e.State == el.KeyPress {
			a.send()
		}
		return true
	})
	if a.skillPicker.open || a.mentionPicker.open {
		input.CaptureKeys(string(key.NameReturn), string(key.NameEnter), string(key.NameTab), string(key.NameEscape), string(key.NameUpArrow), string(key.NameDownArrow))
	}
	if a.detail != nil && a.detail.Run.Status != domainworkflows.RunRunning && a.detail.Run.Status != domainworkflows.RunPaused && a.detail.Run.Status != domainworkflows.RunCompleted {
		input.ReadOnly(true).Placeholder("此执行已结束")
	}
	if a.task != nil && a.runID == "" {
		input.ReadOnly(true).Placeholder("点击开始执行，运行上面的任务。").AutoGrow(1, 1)
		if a.task.Status == domaintasks.StatusQueued {
			input.Placeholder("排队等待执行…")
		}
	}
	controls := el.Div().Row().Items(el.Center).Gap(8).NoShrink()
	if a.task == nil && !a.remoteConversation() {
		controls.Child(a.ui.Add.Render(cx))
	}
	if a.task != nil && a.runID != "" && !a.remoteConversation() {
		attachment := iconAction(cx, "attach-files", "添加附件", "paperclip", a.chooseAttachments, false)
		controls.Child(attachment)
	}
	controls.Child(iconButton("模板", func() { a.goPage("templates") }, kit.ButtonGhost, kit.IconFile).Size(32).Compact(true).Render(cx))
	controls.Child(a.executorPicker(cx))
	if a.task != nil {
		label := map[string]string{"full": "完全访问", "read-only": "只读访问", "workspace-write": "工作区访问"}[a.task.Sandbox]
		if label == "" {
			label = "工作区访问"
		}
		controls.Child(accessValue(cx, a.task.Sandbox, label))
	} else {
		controls.Child(el.Div().W(el.Dp(136)).Child(a.sandbox.Render(cx)))
	}
	if gauge := a.contextGauge(cx); gauge != nil {
		controls.Child(gauge)
	}
	leftControls := controls
	controls = el.Div().Row().Items(el.Center).Gap(8)
	controls.Child(el.Div().Grow().W(el.Dp(0)).ScrollX().Child(leftControls))
	if !a.remoteConversation() && a.workflow.Value() == "single_agent" {
		controls.Child(a.runtimeProfilePicker(cx))
	}
	sendLabel := "发送"
	if a.detail != nil {
		if a.detail.Run.Status == domainworkflows.RunPaused {
			sendLabel = "恢复运行"
		} else if a.detail.Run.Status == domainworkflows.RunRunning {
			sendLabel = "加入后续消息"
		}
	}
	if a.task != nil && a.runID == "" {
		sendLabel = "开始执行"
	}
	if a.library.Queue && a.task == nil {
		sendLabel = "加入队列"
	}
	send := iconButton("", a.send, kit.ButtonPrimary, kit.IconExternalLink).ID("send").Name(sendLabel).Size(30).Compact(true).Loading(a.busy)
	canResumeEmpty := a.detail != nil && !a.detail.Active && a.detail.Run.Status == domainworkflows.RunPaused
	sendDisabled := a.instructionBusy() || a.loading || a.task != nil && a.task.Status == domaintasks.StatusQueued && a.runID == "" || (a.task == nil || a.runID != "") && !canResumeEmpty && strings.TrimSpace(a.prompt) == "" && len(a.attachments.Items) == 0 || a.attachments.Loading || a.attachments.Picking
	if a.task == nil && !a.conversationWorkerReady() {
		sendDisabled = true
	}
	send.SetDisabled(sendDisabled)
	sendControl := send.Render(cx)
	if circle, ok := sendControl.(*el.DivEl); ok {
		circle.Rounded(7).W(el.Dp(32)).H(el.Dp(32)).Px(0)
		if sendDisabled {
			circle.DisabledStyle(func(s *el.Style) { s.Bg(mixColor(theme.Primary, theme.Subtle, 0.5)) })
		}
	}
	if a.detail != nil && a.detail.Active && strings.TrimSpace(a.prompt) == "" && len(a.attachments.Items) == 0 {
		stop := iconButton("", a.stop, kit.ButtonSecondary, kit.IconBan).ID("stop").Name("停止当前轮次").Size(30).Compact(true).Loading(a.busy)
		stop.SetDisabled(a.instructionBusy() || a.loading)
		controls.Child(stop.Render(cx))
	} else if a.detail == nil || a.detail.Run.Status == domainworkflows.RunRunning || a.detail.Run.Status == domainworkflows.RunPaused || a.detail.Run.Status == domainworkflows.RunCompleted {
		controls.Child(sendControl)
	}
	composerBg := theme.Subtle
	box := el.Div().WFull().Px(12).Py(10).Gap(8).Rounded(20).Bg(composerBg)
	if len(a.attachments.Items) > 0 || a.attachments.Loading {
		box.Child(a.attachmentStrip(cx))
	}
	// The composer is already the input's surface. Keep the transparent editor
	// transparent on focus too; the global frameless focus fill is for controls.
	inputPalette := theme.Current()
	inputPalette.Frameless = false
	// Keel renders the native reference pill, including its ink bounds and
	// baseline; OneCatch only selects the background color.
	inputPalette.Subtle = mixColor(theme.Info, composerBg, .20)
	box.Child(cx.Themed(inputPalette, el.ViewFunc(func(*el.Context) el.Element { return input })).WFull(), controls)
	if a.task == nil && !a.conversationWorkerReady() {
		box.Child(el.Text(uiText("所选 Worker 不可用，或不支持当前 Agent。请选择可用的执行位置。")).TextSize(12).TextColor(theme.Danger))
	}
	return el.Div().WFull().Child(a.pendingInstructionList(cx), box)
}

func (a *app) errorBanner(cx *el.Context) el.Element {
	if a.errorText == "" {
		return el.Div().Hidden(true)
	}
	return el.Div().Row().Items(el.Center).Gap(8).P(10).Rounded(8).Bg(theme.Subtle).Child(kit.Icon(kit.IconWarning).Color(theme.DangerText).Size(16).Render(cx), el.Text(a.errorText).TextSize(12).TextColor(theme.DangerText).Grow(), action(cx, "dismiss-error", "", kit.IconClose, func() { a.errorText = "" }))
}

func (a *app) message(cx *el.Context, index int) el.Element {
	rows := a.displayedConversationRows()
	if index >= len(rows) {
		return el.Div()
	}
	e := rows[index]
	if len(e.columns) > 0 {
		row := el.Div().WFull().NoShrink().MaxW(el.Dp(920)).Row().Items(el.Stretch).Gap(16).Py(4)
		for i, cell := range e.columns {
			if i > 0 {
				row.Child(el.Div().W(el.Dp(1)).NoShrink().Child(el.Div().Absolute().Top(-13).Bottom(-13).W(el.Dp(1)).Bg(seamColor)))
			}
			column := el.Div().Grow().W(el.Dp(0))
			if cell != nil {
				column.Child(a.messageEntry(cx, cell, -1))
			}
			row.Child(column)
		}
		return el.Div().NoShrink().Items(el.Center).Child(row)
	}
	return a.messageEntry(cx, e, index)
}

func (a *app) messageEntry(cx *el.Context, e *entry, index int) el.Element {
	if len(e.stack) > 0 {
		column := el.Div().WFull().NoShrink().Gap(4)
		for _, child := range e.stack {
			column.Child(a.messageEntry(cx, child, -1))
		}
		return column
	}
	rows := a.displayedConversationRows()
	content := el.Div().WFull().NoShrink().MaxW(el.Dp(920)).Py(4)
	if index == 0 {
		content.Pt(19)
	}
	if index == len(rows)-1 {
		content.Pb(20)
	}
	if e.stepLabel != "" {
		content.Child(a.workflowNodeHeading(cx, e, false))
	}
	if e.kind == kindTimelineHeader {
		content.Px(14)
		return content
	}
	if e.kind == kindTransition {
		content.Px(14).Child(a.workflowTransitionCard(cx, e))
	} else if e.kind == agentrun.KindQueueStatus {
		content.Px(14).Child(a.queueStatus(cx, e))
	} else if len(e.process) > 0 {
		content.Px(14)
		if index > 0 && rows[index-1].user {
			content.Pt(26)
		}
		content.Child(a.processGroup(cx, e))
	} else if e.user {
		// Align the bubble with the composer's edge, clear of the scrollbar.
		content.Px(4)
		if index > 0 && !rows[index-1].user {
			content.Pt(27)
		}
		width, _ := cx.ViewportSize()
		if !a.sidebarCollapsed(cx) {
			width -= a.sidebarWidth(cx) + 1
		}
		if a.workbench.Visible {
			width -= a.workbench.Width + 1
		}
		maxWidth := min(float32(680), max(float32(160), min(float32(920), width-40)*.72))
		bubble := el.Div().Role("article").Name("用户消息：" + e.text).MaxW(el.Dp(maxWidth)).Px(16).Py(11).Rounded(18).Bg(theme.Subtle)
		if e.recipient != "" {
			bubble.Name("发给 " + e.recipient + "：" + e.text).Gap(8).Child(
				el.Div().Row().Items(el.Center).Gap(6).Child(el.Text("发给").TextSize(10).TextColor(theme.Muted), el.Text("@"+e.recipient).TextSize(12).Medium().TextColor(theme.Info)),
			)
		}
		if e.text != "" {
			bubble.Child(a.userMessageContent(cx, e))
		}
		if len(e.attachments) > 0 {
			bubble.Gap(10).Child(a.conversationAttachments(cx, e.attachments, e.key, false))
		}
		wrapper := el.Div().ID("message/"+e.key).Child(bubble, a.messageActions(cx, e, true))
		content.Items(el.End).Child(wrapper)
	} else if outcome, ok := a.workflowMessageOutcome(e); ok {
		content.Px(14).Child(a.workflowOutcomeCard(cx, e, outcome))
	} else if e.kind == agentrun.KindMessage || e.kind == agentrun.KindError {
		content.Px(14)
		if index > 0 && rows[index-1].user {
			content.Pt(26)
		}
		if a.conversation.messages == nil {
			a.conversation.messages = map[string]*kit.MessageView{}
		}
		message := a.conversation.messages[e.key]
		if message == nil {
			message = kit.Message("", nil).Avatar(nil)
			a.conversation.messages[e.key] = message
		}
		message.Content(el.ViewFunc(func(cx *el.Context) el.Element { return a.conversationDocument(cx, e, false) }))
		content.Child(el.Div().ID("message/"+e.key).WFull().Px(2).Child(message.Render(cx), a.messageActions(cx, e, false)))
	} else {
		content.Child(a.requestMessage(cx, e))
	}
	return el.Div().NoShrink().Items(el.Center).Child(content)
}

func (a *app) requestMessage(cx *el.Context, e *entry) el.Element {
	content := el.Div().WFull().P(14).Gap(10)
	if e.permission != nil {
		content.Child(el.Text("需要你的许可").Medium(), el.Text(e.permission.ToolName+"  "+e.text).TextSize(13), el.Text(e.permission.Description).TextSize(12).TextColor(theme.Muted))
		if len(e.permission.Input) > 0 {
			data, _ := json.MarshalIndent(e.permission.Input, "", "  ")
			content.Child(el.Text(string(data)).Mono().TextSize(12).TextColor(theme.Muted))
		}
		if a.detail != nil && a.detail.Active {
			content.Child(el.Div().Row().Gap(8).Child(button("允许一次", func() { a.respondPermission(e, "allow_once") }, kit.ButtonPrimary).Render(cx), button("拒绝", func() { a.respondPermission(e, "deny") }, kit.ButtonSecondary).Render(cx)))
		}
	} else if e.question != nil {
		content.Child(el.Text("需要你补充信息").Medium())
		for _, q := range e.question.Questions {
			key := requestKey(e.step, e.question.ID) + "/" + q.ID
			content.Child(el.Text(q.Question).TextSize(14))
			options := el.Div().Row().Wrap().Gap(6)
			for _, option := range q.Options {
				options.Child(button(option.Label, func() { a.answers[key] = option.Label }, kit.ButtonSecondary).Selected(a.answers[key] == option.Label).Render(cx))
			}
			answer := a.answers[key]
			content.Child(options, field("answer/"+key, "你的回答", &answer).OnChange(func(value string) { a.answers[key] = value }))
		}
		if a.detail != nil && a.detail.Active {
			content.Child(el.Div().Row().Gap(8).Child(button("提交回答", func() { a.respondQuestion(e, false) }, kit.ButtonPrimary).Render(cx), button("跳过", func() { a.respondQuestion(e, true) }, kit.ButtonGhost).Render(cx)))
		}
	}
	return content
}

func runStatus(status string) string {
	switch status {
	case "running":
		return "正在执行"
	case "completed":
		return "已完成"
	case "paused":
		return "已暂停"
	case "failed":
		return "执行失败"
	case "cancelled":
		return "已取消"
	case "queued":
		return "等待执行"
	default:
		return status
	}
}

func mixColor(foreground, background color.NRGBA, amount float32) color.NRGBA {
	mix := func(a, b uint8) uint8 { return uint8(float32(a)*amount + float32(b)*(1-amount)) }
	return color.NRGBA{R: mix(foreground.R, background.R), G: mix(foreground.G, background.G), B: mix(foreground.B, background.B), A: 255}
}

// conversationScroll is the message list with the Wails client's jump
// control: a round arrow centred 12dp above the composer while the reader is
// scrolled away from the newest message.
func (a *app) conversationScroll(cx *el.Context) el.Element {
	area := el.Div().Grow().H(el.Dp(0)).Items(el.Stretch).Child(a.scroller.Render(cx))
	a.conversation.selection.Decorate(cx, area)
	if a.historyLoading[a.runID] {
		area.Child(el.Div().Absolute().Top(8).Left(0).Right(0).Row().Justify(el.Center).Child(el.Text("正在加载更早的记录…").TextSize(12).TextColor(theme.Muted).Bg(theme.Surface).Rounded(8).Px(12).Py(6)))
	}
	if a.detail == nil || !a.scroller.IsScrolledUp(cx) {
		return area
	}
	jump := el.Div().ID("jump-latest").Role("button").Name("回到最新").Focusable(true).Size(el.Dp(36)).Rounded(18).Center().
		Bg(theme.Surface).Border(1, seamColor).Shadow(theme.ElevationSm).CursorPointer().OnClick(a.scroller.ScrollToEnd).
		Hover(func(s *el.Style) { s.Bg(theme.Subtle) }).FocusStyle(neutralFocus).Child(lucide(cx, "arrow-down", 20, theme.Text))
	return area.Child(el.Div().Absolute().Left(0).Right(0).Bottom(12).Row().Justify(el.Center).Child(jump))
}
