package keeldesktop

import (
	"slices"

	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/theme"
	uiassets "github.com/openmodu/onecatch/internal/app/keel/assets"
	domaintasks "github.com/openmodu/onecatch/internal/domain/tasks"
)

func (a *app) openSearchPalette() {
	if a.ui.SearchPalette == nil {
		a.ui.SearchPalette = kit.Command().Placeholder("搜索对话和项目…").RowHeight(40).MaxHeight(560).HeaderBelowSearch(true).CloseButton(true).
			RenderItem(func(item kit.CommandItem, active bool) el.View {
				return el.ViewFunc(func(cx *el.Context) el.Element {
					icon := "message-square"
					if item.Group == "项目" {
						icon = "folder"
					}
					if item.Group == "操作" {
						icon = "plus"
					}
					return el.Div().Row().Items(el.Center).Gap(10).Child(uiassets.Lucide(cx, icon, 17, theme.Muted), el.Text(item.Title).Grow().MaxLines(1))
				})
			}).
			PanelStyle(func(panel *el.DivEl) { panel.W(el.Dp(620)).Rounded(16) }).
			Header(el.ViewFunc(func(cx *el.Context) el.Element {
				row := el.Div().Row().Items(el.Center).Gap(4).Px(16).Py(8)
				for _, tab := range []struct{ ID, Label string }{{"", "全部"}, {"tasks", "对话"}, {"projects", "项目"}} {
					id := tab.ID
					row.Child(button(tab.Label, func() { a.ui.SearchCategory = id; a.refreshSearchItems() }, kit.ButtonGhost).
						Selected(a.ui.SearchCategory == id).Size(30).Render(cx))
				}
				return row
			})).
			Footer(el.ViewFunc(func(cx *el.Context) el.Element {
				row := el.Div().Row().Items(el.Center).Gap(18).Px(16).Py(12).Bg(theme.Subtle)
				for _, hint := range []struct{ Key, Label string }{{"↑ ↓", "选择"}, {"↵", "打开"}, {"Esc", "关闭"}} {
					row.Child(el.Div().Row().Items(el.Center).Gap(6).Child(
						el.Text(hint.Key).TextSize(11).TextColor(theme.Text).Bg(theme.SubtleHover).Rounded(4).Px(5).Py(2),
						el.Text(hint.Label).TextSize(11).TextColor(theme.Muted)))
				}
				return el.Div().Child(el.Div().H(el.Dp(1)).Bg(theme.Border), row)
			}))
	}
	a.ui.SearchCategory = ""
	a.query = ""
	a.refreshSearchItems()
	a.ui.SearchPalette.SetValue(true)
}

func (a *app) refreshSearchItems() {
	var items []kit.CommandItem
	if a.ui.SearchCategory != "projects" {
		var tasks []domaintasks.Task
		for _, group := range a.tasks {
			tasks = append(tasks, group...)
		}
		slices.SortStableFunc(tasks, func(a, b domaintasks.Task) int { return b.UpdatedAt.Compare(a.UpdatedAt) })
		for _, task := range tasks {
			if !task.DeletedAt.IsZero() {
				continue
			}
			task := task
			items = append(items, kit.CommandItem{Title: task.Title, Group: "最近对话", Icon: kit.IconFile, Keywords: []string{task.Prompt, task.WorkspaceID}, Action: func() { a.openTask(task) }})
		}
	}
	if a.ui.SearchCategory != "tasks" {
		for _, workspace := range a.workspaces {
			workspace := workspace
			items = append(items, kit.CommandItem{Title: workspace.Name, Group: "项目", Icon: kit.IconFolder, Keywords: []string{workspace.Path}, Action: func() { a.newTask(workspace.ID) }})
		}
	}
	if a.ui.SearchCategory == "" {
		items = append(items, kit.CommandItem{Title: "新建对话", Group: "操作", Icon: kit.IconPlus, Action: func() { a.newTask(a.workspaceID) }})
	}
	a.ui.SearchPalette.SetItems(items...)
}

func (a *app) renderSearchPalette(cx *el.Context) el.Element {
	palette := theme.Current()
	if a.dark {
		palette.Surface = theme.RGB(0x303030)
		palette.Subtle = theme.RGB(0x292929)
		palette.SubtleHover = theme.RGB(0x444444)
		palette.Highlight = theme.RGB(0x454545)
		palette.Border = theme.RGB(0x494949)
		palette.Muted = theme.RGB(0xb5b5b5)
	} else {
		palette.Border = theme.RGB(0xe9e9e7)
	}
	return cx.Themed(palette, a.ui.SearchPalette)
}
