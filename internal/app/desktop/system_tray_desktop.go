//go:build windows || linux

package desktop

import (
	"context"
	"sort"
	"sync"

	desktopassets "github.com/openmodu/onecatch/internal/app/desktop/assets"
	domaintasks "github.com/openmodu/onecatch/internal/domain/tasks"
	desktopservice "github.com/openmodu/onecatch/internal/service/desktop"
	"github.com/wailsapp/wails/v3/pkg/application"
	"go.uber.org/zap"
)

const (
	trayNavigationEvent = "onecatch:tray-navigate"
)

func installDesktopSystemTray(
	app *application.App,
	mainWindow *application.WebviewWindow,
	service *desktopservice.Service,
	log *zap.Logger,
	openSettings func(),
) func() {
	showMainWindow := func() {
		// A hidden window may also have been minimised before it was closed.
		// Restore both states so the tray action always brings a usable
		// workbench to the foreground.
		mainWindow.UnMinimise()
		mainWindow.Show().Focus()
	}

	tray := app.SystemTray.New()
	tray.SetIcon(desktopassets.AppIcon)
	// Wails beta.10 does not implement SetTooltip on Linux. StatusNotifier
	// hosts use the label-backed DBus Title for hover text instead; without an
	// explicit label Wails falls back to "Wails".
	tray.SetLabel(Name)
	tray.SetTooltip(Name)
	// StatusNotifier hosts deliver the primary activation separately from the
	// menu. Make that common action restore the main workbench too.
	tray.OnClick(showMainWindow)

	var refreshMu sync.Mutex
	refresh := func() {
		refreshMu.Lock()
		defer refreshMu.Unlock()

		tasks, err := service.ListTasks(context.Background(), "")
		if err != nil {
			log.Error("refresh desktop tray sessions", zap.Error(err))
			return
		}
		tasks = latestActiveTrayTasks(tasks, 3)

		menu := app.NewMenu()
		menu.Add("新建会话").OnClick(func(*application.Context) {
			showMainWindow()
			app.Event.Emit(trayNavigationEvent, map[string]string{"action": "new"})
		})
		menu.AddSeparator()
		menu.Add("活跃会话").SetEnabled(false)
		if len(tasks) == 0 {
			menu.Add("暂无活跃会话").SetEnabled(false)
		}
		for _, task := range tasks {
			task := task
			runID := ""
			runs, listErr := service.ListRunsByTask(context.Background(), task.ID)
			if listErr != nil {
				log.Warn("load desktop tray session run", zap.String("task_id", task.ID), zap.Error(listErr))
			} else if len(runs) > 0 {
				runID = runs[0].ID
			}
			menu.Add(traySessionLabel(task)).OnClick(func(*application.Context) {
				showMainWindow()
				app.Event.Emit(trayNavigationEvent, map[string]string{
					"action":      "open",
					"workspaceId": task.WorkspaceID,
					"taskId":      task.ID,
					"runId":       runID,
				})
			})
		}
		menu.AddSeparator()
		menu.Add("显示 OneCatch").OnClick(func(*application.Context) {
			showMainWindow()
		})
		menu.Add("设置…").OnClick(func(*application.Context) {
			openSettings()
		})
		menu.AddSeparator()
		menu.Add("退出 OneCatch").OnClick(func(*application.Context) {
			app.Quit()
		})
		tray.SetMenu(menu)
	}
	refresh()
	return refresh
}

func latestActiveTrayTasks(tasks []domaintasks.Task, limit int) []domaintasks.Task {
	if limit <= 0 || len(tasks) == 0 {
		return nil
	}
	// The repository normally puts pinned tasks first for the sidebar. The tray
	// is explicitly activity- and recency-based, so filter and order its own
	// copy here.
	result := make([]domaintasks.Task, 0, len(tasks))
	for _, task := range tasks {
		switch task.Status {
		case domaintasks.StatusQueued, domaintasks.StatusReady, domaintasks.StatusRunning, domaintasks.StatusPaused:
			result = append(result, task)
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].UpdatedAt.Equal(result[j].UpdatedAt) {
			return result[i].ID < result[j].ID
		}
		return result[i].UpdatedAt.After(result[j].UpdatedAt)
	})
	if len(result) > limit {
		result = result[:limit]
	}
	return result
}

func traySessionLabel(task domaintasks.Task) string {
	title := trayConversationLabel(task.Title)
	switch task.Status {
	case domaintasks.StatusRunning:
		return title + "  · 运行中"
	case domaintasks.StatusPaused:
		return title + "  · 已暂停"
	case domaintasks.StatusQueued:
		return title + "  · 排队中"
	case domaintasks.StatusReady:
		return title + "  · 就绪"
	default:
		return title
	}
}
