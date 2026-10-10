package keeldesktop

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/theme"
)

func (s *reviewWorkspace) collapseAll(collapsed bool) {
	if s.Collapsed == nil {
		s.Collapsed = map[string]bool{}
	}
	for _, f := range s.Files {
		s.Collapsed[f.Path] = collapsed
	}
}
func (a *app) reviewPanelHeader(cx *el.Context, maximizeLabel, maximizeIcon string) *el.DivEl {
	id := a.workspaceContext()
	s := a.reviewState(id)
	if s.FilePicker == nil {
		s.FilePicker = kit.Popover(el.ViewFunc(func(cx *el.Context) el.Element {

			if s.FileSearchFocus {
				cx.Focus("review-file-search")
				s.FileSearchFocus = false
			}
			list := a.reviewFileList(cx, s, true)
			input := field("review-file-search", "搜索改动文件…", &s.FileQuery).H(el.Dp(32)).WFull().Bg(theme.Surface).FocusStyle(func(st *el.Style) { st.Bg(theme.Surface) }).OnChange(func(string) { s.FileSearchIndex = 0 })
			input.CaptureKeys("↓", "↑", "⏎", "⎋").OnKey(func(e el.KeyEvent) bool {
				if e.State != el.KeyPress {
					return false
				}
				files := reviewFilteredFiles(s)
				switch e.Name {
				case "↓":
					s.FileSearchIndex = max(0, min(len(files)-1, s.FileSearchIndex+1))
					cx.ScrollIntoView("review-file-results", float32(s.FileSearchIndex*32), float32((s.FileSearchIndex+1)*32))
					return true
				case "↑":
					s.FileSearchIndex = max(0, s.FileSearchIndex-1)
					cx.ScrollIntoView("review-file-results", float32(s.FileSearchIndex*32), float32((s.FileSearchIndex+1)*32))
					return true
				case "⏎":
					if len(files) > 0 {
						a.selectReviewFile(s, files[max(0, min(s.FileSearchIndex, len(files)-1))].Path)
					}
					return true
				case "⎋":
					s.FilePicker.SetValue(false)
					return true
				}
				return false
			})
			return el.Div().W(el.Dp(344)).Gap(6).Child(input, list)

		})).Width(360).PanelStyle(func(p *el.DivEl) { p.P(8) }).Placement(el.Bottom, el.End)
		s.FilePicker.Trigger(el.ViewFunc(func(cx *el.Context) el.Element {
			return iconAction(cx, "review-search-toggle", "搜索改动文件", "search", func() { s.FilePicker.Toggle(); s.FileSearchFocus = s.FilePicker.Value(); s.FileSearchIndex = 0 }, s.FilePicker.Value())
		}))
	}
	if s.Options == nil {
		m := kit.Menu().Width(220).CheckSide(el.Right).Placement(el.Bottom, el.End)
		m.CheckItem("显示改动文件", "", s.ShowFiles, func(v bool) { s.ShowFiles = v }).
			CheckItem("按目录分组", "", s.GroupFolders, func(v bool) { s.GroupFolders = v }).
			CheckItem("单独列出测试与生成文件", "", s.SeparateGenerated, func(v bool) { s.SeparateGenerated = v }).Separator().
			Item("折叠所有文件", "", func() { s.collapseAll(true) }).
			Item("展开所有文件", "", func() { s.collapseAll(false) }).Separator().
			CheckItem("并排对比", "", s.SideBySide, func(v bool) { s.SideBySide = v }).
			CheckItem("自动换行", "", !s.NoWrap, func(v bool) { s.NoWrap = !v }).
			CheckItem("差异背景高亮", "", !s.NoHighlight, func(v bool) { s.NoHighlight = !v }).
			CheckItem("隐藏空白差异", "", s.HideWhitespace, func(v bool) { s.HideWhitespace = v }).Separator().
			Item("刷新", "", func() { a.loadReview(id) })
		m.Trigger(el.ViewFunc(func(cx *el.Context) el.Element {
			return iconAction(cx, "review-options", "差异显示选项", "ellipsis-vertical", m.Toggle, m.Value())
		}))
		s.Options = m
	}

	s.Options.SetItemChecked("并排对比", s.SideBySide)
	s.Options.SetItemChecked("自动换行", !s.NoWrap)
	branch := "HEAD"
	if a.captionGit != nil && a.captionGit.Branch != "" {
		branch = a.captionGit.Branch
	}
	label, icon := maximizeLabel, maximizeIcon

	bar := a.panelHeader(cx, "")
	if a.workbench.Maximized && a.sidebarCollapsed(cx) {
		bar.Child(iconAction(cx, "show-sidebar", "展开侧栏", "panel-left", a.toggleSidebar, false))
	}
	s.Options.SetItemChecked("显示改动文件", s.ShowFiles)
	return bar.Child(iconAction(cx, "review-files", "显示改动文件", "list-tree", func() { s.ShowFiles = !s.ShowFiles }, s.ShowFiles), el.Text(branch+" → 工作区").TextSize(12).TextColor(theme.Muted).MaxLines(1).Grow().W(el.Dp(0)), s.FilePicker.Render(cx),
		s.Options.Render(cx), iconAction(cx, "maximize-inspector", label, icon, func() { a.workbench.Maximized = !a.workbench.Maximized }, false),
		iconAction(cx, "hide-inspector", "收起检查面板", "x", func() { a.workbench.Visible = false; a.workbench.Maximized = false }, false))

}
func reviewFileGroup(path string) string {
	name := strings.ToLower(filepath.Base(path))
	if strings.HasSuffix(name, "_test.go") || strings.Contains(name, ".test.") || strings.Contains(name, ".spec.") {
		return "测试文件"
	}
	if strings.Contains(name, ".gen.") || strings.HasSuffix(name, ".pb.go") || strings.HasSuffix(name, ".lock") || strings.Contains(path, "/generated/") || strings.HasPrefix(path, "generated/") {
		return "生成文件"
	}
	return "源文件"
}
func orderedReviewFiles(s *reviewWorkspace) []reviewFile {
	if !s.GroupFolders && !s.SeparateGenerated {
		return s.Files
	}
	files := append([]reviewFile(nil), s.Files...)
	if s.GroupFolders || s.SeparateGenerated {
		sort.SliceStable(files, func(i, j int) bool {
			if s.SeparateGenerated && reviewFileGroup(files[i].Path) != reviewFileGroup(files[j].Path) {
				return reviewFileGroup(files[i].Path) < reviewFileGroup(files[j].Path)
			}
			if s.GroupFolders {
				return files[i].Path < files[j].Path
			}
			return false
		})
	}
	return files
}

