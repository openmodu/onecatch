package keeldesktop

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"

	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/theme"
	"github.com/dyike/keel/ui/window"
	domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"
)

type navigationItem struct {
	ID, Label string
	Icon      kit.IconName
}

var sidebarItems = []navigationItem{{"templates", "模板", kit.IconInbox}, {"skills", "Skills", kit.IconUser}, {"usage", "用量", kit.IconReceipt}, {"workflows", "工作流", kit.IconArchive}}

type uiPreferences struct {
	Version           int      `json:"version"`
	Navigation        []string `json:"navigation"`
	SidebarWidth      float32  `json:"sidebarWidth"`
	Collapsed         bool     `json:"collapsed"`
	Theme             string   `json:"theme"`
	Accent            string   `json:"accent"`
	Language          string   `json:"language,omitempty"`
	ChatFontSize      string   `json:"chatFontSize,omitempty"`
	HideTableDividers bool     `json:"hideTableDividers,omitempty"`
}

type location struct {
	Page, Workspace, Task, Workflow string
	Source                          bool
	Editor                          string
	Raw                             bool
	Advanced                        bool
}
type navigationHistory struct {
	Entries []location
	Index   int
}

func (h *navigationHistory) push(before, after location) {
	if before == after {
		return
	}
	if len(h.Entries) == 0 {
		h.Entries = []location{before}
		h.Index = 0
	}
	// Snapshot edits made since arriving at the current history entry.
	h.Entries[h.Index] = before
	h.Entries = append(h.Entries[:h.Index+1], after)
	h.Index = len(h.Entries) - 1
}
func (h *navigationHistory) move(delta int) (location, bool) {
	next := h.Index + delta
	if next < 0 || next >= len(h.Entries) {
		return location{}, false
	}
	h.Index = next
	return h.Entries[next], true
}

type uiState struct {
	Preferences                     uiPreferences
	PreferencesError                error
	History                         navigationHistory
	Restoring                       bool
	SearchPalette                   *kit.CommandView
	SearchCategory                  string
	Filter                          string
	Expanded, ShowAll               map[string]bool
	More, Footer, Filters           *kit.MenuView
	HeaderMore                      *kit.MenuView
	SidebarNav                      *kit.SidebarView
	SidebarPeek, SidebarPeekBlocked bool
	SidebarPeekKeyboard             bool
	SidebarModel                    string
	SidebarExpanded                 map[string]bool
	SidebarVisibleTasks             []string
	SidebarPreview                  sidebarTaskPreview
	Customize                       *kit.DialogView
	Executor                        *kit.PopoverView
	Add                             *kit.MenuView
	WorkflowSource                  bool
	SelectedWorkflow                string
	WorkflowWindow                  *window.Window
	WorkflowHistory                 navigationHistory
	WorkflowConfirm                 *kit.DialogView
	WorkflowRaw                     bool
	WorkflowAdvanced                bool
	WorkflowRoles                   bool
	WorkflowDraft                   domainworkflows.Definition
	DAG                             dagState
	WorkflowStep                    int
	WorkflowRowMenus                map[string]*kit.MenuView
	WorkflowNew                     *kit.MenuView
	WorkflowSelects                 map[string]*kit.SelectView
	WorkflowNumbers                 map[string]*kit.NumberInputView
	WorkflowStepID                  string
	WorkflowSignal                  string
	WorkflowPolicy                  map[string]string
	SidebarDragOffset               float32
	TaskMenus                       map[string]*kit.MenuView
	TaskAnalyzing                   map[string]bool
	TaskMenuPinned                  map[string]bool // pinned state each task menu was built for
	TaskDialog                      *kit.DialogView
	TaskRenameID, TaskTitle         string
	TaskSaving                      bool
}

