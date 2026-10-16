package keeldesktop

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/theme"
	uiassets "github.com/openmodu/onecatch/internal/app/keel/assets"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

const (
	fileChangePreviewLines = 8
	fileChangeMaxHeight    = 300
	fileChangeLineHeight   = 26
)

type fileDiffLine struct {
	old, new int
	mark     byte
	text     string
}

type fileChangeView struct {
	change         agentrun.FileChange
	added, removed int
	lines          []fileDiffLine
	copy           *kit.CopyButtonView
}

// Parse once per change, not on every frame. Metadata before the first hunk
// is omitted from the compact view; the file identity already lives in the
// card header. Hunk coordinates drive the two source-line columns.
func newFileChangeView(change agentrun.FileChange) *fileChangeView {
	v := &fileChangeView{change: change}
	oldLine, newLine, inHunk := 0, 0, false
	for raw := range strings.SplitSeq(change.Diff, "\n") {
		if strings.HasPrefix(raw, "@@") {
			oldLine, newLine, _ = fileDiffHunkStart(raw)
			inHunk = true
			v.lines = append(v.lines, fileDiffLine{mark: '@', text: raw})
			continue
		}
		if !inHunk || raw == "" {
			continue
		}
		line := fileDiffLine{text: raw}
		switch raw[0] {
		case '+':
			line.mark, line.new, line.text = '+', newLine, raw[1:]
			newLine++
			v.added++
		case '-':
			line.mark, line.old, line.text = '-', oldLine, raw[1:]
			oldLine++
			v.removed++
		case ' ':
			line.old, line.new, line.text = oldLine, newLine, raw[1:]
			oldLine++
			newLine++
		case '\\':
			line.mark = '\\'
		default:
			line.old, line.new = oldLine, newLine
			oldLine++
			newLine++
		}
		v.lines = append(v.lines, line)
	}
	v.copy = kit.CopyButton(func() string { return v.change.Diff }).Content(el.ViewFunc(func(cx *el.Context) el.Element {
		icon := "copy"
		if v.copy.Copied() {
			icon = "check"
		}
		return uiassets.Lucide(cx, icon, 14, theme.Muted)
	}))
	return v
}

func fileDiffHunkStart(header string) (oldLine, newLine int, ok bool) {
	fields := strings.Fields(header)
	if len(fields) < 3 || !strings.HasPrefix(fields[1], "-") || !strings.HasPrefix(fields[2], "+") {
		return 0, 0, false
	}
	parse := func(s string) (int, bool) {
		s = strings.TrimLeft(s, "+-")
		if comma := strings.IndexByte(s, ','); comma >= 0 {
			s = s[:comma]
		}
		n, err := strconv.Atoi(s)
		return n, err == nil
	}
	oldLine, oldOK := parse(fields[1])
	newLine, newOK := parse(fields[2])
	return oldLine, newLine, oldOK && newOK
}

func fileChangePathParts(raw string) (name, dir string) {
	clean := strings.TrimRight(raw, "/\\")
	if clean == "" {
		return raw, ""
	}
	if slash := strings.LastIndexAny(clean, "/\\"); slash >= 0 {
		name, dir = clean[slash+1:], clean[:slash]
		if dir == "" && strings.HasPrefix(clean, "/") {
			dir = "/"
		}
		return name, dir
	}
	return clean, ""
}

func (a *app) fileChangeCards(cx *el.Context, e *entry) el.Element {
	changes := e.changes
	if len(changes) == 0 {
		// Old transcripts only saved paths. Never infer a past edit from the
		// current working tree, which may already contain subsequent edits.
		for path := range strings.SplitSeq(e.text, "\n") {
			if path = strings.TrimSpace(path); path != "" {
				changes = append(changes, agentrun.FileChange{Path: path})
			}
		}
	}
	if a.conversation.fileChanges == nil {
		a.conversation.fileChanges = map[string]*fileChangeView{}
	}
	box := el.Div().WFull().Gap(8)
	for i, change := range changes {
		id := fmt.Sprintf("%s/file/%d", e.key, i)
		v := a.conversation.fileChanges[id]
		if v == nil || v.change != change {
			v = newFileChangeView(change)
			a.conversation.fileChanges[id] = v
		}
		box.Child(a.fileChangeCard(cx, e, id, v))
	}
	return box
}

