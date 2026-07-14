export const singleTemplate = {
  id: "my_single_agent",
  name: "我的单 Agent 流程",
  description: "一个 Agent 独立完成任务，需要时交还给我。",
  entryStepId: "execute",
  policy: { maxTransitions: 20, maxConsecutiveFailures: 3, stepTimeoutSeconds: 1800 },
  steps: [
    {
      id: "execute",
      name: "执行",
      runtime: "codex",
      model: "",
      sandbox: "workspace-write",
      rolePrompt: "你是负责独立完成任务的本地 Agent。",
      instruction: "完成任务、验证结果，并按 outcome contract 汇报。",
      transitions: { completed: "$done", need_human: "$pause" },
    },
  ],
};

export const loopTemplate = {
  id: "my_implement_review",
  name: "我的实现与审查 Loop",
  description: "实现者完成改动，审查者检查；有问题就自动回到实现。",
  entryStepId: "implement",
  policy: { maxTransitions: 20, maxConsecutiveFailures: 3, stepTimeoutSeconds: 1800 },
  steps: [
    {
      id: "implement",
      name: "实现",
      runtime: "codex",
      model: "",
      sandbox: "workspace-write",
      rolePrompt: "你是负责落地代码和测试的实现者。",
      instruction: "实现目标、运行验证，并把变更交给审查者。",
      transitions: { ready_for_review: "review", need_human: "$pause" },
    },
    {
      id: "review",
      name: "审查",
      runtime: "claude",
      model: "",
      sandbox: "read-only",
      rolePrompt: "你是独立、严格的代码审查者。",
      instruction: "检查需求、实现、风险和测试；有问题要求修改。",
      transitions: { changes_requested: "implement", approved: "$done", need_human: "$pause" },
    },
  ],
};

export const dagTemplate = {
  id: "my_parallel_review",
  name: "我的并行审查 DAG",
  description: "安全和测试并行分析，全部完成后汇总落地。",
  mode: "dag",
  entryStepId: "security",
  policy: { maxTransitions: 20, maxConsecutiveFailures: 3, stepTimeoutSeconds: 1800 },
  layout: { nodes: { security: { x: 64, y: 88 }, tests: { x: 64, y: 336 }, synthesis: { x: 412, y: 212 } } },
  steps: [
    { id: "security", name: "安全审查", runtime: "codex", workerId: "local", sandbox: "read-only", dependsOn: [], rolePrompt: "你是安全审查 Agent。", instruction: "独立检查安全风险。", transitions: { completed: "$done", need_human: "$pause" } },
    { id: "tests", name: "测试审查", runtime: "claude", workerId: "local", sandbox: "read-only", dependsOn: [], rolePrompt: "你是测试可靠性审查 Agent。", instruction: "独立检查测试和回归风险。", transitions: { completed: "$done", need_human: "$pause" } },
    { id: "synthesis", name: "汇总落地", runtime: "codex", workerId: "local", sandbox: "workspace-write", dependsOn: ["security", "tests"], rolePrompt: "你负责汇总并落地修改。", instruction: "结合依赖结果实施修改并验证。", transitions: { completed: "$done", need_human: "$pause" } },
  ],
};
