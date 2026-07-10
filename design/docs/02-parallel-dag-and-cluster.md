# PRD 02 并行 DAG 与多机 Worker 技术方案

## 1. 依赖方向

```text
Wails -> localapp -> dag usecase -> workflow/task repos
                         |-> local agentrun engine
                         |-> remote worker client

oneshot-worker HTTP -> worker service -> agentrun engine
```

远端协议和 registry 位于独立业务包，不进入 `internal/data`。Worker server 是 headless 入口，不依赖 Wails。

## 2. 兼容模型

- `Definition.Mode`：空值归一为 `serial`；`dag` 启用依赖校验。
- `Step.DependsOn`：DAG 静态依赖；serial 忽略。
- `Step.WorkerID`：空值归一为 `local`。
- `Definition.Layout.Nodes[stepId]`：画布坐标；不参与调度语义。
- `Run.Nodes`：每节点 durable status、attempt、signal、content、error 和时间。

旧 Workflow/Run JSON 缺失这些字段时仍能读取并按 serial 执行。

## 3. DAG 校验

1. 复用通用 ID、runtime、prompt、policy 校验。
2. 依赖必须存在、唯一且不能指向自己。
3. Kahn topological sort 必须访问全部节点，否则返回 `dag_cycle`。
4. DAG 节点 transitions 只能指向 `$done/$pause/$fail`；步骤间顺序只由 `dependsOn` 决定。
5. 计算无依赖关系的节点对；若两者 sandbox 都可能写入同一 Workspace，返回 `parallel_write_conflict`。
6. 非 local worker 的节点必须为 read-only，否则返回 `remote_write_unsupported`。

## 4. Scheduler

Scheduler 维护一个中心事件循环：从 durable Run 计算 ready 节点，启动 goroutine 执行，并把结果发送回单一 result channel。只有事件循环更新 Run revision，因此并行 goroutine 不直接竞争 Run 快照。

- read-only 节点直接并发。
- write/full 节点执行期间获取 Workspace lock。
- 每个节点创建独立 StepRun，session 仍按 step ID 保存。
- prompt 包含所有直接依赖的 outcome content。
- 任一节点 pause/fail/基础设施错误时取消仍运行节点，统一落为 paused。
- resume 保留 completed nodes，只重试 failed/interrupted/current pending nodes。

## 5. Worker registry

`~/.oneshot/workers.json` 使用原子 JSON 与 `0600` 权限。磁盘对象含 token；返回 UI 的 `WorkerInfo` 不含 token，只含 `hasToken`。

Wails WorkerBinding：

- `ListWorkers()`
- `SaveWorker(input)`
- `DeleteWorker(id)`
- `CheckWorker(id)`

## 6. HTTP 协议

```text
GET  /v1/health
POST /v1/execute
Authorization: Bearer <token>
```

execute 请求携带 workspaceId/runtime/model/sandbox/prompt/resumeSessionId；Worker 从启动参数提供的映射解析路径。响应携带 `agentrun.Result` 与 events。限制 body 1 MiB、设置 server/read/write/idle timeout，不记录 token/prompt。

Worker CLI：

```bash
oneshot-worker --listen 0.0.0.0:9231 --token-env ONESHOT_WORKER_TOKEN \
  --workspace oneshot=/path/to/clone
```

## 7. 画布

React 使用 HTML 绝对定位节点与 SVG path 边。Pointer events 实现拖动；“连接依赖”使用明确的 source/target 两步操作，避免原生 drag/drop 在 WebView 中不稳定。节点坐标归一到画布内容坐标并写回 Layout。

## 8. 错误码

- `dag_cycle`
- `dag_unknown_dependency`
- `parallel_write_conflict`
- `worker_not_found`
- `worker_unavailable`
- `worker_unauthorized`
- `worker_workspace_unmapped`
- `worker_runtime_unavailable`

## 9. 验证

- Domain：DAG 校验、拓扑顺序、写冲突、旧 JSON 兼容。
- Scheduler：并发度、join 顺序、暂停取消、resume 保留 completed。
- Worker：registry、鉴权、health/execute、错误映射。
- Desktop：binding generation、canvas interactions、frontend/Wails build。
