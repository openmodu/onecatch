package keeldesktop

import (
	"fmt"
	"image/color"
	"math"
	"slices"
	"strings"

	"gioui.org/font"
	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/theme"
	"github.com/openmodu/onecatch/internal/domain/harnesses"
	workflows "github.com/openmodu/onecatch/internal/domain/workflows"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

type runtimeProfileState struct {
	popover                 *kit.PopoverView
	effortSlider            *kit.SliderView
	fastTip, resetTip       *kit.TooltipView
	fastButton, resetButton el.View
	menu                    *kit.MenuView
	menuKey                 string
	codex                   agentrun.CodexConfiguration
	key                     string
	version                 uint64
	loading                 bool
	error                   string
	configuration           agentrun.HarnessConfiguration
	ring                    *kit.ProgressCircleView
	tooltip                 *kit.TooltipView
}

// Match the Wails composer: model settings on the right, context pressure by
// the permission control. Do not combine context usage with cumulative tokens.
func (a *app) runtimeProfilePicker(cx *el.Context) el.Element {
	// Resolve the compact summary on the real client without blocking a frame.
	if a.window != nil && a.profile.key != a.runtimeProfileKey() {
		a.loadRuntimeProfile()
	}
	a.buildRuntimeProfileMenu()
	if len(a.profileEfforts(a.model, false)) > 0 {
		if a.profile.popover == nil {
			a.profile.popover = kit.Popover(el.ViewFunc(a.renderReasoningPicker)).Width(272).Placement(el.Top, el.End).Offset(8).
				PanelStyle(func(panel *el.DivEl) { panel.P(14).Rounded(20).Bg(theme.Surface) })
			a.profile.popover.Trigger(el.ViewFunc(func(cx *el.Context) el.Element {
				return a.runtimeProfileTrigger(cx, func() {
					a.profile.popover.Toggle()
					if a.profile.popover.Value() {
						a.loadRuntimeProfile()
					}
				})
			}))
		}
		return a.profile.popover.Render(cx)
	}
	return a.profile.menu.Render(cx)
}

func (a *app) runtimeProfileTrigger(cx *el.Context, toggle func()) el.Element {
	label := a.runtimeProfileLabel()
	return el.Div().ID("runtime-profile").Role("button").Name("模型设置："+label).Focusable(true).CursorPointer().Row().Items(el.Center).Gap(5).H(el.Dp(28)).MaxW(el.Dp(190)).Px(10).Rounded(14).Bg(theme.SubtleHover).
		Hover(func(s *el.Style) { s.Bg(mixColor(theme.Text, theme.SubtleHover, .04)) }).FocusStyle(neutralFocus).OnClick(toggle).
		Child(el.Text(label).TextSize(12).Weight(font.Normal).MaxLines(1), lucide(cx, "chevron-down", 12, theme.Muted))
}

func (a *app) renderReasoningPicker(cx *el.Context) el.Element {
	values := a.profileEfforts(a.model, false)
	current := firstProfileValue(a.reasoningEffort, a.inheritedProfileEffort())
	index := slices.Index(values, current)
	disabled := a.busy || a.loading || a.profile.loading || a.detail != nil && a.detail.Active
	if a.profile.effortSlider == nil {
		a.profile.effortSlider = kit.Slider("", 0, 1).Step(1).OnChange(func(position float64) {
			choices := a.profileEfforts(a.model, false)
			i := int(position)
			if i < 0 || i >= len(choices) {
				return
			}
			a.reasoningEffort = choices[i]
			if a.reasoningEffort == a.inheritedProfileEffort() {
				a.reasoningEffort = ""
			}
			a.profile.menuKey = ""
		}).Appearance(func(style *kit.SliderAppearance) {
			style.TrackSize, style.ThumbSize = 26, 34
			style.TrackRadius, style.ThumbRadius, style.ThumbBorderWidth = 13, 17, 0
			style.TrackColor = color.NRGBA{}
			style.FillColor = color.NRGBA{}
			style.ThumbColor = theme.Text
		})
	}
	slider := a.profile.effortSlider
	slider.SetRange(0, float64(max(0, len(values)-1)))
	slider.SetValue(float64(max(0, index)))
	slider.SetDisabled(disabled || len(values) < 2)
	fastTier := ""
	for _, tier := range a.profileTiers(a.model) {
		if tier.ID == "priority" || tier.ID == "fast" {
			fastTier = tier.ID
			break
		}
	}
	fast := firstProfileValue(a.serviceTier, a.inheritedProfileTier()) == fastTier && fastTier != ""
	fastButton := el.ViewFunc(func(cx *el.Context) el.Element {
		return el.Div().Disabled(disabled || fastTier == "").Child(iconAction(cx, "reasoning-fast", "快速模式", "zap", func() {
			a.serviceTier = fastTier
			if fast {
				a.serviceTier = "standard"
			}
			if a.serviceTier == a.inheritedProfileTier() {
				a.serviceTier = ""
			}
			a.profile.menuKey = ""
		}, fast))
	})
	resetButton := el.ViewFunc(func(cx *el.Context) el.Element {
		return el.Div().Disabled(disabled).Child(iconAction(cx, "reasoning-reset", "恢复默认模型设置", "rotate-ccw", a.resetProfileSelection, false))
	})
	// Retain tooltip identity while their controls follow the current model.
	if a.profile.fastTip == nil {
		a.profile.fastTip = kit.WithTooltip(el.ViewFunc(func(cx *el.Context) el.Element { return a.profile.fastButton.Render(cx) }), "1.5 倍速度，用量更多")
	}
	a.profile.fastButton = fastButton
	if a.profile.resetTip == nil {
		a.profile.resetTip = kit.WithTooltip(el.ViewFunc(func(cx *el.Context) el.Element { return a.profile.resetButton.Render(cx) }), "重置")
	}
	a.profile.resetButton = resetButton
	a.profile.fastTip.Content(el.ViewFunc(func(cx *el.Context) el.Element {
		return el.Div().Gap(2).Child(el.Text("1.5 倍速度"), el.Text("用量更多"))
	}))
	header := el.Div().Row().Items(el.Center).Child(a.profile.fastTip.Render(cx),
		el.Div().Grow().W(el.Dp(0)).Center().Child(el.Text(profileEffortLabel(current)).TextSize(15).Medium()), a.profile.resetTip.Render(cx))

	// Open the model catalog directly from the model name.
	a.profile.menu.Trigger(el.ViewFunc(func(cx *el.Context) el.Element {
		return el.Div().WFull().ID("reasoning-model").Name("选择模型").Role("button").Focusable(true).CursorPointer().OnClick(a.toggleRuntimeProfileMenu).FocusStyle(neutralFocus).
			Row().Justify(el.Center).Items(el.Center).Gap(4).H(el.Dp(28)).Rounded(8).
			Child(el.Text(a.profileModelLabel(a.model, false)).MaxLines(1).TextSize(13).TextColor(theme.Muted), lucide(cx, "chevron-right", 12, theme.Muted))
	}))
	fraction := float32(0)
	if len(values) > 1 {
		fraction = float32(max(0, index)) / float32(len(values)-1)
	}
	fill := mixColor(theme.Text, theme.Surface, .42)
	// Match the native thumb's center: 17 + fraction * (width - 34).
	// Only the outer end is rounded; the straight join stays under the thumb.
	rail := el.Div().Absolute().Left(0).Right(0).Top(6).H(el.Dp(26)).Rounded(13).Bg(theme.SubtleHover).Row().Child(
		el.Div().W(el.Dp(17)).NoShrink(),
		el.Div().Grow().W(el.Dp(0)).HFull().Row().Child(el.Div().W(el.Frac(fraction)).HFull().Bg(fill)),
		el.Div().W(el.Dp(17)).NoShrink(),
		el.Div().Absolute().Left(0).Top(0).Size(el.Dp(26)).Rounded(13).Bg(fill))
	track := el.Div().WFull().Child(rail, slider.Render(cx).(*el.DivEl).Name("推理强度"))
	if len(values) > 1 {
		dots := el.Div().Absolute().Left(15).Right(15).Top(17).Row().Justify(el.SpaceBetween)
		for i := range values {
			dot := el.Div().Size(el.Dp(4)).Rounded(2).Bg(theme.Muted)
			if i == max(0, index) {
				dot.Opacity(0)
			}
			dots.Child(dot)
		}
		track.Child(dots)
	}
	return el.Div().Gap(6).Child(header, a.profile.menu.Render(cx), track)
}

func (a *app) runtimeProfileKey() string {
	return fmt.Sprintf("%s/%d/%s", a.runtime.Value(), a.settings.Revision, a.workspaceForID(a.workspaceID).Path)
}
func (a *app) profileConfiguration() agentrun.HarnessConfiguration {
	if a.profile.key == a.runtimeProfileKey() {
		return a.profile.configuration
	}
	return agentrun.HarnessConfiguration{}
}
func (a *app) profileCodexConfiguration() agentrun.CodexConfiguration {
	if a.profile.key == a.runtimeProfileKey() {
		return a.profile.codex
	}
	return agentrun.CodexConfiguration{}
}
func firstProfileValue(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
func (a *app) inheritedProfileModel() string {
	return firstProfileValue(a.settings.Runtimes[a.runtime.Value()].DefaultModel, a.profileConfiguration().Model)
}
func (a *app) selectedProfileModel(value string) agentrun.HarnessModel {
	value = firstProfileValue(value, a.inheritedProfileModel())
	for _, model := range a.profileConfiguration().Models {
		if model.Model == value {
			return model
		}
	}
	return agentrun.HarnessModel{Model: value, DisplayName: value}
}
func (a *app) inheritedProfileEffort() string {
	return firstProfileValue(a.settings.Runtimes[a.runtime.Value()].ReasoningEffort, a.profileCodexConfiguration().ReasoningEffort, a.selectedProfileModel(a.model).DefaultEffort)
}
func (a *app) inheritedProfileTier() string {
	return firstProfileValue(a.settings.Runtimes[a.runtime.Value()].ServiceTier, a.profileCodexConfiguration().ServiceTier, "standard")
}
func (a *app) profileModelLabel(value string, compact bool) string {
	model := a.selectedProfileModel(value)
	label := firstProfileValue(model.DisplayName, model.Model, "运行环境默认值")
	if compact && a.runtime.Value() == "codex" {
		if strings.HasPrefix(strings.ToLower(label), "gpt-") {
			label = label[4:]
		}
		label = strings.ReplaceAll(label, "-", " ")
	}
	return label
}
func profileEffortLabel(value string) string {
	labels := map[string]string{"none": "无", "off": "关闭", "minimal": "最小", "low": "轻度", "medium": "中", "high": "高", "xhigh": "极高", "max": "最大", "ultra": "Ultra（多 Agent）"}
	return firstProfileValue(labels[value], value, "运行环境默认值")
}
func (a *app) runtimeProfileLabel() string {
	label := a.profileModelLabel(a.model, true)
	capability, _ := harnesses.Find(a.runtime.Value())
	effort := firstProfileValue(a.reasoningEffort, a.inheritedProfileEffort())
	if a.runtime.Value() != "claude" && len(capability.Efforts) > 0 && effort != "" {
		label += " " + profileEffortLabel(effort)
	}
	return label
}
func uniqueProfileValues(values []string) []string {
	var result []string
	for _, value := range values {
		if value != "" && !slices.Contains(result, value) {
			result = append(result, value)
		}
	}
	return result
}
func (a *app) profileEfforts(model string, includeCurrent bool) []string {
	configured := a.profileConfiguration()
	values := slices.Clone(a.selectedProfileModel(model).Efforts)
	if len(values) == 0 {
		values = slices.Clone(configured.Efforts)
	}
	if a.runtime.Value() == "codex" {
		// Keep the five user-facing levels even if Codex advertises extras.
		var levels []string
		for _, level := range []string{"low", "medium", "high", "xhigh", "max"} {
			if slices.Contains(values, level) {
				levels = append(levels, level)
			}
		}
		return levels
	}
	// A saved preference must not add an unsupported stop to a model's slider.
	if len(values) == 0 && model == "" {
		values = append(values, a.profileCodexConfiguration().ReasoningEffort)
	}
	if len(values) == 0 && includeCurrent {
		values = append(values, firstProfileValue(a.reasoningEffort, a.settings.Runtimes[a.runtime.Value()].ReasoningEffort))
	}
	return uniqueProfileValues(values)
}
func (a *app) profileTiers(model string) []agentrun.CodexServiceTier {
	selected := a.selectedProfileModel(model).Model
	for _, item := range a.profileCodexConfiguration().Models {
		if item.Model == selected {
			return item.ServiceTiers
		}
	}
	return nil
}
func (a *app) setProfileModel(model string) {
	a.model = model
	capability, _ := harnesses.Find(a.runtime.Value())
	if len(capability.Efforts) == 0 {
		a.reasoningEffort = ""
	} else if a.reasoningEffort != "" && !slices.Contains(a.profileEfforts(model, false), a.reasoningEffort) {
		a.reasoningEffort = a.selectedProfileModel(model).DefaultEffort
	}
	tiers := []string{"standard"}
	for _, tier := range a.profileTiers(model) {
		tiers = append(tiers, tier.ID)
	}
	if !capability.ServiceTiers {
		a.serviceTier = ""
	} else if a.serviceTier != "" && !slices.Contains(tiers, a.serviceTier) {
		a.serviceTier = "standard"
	}
	a.profile.menuKey = ""
}
func (a *app) resetProfileSelection() {
	a.model, a.reasoningEffort, a.serviceTier = "", "", ""
	a.profile.menuKey = ""
	if a.profile.menu != nil {
		a.profile.menu.SetValue(false)
	}
}

// Keel owns model-list positioning, scrolling, focus, Escape and arrow navigation.
func (a *app) buildRuntimeProfileMenu() {
	disabled := a.busy || a.loading || a.detail != nil && a.detail.Active
	key := fmt.Sprintf("%s/%s/%d/%t/%t/%s/%s/%s/%s", a.runtimeProfileKey(), a.profile.key, a.profile.version, disabled, a.profile.loading, a.profile.error, a.model, a.reasoningEffort, a.serviceTier)
	if a.profile.menu != nil && a.profile.menuKey == key {
		return
	}
	open := a.profile.menu != nil && a.profile.menu.Value()
	menu := kit.Menu().Width(300).CheckSide(el.Right).Placement(el.Top, el.End).Offset(8).Label("选择模型")
	a.profile.menu, a.profile.menuKey = menu, key
	menu.Trigger(el.ViewFunc(func(cx *el.Context) el.Element {
		return a.runtimeProfileTrigger(cx, a.toggleRuntimeProfileMenu)
	}))
	inheritedDescription := "跟随运行环境配置"
	if a.runtime.Value() == "codex" {
		inheritedDescription = "跟随 Codex 配置"
	}
	if a.runtime.Value() == "claude" {
		inheritedDescription = "跟随 Claude Code 配置"
	}
	addProfileChoice(menu, "默认", inheritedDescription, a.model == "", func() { a.setProfileModel("") })
	menu.SetItemDisabled("默认", disabled || a.profile.loading)
	for _, model := range a.profileConfiguration().Models {
		model := model
		label := firstProfileValue(model.DisplayName, model.Model)
		addProfileChoice(menu, label, modelCapacityDescription(model.Load), a.model == model.Model, func() { a.setProfileModel(model.Model) })
		menu.SetItemDisabled(label, disabled || a.profile.loading)
	}
	if a.profile.loading {
		menu.Separator().Label("正在读取模型配置…")
	}
	if a.profile.error != "" {
		menu.Separator().Item("读取配置失败，点击重试", "", func() { a.profile.key = ""; a.loadRuntimeProfile() })
	}
	menu.SetValue(open)
}

func (a *app) toggleRuntimeProfileMenu() {
	a.profile.menu.Toggle()
	if !a.profile.menu.Value() {
		return
	}
	// Trae's load and queue lengths change independently of saved settings.
	// Refresh on an explicit menu open; there is no background polling.
	if a.runtime.Value() == "trae" && !a.profile.loading {
		a.profile.key = ""
	}
	a.loadRuntimeProfile()
}

// Capacity is a provider snapshot, distinct from this run's queue position.
func modelCapacityDescription(load *agentrun.HarnessModelLoad) string {
	if load == nil {
		return ""
	}
	var details []string
	if load.Percent != nil {
		details = append(details, fmt.Sprintf("负载 %d%%", *load.Percent))
	}
	if load.QueueSize != nil {
		if *load.QueueSize == 0 {
			details = append(details, "暂无排队")
		} else {
			details = append(details, fmt.Sprintf("排队 %d 人", *load.QueueSize))
		}
	}
	return strings.Join(details, " · ")
}

func addProfileChoice(menu *kit.MenuView, label, description string, selected bool, selectOption func()) {
	menu.CheckItem(label, "", selected, func(bool) { selectOption() })
	menu.SetItemContent(label, el.ViewFunc(func(*el.Context) el.Element {
		content := el.Div().W(el.Dp(190)).Gap(3).Py(3).Child(el.Text(label).TextSize(13).Medium().MaxLines(1))
		if description != "" {
			content.Child(el.Text(description).TextSize(11).Weight(font.Normal).TextColor(theme.Muted).MaxLines(1))
		}
		return content
	}))
}

// Inspection happens off the UI thread when the composer first needs its profile
// or the menu retries. Results belong to their runtime/settings/workspace snapshot.
func (a *app) loadRuntimeProfile() {
	key := a.runtimeProfileKey()
	if key == a.profile.key && (a.profile.loading || a.profile.error == "") {
		return
	}
	runtime, settings, path := a.runtime.Value(), a.settings.Runtimes[a.runtime.Value()], a.workspaceForID(a.workspaceID).Path
	a.profile.version++
	version := a.profile.version
	a.profile.key, a.profile.loading, a.profile.error = key, true, ""
	a.profile.configuration = agentrun.HarnessConfiguration{}
	a.profile.codex = agentrun.CodexConfiguration{}
	service, ctx := a.backend.service, a.ctx
	go func() {
		var configuration agentrun.HarnessConfiguration
		var codex agentrun.CodexConfiguration
		var err error
		switch runtime {
		case "codex":
			var source agentrun.CodexConfiguration
			source, err = service.InspectCodexConfiguration(ctx, settings, path)
			codex = source
			configuration.Model = source.Model
			for _, model := range source.Models {
				if configuration.Model == "" && model.IsDefault {
					configuration.Model = model.Model
				}
				configuration.Models = append(configuration.Models, agentrun.HarnessModel{Model: model.Model, DisplayName: model.DisplayName, Description: model.Description, DefaultEffort: model.DefaultReasoningEffort, Efforts: model.ReasoningEfforts})
			}
		case "claude":
			var source agentrun.ClaudeConfiguration
			source, err = service.InspectClaudeConfiguration(ctx, settings, path)
			configuration.Model, configuration.Efforts = source.Model, source.Efforts
			for _, model := range source.Models {
				configuration.Models = append(configuration.Models, agentrun.HarnessModel{Model: model.Model, DisplayName: model.DisplayName})
			}
		default:
			configuration, err = service.InspectHarnessConfiguration(ctx, runtime, settings)
		}
		core.Update(func() {
			if a.closed || a.profile.key != key || a.profile.version != version {
				return
			}
			a.profile.loading, a.profile.configuration = false, configuration
			a.profile.codex, a.profile.menuKey = codex, ""
			if err != nil {
				a.profile.error = err.Error()
			}
		})
	}()
}

type contextReading struct {
	window, tokens   int
	ratio            float64
	known, remaining bool
}

func summarizeContextWindow(steps []workflows.StepRun, harness string) contextReading {
	var reading contextReading
	for _, step := range steps {
		if step.ContextWindow > 0 {
			reading.window = step.ContextWindow
		}
		if step.ContextTokens > 0 {
			reading.tokens = step.ContextTokens
		}
	}
	reading.known = reading.window > 0 && reading.tokens > 0
	if !reading.known {
		return reading
	}
	reading.ratio = min(float64(reading.tokens)/float64(reading.window), 1)
	if harness == "codex" {
		reading.remaining = true
		effective := reading.window - 12000
		reading.ratio = 0
		if effective > 0 {
			reading.ratio = min(max(float64(effective-max(reading.tokens-12000, 0))/float64(effective), 0), 1)
		}
	}
	return reading
}
func (a *app) contextReading() contextReading {
	if a.detail == nil {
		return contextReading{}
	}
	return summarizeContextWindow(a.detail.StepRuns, a.continuationHarness())
}
func (a *app) contextGauge(cx *el.Context) el.Element {
	reading := a.contextReading()
	if !reading.known {
		return nil
	}
	percent := int(math.Round(float64(reading.ratio) * 100))
	label := fmt.Sprintf("上下文窗口已用 %d%%：%d / %d Token", percent, reading.tokens, reading.window)
	if reading.remaining {
		label = fmt.Sprintf("上下文剩余 %d%%：%d / %d Token", percent, reading.tokens, reading.window)
	}
	if a.profile.tooltip == nil {
		a.profile.tooltip = kit.WithTooltip(el.ViewFunc(a.renderContextGauge), label).Offset(6)
	}
	a.profile.tooltip.SetText(label)
	return a.profile.tooltip.Render(cx)
}
func (a *app) renderContextGauge(cx *el.Context) el.Element {
	reading := a.contextReading()
	pressure := reading.ratio
	if reading.remaining {
		pressure = 1 - pressure
	}
	arc, fg := theme.Primary, theme.Text
	if pressure >= .9 {
		arc, fg = theme.Danger, theme.Danger
	} else if pressure >= .75 {
		arc, fg = theme.Warning, theme.Warning
	}
	if a.profile.ring == nil {
		a.profile.ring = kit.ProgressCircle("上下文窗口").Size(19)
	}
	a.profile.ring.SetValue(float32(reading.ratio))
	a.profile.ring.Color(arc)
	ringPalette := theme.Current()
	ringPalette.Subtle = mixColor(theme.Muted, theme.Bg, .16)
	ring := cx.Themed(ringPalette, el.ViewFunc(func(cx *el.Context) el.Element { return a.profile.ring.Render(cx) }))
	return el.Div().ID("context-gauge").Row().Items(el.Center).Gap(6).Child(ring, el.Text(fmt.Sprintf("%d%%", int(math.Round(float64(reading.ratio)*100)))).TextSize(12).Weight(font.Normal).TextColor(fg))
}
