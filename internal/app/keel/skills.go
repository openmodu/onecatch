package keeldesktop

import (
	"encoding/json"
	"fmt"
	"image/color"
	"regexp"
	"sort"
	"strings"

	"github.com/dyike/keel/gio/io/pointer"
	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/markdown"
	"github.com/dyike/keel/ui/theme"
	"github.com/openmodu/onecatch/internal/service/skillmanager"
	"github.com/yuin/goldmark/ast"
)

type skillLibraryService interface {
	ListManagedSkills() ([]skillmanager.Skill, error)
	GetSkill(string) (skillmanager.SkillDocument, error)
	CreateSkill(skillmanager.SaveSkillInput) (skillmanager.SkillDocument, error)
	DeleteSkill(string) error
	ListSkillFiles(string) ([]skillmanager.SkillFileEntry, error)
	ReadSkillFile(string) (skillmanager.SkillFileContent, error)
	WriteSkillFile(skillmanager.SaveSkillFileInput) (skillmanager.SkillFileContent, error)
	ScanSkillSyncTargets() ([]skillmanager.SyncTarget, error)
}

type skillWorkspaceState struct {
	api                                                                           skillLibraryService
	Create, Confirm                                                               *kit.DialogView
	Actions                                                                       *kit.MenuView
	TargetMenus                                                                   map[string]*kit.MenuView
	Debug                                                                         skillDebugState
	CreateName, CreateDescription, CreateError, Error, ListError, Busy, Pane, Tab string
	ListVersion, FileVersion, TreeVersion                                         uint64
	Width, RailWidth, TreeWidth, DragX                                            float32
	Visible, Maximized, TreeCollapsed                                             bool
	Tree                                                                          map[string][]skillmanager.SkillFileEntry
	Expanded, DirectoryLoading                                                    map[string]bool
	DirectoryVersions                                                             map[string]uint64
	Buffers                                                                       map[string]*skillFileBuffer
	Active                                                                        string
	Targets                                                                       []skillmanager.SyncTarget
	PreviewView                                                                   *skillMarkdownView
	Preview                                                                       *markdown.Doc
}
type skillFileBuffer struct {
	File         skillmanager.SkillFileContent
	Draft, Error string
	Saving       bool
	Version      uint64
}

func (a *app) skillService() skillLibraryService {
	if a.skills.api != nil {
		return a.skills.api
	}
	return a.backend.service
}
func (a *app) initSkillWorkspace() {
	s := &a.skills
	s.Width, s.RailWidth, s.TreeWidth, s.Tab = 438, 248, 152, "files"
	s.Tree, s.Expanded, s.DirectoryLoading, s.DirectoryVersions = map[string][]skillmanager.SkillFileEntry{}, map[string]bool{}, map[string]bool{}, map[string]uint64{}
	s.Buffers, s.Preview = map[string]*skillFileBuffer{}, newSkillPreview("")
	s.PreviewView = newSkillMarkdownView(s.Preview)
	s.Confirm = kit.Dialog("").Width(448)
	s.Create = kit.Dialog("新建 Skill").Width(448).CloseButton(true).BeforeCancel(func() bool { return s.Busy == "" }).Body(el.ViewFunc(a.skillCreateForm)).Footer(
		button("取消", func() {
			if s.Busy == "" {
				s.Create.SetValue(false)
			}
		}, kit.ButtonGhost),
		button("添加", a.createSkill, kit.ButtonPrimary),
	)
	a.initSkillDebugger()
	s.Actions = kit.Menu().Width(180).Trigger(el.ViewFunc(func(cx *el.Context) el.Element {
		return iconAction(cx, "skill-actions", "Skill 操作", "ellipsis", nil, false)
	})).ContentItem("编辑 SKILL.md", "", menuIcon("square-pen", "编辑 SKILL.md"), func() { a.openSkillInspector("files") }).Separator().ContentItem("删除", "", menuIcon("trash-2", "删除"), a.confirmDeleteSkill)
}

