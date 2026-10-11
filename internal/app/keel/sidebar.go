package keeldesktop

import (
	"fmt"
	"image/color"
	"runtime"
	"slices"
	"strings"
	"time"

	"gioui.org/font"
	"gioui.org/io/pointer"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/theme"
	domaintasks "github.com/openmodu/onecatch/internal/domain/tasks"
)

// sectionTitleWeight is shared by the Sidebar's section headings and the
// 项目管理 heading in its Header, which mirrors them.
const sectionTitleWeight = font.SemiBold

const (
	mainTitleBarHeight     float32 = 52
	macSidebarToggleLeft   float32 = 94
	sidebarPeekInset       float32 = 8
	sidebarHeaderRowHeight float32 = 36
)

// Sidebar owns navigation, expansion, scrolling, keyboard handling and context
// menus. OneCatch supplies the model and calls its existing application actions.
func (a *app) sidebar(cx *el.Context) el.Element {
	_, height := cx.ViewportSize()
	return a.sidebarPanel(cx, height, false)
}

func (a *app) sidebarPanel(cx *el.Context, height float32, peek bool) el.Element {
	if a.ui.SidebarExpanded == nil {
		a.ui.SidebarExpanded = map[string]bool{}
	}
	if a.ui.SidebarNav != nil {
		for _, w := range a.workspaces {
			id := "project/" + w.ID
			if on := a.ui.SidebarNav.Expanded(id); on != a.ui.SidebarExpanded[id] {
				a.ui.Expanded[w.ID] = on
			}
		}
	}
	type section struct {
		title  string
		action el.View
		items  []kit.SidebarItem
	}
	var sections []section
	var links []kit.SidebarItem
	for _, item := range sidebarItems {
		if slices.Contains(a.ui.Preferences.Navigation, item.ID) {
			links = append(links, kit.SidebarItem{ID: item.ID, Label: uiText(item.Label), Icon: item.Icon})
		}
	}
	links = append(links, kit.SidebarItem{ID: "more", Label: uiText("更多"), Icon: kit.IconChevronDown, ContextMenu: a.ui.More})
	sections = append(sections, section{items: links})
	var pinned, projects []kit.SidebarItem
	var iconModel strings.Builder
	query := strings.ToLower(strings.TrimSpace(a.query))
	for _, w := range a.workspaces {
		if w.Hidden {
			continue
		}
		menu := a.projectMenu(w)
		project := kit.SidebarItem{ID: "project/" + w.ID, Label: w.Name, Icon: kit.IconFolder, ContextMenu: menu, SuffixOnHover: true,
			Suffix: el.ViewFunc(func(cx *el.Context) el.Element {
				return el.Div().Row().Items(el.Center).Child(
					iconAction(cx, "new-task/"+w.ID, "新建任务", "plus", func() { a.newTask(w.ID) }, false),
					iconAction(cx, "project-actions/"+w.ID, "项目操作", "ellipsis", menu.Toggle, menu.Value()))
			})}
		if w.RemoteFS != nil {
			project.Tag = kit.Tag("远端").Tone(kit.ToneInfo).Size(20)
		}
		// The model fingerprint reads labels, not tags.
		fmt.Fprintf(&iconModel, "%q/%v;", w.ID, w.RemoteFS != nil)
		var tasks []domaintasks.Task
		for _, task := range a.tasks[w.ID] {
			fmt.Fprintf(&iconModel, "%q/%q/%q;", task.ID, task.Harness, task.WorkflowID)
			if task.Pinned {
				pinned = append(pinned, a.sidebarTask(task))
				continue
			}
			if query != "" && !strings.Contains(strings.ToLower(task.Title+" "+task.Prompt+" "+w.Name), query) || a.ui.Filter != "" && string(task.Status) != a.ui.Filter {
				continue
			}
			tasks = append(tasks, task)
		}
		visible := len(tasks)
		if !a.ui.ShowAll[w.ID] && query == "" && a.ui.Filter == "" {
			visible = min(3, visible)
		}
		for _, task := range tasks[:visible] {
			project.Children = append(project.Children, a.sidebarTask(task))
		}
		if len(tasks) > 3 && query == "" && a.ui.Filter == "" {
			label := "展开显示"
			if a.ui.ShowAll[w.ID] {
				label = "收起"
			}
			project.Children = append(project.Children, kit.SidebarItem{ID: "show-all/" + w.ID, Label: label, Icon: kit.IconChevronDown})
		}
		projects = append(projects, project)
	}
	if len(pinned) > 0 {
		sections = append(sections, section{title: "置顶", items: pinned})
	}
	addProject := el.ViewFunc(func(cx *el.Context) el.Element {
		return iconAction(cx, "add-project", "新建项目", "plus", a.newProject, false)
	})
	sections = append(sections, section{title: "项目", action: addProject, items: projects})
	// Keep the component pointer stable so el retains scroll/focus identity.
	// Rebuild its data only when destinations change; normal frames retain its
	// selection/expansion state.
	var model strings.Builder
	model.WriteString(iconModel.String())
	visibleIDs := map[string]bool{}
	var fingerprint func([]kit.SidebarItem)
	fingerprint = func(items []kit.SidebarItem) {
		for _, item := range items {
			visibleIDs[item.ID] = true
			fmt.Fprintf(&model, "%q/%q/%v;", item.ID, item.Label, item.Icon)
			fingerprint(item.Children)
		}
	}
	for _, section := range sections {
		fmt.Fprintf(&model, "%q", section.title)
		fingerprint(section.items)
	}
	a.ui.SidebarVisibleTasks = nil
	var collectVisibleTasks func([]kit.SidebarItem)
	collectVisibleTasks = func(items []kit.SidebarItem) {
		for _, item := range items {
			if strings.HasPrefix(item.ID, "task/") {
				id := strings.TrimPrefix(item.ID, "task/")
				a.ui.SidebarVisibleTasks = append(a.ui.SidebarVisibleTasks, id)
			}
			if !strings.HasPrefix(item.ID, "project/") || a.ui.Expanded[strings.TrimPrefix(item.ID, "project/")] || query != "" || a.ui.Filter != "" {
				collectVisibleTasks(item.Children)
			}
		}
	}
	for _, section := range sections {
		collectVisibleTasks(section.items)
	}
	if a.ui.SidebarNav == nil {
		a.ui.SidebarNav = kit.Sidebar()
	}
	if a.ui.SidebarModel != model.String() {
		for id, menu := range a.project.Menus {
			if !visibleIDs["project/"+id] {
				menu.SetValue(false)
			}
		}
		for id, menu := range a.ui.TaskMenus {
			if !visibleIDs["task/"+id] {
				menu.SetValue(false)
			}
		}
		next := kit.Sidebar().BorderWidth(0).Collapsible(false).SectionTitleWeight(sectionTitleWeight).OnChange(a.sidebarSelect)
		for _, section := range sections {
			next.SectionAction(section.title, section.action, section.items...)
		}
		*a.ui.SidebarNav = *next
		a.ui.SidebarModel = model.String()
	}
	nav := a.ui.SidebarNav
	selected := a.page
	if a.page == "" && a.taskID != "" {
		selected = "task/" + a.taskID
	}
	if nav.Value() != selected {
		nav.SetValue(selected)
	}
	for _, w := range a.workspaces {
		id := "project/" + w.ID
		on := a.ui.Expanded[w.ID] || query != "" || a.ui.Filter != ""
		nav.SetExpanded(id, on)
		a.ui.SidebarExpanded[id] = nav.Expanded(id)
	}
	nav.Width(a.sidebarWidth(cx)).Height(height).Header(el.ViewFunc(func(cx *el.Context) el.Element { return a.sidebarPanelHeader(cx, peek) })).Footer(el.ViewFunc(func(cx *el.Context) el.Element { return fullMenu(cx, a.ui.Footer) }))
	palette := theme.Current()
	palette.Bg = sidebarPanelColor(peek)
	content := cx.Themed(palette, nav)
	a.sidebarTaskDetails(cx)
	return content
}

