package keeldesktop

import (
	"context"
	_ "embed"
	"os"
	"runtime"

	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/locale"
	"github.com/dyike/keel/ui/window"
	"github.com/openmodu/onecatch/internal/service/appupdate"
)

//go:embed appicon.png
var appIcon []byte

type Options struct{ DataRoot string }

func Run(options Options) error {
	configureMemoryBudget()
	stopDiagnostics, err := startMemoryDiagnostics()
	if err != nil {
		return err
	}
	defer stopDiagnostics()
	prepareCommandEnvironment()
	loadPlatformFonts()
	b, err := openBackend(context.Background(), options.DataRoot)
	if err != nil {
		return err
	}
	icon, err := decodeStatusIcon(appIcon)
	if err != nil {
		b.close()
		return err
	}
	executable, _ := os.Executable()
	if !hasPackagedIcon(runtime.GOOS, executable) {
		if err := window.SetIcon(appIcon); err != nil {
			b.close()
			return err
		}
	}
	locale.Apply(locale.Chinese())
	applyPalette(window.SystemAppearance() == window.AppearanceDark)
	a := newApp(b)
	a.applyLanguage()
	// Restore only the user's existing opt-in. Failure to reopen a listener
	// is visible but does not prevent the desktop from launching.
	if err := b.restoreHostedWorker(a.ctx); err != nil {
		a.errorText = "恢复手机连接失败：" + safeWorkerError(err, "")
	}
	a.dark = a.ui.Preferences.Theme == "dark" || a.ui.Preferences.Theme == "system" && window.SystemAppearance() == window.AppearanceDark
	applyAppPalette(a.dark, a.ui.Preferences.Accent)
	a.logo = kit.Image(icon, "OneCatch").Size(22, 22).Rounded(5)
	a.refresh()
	// Overflowing views always show their thin scrollbar.
	el.SetScrollbarDefault(el.ScrollbarAlways)
	// Tray creation must happen before Main, outside the UI frame lock.
	if err := a.installTray(icon); err != nil {
		a.errorText = "初始化托盘失败：" + err.Error()
	}
	a.showMainWindow()
	if err := a.installApplicationMenu(); err != nil {
		a.close()
		a.window.Close()
		return err
	}
	a.initUpdates()
	core.Update(func() { a.fail(appupdate.SignalReadyFromEnvironment()) })
	window.MainWithOptions(window.MainOptions{KeepAlive: a.tray.icon != nil, OnQuit: a.close})
	return nil
}

func (a *app) showMainWindow() {
	if a.closed {
		return
	}
	if a.window != nil && !a.window.Closed() {
		a.window.Raise()
		return
	}
	if a.tray.root == nil {
		a.tray.root = el.Root(a)
	}
	w := window.Open(platformWindowOptions(runtime.GOOS, window.Options{
		Title: "OneCatch · Keel", Width: 1120, Height: 760, MinWidth: 640, MinHeight: 480,
		Frameless: runtime.GOOS == "darwin", NativeTrafficLights: runtime.GOOS == "darwin",
		TrafficLightLayout: &window.TrafficLightLayout{Height: mainTitleBarHeight, Left: 20},
		Content:            a.tray.root, OnClose: a.mainWindowClosed,
		Shortcuts: map[string]func(){"mod+`": a.toggleTerminal, "mod+enter": a.send, "mod+shift+enter": a.sendSteering, "mod+s": a.saveActiveWorkspaceFile, "mod+k": a.focusTaskSearch, "mod+b": a.toggleSidebar, "mod+[": func() { a.goHistory(-1) }, "mod+]": func() { a.goHistory(1) }},
	}))
	a.window = w
	// Retain the root's scroll, input and focus state across native windows.
	core.Update(func() {
		if !a.closed && !w.Closed() && !w.Maximized() {
			w.ToggleMaximize()
		}
	})
}

func (a *app) mainWindowClosed() {
	a.window = nil
	if a.tray.icon == nil {
		a.close()
	}
}

func (a *app) hideMainWindow() {
	if a.tray.icon != nil && a.window != nil {
		a.window.Close()
	}
}

func (a *app) installApplicationMenu() error {
	menu, err := window.NewMenuBar(
		window.MenuItem{Title: "OneCatch", Role: window.MenuApplication, Children: []window.MenuItem{
			{Title: uiText("设置…"), Shortcut: "mod+,", OnSelect: func() { a.showMainWindow(); a.openSettings() }},
			{Separator: true},
			{Title: uiText("退出 OneCatch"), Shortcut: "mod+q", OnSelect: window.Quit},
		}},
		window.MenuItem{Title: uiText("文件菜单"), Children: []window.MenuItem{
			{Title: uiText("新建任务"), Shortcut: "mod+n", OnSelect: func() { a.showMainWindow(); a.newTask(a.workspaceID) }},
			{Title: uiText("添加项目…"), Shortcut: "mod+o", OnSelect: func() { a.showMainWindow(); a.newProject() }},
		}},
		window.MenuItem{Title: uiText("编辑"), Role: window.MenuEdit, Children: []window.MenuItem{
			{Title: uiText("撤销"), Shortcut: "mod+z", Action: window.MenuUndo},
			{Title: uiText("重做"), Shortcut: "mod+shift+z", Action: window.MenuRedo},
			{Separator: true},
			{Title: uiText("剪切"), Shortcut: "mod+x", Action: window.MenuCut},
			{Title: uiText("复制"), Shortcut: "mod+c", Action: window.MenuCopy},
			{Title: uiText("粘贴"), Shortcut: "mod+v", Action: window.MenuPaste},
			{Title: uiText("全选"), Shortcut: "mod+a", Action: window.MenuSelectAll},
		}},
	)
	if err != nil {
		return err
	}
	a.applicationMenu = menu
	return menu.Install()
}
