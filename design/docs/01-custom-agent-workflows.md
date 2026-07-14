# PRD 01 本地 Agent Workflow 技术方案

## 1. 设计结论

新产品主线不依赖原有市场、登录、计费、订单、HTTP Server 或 MySQL。Wails 桌面进程直接组装本地能力：

```text
Wails binding -> local application service -> workflow usecase -> workflow repo -> JSON/JSONL file store
                                           |-> runtime registry
                                           |-> agentrun engine
                                           |-> workspace lock / git inspector
```

Workflow 采用“信号驱动的有向步骤图”：步骤定义可接受的 outcome signal，转移表决定下一个步骤或终点，回边自然形成 loop。`internal/agentrun` 继续只负责一次 CLI 调用和 runtime event 归一化，不读取 workflow 定义，不决定下一步骤。

旧 `00-*` 代码和文档作为历史实现保留，但 PRD 01 的实现不要求兼容 Order、Billing、Auth、Artifact 或远端 API。

桌面可执行文件只保留 `Runtime`、`Settings`、`Workspace`、`Workflow`、`TaskRun`、`Worker` 六类 Wails service。旧市场 bindings 与指向 `oneshot-server` 的桌面 HTTP client 不作为“历史兼容层”编译进 local-first 桌面；历史领域代码可留在各自 server/usecase 包中，但不得从桌面入口重新注册。

## 2. 相比 Buddy 的调整

保留：

- 最终响应使用结构化协议，不从自然语言猜测完成状态。
- 每个角色/session 独立续跑。
- 自动推进有硬上限和人工暂停入口。
- 原始 runtime event 与高层 workflow event 分开记录。
- 失败、打断和重启恢复是显式状态。

调整：

- 不使用 `RUNNING_CLAUDE/RUNNING_CODEX` 等 runtime 特定状态。
- 不写死 implementer/reviewer、Actor 数量和交替顺序。
- 不把双 `break` 作为唯一完成规则，改用工作流自己的 signals 和 targets。
- 不强制每轮 countdown；需要检查的步骤显式转到 `$pause`，后续可增加可选 checkpoint policy。
- 配置在 Run 启动时复制完整 Workflow 与 Task 权限快照，不使用一个全局流程控制全部任务。

## 3. 本地目录与进程边界

默认应用数据目录固定为用户主目录下的 `~/.oneshot/`：

```text
~/.oneshot/
  workspaces/<workspace-id>.json
  tasks/<task-id>.json
  workflows/<workflow-id>/workflow.json
  runs/<run-id>/run.json
  runs/index.json
  runs/<run-id>/workflow.json
  runs/<run-id>/SUMMARY.md
  runs/<run-id>/events.jsonl
  runs/<run-id>/steps/<step-run-id>/state.json
  runs/<run-id>/steps/<step-run-id>/events.jsonl
  locks/<workspace-key>.lock
  logs/oneshot-desktop.log
```

- JSON 保存结构化对象和运行快照，使用同目录临时文件、`fsync`、`rename` 原子替换。
- JSONL 追加保存 workflow/runtime events；序号从已有文件恢复。
- Workspace 文件永远留在原目录，应用数据目录不复制项目。
- Workspace lock 覆盖 Agent 子进程生命周期；包含 PID、run ID、路径 hash 和 startedAt。
- 启动时发现锁 PID 不存在则标记对应 run 为 interrupted，并清理陈旧锁。

### 3.1 大列表查询

