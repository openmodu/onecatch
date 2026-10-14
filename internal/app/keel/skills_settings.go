package keeldesktop

import (
	"strings"

	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/theme"
	"github.com/openmodu/onecatch/internal/service/skillmanager"
)

type skillTargetService interface {
	ScanSkillSyncTargets() ([]skillmanager.SyncTarget, error)
	AddSkillSyncTarget(skillmanager.AddTargetInput) (skillmanager.SyncTarget, error)
	UpdateSkillSyncTarget(skillmanager.UpdateTargetInput) (skillmanager.SyncTarget, error)
	RemoveSkillSyncTarget(string) error
}
type skillTargetSettings struct {
	api                     skillTargetService
	Targets                 []skillmanager.SyncTarget
	Paths                   map[string]*string
	Focused                 map[string]bool
	Add, Confirm            *kit.DialogView
	Name, Path, Error, Busy string
	Version                 uint64
	Loaded                  bool
}

func (p *settingsPage) skillTargetsAPI() skillTargetService {
	if p.skillTargets.api != nil {
		return p.skillTargets.api
	}
	return p.app.backend.service
}
func (p *settingsPage) initSkillTargetSettings() {
	s := &p.skillTargets
	if s.Paths != nil {
		return
	}
	s.Paths = map[string]*string{}
	s.Focused = map[string]bool{}
	s.Confirm = kit.Dialog("").Width(448)
	s.Add = kit.Dialog("添加目标").Width(448).CloseButton(true).BeforeCancel(func() bool { return s.Busy == "" }).Body(el.ViewFunc(func(cx *el.Context) el.Element {
		body := el.Div().Gap(16).Child(el.Text("为其他兼容 Agent 添加 Skills 目录。目标可以不存在，首次同步时会创建。").TextSize(13).LineHeight(1.625).TextColor(theme.Muted), workflowField("应用名称", field("skill-target-name", "Cursor", &s.Name).ReadOnly(s.Busy != "")), workflowField("Skills 目录", field("skill-target-path", "~/.cursor/skills", &s.Path).Mono().ReadOnly(s.Busy != "")))
		if s.Error != "" {
			body.Child(el.Text(s.Error).TextSize(12).TextColor(theme.DangerText))
		}
		return body
	})).Footer(button("取消", func() {
		if s.Busy == "" {
			s.Add.SetValue(false)
		}
	}, kit.ButtonGhost), button("添加", func() { p.mutateSkillTarget("add", "") }, kit.ButtonPrimary))
}
func (p *settingsPage) loadSkillTargets() {
	if p.app.closed || p.workers.closed {
		return
	}
	p.initSkillTargetSettings()
	s := &p.skillTargets
	if s.Busy != "" {
		return
	}
	s.Version++
	version, api := s.Version, p.skillTargetsAPI()
	s.Busy = "load"
	go func() {
		items, err := api.ScanSkillSyncTargets()
		core.Update(func() { p.applySkillTargets(version, items, err) })
	}()
}
func (p *settingsPage) applySkillTargets(version uint64, items []skillmanager.SyncTarget, err error) {
	s := &p.skillTargets
	if p.app.closed || p.workers.closed || version != s.Version {
		return
	}
	s.Busy = ""
	s.Loaded = true
	if err != nil {
		s.Error = err.Error()
		return
	}
	s.Error = ""
	oldPaths := map[string]string{}
	for _, target := range s.Targets {
		oldPaths[target.ID] = target.Path
	}
	s.Targets = items
	for _, target := range items {
		if _, ok := s.Paths[target.ID]; !ok {
			value := target.Path
			s.Paths[target.ID] = &value
		} else if *s.Paths[target.ID] == oldPaths[target.ID] {
			*s.Paths[target.ID] = target.Path
		}
	}
	p.app.skills.Targets = items
	p.app.rebuildSkillTargetMenus()
}
func (p *settingsPage) commitSkillTargetPath(id string) {
	s := &p.skillTargets
	if s.Busy != "" {
		return
	}
	value := s.Paths[id]
	if value == nil {
		return
	}
	for _, target := range s.Targets {
		if target.ID == id {
			if strings.TrimSpace(*value) == "" || strings.TrimSpace(*value) == target.Path {
				*value = target.Path
				return
			}
			p.mutateSkillTarget("update", id)
			return
		}
	}
}
func (p *settingsPage) commitBlurredSkillPaths(cx *el.Context) {
	s := &p.skillTargets
	for id, was := range s.Focused {
		focused := p.section == "skills" && cx.Focused("skill-target/"+id)
		if was && !focused {
			p.commitSkillTargetPath(id)
		}
		s.Focused[id] = focused
	}
}
func (p *settingsPage) mutateSkillTarget(kind, id string) {
	if p.app.closed || p.workers.closed {
		return
	}
	p.initSkillTargetSettings()
	s := &p.skillTargets
	if s.Busy != "" {
		return
	}
	api := p.skillTargetsAPI()
	name, directory := strings.TrimSpace(s.Name), strings.TrimSpace(s.Path)
	if kind == "add" && (name == "" || directory == "") {
		s.Error = "请填写应用名称和目标目录。"
		return
	}
	if kind == "update" {
		value := s.Paths[id]
		if value == nil {
			return
		}
		directory = strings.TrimSpace(*value)
	}
	if kind == "remove" {
		for _, target := range s.Targets {
			if target.ID == id && target.Builtin {
				return
			}
		}
	}
	s.Version++
	version := s.Version
	s.Busy = kind
	s.Error = ""
	go func() {
		var err error
		switch kind {
		case "add":
			_, err = api.AddSkillSyncTarget(skillmanager.AddTargetInput{Name: name, Path: directory})
		case "update":
			_, err = api.UpdateSkillSyncTarget(skillmanager.UpdateTargetInput{ID: id, Path: directory})
		case "remove":
			err = api.RemoveSkillSyncTarget(id)
		}
		var items []skillmanager.SyncTarget
		if err == nil {
			items, err = api.ScanSkillSyncTargets()
		}
		core.Update(func() {
			if p.app.closed || p.workers.closed || version != s.Version {
				return
			}
			p.applySkillTargets(version, items, err)
			if err != nil {
				return
			}
			if kind == "add" {
				s.Add.SetValue(false)
				s.Name, s.Path = "", ""
			}
			if kind == "update" {
				for _, target := range items {
					if target.ID == id {
						*s.Paths[id] = target.Path
					}
				}
			}
		})
	}()
}
func (p *settingsPage) skillTargetSettings(cx *el.Context) el.Element {
	p.initSkillTargetSettings()
	s := &p.skillTargets
	if !s.Loaded && s.Busy == "" {
		p.loadSkillTargets()
	}
	root := el.Div().Gap(16).Child(s.Add.Render(cx), s.Confirm.Render(cx), el.Div().Row().Items(el.Start).Gap(8).Child(el.Div().Grow().W(el.Dp(0)).Gap(6).Child(el.Text("同步目标").TextSize(15).Bold(), el.Text("OneCatch 把 ~/.onecatch/skills 里的 Skill 同步到这些目录，目录会在首次同步时创建。选择同步哪些 Skill 在 Skills 页的「同步」里设置。").TextSize(12).LineHeight(1.625).TextColor(theme.Muted)), iconAction(cx, "add-skill-target", "添加目标", "plus", func() {
		if s.Busy == "" {
			s.Error = ""
			s.Add.SetValue(true)
		}
	}, false)))
	rows := el.Div().Bg(theme.Surface).Rounded(12).P(16).Gap(8)
	for _, entry := range s.Targets {
		target := entry
		value := s.Paths[target.ID]
		if value == nil {
			continue
		}
		label := el.Div().W(el.Dp(150)).NoShrink().Gap(3).Child(el.Text(target.Name).TextSize(13).Medium().MaxLines(1))
		if !target.Builtin {
			label.Child(el.Text("自定义").TextSize(11).TextColor(theme.Muted))
		}
		input := field("skill-target/"+target.ID, target.Name+" · Skills 目录", value).Mono().TextSize(12).Bg(theme.Subtle).H(el.Dp(32)).MinH(el.Dp(32)).Grow().W(el.Dp(0)).ReadOnly(s.Busy != "").OnSubmit(func(string) { p.commitSkillTargetPath(target.ID) })
		if cx.Focused("skill-target/" + target.ID) {
			s.Focused[target.ID] = true
		}
		row := el.Div().Row().Items(el.Center).Gap(16).Rounded(8).Bg(mixColor(theme.Subtle, theme.Bg, .35)).Px(16).Py(12).Child(label, input)
		if target.Builtin {
			row.Child(el.Div().W(el.Dp(28)))
		} else {
			row.Child(iconAction(cx, "remove-skill-target/"+target.ID, "移除 "+target.Name, "x", func() {
				if s.Busy != "" {
					return
				}
				s.Confirm.ConfirmDanger("移除“"+target.Name+"”？", "只移除同步目标配置，目标目录中的 Skill 文件会保留。", "移除", func() { p.mutateSkillTarget("remove", target.ID) })
			}, false))
		}
		rows.Child(row)
	}
	root.Child(rows)
	if s.Error != "" {
		root.Child(el.Text(s.Error).TextSize(12).TextColor(theme.DangerText), button("重新读取", p.loadSkillTargets, kit.ButtonGhost).Size(28).Render(cx))
	}
	return root
}
