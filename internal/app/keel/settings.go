package keeldesktop

import (
	"context"
	"fmt"
	"maps"
	"strconv"
	"strings"
	"time"

	"github.com/dyike/keel/gio/io/key"
	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/theme"
	domainsettings "github.com/openmodu/onecatch/internal/domain/settings"
)

type settingsPage struct {
	numbers                map[string]*kit.NumberInputView
	app                    *app
	runtime                *kit.SelectView
	enabled                *filledToggle
	binary, model, status  string
	statusError            string
	saving                 bool
	initialized            bool
	section                string
	view                   *kit.SettingsView
	viewModel, viewSection string
	draft                  domainsettings.Settings
	fields                 map[string]*string
	selects                map[string]*kit.SelectView
	switches               map[string]*filledToggle
	workers                workerSettingsState
	mobile                 mobileAccessState
	skillTargets           skillTargetSettings
	actions                settingsActions
	confirm                *kit.DialogView
	currentRuntime         string
	harnesses              map[string]*settingsHarness
	harnessExpanded        map[string]bool
}

func (a *app) openSettings() {
	if a.settingsPanel != nil {
		return
	}
	p := &settingsPage{app: a, runtime: kit.Select("").Hint("选择 Agent").Appearance(false), enabled: newFilledToggle("启用此 Agent", true, true)}
	for _, r := range a.runtimes {
		p.runtime.SetEntries(append(p.runtime.Entries(), kit.SelectOption{Value: r.ID, Label: r.Name})...)
	}
	p.runtime.OnChange(p.selectRuntime)
	p.runtime.SetValue(a.runtime.Value())
	p.selectRuntime(p.runtime.Value())
	p.workers.ctx, p.workers.cancel = context.WithCancel(a.ctx)
	p.watchWorkers()
	p.watchMobileAccess()
	p.loadMobileAccess()
	a.settingsPanel = p
}

func (a *app) closeSettings() {
	a.ui.WorkflowFullscreen = false
	if a.settingsPanel != nil {
		a.settingsPanel.closeWorkers()
		a.settingsPanel = nil
	}
}
func (a *app) settingsOverlay(cx *el.Context) {
	p := a.settingsPanel
	if p == nil {
		return
	}
	width, height := cx.ViewportSize()
	// Only the workflow canvas may take over the window; Esc first leaves full
	// screen and returns to the Settings dialog.
	fullscreen := a.workflowFullscreen()
	panel := el.Div().Role("dialog").Name("设置").W(el.Dp(min(1120, max(0, width-64)))).H(el.Dp(min(760, max(0, height-128)))).Bg(theme.Surface).Rounded(16).Items(el.Stretch)
	if fullscreen {
		panel = el.Div().Role("dialog").Name("编排工作流").W(el.Dp(width)).H(el.Dp(height)).Bg(theme.Surface).Items(el.Stretch).Pt(40).Px(16)
	}
	cx.Overlay("settings-modal", el.Modal(panel).Owner("settings-modal").Scrim(true).KeepOnOutsidePress().BeforeDismiss(func() bool {
		if a.workflowFullscreen() {
			a.ui.WorkflowFullscreen = false
			return false
		}
		return !p.saving
	}).OnDismiss(a.closeSettings))
	if fullscreen {
		panel.Child(a.ui.WorkflowConfirm.Render(cx), a.workflowSettings(cx))
		return
	}
	panel.Child(p.Render(cx))
}

func (p *settingsPage) selectRuntime(id string) {
	if p.currentRuntime != "" && p.draft.Runtimes != nil {
		p.draft.Runtimes[p.currentRuntime] = p.runtimeDraft(p.currentRuntime)
	}
	r := p.app.settings.Runtimes[id]
	if p.draft.Runtimes != nil {
		r = p.draft.Runtimes[id]
	}
	p.currentRuntime = id
	p.binary, p.model, p.status = r.Binary, r.DefaultModel, ""
	p.enabled.SetValue(r.Enabled)
	p.initialized = p.app.settings.SchemaVersion > 0
}