func defaultUIPreferences() uiPreferences {
	return uiPreferences{Version: 1, Navigation: []string{"templates", "skills"}, SidebarWidth: 244, Theme: "system", Accent: "forest"}
}
func normalizeUIPreferences(p uiPreferences) uiPreferences {
	p.Version = 1
	ids := make([]string, 0, len(sidebarItems))
	for _, item := range sidebarItems {
		if slices.Contains(p.Navigation, item.ID) {
			ids = append(ids, item.ID)
		}
	}
	p.Navigation = ids
	p.SidebarWidth = max(180, min(480, p.SidebarWidth))
	if p.Theme != "light" && p.Theme != "dark" {
		p.Theme = "system"
	}
	if !slices.Contains([]string{"forest", "ocean", "violet", "amber"}, p.Accent) {
		p.Accent = "forest"
	}
	if p.Language != "en" {
		p.Language = "zh-CN"
	}
	if !slices.Contains([]string{"small", "standard", "large", "extra-large"}, p.ChatFontSize) {
		p.ChatFontSize = "standard"
	}
	return p
}
func loadUIPreferences(root string) (uiPreferences, error) {
	p := defaultUIPreferences()
	data, err := os.ReadFile(filepath.Join(root, "keel-ui.json"))
	if errors.Is(err, os.ErrNotExist) {
		return p, nil
	}
	if err != nil {
		return p, err
	}
	if err := json.Unmarshal(data, &p); err != nil {
		return defaultUIPreferences(), err
	}
	return normalizeUIPreferences(p), nil
}
func writeUIPreferences(root string, p uiPreferences) error {
	data, err := json.MarshalIndent(normalizeUIPreferences(p), "", "  ")
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(root, ".keel-ui-*")
	if err != nil {
		return err
	}
	path := temp.Name()
	defer os.Remove(path)
	if err = temp.Chmod(0600); err == nil {
		_, err = temp.Write(data)
	}
	if err == nil {
		err = temp.Sync()
	}
	closeErr := temp.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(path, filepath.Join(root, "keel-ui.json"))
}
func (a *app) saveUIPreferences() {
	if a.ui.PreferencesError != nil {
		a.fail(a.ui.PreferencesError)
		return
	}
	a.fail(writeUIPreferences(a.backend.service.DataRoot(), a.ui.Preferences))
}
func (a *app) initNavigation() {
	p, err := loadUIPreferences(a.backend.service.DataRoot())
	a.ui = uiState{Preferences: p, PreferencesError: err, Expanded: map[string]bool{}, ShowAll: map[string]bool{}, WorkflowRowMenus: map[string]*kit.MenuView{}}
	a.ui.Customize = kit.Dialog("编辑侧栏").Width(360).CloseButton(true).Body(el.ViewFunc(a.customizeSidebar)).Footer(button("完成", func() { a.ui.Customize.SetValue(false) }, kit.ButtonPrimary))
	a.ui.Filters = kit.Menu().Width(176).Placement(el.Right, el.Start)
	for _, status := range []string{"", "queued", "running", "paused", "completed", "failed", "cancelled"} {
		value := status
		label := runStatus(value)
		if value == "" {
			label = "全部"
		}
		a.ui.Filters.CheckItem(label, "", value == "", func(bool) {
			a.ui.Filter = value
			for _, s := range []string{"", "queued", "running", "paused", "completed", "failed", "cancelled"} {
				l := runStatus(s)
				if s == "" {
					l = "全部"
				}
				a.ui.Filters.SetItemChecked(l, s == value)
			}
		})
	}
	a.ui.Filters.Trigger(el.ViewFunc(func(cx *el.Context) el.Element {
		return iconAction(cx, "task-filter", "筛选任务状态", "sliders-horizontal", a.ui.Filters.Toggle, a.ui.Filter != "")
	}))
	a.ui.WorkflowConfirm = kit.Dialog("").Width(420)
	a.ui.TaskMenus = map[string]*kit.MenuView{}
	a.ui.TaskMenuPinned = map[string]bool{}
	a.ui.TaskDialog = kit.Dialog("重命名任务").Width(360).CloseButton(true).Body(el.ViewFunc(func(cx *el.Context) el.Element {
		return el.Div().Gap(12).Child(field("task-title", "任务标题", &a.ui.TaskTitle).WFull().ReadOnly(a.ui.TaskSaving), a.errorBanner(cx))
	})).Footer(button("取消", func() { a.ui.TaskDialog.SetValue(false) }, kit.ButtonGhost), button("保存", a.renameTask, kit.ButtonPrimary))
	a.buildMoreMenu()
	a.buildFooterMenu()
	a.ui.Add = kit.Menu().Width(176).CheckSide(el.Right).Placement(el.Top, el.Start)
	a.ui.Add.ContentItem("添加附件…", "", menuIcon("paperclip", "添加附件…"), a.chooseAttachments).Separator().Label("执行方式").CheckItem("立即执行", "", true, func(bool) { a.setQueueMode(false) }).CheckItem("加入队列", "", false, func(bool) { a.setQueueMode(true) })
	a.ui.Add.Trigger(el.ViewFunc(func(cx *el.Context) el.Element {
		return iconAction(cx, "composer-config", "添加与配置", "plus", a.ui.Add.Toggle, a.library.Queue)
	}))

}
func (a *app) currentLocation() location {
	editor := ""
	if a.ui.WorkflowSource {
		editor = a.workflowEditor.Value()
	}
	return location{Page: a.page, Workspace: a.workspaceID, Task: a.taskID, Workflow: a.ui.SelectedWorkflow, Source: a.ui.WorkflowSource, Editor: editor, Raw: a.ui.WorkflowRaw, Advanced: a.ui.WorkflowAdvanced}
}
func (a *app) recordNavigation(before location) {
	if !a.ui.Restoring {
		a.ui.History.push(before, a.currentLocation())
	}
}
func (a *app) goPage(id string) {
	if id == "workflows" && a.window != nil {
		a.openWorkflows()
		return
	}
	before := a.currentLocation()
	a.page, a.errorText = id, ""
	if id == "workflows" && a.ui.SelectedWorkflow == "" {
		for _, d := range a.definitions {
			if d.ID != "single_agent" {
				a.ui.SelectedWorkflow = d.ID
				break
			}
		}
	}
	if id == "skills" {
		a.loadSkills()
	}
	if id == "usage" {
		a.loadUsage(false)
	}
	a.recordNavigation(before)
}
func (a *app) goHistory(delta int) {
	target, ok := a.ui.History.move(delta)
	if !ok {
		return
	}
	a.ui.Restoring = true
	defer func() { a.ui.Restoring = false }()
	if target.Task != "" {
		found := false
		for _, task := range a.tasks[target.Workspace] {
			if task.ID == target.Task {
				a.openTask(task)
				found = true
				break
			}
		}
		if !found {
			a.newTask(target.Workspace)
		}
	} else {
		a.newTask(target.Workspace)
	}
	if target.Page != "" {
		a.goPage(target.Page)
	}
	a.restoreWorkflowLocation(target)
}
func (a *app) selectWorkflow(id string, source bool) {
	before := a.currentLocation()
	a.ui.SelectedWorkflow, a.ui.WorkflowSource = id, source
	if source {
		for _, d := range a.definitions {
			if d.ID == id {
				a.editWorkflow(d)
				break
			}
		}
	}
	a.ui.WorkflowHistory.push(before, a.currentLocation())
}

