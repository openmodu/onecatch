package keeldesktop

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"

	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/theme"
	"github.com/openmodu/onecatch/internal/buildinfo"
	domainharnesses "github.com/openmodu/onecatch/internal/domain/harnesses"
)

func settingsTitle(id string) string {
	for _, section := range settingsSections {
		if section.ID == id {
			return section.Label
		}
	}
	return "外观"
}

func (p *settingsPage) activateSettingsSection(id string) {
	if p.section == id {
		return
	}
	p.section, p.status = id, ""
	if id == "experimental" {
		p.checkEnabledWorkers()
	}
	if id == "mobile" {
		p.loadMobileAccess()
	}
}

// Settings owns navigation and search; section IDs still address repository
// updates. The last synchronized ID also permits external page selection.
func (p *settingsPage) syncSettingsSection() {
	if p.section != p.viewSection {
		p.view.SetValue(settingsTitle(p.section))
	} else {
		for _, section := range settingsSections {
			if section.Label == p.view.Value() {
				p.activateSettingsSection(section.ID)
				break
			}
		}
	}
	p.viewSection = p.section
}

func (p *settingsPage) ensureSettingsView() {
	model := fmt.Sprintf("%s/%d", p.runtime.Value(), len(p.runtime.Entries()))
	if p.view != nil && p.viewModel == model {
		return
	}
	query := ""
	if p.view != nil {
		query = p.view.Query()
	}
	next := kit.Settings().Size(kit.SettingsSizeSmall).GroupVariant(kit.GroupBoxFill)
	icons := []kit.IconName{kit.IconSettings, kit.IconUser, kit.IconInbox, kit.IconExternalLink,
		kit.IconFile, kit.IconArchive, kit.IconLock, kit.IconFolder, kit.IconHome}
	for i, section := range settingsSections {
		groups := p.settingsGroups(section.ID)
		for gi := range groups {
			for ii := range groups[gi].Items {
				item := &groups[gi].Items[ii]
				item.Keywords = append(item.Keywords, section.Label, section.Description)
			}
		}
		next.Page(kit.SettingPage{Title: section.Label, Icon: icons[i], Groups: groups})
	}
	next.SetValue(settingsTitle(p.section))
	next.SetQuery(query)
	if p.view == nil {
		p.view = next
	} else {
		*p.view = *next
	}
	p.viewModel, p.viewSection = model, p.section
}

func (p *settingsPage) settingsFooter(cx *el.Context, id string) el.Element {
	info := el.Div().Grow().W(el.Dp(0)).Gap(4)
	if p.status != "" {
		info.Child(el.Text(p.status).TextSize(12).TextColor(theme.Muted))
	}
	if id != "skills" && id != "experimental" && id != "mobile" {
		info.Child(el.Text(fmt.Sprintf("OneCatch %s · %s", buildinfo.Version, runtime.GOARCH)).TextSize(11).TextColor(theme.Muted))
	}
	root := el.Div().Row().Items(el.Center).Gap(12).Child(info)
	if id != "" && id != "skills" && id != "experimental" && id != "mobile" {
		save := button("保存", p.saveSection, kit.ButtonPrimary).Loading(p.saving)
		save.SetDisabled(!p.initialized)
		reload := button("重新加载", p.reload, kit.ButtonGhost)
		reload.SetDisabled(p.saving)
		root.Child(el.Div().Row().Items(el.Center).NoShrink().Gap(8).Child(reload.Render(cx), save.Render(cx)))
	}
	return root
}

func (p *settingsPage) settingsText(id, label, hint string, initial func() string) kit.SettingItem {
	return kit.SettingItem{Label: label, Control: el.ViewFunc(func(*el.Context) el.Element {
		return p.text(id, hint, initial()).(*el.InputEl).Name(label)
	})}
}
func (p *settingsPage) settingsNumber(id, label string, initial func() int) kit.SettingItem {
	return p.settingsText(id, label, "请输入整数", func() string { return strconv.Itoa(initial()) })
}
func (p *settingsPage) settingsChoice(id, label string, initial func() string, values []string) kit.SettingItem {
	return kit.SettingItem{Label: label, Control: el.ViewFunc(func(cx *el.Context) el.Element {
		return p.choice(cx, id, initial(), values)
	})}
}
func (p *settingsPage) settingsToggle(id, label string, initial func() bool) kit.SettingItem {
	return kit.SettingItem{Label: label, Control: el.ViewFunc(func(cx *el.Context) el.Element {
		control := p.switches[id]
		if control == nil {
			control = newFilledToggle(label, initial(), false)
			control.hideLabel = true
			p.switches[id] = control
		}
		return control.Render(cx)
	})}
}

