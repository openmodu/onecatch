package keeldesktop

import (
	"context"
	"encoding/json"
	"fmt"
	"image/color"
	"path"
	"runtime"
	"sort"
	"strings"
	"time"

	"gioui.org/io/pointer"
	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/theme"
	"github.com/dyike/keel/ui/window"
	domainworkspaces "github.com/openmodu/onecatch/internal/domain/workspaces"
	desktopservice "github.com/openmodu/onecatch/internal/service/desktop"
	"github.com/openmodu/onecatch/internal/service/worker"
)

// All view state belongs to the serialized UI queue. Files are cached by their
// execution context so a tab switch or task navigation cannot discard a draft.
type workbenchState struct {
	Visible, Maximized                bool
	Resizing                          bool
	DetachedWindow                    *window.Window
	Confirm                           *kit.DialogView
	MoreMenu                          *kit.MenuView
	Tab                               string
	Width, DragX                      float32
	Context, Error, Source, NewBranch string
	SourceFollowKey                   string
	Version                           uint64
	Loading, Mutating                 bool
	Snapshot                          *domainworkspaces.GitSnapshot
	Branches                          []domainworkspaces.GitBranch
	Workers                           []worker.Info
	SourcePicker, BranchPicker        *kit.SelectView
	Files                             map[string]*fileWorkspace
	Reviews                           map[string]*reviewWorkspace
	WorktreeMode                      string
	Worktree                          *domainworkspaces.Worktree
	Worktrees                         []domainworkspaces.Worktree
	WorktreeMenu                      *kit.MenuView
	WorktreeVersion                   uint64
}
type fileWorkspace struct {
	Directory, Active, Error string
	Entries                  []desktopservice.WorkspaceFileEntry
	Buffers                  map[string]*fileBuffer
	Loading                  bool
	Version                  uint64
	Tree                     map[string][]desktopservice.WorkspaceFileEntry
	Expanded                 map[string]bool
	DirectoryVersions        map[string]uint64
	DirectoryLoading         map[string]bool
}
type fileBuffer struct {
	Document        desktopservice.WorkspaceFileDocument
	Editor          *kit.CodeEditorView
	Saving          bool
	AutoSaveVersion uint64
	Error           string
}
type reviewWorkspace struct {
	Snapshot  *domainworkspaces.GitSnapshot
	Cancel    context.CancelFunc
	StartPath string

	ShowFiles, FileSearchFocus                                          bool
	FileSearchIndex                                                     int
	ContextLines                                                        int
	NoHighlight                                                         bool
	Options                                                             *kit.MenuView
	FilePicker                                                          *kit.PopoverView
	FileQuery                                                           string
	GroupFolders, SeparateGenerated, SideBySide, NoWrap, HideWhitespace bool
	Jump                                                                string

	Collapsed                                      map[string]bool
	ExpandedContext                                map[string]bool
	Query                                          string
	SearchOpen                                     bool
	CommentKey, Comment, CommentPath, CommentScope string
	CommentLine                                    int
	Files                                          []reviewFile
	Selected, Error                                string
	Loading                                        bool
	Version                                        uint64
	Limit                                          int
}