// menuMinWidth lets icon-and-label menus fit their entries.
const menuMinWidth = 120

func menuIcon(name, label string) el.View {
	return el.ViewFunc(func(cx *el.Context) el.Element {
		return el.Div().Row().Items(el.Center).Gap(10).Child(lucide(cx, name, 16, theme.Muted), el.Text(label).TextSize(13))
	})
}
func (a *app) buildMoreMenu() {
	// A dropdown under the row, inside the sidebar rather than over the page.
	menu := kit.Menu().Width(176).Placement(el.Bottom, el.Start)
	hidden := false
	for _, item := range sidebarItems {
		if !slices.Contains(a.ui.Preferences.Navigation, item.ID) {
			it := item
			hidden = true
			menu.ContentItem(it.Label, "", menuIcon(iconNames[it.Icon], it.Label), func() { a.goPage(it.ID) })
		}
	}
	if hidden {
		menu.Separator()
	}
	menu.ContentItem("编辑侧栏…", "", menuIcon("square-pen", "编辑侧栏…"), func() { a.ui.Customize.SetValue(true) })
	menu.Trigger(el.ViewFunc(func(cx *el.Context) el.Element {
		label := "更多"
		active := false
		for _, item := range sidebarItems {
			if item.ID == a.page && !slices.Contains(a.ui.Preferences.Navigation, item.ID) {
				label = item.Label
				active = true
			}
		}
		return a.nav(cx, "more", label, kit.IconChevronDown, active || menu.Value(), menu.Toggle)
	}))
	a.ui.More = menu
}
func (a *app) buildFooterMenu() {
	mode := kit.Menu().Width(140)
	for _, item := range []struct{ ID, Label string }{{"system", "跟随系统"}, {"light", "浅色"}, {"dark", "深色"}} {
		it := item
		mode.CheckItem(it.Label, "", a.ui.Preferences.Theme == it.ID, func(bool) { a.setAppearance(it.ID, a.ui.Preferences.Accent) })
	}
	menu := kit.Menu().Width(208).Placement(el.Top, el.Start).Offset(6)
	menu.ContentItem("设置", "mod+,", menuIcon("settings", "设置"), a.openSettings).Sub("显示模式", mode).Separator().ContentItem("工作流", "", menuIcon("git-branch", "工作流"), func() { a.goPage("workflows") })
	menu.SetItemContent("显示模式", menuIcon("monitor", "显示模式"))
	menu.Trigger(el.ViewFunc(func(cx *el.Context) el.Element {
		row := el.Div().WFull().ID("app-menu").Role("button").Name("OneCatch 菜单").Focusable(true).Row().Items(el.Center).Gap(8).H(el.Dp(40)).Px(8).Rounded(8).CursorPointer().OnClick(menu.Toggle).Hover(func(s *el.Style) { s.Bg(theme.Highlight) }).FocusStyle(neutralFocus)
		if menu.Value() {
			row.Bg(theme.Highlight)
		}
		return row.Child(a.logo.Render(cx), el.Text("OneCatch").Grow().TextSize(13).Bold(), lucide(cx, "chevron-down", 13, theme.Muted))
	}))
	a.ui.Footer = menu
}
func (a *app) customizeSidebar(cx *el.Context) el.Element {
	root := el.Div().Gap(6).Child(el.Text("选择直接显示在侧栏中的入口。").TextSize(13).TextColor(theme.Muted).Mb(12))
	for _, item := range sidebarItems {
		it := item
		checked := slices.Contains(a.ui.Preferences.Navigation, it.ID)
		row := el.Div().Row().Items(el.Center).Gap(12).H(el.Dp(36)).Px(8).Rounded(8).Role("checkbox").Name(it.Label).Selected(checked).Focusable(true).CursorPointer().OnClick(func() {
			if checked {
				a.ui.Preferences.Navigation = slices.DeleteFunc(a.ui.Preferences.Navigation, func(id string) bool { return id == it.ID })
			} else {
				a.ui.Preferences.Navigation = append(a.ui.Preferences.Navigation, it.ID)
			}
			a.ui.Preferences = normalizeUIPreferences(a.ui.Preferences)
			a.saveUIPreferences()
			a.buildMoreMenu()
		}).Hover(func(s *el.Style) { s.Bg(theme.Highlight) }).FocusStyle(neutralFocus)
		check := el.Div().Size(el.Dp(16)).Rounded(4).Bg(theme.SubtleHover)
		if checked {
			check.Bg(theme.Primary).Center().Child(kit.Icon(kit.IconDone).Size(12).Color(theme.OnColor).Render(cx))
		}
		root.Child(row.Child(check, outlineIcon(cx, it.Icon, 17, theme.Muted), el.Text(it.Label).TextSize(14)))
	}
	return root
}
func (a *app) setAppearance(mode, accent string) {
	a.ui.Preferences.Theme, a.ui.Preferences.Accent = mode, accent
	a.dark = mode == "dark" || mode == "system" && window.SystemAppearance() == window.AppearanceDark
	applyAppPalette(a.dark, accent)
	appearance := window.AppearanceSystem
	if mode == "light" {
		appearance = window.AppearanceLight
	}
	if mode == "dark" {
		appearance = window.AppearanceDark
	}
	window.SetNativeAppearance(appearance)
	a.scroller.Invalidate()
	a.saveUIPreferences()
	a.buildFooterMenu()
	core.Update(func() {})
}