func (p *settingsPage) settingsGroups(id string) []kit.SettingGroup {
	switch id {
	case "":
		return p.appearanceGroups()
	case "harness":
		return p.harnessGroups()
	case "terminal":
		return []kit.SettingGroup{{Title: "Shell", Items: []kit.SettingItem{
			p.settingsText("shell", "Shell", "留空使用系统默认 Shell", func() string { return p.draft.Terminal.Shell }),
			{Label: "启动参数", Description: "每行一个参数", Control: el.ViewFunc(func(*el.Context) el.Element {
				p.text("shell-args", "每行一个参数", strings.Join(p.draft.Terminal.Arguments, "\n"))
				return el.TextArea().ID("settings/shell-args").Name("启动参数").Bind(p.fields["shell-args"]).Placeholder("每行一个参数").AutoGrow(2, 5).Bg(theme.Subtle).Rounded(8).P(12).TextSize(13)
			})},
		}}, {Title: "外观", Items: []kit.SettingItem{
			p.settingsChoice("terminal-theme", "终端主题", func() string { return p.draft.Terminal.Theme }, []string{"system", "paper", "midnight", "contrast"}),
		}}}
	case "execution":
		return []kit.SettingGroup{{Title: "新流程默认值", Items: []kit.SettingItem{
			p.settingsNumber("max-transitions", "最大转移次数", func() int { return p.draft.Execution.MaxTransitions }),
			p.settingsNumber("max-failures", "连续失败上限", func() int { return p.draft.Execution.MaxConsecutiveFailures }),
			p.settingsNumber("step-timeout", "单步超时（秒）", func() int { return p.draft.Execution.StepTimeoutSeconds }),
			p.settingsNumber("dag-concurrency", "本地 DAG 并发数", func() int { return p.draft.Execution.MaxLocalDAGConcurrency }),
			p.settingsNumber("interrupt-grace", "中断宽限期（秒）", func() int { return p.draft.Execution.InterruptGraceSeconds }),
			p.settingsChoice("default-sandbox", "默认访问权限", func() string { return p.draft.Execution.DefaultSandbox }, []string{"read-only", "workspace-write", "full"}),
		}}}
	case "security":
		return []kit.SettingGroup{{Title: "授权", Items: []kit.SettingItem{
			p.settingsToggle("allow-full", "允许完全访问", func() bool { return p.draft.Security.AllowFullSandbox }),
			p.settingsToggle("confirm-full", "每次完全访问运行都需确认", func() bool { return p.draft.Security.ConfirmFullSandboxEveryRun }),
		}}, {Title: "诊断", Items: []kit.SettingItem{
			p.settingsToggle("diagnostics-prompt", "诊断包包含任务指令", func() bool { return p.draft.Security.DiagnosticsIncludePrompt }),
			p.settingsToggle("diagnostics-events", "诊断包包含原始事件", func() bool { return p.draft.Security.DiagnosticsIncludeRawEvents }),
		}}}
	case "storage":
		return []kit.SettingGroup{{Title: "数据", Items: []kit.SettingItem{
			{Label: "数据目录", Description: p.app.backend.service.DataRoot()},
			p.settingsNumber("retention-days", "完成任务保留天数（0 为永久保留）", func() int { return p.draft.Storage.CompletedRunRetentionDays }),
		}}, {Title: "日志", Items: []kit.SettingItem{
			p.settingsChoice("log-level", "日志级别", func() string { return p.draft.Storage.LogLevel }, []string{"debug", "info", "warn", "error"}),
			p.settingsNumber("log-size", "单文件最大大小（MB）", func() int { return p.draft.Storage.LogMaxSizeMB }),
			p.settingsNumber("log-backups", "日志备份数", func() int { return p.draft.Storage.LogMaxBackups }),
			p.settingsNumber("log-age", "日志保留天数", func() int { return p.draft.Storage.LogMaxAgeDays }),
		}}}
	case "skills":
		return []kit.SettingGroup{{Items: []kit.SettingItem{{Label: "Skills 同步目标", Keywords: []string{"Skill", "同步", "目录", "Agent"}, Content: el.ViewFunc(p.skillTargetSettings)}}}}
	case "mobile":
		return []kit.SettingGroup{{Items: []kit.SettingItem{{Label: "手机连接", Keywords: []string{"手机", "远程", "Worker", "配对", "二维码"}, Content: el.ViewFunc(p.mobileAccess)}}}}
	default:
		return []kit.SettingGroup{{Items: []kit.SettingItem{{Label: "远端 Worker", Keywords: []string{"远端", "沙箱", "Worker", "环境", "配对"}, Content: el.ViewFunc(p.workerSettings)}}}}
	}
}

