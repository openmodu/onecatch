package keeldesktop

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/theme"
	desktopservice "github.com/openmodu/onecatch/internal/service/desktop"
	"github.com/openmodu/onecatch/internal/service/worker"
)

type workerHealth struct {
	status       desktopservice.WorkerStatus
	checking, ok bool
	err          string
	version      uint64
}

type workerSettingsState struct {
	ctx              context.Context
	cancel           context.CancelFunc
	closed, unlocked bool
	health           map[string]workerHealth
	form             workerForm
	confirm          *kit.DialogView
	version          uint64
}

type workerForm struct {
	input                                worker.UpdateInput
	code, path, err                      string
	kind                                 *kit.SelectView
	enabled, sync                        *filledToggle
	open, busy, tls, deployment, focused bool
}

func (p *settingsPage) workersVisible() bool {
	return p.workers.unlocked || p.app.settings.Experimental.RemoteWorkersEnabled
}

func (p *settingsPage) closeWorkers() {
	p.workers.closed = true
	p.workers.version++
	p.mobile.version++
	p.mobile.status.Pairing = nil
	p.mobile.copied = nil
	if p.workers.cancel != nil {
		p.workers.cancel()
	}
	p.workers.form = workerForm{}
}

func (p *settingsPage) workerContext() context.Context {
	if p.workers.ctx != nil {
		return p.workers.ctx
	}
	return p.app.ctx
}

func (p *settingsPage) watchWorkers() {
	ctx := p.workerContext()
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				core.Update(func() {
					if !p.workers.closed && !p.app.closed && p.section == "experimental" {
						p.checkEnabledWorkers()
					}
				})
			}
		}
	}()
}

func (p *settingsPage) checkEnabledWorkers() {
	if !p.workersVisible() || p.workers.closed || p.app.closed {
		return
	}
	for _, w := range p.app.workers {
		if w.Enabled {
			p.checkWorker(w.ID)
		}
	}
}

func (p *settingsPage) checkWorker(id string) {
	if p.workers.closed || p.app.closed {
		return
	}
	if p.workers.health == nil {
		p.workers.health = map[string]workerHealth{}
	}
	h := p.workers.health[id]
	if h.checking {
		return
	}
	h.checking, h.version = true, h.version+1
	p.workers.health[id] = h
	ctx, cancel := context.WithTimeout(p.workerContext(), 12*time.Second)
	go func() {
		defer cancel()
		status, err := p.app.backend.service.CheckWorker(ctx, id)
		core.Update(func() { p.applyWorkerHealth(id, h.version, status, err) })
	}()
}

func (p *settingsPage) applyWorkerHealth(id string, version uint64, status desktopservice.WorkerStatus, err error) {
	h, exists := p.workers.health[id]
	if p.workers.closed || p.app.closed || !exists || h.version != version {
		return
	}
	h.checking, h.ok, h.status, h.err = false, err == nil, status, ""
	if err != nil {
		h.err = safeWorkerError(err, "")
	}
	p.workers.health[id] = h
}

func (p *settingsPage) invalidateWorkerHealth(id string) {
	if p.workers.health == nil {
		p.workers.health = map[string]workerHealth{}
	}
	h := p.workers.health[id]
	p.workers.health[id] = workerHealth{version: h.version + 1}
}

func workerUpdateInput(w worker.Info) worker.UpdateInput {
	return worker.UpdateInput{ID: w.ID, Name: w.Name, BaseURL: w.BaseURL, Provider: w.Provider, RemotePath: w.RemotePath, SyncLocal: w.SyncLocal, Enabled: w.Enabled, CAFile: w.CAFile, ClientCertFile: w.ClientCertFile, ClientKeyFile: w.ClientKeyFile, ServerName: w.ServerName, ServerCertificateSHA256: w.ServerCertificateSHA256}
}

