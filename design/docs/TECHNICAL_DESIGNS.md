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

## 全局安全与接口分层规则

- `/api/*` 只用于用户端产品接口，由桌面端、用户侧 SDK 和 Wails binding 调用。
- 用户端接口必须从 session 或 `Authorization: Bearer <token>` 解析当前用户，不能接受客户端传入任意 `userId` 访问其他用户资源。
- 后台管理接口必须与用户端接口严格分离，后续统一使用 `/admin/api/*` 或独立管理服务，且使用独立 router、handler、service 和 DTO。
- 后台管理接口不能通过用户端 Wails binding 暴露，也不能复用用户端 client 自动获得权限。
- 用户端 response DTO 遵循最小披露原则，不返回 `providerSubject`、session token、OAuth token、支付密钥、支付原始回调、内部 `storageUri`、内部错误堆栈或后台备注。
- 邮箱只允许当前用户自己的账号接口返回；订单、流水、交付物列表不冗余返回用户邮箱。
- 用户需求原文、交付物内容、token、支付密钥不得进入普通日志；需要定位问题时使用 request id、order id 和脱敏摘要。
- 越权访问其他用户资源应返回 `404` 或统一权限错误，避免泄露资源存在性；后台权限不足返回 `403`。

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

## Issue 00-014: 桌面端回归原型与 Codex Desktop 风格纠偏

### 评审状态

本方案已按用户确认的“UI follow 原型”方向实现并验证。

00-013 的实现把三段式理解为 `左侧 rail / 中间列表 / 右侧详情`。这适合文件、邮件或对象列表浏览，但不适合 Oneshot 当前的 Agent 服务交易工作台，因为用户的主任务不是浏览对象列表，而是完成一次 Agent 服务购买、需求提交、执行跟踪和交付下载。

### Codex Desktop 参考原则

官方 Codex app 说明将其定义为面向线程、项目、Git、终端和任务侧栏的聚焦桌面体验；任务侧栏用于展示计划、来源、生成物和任务总结，用户可以在主线程中持续协作。由此抽象到 Oneshot：

| 原则 | 对 Oneshot 的落地 |
| --- | --- |
| 左侧是低频全局切换，不是业务筛选容器 | 左侧只放工作台、订单、用量/账单、账号入口 |
| 中间是主工作区 | Agent 选择、需求、扣次、进度和交付物都在中间完成 |
| 右侧是上下文，不是第二主流程 | 用量、订单信息、交付物预览进入可折叠 inspector |
| 操作靠近对象 | 下单按钮靠近结算区，购买靠近余额，下载/分享靠近文件 |
| 渐进披露 | 默认减少常驻信息密度，需要时再展开 inspector 或详情 |
| 桌面原生克制 | 薄分割线、低饱和颜色、紧凑控件、稳定尺寸，避免营销页和后台大卡片感 |

### 与原型的对齐

原型明确的结构是：

```text
左侧导航与账号区 / 中间任务工作区 / 右侧可折叠 inspector
```

00-014 恢复这个结构，但对原型做减重：

- 左侧保留全局导航与账号区，去掉大量 Agent 分类、订单状态、用量明细。
- 中间保留 Agent hero、分类筛选、任务流程、需求、扣次确认、执行进度、交付物。
- 右侧保留 inspector 能力，但不常驻压迫主流程；默认收起为顶部按钮，展开后显示上下文信息。

### 信息架构

```text
左侧 sidebar：
  - 工作台
  - Agent 市场
  - 我的订单
  - 用量与账单
  - 左下账号区

中间 workspace：
  - 顶部 toolbar：返回、当前上下文、Inspector 开关、帮助
  - Agent summary：名称、标签、说明、评分、交付时长、单次价格
  - Agent 分类筛选：segmented control 或横向 filter bar
  - Agent 快捷切换：紧凑 chip，不作为列表主流程
  - 任务流程：Agent 详情、需求填写、扣次确认、执行中、交付物
  - 当前任务内容：需求、结算、进度、交付物

右侧 inspector：
  - 用量与计费
  - 使用记录
  - 订单信息
  - 交付物预览
```

### 按钮位置规则

