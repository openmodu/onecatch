package keeldesktop

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/theme"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

func (a *app) usagePage(cx *el.Context) el.Element {
	now := time.Now()
	width, _ := cx.ViewportSize()
	if !a.sidebarCollapsed(cx) {
		width -= a.sidebarWidth(cx) + 1
	}
	padding := float32(28)
	if width < 700 {
		padding = 16
	}
	root := el.Div().ID("usage-page").Grow().H(el.Dp(0)).ScrollY().P(padding).Gap(24)
	refresh := iconButton("刷新", func() { a.loadUsage(true) }, kit.ButtonGhost, kit.IconRetry).Loading(a.library.UsageLoading)
	root.Child(el.Div().Row().Items(el.Center).Child(el.Text("用量").Grow().TextSize(24).Medium(), refresh.Render(cx)))
	filters := el.Div().Row().Wrap().Gap(6)
	filters.Child(button("全部", func() { a.library.UsageFilter = "" }, kit.ButtonGhost).Selected(a.library.UsageFilter == "").Render(cx))
	for _, usage := range a.library.Usage {
		id := string(usage.Runtime)
		filters.Child(button(usageProviderName(id), func() { a.library.UsageFilter = id }, kit.ButtonGhost).Selected(a.library.UsageFilter == id).Render(cx))
	}
	root.Child(filters)
	aggregate := summarizeUsage(a.library.Usage, a.library.UsageFilter, now)
	metrics := el.Div().Row().Wrap().Gap(12)
	for _, m := range []struct {
		label     string
		value     int64
		available bool
	}{
		{"累计 Token", aggregate.Lifetime, aggregate.Available[0]}, {"单日峰值", aggregate.Peak, aggregate.Available[1]},
		{"连续活跃天数", aggregate.Current, aggregate.Available[2]}, {"最长连续天数", aggregate.Longest, aggregate.Available[3]},
	} {
		value := "—"
		if m.available {
			value = formatNumber(m.value)
		}
		metrics.Child(el.Div().Grow().MinW(el.Dp(135)).P(16).Rounded(10).Bg(theme.Subtle).Gap(8).Child(el.Text(m.label).TextSize(12).TextColor(theme.Muted), el.Text(value).TextSize(22).Medium().MaxLines(1)))
	}
	root.Child(metrics)
	if len(aggregate.Daily) > 0 {
		heatmapID := "usage-activity/" + a.library.UsageFilter
		_, viewport, _ := cx.ScrollStateX(heatmapID)
		if viewport == 0 {
			a.library.UsageHeatmapReady = ""
		}
		if viewport > 0 && a.library.UsageHeatmapReady != heatmapID {
			cx.ScrollToX(heatmapID, 1000000)
			a.library.UsageHeatmapReady = heatmapID
		}
		root.Child(usageHeatmap(aggregate.Daily, now, width-2*padding, heatmapID))
	}
	for _, usage := range a.library.Usage {
		id := string(usage.Runtime)
		if a.library.UsageFilter != "" && a.library.UsageFilter != id {
			continue
		}
		section := el.Div().Gap(14)
		heading := el.Div().Row().Wrap().Items(el.Center).Gap(8).Child(runtimeMark(cx, id, 18), el.Text(usageProviderName(id)).TextSize(16).Medium())
		scope := usage.DailyUsageScope
		if scope == "" {
			scope = usage.Scope
		}
		scopeLabel := "用量范围未知"
		switch scope {
		case agentrun.AccountUsageScopeAccount:
			scopeLabel = "账户活动"
		case agentrun.AccountUsageScopeDevice:
			scopeLabel = "本机活动"
		case agentrun.AccountUsageScopeMixed:
			scopeLabel = "账户与本机活动"
		}
		heading.Child(el.Text(scopeLabel).TextSize(11).TextColor(theme.Muted))
		section.Child(heading)
		for _, limit := range usage.RateLimits {
			name := limit.Name
			if name == "" {
				name = limit.ID
			}
			title := name
			if limit.PlanType != "" {
				title += " · " + limit.PlanType
			}
			card := el.Div().P(16).Rounded(10).Bg(theme.Subtle).Gap(12)
			if title != "" {
				card.Child(el.Text(title).TextSize(12).TextColor(theme.Muted))
			}
			windows := el.Div().Row().Wrap().Gap(20)
			for _, w := range []*agentrun.AccountRateLimitWindow{limit.Primary, limit.Secondary} {
				if w == nil {
					continue
				}
				percent := max(0, min(100, w.UsedPercent))
				color := theme.Primary
				if percent >= 90 {
					color = theme.DangerText
				}
				progress := kit.Progress("").Height(5).Color(color)
				progress.SetValue(float32(percent) / 100)
				reset := "重置时间未知"
				if w.ResetsAt > 0 {
					reset = "重置于 " + time.Unix(w.ResetsAt, 0).Local().Format("01-02 15:04 MST -07:00")
				}
				windows.Child(el.Div().Grow().MinW(el.Dp(200)).Gap(10).Child(el.Div().Row().Child(el.Text(usageWindowLabel(w.WindowDurationMins)).Grow().TextSize(13), el.Text(fmt.Sprintf("已用 %d%%", percent)).TextSize(13).Medium().TextColor(color)), progress.Render(cx), el.Text(reset).TextSize(11).TextColor(theme.Muted)))
			}
			card.Child(windows)
			if limit.QuotaRemaining != nil {
				card.Child(el.Text(fmt.Sprintf("剩余额度：%d", *limit.QuotaRemaining)).TextSize(12))
			}
			section.Child(card)
		}
		if len(usage.RateLimits) == 0 {
			section.Child(el.Text("此来源未提供账户限额").TextSize(12).TextColor(theme.Muted))
		}
		note := usage.Source
		if !usage.FetchedAt.IsZero() {
			note += " · 更新于 " + usage.FetchedAt.Local().Format("01-02 15:04 MST -07:00")
		}
		if note != "" {
			section.Child(el.Text(strings.TrimPrefix(note, " · ")).TextSize(11).TextColor(theme.Muted))
		}
		if usage.Warning != "" {
			section.Child(kit.Alert(usage.Warning).Size(kit.AlertSizeSmall).Tone(kit.ToneWarning).Render(cx))
		}
		root.Child(section)
	}
	errors := make([]string, 0, len(a.library.UsageErrors))
	for id := range a.library.UsageErrors {
		if a.library.UsageFilter != "" && a.library.UsageFilter != id {
			continue
		}
		errors = append(errors, id)
	}
	sort.Strings(errors)
	for _, id := range errors {
		root.Child(kit.Alert(usageProviderName(id) + "：" + a.library.UsageErrors[id]).Size(kit.AlertSizeSmall).Tone(kit.ToneDanger).Render(cx))
	}
	if len(a.library.Usage) == 0 {
		message := "暂无用量数据，请配置并登录支持用量查询的 Agent。"
		if a.library.UsageLoading {
			message = "正在加载用量…"
		}
		root.Child(el.Div().Py(32).Child(el.Text(message).TextSize(13).TextColor(theme.Muted)))
	}
	if len(aggregate.Daily) > 0 {
		key := fmt.Sprintf("%d/%s/%s/%t", a.library.UsageVersion, a.library.UsageFilter, now.Format("2006-01-02"), a.library.UsageLoading)
		if a.library.UsageChart == nil || a.library.UsageChartKey != key {
			labels := make([]string, 30)
			values := make([]float64, 30)
			for i := range labels {
				day := now.AddDate(0, 0, i-29).Format("2006-01-02")
				labels[i] = day[5:]
				values[i] = float64(aggregate.Daily[day])
			}
			a.library.UsageChart = kit.BarChart(labels, kit.Series{Name: "Token", Values: values}).Controls(false).Height(220).Format(usageAxisNumber).TooltipContent(func(_ *el.Context, point kit.ChartTooltip) el.Element {
				return el.Div().Gap(4).Child(el.Text(point.Label).TextSize(12), el.Text(formatNumber(int64(point.Values[0].Value))+" Token").TextSize(13).Medium())
			})
			a.library.UsageChartKey = key
		}
		a.library.UsageChart.SeriesStyle(0, kit.ChartSeriesStyle{Stroke: &theme.Primary, Fill: &theme.Primary})
		root.Child(el.Div().Gap(16).Child(el.Text("近 30 天").TextSize(16).Medium(), a.library.UsageChart.Render(cx)))
	}
	return root
}

