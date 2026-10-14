package keeldesktop

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/markdown"
	"github.com/dyike/keel/ui/theme"
	uiassets "github.com/openmodu/onecatch/internal/app/keel/assets"
	desktopservice "github.com/openmodu/onecatch/internal/service/desktop"
)

type skillDebugService interface {
	DebugSkill(context.Context, desktopservice.SkillDebugInput) (desktopservice.SkillDebugResult, error)
	StopSkillDebug(string)
	SetSkillDebugEmitter(func(desktopservice.SkillDebugFrame))
	ListSkillDebugRuns(string) ([]desktopservice.SkillDebugRecord, error)
	ClearSkillDebugRuns(string) error
}
type skillDebugState struct {
	api                                 skillDebugService
	Name, Prompt, RunID, Viewing, Error string
	PromptFocused                       bool
	Running                             bool
	HistoryVersion                      uint64
	Result                              *desktopservice.SkillDebugResult
	Events                              []desktopservice.SkillDebugEvent
	History                             []desktopservice.SkillDebugRecord
	HistoryMenu                         *kit.MenuView
	Views                               map[string]*skillMarkdownView
	Expanded                            map[int]bool
	Docs                                map[string]*markdown.Doc
}

func (a *app) skillDebugger() skillDebugService {
	if a.skills.Debug.api != nil {
		return a.skills.Debug.api
	}
	return a.backend.service
}
func (a *app) initSkillDebugger() {
	d := &a.skills.Debug
	d.Docs = map[string]*markdown.Doc{}
	d.Views = map[string]*skillMarkdownView{}
	d.Expanded = map[int]bool{}
	a.skillDebugger().SetSkillDebugEmitter(func(frame desktopservice.SkillDebugFrame) { core.Update(func() { a.applySkillDebugFrame(frame) }) })
}
func (a *app) syncSkillDebugger() {
	d := &a.skills.Debug
	name := a.library.SkillName
	if name == d.Name {
		return
	}
	// An in-flight transcript continues to belong to its originating skill.
	if d.Running {
		return
	}
	d.Name = name
	d.Result = nil
	d.Events = nil
	d.Viewing = ""
	d.Error = ""
	d.Docs = map[string]*markdown.Doc{}
	d.Views = map[string]*skillMarkdownView{}
	d.Expanded = map[int]bool{}
	a.loadSkillDebugHistory()
}
func (a *app) loadSkillDebugHistory() {
	d := &a.skills.Debug
	if a.closed {
		return
	}
	if !d.Running && d.Name != a.library.SkillName {
		d.Name = a.library.SkillName
		d.Result = nil
		d.Events = nil
		d.Viewing = ""
		d.Docs = map[string]*markdown.Doc{}
		d.Views = map[string]*skillMarkdownView{}
		d.Expanded = map[int]bool{}
	}
	d.HistoryVersion++
	version, name, api := d.HistoryVersion, d.Name, a.skillDebugger()
	if name == "" {
		d.History = nil
		a.rebuildSkillDebugHistoryMenu()
		return
	}
	go func() {
		records, err := api.ListSkillDebugRuns(name)
		core.Update(func() {
			if a.closed || version != d.HistoryVersion || name != d.Name {
				return
			}
			if err == nil {
				d.History = records
			} else {
				d.History = nil
			}
			a.rebuildSkillDebugHistoryMenu()
		})
	}()
}
func (a *app) applySkillDebugFrame(frame desktopservice.SkillDebugFrame) {
	d := &a.skills.Debug
	if a.closed || !d.Running || d.RunID != frame.RunID || frame.Index < 0 || frame.Index >= 200 {
		return
	}
	for len(d.Events) <= frame.Index {
		d.Events = append(d.Events, desktopservice.SkillDebugEvent{})
	}
	d.Events[frame.Index] = frame.Event
}
func (a *app) runSkillDebug() {
	d := &a.skills.Debug
	if a.closed || d.Running || a.library.SkillName == "" || strings.TrimSpace(d.Prompt) == "" {
		return
	}
	d.Name = a.library.SkillName
	d.RunID = fmt.Sprintf("keel-%d", time.Now().UnixNano())
	d.Running = true
	d.Error = ""
	d.Result = nil
	d.Events = nil
	d.Viewing = ""
	d.Docs = map[string]*markdown.Doc{}
	d.Views = map[string]*skillMarkdownView{}
	d.Expanded = map[int]bool{}
	input := desktopservice.SkillDebugInput{Name: d.Name, Prompt: strings.TrimSpace(d.Prompt), RunID: d.RunID}
	api := a.skillDebugger()
	go func() {
		result, err := api.DebugSkill(a.ctx, input)
		core.Update(func() { a.finishSkillDebug(input.RunID, result, err) })
	}()
}
func (a *app) finishSkillDebug(id string, result desktopservice.SkillDebugResult, err error) {
	d := &a.skills.Debug
	if a.closed || d.RunID != id {
		return
	}
	d.Running = false
	d.RunID = ""
	if err != nil {
		d.Error = err.Error()
	} else {
		d.Result = &result
		d.Events = result.Events
	}
	a.loadSkillDebugHistory()
}
func (a *app) stopSkillDebug() {
	if d := &a.skills.Debug; d.Running && d.RunID != "" {
		api, id := a.skillDebugger(), d.RunID
		go api.StopSkillDebug(id)
	}
}
func (a *app) rebuildSkillDebugHistoryMenu() {
	d := &a.skills.Debug
	menu := kit.Menu().Width(300).Trigger(el.ViewFunc(func(cx *el.Context) el.Element {
		return iconAction(cx, "skill-debug-history", "历史记录", "history", nil, false)
	}))
	for _, entry := range d.History {
		record := entry
		menu.ContentItem(record.StartedAt.Local().Format("1/2 15:04")+" · "+record.Prompt, "", el.ViewFunc(func(cx *el.Context) el.Element { return el.Text(record.Prompt).TextSize(12).MaxLines(1) }), func() {
			if d.Running {
				return
			}
			d.Viewing = record.RunID
			d.Prompt = record.Prompt
			d.Result = &record.Result
			d.Events = record.Result.Events
			d.Error = ""
			d.Docs = map[string]*markdown.Doc{}
			d.Views = map[string]*skillMarkdownView{}
			d.Expanded = map[int]bool{}
		})
	}
	if len(d.History) == 0 {
		menu.Label("还没有调试记录")
	} else {
		menu.Separator().ContentItem("清空记录", "", menuIcon("trash-2", "清空记录"), func() {
			if a.closed || d.Running {
				return
			}
			name, api := d.Name, a.skillDebugger()
			go func() {
				err := api.ClearSkillDebugRuns(name)
				core.Update(func() {
					if a.closed || d.Name != name {
						return
					}
					if err != nil {
						d.Error = err.Error()
						return
					}
					d.History = nil
					d.Viewing = ""
					a.rebuildSkillDebugHistoryMenu()
				})
			}()
		})
	}
	d.HistoryMenu = menu
}
func (a *app) skillDebugMarkdown(cx *el.Context, key, source string) el.Element {
	d := &a.skills.Debug
	doc := d.Docs[key]
	if doc == nil {
		doc = markdown.New(source)
		d.Docs[key] = doc
		d.Views[key] = newSkillMarkdownView(doc)
	} else {
		doc.SetSource(source)
	}
	return d.Views[key].Render(cx)
}
func (a *app) skillDebugInspector(cx *el.Context) el.Element {
	a.syncSkillDebugger()
	d := &a.skills.Debug
	d.PromptFocused = cx.Focused("skill-debug-prompt")
	heading := el.Div().Row().Items(el.Center).H(el.Dp(44)).Px(12).Gap(6).Child(uiassets.Lucide(cx, "play", 13, theme.Muted), el.Text("/"+d.Name).Mono().TextSize(12).MaxLines(1).Grow().W(el.Dp(0)))
	if d.Result != nil {
		label := "失败"
		if d.Result.Stopped {
			label = "已停止"
		} else if d.Result.Succeeded {
			label = "通过"
		}
		heading.Child(el.Text(label).TextSize(11).TextColor(theme.Muted), el.Text(fmt.Sprintf("%.1fs · %d tokens", float64(d.Result.DurationMS)/1000, d.Result.Usage.InputTokens+d.Result.Usage.OutputTokens)).Mono().TextSize(10).TextColor(theme.Muted))
	}
	if d.HistoryMenu == nil {
		a.rebuildSkillDebugHistoryMenu()
	}
	d.HistoryMenu.SetDisabled(d.Running)
	heading.Child(d.HistoryMenu.Render(cx))
	body := el.Div().Grow().H(el.Dp(0)).ScrollY().P(12).Gap(12)
	prompt := el.TextArea().ID("skill-debug-prompt").Name("测试请求").Bind(&d.Prompt).Placeholder("输入一个应该触发此 Skill 的真实任务…").AutoGrow(1, 5).MinH(el.Dp(40)).Bg(theme.Subtle).Rounded(8).P(10).TextSize(12).ReadOnly(d.Running).FocusStyle(neutralInputFocus).Grow().W(el.Dp(0))
	fg := theme.OnColor
	label, icon, variant, callback := "运行调试", "play", kit.ButtonPrimary, a.runSkillDebug
	if d.Running {
		label, icon, variant, callback = "停止", "square", kit.ButtonSecondary, a.stopSkillDebug
		fg = theme.Text
	}
	run := skillDisabledButton(button(label, callback, variant).ID("run-skill-debug").Size(32).Content(el.ViewFunc(func(cx *el.Context) el.Element {
		return el.Div().Row().Items(el.Center).Gap(6).Child(uiassets.Lucide(cx, icon, 13, fg), el.Text(label).TextSize(12).TextColor(fg).DisabledStyle(func(style *el.Style) { style.TextColor(theme.OnColor) }))
	})), !d.Running && (strings.TrimSpace(d.Prompt) == "" || d.Name == ""))
	control := run.Render(cx)
	if div, ok := control.(*el.DivEl); ok {
		div.DisabledStyle(func(style *el.Style) { style.Bg(mixColor(theme.Primary, theme.Bg, .5)).TextColor(theme.OnColor) })
	}
	body.Child(el.Div().Row().Items(el.Start).Gap(8).Child(prompt, control))
	if d.Error != "" {
		body.Child(inspectorNotice(d.Error, true))
	}
	if d.Running && len(d.Events) == 0 {
		body.Child(el.Text("正在调试…").TextSize(12).TextColor(theme.Muted))
	}
	labels := map[string]string{"reasoning": "思考", "message": "回复", "tool_use": "工具调用", "file_change": "文件变更", "tool_result": "工具结果", "error": "错误"}
	for i, event := range d.Events {
		if event.Kind == "" && event.Text == "" {
			continue
		}
		label := labels[event.Kind]
		if label == "" {
			label = event.Kind
		}
		fg := theme.Muted
		if event.Failed {
			fg = theme.DangerText
		}
		block := el.Div().Gap(6).Child(el.Text(label).TextSize(11).Medium().TextColor(fg))
		key := fmt.Sprintf("event/%d", i)
		if event.Kind == "message" || event.Kind == "reasoning" {
			block.Child(a.skillDebugMarkdown(cx, key, event.Text))
			d.Docs[key].SetStreaming(event.Streaming)
		} else {
			text := event.Text
			lines := strings.Split(text, "\n")
			if len(lines) > 8 && !d.Expanded[i] {
				text = strings.Join(lines[:8], "\n")
			}
			block.Child(el.Text(text).Mono().TextSize(11).LineHeight(1.6))
			if len(lines) > 8 {
				index := i
				label := fmt.Sprintf("展开全部 %d 行", len(lines))
				if d.Expanded[i] {
					label = "收起"
				}
				block.Child(button(label, func() { d.Expanded[index] = !d.Expanded[index] }, kit.ButtonGhost).Size(24).Render(cx))
			}
		}
		body.Child(block)
	}
	if d.Result != nil {
		output := el.Div().Mt(4).Pt(16).Gap(8).Child(el.Text("最终输出").TextSize(10).Medium().TextColor(theme.Muted))
		if d.Result.Output != "" {
			output.Child(a.skillDebugMarkdown(cx, "output", d.Result.Output))
		} else {
			output.Child(el.Text("没有输出").TextSize(12).TextColor(theme.Muted))
		}
		if d.Result.SessionID != "" {
			output.Child(el.Text(d.Result.SessionID).Mono().TextSize(10).TextColor(theme.Muted))
		}
		body.Child(el.Div().H(el.Dp(1)).Bg(seamColor), output)
	}
	if !d.Running && d.Result == nil && len(d.Events) == 0 {
		body.Child(el.Text("使用内嵌 Modu SDK 显式调用当前 /skill，测试过程为只读，不会修改 Skill 或工作区。模型与 Provider 使用设置中的 Modu 配置。").TextSize(12).LineHeight(1.625).TextColor(theme.Muted))
	}
	return el.Div().Grow().H(el.Dp(0)).Child(heading, body)
}
