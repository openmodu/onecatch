package keeldesktop

import (
	"fmt"
	"maps"
	"slices"
	"strconv"

	"github.com/dyike/keel/ui/el"
	"github.com/dyike/keel/ui/kit"
	"github.com/dyike/keel/ui/theme"
	domainworkflows "github.com/openmodu/onecatch/internal/domain/workflows"
)

type simpleReview struct {
	Implement, Review, Rounds int
	Retry                     bool
}

// Port of frontend/src/app/simpleWorkflow.js. The guided form is available
// only when it represents every signal; an arbitrary graph stays advanced.
func simpleReviewWorkflow(d domainworkflows.Definition) (simpleReview, bool) {
	if d.Mode == domainworkflows.ModeDAG || len(d.Steps) != 2 {
		return simpleReview{}, false
	}
	implement := -1
	for i, s := range d.Steps {
		if len(s.DependsOn) > 0 {
			return simpleReview{}, false
		}
		if s.ID == d.EntryStepID {
			implement = i
		}
	}
	if implement < 0 {
		return simpleReview{}, false
	}
	review := 1 - implement
	first, last := d.Steps[implement], d.Steps[review]
	if !maps.Equal(first.Transitions, map[string]string{"ready_for_review": last.ID, "need_human": "$pause"}) {
		return simpleReview{}, false
	}
	target := last.Transitions["changes_requested"]
	if target != first.ID && target != "$pause" {
		return simpleReview{}, false
	}
	if !maps.Equal(last.Transitions, map[string]string{"changes_requested": target, "approved": "$done", "need_human": "$pause"}) {
		return simpleReview{}, false
	}
	limit := d.Policy.MaxTransitions
	if limit == 0 {
		limit = 20
	}
	if limit < 2 || limit%2 != 0 {
		return simpleReview{}, false
	}
	return simpleReview{Implement: implement, Review: review, Rounds: limit / 2, Retry: target != "$pause"}, true
}

func setReviewBehavior(d *domainworkflows.Definition, retry bool, rounds int) bool {
	simple, ok := simpleReviewWorkflow(*d)
	if !ok || rounds < 1 || rounds > 50 {
		return false
	}
	d.Policy.MaxTransitions = rounds * 2
	target := "$pause"
	if retry {
		target = d.Steps[simple.Implement].ID
	}
	d.Steps[simple.Review].Transitions["changes_requested"] = target
	return true
}

