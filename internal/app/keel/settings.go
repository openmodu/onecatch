package keeldesktop

import (
	"context"
	"fmt"
	"maps"
	"runtime"
	"strconv"
	"strings"

	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/theme"
	"github.com/dyike/keel/ui/window"
	"github.com/openmodu/onecatch/internal/buildinfo"
	domainharnesses "github.com/openmodu/onecatch/internal/domain/harnesses"
	domainsettings "github.com/openmodu/onecatch/internal/domain/settings"
)

type settingsPage struct {
	app                   *app
	runtime               *kit.SelectView
	enabled               *filledToggle
	binary, model, status string
	saving                bool
	initialized           bool
	section               string
	draft                 domainsettings.Settings
	fields                map[string]*string
	selects               map[string]*kit.SelectView
	switches              map[string]*filledToggle
	workers               workerSettingsState
	mobile                mobileAccessState
	skillTargets          skillTargetSettings
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
		p.draft = a.settings
		p.draft.Runtimes = maps.Clone(a.settings.Runtimes)
		p.selectRuntime(p.runtime.Value())
	}
	left := el.Div().W(el.Dp(216)).HFull().NoShrink().Bg(sidebarColor).Child(dragArea(cx, el.Div().H(el.Dp(52)).WFull()))
	nav := el.Div().Grow().H(el.Dp(0)).ScrollY().Px(12).Py(8).Gap(4)
	for _, item := range settingsSections {
		it := item
		selected := p.section == it.ID
		row := el.Div().ID("settings-section/" + it.ID).Name(it.Label).Role("button").Focusable(true).Px(12).Py(10).Gap(2).Rounded(8).CursorPointer().OnClick(func() {
			p.section = it.ID
			p.status = ""
			if it.ID == "experimental" {
				p.checkEnabledWorkers()
			}
			if it.ID == "mobile" {
				p.loadMobileAccess()
			}
		}).Hover(func(s *el.Style) { s.Bg(theme.Highlight) }).FocusStyle(neutralFocus)
		if selected {
			row.Bg(theme.Highlight)
		}
		row.Child(el.Text(it.Label).TextSize(13).Medium())
		if it.ID != "experimental" {
			row.Child(el.Text(it.Description).TextSize(11).LineHeight(1.375).TextColor(theme.Muted))
		}
		nav.Child(row)
	}
	left.Child(nav)
	label, description := "外观", "语言、显示与主题"
	for _, item := range settingsSections {
		if item.ID == p.section {
			label, description = item.Label, item.Description
		}
	}
	if p.section == "experimental" && p.workersVisible() {
		description = "管理沙箱与远端 Worker。"
	}
	heading := el.Div().Gap(6).Child(el.Text("本地设置").TextSize(11).TextColor(theme.Muted), el.Text(label).TextSize(20).Bold(), el.Text(description).TextSize(14).TextColor(theme.Muted))
	var header el.Element = heading
	if p.section == "experimental" {
		state := fmt.Sprintf("已同步 · 版本 %d", a.settings.Revision)
		if p.workerSettingsDirty() {
			state = "等待保存"
		}
		header = el.Div().Row().Gap(16).Child(heading.Grow().W(el.Dp(0)), el.Text(state).TextSize(12).TextColor(theme.Muted))
	}
	body := el.Div().Grow().H(el.Dp(0)).ScrollY().Px(28).Pt(12).Pb(40).Gap(24).Disabled(p.saving).Child(header)
	if p.section == "" {
		body.Child(p.appearance(cx))
	} else if p.section == "harness" {
		body.Child(p.harness(cx))
	} else if p.section == "experimental" {
		body.Child(p.workerSettings(cx))
	} else if p.section == "mobile" {
		body.Child(p.mobileAccess(cx))
	} else {
		body.Child(p.sectionFields(cx))
	}
	if p.section != "" && p.section != "skills" && p.section != "experimental" && p.section != "mobile" {
		save := button("保存", p.saveSection, kit.ButtonPrimary).Loading(p.saving)
		save.SetDisabled(!p.initialized)
		body.Child(el.Div().Row().Items(el.Center).Gap(12).Child(save.Render(cx), button("重新加载", p.reload, kit.ButtonGhost).Render(cx), el.Text(p.status).TextSize(12).TextColor(theme.Muted)))
	}
	if p.section != "experimental" && p.section != "mobile" && p.section != "skills" {
		body.Child(el.Text(fmt.Sprintf("OneCatch %s · %s", buildinfo.Version, runtime.GOARCH)).TextSize(11).TextColor(theme.Muted))
	} else if p.status != "" {
		body.Child(el.Text(p.status).TextSize(12).TextColor(theme.Muted))
	}
	right := el.Div().Grow().W(el.Dp(0)).HFull().Child(dragArea(cx, el.Div().H(el.Dp(52)).WFull().Pr(216).Center().Child(el.Text("设置").TextSize(14).Medium())), body)
	if p.section == "experimental" && p.workerSettingsDirty() {
		right.Child(p.workerSettingsSaveBar(cx))
	}
	return el.Div().WFull().HFull().Row().Bg(theme.Bg).Child(p.workerOverlays(cx), left, el.Div().W(el.Dp(1)).HFull().Bg(seamColor), right)
}

