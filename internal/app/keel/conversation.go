package keeldesktop

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"strings"
	"time"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/markdown"
	"github.com/dyike/keel/ui/theme"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
	"github.com/yuin/goldmark/ast"
	"golang.org/x/image/math/fixed"
)

type conversationState struct {
	open        map[string]bool
	overridden  map[string]bool
	views       map[string]*conversationDocView
	copyButtons map[string]*kit.CopyButtonView
	messages    map[string]*kit.MessageView
}

// Pair tool returns before grouping adjacent process records. A call ID is
// local to its step; parallel calls must never steal each other's results.
func buildConversationEntries(raw []*entry) []*entry {
	items := make([]*entry, 0, len(raw))
	calls := map[string]*entry{}
	last := map[string]*entry{}
	for _, original := range raw {
		e := *original
		e.details = nil
		if e.kind == agentrun.KindToolResult {
			target := calls[e.step+"/"+e.stream]
			if e.stream == "" {
				target = last[e.step]
			}
			if target != nil {
				target.details = append(target.details, &e)
				target.settled = !e.streaming
				target.failed = target.failed || e.failed
				continue
			}
		}
		if e.kind == agentrun.KindToolUse {
			calls[e.step+"/"+e.stream] = &e
			last[e.step] = &e
		} else if e.kind == agentrun.KindMessage || e.user || e.question != nil || e.permission != nil {
			delete(last, e.step)
		}
		items = append(items, &e)
	}
	out := make([]*entry, 0, len(items))
	for _, e := range items {
		process := !e.user && e.kind != agentrun.KindMessage && e.kind != agentrun.KindError && e.kind != kindTransition
		if !process {
			out = append(out, e)
			continue
		}
		if len(out) > 0 {
			previous := out[len(out)-1]
			if len(previous.process) > 0 && previous.step == e.step {
				previous.process = append(previous.process, e)
				continue
			}
		}
		out = append(out, &entry{key: "process/" + e.key, step: e.step, at: e.at, process: []*entry{e}})
	}
	return out
}

func (a *app) conversationOpen(key string, active bool) bool {
	s := &a.conversation
	if s.open == nil {
		s.open = map[string]bool{}
		s.overridden = map[string]bool{}
	}
	if !s.overridden[key] {
		return active
	}
	return s.open[key]
}
func (a *app) toggleConversation(key string, active bool) {
	next := !a.conversationOpen(key, active)
	a.conversation.open[key], a.conversation.overridden[key] = next, true
	a.scroller.Invalidate(key)
}

func (a *app) processGroup(cx *el.Context, e *entry) el.Element {
	active := a.detail != nil && a.detail.Active && len(a.entries) > 0 && a.entries[len(a.entries)-1].key == e.key
	count, thoughts := 0, 0
	urgent := false
	for _, item := range e.process {
		if item.kind == agentrun.KindToolUse {
			count++
		}
		if item.kind == agentrun.KindReasoning {
			thoughts++
		}
		urgent = urgent || item.permission != nil || item.question != nil
	}
	active = active || urgent
	label := "已处理"
	if active {
		label = "处理中…"
	} else if count > 0 {
		label = fmt.Sprintf("运行了 %d 个工具", count)
	} else if thoughts > 0 {
		label = "思考过程"
	}
	open := a.conversationOpen(e.key, active)
	caret := "chevron-right"
	if open {
		caret = "chevron-down"
	}
	row := el.Div().ID(e.key).Name(label).Role("button").Focusable(true).CursorPointer().Row().Items(el.Center).Gap(6).MinH(el.Dp(32)).Pt(2).Pb(6).Px(2).OnClick(func() { a.toggleConversation(e.key, active) }).FocusStyle(neutralFocus).Child(el.Text(label).TextSize(13).TextColor(theme.Muted), lucide(cx, caret, 14, mixColor(theme.Muted, theme.Bg, .72)))
	group := el.Div().WFull().Child(row)
	if open {
		// As in the Wails client: one bordered card, rows divided by hairlines.
		body := el.Div().WFull().Bg(theme.Surface).Border(1, seamColor).Rounded(11).
			Decorate(func(gtx core.C, draw func()) {
				defer clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Max}, gtx.Dp(11)).Push(gtx.Ops).Pop()
				draw()
			})
		for i, item := range e.process {
			if i > 0 {
				body.Child(el.Div().H(el.Dp(1)).WFull().Bg(seamColor))
			}
			running := active && i == len(e.process)-1 && item.kind == agentrun.KindToolUse && !item.settled && !item.failed
			stalled := !item.settled && !running && !active
			body.Child(a.processItem(cx, item, running, stalled))
		}
		group.Child(body)
	}
	return group
}