func (a *app) simpleWorkflowForm(cx *el.Context) el.Element {
	d := &a.ui.WorkflowDraft
	s, _ := simpleReviewWorkflow(*d)
	body := el.Div().ID("workflow-simple").Grow().H(el.Dp(0)).ScrollY().Px(20).Py(12).Gap(18).Disabled(a.busy)
	body.Child(el.Div().Gap(8).Child(el.Text("先实现，再由另一位 Agent 审查").TextSize(20).Bold(), el.Text("选择分工与审查规则。每次运行时，再填写具体任务目标。").TextSize(13).TextColor(theme.Muted)), workflowField("名称", a.workflowText("simple-name", "工作流名称", &d.Name, false)), el.Div().Row().Wrap().Items(el.Center).Gap(12).P(12).Rounded(10).Bg(theme.Subtle).Child(el.Text("实现任务").TextSize(13), lucide(cx, "arrow-right", 15, theme.Muted), el.Text("检查结果").TextSize(13), lucide(cx, "arrow-right", 15, theme.Muted), lucide(cx, "check", 15, theme.Text), el.Text("通过后完成").TextSize(13)))
	cards := el.Div().Gap(12)
	width, _ := cx.ViewportSize()
	wide := width >= 1100
	if wide {
		cards.Row()
	}
	for n, index := range []int{s.Implement, s.Review} {
		step := &d.Steps[index]
		title, instruction := "实现任务", "实现时需要注意什么"
		if n == 1 {
			title, instruction = "检查结果", "重点检查什么"
		}
		var options []kit.SelectOption
		for _, r := range a.runtimes {
			options = append(options, kit.SelectOption{Value: r.ID, Label: r.Name, Disabled: !r.Enabled && r.ID != step.Runtime})
		}
		if step.Runtime != "" && !slices.ContainsFunc(options, func(r kit.SelectOption) bool { return r.Value == step.Runtime }) {
			options = append(options, kit.SelectOption{Value: step.Runtime, Label: step.Runtime})
		}
		card := el.Div().Gap(12).P(12).Rounded(8).Bg(theme.Surface).Child(el.Div().Row().Items(el.Center).Gap(8).Child(el.Text(fmt.Sprint(n+1)).TextSize(12).Px(8).Py(5).Rounded(14).Bg(theme.Subtle).TextColor(theme.Muted), el.Text(title).TextSize(14).Bold()), workflowField("交给谁", a.workflowSelect(cx, fmt.Sprintf("simple-runtime-%d", index), title+" Agent", step.Runtime, options, func(value string) { step.Runtime = value })), workflowField(instruction, a.workflowText(fmt.Sprintf("simple-instruction-%d", index), instruction, &step.Instruction, true)))
		if !wide {
			card.WFull()
		} else {
			card.Grow().W(el.Dp(0))
		}
		access := map[string]string{"read-only": "只读访问", "workspace-write": "工作区访问", "full": "完全访问"}[step.Sandbox]
		worker := step.WorkerID
		if worker == "" || worker == "local" {
			worker = "本地"
		}
		card.Child(el.Text(access + " · " + worker).TextSize(11).TextColor(theme.Muted))
		cards.Child(card)
	}
	body.Child(cards)
	rules := el.Div().P(16).Rounded(12).Bg(theme.Surface).Gap(14).Child(el.Text("审查没通过，怎么办？").TextSize(14).Bold(), el.Text("审查通过就结束；需要你提供信息时暂停等待。").TextSize(12).TextColor(theme.Muted))
	action := "pause"
	if s.Retry {
		action = "retry"
	}
	rules.Child(workflowField("处理方式", a.workflowSelect(cx, "review-action", "审查规则", action, []kit.SelectOption{{Value: "retry", Label: "自动返回实现，修改后再审查"}, {Value: "pause", Label: "暂停，交给我决定"}}, func(value string) { setReviewBehavior(d, value == "retry", s.Rounds) })))
	if s.Retry {
		options := []kit.SelectOption{}
		rounds := []int{1, 2, 3, 5, 10, s.Rounds}
		slices.Sort(rounds)
		rounds = slices.Compact(rounds)
		for _, round := range rounds {
			options = append(options, kit.SelectOption{Value: strconv.Itoa(round), Label: fmt.Sprintf("%d 轮", round)})
		}
		rules.Child(workflowField("最多审查几轮", a.workflowSelect(cx, "review-rounds", "审查轮数", strconv.Itoa(s.Rounds), options, func(value string) { n, _ := strconv.Atoi(value); setReviewBehavior(d, s.Retry, n) })))
	}
	body.Child(rules)
	body.Child(iconButton("角色要求", func() { a.ui.WorkflowRoles = !a.ui.WorkflowRoles }, kit.ButtonGhost, kit.IconChevronDown).Render(cx))
	if a.ui.WorkflowRoles {
		roles := el.Div().Gap(16)
		for _, index := range []int{s.Implement, s.Review} {
			step := &d.Steps[index]
			roles.Child(workflowField(step.Name+" · 角色要求", a.workflowText(fmt.Sprintf("simple-role-%d", index), step.Name+" · 角色要求", &step.RolePrompt, true)))
		}
		body.Child(roles, el.Text("需要修改信号、权限、模型或执行位置时，请使用高级编辑。").TextSize(11).TextColor(theme.Muted))
	}
	if err := domainworkflows.Validate(*d); err != nil {
		body.Child(el.Text(err.Error()).TextSize(12).TextColor(theme.DangerText))
	}
	return body
}
