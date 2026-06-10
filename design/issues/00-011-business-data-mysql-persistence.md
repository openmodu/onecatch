# Issue 00-011: 业务数据 MySQL 持久化

状态：待开发

## 来源

- PRD：`design/prd/00-agent-marketplace-mvp.md`
- 相关 issue：`design/issues/00-007-mysql-connection.md`
- 相关 issue：`design/issues/00-003-agent-catalog.md`
- 相关 issue：`design/issues/00-004-usage-billing-payment.md`
- 相关 issue：`design/issues/00-005-order-lifecycle.md`
- 相关 issue：`design/issues/00-006-delivery-artifacts.md`

## 目标

将当前内存 repo 中的 Agent、余额、用量流水、订单和交付物数据迁移到 MySQL 持久化实现，确保应用重启后核心业务数据不丢失。

## 范围

- Agent 表与种子数据迁移。
- 用户余额表。
- 用量流水表。
- 订单表。
- 交付物元数据表。
- 各 `internal/repo/*` 从内存实现迁移到 MySQL/GORM 实现。
- 保留无 MySQL 配置时的开发降级策略。
- API 和 Wails binding 前后端联调。

## 非目标

- 真实支付 provider。
- 真实 Agent worker 执行。
- 对象存储文件上传下载。
- 后台管理端。

## 产品需求

- 应用重启后 Agent 目录仍可查询。
- 应用重启后用户余额不丢失。
- 应用重启后订单列表和订单详情不丢失。
- 用量流水必须可追溯每次余额变化。
- 交付物元数据必须归属于订单和用户。

## 技术设计

- 遵守 DDD 依赖方向：
  - `repo/*` 依赖 `pkg/sql.Sql`。
  - `repo/*` 不依赖 `internal/data`。
  - `internal/data` 负责资源生命周期和 repo 聚合。
- 每个业务 repo 包内定义接口和实现。
- MySQL schema 建议覆盖：
  - `agents`
  - `user_balances`
  - `billing_ledger`
  - `orders`
  - `artifacts`
- 写入订单和扣次需要事务边界；可在 usecase 或 repo 事务接口中明确实现。

## 验收标准

- [ ] 配置 MySQL 后 Agent 目录从数据库读取。
- [ ] 配置 MySQL 后余额和流水持久化。
- [ ] 配置 MySQL 后订单创建、列表、详情持久化。
- [ ] 配置 MySQL 后交付物元数据持久化。
- [ ] 服务重启后数据仍存在。
- [ ] 未配置 MySQL 时本地开发仍可运行。
- [ ] `go test ./...` 通过。
- [ ] 前端通过 Wails binding 可看到持久化数据。

## 测试计划

- repo 集成测试。
- usecase 事务一致性测试。
- HTTP API 测试。
- Wails 前端联调测试。
- 手动重启服务验证数据不丢失。

## 交付记录

- `00-007` 已完成 MySQL 连接初始化。
- 当前业务 repo 大多仍是内存数据，需要本 issue 持久化。