func (a *app) sidebarSelect(id string) {
	if id != "more" && !strings.HasPrefix(id, "show-all/") && !strings.HasPrefix(id, "project/") {
		a.ui.SidebarPeek = false
		a.ui.SidebarPeekBlocked = true
	}
	switch {
	case id == "more":
		a.ui.More.SetValue(true)
	case strings.HasPrefix(id, "show-all/"):
		workspaceID := strings.TrimPrefix(id, "show-all/")
		a.ui.ShowAll[workspaceID] = !a.ui.ShowAll[workspaceID]
	case strings.HasPrefix(id, "task/"):
		if task, ok := a.cachedTask(strings.TrimPrefix(id, "task/")); ok {
			a.openTask(task)
		}
	case strings.HasPrefix(id, "project/"):
		// An empty project is still navigable; its suffix creates the task.
		a.workspaceID = strings.TrimPrefix(id, "project/")
	default:
		a.goPage(id)
	}
}

func (a *app) sidebarHeader(cx *el.Context) el.Element { return a.sidebarPanelHeader(cx, false) }
func (a *app) sidebarPanelHeader(cx *el.Context, peek bool) el.Element {
	title := el.Div().H(el.Dp(sidebarHeaderRowHeight))
	toolbar := el.Div().Row().Items(el.Center).H(el.Dp(sidebarHeaderRowHeight)).WFull()
	if runtime.GOOS == "darwin" {
		inset := float32(0)
		if peek {
			inset = sidebarPeekInset
		}
		// Keel Sidebar owns its padding; AppKit owns the traffic-light geometry.
		// Keep the toolbar slot's size but position its controls in window space.
		toolbar.Absolute().Left(0).Right(0).Top(mainTitleBarHeight/2 - theme.SpaceLg - inset - sidebarHeaderRowHeight/2).
			Pl(macSidebarToggleLeft - theme.SpaceLg - inset)
	}
	id, label, icon := "hide-sidebar", "收起侧栏", "panel-left"
	if peek {
		id, label, icon = "peek-show-sidebar", "展开侧栏", "panel-left"
	}
	toolbar.Child(iconAction(cx, id, label, icon, a.toggleSidebar, false), dragArea(cx, el.Div().Grow().HFull()))
	title.Child(toolbar)
	// Status filtering sits beside the search it narrows.
	search := el.Div().Row().Items(el.Center).Gap(4).Child(a.sidebarSearch(cx), a.ui.Filters.Render(cx))
	// Match Sidebar's 10dp section-heading inset inside its outer padding.
	heading := el.Div().Row().Items(el.Center).H(el.Dp(28)).Pl(10).Child(el.Text(uiText("项目管理")).TextSize(theme.TextSm).TextColor(theme.Muted).Weight(sectionTitleWeight))
	return el.Div().Gap(8).Child(title, search, heading)
}

