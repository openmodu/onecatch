package keeldesktop

import (
	"context"
	"strings"
	"time"

	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/theme"
	"github.com/google/uuid"
	templates "github.com/openmodu/onecatch/internal/domain/prompttemplates"
	"github.com/openmodu/onecatch/internal/service/skillmanager"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

type libraryState struct {
	Queue                                bool
	Templates                            []templates.Template
	TemplateID, TemplateSearch, Material string
	TemplateArgs                         map[string]string
	TemplateEditing                      *templates.Template
	TemplateLoadError                    error
	TemplateRevision                     string
	Skills                               []skillmanager.Skill
	SkillSearch, SkillName               string
	SkillDoc                             *skillmanager.SkillDocument
	SkillLoading                         bool
	SkillVersion                         uint64
	Usage                                []agentrun.AccountUsage
	UsageLoading                         bool
	UsageFilter                          string
	UsageChart                           *kit.ChartView
	UsageHeatmapReady                    string
	UsageChartKey                        string
	UsageVersion                         uint64
	UsageErrors                          map[string]string
}

func (a *app) initLibrary() {
	a.library.TemplateArgs = map[string]string{}
	a.library.UsageErrors = map[string]string{}
	result, err := a.backend.service.LoadPromptTemplates(a.ctx)
	a.library.Templates = result.Templates
	a.library.TemplateRevision = result.Revision
	a.library.TemplateLoadError = err
	a.library.TemplateID = "builtin-explain"
}

func (a *app) selectedTemplate() templates.Template {
	for _, item := range a.library.Templates {
		if item.ID == a.library.TemplateID {
			return item
		}
	}
	return a.library.Templates[0]
}
func (a *app) templateContext() map[string]string {
	result := map[string]string{"selection": a.library.Material, "date": time.Now().Format("2006-01-02"), "task": ""}
	for _, w := range a.workspaces {
		if w.ID == a.workspaceID {
			result["project"], result["path"] = w.Name, w.Path
		}
	}
	if a.task != nil {
		result["task"] = a.task.Title
	}
	return result
}
func (a *app) templatesPage(cx *el.Context) el.Element {
	item := a.selectedTemplate()
	list := el.Div().W(el.Dp(240)).NoShrink().HFull().Bg(sidebarColor).Px(16).Py(16).Gap(12)
	list.Child(el.Div().Row().Items(el.Center).Child(el.Text("模板库").Grow().Medium().TextSize(14), iconAction(cx, "new-template", "新建模板", "plus", func() {
		a.library.TemplateEditing = &templates.Template{ID: "custom-" + uuid.NewString(), Body: "{{selection}}"}
	}, false)), field("template-search", "搜索模板", &a.library.TemplateSearch))
	rows := el.Div().ID("template-list").ScrollY().Grow().H(el.Dp(0)).Gap(4)
	for _, t := range a.library.Templates {
		p := t
		if !strings.Contains(strings.ToLower(t.Name), strings.ToLower(a.library.TemplateSearch)) {
			continue
		}
		row := el.Div().Role("button").Name(t.Name).Focusable(true).Rounded(8).Px(10).Py(12).Gap(4).CursorPointer().OnClick(func() {
			a.library.TemplateID = p.ID
			a.library.TemplateArgs = map[string]string{}
			a.library.TemplateEditing = nil
		}).Hover(func(s *el.Style) { s.Bg(theme.Highlight) }).FocusStyle(neutralFocus)
		if item.ID == t.ID {
			row.Bg(theme.Highlight)
		}
		kind := "自定义"
		if t.Builtin {
			kind = "内置"
		}
		rows.Child(row.Child(el.Text(t.Name).TextSize(13).Medium(), el.Text(kind).TextSize(11).TextColor(theme.Muted)))
	}
	list.Child(rows, el.Text("模板保存在本地").TextSize(11).TextColor(theme.Muted))
	detail := el.Div().Grow().W(el.Dp(0)).HFull().P(24).Gap(16)
	if draft := a.library.TemplateEditing; draft != nil {
		detail.Child(el.Text("编辑模板").TextSize(22).Medium(), field("template-name", "模板名称", &draft.Name), el.TextArea().ID("template-body").Bind(&draft.Body).Name("模板内容").Border(0, theme.Border).Bg(theme.Subtle).Rounded(8).P(12).Grow().H(el.Dp(0)).Mono().TextSize(12), el.Text("变量：{{selection}}、{{project}}、{{path}}、{{task}}、{{date}}、{{arg:参数名}}、{{template:模板ID}}").TextSize(11).TextColor(theme.Muted), a.errorBanner(cx), el.Div().Row().Justify(el.End).Gap(8).Child(button("取消", func() { a.library.TemplateEditing = nil }, kit.ButtonGhost).Render(cx), button("保存", a.saveTemplate, kit.ButtonPrimary).Render(cx)))
	} else {
		copyLabel := "编辑"
		if item.Builtin {
			copyLabel = "创建副本"
		}
		detail.Child(el.Div().Row().Items(el.Center).Child(el.Text(item.Name).Grow().TextSize(22).Medium(), iconButton(copyLabel, func() {
			draft := item
			if draft.Builtin {
				draft.ID = "custom-" + uuid.NewString()
				draft.Builtin = false
			}
			a.library.TemplateEditing = &draft
		}, kit.ButtonGhost, kit.IconEdit).Render(cx)))
		args, err := a.backend.service.PromptTemplateArguments(item, a.library.Templates)
		a.fail(err)
		preview, previewErr := a.backend.service.RenderPromptTemplate(item, a.library.Templates, a.templateContext(), a.library.TemplateArgs)
		inputs := el.Div().Grow().W(el.Dp(0)).Gap(10).Child(el.Text("1  输入材料").Medium().TextSize(13), el.TextArea().ID("template-material").Name("待处理材料").Bind(&a.library.Material).AutoGrow(7, 15).Border(0, theme.Border).Bg(theme.Subtle).Rounded(8).P(12).TextSize(13))
		for _, name := range args {
			n := name
			value := a.library.TemplateArgs[n]
			inputs.Child(el.Text(n).TextSize(12), field("template-arg/"+n, "填写 "+n, &value).OnChange(func(v string) { a.library.TemplateArgs[n] = v }))
		}
		output := el.Div().Grow().W(el.Dp(0)).Gap(10).Child(el.Text("2  指令预览").Medium().TextSize(13))
		if previewErr != nil {
			output.Child(el.Text(previewErr.Error()).TextSize(12).TextColor(theme.Muted))
		} else {
			output.Child(el.Text(preview).Mono().TextSize(12).P(12).Rounded(8).Bg(theme.Subtle))
		}
		detail.Child(el.Div().Row().Grow().H(el.Dp(0)).ScrollY().Gap(20).Child(inputs, output), a.errorBanner(cx))
		insert := button("插入输入框", func() {
			if a.prompt != "" {
				a.prompt += "\n\n"
			}
			a.prompt += preview
			a.goPage("")
		}, kit.ButtonPrimary)
		insert.SetDisabled(previewErr != nil || a.workspaceID == "")
		detail.Child(el.Div().Row().Items(el.Center).Child(el.Text("目标：当前任务输入框").Grow().TextSize(12).TextColor(theme.Muted), insert.Render(cx)))
	}
	if a.library.TemplateLoadError != nil {
		detail.Child(el.Text(a.library.TemplateLoadError.Error()).TextColor(theme.DangerText).TextSize(12))
	}
	return el.Div().Row().Grow().H(el.Dp(0)).Child(list, el.Div().W(el.Dp(1)).Bg(seamColor).NoShrink(), detail)
}
func (a *app) saveTemplate() {
	draft := a.library.TemplateEditing
	if draft == nil || a.library.TemplateLoadError != nil {
		return
	}
	result, err := a.backend.service.SavePromptTemplate(a.ctx, *draft, a.library.TemplateRevision)
	if err != nil {
		a.fail(err)
		return
	}
	a.library.TemplateRevision = result.Revision
	a.library.Templates = result.Templates
	a.library.TemplateID = draft.ID
	a.library.TemplateEditing = nil
	a.errorText = ""
}

func (a *app) loadUsage(refresh bool) {
	if a.library.UsageLoading {
		return
	}
	ids := []string{}
	for _, r := range a.runtimes {
		if r.Enabled && r.Available {
			ids = append(ids, r.ID)
		}
	}
	if len(ids) == 0 {
		return
	}
	a.library.UsageLoading = true
	a.library.UsageVersion++
	version := a.library.UsageVersion
	go func() {
		items := []agentrun.AccountUsage{}
		failures := map[string]string{}
		for _, id := range ids {
			ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
			var item agentrun.AccountUsage
			var err error
			if refresh {
				item, err = a.backend.service.SyncAccountUsage(ctx, id)
			} else {
				item, err = a.backend.service.GetAccountUsage(ctx, id)
			}
			cancel()
			if err != nil {
				failures[id] = err.Error()
			} else {
				items = append(items, item)
			}
		}
		core.Update(func() {
			if a.closed || version != a.library.UsageVersion {
				return
			}
			a.library.UsageLoading = false
			a.library.Usage, a.library.UsageErrors = items, failures
		})
	}()
}
