# Issue 01-001: 自定义 Workflow 定义与状态机内核

状态：已完成

## 来源

- PRD：`design/prd/01-custom-agent-workflows.md`
- 技术方案：`design/docs/01-custom-agent-workflows.md`
- 参考实现：外层 `../buddy` 的双 Actor loop；本 issue 只吸收结构化信号、轮次保护和显式收敛原则，不复制固定角色状态机。

## 目标

交付一个不依赖 HTTP、数据库和具体 CLI 的 workflow 领域内核，使调用方可以定义有向、可成环的 Agent 步骤图，校验定义，解析结构化 outcome，并以确定性规则推进、暂停或完成 run。

## 范围

- 定义 `Definition`、`Step`（runtime、role prompt、instruction）、`Policy`、`Run`、`Outcome` 和转移记录。
- 支持普通步骤目标与 `$done`、`$pause`、`$fail` 保留目标。
- 校验入口、ID、signal、目标、可达性、完成路径和保护策略。
- 解析纯 JSON 或单个 JSON fenced block outcome。
- 提供 start、advance、failure、resume 的纯状态转移函数。
- 覆盖自环/回边、未知 signal、终点、轮次上限和连续失败上限测试。

## 非目标

- 不在本 issue 中增加数据库 repo、API、Wails binding 或桌面编辑器。
- 不在本 issue 中调用 `internal/agentrun` 或实现本地 orchestrator。
- 不支持并行步骤、条件表达式或用户脚本。

## 产品需求

- 合法工作流必须至少有一个从入口可达的 `$done` 转移。
- 模型只能返回当前步骤已声明的 signal，不能直接指定下一步骤。
- 达到保护上限必须停在可观察的暂停态。
- 非法定义和非法推进必须返回可识别的领域错误。

## 技术设计

- 后端改动：新增 `internal/domain/workflows` 纯领域包。
- 桌面端 / Wails 改动：无。
- 前端改动：无。
- 数据模型改动：仅新增内存领域结构，不新增表。
- API 或 binding 改动：无。

## 验收标准

- [x] 两步骤 review loop 可以从 implement 推进到 review，再返回 implement，最后完成。
- [x] 非法入口、重复步骤、非法目标、不可达步骤和无完成路径被拒绝。
- [x] 未声明 signal 不改变 run。
- [x] `$pause`、`$fail`、`$done` 进入正确终态。
- [x] 最大转移次数和连续失败次数使用安全默认值并能触发暂停。
- [x] outcome parser 对非法或含多余文本的响应返回协议错误。
- [x] `go test ./internal/domain/workflows` 和 `go test ./...` 通过。

## 测试计划

- 单元测试：已覆盖定义默认值与深拷贝、图校验、runtime/role prompt 必填、协议解析、review 回边、所有终点、未知 signal 输入不变、转移上限、失败上限和定义 ID 匹配。
- 集成测试：在后续 01-003 中用 stub agentrun 覆盖。
- 手动验证：通过 `TestReviewLoopCanReturnAndComplete` 按 implement → review → implement → review → done 顺序逐步检查状态和历史。

## 交付记录

- 新增 `internal/domain/workflows/model.go`：runtime-first Definition/Step/Policy、Run、Outcome 和转移记录。
- 新增 `validation.go`：安全默认值、字段级问题、目标/可达性/完成路径校验。
- 新增 `protocol.go`：严格 JSON outcome parser；兼容单一 JSON fenced block，不接受额外自然语言或模型指定 target。
- 新增 `engine.go`：NewRun、Start、Advance、RecordFailure、Resume；普通边可形成 loop，终点和保护上限行为确定。
- 新增 `workflows_test.go`：领域内核完整单测。
- 根据产品方向澄清，Step 不依赖旧 Agent 市场 `AgentID`，直接要求 local runtime、role prompt 和 instruction。
- 验证结果：`go test ./...`、`go vet ./...`、`go test -race ./internal/domain/workflows ./internal/agentrun ./internal/usecase/execution`、桌面前端 `npm run build` 全部通过。
