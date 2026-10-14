package keeldesktop

import (
	"fmt"
	"image/color"

	"github.com/dyike/keel/gio/io/key"
	"github.com/dyike/keel/gio/io/pointer"
	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/theme"
	uiassets "github.com/openmodu/onecatch/internal/app/keel/assets"
)

func (a *app) buildTerminalSplitMenu() {
	m := kit.Menu().Width(180).Placement(el.Bottom, el.End)
	m.ContentItem("左右分屏", "", menuIcon("columns-2", "左右分屏"), func() { a.createTerminal("vertical") })
	m.ContentItem("上下分屏", "", menuIcon("rows-2", "上下分屏"), func() { a.createTerminal("horizontal") })
	m.Separator().Item("关闭当前分屏", "", func() {
		if p := a.activeTerminal(); p != nil {
			a.removeTerminalPane(p.ID)
		}
	})
	m.Trigger(el.ViewFunc(func(cx *el.Context) el.Element {
		return iconAction(cx, "terminal-split", "分屏终端", "columns-2", m.Toggle, m.Value())
	}))
	a.terminal.SplitMenu = m
}
func (a *app) terminalWidth(cx *el.Context) float32 {
	if a.page == "" {
		return a.inspectorWidth(cx) - 8
	}
	w, _ := cx.ViewportSize()
	if !a.sidebarCollapsed(cx) {
		w -= a.sidebarWidth(cx) + 1
	}
	if a.workbench.Visible {
		w -= a.inspectorWidth(cx) + 1
	}
	return w
}
func (a *app) terminalHeight(cx *el.Context) float32 {
	_, h := cx.ViewportSize()
	return max(100, min(a.terminal.Height, min(560, h-172)))
}
func (a *app) terminalDock(cx *el.Context) el.Element {
	s := &a.terminal
	dock := el.Div().WFull().NoShrink().Bg(theme.Bg).ID("terminal-dock")
	if s.Maximized || a.page == "" {
		dock.Grow().H(el.Dp(0))
	} else {
		dock.H(el.Dp(a.terminalHeight(cx)))
	}
	dock.Child(el.Div().H(el.Dp(1)).WFull().NoShrink().Bg(seamColor))
	if !s.Maximized && a.page != "" {
		dock.Child(el.Div().Absolute().Left(0).Right(0).Top(-5).H(el.Dp(10)).ID("terminal-resize").Role("separator").Name("调整终端高度").Focusable(true).Cursor(pointer.CursorRowResize).OnDrag(func(e el.DragEvent) {
			switch e.Kind {
			case el.DragStart:
				s.DragY = e.Y
			case el.DragMove:
				s.Height = max(190, min(560, s.Height+s.DragY-e.Y))
			}
		}).OnKey(func(e el.KeyEvent) bool {
			if e.State != el.KeyPress {
				return false
			}
			switch e.Name {
			case "↑":
				s.Height = min(560, s.Height+12)
			case "↓":
				s.Height = max(190, s.Height-12)
			default:
				return false
			}
			return true
		}))
	}
	bar := el.Div().Row().Items(el.Center).H(el.Dp(36)).Px(8).Gap(6).NoShrink().Bg(mixColor(theme.Subtle, theme.Bg, .34))
	label := el.Div().Row().Items(el.Center).Gap(7).Px(6).H(el.Dp(26)).Rounded(6).ID("terminal-title").Role("button").Name("聚焦终端").Focusable(true).OnClick(func() { a.focusTerminal(s.Focused) }).CursorPointer().Hover(func(st *el.Style) { st.Bg(theme.Highlight) }).Child(uiassets.Lucide(cx, "square-terminal", 14, theme.Muted), el.Text("终端").TextSize(12).Medium())
	dot := theme.Muted
	if tab := a.terminalTab(); tab != nil {
		for _, id := range tab.Root.panes() {
			if p := s.Panes[id]; p != nil && p.Running {
				dot = theme.RGB(0x3a7735)
				break
			}
		}
	}
	label.Child(el.Div().Size(el.Dp(6)).Rounded(3).Bg(dot))
	bar.Child(label)
	tabs := el.Div().Row().Items(el.Center).HFull().Grow().W(el.Dp(0)).ScrollX()
	wide := a.terminalWidth(cx) > 560
	if wide {
		tabs = el.Div().Row().Items(el.Center).HFull().MaxW(el.Frac(.46)).ScrollX()
	}
	for _, tab := range s.Tabs {
		t := tab
		ids := t.Root.panes()
		if len(ids) == 0 {
			continue
		}
		p := s.Panes[ids[0]]
		if p == nil {
			continue
		}
		active := t.ID == s.Active
		row := el.Div().ID("terminal-tab-row/" + t.ID).Name("终端标签：" + p.Title).Role("group").Row().Items(el.Center).HFull().MinW(el.Dp(82)).MaxW(el.Dp(150)).NoShrink()
		if active {
			row.Bg(theme.Bg).Child(el.Div().Absolute().Top(0).Left(0).Right(0).H(el.Dp(2)).Bg(theme.Primary))
		}
		text := p.Title
		if len(ids) > 1 {
			text += fmt.Sprintf(" · %d", len(ids))
		}
		title := button("", func() { a.selectTerminalTab(t) }, kit.ButtonGhost).ID("terminal-tab/" + t.ID).Name(text).Size(36).Compact(true).Content(el.ViewFunc(func(cx *el.Context) el.Element {
			fg := theme.Muted
			if active {
				fg = theme.Text
			}
			r := el.Div().Row().Items(el.Center).Gap(6).Pl(8).Pr(3).Child(el.Text(text).Mono().TextSize(10).TextColor(fg).MaxLines(1))
			if p.Starting {
				r.Child(el.Div().Size(el.Dp(5)).Rounded(3).Bg(theme.RGB(0xb98b36)))
			}
			return r
		})).Render(cx).(*el.DivEl).MaxW(el.Dp(124)).Justify(el.Start)
		close := iconAction(cx, "terminal-close/"+t.ID, "关闭终端标签 "+text, "x", func() { a.closeTerminalTab(t.ID) }, false).(*el.DivEl).NoShrink()
		row.Child(title, close)
		tabs.Child(row)
	}
	bar.Child(tabs, iconAction(cx, "terminal-new", "新建终端", "plus", func() { a.createTerminal("") }, false))
	if wide {
		bar.Child(el.Div().Grow().W(el.Dp(0)))
		status := "未启动"
		if p := a.activeTerminal(); p != nil {
			if p.Starting {
				status = "正在启动…"
			} else if p.Running {
				status = "运行中"
			} else {
				status = fmt.Sprintf("已退出 · %d", p.ExitCode)
			}
		}
		bar.Child(el.Text(status).TextSize(11).TextColor(theme.Muted).MaxLines(1))
	}
	pane := a.activeTerminal()
	s.SplitMenu.SetDisabled(pane == nil)
	search := iconAction(cx, "terminal-search", "搜索终端", "search", a.openTerminalSearch, s.SearchOpen).(*el.DivEl)
	search.Disabled(pane == nil)
	if pane == nil {
		search.Opacity(.35)
	}
	stop := iconAction(cx, "terminal-terminate", "停止终端", "trash-2", a.terminateTerminal, false).(*el.DivEl)
	stop.Disabled(pane == nil || !pane.Running)
	if pane == nil || !pane.Running {
		stop.Opacity(.35)
	}
	split := s.SplitMenu.Render(cx).(*el.DivEl)
	if pane == nil {
		split.Opacity(.35)
	}
	bar.Child(search, split, stop)
	maxLabel, maxIcon := "放大终端", "maximize-2"
	if s.Maximized {
		maxLabel, maxIcon = "还原终端", "minimize-2"
	}
	bar.Child(iconAction(cx, "terminal-maximize", maxLabel, maxIcon, func() { s.Maximized = !s.Maximized }, false), iconAction(cx, "terminal-collapse", "收起终端", "chevron-down", func() { s.Open, s.Maximized = false, false }, false))
	if a.page == "" {
		bar = a.terminalPanelHeader(cx)
	}
	dock.Child(bar)
	body := el.Div().Grow().H(el.Dp(0)).WFull()
	if tab := a.terminalTab(); tab != nil {
		body.Child(a.terminalSplitView(cx, tab.Root))
	} else {
		body.Center().Child(button("", func() { a.createTerminal("") }, kit.ButtonSecondary).Size(30).Content(el.ViewFunc(func(cx *el.Context) el.Element {
			return el.Div().Row().Items(el.Center).Gap(7).Child(uiassets.Lucide(cx, "plus", 14, theme.Muted), el.Text("启动当前工作区终端").TextSize(11).TextColor(theme.Muted))
		})).Render(cx))
	}
	if s.SearchOpen {
		body.Child(a.terminalSearchView(cx))
	}
	return dock.Child(body)
}
func (a *app) terminalSearchView(cx *el.Context) el.Element {
	s := &a.terminal
	if s.SearchFocus {
		cx.Focus("terminal-search-input")
		s.SearchFocus = false
	}
	input := field("terminal-search-input", "搜索终端", &s.Query).TextSize(11).W(el.Dp(max(40, min(180, a.terminalWidth(cx)-170)))).H(el.Dp(26)).Px(0).Bg(color.NRGBA{}).FocusStyle(func(st *el.Style) { st.BorderColor(color.NRGBA{}).Bg(color.NRGBA{}) }).OnChange(func(string) { a.searchTerminal(false) }).CaptureKeys("⏎", "⌤", "⎋").OnKey(func(e el.KeyEvent) bool {
		if e.State != el.KeyPress {
			return false
		}
		switch e.Name {
		case "⏎", "⌤":
			a.searchTerminal(e.Modifiers&key.ModShift != 0)
		case "⎋":
			s.SearchOpen = false
			a.focusTerminal(s.Focused)
		default:
			return false
		}
		return true
	})
	box := el.Div().Absolute().Top(8).Right(12).Row().Items(el.Center).Gap(4).Px(6).Py(2).Rounded(7).Bg(theme.Surface).MaxW(el.Frac(1)).Child(uiassets.Lucide(cx, "search", 13, theme.Muted), input)
	status := el.Div().W(el.Dp(48))
	if !s.SearchFound {
		status.Child(el.Text("未找到").TextSize(9).TextColor(theme.RGB(0xb4473b)))
	}
	small := func(id, label, icon string, callback func()) el.Element {
		return el.Div().ID(id).Name(label).Role("button").Focusable(true).Size(el.Dp(22)).Center().Rounded(4).CursorPointer().OnClick(callback).Hover(func(st *el.Style) { st.Bg(theme.Highlight) }).FocusStyle(neutralFocus).Child(uiassets.Lucide(cx, icon, 13, theme.Muted))
	}
	return box.Child(status, small("terminal-search-prev", "上一个匹配", "chevron-left", func() { a.searchTerminal(true) }), small("terminal-search-next", "下一个匹配", "chevron-right", func() { a.searchTerminal(false) }), small("terminal-search-close", "关闭终端搜索", "x", func() { s.SearchOpen = false; a.focusTerminal(s.Focused) }))
}
func (a *app) terminalSplitView(cx *el.Context, n *terminalSplit) el.Element {
	if n == nil {
		return el.Div()
	}
	if n.Pane != "" {
		p := a.terminal.Panes[n.Pane]
		if p == nil {
			return el.Div()
		}
		p.Screen.SetPalette(a.settings.Terminal.Theme, a.dark)
		box := el.Div().WFull().HFull().Child(el.Widget(p.Screen).WFull().HFull())
		if p.Error != "" {
			box.Child(el.Div().Absolute().Left(10).Right(10).Bottom(8).Rounded(6).P(8).Bg(theme.Subtle).Child(el.Text(p.Error).TextSize(11).TextColor(theme.RGB(0xb4473b))))
		}
		return box
	}
	vertical := n.Direction == "vertical"
	root := el.Div().WFull().HFull().Decorate(func(gtx core.C, draw func()) {
		if gtx.Enabled() {
			scale := gtx.Metric.PxPerDp
			if scale <= 0 {
				scale = 1
			}
			n.Available = float32(gtx.Constraints.Max.Y) / scale
			if vertical {
				n.Available = float32(gtx.Constraints.Max.X) / scale
			}
		}
		draw()
	})
	first := el.Div().NoShrink().Child(a.terminalSplitView(cx, n.First))
	second := el.Div().Grow().Child(a.terminalSplitView(cx, n.Second))
	seam := el.Div().NoShrink().Bg(seamColor)
	if vertical {
		root.Row()
		first.W(el.Frac(n.Ratio)).HFull()
		second.W(el.Dp(0)).HFull()
		seam.W(el.Dp(1)).HFull()
	} else {
		first.H(el.Frac(n.Ratio)).WFull()
		second.H(el.Dp(0)).WFull()
		seam.H(el.Dp(1)).WFull()
	}
	handle := el.Div().Absolute().ID("terminal-split-resize/" + n.ID).Role("separator").Name("调整终端分屏").Focusable(true)
	if vertical {
		handle.Left(-4).Top(0).Bottom(0).W(el.Dp(9)).Cursor(pointer.CursorColResize)
	} else {
		handle.Top(-4).Left(0).Right(0).H(el.Dp(9)).Cursor(pointer.CursorRowResize)
	}
	minimum := float32(96)
	if vertical {
		minimum = 180
	}
	handle.OnDrag(func(e el.DragEvent) {
		pos := e.Y
		if vertical {
			pos = e.X
		}
		switch e.Kind {
		case el.DragStart:
			n.DragPosition = pos
		case el.DragMove:
			if n.Available > 0 {
				n.Ratio = terminalSplitRatio(n.Ratio+(pos-n.DragPosition)/n.Available, n.Available, minimum)
			}
		}
	}).OnKey(func(e el.KeyEvent) bool {
		if e.State != el.KeyPress {
			return false
		}
		delta := float32(0)
		if vertical && e.Name == "←" || !vertical && e.Name == "↑" {
			delta = -.04
		}
		if vertical && e.Name == "→" || !vertical && e.Name == "↓" {
			delta = .04
		}
		if delta == 0 {
			return false
		}
		n.Ratio = terminalSplitRatio(n.Ratio+delta, n.Available, minimum)
		return true
	})
	return root.Child(first, seam.Child(handle), second)
}