| 操作 | 位置 | 原因 |
| --- | --- | --- |
| `确认并支付 / 开始执行` | 中间 `checkout` 结算区右侧或底部 | 主动作必须靠近扣次、价格和需求上下文 |
| `编辑需求` | 需求卡片标题右侧 | 操作对象是需求文本 |
| `购买次数` | 余额摘要卡片、用量页、账号区浮层 | 操作对象是余额，不是主导航 |
| `Inspector 展开/收起` | 工作区右上角图标按钮 | 这是视图工具，不是业务步骤 |
| `取消订单` | 订单详情或订单上下文菜单 | 次级危险操作，不抢主按钮 |
| `下载 / 分享` | 交付物卡片内 | 操作对象是文件 |
| `订单筛选` | 订单页中间顶部 filter bar | 筛选属于订单列表上下文，不属于全局导航 |
| `Agent 分类` | 工作台中间 filter bar | 分类属于 Agent 市场上下文，不属于全局导航 |

### 前端状态设计

| 状态 | 说明 |
| --- | --- |
| `activeView` | `workbench` / `orders` / `billing` / `account` |
| `activeCategory` | 当前 Agent 分类筛选 |
| `selectedAgentId` | 当前主工作区 Agent |
| `selectedStep` | `details` / `requirement` / `checkout` / `running` / `artifact` |
| `orderFilter` | 订单筛选，传给 `OrderBinding.ListOrders(status)` |
| `selectedOrderId` | 当前订单上下文 |
| `inspectorOpen` | 右侧上下文面板展开状态 |

### 后端 API

本 issue 不新增后端 API。

继续使用已有 API：

| Method | Path | 用途 |
| --- | --- | --- |
| `GET` | `/api/agents` | Agent 分类与 Agent 列表 |
| `GET` | `/api/agents/{id}` | Agent 详情 |
| `GET` | `/api/billing/balance` | 余额摘要 |
| `GET` | `/api/billing/ledger` | 使用记录 |
| `POST` | `/api/billing/purchases` | 购买次数 |
| `POST` | `/api/orders` | 创建订单与扣次 |
| `GET` | `/api/orders?status=` | 订单筛选 |
| `GET` | `/api/orders/{orderID}` | 订单详情 |
| `GET` | `/api/orders/{orderID}/artifacts` | 交付物列表 |

### Wails Binding

不新增 binding，前端对齐现有方法：

| Binding | 用途 |
| --- | --- |
| `AuthBinding.CurrentUser()` | 启动时恢复账号状态 |
| `AuthBinding.LoginWithWechat()` | 微信登录 |
| `AuthBinding.LoginWithGoogle()` | Google 登录 |
| `AuthBinding.Logout()` | 退出登录 |
| `AgentBinding.ListAgents()` | 工作台 Agent 筛选与切换 |
| `AgentBinding.GetAgent(id)` | Agent 详情 |
| `BillingBinding.GetBalance()` | 余额摘要 |
| `BillingBinding.ListLedger()` | 使用记录 |
| `BillingBinding.StartPurchase(input)` | 购买次数 |
| `OrderBinding.CreateOrder(input)` | 确认并支付 / 开始执行 |
| `OrderBinding.ListOrders(status)` | 订单筛选 |
| `OrderBinding.GetOrder(id)` | 订单上下文 |
| `OrderBinding.CancelOrder(id)` | 取消订单 |
| `ArtifactBinding.ListArtifacts(orderID)` | 交付物列表 |
| `ArtifactBinding.DownloadArtifact(id)` | 下载交付物 |
| `ArtifactBinding.ShareArtifact(id)` | 分享交付物 |

### 前后端联调点

| UI 区域 | 调用 |
| --- | --- |
| 启动恢复账号 | `AuthBinding.CurrentUser()` |
| Agent 工作台 | `AgentBinding.ListAgents()`，前端按 `category` 筛选 |
| Agent 切换 | `AgentBinding.GetAgent(id)` 或列表缓存 |
| 余额摘要 | `BillingBinding.GetBalance()` |
| 使用记录 | `BillingBinding.ListLedger()` |
| 购买次数 | `BillingBinding.StartPurchase(input)` 后刷新余额与流水 |
| 确认并支付 | `OrderBinding.CreateOrder(input)` 后刷新余额、订单、进度 |
| 订单页筛选 | `OrderBinding.ListOrders(status)` |
| 订单详情 | `OrderBinding.GetOrder(id)` |
| 交付物 | `ArtifactBinding.ListArtifacts(orderID)` |
| 下载 / 分享 | `ArtifactBinding.DownloadArtifact(id)`、`ArtifactBinding.ShareArtifact(id)` |