func pairedReviewLines(lines []reviewLine) [][2]*reviewLine {
	var pairs [][2]*reviewLine
	for i := 0; i < len(lines); {
		if lines[i].Kind != "add" && lines[i].Kind != "delete" {
			line := lines[i]
			pairs = append(pairs, [2]*reviewLine{&line, &line})
			i++
			continue
		}
		var old, new []reviewLine
		for i < len(lines) && (lines[i].Kind == "add" || lines[i].Kind == "delete") {
			if lines[i].Kind == "delete" {
				old = append(old, lines[i])
			} else {
				new = append(new, lines[i])
			}
			i++
		}
		for j := 0; j < max(len(old), len(new)); j++ {
			var pair [2]*reviewLine
			if j < len(old) {
				pair[0] = &old[j]
			}
			if j < len(new) {
				pair[1] = &new[j]
			}
			pairs = append(pairs, pair)
		}
	}
	return pairs
}
func withoutWhitespaceChanges(lines []reviewLine) []reviewLine {
	var result []reviewLine
	for _, pair := range pairedReviewLines(lines) {
		old, new := pair[0], pair[1]
		if old != nil && new != nil && old.Kind == "delete" && new.Kind == "add" && strings.Join(strings.Fields(old.Text), "") == strings.Join(strings.Fields(new.Text), "") {
			line := *new
			line.Kind = "context"
			line.Old = old.Old
			result = append(result, line)
			continue
		}
		if old != nil {
			result = append(result, *old)
		}
		if new != nil && new != old {
			result = append(result, *new)
		}
	}
	return result
}

