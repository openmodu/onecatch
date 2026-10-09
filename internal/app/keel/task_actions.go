package keeldesktop

import (
	"strings"

	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	domaintasks "github.com/openmodu/onecatch/internal/domain/tasks"
)

func (a *app) cachedTask(id string) (domaintasks.Task, bool) {
	for _, tasks := range a.tasks {
		for _, task := range tasks {
			if task.ID == id {
				return task, true
			}
		}
	}
	return domaintasks.Task{}, false
}

func (a *app) taskMenu(task domaintasks.Task) *kit.MenuView {
	menu := a.ui.TaskMenus[task.ID]
	if menu == nil {
		menu = kit.Menu().Width(168).Placement(el.Right, el.Start)
		menu.ContentItem("重命名", "", menuIcon("square-pen", "重命名"), func() {
			current, exists := a.cachedTask(task.ID)
			if !exists {
				return
			}
			a.ui.TaskRenameID, a.ui.TaskTitle = current.ID, current.Title
			a.errorText = ""
			a.ui.TaskDialog.SetValue(true)
		}).CheckItem("置顶", "", task.Pinned, func(on bool) {
			go func() {
				_, err := a.backend.service.SetTaskPinned(a.ctx, task.ID, on)
				core.Update(func() {
					if a.closed {
						return
					}
					a.fail(err)
					a.refresh()
				})
			}()
		}).Separator().ContentItem("删除", "", menuIcon("trash-2", "删除"), func() {
			a.confirmation.Confirm("删除任务？", "任务和执行记录将从列表中移除。", func() {
				go func() {
					err := a.backend.service.DeleteTask(a.ctx, task.ID)
					core.Update(func() {
						if a.closed {
							return
						}
						a.fail(err)
						if err == nil && a.taskID == task.ID {
							a.newTask(task.WorkspaceID)
						}
						a.refresh()
					})
				}()
			})
		})
		menu.Trigger(el.ViewFunc(func(cx *el.Context) el.Element {
			return iconAction(cx, "task-actions/"+task.ID, "任务操作", "ellipsis", menu.Toggle, menu.Value())
		}))
		a.ui.TaskMenus[task.ID] = menu
	}
	menu.SetItemChecked("置顶", task.Pinned)
	return menu
}

func (a *app) renameTask() {
	if a.ui.TaskSaving || strings.TrimSpace(a.ui.TaskTitle) == "" {
		return
	}
	id, title := a.ui.TaskRenameID, a.ui.TaskTitle
	a.ui.TaskSaving = true
	go func() {
		task, err := a.backend.service.RenameTask(a.ctx, id, title)
		core.Update(func() {
			if a.closed {
				return
			}
			a.ui.TaskSaving = false
			a.fail(err)
			if err == nil {
				a.ui.TaskDialog.SetValue(false)
				if a.taskID == id {
					a.task = &task
				}
			}
			a.refresh()
		})
	}()
}
