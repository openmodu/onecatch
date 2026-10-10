package keeldesktop

import (
	"context"
	"fmt"
	"maps"
	"runtime"
	"strconv"
	"strings"

	"gioui.org/io/key"
	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/theme"
	"github.com/dyike/keel/ui/window"
	domainsettings "github.com/openmodu/onecatch/internal/domain/settings"
)

type settingsPage struct {
	app                    *app
	runtime                *kit.SelectView
	enabled                *filledToggle
	binary, model, status  string
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
}

func (a *app) openSettings() {
	if a.settingsWindow != nil && !a.settingsWindow.Closed() {
		a.settingsWindow.Raise()
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
	a.settingsWindow = window.Open(window.Options{Title: "OneCatch · 设置", Width: 960, Height: 800, MinWidth: 860, MinHeight: 600, Frameless: runtime.GOOS == "darwin", NativeTrafficLights: runtime.GOOS == "darwin", TrafficLightLayout: &window.TrafficLightLayout{Height: 52, Left: 20}, Content: el.Root(p), OnClose: func() { p.closeWorkers(); a.settingsWindow = nil }})
}

func (p *settingsPage) selectRuntime(id string) {
	r := p.app.settings.Runtimes[id]
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
	p.ensureSettingsView()
	p.syncSettingsSection()
	settings := p.view.Render(cx)
	p.syncSettingsSection()
	content := el.Div().Grow().H(el.Dp(0)).Items(el.Stretch).Disabled(p.saving).Child(settings)
	root := el.Div().WFull().HFull().Bg(theme.Bg).Items(el.Stretch).OnKey(func(e el.KeyEvent) bool {
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
		dragArea(cx, el.Div().H(el.Dp(52)).WFull().Center().Child(el.Text("设置").TextSize(14).Medium())), content)
	if p.section == "experimental" && p.workerSettingsDirty() {
		root.Child(p.workerSettingsSaveBar(cx))
	}
	if p.section != "experimental" && p.section != "mobile" && p.section != "skills" || p.status != "" {
		root.Child(p.settingsFooter(cx, p.section).(*el.DivEl).Px(24).Py(12).NoShrink())
	}
	return root
}

var settingsSections = []struct{ ID, Label, Description string }{
	{"", "外观", "语言、显示与主题"}, {"harness", "Harness", "配置各 Harness 的 Agent"}, {"skills", "Skills", "Skill 同步目标与目录"},
	{"mobile", "手机连接", "把这台电脑作为手机端的远程 Worker"}, {"terminal", "终端", "配置 Shell 与终端外观"}, {"execution", "执行", "新流程默认值"}, {"security", "安全", "授权与诊断"}, {"storage", "存储与日志", "本机数据管理"},
	{"experimental", "远端 Worker", "敬请期待"},
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
		var options []kit.SelectOption
		for _, v := range values {
			label := v
			if v == "" {
				label = "默认"
				v = "__default"
			}
			options = append(options, kit.SelectOption{Value: v, Label: label})
		}
		s.SetEntries(options...)
		if value == "" {
			value = "__default"
		}
		s.SetValue(value)
		p.selects[id] = s
	}
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
		id := p.runtime.Value()
		r := d.Runtimes[id]
		r.Enabled = p.enabled.Value()
		r.Binary = strings.TrimSpace(p.binary)
		r.DefaultModel = strings.TrimSpace(p.model)
		r.ReasoningEffort = p.selected("runtime-effort/"+id, r.ReasoningEffort)
		r.Provider = p.selected("runtime-provider/"+id, r.Provider)
		r.ServiceTier = p.selected("runtime-tier/"+id, r.ServiceTier)
		r.Integration = p.selected("runtime-integration/"+id, r.Integration)
		r.ConfigSource = p.selected("runtime-source/"+id, r.ConfigSource)
		r.ConfigPath = p.value("runtime-config/"+id, r.ConfigPath)
		r.RemoteFSEnabled = p.on("runtime-remotefs/"+id, r.RemoteFSEnabled)
		r.MaxContextWindow = p.on("runtime-context/"+id, r.MaxContextWindow)
		if v := p.fields["runtime-env/"+id]; v != nil {
			r.EnvironmentAllowlist = strings.FieldsFunc(*v, func(c rune) bool { return c == ',' || c == '\n' || c == ' ' || c == '\t' })
		}
		d.Runtimes[id] = r
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
		}{{"max-transitions", &d.Execution.MaxTransitions}, {"max-failures", &d.Execution.MaxConsecutiveFailures}, {"step-timeout", &d.Execution.StepTimeoutSeconds}, {"dag-concurrency", &d.Execution.MaxLocalDAGConcurrency}, {"interrupt-grace", &d.Execution.InterruptGraceSeconds}} {
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
		p.status = err.Error()
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
				p.status = err.Error()
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
				p.status = err.Error()
				return
			}
			p.app.settings = value
			p.draft = value
			p.draft.Runtimes = maps.Clone(value.Runtimes)
			p.fields, p.selects, p.switches = nil, nil, nil
			p.selectRuntime(p.runtime.Value())
			p.status = "已重新加载"
		})
	}()
}

func (a *app) toggleTheme() {
	mode := "dark"
	if a.dark {
		mode = "light"
	}
	a.setAppearance(mode, a.ui.Preferences.Accent)
}