func (a *app) loadSkills() {
	if a.closed || a.skills.Busy != "" {
		return
	}
	a.skills.ListVersion++
	version, api := a.skills.ListVersion, a.skillService()
	a.library.SkillLoading = true
	go func() {
		items, err := api.ListManagedSkills()
		var targets []skillmanager.SyncTarget
		if err == nil {
			targets, err = api.ScanSkillSyncTargets()
		}
		core.Update(func() { a.applySkillList(version, items, targets, err) })
	}()
}
func (a *app) applySkillList(version uint64, items []skillmanager.Skill, targets []skillmanager.SyncTarget, err error) {
	if a.closed || version != a.skills.ListVersion {
		return
	}
	a.library.SkillLoading = false
	if err != nil {
		a.skills.ListError = err.Error()
		return
	}
	a.skills.ListError = ""
	a.library.Skills, a.skills.Targets = items, targets
	a.rebuildSkillTargetMenus()
	name := a.library.SkillName
	found := false
	for _, s := range items {
		found = found || s.Name == name
	}
	if !found {
		pane := a.skills.Pane
		name = ""
		if len(items) > 0 {
			name = items[0].Name
		}
		a.selectSkill(name)
		a.skills.Pane = pane
	} else {
		a.library.SkillVersion++
		version, api := a.library.SkillVersion, a.skillService()
		go func() {
			doc, err := api.GetSkill(name)
			core.Update(func() { a.applySkillDocument(version, name, doc, err) })
		}()
	}
	if a.skills.Visible {
		a.refreshSkillTree()
	}
}
func (a *app) selectSkill(name string) {
	if a.closed || a.skills.Busy != "" {
		return
	}
	a.library.SkillVersion++
	version := a.library.SkillVersion
	a.library.SkillName = name
	a.skills.Error = ""
	a.library.SkillDoc = nil
	a.skills.Pane = "skill"
	if name == "" {
		a.library.SkillDoc = nil
		a.skills.Preview.SetSource("")
		return
	}
	api := a.skillService()
	go func() {
		doc, err := api.GetSkill(name)
		core.Update(func() { a.applySkillDocument(version, name, doc, err) })
	}()
}
func (a *app) applySkillDocument(version uint64, name string, doc skillmanager.SkillDocument, err error) {
	if a.closed || version != a.library.SkillVersion || name != a.library.SkillName {
		return
	}
	if err != nil {
		a.skills.Error = err.Error()
		return
	}
	a.library.SkillDoc = &doc
	a.skills.Expanded[name] = true
	if a.skills.Visible {
		a.loadSkillDirectory(name)
	}
}

var skillNameInvalid = regexp.MustCompile(`[^a-z0-9-]`)
var skillNameHyphens = regexp.MustCompile(`-+`)

func normalizeSkillName(name string) string {
	return skillNameHyphens.ReplaceAllString(skillNameInvalid.ReplaceAllString(strings.ToLower(name), "-"), "-")
}
func newManagedSkillTemplate(name, description string) string {
	description = strings.Join(strings.Fields(description), " ")
	title := strings.Split(name, "-")
	for i, v := range title {
		if v != "" {
			title[i] = strings.ToUpper(v[:1]) + v[1:]
		}
	}
	return fmt.Sprintf("---\nname: %s\ndescription: %s\n---\n\n# %s\n\nDescribe when and how the agent should use this skill.\n", name, description, strings.Join(title, " "))
}
func (a *app) skillCreateForm(cx *el.Context) el.Element {
	s := &a.skills
	root := el.Div().Gap(16).Child(el.Text("创建标准目录和 SKILL.md；名称会同时作为 Modu 的 /skill 命令。").TextSize(13).LineHeight(1.625).TextColor(theme.Muted), workflowField("名称", field("new-skill-name", "release-notes", &s.CreateName).ReadOnly(s.Busy != "")), el.Text("只能用小写字母、数字和连字符；用途描述可以写中文。").TextSize(11).TextColor(theme.Muted), workflowField("用途描述", el.TextArea().ID("new-skill-description").Name("用途描述").Bind(&s.CreateDescription).Placeholder("说明何时应调用此 Skill").AutoGrow(3, 5).Bg(theme.Subtle).Border(0, color.NRGBA{}).Rounded(8).P(10).TextSize(13).ReadOnly(s.Busy != "")))
	if s.CreateError != "" {
		root.Child(el.Text(s.CreateError).TextSize(12).TextColor(theme.DangerText))
	}
	return root
}
func (a *app) showSkillCreate() {
	if a.skills.Busy == "" {
		a.skills.CreateError = ""
		a.skills.Create.SetValue(true)
	}
}
func (a *app) createSkill() {
	s := &a.skills
	if a.closed || s.Busy != "" {
		return
	}
	name, description := strings.TrimSpace(normalizeSkillName(s.CreateName)), strings.TrimSpace(s.CreateDescription)
	if !skillmanager.ValidSkillName(name) || description == "" {
		s.CreateError = "请填写有效的 Skill 名称和用途描述"
		return
	}
	s.CreateName, s.Busy, s.CreateError = name, "create", ""
	api := a.skillService()
	go func() {
		doc, err := api.CreateSkill(skillmanager.SaveSkillInput{Name: name, Content: newManagedSkillTemplate(name, description)})
		core.Update(func() {
			if a.closed {
				return
			}
			s.Busy = ""
			if err != nil {
				s.CreateError = err.Error()
				return
			}
			s.Create.SetValue(false)
			s.CreateName, s.CreateDescription = "", ""
			a.library.SkillVersion++
			a.library.SkillName, a.library.SkillDoc = doc.Name, &doc
			s.Pane = "skill"
			a.upsertSkillSummary(doc.Skill)
			a.loadSkills()
		})
	}()
}
func (a *app) upsertSkillSummary(item skillmanager.Skill) {
	for i, s := range a.library.Skills {
		if s.Name == item.Name {
			a.library.Skills[i] = item
			return
		}
	}
	a.library.Skills = append(a.library.Skills, item)
	sort.Slice(a.library.Skills, func(i, j int) bool { return a.library.Skills[i].Name < a.library.Skills[j].Name })
}
func (a *app) confirmDeleteSkill() {
	if a.skills.Busy != "" || a.library.SkillDoc == nil {
		return
	}
	name := a.library.SkillDoc.Name
	a.skills.Confirm.ConfirmDanger("删除 Skill“"+name+"”？", "将删除 ~/.onecatch/skills 中的整个 Skill 目录；已经同步到其他应用的副本不会自动删除。", "删除", func() { a.deleteSkill(name) })
}
func (a *app) deleteSkill(name string) {
	if a.closed || a.skills.Busy != "" || name == "" {
		return
	}
	s := &a.skills
	s.Busy = "delete"
	api := a.skillService()
	go func() {
		err := api.DeleteSkill(name)
		core.Update(func() {
			if a.closed {
				return
			}
			s.Busy = ""
			if err != nil {
				s.Error = err.Error()
				return
			}
			if a.library.SkillName == name {
				a.library.SkillVersion++
				a.library.SkillName = ""
				a.library.SkillDoc = nil
			}
			for path := range s.Buffers {
				if strings.HasPrefix(path, name+"/") {
					delete(s.Buffers, path)
				}
			}
			if strings.HasPrefix(s.Active, name+"/") {
				s.Active = ""
			}
			delete(s.Expanded, name)
			a.loadSkills()
		})
	}()
}