func toolDisplay(e *entry) (title, body, icon string) {
	body = strings.TrimSpace(e.text)
	if start := strings.Index(body, " {"); start > 0 {
		var envelope map[string]any
		if json.Unmarshal([]byte(body[start+1:]), &envelope) == nil {
			for _, key := range []string{"command", "cmd", "path", "file_path", "query", "url", "prompt"} {
				if value, ok := envelope[key].(string); ok && strings.TrimSpace(value) != "" {
					body = value
					break
				}
			}
			title, _ = envelope["description"].(string)
		}
	}
	if title == "" {
		title = strings.Split(strings.TrimSpace(body), "\n")[0]
	}
	icon = "wrench"
	lower := strings.ToLower(title + " " + body)
	if e.kind == agentrun.KindFileChange {
		icon = "file-diff"
	} else if strings.Contains(lower, "search") || strings.Contains(lower, "rg ") || strings.Contains(lower, "搜索") {
		icon = "search"
	} else if strings.Contains(lower, "read") || strings.Contains(lower, "cat ") || strings.Contains(lower, "读取") {
		icon = "book-open"
	} else if strings.Contains(lower, "bash") || strings.Contains(lower, "zsh") || strings.Contains(lower, "git ") || strings.Contains(lower, "运行") {
		icon = "square-terminal"
	}
	return
}

// toolDuration is a call's time to its last result, as the Wails client shows.
func toolDuration(e *entry, running bool) string {
	if e.kind != agentrun.KindToolUse || e.at.IsZero() {
		return ""
	}
	end := time.Time{}
	if running {
		end = time.Now()
	} else if len(e.details) > 0 && e.settled {
		end = e.details[len(e.details)-1].at
	}
	if end.IsZero() || end.Before(e.at) {
		return ""
	}
	d := end.Sub(e.at)
	switch {
	case d < time.Second:
		return fmt.Sprintf("%dms", d.Milliseconds())
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Round(time.Second).Seconds()))
	}
	s := int(d.Round(time.Second).Seconds())
	return fmt.Sprintf("%dm %ds", s/60, s%60)
}

// processItem draws one row of a process card the way the Wails client does:
// thoughts as an italic preview that opens into prose; tools as a quiet row
// (icon, title, time and duration, a state mark only when not plainly done)
// that opens into labelled command and result blocks.
func (a *app) processItem(cx *el.Context, e *entry, running, stalled bool) el.Element {
	if e.permission != nil || e.question != nil {
		return a.requestMessage(cx, e)
	}
	open := a.conversationOpen(e.key, false)
	caret := "chevron-right"
	if open {
		caret = "chevron-down"
	}
	muted := theme.Muted
	toggle := func() { a.toggleConversation(e.key, false) }
	row := func(name string) *el.DivEl {
		return el.Div().ID(e.key).Role("button").Name(name).Focusable(true).CursorPointer().Row().Items(el.Center).Gap(8).MinH(el.Dp(36)).Px(12).
			OnClick(toggle).Hover(func(s *el.Style) { s.Bg(mixColor(theme.Subtle, theme.Surface, .58)) }).FocusStyle(neutralFocus)
	}
	iconBox := func(name string, size float32, c color.NRGBA) el.Element {
		return el.Div().Size(el.Dp(20)).Center().NoShrink().Child(lucide(cx, name, size, c))
	}
	if e.kind == agentrun.KindReasoning {
		preview := strings.Join(strings.Fields(e.text), " ")
		if e.streaming {
			preview = "思考中…"
		}
		summary := row("思考过程："+preview).Child(
			iconBox("brain-circuit", 15, mixColor(muted, theme.Surface, .7)),
			el.Text(preview).TextSize(13).Italic().TextColor(muted).MaxLines(1).Grow().W(el.Dp(0)),
			el.Div().Opacity(.55).Child(lucide(cx, caret, 15, muted)))
		item := el.Div().WFull().Child(summary)
		if open {
			item.Bg(mixColor(theme.Subtle, theme.Surface, .2))
			item.Child(el.Div().H(el.Dp(1)).WFull().Bg(seamColor), el.Div().Pt(2).Pr(12).Pb(12).Pl(40).Child(el.Text(e.text).TextSize(13).Italic().LineHeight(1.6).TextColor(muted)))
		}
		return item
	}
	title, body, icon := toolDisplay(e)
	heading := el.Div().Grow().W(el.Dp(0)).Row().Items(el.Center).Gap(12)
	titleColor := muted
	if open {
		titleColor = theme.Text
	}
	heading.Child(el.Text(title).TextSize(13).Medium().TextColor(titleColor).MaxLines(1).Grow().W(el.Dp(0)))
	meta := e.at.Local().Format("15:04")
	if d := toolDuration(e, running); d != "" {
		meta += " · " + d
	}
	heading.Child(el.Text(meta).TextSize(11).TextColor(mixColor(muted, theme.Surface, .82)).NoShrink())
	stateIcon, stateColor, stateName := "", muted, "完成"
	switch {
	case e.failed:
		stateIcon, stateColor, stateName = "triangle-alert", theme.Danger, "失败"
	case running:
		stateIcon, stateColor, stateName = "loader-circle", theme.Info, "执行中"
	case stalled && e.kind == agentrun.KindToolUse:
		stateIcon, stateName = "clock", "未完成"
	}
	state := el.Div().W(el.Dp(18)).H(el.Dp(20)).Center().NoShrink().Role("status").Name(stateName)
	if stateIcon != "" {
		state.Child(lucide(cx, stateIcon, 13, stateColor))
	}
	summary := row(title).Child(iconBox(icon, 16, muted), heading, state, el.Div().Opacity(.7).Child(lucide(cx, caret, 15, muted)))
	item := el.Div().WFull().Child(summary)
	switch {
	case e.failed:
		item.Bg(mixColor(theme.Danger, theme.Surface, .06))
	case running:
		item.Bg(mixColor(theme.Info, theme.Surface, .06))
	case open:
		item.Bg(mixColor(theme.Subtle, theme.Surface, .28))
	}
	if open {
		label := "命令"
		if e.kind == agentrun.KindFileChange {
			label = "路径"
		}
		section := func(name, text string) el.Element {
			return el.Div().Gap(5).Child(
				el.Text(name).TextSize(10).Weight(font.SemiBold).TextColor(muted),
				el.Div().MaxH(el.Dp(220)).ScrollY().Rounded(8).Bg(theme.Subtle).Px(10).Py(9).Child(el.Text(text).Mono().TextSize(12).LineHeight(1.55).TextColor(muted)))
		}
		content := el.Div().Gap(12).Pt(11).Pr(12).Pb(13).Pl(40).Child(section(label, body))
		for _, detail := range e.details {
			if strings.TrimSpace(detail.text) != "" {
				content.Child(section("结果", detail.text))
			}
		}
		item.Child(el.Div().H(el.Dp(1)).WFull().Bg(seamColor), content)
	}
	return item
}

