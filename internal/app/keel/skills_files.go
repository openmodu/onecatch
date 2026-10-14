package keeldesktop

import (
	"image/color"
	"path"
	"runtime"
	"strings"

	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/theme"
	uiassets "github.com/openmodu/onecatch/internal/app/keel/assets"
	"github.com/openmodu/onecatch/internal/service/skillmanager"
)

func (a *app) openSkillInspector(tab string) {
	s := &a.skills
	s.Visible = true
	s.Tab = tab
	if tab == "debug" {
		a.loadSkillDebugHistory()
		return
	}
	a.refreshSkillTree()
	if name := a.library.SkillName; name != "" {
		s.Expanded[name] = true
		a.openSkillFile(name + "/SKILL.md")
	}
}
func (a *app) refreshSkillTree() {
	s := &a.skills
	s.TreeVersion++
	s.Tree = map[string][]skillmanager.SkillFileEntry{}
	s.DirectoryLoading = map[string]bool{}
	s.DirectoryVersions = map[string]uint64{}
	a.loadSkillDirectory("")
	for dir, expanded := range s.Expanded {
		if expanded {
			a.loadSkillDirectory(dir)
		}
	}
}
func (a *app) loadSkillDirectory(dir string) {
	if a.closed {
		return
	}
	s := &a.skills
	s.DirectoryVersions[dir]++
	version, generation := s.DirectoryVersions[dir], s.TreeVersion
	s.DirectoryLoading[dir] = true
	api := a.skillService()
	go func() {
		entries, err := api.ListSkillFiles(dir)
		core.Update(func() {
			if a.closed || generation != s.TreeVersion || version != s.DirectoryVersions[dir] {
				return
			}
			s.DirectoryLoading[dir] = false
			if err != nil {
				s.Error = err.Error()
				return
			}
			s.Tree[dir] = entries
		})
	}()
}
func (a *app) openSkillFile(name string) {
	if a.closed {
		return
	}
	s := &a.skills
	s.FileVersion++
	s.Active = name
	if s.Buffers[name] != nil {
		return
	}
	version, api := s.FileVersion, a.skillService()
	go func() {
		file, err := api.ReadSkillFile(name)
		core.Update(func() {
			if a.closed || version != s.FileVersion || s.Active != name {
				return
			}
			if err != nil {
				s.Error = err.Error()
				s.Active = ""
				return
			}
			s.Buffers[name] = &skillFileBuffer{File: file, Draft: file.Content}
		})
	}()
}
func (a *app) saveActiveSkillFile() { a.saveSkillFile(a.skills.Active) }
func (a *app) saveSkillFile(name string) {
	s := &a.skills
	b := s.Buffers[name]
	if a.closed || b == nil || b.Saving || s.Busy != "" || b.Draft == b.File.Content {
		return
	}
	b.Saving = true
	b.Error = ""
	b.Version++
	version, draft, api := b.Version, b.Draft, a.skillService()
	go func() {
		file, err := api.WriteSkillFile(skillmanager.SaveSkillFileInput{Path: name, Content: draft})
		var doc *skillmanager.SkillDocument
		if err == nil && path.Base(name) == "SKILL.md" {
			value, getErr := api.GetSkill(strings.Split(name, "/")[0])
			if getErr == nil {
				doc = &value
			}
		}
		core.Update(func() { a.applySkillFileSave(name, b, version, file, doc, err) })
	}()
}
func (a *app) applySkillFileSave(name string, b *skillFileBuffer, version uint64, file skillmanager.SkillFileContent, doc *skillmanager.SkillDocument, err error) {
	if a.closed || a.skills.Buffers[name] != b || version != b.Version {
		return
	}
	b.Saving = false
	if err != nil {
		b.Error = err.Error()
		return
	}
	// The saved snapshot may be older than a draft typed during the write.
	b.File = file
	b.Error = ""
	if doc != nil {
		a.upsertSkillSummary(doc.Skill)
		if doc.Name == a.library.SkillName {
			a.library.SkillDoc = doc
		}
	}
	a.loadSkills()
}
func (a *app) closeSkillFile() {
	s := &a.skills
	name := s.Active
	b := s.Buffers[name]
	if b != nil && b.Saving {
		return
	}
	closeFile := func() {
		if s.Active == name {
			s.FileVersion++
			s.Active = ""
		}
		delete(s.Buffers, name)
	}
	if b != nil && b.Draft != b.File.Content {
		s.Confirm.ConfirmDanger("放弃未保存的修改？", name, "放弃修改", closeFile)
		return
	}
	closeFile()
}
func (a *app) skillInspector(cx *el.Context) el.Element {
	s := &a.skills
	tabs := el.Div().Row().Items(el.Center).Gap(2).Grow().W(el.Dp(0)).P(3).Rounded(8).Bg(mixColor(theme.Subtle, theme.Bg, .72)).Child(iconAction(cx, "skill-tab-files", "文件", "file-code-2", func() { s.Tab = "files"; a.refreshSkillTree() }, s.Tab == "files"), iconAction(cx, "skill-tab-debug", "Modu 调试", "play", func() { s.Tab = "debug"; a.loadSkillDebugHistory() }, s.Tab == "debug"))
	maxIcon, maxLabel := "maximize-2", "放大检查面板"
	if s.Maximized {
		maxIcon, maxLabel = "minimize-2", "还原检查面板"
	}
	title := "Skills 文件"
	if s.Tab == "debug" {
		title = "Modu 调试"
	}
	bar := el.Div().Row().Items(el.Center).H(el.Dp(44)).Px(8).Gap(6).Child(tabs, el.Text(title).TextSize(12).Medium().MaxLines(1), iconAction(cx, "maximize-skill-inspector", maxLabel, maxIcon, func() { s.Maximized = !s.Maximized }, false), iconAction(cx, "hide-skill-inspector", "收起检查面板", "panel-right-close", func() { s.Visible = false; s.Maximized = false }, false))
	if s.Maximized && a.sidebarCollapsed(cx) && runtime.GOOS == "darwin" {
		bar.Pl(94)
	}
	var body el.Element
	if s.Tab == "debug" {
		body = a.skillDebugInspector(cx)
	} else {
		body = a.skillFilesInspector(cx)
	}
	width, _ := cx.ViewportSize()
	if !a.sidebarCollapsed(cx) {
		width -= a.sidebarWidth(cx) + 1
	}
	pane := el.Div().ID("skill-inspector").Role("complementary").Name("Skills 检查面板").HFull().NoShrink().Child(bar, body)
	if s.Maximized {
		pane.W(el.Dp(width - 1))
	} else {
		pane.W(el.Dp(min(s.Width, max(280, width-400))))
	}
	return pane
}
func (a *app) skillFilesInspector(cx *el.Context) el.Element {
	s := &a.skills
	b := s.Buffers[s.Active]
	editor := el.Div().Grow().W(el.Dp(0)).HFull().Bg(mixColor(theme.Subtle, theme.Bg, .6))
	if b == nil {
		editor.Child(el.Div().Grow().H(el.Dp(0)).ScrollY().P(16).Child(el.Text("从右侧文件树选择一个文件。支持编辑 SKILL.md、references 和 scripts。").TextSize(12).LineHeight(1.625).TextColor(theme.Muted)))
	} else {
		dirty := b.Draft != b.File.Content
		heading := el.Div().Row().Items(el.Center).H(el.Dp(40)).Px(8).Gap(4).Child(el.Text(path.Base(s.Active)).Mono().TextSize(11).MaxLines(1).Grow().W(el.Dp(0)))
		if dirty {
			heading.Child(el.Text("未保存").TextSize(10).TextColor(theme.Muted))
		}
		heading.Child(iconAction(cx, "revert-skill-file", "还原", "rotate-ccw", func() {
			if !b.Saving {
				b.Draft = b.File.Content
				b.Error = ""
			}
		}, false), skillDisabledButton(button("保存", a.saveActiveSkillFile, kit.ButtonGhost).ID("save-skill-file").Size(28), !dirty || b.Saving || s.Busy != "").Loading(b.Saving).Render(cx), iconAction(cx, "close-skill-file", "关闭文件", "x", a.closeSkillFile, false))
		editor.Child(heading)
		if b.Error != "" {
			editor.Child(inspectorNotice(b.Error, true))
		}
		editor.Child(el.TextArea().ID("skill-file-editor").Name(s.Active).Bind(&b.Draft).Mono().TextSize(11.5).LineHeight(1.7).Border(0, color.NRGBA{}).FocusStyle(func(style *el.Style) { style.BorderColor(color.NRGBA{}) }).Bg(mixColor(theme.Subtle, theme.Bg, .6)).Px(12).Py(6).Grow().H(el.Dp(0)), el.Text("输入时中间的预览会同步更新 · "+skillSize(int64(len(b.Draft)))).TextSize(10).TextColor(theme.Muted).Px(12).Py(6))
	}
	listing := el.Div().NoShrink().HFull()
	header := el.Div().Row().Items(el.Center).H(el.Dp(44)).Px(8).Gap(4).Child(uiassets.Lucide(cx, "folder", 13, theme.Muted), el.Text("~/.onecatch/skills").Mono().TextSize(10).TextColor(theme.Muted).MaxLines(1).Grow().W(el.Dp(0)), iconAction(cx, "refresh-skill-files", "刷新文件", "refresh-cw", a.refreshSkillTree, false), iconAction(cx, "collapse-skill-tree", "切换文件树", "panel-right-close", func() { s.TreeCollapsed = !s.TreeCollapsed }, s.TreeCollapsed))
	if s.TreeCollapsed {
		listing.W(el.Dp(36)).Child(iconAction(cx, "expand-skill-tree", "展开文件树", "panel-right-open", func() { s.TreeCollapsed = false }, false))
	} else {
		tree := el.Div().ScrollY().Grow().H(el.Dp(0)).Px(4).Pb(8)
		a.appendSkillTree(cx, tree, "", 0)
		listing.W(el.Dp(s.TreeWidth)).Child(tree)
	}
	root := el.Div().Row().Grow().H(el.Dp(0)).Child(editor)
	if !s.TreeCollapsed {
		root.Child(a.skillResizeSeam("skill-tree-resize", "调整 Skill 文件树宽度", &s.TreeWidth, 110, 320, true))
	}
	return el.Div().Grow().H(el.Dp(0)).Child(header, root.Child(listing))
}
func (a *app) appendSkillTree(cx *el.Context, root *el.DivEl, dir string, depth int) {
	if depth > 32 {
		return
	}
	s := &a.skills
	for _, value := range s.Tree[dir] {
		entry := value
		expanded := s.Expanded[entry.Path]
		icon := "file-text"
		if entry.Directory {
			icon = "folder"
			if expanded {
				icon = "folder-open"
			}
		}
		row := el.Div().ID("skill-file/" + entry.Path).Role("button").Name(entry.Path).Focusable(true).CursorPointer().Row().Items(el.Center).H(el.Dp(28)).Pl(float32(6 + depth*12)).Pr(4).Gap(4).Rounded(6).Hover(func(style *el.Style) { style.Bg(theme.Highlight) }).FocusStyle(neutralFocus).OnClick(func() {
			if entry.Directory {
				s.Expanded[entry.Path] = !s.Expanded[entry.Path]
				if s.Expanded[entry.Path] {
					a.loadSkillDirectory(entry.Path)
				}
			} else {
				a.openSkillFile(entry.Path)
			}
		})
		if s.Active == entry.Path {
			row.Bg(theme.Highlight)
		}
		if entry.Directory {
			chevron := "chevron-right"
			if expanded {
				chevron = "chevron-down"
			}
			row.Child(uiassets.Lucide(cx, chevron, 12, theme.Muted))
		}
		row.Child(uiassets.Lucide(cx, icon, 13, theme.Muted), el.Text(entry.Name).TextSize(11).MaxLines(1))
		root.Child(row)
		if entry.Directory && expanded {
			a.appendSkillTree(cx, root, entry.Path, depth+1)
		}
	}
	if s.DirectoryLoading[dir] {
		root.Child(el.Text("加载中…").TextSize(10).TextColor(theme.Muted).Pl(float32(8 + depth*12)).Py(4))
	}
}