// sidebarSearch opens the shared search palette without filtering the sidebar.
func (a *app) sidebarSearch(cx *el.Context) el.Element {
	return el.Div().ID("task-search-box").Name("搜索任务").Role("button").Focusable(true).
		Row().Items(el.Center).Gap(8).Grow().W(el.Dp(0)).H(el.Dp(32)).Px(10).Rounded(8).Bg(theme.Subtle).
		CursorPointer().OnClick(a.focusTaskSearch).Hover(func(s *el.Style) { s.Bg(theme.SubtleHover) }).
		Child(lucide(cx, "search", 16, theme.Muted), el.Text(uiText("搜索")).TextColor(theme.Muted).Grow())
}

func (a *app) sidebarTask(t domaintasks.Task) kit.SidebarItem {
	menu := a.taskMenu(t)
	item := kit.SidebarItem{ID: "task/" + t.ID, Label: t.Title, ContextMenu: menu, SuffixOnHover: true,
		Suffix: el.ViewFunc(func(cx *el.Context) el.Element {
			return a.sidebarTaskActions(cx, t.ID)
		})}
	if t.WorkflowID == "single_agent" || t.WorkflowID == "" && t.Harness != "" {
		item.IconView = el.ViewFunc(func(cx *el.Context) el.Element {
			return el.Div().Role("image").Name("Agent 图标：" + t.Harness).Size(el.Dp(16)).Center().Child(runtimeMark(cx, t.Harness, 16))
		})
	} else {
		item.Icon = kit.IconArchive
	}
	return item
}