func newConversationDocument(source string) *markdown.Doc {
	// Emoji are ordinary text: Markdown paints color glyphs on the text
	// baseline. Local: headings and quotes render per block, so the answer
	// keeps blank-line chunks and a token reparses only the last one.
	return markdown.New(source).Plugins(markdown.Plugin{Local: true, Blocks: map[ast.NodeKind]func(ast.Node, []byte) el.View{
		ast.KindBlockquote: conversationQuote,
		ast.KindHeading: func(node ast.Node, source []byte) el.View {
			value := string(node.Text(source))
			return el.ViewFunc(func(cx *el.Context) el.Element {
				return el.Div().Pt(6).Child(el.Text(value).TextSize(15).Bold().LineHeight(1.3))
			})
		},
	}})
}

type conversationDocView struct {
	root               *el.RootWidget
	doc                *markdown.Doc
	hug                bool
	expanded, overflow bool
	// The hugged bubble's natural width, kept until its text or scale
	// changes: shaping a long prompt on every frame cost every frame.
	hugText  string
	hugPx    int
	hugWidth int
}

func (a *app) conversationDocument(cx *el.Context, e *entry, hug bool) el.Element {
	if a.conversation.views == nil {
		a.conversation.views = map[string]*conversationDocView{}
	}
	doc := a.docs[e.key]
	if doc == nil {
		doc = newConversationDocument(e.text).TableDividers(!a.ui.Preferences.HideTableDividers)
		a.docs[e.key] = doc
	}
	if doc.Source() != e.text {
		doc.SetSource(e.text)
	}
	doc.SetStreaming(e.streaming && a.detail != nil && a.detail.Active)
	v := a.conversation.views[e.key]
	if v == nil || v.doc != doc {
		v = &conversationDocView{doc: doc, hug: hug}
		v.root = el.Embed(el.ViewFunc(func(cx *el.Context) el.Element {
			return renderConversationDocument(cx, doc)
		}))
		a.conversation.views[e.key] = v
	}
	return el.Widget(core.ViewportFunc(func(gtx core.C, visible image.Rectangle) core.D {
		gtx.Metric.PxPerSp *= chatFontSize(a.ui.Preferences.ChatFontSize) / float32(theme.BodySize)
		if hug {
			gtx.Constraints.Min.X = 0
			px := gtx.Sp(theme.BodySize)
			if v.hugText != e.text || v.hugPx != px {
				shaper := theme.Material.Shaper
				// Markdown uses theme.Face, which differs from the app's control
				// font. Measuring with the control font made short bubbles too narrow.
				shaper.LayoutString(text.Parameters{Font: font.Font{Typeface: theme.Face}, PxPerEm: fixed.I(px), MaxWidth: 1 << 20}, e.text)
				width := 0
				for glyph, ok := shaper.NextGlyph(); ok; glyph, ok = shaper.NextGlyph() {
					width = max(width, (glyph.X + glyph.Advance).Ceil())
				}
				v.hugText, v.hugPx, v.hugWidth = e.text, px, width
			}
			gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, v.hugWidth+gtx.Dp(2))
		}
		dims := v.root.LayoutViewport(gtx, visible)
		if hug && !v.expanded && v.overflow != doc.IsClamped() {
			v.overflow = doc.IsClamped()
			a.scroller.Invalidate(e.key)
			gtx.Execute(op.InvalidateCmd{})
		}
		return dims
	}))
}

