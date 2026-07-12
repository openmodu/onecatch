# PRD 01: 本地 Agent 调度编排与自定义 Loop

状态：当前产品主线

## 1. 背景

Oneshot 之前围绕 Agent 服务市场、登录、计费和订单做过探索，但这些能力不再约束当前产品方向。当前产品聚焦一件事：在用户自己的电脑和项目目录中调度多个本地 Agent，减少手工切换 CLI、复制上下文、重复催促审查和判断 loop 是否该继续的成本。

外层 `buddy` 项目验证了“实现者与审查者轮转、结构化结束信号、轮次上限、人工介入”对长程开发任务的价值，但其流程固定为双 Actor 和双 `break` 确认。Oneshot 要进一步把流程本身开放出来：步骤、角色、转移和结束条件都由用户定义，“实现 → 审查 → 返工”只是一个默认模板。

## 2. 产品定位

Oneshot 是一个 local-first 的桌面 Agent workflow runner：

- 工作对象是本地 Workspace，而不是远端订单。
- 核心对象是 Task、Workflow、Run 和 StepRun，而不是用户、计费与交付物。
- Codex、Claude Code 等 CLI 在用户机器上执行，文件直接保留在 Workspace 中。
- Wails 桌面端直接调用本地应用服务；MVP 不依赖远端 HTTP Server、登录或 MySQL。
- 产品价值是把可重复的工作方法固化成可观察、可暂停、可恢复的 Agent loop。

## 3. 目标

- 用户可以选择本地项目目录，输入任务目标并启动 Agent。
- 用户可以创建、校验、保存和复用自己的工作流。
- 工作流由多个步骤和显式转移组成，转移可以回到已执行步骤形成 loop。
- 每个步骤可以选择 runtime、模型、sandbox、角色说明和步骤指令。
- Agent 使用结构化 outcome signal 决定下一条已声明的转移，不能任意跳到未授权步骤。
- loop 支持最大转移次数、连续失败上限、步骤超时、暂停、恢复、终止和人工补充指令。
- 每个步骤保留独立 session/thread ID；应用重启后可恢复。
- 用户能看到当前步骤、历史对话、工具事件、文件变化、错误和下一步原因。

## 4. 非目标

- 不做 Agent 服务市场、登录、支付、扣次和订单生命周期。
- 首版不提供远端团队服务、多人权限和云端同步。
- 首版不提供任意脚本节点、webhook 节点和第三方 SaaS 连接器。
- 首版不提供通用表达式语言、用户代码条件和并行 DAG；只支持串行、有向、可成环的步骤图。
- 首版不允许 Agent 在响应中动态创建步骤或跳转到定义之外的步骤。
- 不兼容 Buddy 的磁盘协议；只复用其已验证的 loop 原则。

## 5. 核心概念

### 5.1 Workspace

用户选择的本地目录。Agent 直接在该目录中读取、修改和验证文件。Oneshot 只记录目录引用、git 状态摘要和运行元数据，不复制整个项目，也不把项目上传到服务端。

同一个 Workspace 默认只允许一个写入型 run 同时执行，避免多个 Agent 流程互相覆盖。

### 5.2 Task

用户想完成的一次具体目标，包含标题、自然语言目标、Workspace、选用的 Workflow 和补充上下文。Task 可以启动多个 Run；重试或从暂停恢复不需要创建“订单”。

### 5.3 Workflow

可复用、可编辑的流程模板，包含：

- 名称、说明、入口步骤。
- 一个或多个 Agent 步骤。
- 每个步骤的 runtime 配置、角色说明、步骤指令和 `signal -> target` 转移表。
- 最大转移次数、最大连续失败次数和步骤超时等保护策略。

转移目标可以是另一个步骤 ID，也可以是保留终点：

- `$done`：任务成功完成。
- `$pause`：暂停，等待人工检查或补充指令。
- `$fail`：任务以业务失败结束。

### 5.4 Outcome 协议

每个步骤的最终文本必须给出结构化结果：

```json
{
  "signal": "ready_for_review",
  "content": "实现完成，已运行单元测试。"
}
```