- Workspace JSON 增加可选 `pinned/hidden`；选择 Workspace 时通过应用服务更新 `lastOpenedAt`，列表按 pinned、lastOpenedAt、name 排序。前端默认展示置顶项、最近项和当前项，搜索/“全部”状态再读取同一份完整 Workspace 列表，不引入第二套持久化文件。“从列表移除”只写入 `hidden=true`，历史详情仍可通过原 Workspace ID 读取；重新加入同一路径会解除隐藏。
- Run repo 新增派生的 `runs/index.json` 和 `ListRuns(RunListQuery)`：保存/更新 Run 时同步维护 ID、Task、Workflow、状态、session 与 updatedAt 摘要；查询先在索引中完成 Workspace TaskID 范围、状态、keyword 与 cursor 过滤，只读取当前页的完整 `run.json`。索引缺失、损坏或目录集合变化时从权威 Run 快照自动重建；旧 `ListRunsByTask` 保留给恢复流程和兼容调用。
- Wails `TaskRunBinding.ListRuns` 输入 `workspaceId/status/keyword/cursor/limit`，应用服务先读取当前 Workspace 的 Task 摘要，再把允许的 Task IDs 与标题匹配结果传给 Run repo。返回 `items/nextCursor/total`，每个 item 同时包含 Run 与 Task，桌面端不再按 Task 发起 N+1 Run 查询。
- cursor 由最后一条记录的 `updatedAt + runID` 编码，排序固定为 `updatedAt desc, runID asc`；默认 50、最大 200。非法 cursor 返回 `run_query_invalid_cursor`。
- 前端以 50 条为一页增量加载，状态/keyword/Workspace 变化会重置 cursor 并清空不再匹配的 Inspector；可见行采用固定高度窗口化渲染，滚动接近底部时加载下一页。

Issue 01-002 不引入数据库。测试可以传入临时 root；生产代码 root 为空时必须解析为 `~/.oneshot/`。列表查询通过枚举目录和读取快照实现；MVP 数据规模以本地个人任务为边界。

## 4. 领域模型

### 4.1 Definition

```go
type Definition struct {
    ID          string
    Name        string
    EntryStepID string
    Steps       []Step
    Policy      Policy
}

type Step struct {
    ID          string
    Name        string
    Runtime     string
    Model       string
    Sandbox     string
    RolePrompt  string
    Instruction string
    Transitions map[string]string
}

type Policy struct {
    MaxTransitions         int
    MaxConsecutiveFailures int
    StepTimeoutSeconds     int
}
```

默认值为 `maxTransitions=20`、`maxConsecutiveFailures=3`、`stepTimeoutSeconds=1800`。普通 target 必须引用步骤 ID；保留 target 只有 `$done`、`$pause`、`$fail`。

### 4.2 Run

```go
type Run struct {
    ID                  string
    WorkflowID          string
    Status              RunStatus
    CurrentStepID       string
    TransitionCount     int
    ConsecutiveFailures int
    Sessions            map[string]string
    History             []TransitionRecord
    StartedAt           time.Time
    UpdatedAt           time.Time
    CompletedAt         time.Time
    LastError           string
}
```

状态集合为 `ready/running/paused/completed/failed/cancelled`。状态不包含 runtime 名称；当前 runtime 由当前步骤解析。

## 5. 定义校验

保存和启动前执行同一套校验：

1. ID、名称、入口和步骤非空。
2. 步骤 ID 与 signal 使用稳定的小写标识符格式。
3. 步骤 ID 唯一；每一步至少有一个转移。
4. 步骤必须配置 runtime、role prompt 和 instruction。
5. 普通 target 存在；保留 target 合法。
6. 从 entry 做图遍历，拒绝不可达步骤。
7. 至少存在一条从 entry 可达的 `$done` 边。
8. policy 在安全范围内；缺失值先补默认值。

允许有环。无限循环风险由最大转移次数兜底，不尝试用静态分析证明所有路径终止。

## 6. Outcome 协议

Prompt 尾部追加当前步骤专属 contract：

```json
{"signal":"<one of current step signals>","content":"summary for the next step and human"}
```

规则：

- 接受整个 final message 为单个 JSON 对象。
- 兼容单个 `json` fenced block。
- 不接受 JSON 前后额外自然语言、多个对象、空 signal 或未知字段。
- parser 只解析 signal/content；target 永远由本地状态机的当前步骤转移表解析。
- protocol error 记为 StepRun failure，不自动猜测 signal。

## 7. 状态转移

| 当前状态 | 事件 | 下一状态 | 说明 |
|---|---|---|---|
| `ready` | start | `running` | currentStep=entry |
| `running` | 普通 target | `running` | currentStep=target，transitionCount+1 |
| `running` | `$done` | `completed` | 记录 completedAt，释放 workspace lock |
| `running` | `$pause` | `paused` | 保留 currentStep，释放 workspace lock |
| `running` | `$fail` | `failed` | 保存 outcome content，释放 workspace lock |
| `running` | runtime/protocol failure | `running` 或 `paused` | 连续失败达到上限时暂停 |
| `running` | user interrupt | `paused` | 中断子进程并记录 interrupted |
| `paused` | resume | `running` | 默认重跑 currentStep |
| 非终态 | cancel | `cancelled` | 释放锁，不删除历史 |

