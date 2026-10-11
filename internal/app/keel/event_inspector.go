package keeldesktop

import (
	"encoding/json"
	"fmt"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/theme"
	desktopservice "github.com/openmodu/onecatch/internal/service/desktop"
	"time"
)

type eventNavigator struct {
	PayloadSource string
	Wheel         float32

	RunID      string
	Seq        int64
	Following  bool
	Slider     *kit.SliderView
	Payload    *kit.CodeEditorView
	PayloadKey string
}

func (s *eventNavigator) index(run string, events []desktopservice.WorkflowEventView) int {
	if s.RunID != run {
		s.RunID = run
		s.Following = true
		s.PayloadKey = ""
	}
	if len(events) == 0 {
		return -1
	}
	if s.Following {
		s.Seq = events[len(events)-1].Seq
		return len(events) - 1
	}
	for i, e := range events {
		if e.Seq == s.Seq {
			return i
		}
	}
	s.Seq = events[0].Seq
	return 0
}
func (s *eventNavigator) selectIndex(events []desktopservice.WorkflowEventView, i int) {
	if len(events) == 0 {
		return
	}
	i = max(0, min(i, len(events)-1))
	s.Seq = events[i].Seq
	s.Following = i == len(events)-1
}
func (a *app) eventInspector(cx *el.Context) el.Element {
	if a.detail == nil {
		return inspectorNotice("选择对话后查看事件。", false)
	}
	events := a.detail.Events
	s := &a.workbench.Events
	index := s.index(a.detail.Run.ID, events)
	if index < 0 {
		return inspectorNotice("暂无事件。", false)
	}
	if s.Slider == nil {
		s.Slider = kit.Slider("", 0, 1).Step(1).OnChange(func(v float64) {
			if a.detail != nil {
				s.selectIndex(a.detail.Events, int(v))
			}
		})
	}
	s.Slider.Appearance(func(style *kit.SliderAppearance) {
		style.TrackSize = 2
		style.ThumbSize = 12
		style.TrackRadius = 3
		style.ThumbRadius = 10
		style.TrackColor = theme.Subtle
		style.FillColor = theme.Muted
		style.ThumbColor = theme.Text
		style.ThumbBorderWidth = 0
	})
	s.Slider.SetRange(0, float64(max(1, len(events)-1)))
	s.Slider.SetValue(float64(index))
	s.Slider.SetDisabled(len(events) < 2)
	selected := events[index]
	column := el.Div().Grow().H(el.Dp(0)).P(12).Gap(10)
	controls := el.Div().Row().Items(el.Center).Gap(6)
	previous := button("上一条", func() { s.selectIndex(events, index-1) }, kit.ButtonGhost).Compact(true).Size(28)
	previous.SetDisabled(index == 0)
	next := button("下一条", func() { s.selectIndex(events, index+1) }, kit.ButtonGhost).Compact(true).Size(28)
	next.SetDisabled(index == len(events)-1)
	controls.Child(previous.Render(cx), el.Text(fmt.Sprintf("%d / %d", index+1, len(events))).TextSize(11).TextColor(theme.Muted), next.Render(cx), el.Div().Grow(), button("最新", func() { s.Following = true }, kit.ButtonGhost).Compact(true).Size(28).Render(cx))
	mode := "查看历史 · 新事件不会切换当前时间点"
	if s.Following {
		mode = "跟随最新事件"
	}
	column.Child(controls, el.Text(mode).TextSize(10).TextColor(theme.Muted))
	// Keep a bounded window of timestamps; only the selected payload is mounted.
	_, viewportHeight := cx.ViewportSize()
	railHeight := max(float32(160), min(float32(420), viewportHeight-250))
	s.Slider.Vertical(railHeight)
	rail := el.Div().W(el.Dp(112)).NoShrink().Gap(8).Items(el.Center)
	rail.Child(el.Text(eventTimeZone(selected.At)).TextSize(10).TextColor(theme.Muted), el.Text("现在").TextSize(10).TextColor(theme.Muted))
	ticks := el.Div().W(el.Dp(80)).NoShrink().Gap(0)
	count := min(len(events), 7)
	for tick := count - 1; tick >= 0; tick-- {
		i := 0
		if count > 1 {
			i = tick * (len(events) - 1) / (count - 1)
		}
		e := events[i]
		ticks.Child(button(eventClock(e.At), func() { s.selectIndex(events, i) }, kit.ButtonGhost).Selected(i == index).Compact(true).Size(26).Render(cx))
		if tick > 0 {
			ticks.Child(el.Div().H(el.Dp(max(0, (railHeight-26)/float32(max(1, count-1))-26))))
		}
	}
	rail.OnScroll(el.ScrollRange{}, el.ScrollRange{Min: -120, Max: 120}, func(e el.ScrollEvent) {
		s.Wheel += e.Y
		if s.Wheel >= 24 {
			s.selectIndex(events, index-1)
			s.Wheel = 0
		} else if s.Wheel <= -24 {
			s.selectIndex(events, index+1)
			s.Wheel = 0
		}
	})
	rail.Child(el.Div().Row().Items(el.Center).Gap(2).Child(ticks, el.Div().W(el.Dp(20)).NoShrink().Child(s.Slider.Render(cx))), el.Text("更早").TextSize(10).TextColor(theme.Muted))
	card := el.Div().Grow().H(el.Dp(0)).WFull().Rounded(12).Bg(theme.Surface).P(12).Gap(12)
	heading := el.Div().Bg(theme.Subtle).Rounded(8).P(12).Gap(6).Child(el.Div().Row().Gap(8).Child(el.Text(fmt.Sprintf("#%d", selected.Seq)).TextSize(11).TextColor(theme.Muted), el.Text(selected.Type).TextSize(13).Medium().Grow().W(el.Dp(0))), el.Text(eventDateTime(selected.At)).TextSize(11).TextColor(theme.Muted))
	if selected.StepID != "" {
		heading.Child(el.Text("步骤 · " + selected.StepID).TextSize(11).TextColor(theme.Muted))
	}
	card.Child(heading)
	key := fmt.Sprintf("%s/%d", s.RunID, selected.Seq)
	if s.Payload == nil || s.PayloadKey != key || s.PayloadSource != selected.Payload {
		source := selected.Payload
		var data any
		if json.Unmarshal([]byte(source), &data) == nil {
			raw, _ := json.MarshalIndent(data, "", "  ")
			source = string(raw)
		}
		if source == "" {
			source = "{}"
		}
		s.Payload = kit.CodeEditor(source).Language("json").SoftWrap(true).Name("事件数据").Fill()
		s.Payload.SetReadOnly(true)
		s.PayloadKey = key
		s.PayloadSource = selected.Payload
	}
	palette := theme.Current()
	palette.CodeBg = theme.Surface
	card.Child(cx.Themed(palette, el.ViewFunc(func(cx *el.Context) el.Element { return s.Payload.Render(cx) })).Grow().H(el.Dp(0)))
	stack := el.Div().Grow().W(el.Dp(0)).HFull().Gap(4)
	if index > 0 {
		stack.Child(el.Div().H(el.Dp(4)).Mx(22).Rounded(4).Bg(theme.Border), el.Div().H(el.Dp(5)).Mx(10).Rounded(4).Bg(theme.Border))
	}
	stack.Child(card)
	return column.Child(el.Div().Row().Grow().H(el.Dp(0)).Gap(8).Bg(theme.Subtle).Rounded(14).P(8).Child(stack, rail))
}

func eventTimeZone(value string) string {
	if at, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return "UTC" + at.Local().Format("-07:00")
	}
	return "本地时间"
}
func eventDateTime(value string) string {
	if at, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return at.Local().Format("2006-01-02 15:04:05") + " " + eventTimeZone(value)
	}
	return value
}
