package keeldesktop

import (
	"encoding/json"
	"fmt"
	"strings"

	"gioui.org/font"
	"gioui.org/text"
	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/markdown"
	"github.com/dyike/keel/ui/theme"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
	"github.com/yuin/goldmark"
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
		process := !e.user && e.kind != agentrun.KindMessage && e.kind != agentrun.KindError
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
		label = "正在处理…"
	} else if count > 0 {
		label = fmt.Sprintf("运行了 %d 个工具", count)
	} else if thoughts > 0 {
		label = "思考"
	}
	open := a.conversationOpen(e.key, active)
	caret := "chevron-right"
	if open {
		caret = "chevron-down"
	}
	row := el.Div().ID(e.key).Name(label).Role("button").Focusable(true).CursorPointer().Row().Items(el.Center).Gap(6).MinH(el.Dp(32)).Pt(2).Pb(6).Px(2).OnClick(func() { a.toggleConversation(e.key, active) }).FocusStyle(neutralFocus).Child(el.Text(label).TextSize(13).TextColor(theme.Muted), lucide(cx, caret, 14, theme.Muted))
	group := el.Div().WFull().Child(row)
	if open {
		body := el.Div().WFull().Bg(theme.Surface).Rounded(11)
		for _, item := range e.process {
			body.Child(a.processItem(cx, item, active))
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
	if strings.Contains(lower, "search") || strings.Contains(lower, "rg ") || strings.Contains(lower, "搜索") {
		icon = "search"
	} else if strings.Contains(lower, "read") || strings.Contains(lower, "cat ") || strings.Contains(lower, "读取") {
		icon = "book-open"
	} else if strings.Contains(lower, "bash") || strings.Contains(lower, "zsh") || strings.Contains(lower, "git ") || strings.Contains(lower, "运行") {
		icon = "terminal"
	}
	return
}

func (a *app) processItem(cx *el.Context, e *entry, active bool) el.Element {
	if e.permission != nil || e.question != nil {
		return a.requestMessage(cx, e)
	}
	title, body, icon := toolDisplay(e)
	if e.kind == agentrun.KindReasoning {
		title = strings.Join(strings.Fields(e.text), " ")
		icon = "brain-circuit"
	}
	open := a.conversationOpen(e.key, false)
	caret := "chevron-right"
	if open {
		caret = "chevron-down"
	}
	heading := el.Div().Grow().W(el.Dp(0)).Child(el.Text(title).TextSize(12).MaxLines(1))
	if e.kind != agentrun.KindReasoning {
		heading.Child(el.Text(e.at.Local().Format("15:04:05")).TextSize(10).TextColor(theme.Muted))
	}
	row := el.Div().ID(e.key).Role("button").Name(title).Focusable(true).CursorPointer().Row().Items(el.Center).Gap(8).Px(12).Py(10).OnClick(func() { a.toggleConversation(e.key, false) }).Hover(func(s *el.Style) { s.Bg(theme.Subtle) }).FocusStyle(neutralFocus).Child(lucide(cx, icon, 16, theme.Muted), heading)
	if e.kind != agentrun.KindReasoning {
		state, fg := "check", theme.Success
		if e.failed {
			state, fg = "triangle-alert", theme.DangerText
		} else if !e.settled && e.kind == agentrun.KindToolUse {
			state, fg = "clock", theme.Muted
			if active {
				state = "loader-circle"
			}
		}
		row.Child(lucide(cx, state, 13, fg))
	}
	row.Child(lucide(cx, caret, 13, theme.Muted))
	result := el.Div().WFull().Child(row)
	if open {
		content := el.Div().Px(14).Pb(14).Gap(8)
		if e.kind == agentrun.KindReasoning {
			content.Child(a.conversationDocument(cx, e, false))
		} else {
			content.Child(el.Text("命令").TextSize(10).TextColor(theme.Muted), el.Text(body).Mono().TextSize(12).LineHeight(1.55))
			for _, detail := range e.details {
				content.Child(el.Text("结果").TextSize(10).TextColor(theme.Muted), el.Text(detail.text).Mono().TextSize(12).LineHeight(1.55))
			}
		}
		result.Child(content)
	}
	return result
}

func newConversationDocument(source string) *markdown.Doc {
	return markdown.New(source).Plugins(markdown.Plugin{Extensions: []goldmark.Extender{conversationEmojiExtension{}}, Inlines: map[ast.NodeKind]func(ast.Node, []byte) markdown.InlineObject{conversationEmojiKind: conversationEmojiObject}, Blocks: map[ast.NodeKind]func(ast.Node, []byte) el.View{
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
	root *el.RootWidget
	doc  *markdown.Doc
	hug  bool
}

func (a *app) conversationDocument(cx *el.Context, e *entry, hug bool) el.Element {
	if a.conversation.views == nil {
		a.conversation.views = map[string]*conversationDocView{}
	}
	doc := a.docs[e.key]
	if doc == nil {
		doc = newConversationDocument(e.text)
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
			root := doc.Render(cx)
			if column, ok := root.(*el.DivEl); ok {
				column.Gap(16)
			}
			return root
		}))
		a.conversation.views[e.key] = v
	}
	return el.Widget(core.Func(func(gtx core.C) core.D {
		gtx.Metric.PxPerSp *= 14 / float32(theme.BodySize)
		if hug {
			gtx.Constraints.Min.X = 0
			shaper := theme.Material.Shaper
			// Markdown uses theme.Face, which differs from the app's control
			// font. Measuring with the control font made short bubbles too narrow.
			shaper.LayoutString(text.Parameters{Font: font.Font{Typeface: theme.Face}, PxPerEm: fixed.I(gtx.Sp(theme.BodySize)), MaxWidth: 1 << 20}, e.text)
			width := 0
			for glyph, ok := shaper.NextGlyph(); ok; glyph, ok = shaper.NextGlyph() {
				width = max(width, (glyph.X + glyph.Advance).Ceil())
			}
			gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, width+gtx.Dp(2))
		}
		return v.root.Layout(gtx)
	}))
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
	actions := el.Div().ID(id + "/actions").Hover(func(*el.Style) {}).Absolute().Bottom(-24).H(el.Dp(24)).Row().Items(el.Center).Gap(3)
	if user {
		actions.Right(4)
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
		box.Name(label).Size(el.Dp(24)).MinH(el.Dp(24)).P(0).Center().FocusStyle(neutralFocus)
	}
	actions.Child(el.Text(e.at.Local().Format("01/02 15:04")).TextSize(12).TextColor(theme.Muted).Px(3), control)
	return actions
}