## 7.1 Runtime 输出流异常

Codex 与 Claude Code 的 JSONL 解析保留单行大小上限，避免异常 runtime 输出导致无界内存增长。scanner 因超长行等读取错误停止时，runner 必须继续读取并丢弃 stdout 直到 EOF，再调用 `cmd.Wait()`；否则子进程可能因 pipe 写满而无法退出。排空后本次 StepRun 按 runtime stream error 失败，不使用不完整的 parser 结果推进 Workflow。

## 7.2 Workflow 编辑器 ID 分配

新增串行步骤和 DAG 节点不得用 `steps.length + 1` 直接生成 ID。客户端从当前定义的全部 ID 中计算相应前缀的最大数字后缀并递增，同时执行占用检查；因此删除中间项后新增不会与仍存在的高序号项冲突。保存时的领域唯一性校验继续作为最终防线。

每次合法 outcome 都追加 TransitionRecord。未知 signal 返回领域错误并保持原 Run 不变。达到 `maxTransitions` 时本次 outcome 仍记录、currentStep 更新为目标，但 Run 进入 paused，不再启动下一步骤。

## 8. 本地持久化（Issue 01-002）

### 8.1 Repo 分层

- `internal/repo/workflows`：Workflow definition、Run、StepRun、event index。
- `internal/repo/tasks`：Workspace 与 Task。
- `pkg/localfile`：通用原子 JSON 写入、安全 ID 和目录同步，不包含业务语义。
- `internal/data/local`：解析 `~/.oneshot/`、创建目录并聚合本地 repos。
- repo 接收基础资源句柄，不 import `internal/data`。

### 8.2 文件布局与一致性

- Workspace、Task 各使用一个 JSON 快照。
- Workflow 使用一个可原子更新的 `workflow.json` 当前模板。
- Run 启动时把当前模板复制为自己的 `workflow.json`；`run.json` 保存 Task、当前位置、sessions、history 和 revision。
- StepRun 使用独立 `state.json`；同一步骤多次执行使用不同 step-run ID。
- 高层 workflow events 与 runtime events 分别使用追加式 JSONL。

runtime event 以 `{seq, at, payload}` JSONL envelope 写入 `runs/<run-id>/steps/<step-run-id>/events.jsonl`。文件追加在进程内按 stream 串行化并 `fsync`；首次追加从已有文件恢复最后 seq。读取接口支持 `afterSeq` 和 `limit`，损坏行返回明确错误，不静默跳过。

模板更新原子替换 `workflows/<id>/workflow.json`，不修改任何已有 Run 快照。Run 更新在 repo 临界区内先读取当前 revision，匹配后原子替换 `run.json`；不匹配映射为 `workflow_state_conflict`。跨进程运行互斥由 01-003 的 Workspace lock 保证。

## 9. Orchestrator（Issue 01-003）

新增 `internal/usecase/workflows`，只依赖以下接口：

- `Repository`：定义、Run、StepRun 和 event 的读写。
- `Engine`：执行 `agentrun.Request`。
- `RuntimeRegistry`：可用性、版本和默认 binary 配置。
- `WorkspaceLocker`：获取、校验和释放 Workspace 写锁。
- `GitInspector`：运行前后 status/diff 摘要，只读访问 git。

每次调度流程：

1. 原子 claim 一个 running Run，读取该 Run 自己的 Workflow 快照与当前步骤。
2. 检查 runtime 可用性；sandbox 取 Task 授权上限与步骤请求的较低权限。
3. 获取 Workspace lock，记录运行前 git 状态。
4. 构造 prompt：Task 目标、role prompt、step instruction、最近 outcomes、人工补充指令、当前允许 signals 和严格 contract。
5. 使用 `sessions[currentStepID]` 作为 `ResumeSessionID` 调用 agentrun。
6. runtime event 追加到 step JSONL；终态保存 session ID、文件变化和 outcome。
7. 调用领域状态机并条件保存 Run。
8. Run 仍为 running 时调度下一步骤；completed/paused/failed/cancelled 时停止并释放锁。

