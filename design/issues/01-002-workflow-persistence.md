# Issue 01-002: Workflow 定义与运行持久化

状态：已完成

## 来源

- PRD：`design/prd/01-custom-agent-workflows.md`
- 技术方案：`design/docs/01-custom-agent-workflows.md`
- 参考：外层 `../buddy/src/main/buddy/store.ts`、`locks.ts`、`paths.ts` 的状态快照、JSONL、目录枚举和运行锁分层。

## 目标

把 Workspace、Task、当前 Workflow 模板、Run Workflow 快照、步骤尝试和事件持久化，使桌面应用重启后仍可查询并恢复。

## 范围

- 新增 `internal/repo/workflows`、`internal/repo/tasks` 接口与纯文件实现。
- 默认数据 root 为 `~/.oneshot/`，测试和开发可显式覆盖。
- 结构化状态使用原子 JSON 快照，事件使用追加式 JSONL。
- 保存当前模板、Run 独立 Workflow 快照、run、step run 和 workflow event。
- 对运行推进增加 revision/状态条件更新，防止重复推进。
- 增加 runtime event JSONL store，并预创建后续 Workspace lock 使用的 `locks/` 目录。
- 接入 `internal/data/local` 目录生命周期与 repo 聚合。

## 非目标

- 不调用本地 Agent。
- 不提供桌面编辑器。

## 产品需求

- 已启动 Run 始终读取启动时复制到 Run 目录的 Workflow 快照。
- 重启后步骤历史、session 和当前位置不丢失。
- 同一 run 的并发推进只能有一个成功。

## 技术设计

- 后端改动：新增 `pkg/localfile`、`internal/data/local`、本地 workflow/task 纯文件 repo。
- 桌面端 / Wails改动：无。
- 前端改动：无。
- 数据模型改动：新增 Workspace/Task/Workflow/Run/StepRun/Event 文件格式。
- API 或 binding 改动：无。

## 验收标准

- [x] 当前模板、Run Workflow 快照和 run state 可从 JSON 文件 repo 往返读取。
- [x] 重启水合后 session、当前步骤和历史一致。
- [x] 并发条件更新不会重复提交同一转移。
- [x] runtime JSONL 事件可按序号增量读取。

## 测试计划

- 已完成本地目录权限、纯文件布局断言、Workspace/Task 重启恢复测试，并确认不创建 `oneshot.db`。
- 已完成模板更新、Run 快照隔离、Run revision 冲突、session/history 水合、StepRun 和 workflow event 测试。
- 已完成 JSONL 首次写入、重启后 seq 恢复、afterSeq/limit 增量读取、崩溃残留半行恢复、路径穿越拒绝、完整损坏行拒绝和 32 goroutine 并发追加测试。

## 交付记录

- 新增 `pkg/localfile`：参考 Buddy 的临时文件 + rename，并增强为 JSON 原子写入、安全路径 ID、文件/目录 `fsync` 和 JSONL 未完成尾行恢复。
- 新增 `internal/data/local`：root 为空时解析为 `~/.oneshot/`，创建 `workspaces/`、`tasks/`、`workflows/`、`runs/`、`locks/`、`logs/`，目录权限为 `0700`。
- 新增 `internal/domain/workspaces`、`internal/domain/tasks`，扩展 workflow Definition/Run，并新增 StepRun、WorkflowEvent、RuntimeEvent。
- 新增 `internal/repo/tasks`：Workspace/Task 保存、详情和列表。
- 新增 `internal/repo/workflows`：当前 Definition 模板、Run Workflow snapshot、Run state/revision 条件更新、StepRun、高层事件和 JSONL runtime event。
- 按产品决策移除 Workflow version：模板固定为 `workflows/<id>/workflow.json`，Run 启动时复制到 `runs/<run-id>/workflow.json`；Definition、Task、Run 不再包含 version 字段。
- 数据布局：纯 JSON/JSONL 文件；`locks/` 的实际进程锁由 01-003 实现。
- 验证结果：`go test ./...`、`go vet ./...`、`go test -race ./pkg/localfile ./internal/data/local ./internal/repo/tasks ./internal/repo/workflows ./internal/domain/workflows` 全部通过。