func (p *settingsPage) Render(cx *el.Context) el.Element {
	p.commitBlurredSkillPaths(cx)
	a := p.app
	if p.section == "experimental" {
		cx.Shortcut("ctrl+alt+s", func() { p.workers.unlocked = true; p.checkEnabledWorkers() })
	}
	if p.fields == nil {
		p.fields, p.selects, p.switches = map[string]*string{}, map[string]*kit.SelectView{}, map[string]*filledToggle{}
		p.viewModel = ""
		p.draft = a.settings
		p.draft.Runtimes = maps.Clone(a.settings.Runtimes)
	}
	if len(p.runtime.Entries()) != len(a.runtimes) {
		options := make([]kit.SelectOption, 0, len(a.runtimes))
		for _, r := range a.runtimes {
			options = append(options, kit.SelectOption{Value: r.ID, Label: r.Name})
		}
		p.runtime.SetEntries(options...)
		if p.runtime.Value() == "" && len(a.runtimes) > 0 {
			p.runtime.SetValue(a.runtimes[0].ID)
			p.selectRuntime(p.runtime.Value())
		}
	}
	if p.draft.SchemaVersion == 0 && a.settings.SchemaVersion > 0 {
		p.viewModel = ""
		p.draft = a.settings
		p.draft.Runtimes = maps.Clone(a.settings.Runtimes)
		p.selectRuntime(p.runtime.Value())
	}
	if p.section == "storage" && !p.actions.usageRequested && p.actions.busy == "" {
		p.actions.usageRequested = true
		p.refreshStorageUsage()
	}
	p.ensureSettingsView()
	w, _ := cx.ViewportSize()
	panelWidth := min(float32(1120), max(float32(0), w-64))
	p.view.AvailableWidth(panelWidth)
	p.syncSettingsSection()
	palette := theme.Current()
	palette.Bg = theme.Surface
	palette.Border = seamColor
	settings := cx.Themed(palette, p.view)
	p.syncSettingsSection()
	content := el.Div().Grow().H(el.Dp(0)).Items(el.Stretch).Disabled(p.saving).Child(settings)
	root := el.Div().WFull().HFull().Bg(theme.Surface).Rounded(16).Items(el.Stretch).OnKey(func(e el.KeyEvent) bool {
		// Sidebar handles focused keys before window shortcuts. Let its
		// unhandled unlock chord bubble to the settings window as well.
		if p.section != "experimental" || e.Name != "S" || e.Modifiers != key.ModCtrl|key.ModAlt {
			return false
		}
		if e.State == el.KeyPress {
			p.workers.unlocked = true
			p.checkEnabledWorkers()
		}
		return true
	}).Child(
		p.workerOverlays(cx),
		p.settingsConfirmation(cx))

	root.Child(content)
	if p.status != "" {
		message := p.status
		cx.After(struct {
			Page    *settingsPage
			Message string
		}{p, message}, 5*time.Second, func() {
			if p.status == message {
				p.status = ""
			}
		})
		tone := kit.ToneInfo
		if p.status == p.statusError {
			tone = kit.ToneDanger
		} else if strings.HasPrefix(p.status, "请") || strings.Contains(p.status, "失败") || strings.Contains(p.status, "重新预览") {
			tone = kit.ToneWarning
		}
		notice := kit.Alert(uiText(p.status)).Size(kit.AlertSizeSmall).Tone(tone).OnClose(func() { p.status = "" })
		inset := float32(24)
		if panelWidth >= 800 {
			inset = 238
		}
		root.Child(el.Div().Pl(inset).Pr(24).Py(8).NoShrink().Child(notice.Render(cx)))
	}
	if panelWidth >= 800 {
		root.Child(el.Div().Absolute().Left(210).Top(0).Bottom(0).W(el.Dp(1)).Bg(seamColor))
	}
	if p.section == "experimental" && p.workerSettingsDirty() {
		root.Child(p.workerSettingsSaveBar(cx))
	}
	if p.section != "experimental" && p.section != "mobile" && p.section != "skills" && p.section != "workflows" && p.section != "updates" || p.status != "" {
		root.Child(p.settingsFooter(cx, p.section).(*el.DivEl).Px(24).Py(12).NoShrink())
	}
	root.Child(el.Div().Absolute().Top(16).Right(16).Child(iconAction(cx, "close-settings", "关闭设置", "x", func() {
		if !p.saving {
			a.closeSettings()
		}
	}, false)))
	return root
}