var settingsSections = []struct{ ID, Label, Description string }{
	{"", "外观", "语言、显示与主题"}, {"harness", "Harness", "配置各 Harness 的 Agent"}, {"skills", "Skills", "Skill 同步目标与目录"},
	{"mobile", "手机连接", "把这台电脑作为手机端的远程 Worker"}, {"terminal", "终端", "配置 Shell 与终端外观"}, {"execution", "执行", "新流程默认值"}, {"security", "安全", "授权与诊断"}, {"storage", "存储与日志", "本机数据管理"},
	{"experimental", "远端 Worker", "敬请期待"},
}

func (p *settingsPage) harness(cx *el.Context) el.Element {
	a := p.app
	root := el.Div().Gap(16).Child(el.Text("Agent").Medium(), p.runtime.Render(cx), p.enabled.Render(cx), workflowField("可执行文件", field("runtime-binary", "留空使用 PATH 中的默认命令", &p.binary)), workflowField("默认模型", field("runtime-model", "留空使用 Agent 的默认模型", &p.model)))
	status := p.status
	for _, r := range a.runtimes {
		if r.ID == p.runtime.Value() && status == "" {
			if r.Available {
				status = "已连接 · " + r.Version
			} else {
				status = "未检测到命令，请配置路径后重新检测。"
			}
		}
	}
	root.Child(el.Text(status).TextSize(12).TextColor(theme.Muted))
	id := p.runtime.Value()
	r := p.draft.Runtimes[id]
	h, _ := domainharnesses.Find(id)
	root.Child(workflowField("环境变量白名单", p.text("runtime-env/"+id, "仅填写变量名称，用逗号分隔", strings.Join(r.EnvironmentAllowlist, ", "))))
	if len(h.Efforts) > 0 {
		root.Child(workflowField("推理强度", p.choice(cx, "runtime-effort/"+id, r.ReasoningEffort, append([]string{""}, h.Efforts...))))
	}
	if len(h.Providers) > 0 {
		root.Child(workflowField("Provider", p.choice(cx, "runtime-provider/"+id, r.Provider, h.Providers)))
	}
	if h.ServiceTiers {
		root.Child(workflowField("服务层级", p.choice(cx, "runtime-tier/"+id, r.ServiceTier, []string{"", "auto", "default", "flex", "priority", "fast"})))
	}
	if len(h.Integrations) > 1 {
		root.Child(workflowField("集成方式", p.choice(cx, "runtime-integration/"+id, r.Integration, h.Integrations)), workflowField("配置来源", p.choice(cx, "runtime-source/"+id, r.ConfigSource, []string{"shared", "isolated"})), workflowField("配置路径", p.text("runtime-config/"+id, "留空使用默认路径", r.ConfigPath)))
	}
	if h.SupportsRemoteFS {
		root.Child(p.toggle(cx, "runtime-remotefs/"+id, "启用 Remote FS", r.RemoteFSEnabled))
	}
	if id == "codex" {
		root.Child(p.toggle(cx, "runtime-context/"+id, "使用最大上下文窗口", r.MaxContextWindow))
	}
	return root
}

