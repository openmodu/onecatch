# Go Server And Wails v3 Implementation Notes

## 技术方向

- Server: Golang。
- Desktop: Golang + Wails v3。
- UI: 当前 React/Vite 原型可作为 Wails 前端迁移基础。
- 参考文档：
  - Wails v3: https://v3.wails.io/
  - Wails v3 installation: https://v3.wails.io/quick-start/installation/

Wails v3 官方 quickstart 当前使用：

```bash
go install github.com/wailsapp/wails/v3/cmd/wails3@latest
wails3 init -n oneshot -t vanilla
wails3 dev
```

实际落地时需要根据团队选择 React 模板或将当前 Vite React 前端迁入 Wails 前端目录。

## 建议仓库结构

```text
oneshot/
  cmd/
    server/              # HTTP/API server 入口
  internal/
    auth/                # 微信、Google 登录与 session
    billing/             # 次数余额、扣次、充值记录
    orders/              # 订单状态机、查询、详情
    agents/              # Agent 目录、定价、能力元数据
    delivery/            # 交付物生成、存储、下载授权
    payment/             # 微信支付、回调、对账
    storage/             # DB、对象存储、事务封装
  desktop/
    oneshot/             # Wails v3 桌面工程
  design/prototype/             # 当前原型
```

## 关键领域模型

首版不要过度抽象，先围绕以下对象落表和接口：

- `User`: 登录账号、登录来源、基础资料。
- `AuthIdentity`: 微信 openid/unionid、Google email/sub。
- `Agent`: 名称、分类、描述、单次价格、预计交付时间、交付物类型。
- `UsageBalance`: 用户剩余次数。
- `UsageLedger`: 购买、扣减、退款、人工调整记录。
- `Order`: 用户、Agent、需求、状态、扣次数、金额、预计完成时间。
- `DeliveryArtifact`: 订单交付物、文件类型、下载地址、生成状态。
- `Payment`: 充值订单、支付渠道、交易号、回调状态。

## 状态机建议

订单状态：

```text
draft -> pending_payment -> paid -> running -> delivering -> delivered
                                      \-> failed
pending_payment -> cancelled
```

用量扣减建议：

- 购买次数成功后写入 `UsageLedger` 正向记录。
- 创建执行订单时在同一事务内扣减次数并写入负向记录。
- 支付成功回调必须幂等。
- Agent 执行失败是否退次数要作为产品规则单独确认。

## API 首版边界

建议先做这些 HTTP API，再让 Wails 桌面复用同一套服务层：

```text
POST /api/auth/wechat/start
POST /api/auth/wechat/callback
POST /api/auth/google/callback
GET  /api/me

GET  /api/agents
GET  /api/agents/{id}

GET  /api/usage/balance
GET  /api/usage/ledger
POST /api/usage/purchase

POST /api/orders
GET  /api/orders
GET  /api/orders/{id}
POST /api/orders/{id}/cancel

GET  /api/orders/{id}/artifacts
GET  /api/artifacts/{id}/download
```

## Wails v3 落地方式

桌面端不要把业务逻辑写进前端组件里。建议：

- Wails Go app 负责窗口、系统能力、账号持久化、本地配置。
- 核心业务仍通过 Go service 调用 server API。
- 前端 React 只负责界面状态、表单、列表和交互反馈。
- Wails bindings 只暴露必要桌面能力，例如打开文件、下载目录、系统通知、本地缓存状态。

## 从原型到生产的拆分步骤

1. 固化业务规则
   - 确认按次计费规则。
   - 确认失败任务是否退次数。
   - 确认 Agent 执行交付 SLA。

2. 建立 Go server 骨架
   - `cmd/server` 启动。
   - 健康检查、配置、日志、DB 连接。
   - 先实现 Agent 列表与 mock 订单 API。

3. 实现账号与登录
   - 微信网页授权或扫码登录。
   - Google OAuth。
   - session/JWT 策略。

4. 实现计费与订单
   - 次数余额。
   - ledger。
   - 订单创建与扣次事务。
   - 支付回调幂等。

5. 实现交付物
   - 交付物元数据。
   - 文件存储。
   - 下载鉴权。

6. 建立 Wails v3 desktop
   - 初始化 Wails v3 工程。
   - 迁入 React UI。
   - 把 mock state 替换为 API client。
   - 增加系统通知和本地下载能力。

7. 验收
   - Server 单元测试和接口测试。
   - 订单扣次事务测试。
   - 支付回调幂等测试。
   - Wails 桌面冒烟测试。
   - 登录、购买次数、下单、交付下载全链路测试。

## 首个可落地里程碑

MVP 只做一个 Agent 类目和一个真实订单闭环：

- 用户可以登录。
- 用户可以购买次数。
- 用户可以选择一个 Agent 并提交需求。
- 系统扣减 1 次并生成订单。
- 订单进入执行中。
- 后台可以上传或生成一个交付物。
- 用户可以在桌面端查看并下载交付物。
