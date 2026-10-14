package keeldesktop

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/theme"
	"github.com/mattn/go-runewidth"
	uiassets "github.com/openmodu/onecatch/internal/app/keel/assets"
	domainworkspaces "github.com/openmodu/onecatch/internal/domain/workspaces"
)

type reviewLine struct {
	Kind, Text string
	Old, New   int
}
type reviewHunk struct {
	Header, Scope string
	Lines         []reviewLine
}
type reviewFile struct {
	Deferred bool

	Path, OldPath        string
	Hunks                []reviewHunk
	Additions, Deletions int
	Status               domainworkspaces.GitFile
}

var diffHunkPattern = regexp.MustCompile(`^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@`)

func diffPath(raw string) string {
	if strings.HasPrefix(raw, "\"") {
		if unquoted, err := strconv.Unquote(raw); err == nil {
			raw = unquoted
		}
	}
	if raw == "/dev/null" {
		return ""
	}
	return strings.TrimPrefix(strings.TrimPrefix(raw, "a/"), "b/")
}

// Git's quoted paths are C strings. Resolve names from +++/--- rather than
// splitting on spaces, which would truncate paths containing whitespace.
func parseGitReview(diff, scope string) []reviewFile {
	var files []reviewFile
	var file *reviewFile
	var hunk *reviewHunk
	oldLine, newLine := 0, 0
	for _, raw := range strings.Split(diff, "\n") {
		if strings.HasPrefix(raw, "diff --git ") {
			header := strings.TrimPrefix(raw, "diff --git ")
			before, after := "", ""
			if strings.HasPrefix(header, "\"") {
				escaped := false
				end := -1
				for i := 1; i < len(header); i++ {
					if escaped {
						escaped = false
						continue
					}
					if header[i] == '\\' {
						escaped = true
						continue
					}
					if header[i] == '"' {
						end = i
						break
					}
				}
				if end >= 0 {
					before = header[:end+1]
					after = strings.TrimSpace(header[end+1:])
				}
			} else if i := strings.LastIndex(header, " b/"); i >= 0 {
				before, after = header[:i], header[i+1:]
			}
			files = append(files, reviewFile{Path: diffPath(after), OldPath: diffPath(before)})
			file = &files[len(files)-1]
			hunk = nil
			continue
		}
		if file == nil {
			continue
		}
		if hunk == nil && strings.HasPrefix(raw, "+++ ") {
			if p := diffPath(raw[4:]); p != "" {
				file.Path = p
			}
			continue
		}
		if hunk == nil && strings.HasPrefix(raw, "--- ") {
			if p := diffPath(raw[4:]); p != "" {
				file.OldPath = p
			}
			continue
		}
		if match := diffHunkPattern.FindStringSubmatch(raw); match != nil {
			oldLine, _ = strconv.Atoi(match[1])
			newLine, _ = strconv.Atoi(match[2])
			file.Hunks = append(file.Hunks, reviewHunk{Header: raw, Scope: scope})
			hunk = &file.Hunks[len(file.Hunks)-1]
			continue
		}
		if hunk == nil || raw == "" {
			continue
		}
		line := reviewLine{Text: raw[1:]}
		switch raw[0] {
		case '+':
			line.Kind = "add"
			line.New = newLine
			newLine++
			file.Additions++
		case '-':
			line.Kind = "delete"
			line.Old = oldLine
			oldLine++
			file.Deletions++
		case ' ':
			line.Kind = "context"
			line.Old = oldLine
			oldLine++
			line.New = newLine
			newLine++
		case '\\':
			line.Kind = "meta"
			line.Text = raw
		default:
			continue
		}
		hunk.Lines = append(hunk.Lines, line)
	}
	return files
}
func buildGitReview(snapshot domainworkspaces.GitSnapshot, staged, working string, untracked map[string]string) []reviewFile {
	files := make([]reviewFile, 0, len(snapshot.Files))
	byPath := map[string]int{}
	for _, item := range append(parseGitReview(staged, "staged"), parseGitReview(working, "worktree")...) {
		if i, ok := byPath[item.Path]; ok {
			files[i].Hunks = append(files[i].Hunks, item.Hunks...)
			files[i].Additions += item.Additions
			files[i].Deletions += item.Deletions
		} else {
			byPath[item.Path] = len(files)
			files = append(files, item)
		}
	}
	for _, status := range snapshot.Files {
		i, ok := byPath[status.Path]
		if !ok {
			i = len(files)
			byPath[status.Path] = i
			files = append(files, reviewFile{Path: status.Path, OldPath: status.Path})
			if source, exists := untracked[status.Path]; exists {
				lines := strings.Split(source, "\n")
				if lines[len(lines)-1] == "" {
					lines = lines[:len(lines)-1]
				}
				hunk := reviewHunk{Header: fmt.Sprintf("@@ -0,0 +1,%d @@", len(lines)), Scope: "untracked"}
				for n, line := range lines {
					hunk.Lines = append(hunk.Lines, reviewLine{Kind: "add", Text: line, New: n + 1})
				}
				files[i].Additions = len(lines)
				files[i].Hunks = []reviewHunk{hunk}
			}
		}
		files[i].Status = status
	}
	return files
}
func (a *app) reviewState(id string) *reviewWorkspace {
	if a.workbench.Reviews == nil {
		a.workbench.Reviews = map[string]*reviewWorkspace{}
	}
	state := a.workbench.Reviews[id]
	if state == nil {
		state = &reviewWorkspace{Limit: 500}
		a.workbench.Reviews[id] = state
	}
	return state
}

