package keeldesktop

import (
	"context"
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
	Events       eventNavigator
	StatusCopies map[string]*kit.CopyButtonView

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
	StepAction                        *stepActionState
	StepFocus                         string
}
type fileWorkspace struct {
	TreeRevision, RowsRevision uint64
	Rows                       []fileTreeRow

	NoWrap bool

	HideTree, SearchFocus bool
	Query                 string
	SearchVersion         uint64
	SearchCancel          context.CancelFunc
	SearchLoading         bool
	SearchResult          desktopservice.WorkspaceFileSearch
	SearchIndex           int
	Tabs                  []string
	Menu                  *kit.MenuView
	OpenLine              int

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
		if a.workbench.Tab == "files" {
			s := a.fileState(a.workspaceContext())
			if len(s.Buffers) == 0 {
				s.SearchFocus = true
			}
		}
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

var inspectorTabs = []struct{ ID, Label, Icon string }{{"status", "Session 状态", "activity"}, {"files", "文件", "file-code-2"}, {"git", "Git", "git-branch"}, {"review", "审阅", "file-diff"}, {"events", "事件时间轴", "clock"}}

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
	state.TreeRevision++
	go func() {
		ctx, cancel := context.WithTimeout(a.ctx, 10*time.Second)
		defer cancel()
		entries, err := a.backend.service.ListWorkspaceFiles(ctx, id, directory)
		core.Update(func() {
			if a.closed || state.DirectoryVersions[directory] != version {
				return
			}
			state.DirectoryLoading[directory] = false
			state.TreeRevision++
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
		state.Version++
		state.Loading = false
		state.Active = name
		if state.OpenLine > 0 {
			existing.Editor.SetCursor(state.OpenLine-1, 0)
			state.OpenLine = 0
		}
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
			state.Tabs = append(state.Tabs, name)
			state.Active = name
			if state.OpenLine > 0 {
				buffer.Editor.SetCursor(state.OpenLine-1, 0)
				state.OpenLine = 0
			}
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
		paths := fileTabPaths(state)
		delete(state.Buffers, name)
		state.Tabs = nil
		at := 0
		for i, p := range paths {
			if p == name {
				at = i
			} else {
				state.Tabs = append(state.Tabs, p)
			}
		}
		if state.Active == name {
			state.Active = ""
			if len(state.Tabs) > 0 {
				state.Active = state.Tabs[min(at, len(state.Tabs)-1)]
			}
		}
	}
	if b.dirty() {
		a.workbench.Confirm.ConfirmDanger("放弃未保存的修改？", name, "放弃修改", close)
	} else {
		close()
	}
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

func (a *app) statusInspector(cx *el.Context) el.Element {
	if a.detail == nil {
		return inspectorNotice("任务开始后，这里会显示执行状态、工作流进度和用量。", false)
	}
	d := a.detail
	column := el.Div().ScrollY().Grow().H(el.Dp(0)).P(14).Gap(12)
	column.Child(kit.StatusMarker(runStatus(string(d.Run.Status))).ID("session-run-status").Render(cx))
	var input, output, cached, created, reasoning int
	duration := sessionDuration(d, time.Now())
	for _, step := range d.StepRuns {
		input += normalizedSessionInput(step.InputTokens, step.CachedInputTokens, step.CacheCreationInputTokens)
		created += step.CacheCreationInputTokens
		output += step.OutputTokens
		cached += step.CachedInputTokens
		reasoning += step.ReasoningOutputTokens

	}
	metric := func(label, value string) el.Element {
		return el.Div().Row().Items(el.Center).Gap(8).Child(el.Text(label).TextSize(11).TextColor(theme.Muted).Grow(), el.Text(value).TextSize(12).Medium())
	}
	column.Child(el.Div().Row().Items(el.Center).Child(el.Text("累计 Token").TextSize(11).TextColor(theme.Muted), el.Div().Grow(), el.Text(sessionNumber(input+output)).TextSize(15).Medium()))
	inputCard := el.Div().Bg(theme.Subtle).Rounded(10).P(12).Gap(8).Child(el.Text("输入总 Token").TextSize(12).TextColor(theme.Muted), el.Text(sessionNumber(input)).TextSize(26).Medium())

	inputCard.Child(el.Div().Bg(theme.Surface).Rounded(8).P(12).Gap(10).Child(metric("缓存命中率", sessionCacheRate(input, cached)), metric("缓存读取", sessionNumber(cached)), metric("缓存写入", sessionNumber(created))))
	column.Child(inputCard, el.Div().Bg(theme.Subtle).Rounded(10).P(12).Gap(8).Child(el.Text("输出总 Token").TextSize(12).TextColor(theme.Muted), el.Text(sessionNumber(output)).TextSize(26).Medium(), metric("推理输出", sessionNumber(reasoning))))
	column.Child(el.Div().Row().Bg(theme.Subtle).Rounded(10).P(16).Gap(16).Child(el.Div().Grow().W(el.Dp(0)).Gap(8).Child(el.Text("已记录耗时").TextSize(11).TextColor(theme.Muted), el.Text(sessionElapsed(duration)).TextSize(16).Medium()), el.Div().Grow().W(el.Dp(0)).Gap(8).Child(el.Text("执行次数").TextSize(11).TextColor(theme.Muted), el.Text(fmt.Sprint(len(d.StepRuns))).TextSize(16).Medium())))

	if len(d.Workflow.Steps) > 1 {
		column.Child(a.workflowProgress(cx), a.sessionRecovery(cx))
		if d.Run.LastError != "" {
			column.Child(inspectorNotice(d.Run.LastError, true))
		}
		return column
	}
	steps := kit.Stepper().Vertical().Size(20)
	entries := make([]kit.StepperItem, 0, len(d.Workflow.Steps))
	current := 0
	for i, step := range d.Workflow.Steps {
		status := "等待执行"
		for _, run := range d.StepRuns {
			if run.StepID == step.ID {
				status = runStatus(string(run.Status))
				if string(run.Status) == "succeeded" {
					status = "成功"
				}
			}
		}
		if step.ID == d.Run.CurrentStepID {
			current = i
		}
		name, subtitle := step.Name, string(step.Runtime)+" · "+status
		entries = append(entries, kit.StepperItem{Label: name, Content: el.ViewFunc(func(*el.Context) el.Element {
			return el.Div().Gap(3).Child(el.Text(name).TextSize(12).Medium(), el.Text(subtitle).TextSize(10).TextColor(theme.Muted))
		})})
	}
	steps.SetEntries(entries...)
	if string(d.Run.Status) == "completed" {
		current = len(entries)
	}
	steps.SetValue(current)
	column.Child(el.Div().Gap(12).Py(8).Child(el.Text("工作流").TextSize(12).Medium(), steps.Render(cx)))
	column.Child(a.sessionRecovery(cx))
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

func (v inspectorWindowView) Render(cx *el.Context) el.Element {
	if usesDesktopHeader(runtime.GOOS) {
		return el.Div().WFull().HFull().Child(el.Div().Row().Items(el.Center).H(el.Dp(44)).Child(dragArea(cx, el.Div().Grow().HFull().Px(12).Justify(el.Center).Child(el.Text("OneCatch · 检查面板").TextSize(13))), desktopWindowButtons(cx)), el.Div().Grow().H(el.Dp(0)).Child(v.app.inspector(cx)))
	}
	return v.app.inspector(cx)
}
func (a *app) detachInspector() {
	if a.workbench.DetachedWindow != nil {
		a.workbench.DetachedWindow.Raise()
		return
	}
	a.workbench.Visible = false
	a.workbench.DetachedWindow = window.Open(platformWindowOptions(runtime.GOOS, window.Options{Title: "OneCatch · 检查面板", Width: 960, Height: 800, MinWidth: 640, MinHeight: 500, Frameless: runtime.GOOS == "darwin", NativeTrafficLights: runtime.GOOS == "darwin", TrafficLightLayout: &window.TrafficLightLayout{Height: 52, Left: 20}, Content: el.Root(inspectorWindowView{a}), Shortcuts: map[string]func(){"mod+s": a.saveActiveWorkspaceFile}, OnClose: func() {
		a.workbench.DetachedWindow = nil
		if !a.closed {
			a.workbench.Visible = true
			a.loadInspector()
		}
	}}))
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
