# Issue 01-003: Workflow Agent 编排执行器

状态：已完成

## 来源

- PRD：`design/prd/01-custom-agent-workflows.md`
- 技术方案：`design/docs/01-custom-agent-workflows.md`
- 前置 issue：`design/issues/01-001-workflow-state-machine-core.md`、`design/issues/01-002-workflow-persistence.md`

## 目标

把 workflow 状态机接到现有 `internal/agentrun`，在同一工作目录中按步骤自动调用 Agent、解析 outcome、保存独立 session 并推进 loop。

## 范围

- workflow usecase 解析当前步骤并构造带 outcome 协议的 prompt。
- 解析 `agentrun.Result.FinalMessage`，持久化 step run 和 workflow events。
- 按 step ID 保存/resume session。
- 支持 context 打断后暂停、跨应用重启恢复和人工补充指令；主动 cancel handle 与持久取消操作由 01-004 封装。
- 获取 Workspace 写锁，记录运行前后 git status/diff 和真实文件变化。
- `$done` 后生成本地运行摘要，不创建订单交付物。
- 本 issue 提供同步 `ExecuteTask/ResumeRun` 用例；异步调度、UI 轮询和主动 cancel handle 在 01-004 application service/binding 中封装。

## 非目标

- 不实现并行步骤。
- 不实现可视化编辑器。

## 产品需求

- 每次只运行当前步骤。
- 未知 signal 和协议错误不猜测转移。
- 达到保护上限停止自动调用 runtime。

## 技术设计

- 后端改动：新增本地 workflow orchestrator usecase，组合 repo、runtime registry、agentrun、workspace lock 和 git inspector。
- 桌面端 / Wails 改动：无。
- 前端改动：无。
- 数据模型改动：消费 01-002 模型。
- API 或 binding 改动：由 `design/issues/01-004-local-workflow-bindings.md` 提供。

## 验收标准

- [x] stub runtime 可跑通 implement → review → implement → review → done。
- [x] 再次进入同一步骤时传入该步骤 session ID。
- [x] 暂停/恢复与应用重启不会重复执行已完成步骤。
- [x] 单 Agent 模板可独立运行，不依赖旧订单/HTTP 服务。

## 测试计划

- orchestrator：真实纯文件 repo + stub engine 集成测试，覆盖四步 review loop、每步骤 session resume、sandbox 降权、prompt handoff、runtime JSONL 和 SUMMARY。
- 恢复：关闭并重新打开 local store 后恢复 `$pause` Run，复用 session、注入人工指令并从 attempt 2 继续。
- 异常：未知 signal 记录为失败并重试；runtime 不可用直接暂停且不回退；Workspace live/stale lock 和 32 goroutine 文件追加 race 覆盖。
- git：真实临时 git repo 覆盖 repo 检测和工作区 status。
- Codex/Claude 实机调用未在本 issue 自动触发，避免未经确认消耗本地 CLI 账号额度；现有 `internal/agentrun` adapter 回归全过，实机冒烟留给 01-004 桌面启动入口联调。

## 交付记录

- 新增 `internal/usecase/workflows`：同步 `ExecuteTask/ResumeRun`，从 Run 自有 Workflow 快照选择步骤，构造严格 outcome prompt，串行调用 agentrun 并持久推进。
- 新增 `internal/workspacelock`：`O_EXCL` 运行锁、PID/run metadata、live owner 拒绝、stale owner 清理和 owner-safe release。
- 新增 `internal/gitinspect`：只读采集 HEAD、porcelain status 和 diff stat；非 git Workspace 正常降级。
- workflow state machine 新增显式 `Pause`；context cancel、runtime unavailable、失败上限和 workflow `$pause` 都落成可恢复状态。
- 每个 StepRun 保存独立 session before/after；同一步骤回环时 resume 自己的 session，不跨角色共享。
- `$done` 生成 `runs/<run-id>/SUMMARY.md`，包含 Workflow、终态、转移数和全部 outcomes。
- 验证结果：`go test ./...`、`go vet ./...`、`go test -race ./pkg/localfile ./internal/data/local ./internal/repo/tasks ./internal/repo/workflows ./internal/domain/workflows ./internal/workspacelock ./internal/gitinspect ./internal/usecase/workflows` 全部通过。
