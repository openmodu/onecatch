package keeldesktop

import (
	"fmt"
	"maps"
	"strconv"

	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/theme"
	domainsettings "github.com/openmodu/onecatch/internal/domain/settings"
	desktopservice "github.com/openmodu/onecatch/internal/service/desktop"
)

type settingsActions struct {
	busy           string
	usageRequested bool
	usageChart     *kit.ChartView
	usage          *desktopservice.StorageUsage
	preview        *desktopservice.CleanupPreview
	previewDays    int
	diagnosticPath string
	prompt, events bool
}

func (p *settingsPage) settingsConfirmation(cx *el.Context) el.Element {
	if p.confirm == nil {
		p.confirm = kit.Dialog("").Width(440)
	}
	return p.confirm.Render(cx)
}

// Capture requests on the UI thread; only apply their results if this settings
// instance is still alive. Disk scans and exports never run during rendering.
func (p *settingsPage) settingsAction(name string, work func() (func(), error)) {
	if p.actions.busy != "" || p.saving || p.workers.closed {
		return
	}
	p.actions.busy = name
	go func() {
		apply, err := work()
		core.Update(func() {
			if p.app.closed || p.workers.closed {
				return
			}
			p.actions.busy = ""
			if err != nil {
				p.setStatusError(err)
				return
			}
			if apply != nil {
				apply()
			}
		})
	}()
}

