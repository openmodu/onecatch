package keeldesktop

import (
	"gioui.org/f32"
	"gioui.org/io/system"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/theme"
	"image"
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

// Match keel-rex's stroke geometry without borders suppressed by the frameless theme.
func windowsCaptionGlyph(cx *el.Context, name string, ink color.NRGBA) el.Element {
	return el.Widget(core.Func(func(gtx core.C) core.D {
		size := gtx.Dp(unit.Dp(12))
		line := func(x, y, w, h float32) {
			paint.FillShape(gtx.Ops, ink, clip.Rect{Min: image.Pt(gtx.Dp(unit.Dp(x)), gtx.Dp(unit.Dp(y))), Max: image.Pt(gtx.Dp(unit.Dp(x+w)), gtx.Dp(unit.Dp(y+h)))}.Op())
		}
		box := func(x, y float32) { line(x, y, 10, 1); line(x, y+9, 10, 1); line(x, y, 1, 10); line(x+9, y, 1, 10) }
		switch name {
		case "minus":
			line(1, 6, 10, 1)
		case "square":
			box(1, 1)
		case "copy":
			line(3, 0, 9, 1)
			line(11, 0, 1, 9)
			box(0, 2)
		default:
			var path clip.Path
			path.Begin(gtx.Ops)
			scale := gtx.Metric.PxPerDp
			path.MoveTo(f32.Pt(scale, scale))
			path.LineTo(f32.Pt(11*scale, 11*scale))
			path.MoveTo(f32.Pt(11*scale, scale))
			path.LineTo(f32.Pt(scale, 11*scale))
			paint.FillShape(gtx.Ops, ink, clip.Stroke{Path: path.End(), Width: scale}.Op())
		}
		return core.D{Size: image.Pt(size, size)}
	}))
}

// Keep the native caption action scoped around its drawn hit area, matching
// keel-rex. Win32 uses this action to handle caption dragging and double-clicks.
func windowsDragArea(d *el.DivEl) *el.DivEl {
	return d.Decorate(func(gtx core.C, draw func()) {
		defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
		system.ActionInputOp(system.ActionMove).Add(gtx.Ops)
		draw()
	})
}
