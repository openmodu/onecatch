package desktop

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	workflows "github.com/openmodu/onecatch/internal/domain/workflows"
	workflowuc "github.com/openmodu/onecatch/internal/usecase/workflows"
)

// RunWorkflowCheck compares effective configuration, not file timestamps.
type RunWorkflowCheck struct {
	Changed bool     `json:"changed"`
	Changes []string `json:"changes,omitempty"`
}

func (a *Service) runWorkflowCandidate(ctx context.Context, run workflows.Run) (workflows.Definition, workflows.Definition, workflowuc.RunResolution, error) {
	snapshot, err := a.store.Repos.Workflows.GetRunDefinition(ctx, run.ID)
	if err != nil {
		return snapshot, workflows.Definition{}, workflowuc.RunResolution{}, err
	}
	if snapshot.ID == directAgentWorkflowID {
		return snapshot, snapshot, workflowuc.RunResolution{}, coded("workflow_update_unavailable", "普通 Agent 会话没有可同步的工作流配置")
	}
	task, err := a.store.Repos.Tasks.GetTask(ctx, run.TaskID)
	if err != nil {
		return snapshot, workflows.Definition{}, workflowuc.RunResolution{}, err
	}
	latest, err := a.definitionFor(ctx, snapshot.ID, task.WorkspaceID)
	if err != nil {
		return snapshot, latest, workflowuc.RunResolution{}, err
	}
	latest, resolution, err := a.resolveExecutionSettings(ctx, task, workflows.Normalize(latest))
	if err != nil {
		return snapshot, latest, resolution, err
	}
	// Loop is selected for this Session independently of the template default.
	latest.LoopEnabled = snapshot.LoopEnabled
	return snapshot, latest, resolution, workflows.Validate(latest)
}

// CheckRunWorkflow is read-only and also works while a run is executing.
func (a *Service) CheckRunWorkflow(ctx context.Context, runID string) (RunWorkflowCheck, error) {
	run, err := a.store.Repos.Workflows.GetRun(ctx, runID)
	if err != nil {
		return RunWorkflowCheck{}, mapError(err)
	}
	snapshot, latest, resolution, err := a.runWorkflowCandidate(ctx, run)
	if err != nil {
		return RunWorkflowCheck{}, mapError(err)
	}
	changes := runWorkflowChanges(snapshot, latest, run, resolution)
	return RunWorkflowCheck{Changed: len(changes) > 0, Changes: changes}, nil
}

// UpdateRunWorkflow never resumes execution. The activity lock prevents a new
// dispatch during the write; the repository also checks paused state/revision.
func (a *Service) UpdateRunWorkflow(ctx context.Context, runID string) (workflows.Run, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.active[runID] != nil {
		return workflows.Run{}, coded("run_invalid_state", "请先暂停，等待后台执行完全停止后再更新工作流配置")
	}
	run, err := a.store.Repos.Workflows.GetRun(ctx, runID)
	if err != nil {
		return run, mapError(err)
	}
	if run.Status != workflows.RunPaused {
		return run, coded("run_invalid_state", "只有已暂停的 Session 可以更新工作流配置")
	}
	snapshot, latest, resolution, err := a.runWorkflowCandidate(ctx, run)
	if err != nil {
		return run, mapError(err)
	}
	changes := runWorkflowChanges(snapshot, latest, run, resolution)
	if len(changes) == 0 {
		return run, nil
	}
	updated := reconcileWorkflowRun(run, snapshot, latest)
	updated.MaxConcurrency = resolution.MaxConcurrency
	updated.InterruptGraceSeconds = resolution.InterruptGraceSeconds
	updated.RuntimeSettings = resolution.RuntimeSettings
	updated.UpdatedAt = time.Now().UTC()
	updated, err = a.store.Repos.Workflows.UpdatePausedRunWorkflow(ctx, updated, latest, run.Revision)
	if err != nil {
		return run, mapError(err)
	}
	payload, _ := json.Marshal(map[string]any{"changes": changes})
	_, err = a.store.Repos.Workflows.AppendEvent(ctx, workflows.WorkflowEvent{RunID: run.ID, Type: "run.workflow_updated", Payload: payload, At: updated.UpdatedAt})
	return updated, mapError(err)
}

func configEqual(left, right any) bool {
	l, _ := json.Marshal(left)
	r, _ := json.Marshal(right)
	return string(l) == string(r)
}