同一步骤再次进入时 resume 自己的 session。不同步骤即便使用相同 runtime 也不共享 session，避免角色上下文污染。

Runtime 不可用时不沿用旧 worker 的“自动换另一个 runtime”策略，因为这会破坏工作流角色语义；Run 应暂停并提示安装 runtime，或修改当前模板后新建 Run。

## 10. Wails Bindings（Issue 01-004）

桌面启动时直接打开 `~/.oneshot/` 的 local store，创建 runtime registry、workspace lock、git inspector 与 workflow usecase。application service 先调用 `StartTask` 持久化 Run 和 Workflow 快照，再用受控 goroutine 调用 `ExecuteRun`；因此 `StartRun` 会立即返回可轮询的 run ID。每个 active run 保存独立 cancel handle，context 打断落盘为 paused 后才能永久 cancel。

Runtime binary 覆盖写入 `~/.oneshot/runtime.json`，只向前端返回 runtime 名称、可用性和版本，不回传配置路径或环境变量。应用首次启动写入 `single_agent` 和 `implement_review` 两个内置模板，但不覆盖用户已有同 ID 模板。

### 10.1 RuntimeBinding

- `ListRuntimes()`
- `CheckRuntime(runtime)`
- `UpdateRuntimeConfig(input)`

### 10.2 WorkspaceBinding

- `ChooseDirectory()`
- `AddWorkspace(path)`
- `ListWorkspaces()`
- `GetWorkspace(id)`
- `GetWorkspaceStatus(id)`

### 10.3 WorkflowBinding

- `ValidateDefinition(input)`
- `CreateDefinition(input)`
- `ListDefinitions()`
- `GetDefinition(id)`
- `UpdateDefinition(id, input)`

### 10.4 TaskRunBinding

- `CreateTask(input)`
- `ListTasks(workspaceID)`
- `StartRun(taskID)`
- `GetRun(runID)`（返回 Run、Task、Workspace、Workflow 快照、StepRuns 与 events）
- `ListRunsByTask(taskID)`
- `ListRunEvents(runID, afterSeq)`
- `InterruptRun(runID)`
- `ResumeRun(runID, instruction)`
- `CancelRun(runID)`

Bindings 调用本地 application service，不通过 HTTP loopback。所有入参在 usecase/domain 校验，前端不决定状态转移。

### 10.5 稳定错误码

- `workflow_invalid_definition`
- `workflow_unknown_signal`
- `workflow_protocol_error`
- `workflow_state_conflict`
- `workflow_transition_limit`
- `workflow_failure_limit`
- `runtime_unavailable`
- `workspace_not_found`
- `workspace_locked`
- `run_not_found`
- `run_invalid_state`

## 11. 前后端联调点