// Menu triggers outside Sidebar also use the standard Keel button behavior.
func (a *app) nav(cx *el.Context, id, text string, icon kit.IconName, selected bool, callback func()) el.Element {
	return button(text, callback, kit.ButtonGhost).ID(id).Icon(icon).Selected(selected).
		Render(cx).(*el.DivEl).WFull().Justify(el.Start).FocusStyle(neutralFocus)
}
func runtimeMark(cx *el.Context, id string, size float32) el.Element {
	if id == "modu" || id == "modu-code" {
		return rasterIcon(cx, "runtimeicons/modu-code.png", size, theme.Text)
	}
	if id == "trae" {
		return rasterIcon(cx, "runtimeicons/trae.png", size)
	}
	names := map[string]string{"codex": "codex.svg", "claude": "claude-code.svg", "pi": "pi.svg", "grok": "grok.svg", "dsh": "deepseek.svg"}
	if name, ok := names[id]; ok {
		return assetIcon(cx, "runtimeicons/"+name, size, theme.Text, id != "pi" && id != "grok")
	}
	return lucide(cx, "bot", size, theme.Muted)
}

// Keep enough space for the composer without changing the user's saved sidebar width.
func (a *app) sidebarAvailableWidth(cx *el.Context) float32 {
	w, _ := cx.ViewportSize()
	if a.page == "" && (a.workbench.Visible || a.terminal.Open) && !a.workbench.Maximized && !a.terminal.Maximized {
		w -= min(a.workbench.Width, max(float32(320), w-680)) + 1
	}
	return w
}
func (a *app) sidebarCollapsed(cx *el.Context) bool {
	return a.ui.Preferences.Collapsed || a.page == "" && a.sidebarAvailableWidth(cx) < 861
}
func (a *app) sidebarWidth(cx *el.Context) float32 {
	if a.sidebarCollapsed(cx) {
		return min(a.ui.Preferences.SidebarWidth, 280)
	}
	return min(a.ui.Preferences.SidebarWidth, max(180, a.sidebarAvailableWidth(cx)-681))
}
func (a *app) toggleSidebar() {
	a.ui.Preferences.Collapsed = !a.ui.Preferences.Collapsed
	a.ui.SidebarPeek = false
	a.ui.SidebarPeekBlocked = a.ui.Preferences.Collapsed
	a.saveUIPreferences()
}
func (a *app) sidebarSeam(cx *el.Context) el.Element {
	// Keep one visible pixel; a wider overlapping hit area makes it draggable.
	return el.Div().W(el.Dp(1)).HFull().Bg(seamColor).NoShrink().Child(el.Div().Absolute().Left(-4).Top(0).Bottom(0).W(el.Dp(9)).ID("sidebar-resize").Name("调整侧栏宽度").Role("separator").Focusable(true).Cursor(pointer.CursorColResize).OnDrag(func(e el.DragEvent) {
		switch e.Kind {
		case el.DragStart:
			a.ui.SidebarDragOffset = e.X
		case el.DragMove:
			a.ui.Preferences.SidebarWidth = max(180, min(480, a.ui.Preferences.SidebarWidth+e.X-a.ui.SidebarDragOffset))
		case el.DragEnd:
			a.saveUIPreferences()
		}
	}).OnKey(func(e el.KeyEvent) bool {
		if e.State != el.KeyPress {
			return false
		}
		width := a.ui.Preferences.SidebarWidth
		switch e.Name {
		case "←":
			width -= 12
		case "→":
			width += 12
		case "Home":
			width = 180
		case "End":
			width = 480
		default:
			return false
		}
		a.ui.Preferences.SidebarWidth = max(180, min(480, width))
		a.saveUIPreferences()
		return true
	}).FocusStyle(neutralFocus))
}
func (a *app) executorPicker(cx *el.Context) el.Element {
	if a.task != nil {
		label := a.task.Harness
		for _, r := range a.runtimes {
			if r.ID == a.task.Harness {
				label = r.Name
			}
		}
		return el.Div().Row().Items(el.Center).Gap(7).H(el.Dp(32)).Px(10).Rounded(16).Bg(theme.Subtle).Child(runtimeMark(cx, a.task.Harness, 14), el.Text(a.conversationLabel(label)).TextSize(12).MaxLines(1))
	}
	if a.ui.Executor == nil {
		a.buildExecutorMenu()
	}
	return a.ui.Executor.Render(cx)
}
func (a *app) buildExecutorMenu() {
	menu := kit.Popover(el.ViewFunc(a.executorOptions)).Width(236).Placement(el.Top, el.Start).
		PanelStyle(func(panel *el.DivEl) { panel.P(8).Rounded(12).Name("选择 Agent 与工作流") })
	menu.Trigger(el.ViewFunc(func(cx *el.Context) el.Element {
		label, id := "选择 Agent", a.runtime.Value()
		direct := a.workflow.Value() == "single_agent"
		if direct {
			for _, r := range a.runtimes {
				if r.ID == id {
					label = r.Name
				}
			}
		} else {
			for _, d := range a.definitions {
				if d.ID == a.workflow.Value() {
					label = d.Name
				}
			}
		}
		if label == "选择 Agent" && id != "" {
			label = id
		}
		if direct {
			label = a.conversationLabel(label)
		}
		row := el.Div().ID("executor").Name(label).Role("button").Focusable(true).Row().Items(el.Center).Gap(7).H(el.Dp(32)).Px(10).Rounded(16).Bg(theme.Subtle).CursorPointer().OnClick(func() {
			menu.Toggle()
		}).Hover(func(s *el.Style) { s.Bg(theme.Highlight) }).FocusStyle(neutralFocus)
		if direct {
			row.Child(runtimeMark(cx, id, 14))
		} else {
			row.Child(lucide(cx, "workflow", 14, theme.Muted))
		}
		return row.Child(el.Text(label).TextSize(12).MaxLines(1), lucide(cx, "chevron-down", 12, theme.Muted))
	}))
	a.ui.Executor = menu
}