func (p *settingsPage) refreshStorageUsage() {
	p.settingsAction("usage", func() (func(), error) {
		usage, err := p.app.backend.service.GetStorageUsage()
		return func() {
			p.actions.usage = &usage
			var labels []string
			var values []float64
			for _, category := range usage.Categories {
				labels = append(labels, category.Name)
				values = append(values, float64(category.Bytes))
			}
			p.actions.usageChart = kit.BarChart(labels, kit.Series{Name: uiText("存储占用"), Values: values}).Controls(false).BarAlignment(kit.BarAlignmentLeft).Height(float32(max(120, len(labels)*36))).Format(func(value float64) string { return skillSize(int64(value)) }).TooltipContent(func(_ *el.Context, point kit.ChartTooltip) el.Element {
				if point.Index < 0 || point.Index >= len(usage.Categories) {
					return el.Div()
				}
				category := usage.Categories[point.Index]
				return el.Div().Gap(4).Child(el.Text(category.Name).Medium(), el.Text(fmt.Sprintf("%s · %d %s", skillSize(category.Bytes), category.Files, uiText("个文件"))))
			})
		}, err
	})
}
func (p *settingsPage) previewCleanup() {
	days, err := strconv.Atoi(p.value("retention-days", strconv.Itoa(p.draft.Storage.CompletedRunRetentionDays)))
	if err != nil || days == 0 {
		p.status = "请先选择 30、90 或 180 天的保留期限"
		return
	}
	p.actions.preview = nil
	ctx := p.workerContext()
	p.settingsAction("preview", func() (func(), error) {
		preview, err := p.app.backend.service.PreviewCleanup(ctx, desktopservice.CleanupPreviewInput{RetentionDays: days})
		return func() { p.actions.preview, p.actions.previewDays = &preview, days }, err
	})
}
func (p *settingsPage) confirmCleanup() {
	preview := p.actions.preview
	if preview == nil || preview.Token == "" || preview.Count == 0 {
		return
	}
	if p.value("retention-days", strconv.Itoa(p.draft.Storage.CompletedRunRetentionDays)) != strconv.Itoa(p.actions.previewDays) {
		p.actions.preview = nil
		p.status = "保留期限已改变，请重新预览"
		return
	}
	p.confirm.ConfirmDanger("清理历史运行", fmt.Sprintf("将永久删除 %d 次已完成的历史运行，预计释放 %s。此操作不可撤销。", preview.Count, skillSize(preview.EstimatedBytes)), "确认清理", func() {
		ctx, token := p.workerContext(), preview.Token
		p.settingsAction("cleanup", func() (func(), error) {
			result, err := p.app.backend.service.ExecuteCleanup(ctx, token)
			return func() {
				p.actions.preview = nil
				p.status = fmt.Sprintf("已清理 %d 次历史运行，释放 %s", len(result.RemovedRunIDs), skillSize(result.RemovedBytes))
				p.refreshStorageUsage()
				p.app.refresh()
			}, err
		})
	})
}
func (p *settingsPage) exportDiagnostics() {
	input := desktopservice.DiagnosticsExportInput{Destination: p.actions.diagnosticPath, IncludePrompt: p.actions.prompt && p.app.settings.Security.DiagnosticsIncludePrompt, IncludeRawEvents: p.actions.events && p.app.settings.Security.DiagnosticsIncludeRawEvents}
	action := func() {
		ctx := p.workerContext()
		p.settingsAction("export", func() (func(), error) {
			result, err := p.app.backend.service.ExportDiagnostics(ctx, input)
			return func() { p.status = "诊断包已导出：" + result.Path }, err
		})
	}
	if input.IncludePrompt || input.IncludeRawEvents {
		p.confirm.ConfirmDanger("导出敏感诊断信息", "诊断包将包含你选择的任务指令或原始事件。请确认后再导出。", "导出", action)
	} else {
		action()
	}
}
func (p *settingsPage) discardSettings() {
	p.draft = p.app.settings
	p.draft.Runtimes = maps.Clone(p.app.settings.Runtimes)
	p.fields, p.selects, p.switches = nil, nil, nil
	p.currentRuntime = ""
	p.selectRuntime(p.runtime.Value())
	p.status = "已放弃未保存的修改"
}
func (p *settingsPage) resetSettings() {
	section, revision := p.section, p.draft.Revision
	if section == "harness" {
		section = domainsettings.SectionRuntime
	}
	p.confirm.ConfirmDanger("恢复默认设置", "只恢复当前分类，其他分类的设置保持不变。", "恢复默认值", func() {
		ctx := p.workerContext()
		p.settingsAction("reset", func() (func(), error) {
			value, err := p.app.backend.service.ResetSettingsSection(ctx, section, revision)
			return func() { p.app.settings = value; p.discardSettings(); p.status = "已恢复默认值"; p.app.refresh() }, err
		})
	})
}
func (p *settingsPage) storageActions() []kit.SettingGroup {
	action := func(label, name string, fn func()) el.View {
		return el.ViewFunc(func(cx *el.Context) el.Element {
			b := button(label, fn, kit.ButtonGhost).Loading(p.actions.busy == name)
			b.SetDisabled(p.actions.busy != "" || p.saving)
			return b.Render(cx)
		})
	}
	return []kit.SettingGroup{
		{Title: "本机数据", Items: []kit.SettingItem{
			{Label: "数据目录", Description: p.app.backend.service.DataRoot(), Control: action("在文件管理器中显示", "reveal", func() {
				p.settingsAction("reveal", func() (func(), error) { return nil, p.app.backend.service.RevealDataRoot() })
			})},
			{Label: "存储占用", Control: action("重新计算", "usage", p.refreshStorageUsage), Content: el.ViewFunc(func(cx *el.Context) el.Element {
				root := el.Div().Py(12).Gap(10).Child(el.Div().Row().Items(el.Center).Child(el.Text(uiText("存储占用")).Grow(), action("重新计算", "usage", p.refreshStorageUsage).Render(cx)))
				if p.actions.usage == nil {
					label := uiText("正在读取…")
					if p.actions.busy != "usage" {
						label = uiText("读取失败，请重新计算")
					}
					return root.Child(el.Text(label).TextSize(12).TextColor(theme.Muted))
				}
				if p.actions.usageChart != nil {
					root.Child(p.actions.usageChart.Render(cx))
				}
				return root
			})},
		}},
		{Title: "历史清理", Items: []kit.SettingItem{
			p.settingsNumber("retention-days", "完成任务保留天数（0 为永久保留）", func() int { return p.draft.Storage.CompletedRunRetentionDays }),
			{Label: "清理预览", Description: "先预览将删除的历史运行，再确认清理。支持 30、90、180 天。", Control: action("预览清理", "preview", p.previewCleanup), Content: el.ViewFunc(func(cx *el.Context) el.Element {
				row := el.Div().WFull().Py(12).Wrap().Justify(el.End).Gap(12).Items(el.Center).Child(action("预览清理", "preview", p.previewCleanup).Render(cx))
				if v := p.actions.preview; v != nil {
					row.Child(el.Text(fmt.Sprintf("%d 次历史运行 · 预计释放 %s", v.Count, skillSize(v.EstimatedBytes))).Grow())
					if v.Count > 0 {
						row.Child(action("清理历史运行", "cleanup", p.confirmCleanup).Render(cx))
					}
				}
				return row
			})},
		}},
		{Title: "日志轮转", Items: []kit.SettingItem{
			p.settingsChoice("log-level", "日志级别", func() string { return p.draft.Storage.LogLevel }, []string{"error", "warn", "info", "debug"}),
			p.settingsNumber("log-size", "单个日志大小（MB）", func() int { return p.draft.Storage.LogMaxSizeMB }),
			p.settingsNumber("log-backups", "日志备份数", func() int { return p.draft.Storage.LogMaxBackups }),
			p.settingsNumber("log-age", "日志保留天数", func() int { return p.draft.Storage.LogMaxAgeDays }),
		}},
		{Title: "导出诊断", Items: []kit.SettingItem{
			{Label: "ZIP 保存路径", Description: "留空保存到数据目录。", Control: el.ViewFunc(func(*el.Context) el.Element {
				return field("diagnostic-path", "onecatch-diagnostics.zip", &p.actions.diagnosticPath).Name("ZIP 保存路径")
			})},
			{Label: "包含任务指令", Description: "需先在安全设置中授权，导出前会再次确认。", Control: el.ViewFunc(func(cx *el.Context) el.Element {
				v := p.diagnosticToggle("export-prompt", p.app.settings.Security.DiagnosticsIncludePrompt, &p.actions.prompt)
				return v.Render(cx)
			})},
			{Label: "包含原始事件", Description: "需先在安全设置中授权，导出前会再次确认。", Control: el.ViewFunc(func(cx *el.Context) el.Element {
				v := p.diagnosticToggle("export-events", p.app.settings.Security.DiagnosticsIncludeRawEvents, &p.actions.events)
				return v.Render(cx)
			})},
			{Label: "诊断包", Control: action("导出 ZIP", "export", p.exportDiagnostics)},
		}},
	}
}
func (p *settingsPage) diagnosticToggle(id string, allowed bool, target *bool) el.View {
	control := p.switches[id]
	if control == nil {
		control = newFilledToggle(id, false, false)
		control.hideLabel = true
		control.onChange = func(v bool) { *target = v }
		p.switches[id] = control
	}
	if !allowed {
		*target = false
	}
	control.SetValue(*target)
	return el.ViewFunc(func(cx *el.Context) el.Element { return el.Div().Disabled(!allowed).Child(control.Render(cx)) })
}

func chatFontSize(value string) float32 {
	switch value {
	case "small":
		return 12
	case "large":
		return 16
	case "extra-large":
		return 18
	default:
		return 14
	}
}
func (a *app) setChatFontSize(value string) {
	a.ui.Preferences.ChatFontSize = value
	a.ui.Preferences = normalizeUIPreferences(a.ui.Preferences)
	a.scroller.Invalidate()
	a.saveUIPreferences()
}
