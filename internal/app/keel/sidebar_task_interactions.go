package keeldesktop

import (
	"fmt"

	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/theme"
)

type sidebarTaskPreview struct {
	id           string
	shown, muted bool
}

// Expanded Sidebar rows do not yet expose a preview slot. Keep the dependency
// on Keel's row IDs here; layout, hit testing and context menus stay in Sidebar.
func (a *app) sidebarTaskRowID(id string) string {
	return fmt.Sprintf("kit-sidebar-%p/task/%s", a.ui.SidebarNav, id)
}

func (a *app) sidebarTaskActions(cx *el.Context, id string) el.Element {
	box := el.Div().Row().Items(el.Center).W(el.Dp(54)).H(el.Dp(36)).NoShrink().Gap(2)
	task, exists := a.cachedTask(id)
	if !exists {
		return box
	}
	label, icon := "置顶任务", "pin"
	if task.Pinned {
		label, icon = "取消置顶任务", "pin-off"
	}
	return box.Child(
		iconAction(cx, "task-pin/"+id, label, icon, func() { a.toggleTaskPinned(id) }, task.Pinned),
		iconAction(cx, "task-delete/"+id, "删除任务", "trash-2", func() { a.confirmDeleteTask(id) }, false))
}

func (a *app) sidebarTaskDetails(cx *el.Context) {
	id := ""
	for _, taskID := range a.ui.SidebarVisibleTasks {
		rowID := a.sidebarTaskRowID(taskID)
		if cx.Hovered(rowID) && !cx.Hovered(rowID+"/suffix") {
			id = taskID
			break
		}
	}
	state := &a.ui.SidebarPreview
	if state.id != id {
		*state = sidebarTaskPreview{id: id}
	}
	if id == "" {
		return
	}
	rowID := a.sidebarTaskRowID(id)
	if !state.shown {
		cx.AfterEnabled(rowID, "sidebar-task-preview/"+id, kit.TooltipDelay, func() { state.shown = true })
	}
	if !state.shown || state.muted || a.taskMenuForIDOpen(id) {
		return
	}
	task, ok := a.cachedTask(id)
	if !ok {
		return
	}
	project := ""
	for _, workspace := range a.workspaces {
		if workspace.ID == task.WorkspaceID {
			project = workspace.Name
			break
		}
	}
	width, _ := cx.ViewportSize()
	panel := el.Div().Role("tooltip").Name("任务详情：" + task.Title).W(el.Dp(min(320, width-16))).P(14).Gap(10).
		Bg(theme.Surface).TextColor(theme.Text).Rounded(12).Shadow(theme.ElevationSm).
		Disabled(true).DisabledStyle(func(s *el.Style) { s.TextColor(theme.Text) }).Child(el.Text(task.Title).TextSize(14).Medium())
	if project != "" {
		panel.Child(el.Div().Row().Items(el.Center).Gap(8).Child(lucide(cx, "folder", 16, theme.Muted), el.Text(project).TextSize(12).TextColor(theme.Muted)))
	}
	cx.Overlay("sidebar-task-preview", el.Anchored(rowID+"/frame", panel).Owner(rowID).Placement(el.Right, el.Center).Offset(6).OnDismiss(func() { state.muted = true }))
}

func (a *app) taskMenuForIDOpen(id string) bool {
	return a.ui.TaskMenus[id] != nil && a.ui.TaskMenus[id].Value()
}