func (a *app) executorOptions(cx *el.Context) el.Element {
	_, height := cx.ViewportSize()
	column := el.Div().ID("executor-options").Gap(2).MaxH(el.Dp(min(float32(420), max(float32(120), height-120)))).ScrollY().Scrollbars(el.ScrollbarAlways)
	if _, viewport, content := cx.ScrollState("executor-options"); viewport > 0 && content > viewport {
		column.Pr(12)
	}
	column.Child(el.Text(uiText("Agent")).TextSize(11).TextColor(theme.Muted).Px(8).Py(4))
	for _, runtime := range a.runtimes {
		r := runtime
		column.Child(listOption(cx, "executor-runtime/"+r.ID, r.Name, runtimeMark(cx, r.ID, 16), func() { a.selectLocalConversation(r.ID, "single_agent") },
			!a.remoteConversation() && a.workflow.Value() == "single_agent" && a.runtime.Value() == r.ID,
			a.task != nil || a.busy || a.loading || !r.Available || !r.Enabled))
	}
	if a.settings.Experimental.RemoteWorkersEnabled && a.workspaceForID(a.workspaceID).RemoteFS == nil {
		for _, info := range a.workers {
			w := info
			for _, runtime := range a.runtimes {
				r := runtime
				if !r.Enabled || !r.Available || !workerSupportsHarness(w, r.ID) {
					continue
				}
				label := r.Name + " · " + workerLabel(w)
				if w.Provider == "volcengine-sandbox" {
					label = workerLabel(w)
				}
				column.Child(listOption(cx, "executor-worker/"+w.ID+"/"+r.ID, label, runtimeMark(cx, r.ID, 16), func() { a.selectConversationWorker(w.ID, r.ID) },
					a.remoteConversation() && a.workerID == w.ID && a.runtime.Value() == r.ID,
					a.task != nil || a.busy || a.loading || len(a.attachments.Items) > 0 || a.attachments.Loading || a.attachments.Picking))
			}
		}
	}
	if a.task == nil {
		column.Child(el.Div().H(el.Dp(1)).WFull().Bg(seamColor).My(6), el.Text(uiText("工作流")).TextSize(11).TextColor(theme.Muted).Px(8))
		for _, definition := range a.definitions {
			if definition.ID != "single_agent" {
				d := definition
				column.Child(listOption(cx, "executor-workflow/"+d.ID, d.Name, lucide(cx, "workflow", 16, theme.Muted), func() { a.selectLocalConversation(a.runtime.Value(), d.ID) }, a.workflow.Value() == d.ID, a.busy || a.loading))
			}
		}
	}
	return column
}