func (a *app) initWorkbench() {
	w := &a.workbench
	w.Tab, w.Width, w.Source = "status", 380, "local"
	w.Confirm = kit.Dialog("").Width(420)
	w.Files = map[string]*fileWorkspace{}
	w.Reviews = map[string]*reviewWorkspace{}
	w.SourcePicker = kit.Select("").Hint("Git 来源").Appearance(false).Size(36).MenuWidth(260).OnChange(a.selectInspectorGitSource)
	w.BranchPicker = kit.Select("").Appearance(false).Size(32).MenuWidth(280).OnChange(func(name string) { a.changeBranch(name, false) })
	a.buildWorktreeMenu()
	a.initProjects()
}
func (a *app) toggleInspector(tab string) {
	if a.workbench.DetachedWindow != nil {
		a.workbench.DetachedWindow.Raise()
		return
	}
	a.terminal.Open, a.terminal.Maximized = false, false
	if tab == "" || a.workbench.Visible && a.workbench.Tab == tab {
		a.workbench.Visible = !a.workbench.Visible
	} else {
		a.workbench.Visible = true
		a.workbench.Tab = tab
	}
	if a.workbench.Visible {
		a.loadInspector()
	}
}
func (a *app) inspectorWidth(cx *el.Context) float32 {
	width, _ := cx.ViewportSize()
	if !a.sidebarCollapsed(cx) {
		width -= a.sidebarWidth(cx) + 1
	}
	if a.workbench.Maximized || a.page == "" && a.terminal.Maximized {
		return max(0, width-12)
	}
	return min(a.workbench.Width, max(0, width-292))
}
func (a *app) inspectorSeam(cx *el.Context) el.Element {
	line := el.Div().W(el.Dp(3)).H(el.Dp(64)).Rounded(2).NoShrink()
	if cx.Hovered("inspector-resize") || a.workbench.Resizing {
		line.Bg(mixColor(theme.Muted, theme.Bg, .45))
	}
	return el.Div().W(el.Dp(12)).NoShrink().HFull().Center().Child(line).ID("inspector-resize").Role("separator").Name("调整检查面板宽度").Focusable(true).Cursor(pointer.CursorColResize).OnDrag(func(e el.DragEvent) {
		switch e.Kind {
		case el.DragStart:
			a.workbench.DragX = e.X
			a.workbench.Resizing = true
			a.workbench.Width = a.inspectorWidth(cx)
		case el.DragMove:
			a.workbench.Width = max(240, min(820, a.inspectorWidth(cx)+a.workbench.DragX-e.X))
		case el.DragEnd:
			a.workbench.Resizing = false
		}
	}).OnKey(func(e el.KeyEvent) bool {
		if e.State != el.KeyPress {
			return false
		}
		switch e.Name {
		case "←":
			a.workbench.Width = min(820, a.workbench.Width+12)
			return true
		case "→":
			a.workbench.Width = max(320, a.workbench.Width-12)
			return true
		}
		return false
	})
}

var inspectorTabs = []struct{ ID, Label, Icon string }{{"status", "状态", "activity"}, {"files", "文件", "file-code-2"}, {"git", "Git", "git-branch"}, {"review", "审阅", "file-diff"}, {"events", "事件", "list-tree"}}

