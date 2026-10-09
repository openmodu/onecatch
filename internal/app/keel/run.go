package keeldesktop

import (
	"bytes"
	"context"
	_ "embed"
	"image/png"
	"runtime"

	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/locale"
	"github.com/dyike/keel/ui/window"
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
	// Restore only the user's existing opt-in. Failure to reopen a listener
	// is visible but does not prevent the desktop from launching.
	if err := b.restoreHostedWorker(a.ctx); err != nil {
		a.errorText = "恢复手机连接失败：" + safeWorkerError(err, "")
	}
	a.dark = a.ui.Preferences.Theme == "dark" || a.ui.Preferences.Theme == "system" && window.SystemAppearance() == window.AppearanceDark
	applyAppPalette(a.dark, a.ui.Preferences.Accent)
	a.logo = kit.Image(icon, "OneCatch").Size(22, 22).Rounded(5)
	a.refresh()
	a.window = window.Open(window.Options{
		Title: "OneCatch · Keel", Width: 1120, Height: 760, MinWidth: 860, MinHeight: 560,
		Frameless: runtime.GOOS == "darwin", NativeTrafficLights: runtime.GOOS == "darwin",
		TrafficLightLayout: &window.TrafficLightLayout{Height: 52, Left: 20},
		Content:            el.Root(a), OnClose: a.close,
		Shortcuts: map[string]func(){"mod+`": a.toggleTerminal, "mod+enter": a.send, "mod+shift+enter": a.sendSteering, "mod+s": a.saveActiveWorkspaceFile, "mod+k": func() { a.ui.SearchOpen = !a.ui.SearchOpen }, "mod+b": a.toggleSidebar, "mod+[": func() { a.goHistory(-1) }, "mod+]": func() { a.goHistory(1) }},
	})
	if err := window.SetApplicationMenu(
		window.MenuItem{Title: "OneCatch", Role: window.MenuApplication, Children: []window.MenuItem{
			{Title: "设置…", Shortcut: "mod+,", OnSelect: a.openSettings},
			{Separator: true},
			{Title: "退出 OneCatch", Shortcut: "mod+q", OnSelect: a.window.Close},
		}},
		window.MenuItem{Title: "文件", Children: []window.MenuItem{
			{Title: "新建任务", Shortcut: "mod+n", OnSelect: func() { a.newTask(a.workspaceID) }},
			{Title: "添加项目…", Shortcut: "mod+o", OnSelect: a.newProject},
		}},
		window.MenuItem{Title: "编辑", Role: window.MenuEdit, Children: []window.MenuItem{
			{Title: "撤销", Shortcut: "mod+z", Action: window.MenuUndo},
			{Title: "重做", Shortcut: "mod+shift+z", Action: window.MenuRedo},
			{Separator: true},
			{Title: "剪切", Shortcut: "mod+x", Action: window.MenuCut},
			{Title: "复制", Shortcut: "mod+c", Action: window.MenuCopy},
			{Title: "粘贴", Shortcut: "mod+v", Action: window.MenuPaste},
			{Title: "全选", Shortcut: "mod+a", Action: window.MenuSelectAll},
		}},
	); err != nil {
		a.close()
		a.window.Close()
		return err
	}
	window.Main()
	return nil
}