### 视觉与交互约束

- 不做营销 hero。
- 不做管理后台式四处堆卡片。
- 不使用大面积蓝色作为主色。
- 不使用过大的圆角卡片，普通卡片圆角控制在 6-8px。
- 不用左侧承载业务长列表。
- 主工作区宽度优先，右侧 inspector 不能压缩核心任务到难以阅读。
- 文字不使用 viewport 字号缩放，按钮和固定控件需要稳定尺寸。
- macOS 桌面窗口下避免内容溢出、遮挡和横向滚动。

### 验证方式

实现完成后必须按以下顺序验证：

1. 静态核对：
   - 对照 `design/prototype/README.md` 的三段式定义。
   - 对照 `design/prototype/src/App.jsx` 的核心流程。
   - 对照本方案的按钮位置表。
2. 自动验证：
   - `go test ./...`
   - `cd desktop/oneshot/frontend && npm run build`
   - `wails3 build DEV=true`
3. 联调验证：
   - 未登录点击主按钮会阻止创建订单。
   - 登录后创建订单会扣减余额并刷新订单。
   - 订单筛选会调用 `OrderBinding.ListOrders(status)`。
   - 交付物下载和分享仍走 Artifact binding。
4. Mac 端视觉检查：
   - 使用 `wails3 dev` 启动原生窗口。
   - 检查 1280x800 和 1440x900。
   - 记录截图路径；如果 macOS 权限导致 `screencapture` 失败，交付记录必须写明原因。
   - 使用浏览器 dev fixture 截图补充视觉核对，但不能替代原生窗口检查。

### 实施边界

本次实现只改：

- `desktop/oneshot/frontend/src/App.jsx`
- `desktop/oneshot/frontend/src/styles.css`
- 前端开发态 fixture
- 相关 issue 的交付记录

未新增后端 API 或 Wails binding。

### 实施结果

- 已按原型恢复 `左侧导航与账号区 / 中间任务工作区 / 右侧可折叠 inspector`。
- 左侧不再承载 Agent 分类、订单筛选和用量明细。
- Agent 分类、订单筛选、流程步骤和主按钮均回到中间工作区。
- 右侧 Inspector 默认收起，展开后展示用量、使用记录、订单信息、交付物预览。
- 浏览器开发态增加 fixture，以支持无 Wails runtime 的视觉 QA。
- 验证通过：
  - `go test ./...`
  - `cd desktop/oneshot/frontend && npm run build`
  - `cd desktop/oneshot && wails3 build DEV=true`
  - `wails3 dev -config ./build/config.yml -port 9255`
  - 浏览器截图：`/tmp/oneshot-ui-1440x900-fixed.png`、`/tmp/oneshot-ui-1280x800-final.png`
  - Mac 原生截图：`/tmp/oneshot-wails-dev.png`

## Issue 00-016: Inspector 菜单与右侧内容面板

### 评审状态

本方案已由用户确认进入实现，并已完成 00-016 交付。

用户提供 Claude Code 作为明确视觉和交互参考：点击右上角工具按钮先弹出菜单，选择 `Diff` 后，右侧出现可关闭的内容面板。Oneshot 不照搬编码工具能力，但采用这个交互模型。

### 交互模型

```text
右上角 Inspector 入口
  -> 点击显示浮层菜单
  -> 选择菜单项
  -> 右侧弹出内容面板
  -> 点击 X 关闭面板
```

### 菜单项映射

| Claude Code 参考 | Oneshot 菜单项 | 面板内容 |
| --- | --- | --- |
| `Preview` | `预览` | 当前交付物预览、文件、下载/分享 |
| `Diff` | `明细` | 当前任务的扣次、价格、订单、流水明细 |
| `Files` | `订单` | 当前订单字段和状态 |
| `Background tasks` | `记录` | 最近使用记录和计费流水 |
| `Plan` | `进度` | 当前订单执行进度 |