func (a *app) setTableDividers(on bool) {
	a.ui.Preferences.HideTableDividers = !on
	for _, doc := range a.docs {
		doc.TableDividers(on)
	}
	a.scroller.Invalidate()
	a.saveUIPreferences()
	core.Update(func() {})
}
func iconAction(cx *el.Context, id, label, name string, callback func(), selected bool) el.Element {
	return button("", callback, kit.ButtonGhost).ID(id).Name(label).Size(26).Selected(selected).
		Content(el.ViewFunc(func(cx *el.Context) el.Element { return lucide(cx, name, 16, theme.Muted) })).
		Render(cx).(*el.DivEl).Size(el.Dp(26)).P(0).Center().Rounded(6).FocusStyle(neutralFocus)
}
func chip(cx *el.Context, id, label, name string, callback func(), filled bool) el.Element {
	row := el.Div().ID(id).Role("button").Name(label).Focusable(true).Row().Items(el.Center).Gap(6).H(el.Dp(32)).Px(10).Rounded(16).CursorPointer().OnClick(callback).Hover(func(s *el.Style) { s.Bg(theme.Highlight) }).FocusStyle(neutralFocus)
	if filled {
		row.Bg(theme.Subtle)
	}
	if name != "" {
		row.Child(lucide(cx, name, 15, theme.Muted))
	}
	return row.Child(el.Text(label).TextSize(12).MaxLines(1), lucide(cx, "chevron-down", 12, theme.Muted))
}

func (a *app) setQueueMode(queued bool) {
	a.library.Queue = queued
	a.ui.Add.SetItemChecked("立即执行", !queued)
	a.ui.Add.SetItemChecked("加入队列", queued)
}

func fullMenu(cx *el.Context, menu *kit.MenuView) el.Element {
	result := menu.Render(cx)
	if div, ok := result.(*el.DivEl); ok {
		div.WFull()
	}
	return result
}

func (a *app) headerMore(cx *el.Context) el.Element {
	if a.ui.HeaderMore == nil {
		menu := kit.Menu().Width(1).Placement(el.Bottom, el.End)
		menu.ContentItem("Session 状态", "", menuIcon("activity", "Session 状态"), func() { a.toggleInspector("status") }).
			ContentItem("事件时间轴", "", menuIcon("clock", "事件时间轴"), func() { a.toggleInspector("events") })
		menu.Trigger(el.ViewFunc(func(cx *el.Context) el.Element {
			return iconAction(cx, "header-more", "更多操作", "ellipsis-vertical", menu.Toggle, menu.Value())
		}))
		a.ui.HeaderMore = menu
	}
	return a.ui.HeaderMore.Render(cx)
}
