# PRD 00: Oneshot Agent 服务市场 MVP

状态：历史方案（不再作为当前产品方向；当前主线见 `design/prd/01-custom-agent-workflows.md`）

## 1. 背景

Oneshot 是一个桌面化 Agent 服务市场。用户可以在桌面工作台中浏览专业 Agent，按使用次数购买额度，提交具体需求，系统扣减次数后执行 Agent，并在订单完成后查看、下载或分享交付物。

当前 PRD 基于 `design/prototype/` 目录中的 React/Vite 原型整理，用于指导后续 Go Server 与 Wails v3 桌面端实现。

## 2. 产品目标

### 2.1 核心目标

- 建立一个桌面端 Agent 服务市场的首个可用闭环。
- 支持用户登录、浏览 Agent、购买次数、提交需求、扣次执行、查看订单、查看交付物。
- 固化“按使用次数计费”的商业模型：一次 Agent 执行扣减 1 次。
- 为后续真实支付、真实 OAuth、后台执行、交付物存储打下产品和接口边界。

### 2.2 非目标

- 不做营销落地页。
- 不做订阅套餐、项目制报价或复杂会员等级。
- 不在 MVP 内支持多工作区、多组织、多角色权限。
- 不在 MVP 内支持离线下单或离线查看完整交付物。
- 不在 MVP 内支持用户自定义 Agent 上架流程。

## 3. 目标用户

### 3.1 主要用户

需要快速购买专业 Agent 服务的个人或小团队用户，例如：

- 需要行业研究、竞品分析、市场洞察的创业者或运营人员。
- 需要内容生成、推广方案、数据分析的增长或营销人员。
- 需要一次性任务交付，而不是长期订阅工具的用户。

### 3.2 用户诉求

- 能快速理解每个 Agent 能做什么、多久交付、价格多少。
- 能按次购买，不需要长期订阅。
- 能清楚看到当前余额、扣次记录和订单状态。
- 能在一个桌面工作台中完成从需求提交到交付下载的闭环。

## 4. 产品形态

### 4.1 平台

- Desktop: Go + Wails v3。
- Backend: Go Server。
- UI: 基于当前 React/Vite prototype 迁移到 Wails 前端。

### 4.2 信息架构

产品采用三段式桌面工作台：

- 左侧导航：工作台、Agent 分类、次数与用量、订单筛选、账号登录。
- 中间工作区：Agent 详情、需求填写、扣次确认、执行进度、交付物状态。
- 右侧 Inspector：用量计费、使用记录、订单信息、交付物预览，可折叠。

## 5. MVP 范围

### 5.1 登录

支持两种登录入口：

- 微信授权登录。
- Google 邮箱登录。

MVP 要求：

- 登录入口固定在左下角账号区。
- 未登录状态展示两个登录按钮。
- 已登录状态展示登录来源、账号状态和退出登录按钮。
- 用户未登录时点击“确认并支付”必须提示先登录，不允许创建订单。

### 5.2 Agent 市场

左侧支持 Agent 分类筛选：

- 全部 Agent
- 内容创作
- 数据分析
- 市场营销
- 设计创意
- 开发与技术
- 办公效率
- 行业研究

Agent 卡片至少展示：

- Agent 名称
- 分类
- 标签
- 简介
- 单次价格
- 评分
- 成交次数
- 平均交付时间
- 交付物说明

MVP 初始 Agent 可包含：

- 行业研究分析师
- 内容增长写手
- 经营数据分析师
- 新品上市策划

### 5.3 Agent 详情与需求填写

用户选择 Agent 后，中间工作区展示：

- Agent 名称、标签、简介。
- 评分、成交次数、平均交付时间。
- 单次价格。
- 同分类或筛选后的 Agent 快捷切换。
- 任务流程 Tab：Agent 详情、需求填写、扣次确认、执行中、交付物。

需求填写要求：

