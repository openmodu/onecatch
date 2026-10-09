package keeldesktop

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/dyike/keel/ui/core"
	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/theme"
	"github.com/openmodu/onecatch/internal/service/skillmanager"
)

type skillSyncService interface {
	SyncSkill(context.Context, string) (skillmanager.SyncSkillResult, error)
	SyncSkills(context.Context, string) (skillmanager.SyncResult, error)
	SetSkillSyncTargetSkills(skillmanager.SetTargetSkillsInput) (skillmanager.SyncTarget, error)
}

func (a *app) skillSyncService() skillSyncService {
	if api, ok := a.skills.api.(skillSyncService); ok {
		return api
	}
	return a.backend.service
}
func skillTargetReceives(target skillmanager.SyncTarget, name string) bool {
	return len(target.Skills) == 0 || slices.Contains(target.Skills, name)
}
func (a *app) toggleSkillTarget(id, name string, on bool) {
	s := &a.skills
	if a.closed || s.Busy != "" || a.library.SkillLoading {
		return
	}
	var target *skillmanager.SyncTarget
	for i := range s.Targets {
		if s.Targets[i].ID == id {
			target = &s.Targets[i]
			break
		}
	}
	if target == nil {
		return
	}
	names := []string{}
	for _, skill := range a.library.Skills {
		selected := skillTargetReceives(*target, skill.Name)
		if skill.Name == name {
			selected = on
		}
		if selected {
			names = append(names, skill.Name)
		}
	}
	if len(names) == len(a.library.Skills) {
		names = []string{}
	}
	// The manager's legacy empty selection means all skills, not none.
	if len(names) == 0 && !on {
		s.Error = "同步目标至少接收一个 Skill。"
		a.rebuildSkillTargetMenus()
		return
	}
	s.Busy = "selection"
	api := a.skillSyncService()
	go func() {
		_, err := api.SetSkillSyncTargetSkills(skillmanager.SetTargetSkillsInput{ID: id, Skills: names})
		core.Update(func() {
			if a.closed {
				return
			}
			s.Busy = ""
			if err != nil {
				s.Error = err.Error()
				a.rebuildSkillTargetMenus()
				return
			}
			a.loadSkills()
		})
	}()
}
func (a *app) syncManagedSkills(name string) {
	s := &a.skills
	if a.closed || s.Busy != "" || a.library.SkillLoading {
		return
	}
	targets := slices.Clone(s.Targets)
	api := a.skillSyncService()
	s.Busy = "sync/" + name
	s.Error = ""
	go func() {
		var err error
		if name != "" {
			_, err = api.SyncSkill(a.ctx, name)
		} else {
			for _, target := range targets {
				if !target.RsyncAvailable || target.TotalSkills == 0 {
					continue
				}
				if _, err = api.SyncSkills(a.ctx, target.ID); err != nil {
					break
				}
			}
		}
		core.Update(func() {
			if a.closed {
				return
			}
			s.Busy = ""
			if err != nil {
				s.Error = err.Error()
			}
			a.loadSkills()
		})
	}()
}
func (a *app) rebuildSkillTargetMenus() {
	s := &a.skills
	s.TargetMenus = map[string]*kit.MenuView{}
	for _, entry := range a.library.Skills {
		skill := entry
		menu := kit.Menu().Width(220).Trigger(el.ViewFunc(func(cx *el.Context) el.Element {
			count := 0
			for _, target := range s.Targets {
				if skillTargetReceives(target, skill.Name) {
					count++
				}
			}
			b := skillDisabledButton(button(fmt.Sprintf("%d / %d 个目标", count, len(s.Targets)), nil, kit.ButtonGhost).Size(28), s.Busy != "" || a.library.SkillLoading || len(s.Targets) == 0).Content(el.ViewFunc(func(cx *el.Context) el.Element {
				return el.Div().Row().Items(el.Center).Gap(8).Child(el.Text(fmt.Sprintf("%d / %d 个目标", count, len(s.Targets))).TextSize(12).TextColor(theme.Muted), lucide(cx, "chevron-down", 13, theme.Muted))
			}))
			return b.Render(cx)
		}))
		for _, value := range s.Targets {
			target := value
			menu.CheckItem(target.Name, "", skillTargetReceives(target, skill.Name), func(on bool) { a.toggleSkillTarget(target.ID, skill.Name, on) })
		}
		s.TargetMenus[skill.Name] = menu
	}
}
func (a *app) skillSyncPage(cx *el.Context) el.Element {
	s := &a.skills
	root := el.Div().Gap(6).Child(el.Text("同步到 Agent 应用").TextSize(19).Bold(), el.Text("每个 Skill 通过 rsync 单独同步到目标目录；Skill 内已删除的资源会清理，但不会删除目标应用中非 OneCatch 管理的其他 Skill。").TextSize(13).LineHeight(1.625).TextColor(theme.Muted))
	targetSection := el.Div().Mt(18).Gap(8).Child(el.Div().Row().Items(el.Center).Gap(12).Child(el.Text("同步目标").TextSize(13).Bold().Grow(), el.Text("在「设置 › Skills」里增删").TextSize(11).TextColor(theme.Muted)))
	targetRows := el.Div().Bg(mixColor(theme.Subtle, theme.Bg, .3)).Rounded(12).Px(14).Py(6).Gap(2)
	labels := map[string]string{"synced": "已同步", "missing": "路径未创建", "out-of-sync": "有更新", "partial": "部分同步", "rsync-unavailable": "缺少 rsync", "error": "同步失败"}
	for _, entry := range s.Targets {
		label := labels[entry.Status]
		if label == "" {
			label = entry.Status
		}
		fg := theme.Primary
		bg := mixColor(theme.Primary, theme.Surface, .09)
		if entry.Status == "synced" {
			fg = theme.Success
			bg = mixColor(theme.Success, theme.Surface, .09)
		}
		targetRows.Child(el.Div().Row().Items(el.Center).Gap(8).Py(6).Child(el.Text(entry.Name).TextSize(12).Medium(), el.Text(label).TextSize(11).TextColor(fg).Bg(bg).Rounded(5).Px(7).Py(2), el.Text(entry.Path).Mono().TextSize(11).TextColor(theme.Muted).MaxLines(1).Grow().W(el.Dp(0))))
		if entry.LastError != "" {
			targetRows.Child(el.Text(entry.LastError).TextSize(11).TextColor(theme.DangerText))
		}
		if !entry.RsyncAvailable {
			targetRows.Child(el.Text("需要安装 rsync 后才能同步。").TextSize(11).TextColor(theme.DangerText))
		}
	}
	targetSection.Child(targetRows)
	root.Child(targetSection)
	library := el.Div().Mt(22).Gap(2).Child(el.Div().Row().Items(el.Center).Child(el.Text("Skill 库").TextSize(13).Bold().Grow(), skillDisabledButton(button("全部同步", func() { a.syncManagedSkills("") }, kit.ButtonGhost).Size(28).Content(el.ViewFunc(func(cx *el.Context) el.Element {
		return el.Div().Row().Items(el.Center).Gap(6).Child(lucide(cx, "refresh-cw", 14, theme.Muted), el.Text("全部同步").TextSize(12).TextColor(theme.Muted))
	})), s.Busy != "" || len(a.library.Skills) == 0).Loading(s.Busy == "sync/").Render(cx)))
	for _, entry := range a.library.Skills {
		skill := entry
		receiving, current, blocked := 0, 0, false
		var receivingNames []string
		for _, target := range s.Targets {
			if skillTargetReceives(target, skill.Name) {
				receiving++
				receivingNames = append(receivingNames, target.Name)
				if slices.Contains(target.SyncedNames, skill.Name) {
					current++
				}
				blocked = blocked || !target.RsyncAvailable
			}
		}
		label := fmt.Sprintf("%d 个目标中有 %d 个待更新", receiving, receiving-current)
		if receiving == 0 {
			label = "未选择同步目标"
		} else if receiving == current {
			label = "已同步到 " + strings.Join(receivingNames, "、")
		}
		row := el.Div().Row().Items(el.Center).Gap(12).Py(12).Child(el.Div().Grow().W(el.Dp(0)).Gap(2).Child(el.Div().MinH(el.Dp(21)).Child(el.Text(skill.Name).TextSize(13).Medium().MaxLines(1)), el.Div().MinH(el.Dp(17)).Child(el.Text(label).TextSize(11).TextColor(theme.Muted).MaxLines(1))))
		if menu := s.TargetMenus[skill.Name]; menu != nil {
			menu.SetDisabled(s.Busy != "" || a.library.SkillLoading)
			row.Child(menu.Render(cx))
		}
		row.Child(skillDisabledButton(button("立即同步", func() { a.syncManagedSkills(skill.Name) }, kit.ButtonGhost).ID("sync-skill/"+skill.Name).Size(28).Content(el.ViewFunc(func(cx *el.Context) el.Element {
			return el.Div().Row().Items(el.Center).Gap(6).Child(lucide(cx, "refresh-cw", 14, theme.Muted), el.Text("立即同步").TextSize(12).TextColor(theme.Muted))
		})), s.Busy != "" || receiving == 0 || blocked).Loading(s.Busy == "sync/"+skill.Name).Render(cx))
		library.Child(row)
	}
	return root.Child(library, el.Text("同步时间、目标路径、Skill 摘要和错误记录在 ~/.onecatch/skill-sync.json。刷新会重新扫描目标路径及内容。").TextSize(11).LineHeight(1.625).TextColor(theme.Muted).Mt(18))
}
