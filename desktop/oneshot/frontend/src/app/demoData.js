import { singleTemplate, loopTemplate, dagTemplate } from "./templates.js";

export const demoNow = new Date().toISOString();

export const demoWorkspaces = [
  { id: "oneshot-demo", name: "oneshot", path: "~/Code/openmodu/oneshot", defaultSandbox: "workspace-write", lastOpenedAt: demoNow },
];

export const demoRuntimes = [
  { id: "codex", name: "Codex", available: true, version: "codex 0.98.0" },
  { id: "claude", name: "Claude Code", available: true, version: "2.1.4" },
  { id: "modu", name: "Modu Code", available: true, version: "ACP · executable" },
];

export const demoWorkflows = [
  { ...singleTemplate, id: "single_agent", name: "单 Agent 完成" },
  { ...loopTemplate, id: "implement_review", name: "实现与审查 Loop" },
  { ...dagTemplate, id: "parallel_review", name: "并行审查 DAG" },
];

export const demoWorkers = [{ id: "mac-mini", name: "Mac mini", baseUrl: "http://192.168.1.20:9231", enabled: true, hasToken: true }];

export const demoTasks = [
  { id: "task_demo", workspaceId: "oneshot-demo", title: "优化本地 Agent 调度流程", prompt: "梳理并改进工作流执行、暂停与恢复体验。", workflowId: "implement_review", status: "paused", createdAt: demoNow, updatedAt: demoNow },
  { id: "task_queue_demo", workspaceId: "oneshot-demo", title: "补齐任务页 Git 工作流", prompt: "接通状态、Diff、提交信息生成和推送，并验证异常提示。", workflowId: "single_agent", status: "queued", executionMode: "queued", queue: { state: "waiting", enqueuedAt: demoNow, authorized: true }, attachments: [], createdAt: demoNow, updatedAt: demoNow },
];

export const demoRun = {
  run: { id: "run_demo", taskId: "task_demo", workflowId: "implement_review", status: "paused", currentStepId: "implement", transitionCount: 2, pauseReason: "workflow_signal", updatedAt: demoNow, history: [
    { fromStepId: "implement", signal: "ready_for_review", target: "review", content: "已完成调度层拆分并补充测试。", at: demoNow },
    { fromStepId: "review", signal: "changes_requested", target: "implement", content: "需要补充 UI 中断后的状态反馈。", at: demoNow },
  ] },
  task: demoTasks[0],
  workspace: demoWorkspaces[0],
  workflow: demoWorkflows[1],
  stepRuns: [
    { id: "step_1", stepId: "implement", attempt: 1, status: "succeeded", signal: "ready_for_review", content: "完成后端应用服务", sessionIdAfter: "019f4b11-codex-demo", startedAt: demoNow, finishedAt: demoNow },
    { id: "step_2", stepId: "review", attempt: 1, status: "succeeded", signal: "changes_requested", content: "要求完善中断反馈", sessionIdAfter: "45db32aa-claude-demo", startedAt: demoNow, finishedAt: demoNow },
  ],
  events: [
    { seq: 1, type: "run.started", stepId: "", at: demoNow },
    { seq: 2, type: "transition.applied", stepId: "implement", at: demoNow },
    { seq: 3, type: "transition.applied", stepId: "review", at: demoNow },
    { seq: 4, type: "run.paused", stepId: "implement", at: demoNow },
  ],
  runtimeEvents: [
    { stepRunId: "step_1", seq: 1, kind: "started", text: "019f4b11-codex-demo", at: demoNow },
    { stepRunId: "step_1", seq: 2, kind: "tool_use", text: "rg -n \"interrupt|pause\" internal desktop", at: demoNow },
    { stepRunId: "step_1", seq: 3, kind: "tool_result", text: "internal/usecase/workflows/usecase.go:212\ndesktop/oneshot/frontend/src/app/App.jsx:118", at: demoNow },
    { stepRunId: "step_1", seq: 4, kind: "file_change", text: "internal/usecase/workflows/usecase.go", at: demoNow },
    { stepRunId: "step_1", seq: 5, kind: "message", text: "已完成调度层拆分并补充中断恢复测试。", at: demoNow },
    { stepRunId: "step_2", seq: 1, kind: "tool_use", text: "go test ./...", at: demoNow },
    { stepRunId: "step_2", seq: 2, kind: "tool_result", text: "ok  github.com/openmodu/oneshot/internal/usecase/workflows", at: demoNow },
    { stepRunId: "step_2", seq: 3, kind: "message", text: "核心流程正确，但需要补充 UI 中断后的状态反馈。", at: demoNow },
    { stepRunId: "step_2", seq: 4, kind: "result", text: '{"signal":"changes_requested","content":"建议在暂停区域明确展示恢复后的当前步骤。"}', at: demoNow },
  ],
  active: false,
};