func (a *app) fileChangeCard(cx *el.Context, e *entry, id string, v *fileChangeView) el.Element {
	change := v.change
	expanded := a.conversationOpen(id, false)
	toggle := func() {
		a.toggleConversation(id, false)
		a.invalidateConversationEntry(e.key)
	}
	action := "编辑文件"
	switch change.Kind {
	case "add":
		action = "新建文件"
	case "delete":
		action = "删除文件"
	case "write":
		action = "写入文件"
	}
	if change.MovePath != "" {
		action = "移动文件"
	}
	if e.failed {
		action += " · 失败"
	}
	caret := "chevron-right"
	if expanded {
		caret = "chevron-down"
	}
	toggleLabel := "查看文件改动：" + change.Path
	if expanded {
		toggleLabel = "收起文件改动：" + change.Path
	}
	name, dir := fileChangePathParts(change.Path)
	title := el.Div().ID(id + "/toggle").Role("button").Name(toggleLabel).Focusable(change.Diff != "").
		Row().Items(el.Center).Gap(8).Grow().W(el.Dp(0)).MinH(el.Dp(38)).FocusStyle(neutralFocus)
	if change.Diff != "" {
		title.CursorPointer().OnClick(toggle)
	}
	title.Child(
		uiassets.Lucide(cx, "file-diff", 16, theme.Muted),
		el.Text(name).TextSize(13).Medium().TextColor(theme.Text).MaxLines(1).NoShrink(),
		el.Text(action).TextSize(12).TextColor(theme.Muted).MaxLines(1).NoShrink(),
	)
	if dir != "" {
		title.Child(el.Text(dir).TextSize(12).TextColor(theme.Muted).MaxLines(1).Grow().W(el.Dp(0)))
	} else {
		title.Child(el.Div().Grow().W(el.Dp(0)))
	}
	header := el.Div().Row().Items(el.Center).Gap(9).Px(12).Child(title)
	if change.Diff != "" {
		if v.added+v.removed > 0 {
			suffix := ""
			if change.Truncated {
				suffix = "…"
			}
			header.Child(
				el.Text(fmt.Sprintf("+%d%s", v.added, suffix)).Mono().TextSize(11).TextColor(theme.Success).NoShrink(),
				el.Text(fmt.Sprintf("−%d%s", v.removed, suffix)).Mono().TextSize(11).TextColor(theme.Danger).NoShrink(),
			)
		}
		header.Child(v.copy.Render(cx))
		header.Child(el.Div().ID(id + "/caret").Role("button").Name(toggleLabel).Focusable(true).CursorPointer().OnClick(toggle).FocusStyle(neutralFocus).Size(el.Dp(20)).Center().Child(uiassets.Lucide(cx, caret, 13, theme.Muted)))
	}
	card := el.Div().ID(id).WFull().Bg(theme.Surface).Border(1, seamColor).Rounded(11).Child(header)
	if change.Diff == "" {
		return card.Child(el.Div().H(el.Dp(1)).Bg(seamColor), el.Text("未保存代码差异").TextSize(11).TextColor(theme.Muted).Px(16).Py(10))
	}
	card.Child(el.Div().H(el.Dp(1)).Bg(seamColor), v.diffBlock(id, expanded))
	footer := el.Div().ID(id + "/expand").Role("button").Name(map[bool]string{false: "展开完整差异", true: "收起差异"}[expanded]).
		Focusable(true).CursorPointer().OnClick(toggle).FocusStyle(neutralFocus).Row().Items(el.Center).MinH(el.Dp(34)).Px(12).
		Child(el.Text(map[bool]string{false: "展开完整差异", true: "收起差异"}[expanded]).TextSize(11).TextColor(theme.Muted))
	card.Child(el.Div().H(el.Dp(1)).Bg(seamColor), footer)
	return card
}

func (v *fileChangeView) diffBlock(id string, expanded bool) el.Element {
	lines := v.lines
	if !expanded && len(lines) > fileChangePreviewLines {
		lines = lines[:fileChangePreviewLines]
	}
	body := el.Div().WFull().Bg(theme.CodeBg)
	if change := v.change; change.MovePath != "" {
		body.Child(el.Text("→ " + change.MovePath).Mono().TextSize(11).TextColor(theme.Muted).Px(12).Py(6))
	}
	for i, line := range lines {
		body.Child(fileDiffRow(id, i, line))
	}
	if v.change.Truncated {
		body.Child(el.Text("差异过大，仅保存部分内容").TextSize(11).TextColor(theme.Muted).Px(12).Py(6))
	}
	if v.change.Kind == "write" {
		body.Child(el.Text("写入内容；原内容未记录").TextSize(11).TextColor(theme.Muted).Px(12).Py(6))
	}
	region := el.Div().ID(id + "/diff").Name("文件差异：" + v.change.Path).Role("region").WFull().Child(body)
	if expanded {
		region.MaxH(el.Dp(fileChangeMaxHeight)).ScrollY().Scrollbars(el.ScrollbarAlways)
	}
	return region
}

func fileDiffRow(id string, index int, line fileDiffLine) el.Element {
	background, foreground := theme.CodeBg, theme.Text
	switch line.mark {
	case '+':
		background, foreground = mixColor(theme.Success, theme.CodeBg, .10), theme.Success
	case '-':
		background, foreground = mixColor(theme.Danger, theme.CodeBg, .09), theme.Danger
	case '@', '\\':
		foreground = theme.Muted
	}
	number := func(value int) el.Element {
		label := ""
		if value > 0 {
			label = strconv.Itoa(value)
		}
		return el.Div().W(el.Dp(36)).NoShrink().Items(el.End).Pr(8).Child(el.Text(label).Mono().TextSize(11).TextColor(theme.Muted).MaxLines(1))
	}
	mark := ""
	if line.mark == '+' || line.mark == '-' {
		mark = string(line.mark)
	}
	text := strings.ReplaceAll(line.text, "\t", "    ")
	return el.Div().ID(fmt.Sprintf("%s/line/%d", id, index)).Row().Items(el.Center).WFull().MinH(el.Dp(fileChangeLineHeight)).Bg(background).Child(
		number(line.old), number(line.new),
		el.Div().W(el.Dp(24)).NoShrink().Items(el.Center).Child(el.Text(mark).Mono().TextSize(12).TextColor(foreground).MaxLines(1)),
		el.Text(text).Mono().TextSize(12).TextColor(foreground).MaxLines(1).Grow().W(el.Dp(0)),
	)
}
