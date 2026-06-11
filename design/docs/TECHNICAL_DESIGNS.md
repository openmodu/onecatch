# 技术方案与接口设计

本文档记录每个 issue 在开发前应先确认的技术方案。后续执行顺序必须是：

```text
Issue 范围确认 -> 技术方案/API 设计 -> 实现 -> 验证 -> 交付记录
```

技术方案必须至少包含：

- 后端 API：method、path、请求、响应、错误码。
- Wails binding：方法名、入参、返回值。
- 客户端 SDK：`clients/oneshot` 方法。
- 数据模型：领域对象和持久化表。
- 前后端联调点：哪个页面/组件调用哪个 binding。
- 验证方式：单测、集成测试、构建或手动验收。

## Issue 00-004: 次数计费与支付

### 后端 API

| Method | Path | 请求 | 响应 | 错误 |
| --- | --- | --- | --- | --- |
| `GET` | `/api/billing/balance` | Bearer token / 当前会话 | `Balance` | `401` 未登录 |
| `GET` | `/api/billing/ledger` | Bearer token / 当前会话 | `LedgerEntry[]` | `401` 未登录 |
| `POST` | `/api/billing/purchases` | `{ "planId": "uses_10", "paymentId": "optional-id" }` | `Purchase` | `401` 未登录，`500` 未知套餐 |

### SDK 与 Binding

| 层 | 方法 |
| --- | --- |
| `clients/oneshot` | `GetBalance(ctx)`、`ListLedger(ctx)`、`StartPurchase(ctx, StartPurchaseRequest)` |
| Wails | `BillingBinding.GetBalance()`、`BillingBinding.ListLedger()`、`BillingBinding.StartPurchase(input)` |

### 数据模型

- `Balance`：用户剩余次数。
- `LedgerEntry`：余额流水，包含 `type`、`delta`、`balanceAfter`、`orderId`、`paymentId`。
- `Purchase`：购买记录，`paymentId` 唯一幂等。
- MySQL 表：`user_balances`、`billing_ledger`、`billing_purchases`。

### 联调点

- 账户详情展示剩余次数和最近流水。
- Agent 详情右上角余额 pill 调用购买次数。
- 创建订单后刷新余额和订单列表。

## Issue 00-005: 订单生命周期

### 后端 API

| Method | Path | 请求 | 响应 | 错误 |
| --- | --- | --- | --- | --- |
| `POST` | `/api/orders` | `{ "agentId": "...", "requirement": { "prompt": "..." } }` | `Order` | `400` 空需求，`401` 未登录，`402` 余额不足，`404` Agent 不存在 |
| `GET` | `/api/orders?status=running` | 可选 `status` | `Order[]` | `401` 未登录 |
| `GET` | `/api/orders/{orderID}` | 当前用户订单 | `Order` | `401` 未登录，`404` 不存在/不归属 |
| `POST` | `/api/orders/{orderID}/cancel` | 当前用户订单 | `Order` | `401` 未登录，`404` 不存在/不归属 |

### SDK 与 Binding

| 层 | 方法 |
| --- | --- |
| `clients/oneshot` | `CreateOrder(ctx, input)`、`ListOrders(ctx, status)`、`GetOrder(ctx, id)`、`CancelOrder(ctx, id)` |
| Wails | `OrderBinding.CreateOrder(input)`、`OrderBinding.ListOrders(status)`、`OrderBinding.GetOrder(id)`、`OrderBinding.CancelOrder(id)` |

### 数据模型

- `Order`：包含用户、Agent、需求、状态、消耗次数、金额、预计完成、失败原因、进度。
- `ProgressStep`：前端进度展示结构。
- MySQL 表：`orders`。

### 联调点

- Agent 详情页右侧需求输入调用 `CreateOrder`。
- 订单页中间列表通过 `ListOrders(status)` 请求后端筛选。
- 订单详情右侧展示进度和操作。

## Issue 00-006: 交付物

### 后端 API

| Method | Path | 请求 | 响应 | 错误 |
| --- | --- | --- | --- | --- |
| `GET` | `/api/orders/{orderID}/artifacts` | 当前用户订单 | `Artifact[]` | `401` 未登录，`404` 不存在，`409` 未交付 |
| `GET` | `/api/artifacts/{artifactID}/download` | 当前用户交付物 | PDF bytes | `401` 未登录，`404` 不存在，`409` 未交付 |
| `POST` | `/api/artifacts/{artifactID}/share` | 当前用户交付物 | `ArtifactShare` | `401` 未登录，`404` 不存在，`409` 未交付 |

### SDK 与 Binding

| 层 | 方法 |
| --- | --- |
| `clients/oneshot` | `ListArtifacts(ctx, orderID)`、`DownloadArtifact(ctx, artifactID)`、`ShareArtifact(ctx, artifactID)` |
| Wails | `ArtifactBinding.ListArtifacts(orderID)`、`ArtifactBinding.DownloadArtifact(id)`、`ArtifactBinding.ShareArtifact(id)`、`ArtifactBinding.ShowInFolder(path)` |

### 数据模型

