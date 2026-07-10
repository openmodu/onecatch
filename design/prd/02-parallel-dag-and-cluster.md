# PRD 02: 并行 DAG、节点画布与多机 Worker

状态：当前开发阶段

## 1. 背景

PRD 01 已交付串行、可成环的本地 Agent Workflow。下一阶段需要把可并行的分析、审查和验证分散到多个节点，允许用户用画布直接组织依赖关系，并把部分节点派发到另一台已准备相同项目 clone 的机器。

## 2. 目标

- Workflow 可选择 `serial` 或 `dag` 模式，旧定义默认保持 `serial`。
- DAG 使用显式依赖边表达 fan-out、并行执行和 all-dependencies join。
- 用户能在画布中拖动节点、连接依赖边、选择 runtime/worker 并保存布局。
- 本地调度器并发执行所有 ready 节点，统一持久化节点状态、outcome 和事件。
- 用户可注册远端 worker，检查健康状态，并把指定节点派发到对应机器。
- 应用中断后，running DAG 恢复为 paused；已完成节点不重复执行。

## 3. 非目标

- DAG 模式首版不允许环；循环继续使用 `serial` 模式。
- 不提供项目文件同步、自动 git push/pull、自动合并或冲突解决。
- 不把明文 HTTP 暴露到公网；远端 worker 仅用于受信任 LAN/VPN。
- 不实现任意脚本节点、动态扩图、条件表达式和跨用户权限系统。

## 4. DAG 语义

- `dependsOn` 是节点的静态前置集合；集合为空的节点是 root。
- 所有依赖节点 completed 后，当前节点进入 ready。
- 多个 ready 节点可并行执行。
- 每个 DAG 节点只执行一次；Agent 必须返回当前节点声明的终点 signal。
- 任一节点返回 `$pause`、失败达到上限或 runtime/worker 不可用时，Run 进入 paused。
- 所有节点 completed 后 Run completed。
- DAG 校验拒绝未知依赖、自依赖、重复依赖和环。

## 5. Workspace 并发规则

- 同一 Task 仍引用一个逻辑 Workspace ID。
- 没有依赖关系、可能同时 ready 的节点若都请求写权限，定义校验失败。
- `read-only` 节点可在本机或不同 worker 并发。
- `workspace-write/full` 节点获取 Workspace lock，并通过依赖关系串行。
- 首版远端节点只允许 `read-only`；写节点必须在 coordinator 本机执行，避免没有文件同步时产生不可见改动。
- 远端 worker 自己维护 Workspace ID 到本机目录的映射；Oneshot 不复制文件。

## 6. 多机 Worker

- 新增 headless `oneshot-worker`，提供 health 和 execute HTTP API。
- Coordinator 保存 worker ID、名称、base URL、token 和 enabled 状态到 `~/.oneshot/workers.json`，文件权限为 `0600`。
- 请求使用 `Authorization: Bearer`；token 不通过列表接口回传。
- 节点的 `workerId` 为空或 `local` 时本机执行，否则发到指定 worker。
- Worker 根据 Workspace ID 查找本地映射，使用现有 agentrun adapters 执行一次 Agent turn，并返回 result 与规范化 events。
- 网络错误、鉴权错误、Workspace 未映射或 runtime 不可用都暂停 Run，不静默改派。

## 7. 画布

- 节点可拖动，位置保存在 Workflow definition 的 layout 中。
- 从节点连接点拖到另一节点生成 `dependsOn`；可选择边并删除。
- 右侧 inspector 编辑节点名称、runtime、worker、sandbox、角色、指令和终点 signals。
- 画布显示 root、ready、running、completed、paused、failed 状态。
- 提供自动布局和缩放；首版不引入重型图编辑依赖。

## 8. 数据对象

- WorkflowDefinition：新增 `mode`、`layout`。
- WorkflowStep：新增 `dependsOn`、`workerId`。
- WorkflowRun：新增 `nodes: map[stepId]NodeState`。
- Worker：`id/name/baseUrl/enabled/createdAt/updatedAt`；token 仅在写入 DTO 出现。
- RemoteExecuteRequest：workspace ID、runtime、model、sandbox、prompt、resume session。
- RemoteExecuteResponse：agentrun result 与 events。

## 9. 验收标准

- [x] 两个无依赖 read-only 节点真实并行执行，join 节点只在二者完成后执行。
- [x] DAG 环、未知依赖和潜在并行写冲突会被明确拒绝。
- [x] 画布可拖动节点、创建/删除依赖边并保存后恢复位置。
- [x] Run inspector 显示每个 DAG 节点状态和并发进度。
- [x] 可注册远端 worker、健康检查，并把节点派发到远端 stub/真实 worker。
- [x] token 不在 worker 列表或普通日志中返回。
- [x] 远端不可用不会回退本机，而是暂停并允许恢复。
- [x] 串行 Workflow 和历史 Run 保持兼容。

## 10. 待后续

- TLS/mTLS 和设备配对。
- Git clone/worktree 同步、产物传输和自动合并。
- 条件 DAG、any-of join、子流程和动态 fan-out。
- Worker 容量、队列、公平调度和断点续传。