func (p *settingsPage) openWorker(w *worker.Info) {
	if p.workers.form.busy || p.workers.closed {
		return
	}
	p.workers.version++
	f := workerForm{open: true, path: "/home/gem", kind: kit.Select("").Appearance(false).Size(36)}
	f.kind.SetEntries(kit.SelectOption{Value: "sandbox", Label: "volc sandbox"}, kit.SelectOption{Value: "worker", Label: "普通远端 Worker"})
	f.kind.SetValue("sandbox")
	f.kind.RenderValue(func(cx *el.Context, entries []kit.SelectOption) el.Element {
		return el.Text(entries[0].Label).TextSize(14).Medium().MaxLines(1)
	})
	f.input.BaseURL, f.input.Enabled = "https://", true
	if w != nil {
		f.input = workerUpdateInput(*w)
		if w.Provider != worker.ProviderVolcengineSandbox {
			f.kind.SetValue("worker")
		}
		if w.RemotePath != "" {
			f.path = w.RemotePath
		}
	}
	f.enabled = newFilledToggle("启用调度", f.input.Enabled, false)
	f.sync = newFilledToggle("同步本地项目", f.input.SyncLocal, false)
	f.enabled.hideLabel, f.sync.hideLabel = true, true
	p.workers.form = f
}

func (f *workerForm) sandbox() bool {
	if f.input.ID != "" {
		return f.input.Provider == worker.ProviderVolcengineSandbox
	}
	return f.kind != nil && f.kind.Value() == "sandbox"
}

func (f *workerForm) canSave() bool {
	if f.busy {
		return false
	}
	if f.input.ID != "" {
		return strings.TrimSpace(f.input.Name) != "" && strings.TrimSpace(f.input.BaseURL) != ""
	}
	if f.sandbox() {
		return worker.IsVolcengineSandboxURL(f.input.BaseURL) && strings.HasPrefix(strings.TrimSpace(f.path), "/")
	}
	return strings.TrimSpace(f.input.BaseURL) != "" && strings.TrimSpace(f.code) != ""
}

// Never display a gateway credential in transport errors. The editable URL
// field is owned by the user; status messages only expose the failure itself.
func safeWorkerError(err error, rawURL string) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	if rawURL != "" {
		message = strings.ReplaceAll(message, rawURL, "[Worker URL]")
	}
	for _, part := range strings.Fields(message) {
		candidate := strings.Trim(part, "\"'(),:")
		if u, e := url.Parse(candidate); e == nil && u.Host != "" && (u.RawQuery != "" || u.User != nil) {
			message = strings.ReplaceAll(message, candidate, "[Worker URL]")
		}
	}
	return message
}

func saveWorkerConnection(ctx context.Context, service *desktopservice.Service, f workerForm) (worker.Info, error) {
	if !f.canSave() {
		return worker.Info{}, fmt.Errorf("请填写连接地址和配对信息")
	}
	if f.input.ID != "" {
		input := f.input
		input.Name, input.BaseURL = strings.TrimSpace(input.Name), strings.TrimSpace(input.BaseURL)
		input.Enabled, input.SyncLocal = f.enabled.Value(), f.sync.Value()
		if f.sandbox() {
			input.RemotePath = strings.TrimSpace(f.path)
		}
		return service.UpdateWorker(ctx, input)
	}
	code := strings.TrimSpace(f.code)
	if f.sandbox() {
		code = strings.TrimSpace(f.path)
	}
	info, err := service.PairWorker(ctx, strings.TrimSpace(f.input.BaseURL), code)
	if err != nil || !f.sandbox() || !f.sync.Value() {
		return info, err
	}
	input := workerUpdateInput(info)
	input.SyncLocal = true
	updated, err := service.UpdateWorker(ctx, input)
	if err != nil {
		return info, err
	} // Pairing already persisted; don't consume the one-time code twice.
	return updated, nil
}

