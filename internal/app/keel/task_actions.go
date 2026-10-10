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
	if menu != nil && a.ui.TaskMenuPinned[task.ID] != task.Pinned {
		// Rebuild so the pin entry's label and icon follow the task.
		menu.SetValue(false)
		menu = nil
	}
	if menu == nil {
		pin, pinIcon := "置顶", "pin"
		if task.Pinned {
			pin, pinIcon = "取消置顶", "pin-off"
		}
		menu = kit.Menu().Width(1).Placement(el.Right, el.Start)
		menu.ContentItem("AI 分析标题和类型", "", menuIcon("sparkles", "AI 分析标题和类型"), func() {
			a.analyzeTaskDetails(task.ID)
		}).ContentItem("重命名", "", menuIcon("square-pen", "重命名"), func() {
			current, exists := a.cachedTask(task.ID)
			if !exists {
				return
			}
			a.ui.TaskRenameID, a.ui.TaskTitle = current.ID, current.Title
			a.errorText = ""
			a.ui.TaskDialog.SetValue(true)
		}).ContentItem(pin, "", menuIcon(pinIcon, pin), func() {
			a.toggleTaskPinned(task.ID)
		}).Separator().ContentItem("删除", "", menuIcon("trash-2", "删除"), func() {
			a.confirmDeleteTask(task.ID)
		})
		menu.Trigger(el.ViewFunc(func(cx *el.Context) el.Element {
			return iconAction(cx, "task-actions/"+task.ID, "任务操作", "ellipsis-vertical", menu.Toggle, menu.Value())
		}))
		a.ui.TaskMenus[task.ID] = menu
		a.ui.TaskMenuPinned[task.ID] = task.Pinned
	}
	menu.SetItemDisabled("AI 分析标题和类型", a.ui.TaskAnalyzing[task.ID])
	label := "AI 分析标题和类型"
	if a.ui.TaskAnalyzing[task.ID] {
		label = "正在分析标题和类型…"
	}
	menu.SetItemContent("AI 分析标题和类型", menuIcon("sparkles", label))
	return menu
}

// Direct sidebar actions and the context menu use the same persistence path.
func (a *app) toggleTaskPinned(id string) {
	task, exists := a.cachedTask(id)
	if !exists {
		return
	}
	go func() {
		_, err := a.backend.service.SetTaskPinned(a.ctx, id, !task.Pinned)
		core.Update(func() {
			if a.closed {
				return
			}
			a.fail(err)
			a.refresh()
		})
	}()
}

func (a *app) confirmDeleteTask(id string) {
	task, exists := a.cachedTask(id)
	if !exists {
		return
	}
	a.confirmation.ConfirmDanger("删除任务？", "将从列表中移除「"+task.Title+"」及其执行记录。", "删除", func() {
		go func() {
			err := a.backend.service.DeleteTask(a.ctx, id)
			core.Update(func() {
				if a.closed {
					return
				}
				a.fail(err)
				if err == nil && a.taskID == id {
					a.newTask(task.WorkspaceID)
				}
				a.refresh()
			})
		}()
	})
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

func (a *app) analyzeTaskDetails(id string) {
	if a.ui.TaskAnalyzing[id] {
		return
	}
	if _, exists := a.cachedTask(id); !exists {
		return
	}
	if a.ui.TaskAnalyzing == nil {
		a.ui.TaskAnalyzing = make(map[string]bool)
	}
	a.ui.TaskAnalyzing[id] = true
	a.errorText = ""
	go func() {
		task, err := a.backend.service.AnalyzeTaskDetails(a.ctx, id)
		core.Update(func() {
			delete(a.ui.TaskAnalyzing, id)
			if a.closed {
				return
			}
			a.fail(err)
			if err == nil {
				a.applyAnalyzedTaskDetails(task)
				a.refresh()
			}
		})
	}()
}

func (a *app) applyAnalyzedTaskDetails(updated domaintasks.Task) {
	apply := func(task *domaintasks.Task) {
		if task.ID != updated.ID {
			return
		}
		task.Title, task.Category, task.CategorySource, task.UpdatedAt = updated.Title, updated.Category, updated.CategorySource, updated.UpdatedAt
	}
	for _, tasks := range a.tasks {
		for i := range tasks {
			apply(&tasks[i])
		}
	}
	if a.task != nil {
		apply(a.task)
	}
	// Analysis may finish after navigation, or before a session has a run.
	if a.detail != nil {
		apply(&a.detail.Task)
	}
	if a.ui.SearchPalette != nil && a.ui.SearchPalette.Value() {
		a.refreshSearchItems()
	}
}
