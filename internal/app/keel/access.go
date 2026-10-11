package keeldesktop

import (
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/theme"
)

// newAccessSelect shares permission presentation while keeping each control's state independent.
func newAccessSelect() *kit.SelectView {
	selector := kit.Select("").Hint("工作区访问").Appearance(false).Size(32).MenuWidth(320).RowHeight(52)
	selector.SetEntries(kit.SelectOption{Value: "read-only", Label: "只读访问"}, kit.SelectOption{Value: "workspace-write", Label: "工作区访问"}, kit.SelectOption{Value: "full", Label: "完全访问"})
	selector.RenderItem(func(cx *el.Context, item kit.SelectItemContext) el.Element {
		tint, secondary := theme.Text, theme.Muted
		icon, description := "lock", "仅查看文件，不允许修改"
		switch item.Option.Value {
		case "workspace-write":
			icon, description = "shield-check", "允许修改当前工作区内的文件"
		case "full":
			icon, description = "shield-alert", "可访问互联网和电脑上的工作区外文件"
			tint, secondary = accessWarningColor(), accessWarningColor()
		}
		row := el.Div().Row().Items(el.Center).Gap(9)
		if item.Option.Value == "full" {
			background := tint
			background.A = 24
			row.Bg(background).Rounded(6).Px(6)
		}
		return row.Child(
			lucide(cx, icon, 17, tint),
			el.Div().Grow().W(el.Dp(0)).Gap(2).Child(
				el.Text(item.Option.Label).TextColor(tint).MaxLines(1),
				el.Text(description).TextSize(11).TextColor(secondary).MaxLines(1),
			),
		)
	}).RenderValue(func(cx *el.Context, entries []kit.SelectOption) el.Element {
		return el.Div().Row().Child(accessValue(cx, entries[0].Value, entries[0].Label))
	})
	return selector
}

func accessValue(cx *el.Context, value, label string) el.Element {
	tint, icon := theme.Text, "shield-check"
	row := el.Div().Row().Items(el.Center).Gap(5).H(el.Dp(24))
	if value == "read-only" {
		icon = "lock"
	}
	if value == "full" {
		tint, icon = accessWarningColor(), "shield-alert"
		background := tint
		background.A = 24
		row.Bg(background).Rounded(14).Px(8)
	}
	return row.Child(lucide(cx, icon, 16, tint), el.Text(label).TextColor(tint).MaxLines(1))
}