func (p *settingsPage) saveWorker() {
	f := p.workers.form
	if !f.canSave() || p.workers.closed || p.app.closed {
		return
	}
	p.workers.form.busy, p.workers.form.err = true, ""
	version := p.workers.version
	ctx, cancel := context.WithTimeout(p.workerContext(), 30*time.Second)
	go func() {
		defer cancel()
		info, err := saveWorkerConnection(ctx, p.app.backend.service, f)
		items, listErr := p.app.backend.service.ListWorkers(ctx)
		core.Update(func() {
			if p.workers.closed || p.app.closed || version != p.workers.version {
				return
			}
			p.workers.form.busy = false
			if listErr == nil {
				p.app.workers = items
				p.app.buildExecutorMenu()
				p.app.followActiveInspectorSource()
			}
			if info.ID != "" {
				p.invalidateWorkerHealth(info.ID)
			}
			if err != nil {
				if info.ID != "" {
					p.openWorker(&info)
				}
				p.workers.form.err = safeWorkerError(err, f.input.BaseURL)
				return
			}
			if listErr != nil {
				p.workers.form.err = safeWorkerError(listErr, f.input.BaseURL)
				return
			}
			p.closeWorkerForm()
			p.status = "已保存环境"
			if info.Enabled {
				p.checkWorker(info.ID)
			}
		})
	}()
}

func (p *settingsPage) closeWorkerForm() {
	if p.workers.form.busy {
		return
	}
	p.workers.version++
	p.workers.form = workerForm{}
}

func (p *settingsPage) deleteWorker(w worker.Info) {
	if p.workers.confirm == nil {
		p.workers.confirm = kit.Dialog("").Width(420)
	}
	p.workers.confirm.ConfirmDanger("删除 Worker “"+workerLabel(w)+"”？", "只移除这台 Worker 的配置，不会删除远端文件。", "删除 Worker", func() {
		p.invalidateWorkerHealth(w.ID)
		ctx := p.workerContext()
		go func() {
			err := p.app.backend.service.DeleteWorker(ctx, w.ID)
			items, listErr := p.app.backend.service.ListWorkers(ctx)
			core.Update(func() {
				if p.workers.closed || p.app.closed {
					return
				}
				if err != nil {
					p.status = safeWorkerError(err, "")
					return
				}
				if listErr != nil {
					p.status = safeWorkerError(listErr, "")
					return
				}
				p.app.workers = items
				p.app.buildExecutorMenu()
				p.app.followActiveInspectorSource()
				p.status = "已删除环境"
			})
		}()
	})
}

func workerSwitch(cx *el.Context, label, description string, toggle *filledToggle) el.Element {
	text := el.Div().Grow().W(el.Dp(0)).Gap(4).Child(el.Text(label).TextSize(14).Bold().LineHeight(1.4))
	if description != "" {
		text.Child(el.Text(description).TextSize(12).LineHeight(1.5).TextColor(theme.Muted))
	}
	return el.Div().Row().Items(el.Center).Gap(16).Px(16).Py(14).Rounded(12).Bg(mixColor(theme.Subtle, theme.Surface, .45)).Child(text, toggle.Render(cx))
}

