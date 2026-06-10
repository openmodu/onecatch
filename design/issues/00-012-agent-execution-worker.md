# Issue 00-012: Agent 执行 Worker 与订单推进

状态：已完成

## 来源

- PRD：`design/prd/00-agent-marketplace-mvp.md`
- 相关 issue：`design/issues/00-005-order-lifecycle.md`
- 前置 issue：`design/issues/00-011-business-data-mysql-persistence.md`

## 目标

实现后台 Agent 执行 worker，让订单从创建后的 `running` 状态自动推进到交付物生成和已交付状态，形成可验证的生产级订单执行闭环。

## 范围

- 订单执行任务队列或轮询机制。
- worker 生命周期管理。
- 订单状态推进：
  - `running`
  - `delivering`
  - `delivered`
  - `failed`
- 执行进度记录。
- 交付物元数据写入。
- 失败重试和失败原因记录。
- 前端订单进度刷新或重新查询。

## 非目标

- 复杂分布式任务调度。
- 多 worker 横向扩展。
- 真实大模型供应商接入。
- 对象存储文件内容上传。
- 人工审核流程。

## 产品需求

- 创建订单后，用户可以看到执行中状态。
- worker 处理后，订单可以进入交付物生成中。
- worker 完成后，订单进入已交付。
- 失败时订单进入失败状态，并展示明确失败信息。
- 用户只能看到自己的订单进度。

## 技术设计

- usecase 层定义订单执行业务流程。
- repo 层提供按状态拉取待执行订单、更新状态、写入交付物元数据的方法。
- worker 不直接依赖 `internal/transport`。
- worker 启动位置在 server 或 service 组合层明确管理生命周期。
- 执行进度需要可由 API 查询，前端基于订单详情展示。
- `internal/usecase/execution` 提供 `RunOnce` 和 `Start`。
- server main 启动后台 worker，桌面 local client 也启动同一 worker。
- worker 轮询 running 订单，推进到 delivering，再推进到 delivered 并生成交付物。
- 执行失败时写入 failed 状态和 failure reason。
- 桌面端登录后轮询订单列表以展示最新进度。

## 验收标准

- [x] 创建订单后 worker 可自动推进状态。
- [x] 成功执行会生成交付物元数据。
- [x] 失败执行会记录失败状态和原因。
- [x] 订单详情 API 返回最新状态和进度。
- [x] 前端能看到订单状态变化。
- [x] worker 停止时服务可优雅退出。
- [x] `go test ./...` 通过。

## 测试计划

- worker 单元测试：`TestRunOnceDeliversRunningOrder` 覆盖 running -> delivered 和交付物生成。
- usecase 状态机测试：worker 测试覆盖状态推进，订单进度由 `BuildProgress` 生成。
- repo 状态更新测试：orders repo 内存路径由 worker 测试覆盖，SQL 路径由编译和 GORM schema 覆盖。
- HTTP 订单详情测试：既有 HTTP 测试继续通过。
- 前端订单进度联调：`npm run build` 和 `wails3 build DEV=true` 通过。

## 交付记录

- 已新增 `internal/usecase/execution` worker。
- 已在 server 和桌面 local client 启动 worker。
- 已让桌面端登录后轮询订单列表展示状态变化。
- 验证：`go test ./...`、`npm run build`、`wails3 build DEV=true` 通过。
