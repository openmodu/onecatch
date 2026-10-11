package keeldesktop

import (
	"fmt"
	"gioui.org/io/key"
	"strconv"
	"strings"

	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/locale"
	"github.com/dyike/keel/ui/theme"
	domainharnesses "github.com/openmodu/onecatch/internal/domain/harnesses"
)

func settingsTitle(id string) string {
	for _, section := range settingsSections {
		if section.ID == id {
			return uiText(section.Label)
		}
	}
	return uiText("外观")
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
			if uiText(section.Label) == p.view.Value() {
				p.activateSettingsSection(section.ID)
				break
			}
		}
	}
	p.viewSection = p.section
}

func (p *settingsPage) ensureSettingsView() {
	model := fmt.Sprintf("%s/%d/%d", p.runtime.Value(), len(p.runtime.Entries()), locale.Revision())
	if p.view != nil && p.viewModel == model {
		return
	}
	query := ""
	if p.view != nil {
		query = p.view.Query()
	}
	next := kit.Settings().Size(kit.SettingsSizeMedium).GroupVariant(kit.GroupBoxNormal).SearchInSidebar(true).PageHeading(false)
	icons := []kit.IconName{kit.IconSettings, kit.IconUser, kit.IconInbox, kit.IconLock, kit.IconFolder, kit.IconPhone, kit.IconComputer, kit.IconArchive, kit.IconArchive}
	for i, section := range settingsSections {
		groups := p.settingsGroups(section.ID)
		for gi := range groups {
			groups[gi].Title = uiText(groups[gi].Title)
			for ii := range groups[gi].Items {
				item := &groups[gi].Items[ii]
				item.Keywords = append(item.Keywords, section.Label, section.Description, item.Label, item.Description)
				// Keep help text searchable without displaying a second line on every setting.
				item.Keywords = append(item.Keywords, uiText(item.Description))
				if item.Label != "数据目录" {
					item.Description = ""
					item.DescriptionContent = nil
				}
				item.Label, item.Description = uiText(item.Label), uiText(item.Description)
			}
		}
		next.Page(kit.SettingPage{Category: uiText(section.Category), Title: uiText(section.Label), Icon: icons[i], Groups: groups})
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
	info := el.Div().Gap(4)

	root := el.Div().WFull().Row().Items(el.Center).Gap(12).Child(info)
	width, _ := cx.ViewportSize()
	if width < 900 {
		root.Col().Items(el.Stretch)
		info.WFull()
	} else {
		info.Grow().W(el.Dp(0))
	}
	if id != "" && id != "skills" && id != "experimental" && id != "mobile" {
		save := button("保存", p.saveSection, kit.ButtonPrimary).Loading(p.saving)
		save.SetDisabled(!p.initialized)
		reload := button("重新加载", p.reload, kit.ButtonGhost)
		reload.SetDisabled(p.saving)
		root.Child(el.Div().WrapFit().Items(el.Center).Justify(el.End).NoShrink().Gap(8).Child(button("恢复默认值", p.resetSettings, kit.ButtonGhost).Render(cx), button("放弃修改", p.discardSettings, kit.ButtonGhost).Render(cx), reload.Render(cx), save.Render(cx)))
	}
	return root
}

func (p *settingsPage) settingsText(id, label, hint string, initial func() string) kit.SettingItem {
	return kit.SettingItem{Label: label, Control: el.ViewFunc(func(*el.Context) el.Element {
		return p.text(id, hint, initial()).(*el.InputEl).Name(uiText(label))
	})}
}
func (p *settingsPage) settingsNumber(id, label string, initial func() int) kit.SettingItem {
	return kit.SettingItem{Label: label, Control: el.ViewFunc(func(cx *el.Context) el.Element {
		if p.numbers == nil {
			p.numbers = map[string]*kit.NumberInputView{}
		}
		p.text(id, "", strconv.Itoa(initial()))
		control := p.numbers[id]
		if control == nil {
			control = kit.NumberInput("").Size(32).Decimals(0).Range(0, 2147483647).VerticalButtons(true).OnChange(func(value float64) { *p.fields[id] = strconv.FormatInt(int64(value), 10) })
			p.numbers[id] = control
		}
		value, err := strconv.ParseFloat(*p.fields[id], 64)
		if err == nil && control.Value() != value {
			control.SetValue(value)
		}
		return el.Div().W(el.Dp(80)).MaxW(el.Full).Bg(theme.Subtle).Rounded(8).Items(el.Stretch).Child(control.Render(cx))
	})}
}
func (p *settingsPage) settingsChoice(id, label string, initial func() string, values []string) kit.SettingItem {
	return kit.SettingItem{Label: label, Control: el.ViewFunc(func(cx *el.Context) el.Element {
		control := p.choice(cx, id, initial(), values)
		if id == "log-level" {
			p.selects[id].Size(32)
			return control.(*el.DivEl).W(el.Dp(96)).MaxW(el.Full)
		}
		if id == "default-sandbox" {
			return control.(*el.DivEl).W(el.Dp(148)).MaxW(el.Full)
		}
		return control
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
	case "workflows":
		return []kit.SettingGroup{{Items: []kit.SettingItem{{Label: "工作流管理", Content: el.ViewFunc(func(cx *el.Context) el.Element {
			_, h := cx.ViewportSize()
			return el.Div().H(el.Dp(min(580, max(240, h-190)))).Items(el.Stretch).Child(p.app.ui.WorkflowConfirm.Render(cx), p.app.workflowPageLayout(cx, false))
		})}}}}
	case "":
		return p.appearanceGroups()
	case "harness":
		return p.harnessGroups()
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
		return p.storageActions()
	case "skills":
		return []kit.SettingGroup{{Items: []kit.SettingItem{{Label: "Skills 同步目标", Keywords: []string{"Skill", "同步", "目录", "Agent"}, Content: el.ViewFunc(p.skillTargetSettings)}}}}
	case "mobile":
		return []kit.SettingGroup{{Items: []kit.SettingItem{{Label: "手机连接", Keywords: []string{"手机", "远程", "Worker", "配对", "二维码"}, Content: el.ViewFunc(p.mobileAccess)}}}}
	default:
		return []kit.SettingGroup{{Items: []kit.SettingItem{{Label: "远端 Worker", Keywords: []string{"远端", "沙箱", "Worker", "环境", "配对"}, Content: el.ViewFunc(p.workerSettings)}}}}
	}
}

func (p *settingsPage) appearanceGroups() []kit.SettingGroup {
	rows := []kit.SettingItem{p.languageSetting()}
	rows = append(rows, kit.SettingItem{Label: "聊天字体大小", Description: "只调整对话正文的文字大小，立即生效。", Control: el.ViewFunc(func(cx *el.Context) el.Element {
		row := el.Div().Row().Justify(el.End).WFull().Gap(4)
		for i, value := range []string{"small", "standard", "large", "extra-large"} {
			row.Child(button([]string{"小", "标准", "大", "特大"}[i], func() { p.app.setChatFontSize(value) }, kit.ButtonGhost).Compact(true).Size(28).Selected(normalizeUIPreferences(p.app.ui.Preferences).ChatFontSize == value).Render(cx))
		}
		return row
	})})
	for _, group := range []struct {
		label, description string
		values, labels     []string
	}{
		{"显示模式", "跟随系统外观，或固定使用浅色、深色模式。", []string{"system", "light", "dark"}, []string{"跟随系统", "浅色", "深色"}},
	} {
		rows = append(rows, kit.SettingItem{Label: group.label, Description: group.description, Keywords: group.labels, Control: el.ViewFunc(func(cx *el.Context) el.Element {
			options := el.Div().Row().Justify(el.End).WFull().Gap(4)
			selected := p.app.ui.Preferences.Theme
			for i, value := range group.values {
				options.Child(button(group.labels[i], func() {
					p.app.setAppearance(value, p.app.ui.Preferences.Accent)
				}, kit.ButtonGhost).Compact(true).Size(28).Selected(selected == value).Render(cx))
			}
			return options
		})})
	}
	rows = append(rows, kit.SettingItem{Label: "表格分隔线", Description: "显示对话表格内部的行、列分隔线，切换后立即生效。", Keywords: []string{"Markdown", "表格", "分隔线"}, Control: el.ViewFunc(func(cx *el.Context) el.Element {
		control := p.switches["table-dividers"]
		if control == nil {
			control = newFilledToggle("表格分隔线", true, false)
			control.hideLabel = true
			control.onChange = p.app.setTableDividers
			p.switches["table-dividers"] = control
		}
		control.SetValue(!p.app.ui.Preferences.HideTableDividers)
		return control.Render(cx)
	})})
	rows = append(rows, p.terminalThemeSetting())
	return []kit.SettingGroup{{Title: "外观", Items: rows}}
}

func (p *settingsPage) harnessRows(id string) []kit.SettingItem {
	h, _ := domainharnesses.Find(id)
	rows := []kit.SettingItem{
		p.settingsToggle("runtime-enabled/"+id, uiText("启用")+" "+h.Name, func() bool { return p.draft.Runtimes[id].Enabled }),
		p.settingsText("runtime-binary/"+id, "可执行文件", "留空使用 PATH 中的默认命令", func() string { return p.draft.Runtimes[id].Binary }),
		{Label: "默认模型", Control: el.ViewFunc(func(cx *el.Context) el.Element { return p.harnessModel(cx, id) })},
		p.settingsText("runtime-env/"+id, "环境变量白名单", "仅填写变量名称，用逗号分隔", func() string { return strings.Join(p.draft.Runtimes[id].EnvironmentAllowlist, ", ") }),
	}
	if len(h.Efforts) > 0 {
		rows = append(rows, p.settingsChoice("runtime-effort/"+id, "推理强度", func() string { return p.draft.Runtimes[id].ReasoningEffort }, p.harnessEfforts(id, h.Efforts)))
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
			p.settingsChoice("runtime-source/"+id, "配置来源", func() string { return p.draft.Runtimes[id].ConfigSource }, []string{"shared", "onecatch"}),
			p.settingsText("runtime-config/"+id, "配置路径", "留空使用默认路径", func() string { return p.draft.Runtimes[id].ConfigPath }))
	}
	if h.SupportsRemoteFS {
		rows = append(rows, p.settingsToggle("runtime-remotefs/"+id, "启用 Remote FS", func() bool { return p.draft.Runtimes[id].RemoteFSEnabled }))
	}
	if id == "codex" {
		rows = append(rows, p.settingsToggle("runtime-context/"+id, "使用最大上下文窗口", func() bool { return p.draft.Runtimes[id].MaxContextWindow }))
	}
	return rows
}

func (p *settingsPage) harnessGroups() []kit.SettingGroup {
	return []kit.SettingGroup{{Title: "Agent 配置", Items: []kit.SettingItem{{Label: "Harness", Content: el.ViewFunc(p.harnessCards)}}}}
}

func (p *settingsPage) harnessCards(cx *el.Context) el.Element {
	if p.harnessExpanded == nil {
		p.harnessExpanded = map[string]bool{}
	}
	root := el.Div().Items(el.Stretch)
	width, _ := cx.ViewportSize()
	columns := 2
	if min(float32(1120), width-64) < 800 {
		columns = 1
	}
	for index, runtime := range p.runtime.Entries() {
		id, name := runtime.Value, runtime.Label
		expanded := p.harnessExpanded[id]
		arrow := kit.IconChevronRight
		if expanded {
			arrow = kit.IconChevronDown
		}
		toggle := func() { p.harnessExpanded[id] = !p.harnessExpanded[id] }
		header := el.Div().OnKey(func(e el.KeyEvent) bool {
			if e.Name != string(key.NameReturn) && e.Name != string(key.NameSpace) {
				return false
			}
			if e.State == el.KeyPress {
				toggle()
			}
			return true
		}).Row().Items(el.Center).Gap(10).Py(16).Focusable(true).Role("button").Name(name).Selected(expanded).CursorPointer().OnClick(toggle).Child(el.Text(name).Medium().Grow(), kit.Icon(arrow).Size(16).Render(cx))
		if index > 0 {
			root.Child(el.Div().H(el.Dp(1)).NoShrink().Bg(seamColor))
		}
		card := el.Div().Items(el.Stretch).Child(header)
		if expanded {
			grid := el.Div().Grid(columns).Gap(16).Items(el.Stretch)
			for _, row := range p.harnessRows(id) {
				cell := el.Div().Items(el.Stretch).Gap(6).Child(el.Text(uiText(row.Label)).TextSize(12).TextColor(theme.Muted), row.Control.Render(cx))
				grid.Child(cell)
			}
			card.Child(el.Div().Pb(16).Gap(16).Items(el.Stretch).Child(grid, p.harnessStatus(cx, id)))
		}
		root.Child(card)
	}
	return root
}

func (p *settingsPage) terminalThemeSetting() kit.SettingItem {
	return kit.SettingItem{Label: "终端主题", Control: el.ViewFunc(func(cx *el.Context) el.Element {
		control := p.choice(cx, "terminal-theme", p.app.settings.Terminal.Theme, []string{"system", "paper", "midnight", "contrast"})
		choice := p.selects["terminal-theme"]
		choice.SetValue(p.app.settings.Terminal.Theme)
		choice.SetDisabled(p.actions.busy != "" || p.saving)
		choice.OnChange(func(value string) {
			settings := p.app.settings.Terminal
			settings.Theme = value
			revision, ctx := p.app.settings.Revision, p.workerContext()
			p.settingsAction("terminal-theme", func() (func(), error) {
				result, err := p.app.backend.service.UpdateTerminalSettings(ctx, settings, revision)
				return func() {
					p.app.settings = result
					p.draft.Terminal, p.draft.Revision = result.Terminal, result.Revision
				}, err
			})
		})
		return control.(*el.DivEl).W(el.Dp(128)).MaxW(el.Full)
	})}

}