func (p *settingsPage) workerSettings(cx *el.Context) el.Element {
	if !p.workersVisible() {
		return el.Div().Gap(28).Child(el.Div().P(16).Rounded(8).Bg(theme.Subtle).Gap(6).Child(el.Text("敬请期待").TextSize(11).TextColor(theme.Muted), el.Text("远端 Worker 正在开发中").TextSize(14).Medium().TextColor(theme.Muted), el.Text("当前版本仅展示功能入口，暂不支持启用、配置或管理。").TextSize(12).TextColor(theme.Muted)), el.Div().Gap(12).Child(el.Text("远端 Worker 调度").TextSize(14).Medium().TextColor(theme.Muted), el.Text("当前版本仅展示功能入口，暂不支持启用、配置或管理。").TextSize(12).TextColor(theme.Muted)))
	}
	toggle := p.switches["remote-workers"]
	if toggle == nil {
		toggle = newFilledToggle("启用远端 Worker", p.draft.Experimental.RemoteWorkersEnabled, false)
		toggle.hideLabel = true
		p.switches["remote-workers"] = toggle
	}
	root := el.Div().Gap(24).Child(el.Div().Gap(14).Child(el.Text("远端执行").TextSize(15).Bold().LineHeight(1.25), el.Div().P(16).Bg(theme.Surface).Rounded(8).Child(workerSwitch(cx, "启用远端 Worker", "在新对话和工作流中使用远端环境。", toggle))))
	section := el.Div().Gap(14).Child(el.Div().Row().Items(el.Center).Child(el.Text("配置环境").TextSize(15).Bold().LineHeight(1.25), el.Div().Grow(), button("添加环境", func() { p.openWorker(nil) }, kit.ButtonPrimary).Size(32).Render(cx)))
	list := el.Div().Bg(theme.Surface).Rounded(8)
	if len(p.app.workers) == 0 {
		list.Child(el.Text("暂无环境").Px(16).Py(24).TextSize(14).TextColor(theme.Muted))
	}
	for i, info := range p.app.workers {
		w := info
		h := p.workers.health[w.ID]
		label, description := "未连接", "普通远端 Worker"
		if !w.Enabled {
			label = "已停用"
		} else if h.ok {
			label = "已连接"
		} else if h.checking {
			label = "正在检查…"
		}
		if w.Provider == worker.ProviderVolcengineSandbox {
			description = "volc sandbox"
			if w.SyncLocal {
				description = "本地项目 · 按轮同步"
			}
		}
		row := el.Div().Row().Wrap().Items(el.Center).Gap(12).Px(16).Py(16)
		row.Child(el.Div().Grow().MinW(el.Dp(120)).Gap(4).Child(el.Text(workerLabel(w)).TextSize(14).Medium().LineHeight(1.4), el.Text(description).TextSize(12).LineHeight(1.4).TextColor(theme.Muted)))
		color := theme.Muted
		if w.Enabled && h.ok {
			color = theme.Success
		}
		row.Child(el.Text(label).TextSize(12).TextColor(color))
		check := workerActionButton("测试连接", func() { p.checkWorker(w.ID) }).Name("测试连接 " + workerLabel(w))
		check.SetDisabled(h.checking)
		remove := workerActionButton("删除", func() { p.deleteWorker(w) }).Name("删除 " + workerLabel(w)).Appearance(func(ap kit.ButtonAppearance) kit.ButtonAppearance {
			ap.Foreground = theme.Danger
			ap.Border, ap.Focus = theme.Border, theme.Border
			ap.Background = mixColor(theme.Danger, theme.Surface, .04)
			return ap
		})
		row.Child(check.Render(cx), workerActionButton("编辑", func() { p.openWorker(&w) }).Name("编辑 "+workerLabel(w)).Render(cx), remove.Render(cx))
		list.Child(row)
		if h.err != "" {
			list.Child(el.Text(h.err).TextSize(12).TextColor(theme.Danger).Px(16))
		}
		if i < len(p.app.workers)-1 {
			list.Child(el.Div().H(el.Dp(1)).WFull().Bg(seamColor))
		}
	}
	return root.Child(section.Child(list))
}