func (p *settingsPage) text(id, hint, initial string) el.Element {
	if p.fields[id] == nil {
		value := initial
		p.fields[id] = &value
	}
	return field("settings/"+id, hint, p.fields[id]).WFull().MinH(el.Dp(36))
}
func (p *settingsPage) number(id, label string, value int) el.Element {
	return workflowField(label, p.text(id, label, strconv.Itoa(value)))
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
func (p *settingsPage) toggle(cx *el.Context, id, label string, value bool) el.Element {
	s := p.switches[id]
	if s == nil {
		s = newFilledToggle(label, value, false)
		p.switches[id] = s
	}
	return el.Div().Py(8).Child(s.Render(cx))
}
func (p *settingsPage) appearance(cx *el.Context) el.Element {
	a := p.app
	root := el.Div().Gap(12).Child(el.Text("界面").TextSize(15).Bold(), el.Text("调整 OneCatch 的外观与显示方式。").TextSize(12).TextColor(theme.Muted))
	for _, group := range []struct {
		label, description string
		values, labels     []string
		selected           string
		accent             bool
	}{
		{"显示模式", "跟随系统外观，或固定使用浅色、深色模式。", []string{"system", "light", "dark"}, []string{"跟随系统", "浅色", "深色"}, a.ui.Preferences.Theme, false},
		{"主题配色", "选择用于按钮、选中状态和重点信息的强调色。", []string{"forest", "ocean", "violet", "amber"}, []string{"森林", "海洋", "紫罗兰", "琥珀"}, a.ui.Preferences.Accent, true},
	} {
		options := el.Div().Row().Wrap().Gap(4)
		for i, value := range group.values {
			v, accent := value, group.accent
			options.Child(button(group.labels[i], func() {
				if accent {
					a.setAppearance(a.ui.Preferences.Theme, v)
				} else {
					a.setAppearance(v, a.ui.Preferences.Accent)
				}
			}, kit.ButtonGhost).Selected(group.selected == v).Render(cx))
		}
		root.Child(el.Div().Grid(2).Items(el.Center).P(16).Rounded(10).Bg(theme.Surface).Gap(16).Child(el.Div().Gap(6).Child(el.Text(group.label).TextSize(13).Medium(), el.Text(group.description).TextSize(11).TextColor(theme.Muted)), options.Justify(el.End)))
	}
	return root
}

func (p *settingsPage) sectionFields(cx *el.Context) el.Element {
	d := p.draft
	root := el.Div().Gap(16)
	switch p.section {
	case "skills":
		root.Child(p.skillTargetSettings(cx))
	case "terminal":
		p.text("shell-args", "每行一个参数", strings.Join(d.Terminal.Arguments, "\n"))
		root.Child(workflowField("Shell", p.text("shell", "留空使用系统默认 Shell", d.Terminal.Shell)), workflowField("启动参数", el.TextArea().ID("settings/shell-args").Name("启动参数").Bind(p.fields["shell-args"]).Placeholder("每行一个参数").AutoGrow(2, 5).Bg(theme.Subtle).Rounded(8).P(12).TextSize(13)), workflowField("终端主题", p.choice(cx, "terminal-theme", d.Terminal.Theme, []string{"system", "paper", "midnight", "contrast"})))
	case "execution":
		root.Child(p.number("max-transitions", "最大转移次数", d.Execution.MaxTransitions), p.number("max-failures", "连续失败上限", d.Execution.MaxConsecutiveFailures), p.number("step-timeout", "单步超时（秒）", d.Execution.StepTimeoutSeconds), p.number("dag-concurrency", "本地 DAG 并发数", d.Execution.MaxLocalDAGConcurrency), p.number("interrupt-grace", "中断宽限期（秒）", d.Execution.InterruptGraceSeconds), workflowField("默认访问权限", p.choice(cx, "default-sandbox", d.Execution.DefaultSandbox, []string{"read-only", "workspace-write", "full"})))
	case "security":
		root.Child(p.toggle(cx, "allow-full", "允许完全访问", d.Security.AllowFullSandbox), p.toggle(cx, "confirm-full", "每次完全访问运行都需确认", d.Security.ConfirmFullSandboxEveryRun), p.toggle(cx, "diagnostics-prompt", "诊断包包含任务指令", d.Security.DiagnosticsIncludePrompt), p.toggle(cx, "diagnostics-events", "诊断包包含原始事件", d.Security.DiagnosticsIncludeRawEvents))
	case "storage":
		root.Child(el.Text("数据目录："+p.app.backend.service.DataRoot()).TextSize(12).TextColor(theme.Muted), p.number("retention-days", "完成任务保留天数（0 为永久保留）", d.Storage.CompletedRunRetentionDays), workflowField("日志级别", p.choice(cx, "log-level", d.Storage.LogLevel, []string{"debug", "info", "warn", "error"})), p.number("log-size", "单文件最大大小（MB）", d.Storage.LogMaxSizeMB), p.number("log-backups", "日志备份数", d.Storage.LogMaxBackups), p.number("log-age", "日志保留天数", d.Storage.LogMaxAgeDays))
	}
	return root
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