func usageProviderName(id string) string {
	switch id {
	case "codex":
		return "Codex"
	case "claude":
		return "Claude Code"
	case "modu":
		return "Modu Code"
	case "pi":
		return "Pi"
	}
	return id
}

func usageWindowLabel(minutes int64) string {
	if minutes <= 0 {
		return "用量窗口"
	}
	if minutes%1440 == 0 {
		return fmt.Sprintf("%d 天限额", minutes/1440)
	}
	if minutes%60 == 0 {
		return fmt.Sprintf("%d 小时限额", minutes/60)
	}
	return fmt.Sprintf("%d 分钟限额", minutes)
}

func formatNumber(value int64) string {
	raw := fmt.Sprint(value)
	var groups []string
	for len(raw) > 3 {
		groups = append([]string{raw[len(raw)-3:]}, groups...)
		raw = raw[:len(raw)-3]
	}
	return strings.Join(append([]string{raw}, groups...), ",")
}
func usageHeatmap(daily map[string]int64, now time.Time, width float32, id string) el.Element {
	cellSize := max(float32(8), (width-52*4)/53)
	end := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	start := end.AddDate(0, 0, -52*7-int(end.Weekday()))
	var visible []int64
	for day := start; !day.After(end); day = day.AddDate(0, 0, 1) {
		if value := daily[day.Format("2006-01-02")]; value > 0 {
			visible = append(visible, value)
		}
	}
	sort.Slice(visible, func(i, j int) bool { return visible[i] < visible[j] })
	grid := el.Div().ID(id).Row().Gap(4).ScrollX().Scrollbars(el.ScrollbarHover)
	for week := 0; week < 53; week++ {
		column := el.Div().Gap(4).Pb(12).NoShrink()
		for weekday := 0; weekday < 7; weekday++ {
			date := start.AddDate(0, 0, week*7+weekday)
			tokens := daily[date.Format("2006-01-02")]
			level := 0
			if tokens > 0 && len(visible) > 0 {
				level = usageHeatLevel(tokens, visible)
			}
			fg := theme.Subtle
			if level > 0 {
				fg = theme.Primary
				fg.A = uint8(50 + level*50)
			}
			if date.After(end) {
				fg.A = 0
			}
			column.Child(el.Div().Size(el.Dp(cellSize)).NoShrink().Rounded(3).Bg(fg).Role("gridcell").Name(date.Format("2006-01-02") + " · " + formatNumber(tokens) + " Token"))
		}
		grid.Child(column)
	}
	return el.Div().Gap(12).Child(el.Text("活跃记录").TextSize(16).Medium(), el.Text("过去 53 周 · 每日 Token").TextSize(12).TextColor(theme.Muted), grid)
}

func usageAxisNumber(value float64) string {
	switch {
	case value >= 1e9:
		return fmt.Sprintf("%.1fB", value/1e9)
	case value >= 1e6:
		return fmt.Sprintf("%.0fM", value/1e6)
	case value >= 1e3:
		return fmt.Sprintf("%.0fK", value/1e3)
	default:
		return fmt.Sprintf("%.0f", value)
	}
}
