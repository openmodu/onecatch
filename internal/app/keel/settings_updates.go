package keeldesktop

import (
	"context"
	"fmt"

	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/theme"
	"github.com/dyike/keel/ui/window"
	"github.com/openmodu/onecatch/internal/platform/buildinfo"
	"github.com/openmodu/onecatch/internal/service/appupdate"
)

type desktopUpdater interface {
	Check(context.Context) (appupdate.Status, error)
	Download(context.Context) (appupdate.Status, error)
	Apply(context.Context) error
	Close()
}

type appUpdates struct {
	service          desktopUpdater
	status           appupdate.Status
	busy             bool
	err              string
	dismissedVersion string
}

type nativeUpdateHost struct{ app *app }

func (h nativeUpdateHost) Quit() {
	window.Quit()
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

func (a *app) openUpdates() {
	a.openSettings()
	a.settingsPanel.section = "updates"
}

func (a *app) updateAction(action string) {
	if a.updates.busy || a.updates.service == nil {
		return
	}
	service, ctx := a.updates.service, a.ctx
	a.updates.busy, a.updates.err = true, ""
	post := a.postUpdate
	if post == nil {
		post = core.Update
	}
	go func() {
		var status appupdate.Status
		var err error
		switch action {
		case "check":
			status, err = service.Check(ctx)
		case "download":
			status, err = service.Download(ctx)
		case "apply":
			err = service.Apply(ctx)
		}
		post(func() {
			if a.closed {
				return
			}
			a.updates.busy = false
			if action != "apply" {
				a.updates.status = status
			}
			if err != nil {
				a.updates.err = err.Error()
			}
		})
	}()
}

func (a *app) sidebarFooter(cx *el.Context) el.Element {
	return el.Div().WFull().Gap(6).Child(a.updateNotice(cx), fullMenu(cx, a.ui.Footer))
}

func (a *app) updateNotice(cx *el.Context) el.Element {
	s := a.updates.status
	if s.AvailableVersion == "" || s.AvailableVersion == a.updates.dismissedVersion || s.State != "available" && s.State != "ready" {
		return el.Div()
	}
	label := fmt.Sprintf(uiText("发现新版本 %s"), s.AvailableVersion)
	update := button(label, a.openUpdates, kit.ButtonSecondary).Render(cx).(*el.DivEl).ID("sidebar-update").Name(label).Grow().W(el.Dp(0))
	return el.Div().Row().WFull().Items(el.Center).Gap(4).Child(
		update,
		iconAction(cx, "dismiss-update", "暂不更新", "x", func() { a.updates.dismissedVersion = s.AvailableVersion }, false),
	)
}

func (p *settingsPage) updateSettings(cx *el.Context) el.Element {
	a, s := p.app, p.app.updates.status
	version := s.CurrentVersion
	if version == "" {
		version = buildinfo.Version
	}
	root := el.Div().WFull().Gap(16).Child(el.Text("OneCatch").TextSize(20).Bold(), el.Text(fmt.Sprintf(uiText("当前版本：%s"), version)))
	message := uiText("启动后自动检查更新，也可以手动检查。")
	switch s.State {
	case "up-to-date":
		message = uiText("已是最新版本")
	case "available", "ready":
		message = fmt.Sprintf(uiText("发现新版本 %s"), s.AvailableVersion)
	}
	if !s.VerificationEnabled {
		message = uiText("此构建未启用自动更新，请从发布页下载正式版本。")
	}
	root.Child(el.Text(message).TextColor(theme.Muted))
	controls := el.Div().WrapFit().Items(el.Center).Gap(8)
	check := button("检查更新", func() { a.updateAction("check") }, kit.ButtonSecondary).Loading(a.updates.busy)
	check.SetDisabled(a.updates.busy || !s.VerificationEnabled || a.updates.service == nil)
	controls.Child(check.Render(cx))
	if s.State == "available" && s.AutomaticSupported {
		download := button("下载更新", func() { a.updateAction("download") }, kit.ButtonPrimary)
		download.SetDisabled(a.updates.busy)
		controls.Child(download.Render(cx))
	}
	if s.State == "ready" && s.AutomaticSupported {
		install := button("重启并安装", func() { a.updateAction("apply") }, kit.ButtonPrimary)
		install.SetDisabled(a.updates.busy)
		controls.Child(install.Render(cx))
		root.Child(el.Text(uiText("安装会退出应用，请先结束正在运行的任务。")).TextSize(13).TextColor(theme.Muted))
	}
	controls.Child(button("打开发布页", func() { a.fail(core.OpenURL("https://github.com/openmodu/onecatch/releases/latest")) }, kit.ButtonGhost).Render(cx))
	root.Child(controls)
	if a.updates.busy {
		root.Child(el.Text(uiText("正在处理更新…")).TextSize(13).TextColor(theme.Muted))
	}
	errText := a.updates.err
	if errText == "" {
		errText = s.Error
	}
	if errText != "" {
		root.Child(el.Text(errText).TextSize(13).TextColor(theme.Danger))
	}
	if s.Notes != "" {
		root.Child(el.Text(uiText("更新说明")).Bold(), el.Text(s.Notes).TextSize(13))
	}
	return root
}