- 编辑器显示领域层字段级 validation issues，不维护另一套完整图校验器。
- Run 详情返回 currentStepId、runtime、status、计数、pause reason 和 outcome 历史。
- 前端把普通步骤 target 与 `$done/$pause/$fail` 分开显示。
- runtime event 与 workflow event 使用各自 seq 增量刷新。
- 文件变化来自实际 git/status 与 agentrun file event，不依赖 Agent 自述。
- App 启动先检测 runtimes，并扫描遗留的 running Runs：无 live Workspace lock 的 Run 按 interrupted 落为 paused，有其他存活进程持锁的 Run 保持不变；随后确保内置模板存在并加载普通 UI。
- 桌面主入口只注册 Runtime、Workspace、Workflow、TaskRun 四组 binding；旧市场、登录、订单、计费和交付物 binding 不再注册。
- 首版编辑器使用步骤卡片、signal/target 行和只读流程预览表达 loop，不引入画布依赖。
- Run inspector 轮询组合详情；active run 约 1 秒刷新，非 active run 降频刷新。浏览器开发预览在没有 Wails runtime 时使用只读 demo 数据，正式桌面始终调用生成的 bindings。
- Run inspector 按步骤展示当前 Runtime session ID 与人工接管命令。优先读取 `Run.sessions[stepId]`，旧数据回退到最新 StepRun 的 `sessionIdAfter/sessionIdBefore`；同一步骤只显示当前可恢复会话。Codex 使用 `codex resume <session-id>`，Claude Code 使用 `claude --resume <session-id>`，未知 Runtime 只显示 ID、不猜测命令。
- Run inspector 的执行历史采用会话时间线：Task prompt 与非空恢复 instruction 是用户消息；StepRun 是带轮次、步骤、Runtime 和状态的 Agent 回合；message/result/error 作为可读回复，连续的 tool/reasoning/file events 收进默认折叠的活动组。UI 不截断为最后若干事件，outcome JSON 只显示 `content`。`run.resumed` 事件在本地 payload 中保存 trim 后的 instruction，以便重启后仍能重建对话；旧事件缺少该字段时保持兼容。
- Issue 01-010 起，Run inspector 不再把连续 tool/reasoning/file events 汇总为一个活动组。前端 view model 按 runtime event `seq` 生成交错的 message/tool items；每个 tool item 是独立且默认关闭的 disclosure，相邻 `tool_result` 归入前一个 `tool_use` 的展开详情。Run 标题、状态、当前步骤、Workflow 步骤和 runtime sessions 合并为一个紧凑摘要，流程与恢复命令放在同一个默认折叠的详情区。
- 工具摘要优先从常见的 `zsh -lc` launcher 中提取首条可读命令，完整原始事件仍保留在展开详情；用户与 Agent 消息使用独立身份标识，工具项通过同一组 token 呈现 hover、键盘 focus、open 和 running 状态。
- 方案 2 的最终结构不再渲染 StepRun “第 N 轮”头部，而是让各轮内部事件直接连续排列；summary 采用标题/状态/计数与当前步骤/会话恢复两行布局。工具 disclosure 使用 `@phosphor-icons/react` caret，闭合行固定为展开入口、可读动作、状态和时间四列。
- 事件类型的可见标识采用 icon-first 规则：用户消息为白色 `Circle`（通过 `--ui-event-user` 与 token 描边在深浅主题保持可见），Agent 消息为 cyan `Circle`，工具/文件/过程项不再显示 kind 文本并沿用 disclosure caret。`aria-label` 继续包含用户、Agent 和具体 runtime event kind，避免纯视觉标记损失辅助技术语义。
- Conversation message header 为左右布局：左侧仅保留圆点与 runtime，右侧固定为轮次元数据和贴近内容边缘的 55px 时间列。Agent 的 `第 N 轮` 直接使用 StepRun 在 timeline 中的 `round`，同一 StepRun 内多条 message 共享轮次；用户消息不虚构轮次。初始 task message 的时间依次读取 task.createdAt、run.startedAt、task.updatedAt、run.updatedAt。
- Tool caret 位于 summary 首列，闭合显示 `CaretRight`、展开显示 `CaretDown`；summary 列为 `var(--ui-event-marker-size) / minmax(0, 1fr) / 45px / 55px`。圆点和 caret 共用 9px marker 尺寸与正文前景 token，因此左边缘、中心线和主题对比保持一致；所有时间在内容区右边缘对齐。
- Workspace 切换是任务列表与 Inspector 的一致性边界。前端在选择新 Workspace 时立即清空 `tasks`、`runs`、`selectedRunID`、`runDetail` 和未提交的恢复指令，避免旧 Run 详情短暂或持续串入新目录；Task/Run 异步加载各使用单调递增的请求版本，晚到的旧 Workspace 响应必须丢弃。新运行列表落地后通过 `runPairsContain` 再校验已选 Run，只有仍属于当前列表时才允许保留详情。
- Run 恢复保持 application service 的异步 dispatch 语义。前端收到 `ResumeRun` 已受理后，将匹配的 Inspector 乐观标记为 active，并用 Run ID 记录 pending resume；按钮显示“恢复中”且保持禁用，active 轮询负责确认持久化快照从 paused 推进。Run 仍在中断收尾时显示“等待停止”，同样不允许提交恢复。pending 在 Run 离开 paused、active 结束、选择发生变化或请求报错时清理；运行控制使用同步 ref 屏蔽重绘前双击，延迟刷新必须再次校验当前 selected Run ID，不能把旧 Inspector 带回当前 Workspace。

### 11.1 Modu Code ACP Runtime（Issue 01-009）

Modu Code 使用 JSON-RPC 2.0 LDJSON over stdio，不通过 shell 拼接任务参数。`ModuRunner` 为每次 StepRun 启动一个受 context 管理的子进程，依次发送：

