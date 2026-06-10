# Issue 00-004: 次数计费与支付

状态：已完成

## 来源

- PRD：`design/prd/00-agent-marketplace-mvp.md`
- 相关原型：`design/prototype/`

## 目标

实现按使用次数计费：用户购买次数余额，执行 Agent 扣减 1 次，所有余额变化写入用量流水。

## 范围

- 查询次数余额。
- 查询用量流水。
- 购买次数流程边界。
- 支付回调边界。
- 创建执行订单时扣减 1 次。
- 余额不足处理。

## 非目标

- 订阅套餐。
- 项目制报价。
- 优惠券。
- 自动退款；但需要保留退款流水类型。

## 产品需求

- 用户可以看到剩余次数。
- 用户可以从侧边栏或 Inspector 打开购买次数流程。
- 购买成功后增加次数余额。
- 执行 Agent 扣减 1 次。
- 余额为 0 时，不能创建订单，应引导购买次数。
- 使用记录展示购买和扣减。

## 技术设计

- 后端接口：
  - `GET /api/billing/balance`
  - `GET /api/billing/ledger`
  - `POST /api/billing/purchases`
- 桌面端 bindings：
  - `BillingBinding.GetBalance()`
  - `BillingBinding.ListLedger()`
  - `BillingBinding.StartPurchase(planID)`
- 领域对象：
  - `UsageBalance`
  - `UsageLedger`
  - `PurchasePlan`
  - `Purchase`
- `internal/domain/billing` 增加余额、流水、购买计划和购买记录。
- `internal/repo/billing` 以 repo 包内接口封装余额、流水和购买幂等记录。
- `internal/usecase/billing` 提供余额查询、流水查询、开发购买和订单扣次。
- `POST /api/billing/purchases` 在显式 `paymentId` 下幂等，重复回调不会重复加次。
- 当前内存 repo 中购买和流水仍是开发实现；生产 MySQL 事务在 `00-011` 补齐。
- 真实支付 provider、回调验签和外部支付网关在 `00-010` / 后续支付 issue 中处理。

## 验收标准

- [x] 用户可以查看剩余次数。
- [x] 用户可以查看用量流水。
- [x] 购买成功会增加次数余额。
- [x] 创建订单会准确扣减 1 次。
- [x] 余额不足时阻止创建订单。
- [x] 重复支付回调不会重复增加余额。
- [x] 每条流水记录变更后的余额。

## 测试计划

- 单元测试扣次成功：由 HTTP 集成测试覆盖订单创建后的扣次结果。
- 单元测试余额不足：订单创建仍复用 `ErrInsufficientBalance`，HTTP 映射为 402。
- 单元测试购买和扣减后的流水余额：`TestBillingPurchaseAndOrderDebit` 覆盖购买、重复支付、订单扣次和流水余额。
- 集成测试订单创建和扣次事务：`TestBillingPurchaseAndOrderDebit` 覆盖 API 级创建订单后余额从 20 到 19。
- 测试支付回调幂等：`TestBillingPurchaseAndOrderDebit` 使用相同 `paymentId` 重复购买，余额只增加一次。

## 交付记录

- 已实现开发购买接口 `POST /api/billing/purchases` 和桌面端 `BillingBinding.StartPurchase`。
- 已将桌面端余额、购买按钮和订单创建后的余额刷新接入真实 binding。
- 已补充 HTTP 集成测试覆盖购买幂等、扣次和流水。
- 当前交付使用内存 repo；生产 MySQL 事务落在 `00-011`。
- 真实外部支付 provider 与回调验签不在本 issue 内。