`Terminal` 不映射到 Oneshot MVP，避免引入无关编码工具能力。

### 前端状态

| 状态 | 说明 |
| --- | --- |
| `inspectorMenuOpen` | 右上角浮层菜单是否展开 |
| `activeInspectorPanel` | `preview` / `detail` / `order` / `records` / `progress` |
| `inspectorPanelOpen` | 右侧内容面板是否打开 |

### 前端结构

```text
topbar
  inspector trigger
  inspector menu popover

workspace
  main workbench
  inspector drawer
    drawer header
    drawer content
    drawer close button
```

### API / Binding

本 issue 不新增后端 API 或 Wails binding。面板复用当前页面已经加载的数据和现有 binding：

| 面板 | 数据来源 |
| --- | --- |
| `预览` | `ArtifactBinding.ListArtifacts(orderID)`，`DownloadArtifact`，`ShareArtifact` |
| `明细` | `selectedAgent`、`selectedOrder`、`balance`、`ledger` |
| `订单` | `OrderBinding.ListOrders(status)` 已加载的当前订单 |
| `记录` | `BillingBinding.ListLedger()` 和订单列表 |
| `进度` | 当前订单 `progress` |

### 视觉约束

- 菜单应是轻量浮层，锚定右上角 Inspector 入口。
- 菜单不应改变主工作区布局。
- 右侧面板可覆盖或占据右侧区域，但必须有清晰边界和关闭按钮。
- 右侧面板宽度建议 420-460px；1280px 宽窗口下可降到 380px。
- 面板内部滚动，页面整体不横向溢出。
- 不使用编码产品专属的 Terminal/Files 文案，除非后续产品确实加入这些能力。

### 验证方式

1. 浏览器视觉验证：
   - 默认工作台。
   - Inspector 菜单打开态。
   - 选择 `明细` 后的右侧面板态。
   - 关闭面板后的恢复态。
2. Mac 原生视觉验证：
   - `wails3 dev -config ./build/config.yml -port <free-port>`
   - 检查 macOS 窗口控制点、顶部工具按钮、菜单和右侧面板不重叠。
3. 自动验证：
   - `go test ./...`
   - `cd desktop/oneshot/frontend && npm run build`
   - `cd desktop/oneshot && wails3 build DEV=true`

### 验证结果

- `cd desktop/oneshot/frontend && npm run build` 通过。
- `go test ./...` 通过。
- `cd desktop/oneshot && wails3 build DEV=true` 通过，仅有 macOS SDK 链接版本 warning。
- `cd desktop/oneshot && wails3 dev -port 9259` 通过，Wails 原生启动并连接 `http://localhost:9259` 前端 dev server。
- 浏览器视觉检查通过：
  - Inspector 入口点击后显示浮层菜单。
  - 菜单项 `明细` 打开右侧 drawer，标题、上下文路径和关闭按钮可见。
  - 1280px 宽下主区与 drawer 为 `630px 430px`，无横向溢出。
  - 关闭 drawer 后主区恢复单栏。

## Issue 00-017: Mac native 桌面视觉系统收敛

### 评审状态

本方案已由用户确认进入实现，并已完成 00-017 交付。

本方案基于 checkpoint 提交 `e6be354 chore(checkpoint): preserve current desktop UI state` 继续优化。Fable 版本已经完成了部分 native 方向的探索：Wails 原生菜单、toolbar drag/no-drag、图标化导航、Inspector popover 和快捷键展示。00-017 不推翻这些交互，只收敛视觉系统和 Mac 基础行为。

### 问题诊断

当前版本仍有三类问题：

1. 视觉仍偏 Web app：
   - 大面积米色/陶土色让界面更像网页后台或品牌页。
   - 主工作区仍由大量 bordered cards 组成，桌面工具感不足。
   - Inspector `明细` 仍是彩色大块，而不是更易扫读的 panel/list。
2. Mac 基础行为不完整：
   - `body { user-select: none; }` 会阻止订单号、说明文本等内容复制。
   - 全局 `contextmenu` 拦截会破坏 Mac 常见右键行为。
3. Dark mode 半成品风险：
   - CSS 已声明 `color-scheme: light dark`，但 Wails 背景和截图验收没有覆盖 dark mode。
   - 本期不应带半完成 dark mode 进入验收。