// Long user messages follow the Wails preview budget; the stored source and
// copy action always retain the complete message.
func (a *app) userMessageContent(cx *el.Context, e *entry) el.Element {
	document := a.conversationDocument(cx, e, true)
	v := a.conversation.views[e.key]
	_, height := cx.ViewportSize()
	limit := min(float32(440), max(float32(1), height*.52))
	if v.expanded {
		v.doc.MaxHeight(0)
	} else {
		v.doc.MaxHeight(unit.Dp(limit))
	}
	preview := el.Div().Child(document).Decorate(func(gtx core.C, draw func()) {
		draw()
		if !v.overflow || v.expanded {
			return
		}
		bottom := gtx.Constraints.Max.Y
		top := max(0, bottom-gtx.Dp(76))
		defer clip.Rect(image.Rect(0, top, gtx.Constraints.Max.X, bottom)).Push(gtx.Ops).Pop()
		transparent := theme.Subtle
		transparent.A = 0
		paint.LinearGradientOp{Stop1: f32.Pt(0, float32(top)), Stop2: f32.Pt(0, float32(top)+float32(bottom-top)*.88), Color1: transparent, Color2: theme.Subtle}.Add(gtx.Ops)
		paint.PaintOp{}.Add(gtx.Ops)
	})
	body := el.Div().Child(preview)
	if v.overflow || v.expanded {
		label, caret := "显示更多", "chevron-down"
		if v.expanded {
			label, caret = "收起", "chevron-up"
		}
		disclosure := el.Div().ID("message/"+e.key+"/disclosure").Name(label).Role("button").Focusable(true).CursorPointer().Row().Items(el.Center).Gap(5).H(el.Dp(32)).Px(2).Rounded(6).
			FocusStyle(neutralFocus).Hover(func(s *el.Style) { s.Bg(theme.SubtleHover) }).
			OnClick(func() { v.expanded = !v.expanded; a.scroller.Invalidate(e.key) }).
			Child(el.Text(label).TextSize(13).TextColor(theme.Muted), lucide(cx, caret, 15, theme.Muted))
		body.Child(el.Div().Row().Items(el.Center).Pt(5).Child(disclosure))
	}
	return body
}

func (a *app) messageActions(cx *el.Context, e *entry, user bool) el.Element {
	id := "message/" + e.key
	if a.conversation.copyButtons == nil {
		a.conversation.copyButtons = map[string]*kit.CopyButtonView{}
	}
	copyButton := a.conversation.copyButtons[e.key]
	if copyButton == nil {
		key := e.key
		copyButton = kit.CopyButton(func() string {
			if doc := a.docs[key]; doc != nil {
				return doc.Source()
			}
			return ""
		})
		a.conversation.copyButtons[key] = copyButton
		copyButton.Content(el.ViewFunc(func(cx *el.Context) el.Element {
			icon := "copy"
			if copyButton.Copied() {
				icon = "check"
			}
			return lucide(cx, icon, 12, theme.Muted)
		}))
	}
	copied := copyButton.Copied()
	visible := cx.Hovered(id) || cx.Hovered(id+"/actions") || cx.FocusWithin(id+"/actions") || copied
	// The metadata row must not inherit a short message bubble's narrow width.
	actions := el.Div().ID(id + "/actions").Hover(func(*el.Style) {}).Absolute().Bottom(-24).W(el.Dp(132)).H(el.Dp(24)).Row().Items(el.Center).Gap(3)
	if user {
		actions.Right(0).Justify(el.End)
	} else {
		actions.Left(2)
	}
	if !visible {
		actions.Opacity(0)
	}
	label := "复制消息"
	if copied {
		label = "已复制"
	}
	control := copyButton.Render(cx)
	if box, ok := control.(*el.DivEl); ok {
		box.Name(label).Size(el.Dp(24)).MinH(el.Dp(24)).NoShrink().P(0).Center().FocusStyle(neutralFocus)
	}
	timestamp := el.Text(e.at.Local().Format("01/02 15:04")).MaxLines(1).NoShrink().TextSize(12).TextColor(theme.Muted)
	if user {
		actions.Child(timestamp, control)
	} else {
		actions.Child(timestamp.Px(3), control)
	}
	return actions
}