func (a *app) focusTaskSearch() {
	a.openSearchPalette()
}

// The peek uses the same Sidebar instance, model and menus as the docked view.
// It is temporary: only a click on the toggle changes the stored preference.
func (a *app) sidebarPeek(cx *el.Context) el.Element {
	left := float32(24)
	if runtime.GOOS == "darwin" {
		left = macSidebarToggleLeft
	}
	probe := el.Div().ID("sidebar-peek-trigger").Absolute().Left(left).Top((mainTitleBarHeight - 28) / 2).Size(el.Dp(28))
	position := el.Div().ID("sidebar-peek-position").Absolute().Left(sidebarPeekInset).Top(sidebarPeekInset - 1).Size(el.Dp(1))
	trigger := el.Div().Absolute().Left(0).Top(0).Child(probe, position)
	if !a.sidebarCollapsed(cx) {
		a.ui.SidebarPeek = false
		return trigger
	}
	hovered := cx.Hovered("sidebar-peek-trigger") || cx.Hovered("show-sidebar") || cx.Hovered("peek-show-sidebar")
	if !hovered && !cx.Hovered("sidebar-peek-panel") {
		a.ui.SidebarPeekBlocked = false
	}
	if (hovered || cx.FocusVisible("show-sidebar")) && !a.ui.SidebarPeekBlocked {
		if !a.ui.SidebarPeek {
			a.ui.SidebarPeekKeyboard = !hovered && cx.FocusVisible("show-sidebar")
		}
		a.ui.SidebarPeek = true
	}
	if !a.ui.SidebarPeek {
		return trigger
	}
	if cx.Hovered("sidebar-peek-panel") {
		a.ui.SidebarPeekKeyboard = false
	}
	keep := hovered || cx.Hovered("sidebar-peek-panel") || cx.FocusVisible("show-sidebar") || a.sidebarMenuOpen() || (a.ui.SidebarPeekKeyboard && cx.FocusWithin("sidebar-peek-panel"))
	if !keep {
		cx.AfterEnabled("sidebar-peek-panel", "sidebar-peek-hide", 120*time.Millisecond, func() { a.ui.SidebarPeek = false })
	}
	_, height := cx.ViewportSize()
	panelHeight := min(float32(560), max(float32(80), height-16))
	panel := el.Div().ID("sidebar-peek-panel").Role("navigation").Name("侧栏预览").W(el.Dp(a.sidebarWidth(cx))).H(el.Dp(panelHeight)).Bg(sidebarPanelColor(true)).Rounded(16).Shadow(theme.ElevationMd)
	dismiss := func() { a.ui.SidebarPeek = false; a.ui.SidebarPeekBlocked = true }
	cx.Overlay("sidebar-peek", el.Anchored("sidebar-peek-position", panel).Placement(el.Bottom, el.Start).Offset(0).OnDismiss(dismiss))
	// Declare the parent before rendering nested Sidebar menus and previews.
	panel.Child(a.sidebarPanel(cx, panelHeight, true))
	return trigger
}

func (a *app) sidebarMenuOpen() bool {
	for _, menu := range []*kit.MenuView{a.ui.More, a.ui.Footer, a.ui.Filters} {
		if menu != nil && menu.Value() {
			return true
		}
	}
	for _, menu := range a.project.Menus {
		if menu.Value() {
			return true
		}
	}
	for _, menu := range a.ui.TaskMenus {
		if menu.Value() {
			return true
		}
	}
	return false
}

// A temporary pane is an elevated surface; the docked pane sits between the
// page and controls. Keel still owns the navigation rows and their interaction.
func sidebarPanelColor(peek bool) color.NRGBA {
	if peek {
		return theme.Surface
	}
	return sidebarColor
}