- `Artifact`：交付物元数据。
- `Download`：服务端下载内容。
- `Share`：分享 token 和 URL。
- MySQL 表：`artifacts`、`artifact_shares`。

### 联调点

- 订单详情右侧交付物区域展示文件。
- 文件行右侧提供下载、分享。
- 下载后调用 `ShowInFolder`。

## Issue 00-010: OAuth 与安全会话

### 后端 API

| Method | Path | 请求 | 响应 | 错误 |
| --- | --- | --- | --- | --- |
| `POST` | `/api/auth/wechat/start` | 无 | `OAuthStart` | `500` state 生成失败 |
| `POST` | `/api/auth/wechat/callback` | `OAuthCallbackInput` 或空开发降级 | `Session` | `401` state 错误 |
| `POST` | `/api/auth/google/callback` | `OAuthCallbackInput` 或空开发降级 | `Session` | `401` state 错误 |
| `POST` | `/api/auth/logout` | Bearer token / 当前会话 | `{ "ok": true }` | `401` 未登录 |
| `GET` | `/api/me` | Bearer token / 当前会话 | `User` | `401` 未登录 |

### SDK 与 Binding

| 层 | 方法 |
| --- | --- |
| `clients/oneshot` | `CurrentUser(ctx)`、`StartWechat(ctx)`、`LoginWithWechat(ctx)`、`LoginWithGoogle(ctx)`、`Logout(ctx)` |
| Wails | `AuthBinding.CurrentUser()`、`AuthBinding.LoginWithWechat()`、`AuthBinding.LoginWithGoogle()`、`AuthBinding.Logout()` |

### 数据模型

- `OAuthStart`：provider、authUrl、state。
- `Session`：token、provider、user、expiresAt。
- `AuthIdentity`：provider 和 providerSubject 唯一身份。
- MySQL 表：`users`、`auth_identities`。

### 联调点

- 账户详情页登录区调用登录 binding。
- HTTP client 自动保存 token 到用户配置目录，并使用 `Authorization: Bearer <token>`。

## Issue 00-011: 业务数据 MySQL 持久化

### 资源与 Repo 设计

| Repo | 内存降级 | MySQL 表 |
| --- | --- | --- |
| `agents` | seed catalog | `agents` |
| `users` | user/identity map | `users`、`auth_identities` |
| `billing` | balance/ledger/purchase map | `user_balances`、`billing_ledger`、`billing_purchases` |
| `orders` | order map | `orders` |
| `artifacts` | artifact/share map | `artifacts`、`artifact_shares` |

### 依赖方向

```text
transport/api -> service -> usecase -> repo -> pkg/sql
data -> pkg/sql
data -> repo
```

repo 包内部决定 `sql == nil` 时走内存，存在 `pkg/sql.Sql` 时走 GORM。

### 联调点

- 未配置 MySQL：桌面 `wails3 dev` 本地内存可用。
- 配置 `ONESHOT_MYSQL_DSN`：服务端 repo 自动 `AutoMigrate` 并持久化核心业务数据。

## Issue 00-012: Agent 执行 Worker

### Usecase 设计

| 方法 | 说明 |
| --- | --- |
| `Execution.RunOnce(ctx)` | 扫描 running 订单并推进一次 |
| `Execution.Start(ctx, interval)` | 后台定时执行 worker |

### 状态推进

```text
running -> delivering -> delivered
                  \-> failed
```

成功后调用 `Artifacts.CreateForOrder` 生成交付物元数据。失败时写入 `StatusFailed` 和 `FailureReason`。

### 启动位置

- server：`cmd/oneshot-server/main.go` 启动 worker。
- desktop local client：`desktop/oneshot/client.go` 启动本进程 worker。

### 联调点

- 前端登录后每 2 秒刷新订单列表。
- 订单详情展示最新状态、进度和交付物。

## Issue 00-013: 桌面端三段式交互重构

### 信息架构

```text
左侧 rail：Agent / 订单 / 账户
中间 pane：列表和筛选
右侧 pane：当前对象详情和主操作
```

### 前端状态

| 状态 | 说明 |
| --- | --- |
| `activeSection` | `agents` / `orders` / `account` |
| `activeCategory` | Agent 分类筛选 |
| `orderFilter` | 后端订单状态筛选 |
| `selectedAgentId` | 右侧 Agent 详情对象 |
| `selectedOrderId` | 右侧订单详情对象 |

### 按钮位置规则

| 操作 | 位置 |
| --- | --- |
| 开始执行 | Agent 详情右侧需求输入区底部 |
| 购买次数 | Agent 详情余额 pill、账户详情余额区 |
| 下载 / 分享 | 订单详情交付物区域 |
| 取消订单 | 订单详情右上角次级按钮 |
| 分类筛选 | Agent 中间列表顶部 |
| 订单筛选 | 订单中间列表顶部 |

### API / Binding 对齐

- `OrderBinding.ListOrders(status)` 传入状态。
- `clients/oneshot.ListOrders(ctx, status)` 拼接 `/api/orders?status=`。
- 后端复用 `GET /api/orders?status=`。

### 验证

- `go test ./...`
- `npm run build`
- `wails3 build DEV=true`
- `wails3 dev` 原生窗口启动检查。