var settingsSections = []struct{ ID, Label, Description, Category string }{
	{"", "外观", "语言、显示与主题", "Settings"},
	{"harness", "Harness", "配置各 Harness 的 Agent", "Settings"},
	{"skills", "Skills", "Skill 同步目标与目录", "Settings"},
	{"security", "安全", "授权与诊断", "Settings"},
	{"storage", "存储与日志", "本机数据管理", "Settings"},
	{"mobile", "手机连接", "把这台电脑作为手机端的远程 Worker", "远程连接"},
	{"experimental", "远端 Worker", "敬请期待", "远程连接"},
	{"execution", "执行", "新流程默认值", "工作流"},
	{"workflows", "工作流管理", "创建、编辑与预览工作流", "工作流"},
	{"updates", "软件更新", "版本、更新说明与安装", "OneCatch"},
}

func (p *settingsPage) text(id, hint, initial string) el.Element {
	if p.fields[id] == nil {
		value := initial
		p.fields[id] = &value
	}
	return field("settings/"+id, hint, p.fields[id]).WFull().MinH(el.Dp(36))
}
func (p *settingsPage) choice(cx *el.Context, id, value string, values []string) el.Element {
	s := p.selects[id]
	if s == nil {
		s = kit.Select("").Hint("默认").Appearance(false).Size(36)
		if id == "default-sandbox" {
			s = newAccessSelect().MenuAlign(el.End)
		}

		if value == "" {
			value = "__default"
		}
		s.SetValue(value)
		p.selects[id] = s
	}
	// Refresh labels when the interface language changes without resetting drafts.
	var options []kit.SelectOption
	for _, value := range values {
		label := settingsOptionLabel(id, value)
		if value == "" {
			value = "__default"
		}
		options = append(options, kit.SelectOption{Value: value, Label: label})
	}
	s.SetEntries(options...)

	return s.Render(cx)
}
func (p *settingsPage) value(id string, fallback string) string {
	if v := p.fields[id]; v != nil {
		return strings.TrimSpace(*v)
	}
	return fallback
}
func (p *settingsPage) selected(id, fallback string) string {
	if v := p.selects[id]; v != nil {
		if v.Value() == "__default" {
			return ""
		}
		return v.Value()
	}
	return fallback
}
func (p *settingsPage) on(id string, fallback bool) bool {
	if v := p.switches[id]; v != nil {
		return v.Value()
	}
	return fallback
}
func (p *settingsPage) draftForSave() (domainsettings.Settings, error) {
	d := p.draft
	d.Runtimes = maps.Clone(d.Runtimes)
	number := func(id string, target *int) error {
		if v := p.fields[id]; v != nil {
			value, err := strconv.Atoi(strings.TrimSpace(*v))
			if err != nil {
				return fmt.Errorf("%s：请输入整数", id)
			}
			*target = value
		}
		return nil
	}
	switch p.section {
	case "harness":
		for id := range d.Runtimes {
			d.Runtimes[id] = p.runtimeDraft(id)
		}
	case "terminal":
		d.Terminal.Shell = p.value("shell", d.Terminal.Shell)
		d.Terminal.Theme = p.selected("terminal-theme", d.Terminal.Theme)
		if v := p.fields["shell-args"]; v != nil {
			d.Terminal.Arguments = strings.Split(*v, "\n")
		}
	case "execution":
		for _, f := range []struct {
			id     string
			target *int
		}{{"max-rework-rounds", &d.Execution.MaxReworkRounds}, {"max-failures", &d.Execution.MaxConsecutiveFailures}, {"step-timeout", &d.Execution.StepTimeoutSeconds}, {"max-concurrency", &d.Execution.MaxConcurrency}, {"interrupt-grace", &d.Execution.InterruptGraceSeconds}} {
			if err := number(f.id, f.target); err != nil {
				return d, err
			}
		}
		d.Execution.DefaultSandbox = p.selected("default-sandbox", d.Execution.DefaultSandbox)
	case "security":
		d.Security.AllowFullSandbox = p.on("allow-full", d.Security.AllowFullSandbox)
		d.Security.ConfirmFullSandboxEveryRun = p.on("confirm-full", d.Security.ConfirmFullSandboxEveryRun)
		d.Security.DiagnosticsIncludePrompt = p.on("diagnostics-prompt", d.Security.DiagnosticsIncludePrompt)
		d.Security.DiagnosticsIncludeRawEvents = p.on("diagnostics-events", d.Security.DiagnosticsIncludeRawEvents)
	case "storage":
		for _, f := range []struct {
			id     string
			target *int
		}{{"retention-days", &d.Storage.CompletedRunRetentionDays}, {"log-size", &d.Storage.LogMaxSizeMB}, {"log-backups", &d.Storage.LogMaxBackups}, {"log-age", &d.Storage.LogMaxAgeDays}} {
			if err := number(f.id, f.target); err != nil {
				return d, err
			}
		}
		d.Storage.LogLevel = p.selected("log-level", d.Storage.LogLevel)
	case "experimental":
		d.Experimental.RemoteWorkersEnabled = p.on("remote-workers", d.Experimental.RemoteWorkersEnabled)
	}
	return d, nil
}