- 用户可以编辑一段自然语言需求。
- 非编辑状态下显示需求摘要。
- 点击“编辑需求”进入可编辑状态。
- 需求不能为空；生产实现中创建订单前必须校验。

### 5.4 按次计费

计费模型：

- 用户购买“次数”作为余额。
- 每次执行 Agent 固定扣减 1 次。
- 单次价格随 Agent 展示，但 MVP 的执行扣费单位以次数为准。

用量展示：

- 当前剩余次数。
- 本次扣减次数。
- 单次价格。
- 应付金额。
- 使用记录。

购买次数：

- 用户可打开购买次数弹窗。
- 用户选择购买数量。
- 支付成功后增加余额。
- 原型中为模拟支付；生产实现必须接入真实支付和支付回调。

扣次规则：

- 余额大于 0 时，确认执行后扣减 1 次并创建订单。
- 余额等于 0 时，打开购买次数流程。
- 扣减和订单创建必须在服务端同一事务内完成。
- Agent 执行失败是否退次数待产品规则确认。

### 5.5 订单

订单来源：

- 用户选择 Agent。
- 填写需求。
- 确认扣减 1 次。
- 系统创建订单并进入执行中。

订单状态：

```text
draft -> pending_payment -> paid -> running -> delivering -> delivered
                                      \-> failed
pending_payment -> cancelled
```

MVP 前端展示状态：

- 待支付
- 执行中
- 已交付

订单列表支持筛选：

- 全部订单
- 待支付
- 执行中
- 已交付

订单详情至少包含：

- 订单号
- 创建时间
- 订单状态
- 预计完成时间
- 本次扣减次数
- 总金额
- 关联 Agent

### 5.6 执行进度

中间工作区展示订单执行时间线：

1. 需求已提交
2. 扣次确认中
3. 执行中
4. 交付物生成中
5. 已交付

要求：

- 当前节点需要有明确视觉状态。
- 已完成、当前、未开始节点需要可区分。
- 预计完成时间来自订单或 Agent 交付时长。

### 5.7 交付物

交付物展示位置：

- 中间工作区的“交付物”流程页。
- 右侧 Inspector 的交付物预览卡片。

交付物信息：

- 文件名
- 文件类型
- 文件大小
- 预览图
- 下载按钮
- 分享按钮

MVP 交付物类型：

- PDF 报告。

要求：

- 已交付订单可以查看交付物。
- 下载按钮触发本地下载。
- 分享按钮生成分享链接或复制分享信息。
- 未交付订单不应展示可下载文件。

### 5.8 右侧 Inspector

右侧 Inspector 包含：

- 用量与计费
- 使用记录
- 订单信息
- 交付物预览

要求：

- 可以折叠。
- 折叠后保留窄轨入口，展示“详情”和剩余次数。
- 展开后保持独立滚动，不影响中间工作区。

## 6. 核心用户流程

### 6.1 首次使用流程

1. 用户打开桌面端。
2. 用户在左下角选择微信或 Google 登录。
3. 用户浏览 Agent 分类。
4. 用户选择一个 Agent。
5. 用户查看 Agent 详情、价格和交付时间。
6. 用户填写需求。
7. 用户确认扣减 1 次。
8. 系统创建订单并进入执行中。
9. 用户在右侧查看订单状态。
10. 订单交付后用户下载交付物。

### 6.2 余额不足流程

1. 用户选择 Agent 并填写需求。
2. 用户点击确认执行。
3. 系统发现剩余次数为 0。
4. 打开购买次数弹窗。
5. 用户完成购买。
6. 系统增加次数余额。
7. 用户重新确认执行。

### 6.3 查看历史订单流程

1. 用户在左侧选择订单筛选条件。
2. 系统展示匹配订单。
3. 用户点击使用记录中的订单。
4. 右侧 Inspector 展示订单详情。
5. 如订单已交付，展示交付物预览和下载入口。

## 7. 功能需求

### 7.1 账号与会话

