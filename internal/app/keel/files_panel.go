package keeldesktop

import (
	"context"
	"path"
	"strings"
	"time"

	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	_ "github.com/dyike/keel/ui/highlight"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/theme"
	uiassets "github.com/openmodu/onecatch/internal/app/keel/assets"
	desktopservice "github.com/openmodu/onecatch/internal/service/desktop"
)

func fileTabPaths(s *fileWorkspace) []string {
	paths := make([]string, 0, len(s.Buffers))
	seen := map[string]bool{}
	for _, p := range s.Tabs {
		if s.Buffers[p] != nil && !seen[p] {
			paths = append(paths, p)
			seen[p] = true
		}
	}
	for _, p := range sortedBufferPaths(s.Buffers) {
		if !seen[p] {
			paths = append(paths, p)
		}
	}
	return paths
}
func fileGlyph(cx *el.Context, name string) el.Element {
	label := ""
	fg := theme.Info
	switch strings.ToLower(path.Ext(name)) {
	case ".go":
		label = "go"
	case ".md":
		label = "M↓"
		fg = theme.Success
	case ".ts", ".tsx":
		label = "TS"
	case ".js", ".jsx":
		label = "JS"
		fg = theme.Warning
	case ".json":
		label = "{}"
		fg = theme.Warning
	}
	if label != "" {
		return el.Div().W(el.Dp(16)).NoShrink().Items(el.Center).Child(el.Text(label).TextSize(9).Medium().TextColor(fg))
	}
	return uiassets.Lucide(cx, "file-text", 14, theme.Muted)
}
func (a *app) filesPanelHeader(cx *el.Context, maximizeLabel, maximizeIcon string) *el.DivEl {
	id := a.workspaceContext()
	s := a.fileState(id)
	if s.Menu == nil {
		menu := kit.Menu().Width(1).Placement(el.Bottom, el.End)
		menu.Item("刷新文件列表", "", func() {
			a.refreshFileTree(id)
			if s.Query != "" {
				a.searchFiles(id)
			}
		}).
			Item("折叠所有目录", "", func() { clear(s.Expanded); s.TreeRevision++ }).
			Item("查找文件", "", func() { s.HideTree = false; s.SearchFocus = true }).
			CheckItem("自动换行", "", !s.NoWrap, func(on bool) { s.NoWrap = !on }).
			Item("在当前文件中查找", "", func() {
				if b := s.Buffers[s.Active]; b != nil {
					b.Editor.OpenSearch(false)
				}
			}).
			Separator().Item("保存当前文件", "⌘S", func() { a.saveFile(id, s.Active) }).
			Item("关闭当前文件", "", func() { a.closeFile(id, s.Active) })
		menu.Trigger(el.ViewFunc(func(cx *el.Context) el.Element {
			return iconAction(cx, "files-more", "文件更多操作", "ellipsis-vertical", menu.Toggle, menu.Value())
		}))
		s.Menu = menu
	}
	bar := a.panelHeader(cx, "")
	if a.workbench.Maximized && a.sidebarCollapsed(cx) {
		bar.Child(iconAction(cx, "show-sidebar", "展开侧栏", "panel-left", a.toggleSidebar, false))
	}
	bar.Child(iconAction(cx, "files-toggle-tree", "切换文件树", "list-tree", func() { s.HideTree = !s.HideTree }, !s.HideTree))
	tabs := el.Div().Row().Items(el.Center).Gap(3).Grow().W(el.Dp(0)).ScrollX().Scrollbars(el.ScrollbarHidden)
	paths := fileTabPaths(s)
	if len(paths) == 0 {
		tabs.Child(el.Text("Files").TextSize(12).TextColor(theme.Muted))
	}
	for _, name := range paths {
		p := name
		b := s.Buffers[p]
		label := path.Base(p)
		if b.dirty() {
			label += " •"
		}
		choose := el.Div().Row().Items(el.Center).Gap(4).Px(5).H(el.Dp(26)).Role("button").Name("打开标签 "+p).Focusable(true).FocusStyle(neutralFocus).CursorPointer().OnClick(func() { s.Version++; s.Loading = false; s.Active = p }).Child(fileGlyph(cx, p), el.Text(label).TextSize(11).MaxLines(1))
		tab := el.Div().Row().Items(el.Center).NoShrink().Rounded(5).Pr(2).Hover(func(st *el.Style) { st.Bg(theme.SubtleHover) }).Child(choose, iconAction(cx, "file-close/"+p, "关闭标签 "+p, "x", func() { a.closeFile(id, p) }, false))
		if s.Active == p {
			tab.Bg(theme.Subtle)
		}
		tabs.Child(tab)
	}
	return bar.Child(tabs, iconAction(cx, "files-find", "查找当前文件", "search", func() {
		if b := s.Buffers[s.Active]; b != nil {
			b.Editor.OpenSearch(false)
		} else {
			s.HideTree = false
			s.SearchFocus = true
		}
	}, false), s.Menu.Render(cx), iconAction(cx, "maximize-inspector", maximizeLabel, maximizeIcon, func() { a.workbench.Maximized = !a.workbench.Maximized }, false), iconAction(cx, "hide-inspector", "收起检查面板", "x", func() { a.workbench.Visible = false; a.workbench.Maximized = false }, false))
}
func (a *app) searchFiles(id string) {
	s := a.fileState(id)
	s.SearchVersion++
	version := s.SearchVersion
	if s.SearchCancel != nil {
		s.SearchCancel()
	}
	s.SearchIndex = 0
	s.SearchResult = desktopservice.WorkspaceFileSearch{}
	query := s.Query
	s.SearchLoading = strings.TrimSpace(query) != ""
	if !s.SearchLoading {
		return
	}
	ctx, cancel := context.WithTimeout(a.ctx, 15*time.Second)
	s.SearchCancel = cancel
	go func() {
		defer cancel()
		timer := time.NewTimer(250 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		result, err := a.backend.service.SearchWorkspaceFiles(ctx, id, query)
		core.Update(func() {
			if a.closed || s.SearchVersion != version {
				return
			}
			s.SearchLoading = false
			if err != nil {
				if ctx.Err() != context.Canceled {
					s.Error = err.Error()
				}
				return
			}
			s.Error = ""
			s.SearchResult = result
		})
	}()
}
func (a *app) openFileMatch(id string, match desktopservice.WorkspaceFileMatch) {
	s := a.fileState(id)
	s.OpenLine = match.Line
	for dir := path.Dir(match.Path); dir != "." && dir != ""; dir = path.Dir(dir) {
		s.Expanded[dir] = true
		s.TreeRevision++
		if s.Tree[dir] == nil && !s.DirectoryLoading[dir] {
			a.loadDirectory(id, dir)
		}
	}
	a.openWorkspaceFile(id, match.Path)
}

type fileTreeRow struct {
	entry   desktopservice.WorkspaceFileEntry
	depth   int
	loading bool
}

func visibleFileRows(s *fileWorkspace) []fileTreeRow {
	if s.Rows != nil && s.RowsRevision == s.TreeRevision {
		return s.Rows
	}
	var rows []fileTreeRow
	var walk func(string, int)
	walk = func(parent string, depth int) {
		if depth > 32 {
			return
		}
		entries := s.Tree[parent]
		if parent == "" && entries == nil {
			entries = s.Entries
		}
		for _, entry := range entries {
			rows = append(rows, fileTreeRow{entry: entry, depth: depth})
			if entry.Directory && s.Expanded[entry.Path] {
				walk(entry.Path, depth+1)
			}
		}
		if s.DirectoryLoading[parent] {
			rows = append(rows, fileTreeRow{depth: depth, loading: true})
		}
	}
	walk("", 0)
	s.Rows = rows
	s.RowsRevision = s.TreeRevision
	return rows
}
func (a *app) fileInspector(cx *el.Context) el.Element {
	id := a.workspaceContext()
	s := a.fileState(id)
	if id == "" {
		return inspectorNotice("选择项目后浏览文件。", false)
	}
	editor := el.Div().Grow().W(el.Dp(0)).HFull()
	if b := s.Buffers[s.Active]; b != nil {
		breadcrumb := strings.ReplaceAll(s.Active, "/", " / ")
		editor.Child(el.Div().Row().Items(el.Center).H(el.Dp(30)).Px(10).Child(el.Text(breadcrumb).TextSize(11).TextColor(theme.Muted).MaxLines(1).Grow().W(el.Dp(0))))
		if b.Error != "" {
			name := s.Active
			editor.Child(inspectorNotice(b.Error, true), button("重新读取文件", func() { a.reloadFile(id, name) }, kit.ButtonGhost).Compact(true).Render(cx))
		}
		b.Editor.SoftWrap(!s.NoWrap)
		palette := theme.Current()
		palette.CodeBg = theme.Surface
		editor.Child(cx.Themed(palette, el.ViewFunc(func(cx *el.Context) el.Element { return b.Editor.Render(cx) })).Grow().H(el.Dp(0)))
	} else {
		editor.Child(el.Div().Grow().WFull().Items(el.Center).Justify(el.Center).Gap(8).Child(uiassets.Lucide(cx, "folder", 24, theme.Muted), el.Text("打开的文件会显示在这里").TextSize(12), el.Text("从文件树中选择文件").TextSize(11).TextColor(theme.Muted)))
	}
	if s.Loading {
		editor.Child(el.Text("正在打开文件…").TextSize(11).TextColor(theme.Muted).P(6))
	}
	body := el.Div().Row().Grow().H(el.Dp(0)).WFull()
	if !s.HideTree {
		width := min(float32(230), max(float32(160), a.inspectorWidth(cx)*.3))
		tree := el.Div().W(el.Dp(width)).NoShrink().HFull().Px(5)
		if s.SearchFocus {
			cx.Focus("files-filter")
			s.SearchFocus = false
		}
		filter := field("files-filter", "筛选文件，? 搜索内容", &s.Query).H(el.Dp(30)).WFull().TextSize(11).Bg(theme.Surface).FocusStyle(neutralFocus).OnChange(func(string) { a.searchFiles(id); cx.ScrollTo("files-results", 0) })
		filter.CaptureKeys("↓", "↑", "⏎", "⎋").OnKey(func(e el.KeyEvent) bool {
			if e.State != el.KeyPress {
				return false
			}
			matches := s.SearchResult.Matches
			switch e.Name {
			case "⎋":
				s.Query = ""
				a.searchFiles(id)
				return true
			case "↓":
				s.SearchIndex = min(len(matches)-1, s.SearchIndex+1)
			case "↑":
				s.SearchIndex = max(0, s.SearchIndex-1)
			case "⏎":
				if len(matches) > 0 {
					a.openFileMatch(id, matches[max(0, s.SearchIndex)])
				}
				return true
			default:
				return false
			}
			cx.ScrollIntoView("files-results", float32(max(0, s.SearchIndex)*28), float32((max(0, s.SearchIndex)+1)*28))
			return true
		})
		tree.Child(el.Div().Row().Items(el.Center).Gap(3).Child(uiassets.Lucide(cx, "search", 12, theme.Muted), filter.Grow().W(el.Dp(0))))
		if s.Error != "" {
			tree.Child(inspectorNotice(s.Error, true))
		}
		searching := strings.TrimSpace(s.Query) != ""
		listID := "file-tree"
		if searching {
			listID = "files-results"
		}
		list := el.Div().ID(listID).Role("tree").Name("工作区文件").Grow().H(el.Dp(0)).ScrollY()
		rows := visibleFileRows(s)
		if searching {
			rows = nil
			for _, m := range s.SearchResult.Matches {
				rows = append(rows, fileTreeRow{entry: desktopservice.WorkspaceFileEntry{Path: m.Path, Name: path.Base(m.Path)}})
			}
		}
		offset, view, _ := cx.ScrollState(listID)
		if view <= 0 {
			view = 800
		}
		first := max(0, min(len(rows), int(offset/28)-2))
		last := min(len(rows), first+int(view/28)+5)
		if first > 0 {
			list.Child(el.Div().H(el.Dp(float32(first * 28))).NoShrink())
		}
		for i := first; i < last; i++ {
			i, item := i, rows[i]
			entry := item.entry
			if item.loading {
				list.Child(el.Div().H(el.Dp(28)).Child(el.Text("读取中…").TextSize(10).TextColor(theme.Muted)))
				continue
			}
			row := el.Div().Row().Items(el.Center).Pl(float32(4 + item.depth*12)).Pr(4).H(el.Dp(28)).NoShrink().Gap(4).Rounded(4).ID("file-entry/" + entry.Path).Role("treeitem").Name(entry.Path).Focusable(true).FocusStyle(neutralFocus).CursorPointer().Hover(func(st *el.Style) { st.Bg(theme.SubtleHover) }).OnClick(func() {
				if searching {
					a.openFileMatch(id, s.SearchResult.Matches[i])
					return
				}
				if entry.Directory {
					s.Expanded[entry.Path] = !s.Expanded[entry.Path]
					s.TreeRevision++
					if s.Expanded[entry.Path] && s.Tree[entry.Path] == nil && !s.DirectoryLoading[entry.Path] {
						a.loadDirectory(id, entry.Path)
					}
				} else {
					s.OpenLine = 0
					a.openWorkspaceFile(id, entry.Path)
				}
			})
			if s.Active == entry.Path || searching && s.SearchIndex == i {
				row.Bg(theme.Subtle)
			}
			if entry.Directory {
				icon := "chevron-right"
				if s.Expanded[entry.Path] {
					icon = "chevron-down"
				}
				row.Child(uiassets.Lucide(cx, icon, 12, theme.Muted))
			} else {
				row.Child(fileGlyph(cx, entry.Path))
			}
			row.Child(el.Text(entry.Name).TextSize(11).MaxLines(1).Grow().W(el.Dp(0)))
			if searching {
				dir := path.Dir(entry.Path)
				if dir != "." {
					row.Child(el.Text(dir).TextSize(10).TextColor(theme.Muted).MaxLines(1).MaxW(el.Frac(.4)))
				}
			}
			list.Child(row)
		}
		if last < len(rows) {
			list.Child(el.Div().H(el.Dp(float32((len(rows) - last) * 28))).NoShrink())
		}
		if searching && len(rows) == 0 {
			label := "没有匹配的文件"
			if s.SearchLoading {
				label = "搜索中…"
			}
			list.Child(el.Text(label).TextSize(11).TextColor(theme.Muted).P(8))
		}
		tree.Child(list)
		if s.SearchResult.Truncated && searching {
			tree.Child(el.Text("结果已截断，请缩小搜索范围").TextSize(10).TextColor(theme.Muted).P(4))
		}
		body.Child(tree, el.Div().W(el.Dp(1)).HFull().NoShrink().Bg(theme.Subtle))
	}
	return body.Child(editor)
}