const reviewBatchSize = 20

func reviewBatch(files []domainworkspaces.GitFile, selected string) []domainworkspaces.GitFile {
	start := 0
	for i, file := range files {
		if file.Path == selected {
			start = i / reviewBatchSize * reviewBatchSize
			break
		}
	}
	return files[start:min(len(files), start+reviewBatchSize)]
}

func (a *app) loadReview(id string) { a.loadReviewBatch(id, true) }

func (a *app) loadReviewBatch(id string, refresh bool) {
	state := a.reviewState(id)
	if state.Cancel != nil {
		state.Cancel()
	}
	ctx, cancel := context.WithTimeout(a.ctx, 15*time.Second)
	state.Cancel = cancel
	selected := state.Selected
	cached := state.Snapshot
	state.Version++
	version := state.Version
	contextLines := max(3, state.ContextLines)
	state.Error = ""
	if id == "" {
		state.Loading = false
		state.Files = nil
		cancel()
		return
	}
	state.Loading = true
	go func() {
		defer cancel()
		var snapshot domainworkspaces.GitSnapshot
		var err error
		if refresh || cached == nil {
			snapshot, err = a.backend.service.GitReviewStatus(ctx, id)
		} else {
			snapshot = *cached
		}
		batch := reviewBatch(snapshot.Files, selected)
		paths := make([]string, 0, len(batch))
		for _, file := range batch {
			paths = append(paths, file.Path)
		}
		var staged, working string
		if err == nil && snapshot.IsRepo {
			staged, err = a.backend.service.GitDiffPathsContext(ctx, id, true, contextLines, paths)
		}
		if err == nil && snapshot.IsRepo {
			working, err = a.backend.service.GitDiffPathsContext(ctx, id, false, contextLines, paths)
		}
		untracked := map[string]string{}
		remaining := 4 << 20
		if err == nil {
			for _, file := range batch {
				if strings.Contains(file.Index+file.Worktree, "?") && remaining > 0 {
					doc, readErr := a.backend.service.ReadWorkspaceFile(ctx, id, file.Path)
					if readErr == nil && len(doc.Content) <= remaining {
						untracked[file.Path] = doc.Content
						remaining -= len(doc.Content)
					}
				}
			}
		}
		page := snapshot
		page.Files = batch
		loaded := buildGitReview(page, staged, working, untracked)
		byPath := make(map[string]reviewFile, len(loaded))
		for _, file := range loaded {
			byPath[file.Path] = file
		}
		files := make([]reviewFile, 0, len(snapshot.Files))
		for _, status := range snapshot.Files {
			if file, ok := byPath[status.Path]; ok {
				files = append(files, file)
			} else {
				files = append(files, reviewFile{Path: status.Path, Status: status, Deferred: true})
			}
		}
		core.Update(func() {
			if a.closed || state.Version != version {
				return
			}
			state.Loading = false
			if err != nil {
				state.Error = err.Error()
				return
			}
			state.Files = files
			state.Snapshot = &snapshot
			selectedExists := false
			for _, f := range files {
				if f.Path == state.Selected {
					selectedExists = true
					break
				}
			}
			if !selectedExists {
				state.StartPath = ""
				state.Jump = ""
				state.Selected = ""
				if len(files) > 0 {
					state.Selected = files[0].Path
				}
			}

		})
	}()
}
func (a *app) reviewInspector(cx *el.Context) el.Element {
	id := a.workspaceContext()
	state := a.reviewState(id)
	if id == "" {
		return inspectorNotice("选择项目后查看改动。", false)
	}
	if state.Collapsed == nil {
		state.Collapsed = map[string]bool{}
	}
	if state.ExpandedContext == nil {
		state.ExpandedContext = map[string]bool{}
	}
	root := el.Div().Grow().H(el.Dp(0)).Bg(theme.Surface)
	if state.Error != "" {
		root.Child(inspectorNotice(state.Error, true))
	}
	if state.Loading {
		root.Child(inspectorNotice("正在读取改动…", false))
	}
	if len(state.Files) == 0 && !state.Loading {
		return root.Child(inspectorNotice("工作目录没有未提交的改动。", false))
	}
	var blocks []el.Element
	headers := map[int]string{}
	canvasWidth := a.inspectorWidth(cx) - 18
	if state.ShowFiles {
		canvasWidth -= 220
	}
	if a.workbench.DetachedWindow != nil {
		canvasWidth, _ = cx.ViewportSize()
		canvasWidth -= 10
	}
	canvas := el.Div().W(el.Dp(max(100, canvasWidth))).Decorate(func(gtx core.C, draw func()) {
		// Layout includes clipped blocks; paint callbacks on individual headers do not.
		// Measure the current layout so wrapping and collapsed sections cannot stale the target.
		if state.Jump != "" {
			y := float32(0)
			for i, block := range blocks {
				if headers[i] == state.Jump {
					cx.ScrollTo("review-diff/"+id, y)
					state.Jump = ""
					break
				}
				_, height := cx.LayoutSize(block)
				y += height
			}
		}
		draw()
	})
	appendBlock := func(block el.Element) {
		blocks = append(blocks, block)
		canvas.Child(block)
	}
	if state.NoWrap {
		width := a.inspectorWidth(cx) - 8
		for _, f := range state.Files {
			for _, h := range f.Hunks {
				for _, l := range h.Lines {
					width = max(width, float32(runewidth.StringWidth(strings.ReplaceAll(l.Text, "\t", "    ")))*7.3+80)
				}
			}
		}
		if state.SideBySide {
			width *= 2
		}
		canvas.W(el.Dp(width))
	}
	lastGroup := ""
	remaining := state.Limit
	beforeTarget := state.StartPath != ""
	for _, file := range orderedReviewFiles(state) {
		file := file
		if file.Deferred {
			continue
		}
		if file.Path == state.StartPath {
			beforeTarget = false
		}
		group := ""
		if state.SeparateGenerated {
			group = reviewFileGroup(file.Path)
		}
		if state.GroupFolders {
			group += " / " + filepath.Dir(file.Path)
		}
		if group != "" && group != lastGroup {
			appendBlock(el.Div().WFull().Px(12).Py(8).Bg(theme.Subtle).Child(el.Text(strings.TrimPrefix(group, " / ")).TextSize(11).TextColor(theme.Muted)))
			lastGroup = group
		}

		arrow := "chevron-down"
		if state.Collapsed[file.Path] {
			arrow = "chevron-right"
		}
		dir := filepath.Dir(file.Path)
		if dir == "." {
			dir = ""
		}
		row := el.Div().Row().Items(el.Center).WFull().MinH(el.Dp(36)).Px(10).Py(6).Gap(6).Bg(theme.Subtle).
			ID("review-file/"+file.Path).Role("button").Name(file.Path).Focusable(true).CursorPointer().FocusStyle(neutralFocus).
			OnClick(func() { state.Collapsed[file.Path] = !state.Collapsed[file.Path] }).
			Child(uiassets.Lucide(cx, arrow, 14, theme.Muted), uiassets.Lucide(cx, "file-diff", 13, theme.Success),
				el.Text(filepath.Base(file.Path)).TextSize(12).MaxLines(1),
				el.Text(dir).TextSize(11).TextColor(theme.Muted).MaxLines(1).MaxW(el.Frac(.3)),
				el.Text(fmt.Sprintf("+%d", file.Additions)).TextSize(11).TextColor(theme.Success),
				el.Text(fmt.Sprintf("−%d", file.Deletions)).TextSize(11).TextColor(theme.DangerText))
		headers[len(blocks)] = file.Path
		appendBlock(row)
		if state.Collapsed[file.Path] || beforeTarget {
			continue
		}
		if len(file.Hunks) == 0 {
			appendBlock(inspectorNotice("此文件没有可预览的文本改动。", false))
		}
		for hi, hunk := range file.Hunks {
			if state.HideWhitespace {
				hunk.Lines = withoutWhitespaceChanges(hunk.Lines)
			}
			if state.SideBySide {
				for pi, pair := range pairedReviewLines(hunk.Lines) {
					if remaining <= 0 {
						appendBlock(button("显示更多行", func() { state.Limit += 500 }, kit.ButtonGhost).Compact(true).Render(cx))
						return root.Child(a.reviewScroll(cx, id, state, canvas))
					}
					remaining--
					pairRow := el.Div().Row().WFull().Items(el.Stretch)
					for side, line := range pair {
						cell := el.Div().Grow().W(el.Dp(0))
						if line != nil {
							cell.Child(a.reviewLineView(cx, state, file.Path, hunk.Scope, fmt.Sprintf("%s/%d/p%d/%d", file.Path, hi, pi, side), *line))
						}
						pairRow.Child(cell)
					}
					appendBlock(pairRow)
				}
				continue
			}
			scope := map[string]string{"staged": "已暂存", "worktree": "工作区", "untracked": "新增文件"}[hunk.Scope]
			if len(file.Hunks) > 1 {
				appendBlock(el.Div().Px(12).Py(4).Child(el.Text(scope).TextSize(10).TextColor(theme.Muted)))
			}
			if len(hunk.Lines) > 0 && hunk.Lines[0].New > 1 && hunk.Scope != "untracked" {
				appendBlock(button(fmt.Sprintf("⌃  %d 行未修改内容", hunk.Lines[0].New-1), func() { a.expandReviewContext(id) }, kit.ButtonGhost).Compact(true).Size(30).Render(cx))
			}
			for li := 0; li < len(hunk.Lines); li++ {
				line := hunk.Lines[li]
				key := fmt.Sprintf("%s/%d/%d", file.Path, hi, li)
				if line.Kind == "context" {
					end := li + 1
					for end < len(hunk.Lines) && hunk.Lines[end].Kind == "context" {
						end++
					}
					if end-li > 4 && state.ContextLines <= 3 && !state.ExpandedContext[key] {
						appendBlock(button(fmt.Sprintf("展开 %d 行未修改内容", end-li), func() { state.ExpandedContext[key] = true }, kit.ButtonGhost).Compact(true).Render(cx))
						li = end - 1
						continue
					}
				}
				if remaining <= 0 {
					appendBlock(button("显示更多行", func() { state.Limit += 500 }, kit.ButtonGhost).Compact(true).Render(cx))
					return root.Child(a.reviewScroll(cx, id, state, canvas))
				}
				remaining--
				appendBlock(a.reviewLineView(cx, state, file.Path, hunk.Scope, key, line))
			}
			if hunk.Scope != "untracked" && state.ContextLines < 2000 {
				appendBlock(button("显示更多上下文", func() { a.expandReviewContext(id) }, kit.ButtonGhost).Compact(true).Size(32).Render(cx))
			}
		}
	}
	return root.Child(a.reviewScroll(cx, id, state, canvas))
}

