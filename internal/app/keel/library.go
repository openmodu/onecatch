package keeldesktop

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/theme"
	"github.com/google/uuid"
	"github.com/openmodu/onecatch/internal/service/skillmanager"
	"github.com/openmodu/onecatch/internal/usecase/agentrun"
)

//go:embed templates.json
var templateCatalog []byte

type promptTemplate struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Body    string `json:"body"`
	Builtin bool   `json:"builtin,omitempty"`
}
type templateLibrary struct {
	Version   int              `json:"version"`
	Templates []promptTemplate `json:"templates"`
}
type libraryState struct {
	Queue                                bool
	Templates                            []promptTemplate
	TemplateID, TemplateSearch, Material string
	TemplateArgs                         map[string]string
	TemplateEditing                      *promptTemplate
	TemplateLoadError                    error
	TemplateOriginal                     []byte
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
	var builtin []promptTemplate
	_ = json.Unmarshal(templateCatalog, &builtin)
	a.library.Templates = builtin
	a.library.TemplateID = "builtin-explain"
	data, err := os.ReadFile(filepath.Join(a.backend.service.DataRoot(), "keel-prompt-templates.json"))
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err != nil {
		a.library.TemplateLoadError = err
		return
	}
	a.library.TemplateOriginal = data
	var saved templateLibrary
	if err = json.Unmarshal(data, &saved); err != nil || saved.Version != 1 {
		a.library.TemplateLoadError = fmt.Errorf("模板库格式无效")
		return
	}
	if err = validateTemplates(saved.Templates); err != nil {
		a.library.TemplateLoadError = err
		return
	}
	a.library.Templates = append(a.library.Templates, saved.Templates...)
}
func (a *app) selectedTemplate() promptTemplate {
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
		a.library.TemplateEditing = &promptTemplate{ID: "custom-" + uuid.NewString(), Body: "{{selection}}"}
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
		args, err := templateArguments(item, a.library.Templates)
		a.fail(err)
		preview, previewErr := renderTemplate(item, a.library.Templates, a.templateContext(), a.library.TemplateArgs)
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
	custom := []promptTemplate{}
	for _, p := range a.library.Templates {
		if !p.Builtin && p.ID != draft.ID {
			custom = append(custom, p)
		}
	}
	custom = append(custom, *draft)
	if err := validateTemplates(custom); err != nil {
		a.fail(err)
		return
	}
	library := []promptTemplate{}
	for _, p := range a.library.Templates {
		if p.Builtin {
			library = append(library, p)
		}
	}
	library = append(library, custom...)
	if _, err := templateArguments(*draft, library); err != nil {
		a.fail(err)
		return
	}
	path := filepath.Join(a.backend.service.DataRoot(), "keel-prompt-templates.json")
	current, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		a.fail(err)
		return
	}
	if string(current) != string(a.library.TemplateOriginal) {
		a.fail(fmt.Errorf("模板库已被其他窗口修改，请重新打开后保存"))
		return
	}
	data, _ := json.MarshalIndent(templateLibrary{1, custom}, "", "  ")
	if err := writeAtomic(path, data); err != nil {
		a.fail(err)
		return
	}
	a.library.TemplateOriginal = data
	a.library.Templates = library
	a.library.TemplateID = draft.ID
	a.library.TemplateEditing = nil
	a.errorText = ""
}
func writeAtomic(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".keel-write-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), path)
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