func runWorkflowChanges(snapshot, latest workflows.Definition, run workflows.Run, resolution workflowuc.RunResolution) []string {
	snapshot, latest = workflows.Normalize(snapshot), workflows.Normalize(latest)
	var changes []string
	if snapshot.Name != latest.Name || snapshot.Description != latest.Description {
		changes = append(changes, "工作流名称或说明")
	}
	oldIDs, newIDs := []string{}, []string{}
	for _, step := range snapshot.Steps {
		oldIDs = append(oldIDs, step.ID)
		if _, ok := workflows.FindStep(latest, step.ID); !ok {
			changes = append(changes, fmt.Sprintf("删除步骤「%s」", step.Name))
		}
	}
	for _, step := range latest.Steps {
		newIDs = append(newIDs, step.ID)
		old, ok := workflows.FindStep(snapshot, step.ID)
		if !ok {
			changes = append(changes, fmt.Sprintf("新增步骤「%s」", step.Name))
			continue
		}
		var fields []string
		for _, field := range []struct{ label, old, next string }{
			{"名称", old.Name, step.Name}, {"类型", old.Kind, step.Kind}, {"Agent", old.Runtime, step.Runtime},
			{"模型", old.Model, step.Model}, {"权限", old.Sandbox, step.Sandbox}, {"Worker", old.WorkerID, step.WorkerID},
			{"推理强度", old.ReasoningEffort, step.ReasoningEffort},
			{"提示词", old.Prompt, step.Prompt}, {"命令", old.Command, step.Command},
		} {
			if field.old != field.next {
				fields = append(fields, field.label)
			}
		}
		if len(fields) > 0 {
			changes = append(changes, fmt.Sprintf("步骤「%s」：%s", step.Name, strings.Join(fields, "、")))
		}
	}
	if !slices.Equal(oldIDs, newIDs) && len(oldIDs) == len(newIDs) {
		oldSet, newSet := slices.Clone(oldIDs), slices.Clone(newIDs)
		slices.Sort(oldSet)
		slices.Sort(newSet)
		if slices.Equal(oldSet, newSet) {
			changes = append(changes, "步骤顺序")
		}
	}
	// Edge order has no execution meaning.
	edges := func(items []workflows.Edge) []string {
		out := make([]string, 0, len(items))
		for _, edge := range items {
			data, _ := json.Marshal(edge)
			out = append(out, string(data))
		}
		slices.Sort(out)
		return out
	}
	if !slices.Equal(edges(snapshot.Edges), edges(latest.Edges)) {
		changes = append(changes, "步骤连线或返工规则")
	}
	if snapshot.Policy != latest.Policy {
		changes = append(changes, "执行策略（超时、返工或失败次数）")
	}
	if !configEqual(snapshot.Layout, latest.Layout) {
		changes = append(changes, "画布布局")
	}
	if run.MaxConcurrency != resolution.MaxConcurrency || run.InterruptGraceSeconds != resolution.InterruptGraceSeconds {
		changes = append(changes, "执行设置（并发或中断等待时间）")
	}
	for runtime, value := range resolution.RuntimeSettings {
		if !configEqual(run.RuntimeSettings[runtime], value) {
			changes = append(changes, runtime+" 运行时设置")
		}
	}
	// Stable ordering keeps checks and event payloads deterministic.
	slices.Sort(changes)
	return changes
}

func reconcileWorkflowRun(run workflows.Run, snapshot, latest workflows.Definition) workflows.Run {
	out := workflows.CloneRun(run)
	if out.Nodes == nil {
		out.Nodes = map[string]workflows.NodeState{}
	}
	for id := range out.Nodes {
		if _, ok := workflows.FindStep(latest, id); !ok {
			delete(out.Nodes, id)
		}
	}
	for id := range out.Sessions {
		old, existed := workflows.FindStep(snapshot, id)
		next, exists := workflows.FindStep(latest, id)
		if !existed || !exists || old.Runtime != next.Runtime || old.Kind != next.Kind || old.WorkerID != next.WorkerID {
			delete(out.Sessions, id)
		}
	}
	for _, step := range latest.Steps {
		if _, ok := out.Nodes[step.ID]; !ok {
			out.Nodes[step.ID] = workflows.NodeState{StepID: step.ID, Status: workflows.NodePending}
		}
	}
	validRework := map[string]bool{}
	for _, edge := range latest.Edges {
		if edge.Kind == workflows.EdgeRework {
			validRework[workflows.EdgeKey(edge.From, edge.To)] = true
		}
	}
	for key := range out.ReworkRounds {
		if !validRework[key] {
			delete(out.ReworkRounds, key)
		}
	}
	if _, ok := workflows.FindStep(latest, out.CurrentStepID); !ok {
		out.CurrentStepID = workflows.EntrySteps(latest)[0]
	}
	return out
}
