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
	root := el.Div().ID("usage-page").Grow().H(el.Dp(0)).ScrollY().P(32).Gap(20)
	refresh := iconButton("刷新", func() { a.loadUsage(true) }, kit.ButtonGhost, kit.IconRetry).Loading(a.library.UsageLoading)
	root.Child(el.Div().Row().Items(el.Center).Gap(16).Child(lucide(cx, "chart-no-axes-column", 28, theme.Primary), el.Div().Grow().Gap(8).Child(el.Text("Agent 用量").TextSize(28).Medium(), el.Text("汇总各运行时的每日 Token 用量和账户限额。").TextSize(13).TextColor(theme.Muted)), refresh.Render(cx)))
	filters := el.Div().Row().Wrap().Gap(8)
	filters.Child(button("全部", func() { a.library.UsageFilter = "" }, kit.ButtonGhost).Selected(a.library.UsageFilter == "").Render(cx))
	for _, usage := range a.library.Usage {
		id := string(usage.Runtime)
		filters.Child(button(id, func() { a.library.UsageFilter = id }, kit.ButtonGhost).Selected(a.library.UsageFilter == id).Render(cx))
	}
	root.Child(el.Div().P(18).Gap(12).Rounded(12).Bg(theme.Surface).Child(el.Text("用量来源").TextSize(15).Medium(), el.Text("账户限额与本机活动按来源分别展示。").TextSize(12).TextColor(theme.Muted), filters))
	aggregate := summarizeUsage(a.library.Usage, a.library.UsageFilter, time.Now())
	daily := aggregate.Daily
	metrics := el.Div().Row().Wrap().Gap(12)
	for _, m := range []struct {
		Label     string
		Value     int64
		Available bool
	}{{"累计 Token", aggregate.Lifetime, aggregate.Available[0]}, {"单日峰值", aggregate.Peak, aggregate.Available[1]}, {"当前连续天数", aggregate.Current, aggregate.Available[2]}, {"最长连续天数", aggregate.Longest, aggregate.Available[3]}} {
		value := "—"
		if m.Available {
			value = formatNumber(m.Value)
		}
		metrics.Child(el.Div().Grow().MinW(el.Dp(150)).P(18).Rounded(12).Bg(theme.Surface).Gap(10).Child(el.Text(m.Label).TextSize(12).TextColor(theme.Muted), el.Text(value).Name(value).TextSize(20).Medium().MaxLines(1)))
	}
	root.Child(metrics, usageHeatmap(daily, time.Now()))
	for _, usage := range a.library.Usage {
		id := string(usage.Runtime)
		if a.library.UsageFilter != "" && a.library.UsageFilter != id {
			continue
		}
		for _, limit := range usage.RateLimits {
			name := limit.Name
			if name == "" {
				name = limit.ID
			}
			card := el.Div().P(20).Rounded(12).Bg(theme.Surface).Gap(12).Child(el.Div().Row().Items(el.Center).Gap(8).Child(runtimeMark(cx, id, 16), el.Text(id+" · "+name).Medium().TextSize(15)), el.Text(limit.PlanType).TextSize(11).TextColor(theme.Muted))
			windows := el.Div().Row().Wrap().Gap(12)
			for _, w := range []*agentrun.AccountRateLimitWindow{limit.Primary, limit.Secondary} {
				if w == nil {
					continue
				}
				percent := max(0, min(100, w.UsedPercent))
				reset := "重置时间未知"
				if w.ResetsAt > 0 {
					reset = "重置于 " + time.Unix(w.ResetsAt, 0).Local().Format("01-02 15:04")
				}
				progress := kit.Progress("").Height(6).Color(theme.Primary)
				progress.SetValue(float32(percent) / 100)
				windows.Child(el.Div().Grow().MinW(el.Dp(180)).P(16).Rounded(8).Bg(theme.Subtle).Gap(12).Child(el.Text(fmt.Sprintf("%d 分钟窗口", w.WindowDurationMins)).TextSize(12).TextColor(theme.Muted), el.Text(fmt.Sprintf("已用 %d%% · 剩余 %d%%", percent, 100-percent)).Medium().TextSize(18), progress.Render(cx), el.Text(reset).TextSize(11).TextColor(theme.Muted)))
			}
			card.Child(windows)
			if limit.QuotaRemaining != nil {
				card.Child(el.Text(fmt.Sprintf("剩余额度：%d", *limit.QuotaRemaining)).TextSize(12).TextColor(theme.Muted))
			}
			root.Child(card)
		}
		scope := string(usage.DailyUsageScope)
		if scope == "" {
			scope = string(usage.Scope)
		}
		note := id + " · " + scope + " · " + usage.Source
		if usage.Warning != "" {
			note += "\n" + usage.Warning
		}
		root.Child(el.Text(note).TextSize(11).TextColor(theme.Muted))
	}
	errors := make([]string, 0, len(a.library.UsageErrors))
	for id := range a.library.UsageErrors {
		errors = append(errors, id)
	}
	sort.Strings(errors)
	for _, id := range errors {
		root.Child(el.Text(id + "：" + a.library.UsageErrors[id]).TextSize(12).TextColor(theme.Muted))
	}
	if len(a.library.Usage) == 0 && !a.library.UsageLoading {
		root.Child(el.Text("暂无用量数据，请配置并登录支持用量查询的 Agent。").TextSize(13).TextColor(theme.Muted))
	}
	recent := el.Div().P(20).Rounded(12).Bg(theme.Surface).Gap(12).Child(el.Text("每日用量").TextSize(18).Medium())
	for offset := 0; offset < 14; offset++ {
		day := time.Now().AddDate(0, 0, -offset).Format("2006-01-02")
		recent.Child(el.Div().Row().Child(el.Text(day).Grow().TextSize(12).TextColor(theme.Muted), el.Text(formatNumber(daily[day])).Mono().TextSize(12)))
	}
	root.Child(recent)
	return root
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
func usageHeatmap(daily map[string]int64, now time.Time) el.Element {
	end := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	start := end.AddDate(0, 0, -52*7-int(end.Weekday()))
	var visible []int64
	for day := start; !day.After(end); day = day.AddDate(0, 0, 1) {
		if value := daily[day.Format("2006-01-02")]; value > 0 {
			visible = append(visible, value)
		}
	}
	sort.Slice(visible, func(i, j int) bool { return visible[i] < visible[j] })
	grid := el.Div().Row().Gap(4).ScrollX()
	for week := 0; week < 53; week++ {
		column := el.Div().Gap(4)
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
			column.Child(el.Div().Size(el.Dp(12)).Rounded(3).Bg(fg).Role("gridcell").Name(date.Format("2006-01-02") + " · " + formatNumber(tokens) + " Token"))
		}
		grid.Child(column)
	}
	return el.Div().P(20).Rounded(12).Bg(theme.Surface).Gap(16).Child(el.Text("全局用量热力图").TextSize(18).Medium(), el.Text("过去 53 周的每日 Token 活动").TextSize(12).TextColor(theme.Muted), grid)
}
