package keeldesktop

import (
	"bytes"
	"context"
	_ "embed"
	"image/png"
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
	prepareCommandEnvironment()
	b, err := openBackend(context.Background(), options.DataRoot)
	if err != nil {
		return err
	}
	icon, err := png.Decode(bytes.NewReader(appIcon))
	if err != nil {
		b.close()
		return err
	}
	if err := window.SetIcon(appIcon); err != nil {
		b.close()
		return err
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
	a.window = window.Open(window.Options{
		Title: "OneCatch · Keel", Width: 1120, Height: 760, MinWidth: 640, MinHeight: 480,
		Frameless: runtime.GOOS == "darwin", NativeTrafficLights: runtime.GOOS == "darwin",
		TrafficLightLayout: &window.TrafficLightLayout{Height: mainTitleBarHeight, Left: 20},
		Content:            el.Root(a), OnClose: a.close,
		Shortcuts: map[string]func(){"mod+`": a.toggleTerminal, "mod+enter": a.send, "mod+shift+enter": a.sendSteering, "mod+s": a.saveActiveWorkspaceFile, "mod+k": a.focusTaskSearch, "mod+b": a.toggleSidebar, "mod+[": func() { a.goHistory(-1) }, "mod+]": func() { a.goHistory(1) }},
	})
	// Window actions wait for the first native frame before applying.
	core.Update(func() {
		if !a.closed && !a.window.Maximized() {
			a.window.ToggleMaximize()
		}
	})
	if err := a.installApplicationMenu(); err != nil {
		a.close()
		a.window.Close()
		return err
	}
	a.initUpdates()
	core.Update(func() { a.fail(appupdate.SignalReadyFromEnvironment()) })
	window.Main()
	return nil
}

func (a *app) installApplicationMenu() error {
	return window.SetApplicationMenu(
		window.MenuItem{Title: "OneCatch", Role: window.MenuApplication, Children: []window.MenuItem{
			{Title: uiText("设置…"), Shortcut: "mod+,", OnSelect: a.openSettings},
			{Separator: true},
			{Title: uiText("退出 OneCatch"), Shortcut: "mod+q", OnSelect: a.window.Close},
		}},
		window.MenuItem{Title: uiText("文件菜单"), Children: []window.MenuItem{
			{Title: uiText("新建任务"), Shortcut: "mod+n", OnSelect: func() { a.newTask(a.workspaceID) }},
			{Title: uiText("添加项目…"), Shortcut: "mod+o", OnSelect: a.newProject},
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
}
