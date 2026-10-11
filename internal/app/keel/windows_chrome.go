package keeldesktop

import (
	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/theme"
	"image/color"
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
		}).Child(windowsCaptionGlyph(cx, icon, ink))
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
			trigger := el.Div().ID("windows-application-menu").Role("button").Name("OneCatch 应用菜单").Size(el.Dp(28)).Center().OnClick(menu.Toggle).Hover(func(s *el.Style) { s.Bg(theme.SubtleHover) })
			if a.logo != nil {
				trigger.Child(a.logo.Render(cx))
			}
			return trigger
		}))
		a.windowsMenu = menu
	}
	return a.windowsMenu.Render(cx)
}

func windowsShortcutLabel(shortcut string) string {
	return strings.NewReplacer("mod", "Ctrl", "shift", "Shift", "alt", "Alt", "enter", "Enter").Replace(shortcut)
}

// Caption symbols must not fall back to a document icon when an SVG is absent.
func windowsCaptionGlyph(cx *el.Context, name string, ink color.NRGBA) el.Element {
	switch name {
	case "minus":
		return el.Div().ID("caption-minimize-glyph").W(el.Dp(10)).H(el.Dp(1)).Bg(ink)
	case "square":
		return el.Div().Size(el.Dp(10)).Border(1, ink)
	case "copy":
		return el.Div().Size(el.Dp(12)).Child(
			el.Div().Absolute().Left(3).Top(0).W(el.Dp(9)).H(el.Dp(1)).Bg(ink),
			el.Div().Absolute().Left(11).Top(0).W(el.Dp(1)).H(el.Dp(9)).Bg(ink),
			el.Div().Absolute().Left(0).Top(3).Size(el.Dp(9)).Border(1, ink))
	default:
		return lucide(cx, "x", 12, ink)
	}
}
