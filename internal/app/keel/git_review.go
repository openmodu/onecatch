package keeldesktop

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/theme"
	"github.com/mattn/go-runewidth"
	domainworkspaces "github.com/openmodu/onecatch/internal/domain/workspaces"
	"path/filepath"
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
func (a *app) loadReview(id string) {
	state := a.reviewState(id)
	state.Version++
	version := state.Version
	state.Error = ""
	if id == "" {
		state.Loading = false
		state.Files = nil
		return
	}
	state.Loading = true
	go func() {
		ctx, cancel := context.WithTimeout(a.ctx, 15*time.Second)
		defer cancel()
		snapshot, err := a.backend.service.GitStatus(ctx, id)
		var staged, working string
		if err == nil && snapshot.IsRepo {
			staged, err = a.backend.service.GitDiff(ctx, id, true)
		}
		if err == nil && snapshot.IsRepo {
			working, err = a.backend.service.GitDiff(ctx, id, false)
		}
		untracked := map[string]string{}
		remaining := 4 << 20
		if err == nil {
			for _, file := range snapshot.Files {
				if strings.Contains(file.Index+file.Worktree, "?") && remaining > 0 {
					doc, readErr := a.backend.service.ReadWorkspaceFile(ctx, id, file.Path)
					if readErr == nil && len(doc.Content) <= remaining {
						untracked[file.Path] = doc.Content
						remaining -= len(doc.Content)
					}
				}
			}
		}
		files := buildGitReview(snapshot, staged, working, untracked)
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
			selectedExists := false
			for _, f := range files {
				if f.Path == state.Selected {
					selectedExists = true
					break
				}
			}
			if !selectedExists {
				state.Selected = ""
				if len(files) > 0 {
					state.Selected = files[0].Path
				}
			}
			if a.workspaceContext() == id {
				a.captionGit = &snapshot
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
	root := el.Div().Grow().H(el.Dp(0))
	added, deleted := 0, 0
	for _, file := range state.Files {
		added += file.Additions
		deleted += file.Deletions
	}
	root.Child(el.Div().Row().Items(el.Center).Px(14).H(el.Dp(40)).Gap(8).Child(el.Text("审阅").TextSize(12).Medium().Grow(), el.Text(fmt.Sprintf("+%d", added)).Mono().TextSize(11).TextColor(theme.Success), el.Text(fmt.Sprintf("−%d", deleted)).Mono().TextSize(11).TextColor(theme.DangerText), iconAction(cx, "review-refresh", "刷新", "refresh-cw", func() { a.loadReview(id) }, false), iconAction(cx, "review-close", "关闭", "x", func() { a.workbench.Tab = "status" }, false)))
	if state.Error != "" {
		root.Child(inspectorNotice(state.Error, true))
	}
	if state.Loading {
		root.Child(inspectorNotice("正在读取改动…", false))
	}
	if len(state.Files) == 0 && !state.Loading {
		return root.Child(inspectorNotice("工作目录没有未提交的改动。", false))
	}
	width := a.inspectorWidth(cx)
	if a.workbench.DetachedWindow != nil {
		width, _ = cx.ViewportSize()
	}
	minWidth := float32(620)
	if width <= 560 {
		minWidth = 520
	}
	for _, file := range state.Files {
		for _, hunk := range file.Hunks {
			for _, line := range hunk.Lines {
				minWidth = max(minWidth, float32(runewidth.StringWidth(strings.ReplaceAll(line.Text, "\t", "    ")))*6.95+120)
			}
		}
	}
	diffID := "review-diff/" + id
	diff := el.Div().ID(diffID).Name("代码差异").Role("region").Grow().W(el.Dp(0)).HFull().ScrollY().ScrollX()
	canvas := el.Div().W(el.Dp(minWidth))
	offsets := map[string]float32{}
	vertical := float32(0)
	remaining := state.Limit
	for _, file := range state.Files {
		offsets[file.Path] = vertical
		vertical += 36
		canvas.Child(el.Div().Row().Items(el.Center).Px(12).H(el.Dp(36)).Gap(7).Bg(mixColor(theme.Subtle, theme.Surface, .04)).Child(lucide(cx, "file-diff", 15, theme.Muted), el.Text(file.Path).Mono().TextSize(12).Medium().MaxLines(1).Grow().W(el.Dp(0)), el.Text(fmt.Sprintf("+%d", file.Additions)).Mono().TextSize(11).TextColor(theme.Success), el.Text(fmt.Sprintf("−%d", file.Deletions)).Mono().TextSize(11).TextColor(theme.DangerText)))
		if len(file.Hunks) == 0 {
			canvas.Child(inspectorNotice("此文件没有可预览的文本改动。", false))
		}
		for _, hunk := range file.Hunks {
			vertical += 28
			label := map[string]string{"staged": "已暂存", "worktree": "工作区", "untracked": "新增文件"}[hunk.Scope]
			canvas.Child(el.Div().Row().Items(el.Center).H(el.Dp(28)).Bg(mixColor(theme.RGB(0x1f6475), theme.Surface, .08)).Child(el.Text(label).TextSize(10).Medium().TextColor(theme.RGB(0x1f6475)).Pl(12).W(el.Dp(74)), el.Text(hunk.Header).Mono().TextSize(11).TextColor(theme.RGB(0x1f6475)).MaxLines(1).Grow().W(el.Dp(0))))
			for _, line := range hunk.Lines {
				if remaining <= 0 {
					canvas.Child(button("显示更多行", func() { state.Limit += 500 }, kit.ButtonGhost).Compact(true).Render(cx))
					return root.Child(a.reviewLayout(cx, state, diff.Child(canvas), diffID, offsets, width))
				}
				remaining--
				vertical += 21
				old, new := "", ""
				if line.Old > 0 {
					old = strconv.Itoa(line.Old)
				}
				if line.New > 0 {
					new = strconv.Itoa(line.New)
				}
				fg := theme.Text
				marker := " "
				bg := theme.Bg
				switch line.Kind {
				case "add":
					marker = "+"
					bg = mixColor(theme.Success, theme.Surface, .12)
				case "delete":
					marker = "−"
					bg = mixColor(theme.DangerText, theme.Surface, .11)
				case "meta":
					fg = theme.Muted
				}
				markerColor := theme.Muted
				if line.Kind == "add" {
					markerColor = theme.Success
				} else if line.Kind == "delete" {
					markerColor = theme.DangerText
				}
				canvas.Child(el.Div().Row().Items(el.Center).H(el.Dp(21)).Bg(bg).Child(el.Text(old).Mono().TextSize(11.5).TextAlign(el.End).TextColor(theme.Muted).Px(7).W(el.Dp(42)), el.Text(new).Mono().TextSize(11.5).TextAlign(el.End).TextColor(theme.Muted).Px(7).W(el.Dp(42)), el.Text(marker).Mono().TextSize(11.5).TextColor(markerColor).TextAlign(el.Center).W(el.Dp(18)), el.Text(line.Text).Mono().TextSize(11.5).TextColor(fg).MaxLines(1).Pr(18)))
			}
		}
	}
	return root.Child(a.reviewLayout(cx, state, diff.Child(canvas), diffID, offsets, width))
}

func (a *app) reviewLayout(cx *el.Context, state *reviewWorkspace, diff el.Element, diffID string, offsets map[string]float32, width float32) el.Element {
	layout := el.Div().Row().Grow().H(el.Dp(0)).Child(diff)
	if width <= 560 {
		return layout
	}
	treeWidth := max(float32(190), width*.27)
	tree := el.Div().W(el.Dp(treeWidth)).NoShrink().HFull().Child(el.Div().Row().Items(el.Center).Px(12).H(el.Dp(36)).Child(el.Text("改动文件").TextSize(11).TextColor(theme.Muted).Grow(), el.Text(fmt.Sprint(len(state.Files))).TextSize(11)))
	files := el.Div().Grow().H(el.Dp(0)).ScrollY()
	for _, file := range state.Files {
		entry := file
		row := button("", func() { state.Selected = entry.Path; cx.ScrollTo(diffID, offsets[entry.Path]) }, kit.ButtonGhost).Size(48).ID("review-file/" + entry.Path).Name(entry.Path).Selected(state.Selected == entry.Path).Content(el.ViewFunc(func(cx *el.Context) el.Element {
			return el.Div().Row().W(el.Dp(treeWidth-20)).Items(el.Center).Gap(7).Child(lucide(cx, "file-diff", 14, theme.Muted), el.Div().Grow().W(el.Dp(0)).Gap(2).Child(el.Text(filepath.Base(entry.Path)).TextSize(11).Medium().MaxLines(1), el.Text(filepath.Dir(entry.Path)).TextSize(10).TextColor(theme.Muted).MaxLines(1)), el.Text(fmt.Sprintf("+%d −%d", entry.Additions, entry.Deletions)).Mono().TextSize(10).TextColor(theme.Muted))
		})).Render(cx)
		if box, ok := row.(*el.DivEl); ok {
			box.WFull().Px(10).MinH(el.Dp(48))
		}
		files.Child(row)
	}
	return layout.Child(el.Div().W(el.Dp(1)).NoShrink().HFull().Bg(seamColor), tree.Child(files))
}
