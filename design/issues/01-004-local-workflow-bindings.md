# Issue 01-004: 本地 Workflow 应用服务与 Wails Bindings

状态：已完成

## 来源

- PRD：`design/prd/01-custom-agent-workflows.md`
- 技术方案：`design/docs/01-custom-agent-workflows.md`
- 前置 issue：`design/issues/01-002-workflow-persistence.md`、`design/issues/01-003-workflow-orchestrator.md`

## 目标

让 Wails 桌面端直接访问本地 runtime、Workspace、Workflow、Task 和 Run 能力，不经过旧 HTTP Server、登录或订单接口。

## 范围

- 组合本地 store、runtime registry、orchestrator 和 application service。
- Runtime 检测和本地配置 bindings。
- Workspace 添加、列表和状态 bindings。
- Workflow 校验、创建、列表、详情和更新 bindings。
- Task 创建以及 Run 启动、事件查询、打断、恢复和终止 bindings。
- application service 把同步 orchestrator 包装为后台 goroutine，维护每个 run 的 cancel handle 和最后错误。
- WorkspaceBinding 提供 macOS 原生目录选择器，并保留路径输入作为降级方式。
- 稳定错误码和面向前端的最小 DTO。

## 非目标

- 不提供远端 HTTP API、Go HTTP SDK 或账号鉴权。
- 不实现工作流编辑 UI。

## 产品需求

- 所有写入由本地 usecase/domain 校验。
- Run 控制操作必须校验当前状态和 Workspace lock。
- 环境变量、完整敏感路径和未脱敏 stderr 不直接暴露给前端。

## 技术设计

- 后端改动：新增 local application service 与 provider wiring。
- 桌面端 / Wails 改动：新增 RuntimeBinding、WorkspaceBinding、WorkflowBinding、TaskRunBinding。
- 前端改动：仅接入生成的 binding 类型。
- 数据模型改动：无新增表。
- API 或 binding 改动：见技术方案第 10 节。

## 验收标准

- [x] 桌面进程不启动 HTTP Server 也能创建 Task 并启动 Run。
- [x] bindings 与本地领域 DTO 字段一致。
- [x] Workspace locked、runtime unavailable 和状态冲突返回稳定错误码。

## 测试计划

- application service 单测、binding 边界测试、Wails 本地冒烟。

## 交付记录

- 新增 `internal/app/localapp`，组合纯文件 store、可热更新 runtime registry 和 workflow orchestrator；内置模板只在缺失时创建。
- Run 启动拆成持久化准备与后台执行，application service 管理 cancel handle、后台错误和关闭等待；支持轮询详情、事件增量读取、打断、恢复与终止。
- 启动时扫描遗留 running Runs：只有在 Workspace 没有 live owner 时才恢复为 interrupted/paused，避免崩溃窗口留下不可操作状态，也不干扰另一个存活进程。
- 新增 Runtime、Workspace、Workflow、TaskRun 四组 Wails bindings；Workspace 使用系统目录选择器，所有业务调用直接进入本地 application service。
- 桌面入口不再创建 HTTP client 或注册 Auth、Agent、Artifact、Billing、Conversation、Order services。
- application service 集成测试使用真实纯文件 repo、stub Agent 和后台 goroutine 跑通创建 Task → 启动 → completed。
- 验证：`go test ./...`、`go vet ./...`、目标包 `go test -race`、`wails3 generate bindings -clean=true` 和 `wails3 build DEV=true` 全部通过；桌面二进制完成启动冒烟。
