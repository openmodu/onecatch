# Issue 00-005: 订单生命周期

状态：已完成

## 来源

- PRD：`design/prd/00-agent-marketplace-mvp.md`
- 相关原型：`design/prototype/`
- 技术方案：`design/docs/TECHNICAL_DESIGNS.md#issue-00-005-订单生命周期`

## 目标

实现 MVP Agent 工作流中的订单创建、列表、详情、取消、状态展示和执行进度。

## 范围

- 基于选中的 Agent 和需求创建订单。
- 查询用户订单列表。
- 按状态筛选订单。
- 查询订单详情。
- 取消符合条件的订单。
- 展示执行进度时间线。
- 持久化订单、用户、Agent 和扣次记录之间的关系。

## 非目标

- 完整异步 Agent 执行引擎。
- 人工运营后台。
- SLA 升级机制。
- 多步骤审批流。

## 产品需求

- 用户登录且扣次成功后可以创建订单。
- 新执行订单进入执行中状态。
- 订单列表支持全部、待支付、执行中、已交付视图。
- 订单详情展示订单号、创建时间、状态、预计完成时间、扣减次数、金额和 Agent。
- 执行进度展示需求已提交、扣次确认、执行中、交付物生成中、已交付。
- 取消行为必须遵循订单状态机。

## 技术设计

- 后端接口：
  - `POST /api/orders`
  - `GET /api/orders`
  - `GET /api/orders/{id}`
  - `POST /api/orders/{id}/cancel`
- 桌面端 bindings：
  - `OrderBinding.CreateOrder(agentID, requirement)`
  - `OrderBinding.ListOrders()`
  - `OrderBinding.GetOrder(orderID)`
  - `OrderBinding.CancelOrder(orderID)`
- 领域对象：`Order`。
- `Order` 增加 Agent 名称、金额、预计完成时间、失败原因和进度时间线。
- `internal/usecase/orders` 校验非空需求、查询 Agent 价格与交付信息、扣次成功后创建 running 订单。
- `GET /api/orders?status=` 支持按状态筛选。
- `OrderBinding` 桌面端创建、列表、详情、取消均走真实 client。
- 桌面端订单列表、订单信息和执行进度从真实 binding 加载。
- 状态机：

```text
draft -> pending_payment -> paid -> running -> delivering -> delivered
                                      \-> failed
pending_payment -> cancelled
```

## 验收标准

- [x] 已登录用户可以用非空需求创建订单。
- [x] 订单创建记录 Agent、用户、需求、金额、状态和消耗次数。
- [x] 扣次失败时订单创建失败。
- [x] 用户可以查看自己的订单列表。
- [x] 用户可以查看自己的订单详情。
- [x] 用户不能查看其他用户的订单。
- [x] 符合条件的订单可以取消。
- [x] 执行进度时间线与当前订单状态一致。

## 测试计划

- 单元测试订单创建校验：`TestOrderValidationAndArtifacts` 覆盖空需求 400。
- 单元测试订单状态取消规则：当前由 domain `CanCancel` 和 handler 既有测试路径覆盖，后续可补独立 domain table test。
- 集成测试订单创建和扣次：`TestBillingPurchaseAndOrderDebit` 覆盖登录、购买、创建订单、扣次和订单字段。
- 测试订单列表、详情、取消 handler：既有 HTTP handler 测试继续通过。
- 手动验证桌面端订单筛选和执行进度：前端已切到真实 binding，`npm run build` 通过。

## 交付记录

- 已补齐订单创建字段、非空需求校验、状态筛选和进度时间线。
- 已将桌面端订单列表、订单信息和进度展示接入真实 `OrderBinding`。
- `go test ./...` 和桌面前端 `npm run build` 已通过。
- 生产持久化在 `00-011`，worker 驱动状态推进在 `00-012`。