### 设计目标

```text
保留当前交互模型
  -> 收敛色彩到 macOS light neutral
  -> 收敛 sidebar / toolbar / popover / drawer 到 native 桌面结构
  -> 恢复选择、复制、右键等基础行为
  -> 用截图和构建验证不跑偏
```

### 视觉系统

| 区域 | 当前问题 | 目标 |
| --- | --- | --- |
| 全局背景 | 米色偏重 | macOS neutral `#f5f5f7` 一类浅灰 |
| Sidebar | 色块和卡片感明显 | source list，低对比 selection |
| Toolbar | 仍像网页按钮栏 | 原生 toolbar，轻按钮和拖拽区 |
| Popover | 接近目标但阴影/选中态偏品牌 | native menu 式轻浮层 |
| Main | 大量卡片边框 | grouped sections，弱边界 |
| Inspector | 彩色摘要块 | inspector panel + list/table/diff rows |

### API / Binding

本 issue 不新增后端 API 或 Wails binding。

| 数据 | 来源 |
| --- | --- |
| Agent | 已有 `AgentBinding.ListAgents()` |
| 余额/流水 | 已有 `BillingBinding.GetBalance()` / `BillingBinding.ListLedger()` |
| 订单 | 已有 `OrderBinding.ListOrders(status)` |
| 交付物 | 已有 `ArtifactBinding.ListArtifacts(orderID)` |

### Wails 改动

- 保留当前 `AppMenu`、`EditMenu`、`WindowMenu` 原生菜单角色。
- 将 `application.WebviewWindowOptions.BackgroundColour` 调整为与前端 light 背景一致的中性灰。
- 不新增窗口、不新增系统菜单命令、不新增 binding。

### 前端改动

#### `styles.css`

- 将 `:root` 收敛为 light mode token：
  - `--bg`：macOS neutral background。
  - `--sidebar`：轻 source list 背景。
  - `--surface`：内容白色。
  - `--line`：低对比分隔线。
  - `--accent`：只用于主按钮、金额和轻 selection。
- 移除本期未验收的 `@media (prefers-color-scheme: dark)` token。
- 将文本选择策略从全局禁用改为控件级禁用：
  - 禁用：button、nav item、drag region。
  - 允许：正文、订单号、需求文本、drawer 文本、输入框。
- 移除全局右键禁用相关视觉假设。
- 调整：
  - `.sidebar`、`.nav-block`、`.auth-card`
  - `.topbar`、`.back-link`、`.ghost-link`、`.inspector-trigger`
  - `.inspector-menu`
  - `.agent-hero`、`.segmented`、`.agent-strip`、`.flow-tabs`、`.brief-grid`、`.checkout-panel`
  - `.inspector-drawer`、`.drawer-header`、`.detail-diff-row`、`.drawer-list`
- 恢复并维护 860px 以下断点，避免小窗口布局损坏。

#### `App.jsx`

- 移除全局 `contextmenu` event handler。
- 保留当前 `Icon` 组件，不在本 issue 引入新依赖。
- 如视觉需要，仅微调 drawer 明细 DOM 的 class 或结构，不改变数据来源和业务状态。
- 保留 `⌘1` 到 `⌘5` 打开 Inspector 面板的行为。

### 可复制与右键边界

| 元素 | 选择 | 右键 |
| --- | --- | --- |
| 按钮、导航、toolbar 控件 | 不选择 | 浏览器/Wails 默认 |
| 需求文本、订单号、金额、drawer 明细 | 可选择 | 不阻止 |
| textarea/input | 可选择 | 不阻止 |
| Wails drag 区 | 不选择 | 不阻止 |

### 验证计划

1. 自动验证：
   - `cd desktop/oneshot/frontend && npm run build`
   - `go test ./...`
   - `cd desktop/oneshot && wails3 build DEV=true`
2. 浏览器视觉验证：
   - 1280x800 默认态。
   - 1280x800 Inspector 菜单态。
   - 1280x800 `明细` drawer 态。
   - 1440x900 默认态和 drawer 态。
   - 记录截图路径。
   - 使用 DOM 检查 `document.documentElement.scrollWidth <= window.innerWidth`。