func (a *app) reviewScroll(cx *el.Context, id string, state *reviewWorkspace, canvas el.Element) el.Element {
	diff := el.Div().ID("review-diff/" + id).Name("代码差异").Role("region").Grow().WFull().H(el.Dp(0)).ScrollY().ScrollX().Child(canvas)
	container := el.Div().Grow().W(el.Dp(0)).HFull()
	if state.Snapshot != nil && len(state.Snapshot.Files) > reviewBatchSize {
		batch := reviewBatch(state.Snapshot.Files, state.Selected)
		start := 0
		for i, f := range state.Snapshot.Files {
			if len(batch) > 0 && f.Path == batch[0].Path {
				start = i
				break
			}
		}
		nav := el.Div().Row().Items(el.Center).Gap(8).Px(8).Py(4)
		if start > 0 {
			path := state.Snapshot.Files[start-reviewBatchSize].Path
			nav.Child(button("上一批", func() { a.selectReviewFile(state, path) }, kit.ButtonGhost).Compact(true).Render(cx))
		}
		nav.Child(el.Text(fmt.Sprintf("%d–%d / %d 个文件", start+1, start+len(batch), len(state.Snapshot.Files))).TextSize(11).TextColor(theme.Muted))
		if start+len(batch) < len(state.Snapshot.Files) {
			path := state.Snapshot.Files[start+len(batch)].Path
			nav.Child(button("下一批", func() { a.selectReviewFile(state, path) }, kit.ButtonGhost).Compact(true).Render(cx))
		}
		container.Child(nav)
	}
	container.Child(diff)
	row := el.Div().Row().Grow().H(el.Dp(0)).WFull()
	if state.ShowFiles {
		row.Child(el.Div().W(el.Dp(220)).NoShrink().HFull().P(6).Bg(theme.Subtle).Child(a.reviewFileList(cx, state, false)))
	}
	return row.Child(container)
}

