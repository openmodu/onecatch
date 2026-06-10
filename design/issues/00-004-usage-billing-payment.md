# Issue 00-004: 次数计费与支付

状态：待开发

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
  - `Payment`
- 扣次和订单创建必须在服务端同一事务内完成。
- 支付回调必须验签且幂等。

## 验收标准

- [ ] 用户可以查看剩余次数。
- [ ] 用户可以查看用量流水。
- [ ] 购买成功会增加次数余额。
- [ ] 创建订单会准确扣减 1 次。
- [ ] 余额不足时阻止创建订单。
- [ ] 重复支付回调不会重复增加余额。
- [ ] 每条流水记录变更后的余额。

## 测试计划

- 单元测试扣次成功。
- 单元测试余额不足。
- 单元测试购买和扣减后的流水余额。
- 集成测试订单创建和扣次事务。
- 测试支付回调幂等。

## 交付记录

- 当前仓库已有开发用内存余额和流水能力。
- 真实支付 provider 尚未接入。