func reviewFilteredFiles(s *reviewWorkspace) []reviewFile {
	if strings.TrimSpace(s.FileQuery) == "" {
		return orderedReviewFiles(s)
	}
	var files []reviewFile
	for _, f := range orderedReviewFiles(s) {
		if strings.Contains(strings.ToLower(f.Path), strings.ToLower(strings.TrimSpace(s.FileQuery))) {
			files = append(files, f)
		}
	}
	return files
}
func (a *app) selectReviewFile(s *reviewWorkspace, path string) {
	if s.Collapsed == nil {
		s.Collapsed = map[string]bool{}
	}
	s.Collapsed[path] = false
	s.Selected = path
	s.Jump = path
	s.StartPath = path
	s.Limit = 500
	for _, file := range s.Files {
		if file.Path == path && (file.Deferred || s.Loading) {
			a.loadReviewBatch(a.workspaceContext(), false)
			break
		}
	}
	if s.FilePicker != nil {
		s.FilePicker.SetValue(false)
	}
}
func (a *app) reviewFileList(cx *el.Context, s *reviewWorkspace, search bool) el.Element {
	width := float32(208)
	files := orderedReviewFiles(s)
	if search {
		width = 344
		files = reviewFilteredFiles(s)
	}
	listID := "review-file-navigation"
	if search {
		listID = "review-file-results"
	}
	list := el.Div().ID(listID).W(el.Dp(width)).MaxH(el.Dp(360)).ScrollY()
	if search {
		list.ID("review-file-results")
	}
	if !search {
		list.MaxH(el.Auto).Grow().H(el.Dp(0))
	}
	offset, viewport, _ := cx.ScrollState(listID)
	if viewport <= 0 {
		viewport = 800
	}
	first := max(0, min(len(files), int(offset/32)-2))
	last := min(len(files), first+int(viewport/32)+5)
	if first > 0 {
		list.Child(el.Div().H(el.Dp(float32(first * 32))).NoShrink())
	}
	for i := first; i < last; i++ {
		i, f := i, files[i]
		dir := filepath.Dir(f.Path)
		if dir == "." {
			dir = ""
		}
		row := el.Div().W(el.Dp(width-10)).H(el.Dp(32)).Row().Items(el.Center).Px(6).Gap(5).Rounded(5).
			Role("button").Name("跳转到 "+f.Path).Focusable(true).CursorPointer().FocusStyle(neutralFocus).Hover(func(st *el.Style) { st.Bg(theme.SubtleHover) }).
			OnClick(func() { a.selectReviewFile(s, f.Path) }).
			Child(lucide(cx, "file-diff", 12, theme.Success),
				el.Div().Row().Items(el.Center).Gap(4).Grow().W(el.Dp(0)).Child(el.Text(filepath.Base(f.Path)).TextSize(11).MaxLines(1), el.Text(dir).TextSize(10).TextColor(theme.Muted).MaxLines(1).Grow().W(el.Dp(0))),
				el.Text(fmt.Sprintf("+%d", f.Additions)).TextSize(10).TextColor(theme.Success).NoShrink(), el.Text(fmt.Sprintf("−%d", f.Deletions)).TextSize(10).TextColor(theme.DangerText).NoShrink())
		if search && i == s.FileSearchIndex || !search && s.Selected == f.Path {
			row.Bg(theme.Subtle)
		}
		list.Child(row)
	}
	if last < len(files) {
		list.Child(el.Div().H(el.Dp(float32((len(files) - last) * 32))).NoShrink())
	}
	if len(files) == 0 {
		list.Child(el.Text("没有匹配的文件").TextSize(12).TextColor(theme.Muted).P(10))
	}
	return list
}

func reviewAdditionLabel(f reviewFile) string {
	if f.Deferred {
		return ""
	}
	return fmt.Sprintf("+%d", f.Additions)
}
func reviewDeletionLabel(f reviewFile) string {
	if f.Deferred {
		return ""
	}
	return fmt.Sprintf("−%d", f.Deletions)
}