func (p *settingsPage) workerOverlays(cx *el.Context) el.Element {
	root := el.Div().ID("worker-connection").Absolute().Size(el.Dp(0))
	if p.workers.confirm != nil {
		root.Child(p.workers.confirm.Render(cx))
	}
	f := &p.workers.form
	if !f.open {
		return root
	}
	w, h := cx.ViewportSize()
	title, action := "配置环境", "配对并保存"
	if f.sandbox() {
		action = "连接沙箱"
	}
	if f.input.ID != "" {
		title, action = "远端 Worker", "保存修改"
	}
	panel := el.Div().Role("dialog").Name(title).W(el.Dp(576)).MaxW(el.Dp(w - 48)).MaxH(el.Dp(h - 48)).Rounded(8).Shadow(theme.ElevationLg).Bg(theme.Bg).Child(el.Text(title).TextSize(18).Bold().LineHeight(1).Px(24).Pt(24).Pb(16))
	body := el.Div().LineHeight(1.375).MaxH(el.Dp(h - 190)).ScrollY().Px(24).Pb(16).Gap(16).Disabled(f.busy)
	input := func(id, label, placeholder string, value *string) *el.InputEl {
		return field("worker/"+id, placeholder, value).Name(label).WFull().H(el.Dp(36)).TextSize(14).Medium().OnSubmit(func(string) { p.saveWorker() })
	}
	if f.input.ID == "" {
		f.kind.SetDisabled(f.busy)
		body.Child(workerField("环境类型", el.Div().Bg(theme.Subtle).Rounded(8).Child(f.kind.Render(cx))))
		label, placeholder := "基础 URL", "https://192.168.1.20:9231"
		if f.sandbox() {
			label, placeholder = "沙箱连接 URL", "https://…volceapi.com/?faasInstanceName=…&Authorization=…"
		}
		group := workerField(label, input("url", label, placeholder, &f.input.BaseURL).OnChange(func(value string) {
			if worker.IsVolcengineSandboxURL(value) {
				f.kind.SetValue("sandbox")
			}
		}))
		if f.sandbox() {
			group.Child(el.Text("粘贴包含 faasInstanceName 和 Authorization 的完整 URL，无需配对码。").TextSize(12).TextColor(theme.Muted))
		}
		body.Child(group)
		if !f.sandbox() {
			body.Child(workerField("配对码", input("code", "配对码", "ABCD2345", &f.code).OnChange(func(value string) { f.code = strings.ToUpper(value) })).Child(el.Text("在 B 侧使用 --pair 启动 Worker，然后输入日志中的 8 位一次性配对码").TextSize(12).TextColor(theme.Muted)))
		}
	} else {
		body.Child(workerField("名称", input("name", "名称", "Build Mac mini", &f.input.Name)), workerField("基础 URL", input("url", "基础 URL", "https://192.168.1.20:9231", &f.input.BaseURL)))
		if !f.sandbox() {
			group := el.Div().Gap(12).Child(p.workerDisclosure(cx, "TLS 与设备配对", &f.tls))
			if f.tls {
				group.Child(workerField("服务端证书 SHA-256（可选）", input("fingerprint", "服务端证书 SHA-256（可选）", "worker 启动日志中的 64 位指纹", &f.input.ServerCertificateSHA256)))
				for _, fields := range [][]struct {
					id, label, hint string
					value           *string
				}{
					{{"ca", "CA 文件（可选）", "/path/to/ca.pem", &f.input.CAFile}, {"server-name", "TLS Server Name（可选）", "worker.example.internal", &f.input.ServerName}},
					{{"cert", "mTLS 客户端证书", "/path/to/client.pem", &f.input.ClientCertFile}, {"key", "mTLS 客户端私钥", "/path/to/client-key.pem", &f.input.ClientKeyFile}},
				} {
					row := el.Div().Row().Gap(16)
					for _, field := range fields {
						row.Child(workerField(field.label, input(field.id, field.label, field.hint, field.value)).Grow().W(el.Dp(0)))
					}
					group.Child(row)
				}
			}
			body.Child(group)
		}
	}
	if f.sandbox() {
		body.Child(workerField("沙箱工作目录", input("path", "沙箱工作目录", "/home/gem", &f.path)).Child(el.Text("沙箱内已有目录；开启同步后，在此目录下创建项目副本。").TextSize(12).TextColor(theme.Muted)))
	}
	if f.input.ID != "" {
		body.Child(workerSwitch(cx, "启用调度", "", f.enabled))
	}
	if f.sandbox() {
		body.Child(workerSwitch(cx, "同步本地项目", "执行前同步源码，结束后回传修改。忽略依赖、构建目录及常见凭据文件。", f.sync))
	} else {
		body.Child(p.workerDisclosure(cx, "部署说明", &f.deployment))
		if f.deployment {
			body.Child(el.Div().Gap(8).Child(el.Text(workerDeploymentCommand(f.input.ID)).Mono().TextSize(12).Bg(theme.Subtle).P(12).Rounded(8), button("复制命令", func() { el.WriteClipboard(workerDeploymentCommand(f.input.ID)) }, kit.ButtonGhost).Size(24).Render(cx)))
		}
	}
	if f.err != "" {
		body.Child(el.Text(f.err).TextSize(12).TextColor(theme.Danger))
	}
	if f.busy {
		action = "正在连接…"
	}
	save := button(action, p.saveWorker, kit.ButtonPrimary).Size(32).Content(el.ViewFunc(func(cx *el.Context) el.Element {
		return el.Text(action).TextSize(14).Medium().TextColor(theme.OnColor).DisabledStyle(func(s *el.Style) { s.TextColor(theme.OnColor) })
	}))
	save.SetDisabled(!f.canSave())
	cancel := button("取消", p.closeWorkerForm, kit.ButtonGhost).Size(32)
	cancel.SetDisabled(f.busy)
	saveControl := save.Render(cx)
	if d, ok := saveControl.(*el.DivEl); ok {
		d.DisabledStyle(func(s *el.Style) { s.Bg(mixColor(theme.Primary, theme.Bg, .5)).TextColor(theme.OnColor) })
	}
	panel.Child(body, el.Div().H(el.Dp(1)).Bg(seamColor), el.Div().Row().Justify(el.End).Gap(8).Px(24).Py(16).RoundedCorners(0, 0, 8, 8).Bg(mixColor(theme.Subtle, theme.Bg, .45)).Child(cancel.Render(cx), saveControl))
	cx.Overlay("worker-connection", el.Modal(panel).Owner("worker-connection").BeforeDismiss(func() bool { return !f.busy }).OnDismiss(p.closeWorkerForm))
	if !f.focused {
		target := "worker/url"
		if f.input.ID != "" {
			target = "worker/name"
		}
		cx.Focus(target)
		f.focused = true
	}
	return root
}

