# Issue 01-006: 本地 Runtime 与 Workflow 编辑器正确性加固

状态：已完成

## 来源

- PRD：`design/prd/01-custom-agent-workflows.md`
- 技术方案：`design/docs/01-custom-agent-workflows.md`

## 目标

清理桌面端已经不可达的旧市场 API 入口，并修复 Agent 超长输出与 Workflow 节点重复 ID 两个实际执行路径上的正确性问题。

## 范围

- 删除桌面端未注册、未构造的 Auth、Billing、Agent、Order、Conversation、Artifact bindings 及旧 HTTP client。
- Agent JSONL 单行超过解析上限时继续排空 stdout，确保子进程可以退出，并返回明确的流读取错误。
- 串行步骤和 DAG 节点新增时从现有 ID 集合分配未占用的稳定 ID，删除后再新增不得冲突。
- 为 runtime 流排空和 ID 分配补充回归测试。

## 非目标

- 不删除仍供 server 或历史测试使用的 `clients/oneshot`、订单、计费等非桌面代码。
- 不修改 DAG 并发调度语义、事件持久化策略或 Runtime 检测性能。
- 不改变已注册的 Runtime、Settings、Workspace、Workflow、TaskRun、Worker Wails bindings。

## 产品需求

- 当前 local-first 桌面构建中不再携带可被误注册的旧市场 bindings。
- 异常大的 Agent 单行输出不能令本地 Run 永久挂起；UI 应收到可诊断错误。
- 用户删除中间步骤或节点后再次新增，已有步骤 ID、画布选中态和依赖连线保持不变。

## 技术设计

- 后端改动：`streamProcess` 在 scanner 出错后同步排空剩余 stdout，再等待子进程；读取错误作为本次执行错误返回。
- 桌面端 / Wails 改动：删除从未加入 `application.Options.Services` 的六类旧 binding 和 `newDesktopClient`。
- 前端改动：抽取纯函数 `nextWorkflowItemID`，按现有 `<prefix>_<number>` 最大序号递增，并对任意已有 ID 做占用检查。
- 数据模型改动：无。
- API 或 binding 改动：已注册 binding 无变化；仅移除不可达类型。
- 错误码：不新增跨层错误码；runtime stream 读取失败沿现有执行失败路径携带 `read stream` 原因。
- 前后端联调点：Workflow 保存 payload 结构不变，只保证新增项 ID 唯一。

## 验收标准

- [x] 桌面目录中不再存在六类旧市场 binding 和旧 HTTP client，Wails 构建仍通过。
- [x] 超过 8 MiB 的单行输出不会阻塞测试进程，并返回 stdout 读取错误。
- [x] 删除 `step_2` 或 `node_2` 后新增不会与仍存在的更高序号 ID 冲突。
- [x] Go 测试、前端测试和生产构建通过。

## 测试计划

- Go：为 `streamProcess` 增加超长单行回归测试并运行 `go test ./...`。
- 前端：使用 Node 内置测试运行器覆盖串行步骤、DAG 节点和非标准 ID。
- 集成：运行前端 production build 与 Wails DEV build。

## 交付记录

- 删除 `desktop/oneshot/bindings` 下六类旧市场 binding 及 `desktop/oneshot/client.go`，并同步改正桌面 README；已注册的六个本地 Wails service 保持不变。
- `streamProcess` 在 scanner 错误后通过 `io.Copy(io.Discard, stdout)` 排空管道，再等待进程并返回明确错误；9 MiB 单行 helper-process 回归测试在超时前正常结束。
- 前端抽取 `nextWorkflowItemID`，串行步骤和 DAG 节点都按已有最大安全数字后缀递增；使用 Node 内置测试运行器覆盖删除后新增与非标准 ID。
- 验证：`go test ./...`、`npm test`、`npm run build`、`wails3 build DEV=true`、`git diff --check` 全部通过；Wails 仅输出既有的 macOS link target 版本警告。
