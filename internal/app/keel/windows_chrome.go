package keeldesktop

import (
	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/theme"
	"strings"
)

func windowsWindowButtons(cx *el.Context) el.Element {
	row := el.Div().Row().Items(el.Center).NoShrink().H(el.Dp(44)).Child(el.Div().W(el.Dp(1)).H(el.Dp(20)).Bg(seamColor), el.Div().W(el.Dp(7)))
	w := core.CurrentWindow()
	if w == nil {
		return row
	}
	control := func(id, label, icon string, action func()) el.Element {
		ink := theme.Muted
		if id == "window-close" && cx.Hovered(id) {
			ink = theme.RGB(0xffffff)
		}
		return el.Div().ID(id).Role("button").Name(label).W(el.Dp(44)).H(el.Dp(44)).Center().Focusable(false).
			OnClick(action).Hover(func(s *el.Style) {
			if id == "window-close" {
				s.Bg(theme.RGB(0xc42b1c))
			} else {
				s.Bg(theme.SubtleHover)
			}
		}).Child(lucide(cx, icon, 12, ink))
	}
	maxLabel, maxIcon := "最大化", "square"
	if w.Maximized() {
		maxLabel, maxIcon = "还原", "copy"
	}
	return row.Child(control("window-minimize", "最小化", "minus", w.Minimize), control("window-maximize", maxLabel, maxIcon, w.ToggleMaximize), control("window-close", "关闭", "x", w.Close))
}

func (a *app) windowsApplicationMenu(cx *el.Context) el.Element {
	if a.applicationMenu == nil {
		return el.Div()
	}
	if a.windowsMenu == nil {
		menu := kit.Menu().Width(208).Placement(el.Bottom, el.Start)
		for groupIndex, group := range a.applicationMenu.Items() {
			if groupIndex > 0 {
				menu.Separator()
			}
			for _, item := range group.Children {
				if item.Separator {
					menu.Separator()
					continue
				}
				id := item.ID
				menu.Item(item.Title, windowsShortcutLabel(item.Shortcut), func() { a.applicationMenu.Invoke(id) })
			}
		}
		menu.Trigger(el.ViewFunc(func(cx *el.Context) el.Element {
			return iconAction(cx, "windows-application-menu", "应用菜单", "menu", menu.Toggle, menu.Value())
		}))
		a.windowsMenu = menu
	}
	return a.windowsMenu.Render(cx)
}

func windowsShortcutLabel(shortcut string) string {
	return strings.NewReplacer("mod", "Ctrl", "shift", "Shift", "alt", "Alt", "enter", "Enter").Replace(shortcut)
}
