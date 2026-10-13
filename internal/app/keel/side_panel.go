package keeldesktop

import (
	"github.com/dyike/keel/gio/op/clip"
	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/theme"
	"image"
	"image/color"
	"runtime"
)

func (a *app) sidePanelCard(cx *el.Context, content el.Element) el.Element {
	return el.Div().W(el.Dp(a.inspectorWidth(cx))).HFull().NoShrink().P(8).Pl(0).Child(
		el.Div().WFull().HFull().Bg(theme.Surface).Rounded(14).Shadow(theme.ElevationSm).Child(el.Div().WFull().HFull().Decorate(func(gtx core.C, draw func()) {
			defer clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Max}, gtx.Dp(14)).Push(gtx.Ops).Pop()
			draw()
		}).Child(content)))
}

func (a *app) panelHeader(cx *el.Context, title string) *el.DivEl {
	bar := el.Div().Row().Items(el.Center).H(el.Dp(40)).Px(10).Gap(4).NoShrink().Bg(theme.Surface)
	if title != "" {
		bar.Child(el.Text(title).TextSize(13).TextColor(theme.Muted))
	}
	if runtime.GOOS == "darwin" && (a.workbench.Maximized || a.terminal.Maximized) && a.sidebarCollapsed(cx) {
		// The panel starts after the 12dp resize seam and 8dp top inset.
		// Match the main title bar in window coordinates.
		bar.Pl(macSidebarToggleLeft - 12).H(el.Dp(mainTitleBarHeight - 16))
	}
	return bar
}

func (a *app) inspectorPanelHeader(cx *el.Context) *el.DivEl {
	w := &a.workbench
	if w.MoreMenu == nil {
		menu := kit.Menu().Width(1).Placement(el.Bottom, el.End)
		for _, tab := range inspectorTabs {
			if tab.ID != "status" && tab.ID != "events" {
				continue
			}
			tab := tab
			menu.ContentItem(tab.Label, "", menuIcon(tab.Icon, tab.Label), func() { w.Tab = tab.ID; a.loadInspector() })
		}
		menu.Trigger(el.ViewFunc(func(cx *el.Context) el.Element {
			return iconAction(cx, "inspector-more", "面板更多操作", "ellipsis-vertical", menu.Toggle, menu.Value())
		}))
		w.MoreMenu = menu
	}
	title := "Changes"
	if w.Tab == "files" {
		title = "Files"
	} else if w.Tab != "review" {
		for _, tab := range inspectorTabs {
			if tab.ID == w.Tab {
				title = tab.Label
			}
		}
	}
	label, icon := "放大状态栏", "maximize-2"
	if w.Maximized {
		label, icon = "还原状态栏", "minimize-2"
	}
	bar := a.panelHeader(cx, title)
	if w.Tab == "review" {
		return a.reviewPanelHeader(cx, label, icon)

	} else if w.Tab == "files" {
		return a.filesPanelHeader(cx, label, icon)
	} else {
		bar.Child(el.Div().Grow())
	}
	return bar.Child(w.MoreMenu.Render(cx),
		iconAction(cx, "maximize-inspector", label, icon, func() { w.Maximized = !w.Maximized }, false),
		iconAction(cx, "hide-inspector", "收起检查面板", "x", func() { w.Visible = false; w.Maximized = false }, false))
}

func (a *app) terminalPanelHeader(cx *el.Context) *el.DivEl {
	s := &a.terminal
	if s.MoreMenu == nil {
		menu := kit.Menu().Width(1).Placement(el.Bottom, el.End)
		menu.ContentItem("搜索终端", "", menuIcon("search", "搜索终端"), a.openTerminalSearch).
			ContentItem("左右分屏", "", menuIcon("columns-2", "左右分屏"), func() { a.createTerminal("vertical") }).
			ContentItem("上下分屏", "", menuIcon("rows-2", "上下分屏"), func() { a.createTerminal("horizontal") })
		menu.Trigger(el.ViewFunc(func(cx *el.Context) el.Element {
			return iconAction(cx, "terminal-more", "终端更多操作", "ellipsis-vertical", menu.Toggle, menu.Value())
		}))
		s.MoreMenu = menu
	}
	bar := a.panelHeader(cx, "Terminal")
	tabs := el.Div().Row().Items(el.Center).Gap(4).Grow().W(el.Dp(0)).ScrollX().Scrollbars(el.ScrollbarHidden)
	for _, tab := range s.Tabs {
		tab := tab
		ids := tab.Root.panes()
		if len(ids) == 0 || s.Panes[ids[0]] == nil {
			continue
		}
		title := s.Panes[ids[0]].Title
		label := button(title, func() { a.selectTerminalTab(tab) }, kit.ButtonGhost).Size(26).Compact(true).Render(cx).(*el.DivEl)
		label.Hover(func(s *el.Style) { s.Bg(color.NRGBA{}) })
		close := iconAction(cx, "terminal-close/"+tab.ID, "关闭终端标签 "+title, "x", func() { a.closeTerminalTab(tab.ID) }, false).(*el.DivEl)
		close.Size(el.Dp(22)).Rounded(4).Hover(func(s *el.Style) { s.Bg(theme.Highlight) })
		item := el.Div().Row().Items(el.Center).NoShrink().H(el.Dp(28)).Pr(3).Rounded(7).
			Hover(func(s *el.Style) { s.Bg(theme.SubtleHover) }).Child(label, close)
		if tab.ID == s.Active {
			item.Bg(theme.Subtle)
		}
		tabs.Child(item)
	}
	label, icon := "放大终端", "maximize-2"
	if s.Maximized {
		label, icon = "还原终端", "minimize-2"
	}
	return bar.Child(tabs, iconAction(cx, "terminal-new", "新建终端", "plus", func() { a.createTerminal("") }, false),
		s.MoreMenu.Render(cx), iconAction(cx, "terminal-maximize", label, icon, func() { s.Maximized = !s.Maximized }, false),
		iconAction(cx, "terminal-collapse", "关闭所有终端", "x", a.closeTerminalPanel, false))
}