func (p *settingsPage) appearanceGroups() []kit.SettingGroup {
	var rows []kit.SettingItem
	for _, group := range []struct {
		label, description string
		values, labels     []string
		accent             bool
	}{
		{"显示模式", "跟随系统外观，或固定使用浅色、深色模式。", []string{"system", "light", "dark"}, []string{"跟随系统", "浅色", "深色"}, false},
		{"主题配色", "选择按钮和选中状态的强调色。", []string{"forest", "ocean", "violet", "amber"}, []string{"森林", "海洋", "紫罗兰", "琥珀"}, true},
	} {
		rows = append(rows, kit.SettingItem{Label: group.label, Description: group.description, Keywords: group.labels, Control: el.ViewFunc(func(cx *el.Context) el.Element {
			options := el.Div().Row().Wrap().Gap(4)
			selected := p.app.ui.Preferences.Theme
			if group.accent {
				selected = p.app.ui.Preferences.Accent
			}
			for i, value := range group.values {
				options.Child(button(group.labels[i], func() {
					if group.accent {
						p.app.setAppearance(p.app.ui.Preferences.Theme, value)
					} else {
						p.app.setAppearance(value, p.app.ui.Preferences.Accent)
					}
				}, kit.ButtonGhost).Selected(selected == value).Render(cx))
			}
			return options
		})})
	}
	return []kit.SettingGroup{{Title: "界面", Items: rows}}
}

func (p *settingsPage) harnessGroups() []kit.SettingGroup {
	id := p.runtime.Value()
	h, _ := domainharnesses.Find(id)
	p.runtime.OnChange(p.selectRuntime)
	rows := []kit.SettingItem{
		{Label: "Agent", Control: p.runtime},
		{Label: "启用此 Agent", Control: el.ViewFunc(func(cx *el.Context) el.Element { p.enabled.hideLabel = true; return p.enabled.Render(cx) })},
		{Label: "可执行文件", Control: el.ViewFunc(func(*el.Context) el.Element {
			return field("runtime-binary", "留空使用 PATH 中的默认命令", &p.binary).Name("可执行文件")
		})},
		{Label: "默认模型", Control: el.ViewFunc(func(*el.Context) el.Element {
			return field("runtime-model", "留空使用 Agent 的默认模型", &p.model).Name("默认模型")
		})},
		p.settingsText("runtime-env/"+id, "环境变量白名单", "仅填写变量名称，用逗号分隔", func() string { return strings.Join(p.draft.Runtimes[id].EnvironmentAllowlist, ", ") }),
	}
	if len(h.Efforts) > 0 {
		rows = append(rows, p.settingsChoice("runtime-effort/"+id, "推理强度", func() string { return p.draft.Runtimes[id].ReasoningEffort }, append([]string{""}, h.Efforts...)))
	}
	if len(h.Providers) > 0 {
		rows = append(rows, p.settingsChoice("runtime-provider/"+id, "Provider", func() string { return p.draft.Runtimes[id].Provider }, h.Providers))
	}
	if h.ServiceTiers {
		rows = append(rows, p.settingsChoice("runtime-tier/"+id, "服务层级", func() string { return p.draft.Runtimes[id].ServiceTier }, []string{"", "auto", "default", "flex", "priority", "fast"}))
	}
	if len(h.Integrations) > 1 {
		rows = append(rows,
			p.settingsChoice("runtime-integration/"+id, "集成方式", func() string { return p.draft.Runtimes[id].Integration }, h.Integrations),
			p.settingsChoice("runtime-source/"+id, "配置来源", func() string { return p.draft.Runtimes[id].ConfigSource }, []string{"shared", "isolated"}),
			p.settingsText("runtime-config/"+id, "配置路径", "留空使用默认路径", func() string { return p.draft.Runtimes[id].ConfigPath }))
	}
	if h.SupportsRemoteFS {
		rows = append(rows, p.settingsToggle("runtime-remotefs/"+id, "启用 Remote FS", func() bool { return p.draft.Runtimes[id].RemoteFSEnabled }))
	}
	if id == "codex" {
		rows = append(rows, p.settingsToggle("runtime-context/"+id, "使用最大上下文窗口", func() bool { return p.draft.Runtimes[id].MaxContextWindow }))
	}
	return []kit.SettingGroup{{Title: "Agent 配置", Items: rows, Footer: el.ViewFunc(func(*el.Context) el.Element {
		status := ""
		for _, agent := range p.app.runtimes {
			if agent.ID == p.runtime.Value() {
				status = "未检测到命令，请配置路径后重新检测。"
				if agent.Available {
					status = "已连接 · " + agent.Version
				}
				break
			}
		}
		return el.Text(status).TextSize(12).TextColor(theme.Muted)
	})}}
}