func (p *settingsPage) saveSection() {
	if p.saving || !p.initialized {
		return
	}
	d, err := p.draftForSave()
	if err != nil {
		p.setStatusError(err)
		return
	}
	section := p.section
	runtimeID := p.runtime.Value()
	p.saving = true
	p.status = "正在保存…"
	go func() {
		var result domainsettings.Settings
		var err error
		s := p.app.backend.service
		switch section {
		case "harness":
			result, err = s.UpdateRuntimeSettings(p.app.ctx, d.Runtimes, d.Revision)
		case "terminal":
			result, err = s.UpdateTerminalSettings(p.app.ctx, d.Terminal, d.Revision)
		case "execution":
			result, err = s.UpdateExecutionSettings(p.app.ctx, d.Execution, d.Revision)
		case "security":
			result, err = s.UpdateSecuritySettings(p.app.ctx, d.Security, d.Revision)
		case "storage":
			result, err = s.UpdateStorageSettings(p.app.ctx, d.Storage, d.Revision)
		case "experimental":
			result, err = s.UpdateExperimentalSettings(p.app.ctx, d.Experimental, d.Revision)
		}
		var probeErr error
		if section == "harness" && err == nil {
			_, probeErr = s.CheckRuntime(runtimeID)
		}
		core.Update(func() {
			if p.app.closed {
				return
			}
			p.saving = false
			if err != nil {
				p.setStatusError(err)
				return
			}
			p.draft = result
			p.draft.Runtimes = maps.Clone(result.Runtimes)
			p.app.settings = result
			p.status = "已保存"
			if probeErr != nil {
				p.status = "已保存；检测失败：" + probeErr.Error()
			}
			p.app.refresh()
		})
	}()
}
func (p *settingsPage) reload() {
	if p.saving {
		return
	}
	p.saving = true
	go func() {
		value, err := p.app.backend.service.GetSettings(p.app.ctx)
		core.Update(func() {
			if p.app.closed {
				return
			}
			p.saving = false
			if err != nil {
				p.setStatusError(err)
				return
			}
			p.app.settings = value
			p.draft = value
			p.draft.Runtimes = maps.Clone(value.Runtimes)
			p.fields, p.selects, p.switches = nil, nil, nil
			p.currentRuntime = ""
			p.selectRuntime(p.runtime.Value())
			p.status = "已重新加载"
		})
	}()
}

func settingsOptionLabel(id, value string) string {
	if value == "" {
		return uiText("默认")
	}
	if id == "default-sandbox" {
		switch value {
		case "read-only":
			return uiText("只读")
		case "workspace-write":
			return uiText("工作区访问")
		case "full":
			return uiText("完全访问")
		}
	}
	if id == "terminal-theme" {
		switch value {
		case "system":
			return uiText("跟随系统")
		case "paper":
			return uiText("浅色")
		case "midnight":
			return uiText("深色")
		case "contrast":
			return uiText("高对比度")
		}
	}
	return value
}

func (p *settingsPage) setStatusError(err error) {
	p.status = err.Error()
	p.statusError = p.status
}