func (a *app) inspector(cx *el.Context) el.Element {
	tabs := el.Div().Row().Items(el.Center).Gap(2).Grow().W(el.Dp(0)).P(3).Rounded(8).Bg(mixColor(theme.Subtle, theme.Bg, .72))
	title := "状态"
	for _, item := range inspectorTabs {
		it := item
		if a.workbench.Tab == it.ID {
			title = it.Label
		}
		tabs.Child(iconAction(cx, "inspector-tab/"+it.ID, it.Label, it.Icon, func() { a.workbench.Tab = it.ID; a.loadInspector() }, a.workbench.Tab == it.ID))
	}

	bar := el.Div().Row().Items(el.Center).H(el.Dp(44)).Px(8).Gap(6).Child(tabs, dragArea(cx, el.Div().Row().Items(el.Center).HFull().Child(el.Text(title).TextSize(12).Medium().MaxLines(1))))
	if a.workbench.DetachedWindow != nil {
		bar.Child(iconAction(cx, "dock-inspector", "停靠检查面板", "picture-in-picture-2", func() { a.workbench.DetachedWindow.Close() }, false))
	} else {
		maxLabel, maxIcon := "放大状态栏", "maximize-2"
		if a.workbench.Maximized {
			maxLabel, maxIcon = "还原状态栏", "minimize-2"
		}
		bar.Child(iconAction(cx, "maximize-inspector", maxLabel, maxIcon, func() { a.workbench.Maximized = !a.workbench.Maximized }, false), iconAction(cx, "detach-inspector", "在独立窗口打开", "square-arrow-out-up-right", a.detachInspector, false), iconAction(cx, "hide-inspector", "收起检查面板", "panel-right-close", func() { a.workbench.Visible = false; a.workbench.Maximized = false }, false))
	}
	if runtime.GOOS == "darwin" && (a.workbench.DetachedWindow != nil || a.workbench.Maximized && a.ui.Preferences.Collapsed) {
		bar.Pl(94)
	}

	if a.workbench.DetachedWindow == nil {
		bar = a.inspectorPanelHeader(cx)
	}
	var body el.Element
	a.syncInspectorGitSource()
	switch a.workbench.Tab {
	case "git":
		body = a.gitInspector(cx)
	case "files":
		body = a.fileInspector(cx)
	case "review":
		body = a.reviewInspector(cx)
	case "events":
		body = a.eventInspector(cx)
	default:
		body = a.statusInspector(cx)
	}
	pane := el.Div().ID("inspector").Name("检查面板").NoShrink().HFull().Child(a.workbench.Confirm.Render(cx), bar, body)
	pane.WFull()
	return pane
}
func (a *app) loadInspector() {
	id := a.workspaceContext()
	if a.workbench.Context != id {
		a.workbench.Context = id
		a.workbench.Snapshot = nil
		a.workbench.Branches = nil
		a.workbench.NewBranch = ""
		a.workbench.Error = ""
		a.workbench.Version++
	}
	switch a.workbench.Tab {
	case "git":
		a.loadInspectorGit()
	case "files":
		a.loadDirectory(id, a.fileState(id).Directory)
	case "review":
		a.loadReview(id)
	}
}
func (a *app) loadInspectorGit() {
	w := &a.workbench
	a.syncInspectorGitSource()
	id, source := a.workspaceContext(), w.Source
	w.Version++
	version := w.Version
	if id == "" {
		w.Loading = false
		return
	}
	w.Loading, w.Error = true, ""
	go func() {
		ctx, cancel := context.WithTimeout(a.ctx, 15*time.Second)
		defer cancel()
		result := readInspectorGit(ctx, a.backend.service, id, source)
		core.Update(func() { a.applyInspectorGit(version, id, source, result) })
	}()
}
func worktreeBound(id string) bool {
	return strings.HasPrefix(id, "task:") || strings.HasPrefix(id, "worktree:")
}
func gitBranchLabel(snapshot *domainworkspaces.GitSnapshot) string {
	if snapshot == nil {
		return "读取中…"
	}
	if snapshot.Branch != "" {
		return snapshot.Branch
	}
	if len(snapshot.Head) >= 7 {
		return snapshot.Head[:7]
	}
	return "尚无提交"
}
func (a *app) changeBranch(name string, create bool) {
	w := &a.workbench
	id := a.workspaceContext()
	name = strings.TrimSpace(name)
	if w.Mutating || w.Loading || name == "" || worktreeBound(id) || w.Source != "local" {
		return
	}
	if !create && w.Snapshot != nil && name == w.Snapshot.Branch {
		return
	}
	w.Mutating = true
	w.Error = ""
	go func() {
		var err error
		if create {
			_, err = a.backend.service.GitCreateBranch(a.ctx, id, name)
		} else {
			_, err = a.backend.service.GitSwitchBranch(a.ctx, id, name)
		}
		core.Update(func() {
			if a.closed {
				return
			}
			w.Mutating = false
			if id != a.workspaceContext() || w.Source != "local" {
				return
			}
			if err != nil {
				w.Error = err.Error()
				if w.Snapshot != nil {
					w.BranchPicker.SetValue(w.Snapshot.Branch)
				}
				return
			}
			w.NewBranch = ""
			a.loadContextGit()
			a.loadInspectorGit()
		})
	}()
}
func inspectorNotice(text string, failure bool) el.Element {
	fg := theme.Muted
	if failure {
		fg = theme.DangerText
	}
	return el.Div().Px(14).Py(16).Child(el.Text(text).TextSize(12).TextColor(fg))
}
func (a *app) gitInspector(cx *el.Context) el.Element {
	w := &a.workbench
	if a.workspaceContext() == "" {
		return inspectorNotice("选择项目后查看 Git 状态。", false)
	}
	column := el.Div().Grow().H(el.Dp(0)).ScrollY().P(14).Gap(14)
	if len(w.Workers) > 0 {
		w.SourcePicker.SetDisabled(w.Mutating)
		control := w.SourcePicker.Render(cx)
		if row, ok := control.(*el.DivEl); ok {
			row.Name("Git 来源")
		}
		column.Child(el.Div().Rounded(6).Bg(theme.Subtle).Child(control))
	}
	head := ""
	if w.Snapshot != nil {
		head = w.Snapshot.Head
		if len(head) > 7 {
			head = head[:7]
		}
	}
	refresh := button("刷新", a.loadInspectorGit, kit.ButtonGhost).ID("refresh-git").Name("刷新 Git").Size(24).Compact(true)
	refresh.SetDisabled(w.Loading)
	header := el.Div().Row().Items(el.Start).Gap(8).Child(el.Div().Grow().W(el.Dp(0)).Child(el.Text("仓库").TextSize(11).LineHeight(1.5).TextColor(theme.Muted), el.Text(gitBranchLabel(w.Snapshot)).TextSize(14).Bold().LineHeight(1.43).Mt(4).MaxLines(1), el.Text(head).Mono().TextSize(11).LineHeight(1.5).Mt(2).TextColor(theme.Muted)), refresh.Render(cx))
	column.Child(header)
	if w.Error != "" {
		column.Child(inspectorNotice(w.Error, true))
	}
	if w.Loading {
		column.Child(el.Text("正在读取…").TextSize(11).TextColor(theme.Muted))
	}
	if w.Snapshot == nil {
		return column
	}
	s := w.Snapshot
	if !s.IsRepo {
		return column.Child(el.Text("这个项目不是 Git 仓库。").TextSize(12).TextColor(theme.Muted))
	}
	changeLabel, changeColor := fmt.Sprintf("%d 个变更", len(s.Files)), theme.Warning
	if len(s.Files) == 0 {
		changeLabel, changeColor = "工作目录干净", theme.Success
	}
	column.Child(el.Div().Row().Items(el.Center).Rounded(6).Bg(mixColor(theme.Subtle, theme.Bg, .5)).Px(10).Py(8).Gap(12).Child(el.Text(fmt.Sprintf("↑ %d  ↓ %d", s.Ahead, s.Behind)).TextSize(11).TextColor(theme.Muted), el.Div().Grow(), el.Text(changeLabel).TextSize(11).TextColor(changeColor)))
	if !worktreeBound(a.workspaceContext()) && w.Source == "local" {
		w.BranchPicker.SetDisabled(w.Mutating || w.Loading)
		create := button("创建", func() { a.changeBranch(w.NewBranch, true) }, kit.ButtonSecondary).Size(30).Compact(true)
		create.SetDisabled(w.Mutating || w.Loading || strings.TrimSpace(w.NewBranch) == "")
		column.Child(el.Div().Gap(8).Child(el.Text("分支").TextSize(11).TextColor(theme.Muted), el.Div().Rounded(6).Bg(theme.Subtle).Child(w.BranchPicker.Render(cx)), el.Div().Row().Items(el.Center).Gap(6).Child(field("new-git-branch", "新分支名称，例如 feature/login", &w.NewBranch).Grow().W(el.Dp(0)).ReadOnly(w.Mutating), create.Render(cx))))
	}
	if w.Source != "local" {
		column.Child(el.Text("远程 Worker 的 Git 状态仅供查看；分支管理请在对应机器上操作。").TextSize(11).LineHeight(1.625).TextColor(theme.Muted))
	}
	changes := el.Div().Gap(4).Child(el.Text("变更").TextSize(11).TextColor(theme.Muted))
	for _, file := range s.Files {
		f := file
		label, fg := gitFileLabel(f)
		row := el.Div().Row().Items(el.Center).Gap(8).Py(2).Name(f.Path).Child(el.Text(label).TextSize(11).TextColor(fg).Medium(), el.Text(f.Path).Mono().TextSize(11).TextColor(theme.Muted).MaxLines(1).Grow().W(el.Dp(0)))
		if w.Source == "local" {
			row.Role("button").Focusable(true).CursorPointer().OnClick(func() {
				a.workbench.Tab = "review"
				a.reviewState(a.workspaceContext()).Selected = f.Path
				a.loadInspector()
			})
		}
		changes.Child(row)
	}
	column.Child(changes)
	if len(s.Files) == 0 {
		column.Child(el.Text("工作目录没有未提交的改动。").TextSize(11).TextColor(theme.Muted))
	}
	return column
}
func gitFileLabel(file domainworkspaces.GitFile) (string, color.NRGBA) {
	code := file.Index + file.Worktree
	switch {
	case strings.ContainsAny(code, "?A"):
		return "新增", theme.Success
	case strings.Contains(code, "D"):
		return "删除", theme.DangerText
	case strings.Contains(code, "R"):
		return "重命名", theme.Info
	default:
		return "修改", theme.Warning
	}
}