- 系统应支持微信授权登录。
- 系统应支持 Google 邮箱登录。
- 系统应支持退出登录。
- 系统应在桌面端安全保存会话。
- 系统应能获取当前登录用户。
- 未登录用户不能创建订单或扣减次数。

### 7.2 Agent 目录

- 系统应返回 Agent 分类列表。
- 系统应返回 Agent 列表。
- 系统应支持按分类筛选 Agent。
- 系统应返回 Agent 详情。
- 系统应展示 Agent 的价格、交付时间、评分、成交次数和交付物说明。

### 7.3 次数余额

- 系统应展示用户剩余次数。
- 系统应展示用户使用记录。
- 系统应支持购买次数。
- 系统应在购买成功后增加余额。
- 系统应在创建执行订单时扣减 1 次。
- 系统应记录每一次购买、扣减、退款或人工调整。

### 7.4 订单

- 系统应支持创建订单。
- 创建订单必须关联用户、Agent、需求和扣减记录。
- 系统应支持查询订单列表。
- 系统应支持按状态筛选订单。
- 系统应支持查询订单详情。
- 系统应支持取消未进入不可取消阶段的订单。
- 系统应展示订单执行状态和预计完成时间。

### 7.5 支付

- 系统应支持购买次数支付。
- 系统应支持支付回调。
- 支付回调必须幂等。
- 支付成功后必须写入用量流水。
- 支付失败或取消不能增加余额。

### 7.6 交付物

- 系统应支持订单交付物元数据管理。
- 系统应支持交付物预览。
- 系统应支持交付物下载鉴权。
- 系统应支持生成或复制分享链接。

### 7.7 桌面端能力

- 桌面端应支持本地下载目录选择。
- 桌面端应支持下载完成后在文件夹中显示。
- 桌面端可支持系统通知，用于订单交付提醒。
- 桌面端 Wails bindings 只暴露必要桌面能力和业务调用，不承载核心业务规则。

## 8. 数据对象

### 8.1 User

- `id`
- `displayName`
- `avatarUrl`
- `createdAt`
- `updatedAt`

### 8.2 AuthIdentity

- `id`
- `userId`
- `provider`: `wechat` 或 `google`
- `providerSubject`
- `email`
- `createdAt`

### 8.3 Agent

- `id`
- `name`
- `category`
- `tag`
- `summary`
- `description`
- `price`
- `priceUses`
- `rating`
- `usageCount`
- `estimatedDuration`
- `deliverable`
- `artifactTypes`
- `status`

### 8.4 UsageBalance

- `userId`
- `remaining`
- `updatedAt`

### 8.5 UsageLedger

- `id`
- `userId`
- `type`: `purchase`、`debit`、`refund`、`adjust`
- `orderId`
- `paymentId`
- `delta`
- `balanceAfter`
- `createdAt`

### 8.6 Order

- `id`
- `userId`
- `agentId`
- `requirement`
- `status`
- `usageCost`
- `amount`
- `createdAt`
- `updatedAt`
- `estimatedCompletedAt`
- `completedAt`

### 8.7 Payment

- `id`
- `userId`
- `provider`
- `providerTransactionId`
- `status`
- `usageCount`
- `amount`
- `createdAt`
- `updatedAt`

### 8.8 DeliveryArtifact

- `id`
- `orderId`
- `name`
- `mimeType`
- `size`
- `storageUri`
- `previewUri`
- `shareToken`
- `createdAt`

## 9. 接口边界

MVP 建议 API：

```text
POST /api/auth/wechat/start
POST /api/auth/wechat/callback
POST /api/auth/google/callback
POST /api/auth/logout
GET  /api/me

GET  /api/agents
GET  /api/agents/{id}

GET  /api/billing/balance
GET  /api/billing/ledger
POST /api/billing/purchases

POST /api/orders
GET  /api/orders
GET  /api/orders/{id}
POST /api/orders/{id}/cancel

GET  /api/orders/{id}/artifacts
GET  /api/artifacts/{id}/download
POST /api/artifacts/{id}/share
```