func parseManagedSkill(content string) (map[string]string, string) {
	text := strings.ReplaceAll(content, "\r\n", "\n")
	values := map[string]string{}
	if !strings.HasPrefix(text, "---\n") {
		return values, content
	}
	end := strings.Index(text[4:], "\n---")
	if end < 0 {
		return values, content
	}
	for _, line := range strings.Split(text[4:4+end], "\n") {
		if line == "" || line[0] == ' ' || line[0] == '\t' {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok || strings.TrimSpace(key) == "" {
			continue
		}
		value = strings.TrimSpace(value)
		if strings.HasPrefix(value, "\"") {
			var decoded string
			if json.Unmarshal([]byte(value), &decoded) == nil {
				value = decoded
			}
		}
		values[strings.TrimSpace(key)] = strings.Trim(value, "\"'")
	}
	body := text[4+end+4:]
	body = strings.TrimPrefix(body, "\n")
	return values, body
}
func skillSize(bytes int64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", max(0, bytes))
	}
	if bytes < 10240 {
		return fmt.Sprintf("%.1f KB", float64(bytes)/1024)
	}
	if bytes < 1048576 {
		return fmt.Sprintf("%.0f KB", float64(bytes)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(bytes)/1048576)
}
func (a *app) skillPreviewContent() string {
	if a.library.SkillDoc == nil {
		return ""
	}
	if b := a.skills.Buffers[a.library.SkillDoc.Name+"/SKILL.md"]; b != nil && b.Draft != b.File.Content {
		return b.Draft
	}
	return a.library.SkillDoc.Content
}

func (a *app) skillsPage(cx *el.Context) el.Element {
	s := &a.skills
	available, _ := cx.ViewportSize()
	if !a.sidebarCollapsed(cx) {
		available -= a.sidebarWidth(cx) + 1
	}
	if s.Visible {
		available -= min(s.Width, max(280, available-400)) + 1
	}
	railWidth := min(s.RailWidth, max(176, available-161))
	mainWidth := max(float32(0), available-railWidth-1)
	padding := float32(32)
	headerPadding := float32(24)
	if mainWidth < 320 {
		padding, headerPadding = 12, 12
	}
	bar := el.Div().Row().Items(el.Center).H(el.Dp(48)).Px(8).Gap(1).Child(iconAction(cx, "refresh-skills", "刷新", "refresh-cw", a.loadSkills, false), iconAction(cx, "create-skill", "新建 Skill", "plus", a.showSkillCreate, false), iconAction(cx, "sync-skills", "同步", "folder-sync", func() { s.Pane = "sync"; a.loadSkills() }, s.Pane == "sync"))
	rail := el.Div().Role("complementary").Name("Skill 库").W(el.Dp(railWidth)).NoShrink().HFull().Child(bar, el.Div().Px(8).Pb(8).Child(field("skill-search", "搜索 Skill", &a.library.SkillSearch).H(el.Dp(28)).MinH(el.Dp(28)).MaxH(el.Dp(28)).Py(0).Px(10).TextSize(12)))
	rows := el.Div().ScrollY().Grow().H(el.Dp(0)).Px(8).Pb(8).Gap(1)
	count := 0
	for _, entry := range a.library.Skills {
		skill := entry
		if !strings.Contains(strings.ToLower(skill.Name+" "+skill.Description), strings.ToLower(strings.TrimSpace(a.library.SkillSearch))) {
			continue
		}
		row := el.Div().ID("skill/" + skill.Name).Role("button").Name(skill.Name + " " + skill.Description).Focusable(true).CursorPointer().Rounded(8).Px(10).Py(8).Gap(2).Disabled(s.Busy != "").OnClick(func() { a.selectSkill(skill.Name) }).Hover(func(style *el.Style) { style.Bg(theme.Highlight) }).FocusStyle(neutralFocus)
		if s.Pane != "sync" && skill.Name == a.library.SkillName {
			row.Bg(theme.Highlight)
		}
		rows.Child(row.Child(el.Div().MinH(el.Dp(21)).Child(el.Text(skill.Name).TextSize(13).Medium().MaxLines(1)), el.Div().MinH(el.Dp(18)).Child(el.Text(skill.Description).TextSize(11).LineHeight(1.625).MaxLines(1).TextColor(theme.Muted))))
		count++
	}
	if count == 0 {
		label := "没有匹配的 Skill"
		if len(a.library.Skills) == 0 {
			label = "还没有 Skill"
		}
		rows.Child(el.Text(label).TextSize(11).TextColor(theme.Muted).Px(10).Py(12))
	}
	rail.Child(rows)
	detail := el.Div().Grow().W(el.Dp(0)).HFull().ScrollY().Px(padding).Pt(28).Pb(36).Items(el.Center)
	content := el.Div().WFull().MaxW(el.Dp(704)).Gap(12)
	if s.ListError != "" {
		content.Child(el.Text(s.ListError).TextSize(12).TextColor(theme.DangerText))
	}
	if s.Error != "" {
		content.Child(el.Text(s.Error).TextSize(12).TextColor(theme.DangerText))
	}
	if s.Pane == "sync" {
		content.Child(a.skillSyncPage(cx))
	} else if doc := a.library.SkillDoc; doc != nil {
		metadata, body := parseManagedSkill(a.skillPreviewContent())
		description := metadata["description"]
		if description == "" {
			description = doc.Description
		}
		header := el.Div().Px(headerPadding).Pt(20).Pb(16).Gap(12)
		actions := el.Div().Row().Items(el.Center).Gap(1).Child(iconAction(cx, "edit-skill", "编辑 SKILL.md", "square-pen", func() { a.openSkillInspector("files") }, false), iconAction(cx, "debug-skill", "Modu 调试", "play", func() { a.openSkillInspector("debug") }, false), s.Actions.Render(cx))
		title := el.Div().Gap(4).Child(el.Div().MinH(el.Dp(24)).Child(el.Text(doc.Name).TextSize(19).Bold().MaxLines(1)), el.Div().MinH(el.Dp(21)).Child(el.Text(description).TextSize(13).LineHeight(1.625).TextColor(theme.Muted)))
		if mainWidth-padding*2-headerPadding*2 < 220 {
			header.Child(actions, title)
		} else {
			header.Child(el.Div().Row().Items(el.Start).Gap(12).Child(title.Grow().W(el.Dp(0)), actions))
		}
		header.Child(el.Div().MinH(el.Dp(18)).Child(el.Text(doc.Path + " · " + skillSize(doc.SizeBytes) + " · " + doc.UpdatedAt.Local().Format("2006/1/2 15:04:05")).Mono().TextSize(11).TextColor(theme.Muted).MaxLines(1)))
		keys := []string{}
		for key := range metadata {
			if key != "name" && key != "description" {
				keys = append(keys, key)
			}
		}
		sort.Strings(keys)
		for _, key := range keys {
			header.Child(el.Div().Row().Gap(6).Child(el.Text(key).Mono().TextSize(10).TextColor(theme.Muted), el.Text(metadata[key]).TextSize(11).MaxLines(1)))
		}
		s.Preview.SetSource(body)
		prose := el.Div().Px(headerPadding).Py(20)
		if strings.TrimSpace(body) == "" {
			prose.Child(el.Text("这个 Skill 的正文还是空的，在右侧编辑 SKILL.md 补上说明。").TextSize(12).TextColor(theme.Muted))
		} else {
			prose.Child(s.PreviewView.Render(cx))
		}
		content.Child(el.Div().Rounded(16).Bg(theme.Surface).Child(header, el.Div().H(el.Dp(1)).Bg(seamColor), prose), el.Text("直接编辑 Markdown。保存时会校验名称、frontmatter 和 description。").TextSize(11).LineHeight(1.625).TextColor(theme.Muted).Px(4))
	} else {
		content.Items(el.Center).Gap(8).Child(el.Text("从第一个 Skill 开始").TextSize(17).Bold(), el.Text("Skill 保存在 ~/.onecatch/skills，可编辑、通过 Modu 单独调试，并同步到其他 Agent。").TextSize(13).LineHeight(1.625).TextColor(theme.Muted), button("新建 Skill", a.showSkillCreate, kit.ButtonPrimary).Size(28).Render(cx))
	}
	detail.Child(content)
	return el.Div().Row().Grow().H(el.Dp(0)).Child(rail, a.skillResizeSeam("skill-rail", "调整 Skill 库宽度", &s.RailWidth, 176, 420, false), detail)
}

func (a *app) skillResizeSeam(id, label string, width *float32, minimum, maximum float32, fromRight bool) el.Element {
	start := &a.skills.DragX
	return el.Div().W(el.Dp(1)).NoShrink().HFull().Bg(seamColor).Child(el.Div().Absolute().Left(-4).Top(0).Bottom(0).W(el.Dp(9)).ID(id).Role("separator").Name(label).Focusable(true).Cursor(pointer.CursorColResize).OnDrag(func(e el.DragEvent) {
		if e.Kind == el.DragStart {
			*start = e.X
		}
		if e.Kind == el.DragMove {
			delta := e.X - *start
			if fromRight {
				delta = -delta
			}
			*width = max(minimum, min(maximum, *width+delta))
		}
	}).OnKey(func(e el.KeyEvent) bool {
		if e.State != el.KeyPress {
			return false
		}
		delta := float32(12)
		if fromRight {
			delta = -delta
		}
		switch e.Name {
		case "←":
			*width = max(minimum, min(maximum, *width-delta))
			return true
		case "→":
			*width = max(minimum, min(maximum, *width+delta))
			return true
		}
		return false
	}))
}

func skillDisabledButton(b *kit.ButtonView, disabled bool) *kit.ButtonView {
	b.SetDisabled(disabled)
	return b
}

// The source renders headings at the prose size inside its compact Skill card.
// Register a per-document renderer rather than changing Markdown across chat.
func newSkillPreview(source string) *markdown.Doc {
	return markdown.New(source).Plugins(markdown.Plugin{Blocks: map[ast.NodeKind]func(ast.Node, []byte) el.View{
		ast.KindHeading: func(node ast.Node, source []byte) el.View {
			text := string(node.Text(source))
			return el.ViewFunc(func(cx *el.Context) el.Element {
				return el.Div().Role("heading").Name(text).MinH(el.Dp(24)).Child(el.Text(text).TextSize(float32(theme.BodySize)).Bold().LineHeight(1.75))
			})
		},
		ast.KindParagraph: func(node ast.Node, source []byte) el.View {
			body := markdown.New(string(node.Lines().Value(source)))
			return el.ViewFunc(func(cx *el.Context) el.Element {
				return el.Div().MinH(el.Dp(24)).Child(body.Render(cx))
			})
		},
	}})
}

type skillMarkdownView struct{ root *el.RootWidget }

func newSkillMarkdownView(doc *markdown.Doc) *skillMarkdownView {
	return &skillMarkdownView{root: el.Embed(el.ViewFunc(func(cx *el.Context) el.Element {
		root := doc.Render(cx)
		if column, ok := root.(*el.DivEl); ok {
			column.Gap(16)
		}
		return root
	}))}
}
func (v *skillMarkdownView) Render(cx *el.Context) el.Element {
	return el.Widget(core.Func(func(gtx core.C) core.D {
		gtx.Metric.PxPerSp *= 13.5 / float32(theme.BodySize)
		return v.root.Layout(gtx)
	}))
}