func (a *app) fileState(id string) *fileWorkspace {
	if a.workbench.Files == nil {
		a.workbench.Files = map[string]*fileWorkspace{}
	}
	state := a.workbench.Files[id]
	if state == nil {
		state = &fileWorkspace{Buffers: map[string]*fileBuffer{}, Tree: map[string][]desktopservice.WorkspaceFileEntry{}, Expanded: map[string]bool{}, DirectoryVersions: map[string]uint64{}, DirectoryLoading: map[string]bool{}}
		a.workbench.Files[id] = state
	}
	return state
}
func (a *app) loadDirectory(id, directory string) {
	state := a.fileState(id)
	if state.DirectoryVersions == nil {
		state.DirectoryVersions = map[string]uint64{}
		state.DirectoryLoading = map[string]bool{}
		state.Tree = map[string][]desktopservice.WorkspaceFileEntry{}
		state.Expanded = map[string]bool{}
	}
	state.DirectoryVersions[directory]++
	version := state.DirectoryVersions[directory]
	state.Error = ""
	if id == "" {
		return
	}
	state.DirectoryLoading[directory] = true
	go func() {
		ctx, cancel := context.WithTimeout(a.ctx, 10*time.Second)
		defer cancel()
		entries, err := a.backend.service.ListWorkspaceFiles(ctx, id, directory)
		core.Update(func() {
			if a.closed || state.DirectoryVersions[directory] != version {
				return
			}
			state.DirectoryLoading[directory] = false
			if err != nil {
				state.Error = err.Error()
				return
			}
			state.Tree[directory] = entries
			if directory == "" {
				state.Entries = entries
			}
		})
	}()
}
func (a *app) openWorkspaceFile(id, name string) {
	state := a.fileState(id)
	if existing := state.Buffers[name]; existing != nil {
		state.Active = name
		return
	}
	state.Version++
	version := state.Version
	state.Loading = true
	state.Error = ""
	go func() {
		ctx, cancel := context.WithTimeout(a.ctx, 10*time.Second)
		defer cancel()
		doc, err := a.backend.service.ReadWorkspaceFile(ctx, id, name)
		core.Update(func() {
			if a.closed || state.Version != version {
				return
			}
			state.Loading = false
			if err != nil {
				state.Error = err.Error()
				return
			}
			buffer := newFileBuffer(doc)
			buffer.Editor.OnChange(func(string) { a.queueAutoSave(id, name, buffer) })
			state.Buffers[name] = buffer
			state.Active = name
		})
	}()
}
func newFileBuffer(doc desktopservice.WorkspaceFileDocument) *fileBuffer {
	return &fileBuffer{Document: doc, Editor: kit.CodeEditor(doc.Content).Language(fileLanguage(doc.Path)).Name(doc.Path).Fill()}
}
func fileLanguage(name string) string {
	switch strings.ToLower(path.Ext(name)) {
	case ".go":
		return "go"
	case ".js", ".jsx":
		return "javascript"
	case ".ts", ".tsx":
		return "typescript"
	case ".json":
		return "json"
	case ".md":
		return "markdown"
	case ".py":
		return "python"
	case ".sh":
		return "bash"
	case ".css":
		return "css"
	case ".html":
		return "html"
	case ".yaml", ".yml":
		return "yaml"
	default:
		return "text"
	}
}
func (b *fileBuffer) dirty() bool { return b.Editor.Value() != b.Document.Content }
func (a *app) saveFile(id, name string) {
	state := a.fileState(id)
	b := state.Buffers[name]
	if b == nil || b.Saving || !b.dirty() {
		return
	}
	input := desktopservice.WriteWorkspaceFileInput{WorkspaceID: id, Path: name, Content: b.Editor.Value(), ExpectedHash: b.Document.Hash}
	b.Saving, b.Error = true, ""
	// Editing may continue while saving. Only the submitted revision becomes the
	// saved document; newer text remains dirty and will be included next time.
	go func() {
		doc, err := a.backend.service.WriteWorkspaceFile(a.ctx, input)
		core.Update(func() {
			if a.closed {
				return
			}
			b.Saving = false
			if err != nil {
				b.Error = err.Error()
				return
			}
			b.Document = doc
			if b.dirty() {
				a.queueAutoSave(id, name, b)
			}
			if a.workspaceContext() == id {
				a.loadContextGit()
			}
		})
	}()
}
func (a *app) closeFile(id, name string) {
	state := a.fileState(id)
	b := state.Buffers[name]
	if b == nil || b.Saving {
		return
	}
	close := func() {
		delete(state.Buffers, name)
		if state.Active == name {
			state.Active = ""
		}
	}
	if b.dirty() {
		a.workbench.Confirm.ConfirmDanger("放弃未保存的修改？", name, "放弃修改", close)
	} else {
		close()
	}
}
func (a *app) fileInspector(cx *el.Context) el.Element {
	id := a.workspaceContext()
	state := a.fileState(id)
	if id == "" {
		return inspectorNotice("选择项目后浏览文件。", false)
	}
	editor := el.Div().Grow().W(el.Dp(0)).HFull()
	tabs := el.Div().Row().ScrollX().Items(el.Center).Gap(4).Px(8).H(el.Dp(32))
	if len(state.Buffers) == 0 {
		tabs.Child(el.Text("未打开文件").TextSize(11).TextColor(theme.Muted))
	}
	for _, name := range sortedBufferPaths(state.Buffers) {
		p := name
		buffer := state.Buffers[p]
		label := pathBase(p)
		if buffer.dirty() {
			label += " •"
		}
		tabs.Child(button(label, func() { state.Active = p }, kit.ButtonGhost).Selected(state.Active == p).Compact(true).Size(28).Render(cx))
	}
	editor.Child(tabs)
	if b := state.Buffers[state.Active]; b != nil {
		name := state.Active
		save := button("保存", func() { a.saveFile(id, name) }, kit.ButtonGhost).Compact(true).Size(28)
		save.SetDisabled(!b.dirty() || b.Saving)
		editor.Child(el.Div().Row().Items(el.Center).Px(8).Gap(4).H(el.Dp(32)).Child(el.Text(name).Mono().TextSize(11).MaxLines(1).Grow().W(el.Dp(0)), save.Render(cx), iconAction(cx, "close-file", "关闭文件", "x", func() { a.closeFile(id, name) }, false)))
		if b.Error != "" {
			editor.Child(inspectorNotice(b.Error, true), button("重新读取文件", func() { a.reloadFile(id, name) }, kit.ButtonGhost).Compact(true).Render(cx))
		}
		editor.Child(b.Editor.Render(cx))
	} else {
		editor.Child(inspectorNotice("从右侧文件树选择一个 UTF-8 文本文件开始编辑。", false))
	}
	treeWidth := float32(140)
	if a.workbench.Maximized || a.workbench.DetachedWindow != nil {
		treeWidth = 200
	}
	tree := el.Div().W(el.Dp(treeWidth)).NoShrink().HFull().Child(el.Div().Row().Items(el.Center).Gap(4).Px(8).H(el.Dp(32)).Child(el.Text("工作区文件").TextSize(11).TextColor(theme.Muted).Grow().W(el.Dp(0)).MaxLines(1), iconAction(cx, "file-refresh", "刷新文件列表", "refresh-cw", func() { a.refreshFileTree(id) }, false)))
	listing := el.Div().ID("file-tree").Role("tree").Name("工作区文件").ScrollY().Grow().H(el.Dp(0)).Px(4)
	if state.Error != "" {
		listing.Child(inspectorNotice(state.Error, true))
	}
	var directory func(string, int)
	directory = func(parent string, depth int) {
		if depth > 32 {
			return
		}
		entries := state.Tree[parent]
		if parent == "" && entries == nil {
			entries = state.Entries
		}
		for _, item := range entries {
			entry := item
			icon := "file-text"
			if entry.Directory {
				icon = "folder"
			}
			row := el.Div().Row().Items(el.Center).Pl(float32(4 + depth*12)).Pr(4).H(el.Dp(28)).Gap(4).Rounded(5).ID("file-entry/" + entry.Path).Role("treeitem").Name(entry.Name).Focusable(true).CursorPointer().OnClick(func() {
				if entry.Directory {
					state.Expanded[entry.Path] = !state.Expanded[entry.Path]
					if state.Expanded[entry.Path] && state.Tree[entry.Path] == nil {
						a.loadDirectory(id, entry.Path)
					}
				} else {
					a.openWorkspaceFile(id, entry.Path)
				}
			}).Hover(func(s *el.Style) { s.Bg(theme.Highlight) }).FocusStyle(neutralFocus)
			if state.Active == entry.Path {
				row.Bg(theme.Highlight)
			}
			row.Child(lucide(cx, icon, 13, theme.Muted), el.Text(entry.Name).TextSize(11).MaxLines(1).Grow().W(el.Dp(0)))
			listing.Child(row)
			if entry.Directory && state.Expanded[entry.Path] {
				directory(entry.Path, depth+1)
			}
		}
		if state.DirectoryLoading[parent] {
			listing.Child(el.Text("读取中…").TextSize(10).TextColor(theme.Muted).Px(8).Py(4))
		}
	}
	directory("", 0)
	tree.Child(listing)
	return el.Div().Row().Grow().H(el.Dp(0)).Child(editor, el.Div().W(el.Dp(1)).NoShrink().HFull().Bg(seamColor), tree)
}
func (a *app) refreshFileTree(id string) {
	state := a.fileState(id)
	directories := make([]string, 0, len(state.Tree)+1)
	directories = append(directories, "")
	for name := range state.Tree {
		if name != "" {
			directories = append(directories, name)
		}
	}
	sort.Strings(directories)
	for _, directory := range directories {
		a.loadDirectory(id, directory)
	}
}
func (a *app) saveActiveWorkspaceFile() {
	if a.page == "skills" {
		a.saveActiveSkillFile()
		return
	}
	if a.workbench.Tab != "files" || !a.workbench.Visible && a.workbench.DetachedWindow == nil {
		return
	}
	id := a.workspaceContext()
	a.saveFile(id, a.fileState(id).Active)
}
func pathBase(name string) string { return path.Base(name) }
func (a *app) eventInspector(cx *el.Context) el.Element {
	if a.detail == nil {
		return inspectorNotice("选择执行后查看事件。", false)
	}
	column := el.Div().ScrollY().Grow().H(el.Dp(0)).P(14).Gap(8)
	for i := len(a.detail.Events) - 1; i >= 0; i-- {
		e := a.detail.Events[i]
		card := el.Div().Rounded(6).Bg(theme.Subtle).P(10).Gap(6).Child(el.Div().Row().Gap(8).Items(el.Center).Child(el.Text(fmt.Sprint(e.Seq)).TextSize(11).TextColor(theme.Muted), el.Text(e.Type).TextSize(12).Medium().Grow().W(el.Dp(0)).MaxLines(1), el.Text(eventClock(e.At)).TextSize(11).TextColor(theme.Muted)))
		if e.StepID != "" {
			card.Child(el.Text(e.StepID).Mono().TextSize(11).TextColor(theme.Muted))
		}
		if len(e.Payload) > 0 && string(e.Payload) != "{}" {
			var data any
			source := string(e.Payload)
			if json.Unmarshal([]byte(e.Payload), &data) == nil {
				raw, _ := json.MarshalIndent(data, "", "  ")
				source = string(raw)
			}
			card.Child(el.Text(source).Mono().TextSize(11).TextColor(theme.Muted))
		}
		column.Child(card)
	}
	if len(a.detail.Events) == 0 {
		column.Child(el.Text("暂无事件。").TextSize(12).TextColor(theme.Muted))
	}
	return column
}
func (a *app) statusInspector(cx *el.Context) el.Element {
	if a.detail == nil {
		return inspectorNotice("任务开始后，这里会显示执行状态、工作流进度和用量。", false)
	}
	d := a.detail
	column := el.Div().ScrollY().Grow().H(el.Dp(0)).P(14).Gap(12)
	column.Child(el.Div().Row().Items(el.Center).Gap(8).Rounded(8).Bg(theme.Subtle).Px(12).Py(10).Child(el.Text(runStatus(string(d.Run.Status))).TextSize(12).Medium(), el.Text(d.Run.CurrentStepID).TextSize(12).MaxLines(1).Grow().W(el.Dp(0))))
	var input, output, cached, reasoning int
	var duration int64
	for _, step := range d.StepRuns {
		input += step.InputTokens
		output += step.OutputTokens
		cached += step.CachedInputTokens
		reasoning += step.ReasoningOutputTokens
		duration += step.DurationMS
	}
	metric := func(label, value string) el.Element {
		return el.Div().Row().Items(el.Center).Gap(8).Child(el.Text(label).TextSize(11).TextColor(theme.Muted).Grow(), el.Text(value).TextSize(13).Medium())
	}
	column.Child(el.Div().Bg(theme.Subtle).Rounded(8).P(12).Gap(8).Child(metric("输入 Token", fmt.Sprint(input)), metric("输出 Token", fmt.Sprint(output)), metric("缓存读取", fmt.Sprint(cached)), metric("推理输出", fmt.Sprint(reasoning)), metric("用时", fmt.Sprintf("%.1f 秒", float64(duration)/1000)), metric("执行次数", fmt.Sprint(len(d.StepRuns)))))
	steps := el.Div().Bg(theme.Subtle).Rounded(8).P(12).Gap(8).Child(el.Text("工作流").TextSize(11).TextColor(theme.Muted))
	for i, step := range d.Workflow.Steps {
		status := "等待执行"
		for _, run := range d.StepRuns {
			if run.StepID == step.ID {
				status = runStatus(string(run.Status))
			}
		}
		fg := theme.Muted
		if step.ID == d.Run.CurrentStepID {
			fg = theme.PrimaryText
		}
		steps.Child(el.Div().Row().Items(el.Center).Gap(10).Child(el.Text(fmt.Sprint(i+1)).TextSize(11).Medium().TextColor(fg), el.Div().Gap(3).Grow().W(el.Dp(0)).Child(el.Text(step.Name).TextSize(12).Medium().TextColor(fg), el.Text(string(step.Runtime)+" · "+status).TextSize(11).TextColor(theme.Muted))))
	}
	column.Child(steps)
	sessions := el.Div().Bg(theme.Subtle).Rounded(8).P(12).Gap(8).Child(el.Text("恢复会话").TextSize(11).TextColor(theme.Muted))
	for _, step := range d.Workflow.Steps {
		if id := d.Run.Sessions[step.ID]; id != "" {
			sessions.Child(el.Text(step.Name+" · "+string(step.Runtime)).TextSize(11).TextColor(theme.Muted), el.Text(id).Mono().TextSize(11))
		}
	}
	if len(d.Run.Sessions) > 0 {
		column.Child(sessions)
	}
	if d.Run.LastError != "" {
		column.Child(inspectorNotice(d.Run.LastError, true))
	}
	return column
}