`signal` 必须存在于当前步骤的转移表中。`content` 用于运行历史、下一步骤上下文和用户判断。模型只提交 signal，服务端状态机根据当前定义解释 target。

### 5.5 Loop 示例

```text
implement --ready_for_review--> review
review --changes_requested----> implement
review --approved-------------> $done
implement/review --need_human--> $pause
```

同一模型还可以表达“调研 → 分析 → 写作 → 质检 → 返工”等三个以上步骤的流程。

## 6. 用户流程

### 6.1 首次运行

1. 用户打开 Oneshot，应用检测本地 Codex、Claude Code 等 runtime 是否可用。
2. 用户选择一个本地 Workspace。
3. 用户输入任务目标，选择“单 Agent”或“实现 → 审查”内置模板。
4. 应用获取 Workspace 写锁并启动入口步骤。
5. UI 实时显示 Agent 消息、工具调用、文件变化和步骤状态。
6. outcome 指向下一步骤时自动推进；到达终点或保护上限时停止。

### 6.2 创建自定义 Workflow

1. 用户复制内置模板或新建工作流。
2. 用户添加步骤，为每个步骤选择 runtime，并填写角色和步骤指令。
3. 用户为步骤声明 outcome signal 和目标步骤/终点。
4. 系统实时校验入口、步骤 ID、转移目标、可达性和保护上限。
5. 校验通过后原子保存当前模板；已经启动的 Run 始终使用启动时复制的完整 Workflow 快照。

### 6.3 自动推进与上下文交接

1. Orchestrator 读取当前步骤并启动对应 runtime。
2. Prompt 包含 Task 目标、步骤指令、允许 signals、必要的最近历史和人工补充指令。
3. Agent 返回 outcome；系统记录 StepRun 并选择已声明的转移。
4. 再次进入同一步骤时 resume 该步骤自己的 session；不同步骤不共享 session。
5. 到达 `$done` 后展示最终摘要、git diff 和文件变化；不把它们包装为订单交付物。

### 6.4 人工介入

1. 用户可在运行中请求打断，应用终止当前子进程并进入暂停态。
2. 用户可在暂停态补充指令，从当前步骤继续，或终止整个 Run。
3. 达到最大转移次数或连续失败上限时自动暂停，不无限消耗 runtime。
4. 首版不强制每轮等待倒计时；需要人工检查的流程显式转到 `$pause`。

## 7. 功能需求

### 7.1 Runtime 管理

- 检测 Codex、Claude Code、Modu Code 的安装状态和版本/协议可用性。
- 允许为 runtime 配置 binary path、默认 model 和环境变量白名单；Modu Code 还允许显式选择 Provider。
- Modu Code 通过 ACP JSON-RPC/LDJSON 接入，不使用可注入的 shell 命令模板；当前 CLI 不支持跨进程 session 恢复时，Oneshot 必须明确按无原生会话恢复能力运行，不得展示无效恢复命令。
- runtime 不可用时不得静默换成另一个角色；应暂停并明确提示。
- CLI 原始事件统一映射为消息、推理、工具、文件变化、用量、结果和错误。

### 7.2 Workflow 定义与校验

- 步骤 ID 和 signal 在同一定义内必须唯一且稳定。
- 入口步骤和所有非终点转移目标必须存在。
- 从入口至少能到达一个 `$done` 转移。
- 不可达步骤应作为校验错误返回。
- 每个步骤必须配置 runtime，并包含非空角色/步骤指令。
- 系统必须为未设置的保护策略补充安全默认值。

### 7.3 执行语义

- 一个 Run 同一时间最多执行一个步骤。
- 一个 Workspace 默认最多存在一个写入型 Run。
- 每次步骤尝试、输出、signal、转移、session ID 和错误必须本地持久化。
- Agent 输出不是合法 outcome、signal 未声明或 runtime 异常时，不得猜测下一步骤。
- 应用意外退出后，运行中的子进程视为已中断；恢复时由用户确认后重跑当前步骤。

### 7.4 安全与资源上限

