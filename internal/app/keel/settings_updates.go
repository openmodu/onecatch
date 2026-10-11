package keeldesktop

import (
	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/theme"
	"github.com/openmodu/onecatch/internal/buildinfo"
	"github.com/openmodu/onecatch/internal/service/appupdate"
)

type appUpdates struct {
	service *appupdate.Service
	status  appupdate.Status
	busy    bool
	err     string
}

type nativeUpdateHost struct{ app *app }

func (h nativeUpdateHost) Quit() {
	core.Update(func() {
		if h.app.window != nil {
			h.app.window.Close()
		}
	})
}
func (a *app) initUpdates() {
	host := nativeUpdateHost{a}
	service, err := appupdate.New(a.backend.service.DataRoot(), func(status appupdate.Status) {
		core.Update(func() {
			if !a.closed {
				a.updates.status = status
			}
		})
	}, host.Quit)
	if err != nil {
		a.updates.err = err.Error()
		return
	}
	a.updates.service = service
	a.updates.status = service.Status()
	// Use the existing check interval, never a frame-driven network request.
	service.Start()
}
func (p *settingsPage) updateAction(action string) {
	a := p.app
	if a.updates.busy {
		return
	}
	if a.updates.service == nil {
		a.initUpdates()
	}
	if a.updates.service == nil {
		return
	}
	a.updates.busy, a.updates.err = true, ""
	service, ctx := a.updates.service, a.ctx
	go func() {
		var err error
		switch action {
		case "download":
			_, err = service.Download(ctx)
		case "apply":
			err = service.Apply(ctx)
		default:
			_, err = service.Check(ctx)
		}
		status := service.Status()
		core.Update(func() {
			if a.closed {
				return
			}
			a.updates.busy = false
			a.updates.status = status
			if err != nil {
				a.updates.err = err.Error()
			}
		})
	}()
}
func (p *settingsPage) updateSettingsGroup() kit.SettingGroup {
	return kit.SettingGroup{Title: "软件更新", Items: []kit.SettingItem{{Label: "OneCatch " + buildinfo.Version, Content: el.ViewFunc(func(cx *el.Context) el.Element {
		state := p.app.updates
		label := "检查可用更新"
		if buildinfo.Version == "dev" {
			label = "开发版本不提供自动更新"
		} else {
			switch state.status.State {
			case "up-to-date":
				label = "当前已是最新版本"
			case "available":
				label = "发现新版本 " + state.status.AvailableVersion
			case "ready":
				label = "更新已下载，重启后安装"
			case "checking":
				label = "正在检查更新…"
			case "downloading", "installing":
				label = "正在下载并校验更新…"
			case "unconfigured":
				label = "此构建未配置签名更新"
			}
		}
		root := el.Div().Gap(10)
		row := el.Div().Row().Items(el.Center).Gap(8).Child(el.Text(label).TextSize(13).TextColor(theme.Muted).Grow())
		check := button("检查更新", func() { p.updateAction("check") }, kit.ButtonGhost).Loading(state.busy)
		check.SetDisabled(state.busy || buildinfo.Version == "dev")
		row.Child(check.Render(cx))
		if state.status.State == "available" {
			b := button("下载更新", func() { p.updateAction("download") }, kit.ButtonPrimary)
			b.SetDisabled(state.busy)
			row.Child(b.Render(cx))
		}
		if state.status.State == "ready" && state.status.AutomaticSupported {
			b := button("重启并安装", func() {
				p.confirm.ConfirmDanger("安装更新", "应用将退出并重启，请先结束正在执行的任务。", "重启并安装", func() { p.updateAction("apply") })
			}, kit.ButtonPrimary)
			b.SetDisabled(state.busy)
			row.Child(b.Render(cx))
		}
		root.Child(row)
		if state.err != "" {
			root.Child(el.Text(state.err).TextSize(12).TextColor(theme.DangerText))
		}
		if state.status.Notes != "" {
			root.Child(el.Text(state.status.Notes).TextSize(12).TextColor(theme.Muted))
		}
		return root
	})}}}
}