func (p *settingsPage) workerDisclosure(cx *el.Context, label string, open *bool) el.Element {
	icon := "chevron-right"
	if *open {
		icon = "chevron-down"
	}
	control := button(label, func() { *open = !*open }, kit.ButtonGhost).Name(label).Size(20).Compact(true).Content(el.ViewFunc(func(cx *el.Context) el.Element {
		return el.Div().Row().Items(el.Center).Gap(6).Child(lucide(cx, icon, 12, theme.Muted), el.Text(label).TextSize(12).TextColor(theme.Muted))
	})).Render(cx)
	if row, ok := control.(*el.DivEl); ok {
		row.Justify(el.Start).Px(0)
	}
	return control
}

func workerField(label string, content el.Element) *el.DivEl {
	return el.Div().Gap(8).Child(el.Text(label).TextSize(12).Medium(), content)
}

func workerActionButton(label string, action func()) *kit.ButtonView {
	return button(label, action, kit.ButtonSecondary).Size(24).Compact(true).Content(el.ViewFunc(func(cx *el.Context) el.Element { return el.Text(label).TextSize(12).Medium() }))
}

func workerDeploymentCommand(id string) string {
	if id == "" {
		id = "remote-worker"
	}
	quoted := "'" + strings.ReplaceAll(id, "'", "'\"'\"'") + "'"
	return strings.Join([]string{"onecatch worker \\", "  --install-service \\", "  --listen 0.0.0.0:9231 \\", "  --id " + quoted + " \\", "  --pair \\", "  --tls-cert '<server.pem>' \\", "  --tls-key '<server-key.pem>'"}, "\n")
}

func (p *settingsPage) workerSettingsDirty() bool {
	toggle := p.switches["remote-workers"]
	return p.workersVisible() && toggle != nil && toggle.Value() != p.app.settings.Experimental.RemoteWorkersEnabled
}

func (p *settingsPage) workerSettingsSaveBar(cx *el.Context) el.Element {
	bar := el.Div().Row().Items(el.Center).Gap(16).P(16).NoShrink().Bg(theme.Subtle).Child(el.Div().Grow().W(el.Dp(0)).Gap(6).Child(el.Text("当前分区有未保存的修改").TextSize(14).Medium(), el.Text("保存后立即应用；不会追溯修改已启动的运行。").TextSize(12).TextColor(theme.Muted)))
	discard := button("放弃", func() {
		p.switches["remote-workers"].SetValue(p.app.settings.Experimental.RemoteWorkersEnabled)
		p.status = ""
	}, kit.ButtonGhost).Size(32)
	save := button("保存", p.saveSection, kit.ButtonPrimary).Size(32).Loading(p.saving)
	discard.SetDisabled(p.saving)
	save.SetDisabled(!p.initialized)
	return bar.Child(discard.Render(cx), save.Render(cx))
}
