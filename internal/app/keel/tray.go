package keeldesktop

import (
	"fmt"
	"image"
	"image/color"
	"runtime"
	"slices"
	"strings"

	"github.com/dyike/keel/native/tray"
	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/window"
	"github.com/mattn/go-runewidth"
	domaintasks "github.com/openmodu/onecatch/internal/domain/tasks"
)

type trayIcon interface {
	SetMenu([]tray.MenuItem) error
	Close() error
}

type trayState struct {
	icon  trayIcon
	model string
	root  *el.RootWidget
}

func (a *app) installTray(icon image.Image) error {
	if runtime.GOOS == "darwin" {
		icon = trayTemplateIcon(icon)
	}
	items := a.trayMenu()
	status, err := tray.New(tray.Options{
		Icon: icon, Template: runtime.GOOS == "darwin", Tooltip: "OneCatch", Menu: items,
		OnActivate: a.trayAction(a.showMainWindow),
	})
	if err != nil {
		return err
	}
	a.tray.icon, a.tray.model = status, trayMenuModel(items)
	return nil
}

// Native tray callbacks run outside the frame lock, including with no window.
func (a *app) trayAction(action func()) func() {
	return func() {
		post := a.postUpdate
		if post == nil {
			post = core.Update
		}
		post(func() {
			if !a.closed {
				action()
			}
		})
	}
}

func (a *app) trayMenu() []tray.MenuItem {
	items := []tray.MenuItem{
		{ID: "show", Title: uiText("显示 OneCatch"), OnSelect: a.trayAction(a.showMainWindow)},
		{ID: "hide", Title: uiText("隐藏窗口"), OnSelect: a.trayAction(a.hideMainWindow)},
		{ID: "new", Title: uiText("新建任务"), OnSelect: a.trayAction(func() { a.showMainWindow(); a.newTask(a.workspaceID) })},
		{Separator: true},
		{ID: "recent", Title: uiText("最近活跃的 Session"), Disabled: true},
	}
	workspaces := map[string]string{}
	var recent []domaintasks.Task
	for _, workspace := range a.workspaces {
		if workspace.Hidden {
			continue
		}
		workspaces[workspace.ID] = workspace.Name
		for _, task := range a.tasks[workspace.ID] {
			if task.ID != "" {
				recent = append(recent, task)
			}
		}
	}
	slices.SortStableFunc(recent, func(a, b domaintasks.Task) int {
		if runningA, runningB := a.Status == domaintasks.StatusRunning, b.Status == domaintasks.StatusRunning; runningA != runningB {
			if runningA {
				return -1
			}
			return 1
		}
		if order := sidebarTaskActivity(b).Compare(sidebarTaskActivity(a)); order != 0 {
			return order
		}
		return strings.Compare(a.ID, b.ID)
	})
	for _, task := range recent[:min(5, len(recent))] {
		id := task.ID
		label := traySessionLabel(task.Title, 28)
		if name := workspaces[task.WorkspaceID]; name != "" {
			label = traySessionLabel(name, 10) + " · " + label
		}
		if task.Status == domaintasks.StatusRunning {
			label = "[" + uiText("正在运行") + "] " + label
		}
		items = append(items, tray.MenuItem{ID: "task/" + id, Title: label, OnSelect: a.trayAction(func() {
			// Look up current data: the menu snapshot may outlive this task.
			if _, ok := a.cachedTask(id); !ok {
				return
			}
			a.showMainWindow()
			if a.taskID != id || a.page != "" {
				a.sidebarSelect("task/" + id)
			}
		})})
	}
	if len(recent) == 0 {
		items = append(items, tray.MenuItem{ID: "empty", Title: uiText("暂无会话"), Disabled: true})
	}
	return append(items, tray.MenuItem{Separator: true},
		tray.MenuItem{ID: "settings", Title: uiText("设置…"), OnSelect: a.trayAction(func() { a.showMainWindow(); a.openSettings() })},
		tray.MenuItem{ID: "quit", Title: uiText("退出 OneCatch"), OnSelect: window.Quit},
	)
}

func (a *app) refreshTrayMenu() {
	if a.closed || a.tray.icon == nil {
		return
	}
	items := a.trayMenu()
	model := trayMenuModel(items)
	if model == a.tray.model {
		return
	}
	if err := a.tray.icon.SetMenu(items); err != nil {
		a.fail(err)
		return
	}
	a.tray.model = model
}

func trayMenuModel(items []tray.MenuItem) string {
	var model strings.Builder
	for _, item := range items {
		fmt.Fprintf(&model, "%q/%q/%t/%t;", item.ID, item.Title, item.Disabled, item.Separator)
	}
	return model.String()
}

func traySessionLabel(text string, columns int) string {
	text = strings.Join(strings.Fields(text), " ")
	if text == "" {
		return uiText("未命名会话")
	}
	return runewidth.Truncate(text, columns, "…")
}

// Remove the app icon's dark tile; AppKit supplies the template's ink color.
func trayTemplateIcon(source image.Image) image.Image {
	bounds := source.Bounds()
	mask := image.NewNRGBA(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, alpha := source.At(x, y).RGBA()
			brightness := max(r, g, b) >> 8
			coverage := min(90, max(0, int(brightness)-55))
			mask.SetNRGBA(x, y, color.NRGBA{A: uint8((alpha >> 8) * uint32(coverage) / 90)})
		}
	}
	return mask
}