func sortedBufferPaths(buffers map[string]*fileBuffer) []string {
	names := make([]string, 0, len(buffers))
	for name := range buffers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func eventClock(value string) string {
	if at, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return at.Local().Format("15:04:05")
	}
	return value
}

type inspectorWindowView struct{ app *app }

func (v inspectorWindowView) Render(cx *el.Context) el.Element { return v.app.inspector(cx) }
func (a *app) detachInspector() {
	if a.workbench.DetachedWindow != nil {
		a.workbench.DetachedWindow.Raise()
		return
	}
	a.workbench.Visible = false
	a.workbench.DetachedWindow = window.Open(window.Options{Title: "OneCatch · 检查面板", Width: 960, Height: 800, MinWidth: 640, MinHeight: 500, Frameless: runtime.GOOS == "darwin", NativeTrafficLights: runtime.GOOS == "darwin", TrafficLightLayout: &window.TrafficLightLayout{Height: 52, Left: 20}, Content: el.Root(inspectorWindowView{a}), Shortcuts: map[string]func(){"mod+s": a.saveActiveWorkspaceFile}, OnClose: func() {
		a.workbench.DetachedWindow = nil
		if !a.closed {
			a.workbench.Visible = true
			a.loadInspector()
		}
	}})
	a.loadInspector()
}

func (a *app) queueAutoSave(id, name string, buffer *fileBuffer) {
	buffer.AutoSaveVersion++
	version := buffer.AutoSaveVersion
	time.AfterFunc(500*time.Millisecond, func() {
		core.Update(func() {
			if a.closed || version != buffer.AutoSaveVersion || a.fileState(id).Buffers[name] != buffer || buffer.Saving {
				return
			}
			a.saveFile(id, name)
		})
	})
}
func (a *app) reloadFile(id, name string) {
	state := a.fileState(id)
	buffer := state.Buffers[name]
	if buffer == nil || buffer.Saving {
		return
	}
	reload := func() { buffer.AutoSaveVersion++; delete(state.Buffers, name); a.openWorkspaceFile(id, name) }
	if buffer.dirty() {
		a.workbench.Confirm.ConfirmDanger("重新读取文件？", "未保存的修改将被磁盘上的内容替换。", "重新读取", reload)
	} else {
		reload()
	}
}