接口分层规则：

- `/api/*` 只作为用户端产品接口，面向桌面端和用户侧 client。
- 后台管理接口不得混入 `/api/*` 用户端接口；后续如需后台管理，必须单独使用 `/admin/api/*` 或独立管理服务，并有独立鉴权、权限和审计。
- 用户端接口只能返回当前用户有权访问的数据，不得通过参数传入任意 `userId` 查询其他用户数据。
- 后台管理接口不得复用用户端 DTO 直接暴露敏感字段；需要单独定义 admin DTO，并默认脱敏。

Wails bindings 建议：

```text
AuthBinding.LoginWithGoogle()
AuthBinding.Logout()
AuthBinding.CurrentUser()

AgentBinding.ListAgents()
AgentBinding.GetAgent(agentID)

BillingBinding.GetBalance()
BillingBinding.ListLedger()
BillingBinding.StartPurchase(planID)

OrderBinding.CreateOrder(agentID, requirement)
OrderBinding.ListOrders()
OrderBinding.GetOrder(orderID)
OrderBinding.CancelOrder(orderID)

交付物下载 binding 在实现交付物 issue 时再新增。
```

## 10. 权限与安全

- 所有订单、余额、流水、交付物都必须按当前用户鉴权。
- 下载交付物必须校验订单归属。
- 用户端接口和后台管理接口必须严格区分路由、handler、service 入口和 DTO。
- 用户端响应遵循最小披露原则，只返回当前界面需要展示的字段。
- 不得在用户端接口返回其他用户的邮箱、OAuth provider subject、会话 token、支付交易细节、存储 URI、内部错误堆栈或后台备注。
- 后台管理访问用户数据必须有独立管理员身份、细粒度权限和审计日志；默认展示脱敏数据，查看敏感原文必须有明确权限边界。
- 支付回调必须验签。
- 支付回调必须幂等，不能重复增加次数。
- 桌面端不得保存明文敏感凭证。
- 服务端日志不得记录 OAuth token、session token、支付密钥、用户敏感输入全文或交付物原文。

## 11. 非功能需求

### 11.1 性能

- Agent 列表首屏加载目标小于 1 秒。
- 订单列表常规查询目标小于 1 秒。
- 桌面端启动后应尽快显示主工作台，不因远端接口阻塞白屏。

### 11.2 可用性

- 所有主按钮必须有反馈。
- 支付、扣次、下单、下载失败时必须展示错误提示。
- 右侧 Inspector 折叠与展开不应丢失当前选择的订单。
- 小屏宽度下内容不能重叠。

### 11.3 可观测性

- 服务端需要记录登录、支付、扣次、订单状态变更、交付物下载的审计日志。
- 支付回调、扣次事务、订单执行失败需要可追踪。

## 12. 验收标准

MVP 验收必须覆盖：

- 用户可以通过微信或 Google 登录。
- 未登录用户点击下单会被阻止并看到提示。
- 用户可以浏览 Agent 分类并切换 Agent。
- 用户可以编辑需求。
- 用户有余额时，确认执行会扣减 1 次并创建订单。
- 用户余额不足时，会进入购买次数流程。
- 用户可以查看余额和使用记录。
- 用户可以按状态筛选订单。
- 用户可以查看订单详情和执行进度。
- 已交付订单可以预览和下载 PDF 交付物。
- 右侧 Inspector 可以折叠和展开。
- 桌面端和后端共享同一套订单与计费业务规则。

## 13. 待确认问题

- Agent 执行失败是否自动退回次数。
- 购买次数是否支持套餐折扣。
- 交付物分享链接是否需要有效期和访问密码。
- 微信登录采用扫码登录、网页授权还是桌面端外部浏览器授权。
- Google 登录是否只允许邮箱登录，是否限制企业域名。
- 订单进入执行中后是否允许取消。
- 后台上传交付物还是 Agent 自动生成交付物。