func reviewMatches(file reviewFile, query string) bool {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" || strings.Contains(strings.ToLower(file.Path), query) {
		return true
	}
	for _, h := range file.Hunks {
		for _, l := range h.Lines {
			if strings.Contains(strings.ToLower(l.Text), query) {
				return true
			}
		}
	}
	return false
}
func (a *app) addReviewComment(state *reviewWorkspace) {
	if strings.TrimSpace(state.Comment) == "" {
		return
	}
	if a.prompt != "" {
		a.prompt += "\n\n"
	}
	a.prompt += fmt.Sprintf("请修改 %s 第 %d 行（%s）：\n%s", state.CommentPath, state.CommentLine, state.CommentScope, strings.TrimSpace(state.Comment))
	state.Comment, state.CommentKey = "", ""
}

func (a *app) reviewLineView(cx *el.Context, state *reviewWorkspace, path, scope, key string, line reviewLine) el.Element {
	result := el.Div().WFull()
	text := el.Text(strings.ReplaceAll(line.Text, "\t", "    ")).Mono().TextSize(12).LineHeight(1.5).Grow().W(el.Dp(0)).Py(3).Pr(12)
	if state.NoWrap {
		text.MaxLines(1)
	}

	number := line.New
	if number == 0 {
		number = line.Old
	}
	lineNumber := ""
	if number > 0 {
		lineNumber = strconv.Itoa(number)
	}
	mark, fg, bg := " ", theme.Muted, theme.Surface
	switch line.Kind {
	case "add":
		mark, fg, bg = "+", theme.Success, mixColor(theme.Success, theme.Surface, .10)
	case "delete":
		mark, fg, bg = "−", theme.DangerText, mixColor(theme.DangerText, theme.Surface, .09)
	}
	if state.NoHighlight {
		bg = theme.Surface
	}
	stripe := el.Div().Absolute().Left(0).Top(0).Bottom(0).W(el.Dp(3))
	if line.Kind == "add" || line.Kind == "delete" {
		stripe.Bg(fg)
	}
	n := number
	result.Child(el.Div().Row().WFull().Bg(bg).ID("review-line/"+key).Role("button").Name(fmt.Sprintf("评论 %s 第 %d 行", path, n)).CursorPointer().
		OnClick(func() {
			state.CommentKey = key
			state.CommentPath = path
			state.CommentLine = n
			state.CommentScope = scope
		}).
		Child(stripe, el.Div().W(el.Dp(40)).NoShrink().Items(el.End).Pt(3).Pr(6).Child(el.Text(lineNumber).Mono().TextSize(11).TextColor(fg)),
			el.Text(mark).Mono().TextSize(11).TextColor(fg).W(el.Dp(16)).NoShrink().Pt(3),
			text))
	if state.CommentKey == key {
		result.Child(el.Div().WFull().Px(14).Py(8).Gap(6).Bg(theme.Subtle).Child(
			el.Text(fmt.Sprintf("%s · 第 %d 行", path, n)).TextSize(11).TextColor(theme.Muted),
			el.TextArea().ID("review-comment").Name("修改意见").Placeholder("描述希望如何修改这一行…").Bind(&state.Comment).AutoGrow(2, 5).WFull().TextSize(12),
			el.Div().Row().Gap(6).Child(button("加入对话", func() { a.addReviewComment(state) }, kit.ButtonGhost).Compact(true).Render(cx), button("取消", func() { state.CommentKey = "" }, kit.ButtonGhost).Compact(true).Render(cx))))
	}
	return result
}

func (a *app) expandReviewContext(id string) {
	s := a.reviewState(id)
	if s.Loading {
		return
	}
	s.ContextLines = min(2000, max(3, s.ContextLines)+20)
	s.Limit += 500
	a.loadReview(id)
}