- 默认最大转移次数为 20，默认最大连续失败次数为 3，默认步骤超时为 30 分钟。
- sandbox 权限不得高于用户为 Task 明确授予的上限。
- 运行时环境变量、原始 stderr 和本地敏感路径在 UI 中按需脱敏。
- 工作目录锁必须包含 run ID 和进程信息，并能清理失效锁。

### 7.5 可观察性与效率

- UI 明确展示当前步骤、runtime、已使用转移次数、暂停原因和下一目标。
- 每个 StepRun 展示结构化 outcome、耗时、session、文件变化和验证结果。
- 用户可以查看整个 Run 的时间线和最近 git diff。
- 提供“单 Agent”“实现 → 审查”内置模板，并允许复制修改。
- Workflow 只保留当前模板；历史执行使用各 Run 自己的 Workflow 快照查看和恢复。

## 8. 数据对象

### 8.1 Workspace

- `id`、`name`、`path`
- `defaultSandbox`
- `createdAt`、`lastOpenedAt`

### 8.2 Task

- `id`、`workspaceId`、`title`、`prompt`
- `workflowId`
- `status`
- `createdAt`、`updatedAt`

### 8.3 WorkflowDefinition

- `id`、`name`、`description`
- `entryStepId`
- `steps[]`、`policy`
- `createdAt`、`updatedAt`

### 8.4 WorkflowStep

- `id`、`name`
- `runtime`、`model`、`sandbox`
- `rolePrompt`、`instruction`
- `transitions: map[signal]target`

### 8.5 WorkflowRun

- `id`、`taskId`、`workflowId`
- `status`、`currentStepId`
- `transitionCount`、`consecutiveFailures`
- `sessions: map[stepId]sessionId`
- `startedAt`、`updatedAt`、`completedAt`

### 8.6 WorkflowStepRun

- `id`、`runId`、`stepId`、`attempt`
- `status`、`signal`、`content`
- `sessionIdBefore`、`sessionIdAfter`
- `startedAt`、`finishedAt`、`error`

## 9. 本地架构边界

- Wails desktop 入口负责组装本地 store、runtime registry、workflow usecase 和 bindings。
- workflow usecase 负责选择步骤、构造 prompt、调用 agentrun、解析 outcome 并推进状态。
- `internal/agentrun` 只负责单次 runtime 调用与事件归一化，不感知工作流图。
- 本地 store 使用可直接检查和备份的 JSON/JSONL 文件，把 Workspace、Task、Workflow、Run 和事件持久化到 `~/.oneshot/`；业务 repo 不依赖生命周期层。
- Wails binding 只暴露 runtime、workspace、workflow、task 和 run 操作，不包含状态转移规则。
- MVP 不要求 HTTP transport、Go SDK、登录或远端 service 参与执行链。

## 10. 验收标准

- [ ] 用户可选择本地 Workspace，并看到 Codex/Claude Code 可用状态。
- [ ] 用户可以用单 Agent 模板完成一个本地任务。
- [ ] 用户可以保存并校验至少包含两个步骤和一个回边的工作流。
- [ ] “实现 → 审查 → 返工”能在审查不通过时回到实现步骤，在通过时进入 `$done`。
- [ ] 非法目标、不可达步骤、无完成路径和未知 signal 会被明确拒绝。
- [ ] 达到最大转移次数或连续失败上限后自动暂停。
- [ ] 每个步骤的 session 独立保存，并在再次进入该步骤时 resume。
- [ ] 应用重启后可查看 Run 历史，并可确认恢复暂停的 Run。
- [ ] 用户可以打断、补充指令、恢复或终止 Run。
- [ ] 用户能查看步骤时间线、文件变化和 git diff。
- [ ] Wails binding、本地数据模型和技术方案一致。

## 11. 待确认问题

- MVP 是否只支持 Codex 与 Claude Code，还是同时接入 OpenCode/Kimi。
- Workflow 编辑器首版采用表单/列表，还是直接做节点画布。
- `$pause` 恢复后是否只允许重跑当前步骤；当前建议首版如此。
- 是否需要在自动推进前提供可选 checkpoint，而不是全局 countdown。
- 后续是否需要 CLI/headless runner、并行分支和云端同步。