3. Mac 原生验证：
   - `cd desktop/oneshot && wails3 dev -port <free-port>`
   - 检查 macOS 窗口控制点、toolbar、popover 和 drawer 不重叠。
   - 检查 Wails native window 背景与前端背景无明显色差。
4. 行为验证：
   - 选择并复制订单号、需求文本、drawer 明细。
   - 非输入区域右键不被应用层阻止。
   - Inspector 菜单点击外部关闭。
   - `⌘1` 到 `⌘5` 可打开对应面板。

### 验收风险

- 如果继续保留暖色主背景，即使控件变轻，仍会偏 Web/品牌页，不符合 Mac native 目标。
- 如果保留全局禁止选择和右键，会和 Mac 桌面行为冲突。
- 如果同时做 dark mode，会扩大验证范围；本 issue 明确只验收 light mode。

### 验证结果

- `cd desktop/oneshot/frontend && npm run build` 通过。
- `go test ./...` 通过。
- `cd desktop/oneshot && wails3 build DEV=true` 通过，仅有 macOS SDK 链接版本 warning。
- `cd desktop/oneshot && wails3 dev -port 9262` 通过，Wails 原生启动并连接 `http://localhost:9262` 前端 dev server。
- 浏览器视觉截图：
  - 1280 默认态：`/tmp/oneshot-017-default-1280.png`
  - 1280 菜单态：`/tmp/oneshot-017-menu-1280.png`
  - 1280 明细 drawer 态：`/tmp/oneshot-017-detail-1280.png`
  - 1440 默认态：`/tmp/oneshot-017-default-1440.png`
  - 1440 明细 drawer 态：`/tmp/oneshot-017-detail-1440.png`
- DOM 检查：
  - 1280 和 1440 下 `document.documentElement.scrollWidth <= window.innerWidth`。
  - drawer 打开时 1280 为 `630px 430px`，1440 为 `790px 430px`。
  - `body` 与 `.main-column` 可选择文本，导航按钮不可选择。
- 代码审计：
  - `App.jsx` 已移除全局 `contextmenu` 拦截。
  - `styles.css` 已移除 `prefers-color-scheme: dark` 半成品 token。
  - Wails window background 已改为中性灰。
  - 导航和 Inspector 图标已调为轻笔画、单色、统一尺寸的 template symbol 风格。

## Issue 00-015: 用户端与后台管理接口安全边界

### 后端 API

本 issue 不新增用户端业务 API，先定义和审计接口边界。

| 类型 | 路由前缀 | 调用方 | 鉴权 | 说明 |
| --- | --- | --- | --- | --- |
| 用户端产品 API | `/api/*` | 桌面端、用户侧 SDK、Wails binding | 当前用户 session / Bearer token | 只返回当前用户有权访问的数据 |
| 后台管理 API | `/admin/api/*` 或独立管理服务 | 后台管理端 | 管理员身份、权限、审计 | 不复用用户端 DTO，不通过用户端 binding 暴露 |

### 用户端 DTO 规则

- `User`：可返回当前用户 `id`、`displayName`、`email`、`avatarUrl`、`status`；不得返回身份 provider subject 或 token。
- `Balance` / `LedgerEntry`：只返回当前用户余额和流水展示字段；不得返回支付原始回调或其他用户信息。
- `Order`：只返回当前用户订单展示字段；不得返回后台备注、内部错误堆栈或其他用户身份信息。
- `Artifact`：只返回展示和下载所需元数据；不得返回内部 `storageUri`。

### 后台管理边界

- 后台 handler 不放在现有用户端 handler 文件中，避免误注册到 `/api/*`。
- 后台 service 不复用用户端 service 直接返回领域对象，必须经过 admin DTO 脱敏。
- 后台敏感原文查看必须记录管理员、目标资源、操作时间、原因和 request id。
- 后台接口的权限不足返回 `403`，未登录返回 `401`。

### 验证方式

- 路由审计：枚举 `/api/*`，确认没有后台管理能力。
- DTO 测试：序列化响应不包含 `providerSubject`、token、内部 `storageUri`、支付原始回调或后台备注。
- 跨用户测试：订单、交付物、余额和流水不能跨用户读取。
- 日志测试：Authorization、OAuth callback、支付 callback、订单需求和交付物内容不会以原文进入普通日志。