1. `initialize`，确认 ACP protocol version。
2. `session/new`，以当前 Workspace 作为 `cwd`。
3. `session/prompt`，prompt 只包含一个 `type=text` content block。
4. 读取 `session/update` 的 `agent_message_chunk`，聚合为一个 `message` 与最终 `result` 事件。

Runtime 设置新增可选 `provider`，仅 `modu` 接受 `auto/openai/anthropic/gemini`。非 auto 值在启动时写入 `MODU_CODE_PROVIDER`；默认模型写入 `MODU_CODE_MODEL`。API Key 和 `OPENAI_BASE_URL` 仍只通过已有 environment allowlist 从宿主继承，设置文件不保存 secret 值。binary 留空解析 `modu-code`。

未配置 binary 时优先解析当前 Modu Code 命令 `modu_code`，并以 `--acp` 启动；若当前命令不存在，再回退到旧版 `modu-code`。`HOME` 属于基础继承环境，因此当前命令会直接读取 `~/.modu/config.toml`，保持与终端默认 Provider、模型和凭据一致，不要求用户在 Oneshot 重复配置 API Key。显式 environment allowlist 仅用于额外的环境变量覆盖。若子进程在 ACP 响应前异常退出，进程退出状态与 stderr 是主错误；`unexpected EOF` 仅视为伴随症状，不展示在主错误前。

当前已验证的 Modu Code 版本只在进程内维护 `sessions`，没有 `session/load`，且每次进程都会从 `modu-sess-1` 重新编号。因此 Oneshot 不持久化或展示该短生命周期 ID，不生成恢复命令；Loop 再入和人工恢复会启动新 session，并使用 Orchestrator 已有的 Task、最近 outcomes 与人工补充 prompt 衔接。未来 Modu Code 声明持久 session capability 后，再单独增加 capability negotiation 与恢复语义。

首版消费 Modu Code 当前实现的 agent text chunk，并处理工具权限反向请求。若 Agent 发出其他未支持的 ACP 反向请求，runner 返回明确 protocol error。配置检查只验证 binary 可执行，不启动 provider，因此不消耗模型额度。

### 11.2 Modu ACP 权限反向请求（Issue 01-015）

Runner 按 Sandbox 构造 Modu 参数：`workspace-write/full` 使用 `modu_code --acp --no-approve`，因为 Workflow 启动本身已经完成授权；`read-only` 只使用 `--acp`，保留 Modu 工具审批。若当前或兼容版本仍发送 `session/request_permission` 反向 JSON-RPC，Runner 在等待 `session/prompt` 响应的同一个扫描循环中处理该请求，并使用相同 ID 写回 `{outcome:{optionId}}`；响应后继续读取原 prompt 的 message chunk 与最终结果。

- `read-only`：选择 `reject_once`，Modu 自身判定无需审批的只读工具仍可运行。
- `workspace-write` / `full`：正常由 `--no-approve` 避免交互；若仍收到反向请求则选择 `allow_once` 兼容，不使用 `allow_always`。
- 请求到达时追加 `tool_use`，响应后追加 `tool_result`，二者保留原始参数/响应供本地诊断。
- 未知反向方法、缺少匹配选项或非法参数继续作为明确 protocol error 失败，不做宽松兜底。

ACP permission 是工具审批协议，不是操作系统级进程隔离；Modu 的 `workspace-write` 首版依赖 Workflow 授权、任务 prompt 和 Modu 自身权限规则约束，后续若要求强目录边界需单独接入系统 sandbox。

## 12. 验证计划

- 领域：图校验、协议 parser、回边、所有终点、未知 signal、上限和 resume。
- Repo：模板原子更新、Run Workflow 快照隔离、revision 冲突、重启水合和 JSONL seq 恢复。
- Orchestrator：stub engine 跑通 review loop、逐步骤 session resume、协议错误、runtime 失败、workspace lock 和取消竞态。
- Bindings：DTO、稳定错误、run 控制状态边界。
- Desktop：模板创建、回边显示、运行暂停/恢复、diff 和危险 sandbox 确认。
- 回归：`go test ./... -race`、`go vet ./...`、前端 build、Codex/Claude 本地冒烟。
