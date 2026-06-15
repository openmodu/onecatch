# Issue 00-018: 桌面端未登录态 binding 调用收敛

状态：已完成

## 来源

- PRD：`design/prd/00-agent-marketplace-mvp.md`
- 相关 issue：
  - `design/issues/00-002-auth-session.md`
  - `design/issues/00-014-desktop-prototype-alignment.md`
  - `design/issues/00-015-user-admin-api-boundary-security.md`
- 技术方案：`design/docs/TECHNICAL_DESIGNS.md#issue-00-018-桌面端未登录态-binding-调用收敛`

## 目标

修复 Wails dev 未登录首屏和浏览器视觉预览之间的边界：桌面原生运行时必须走真实鉴权，不允许因为 Vite dev mode 启用 fixture fallback 而先调用受保护 binding 再回退。

## 范围

- 收紧 `canUseFallback()`：
  - 普通浏览器视觉 QA 可继续使用 fixture。
  - Wails runtime 中即使是 dev mode，也不使用 fixture fallback。
- 保持未登录首屏只允许加载公开 Agent catalog 和当前用户探测。
- 未登录用户点击购买或下单必须被前端拦截，不触发受保护 billing/order binding。
- 未登录用户点击 Agent 进入 conversation、发送消息或确认扣次时必须被前端拦截，不触发受保护 conversation binding。
- 未登录用户切回工作台或存在 stale selected order 时，不允许自动触发交付物列表、下载、分享或取消订单 binding。

## 非目标

- 不新增后端 API。
- 不新增 Wails binding。
- 不改变 Bearer-only 鉴权。
- 不改变订单、扣次、交付物业务状态机。

## 产品需求

- Wails 原生端未登录首屏不应刷 `unauthenticated` binding 错误。
- 普通浏览器打开 Vite 页面时仍可使用 fixture 做视觉 QA。
- 登录成功后再加载余额、订单和交付物数据。
- 未登录点击购买或确认支付时提示登录，不创建订单、不扣次。
- 未登录点击 Agent 会话、消息发送或会话确认时提示登录，不创建 conversation、不创建订单、不扣次。
- 未登录点击工作台不应触发 `ArtifactBinding.ListArtifacts` 或其他订单后续操作 binding。

## 技术设计

- 后端改动：无。
- 桌面端 / Wails 改动：无。
- 前端改动：
  - 增加 Wails runtime 检测，只以 `window._wails.environment.OS` 作为原生桌面运行时信号。
  - 不用 `/wails/runtime` HTTP transport 可用性判断，因为普通浏览器打开 Vite dev server 时也可能尝试访问 Wails runtime；该场景应继续走 fixture。
  - `canUseFallback()` 必须同时满足 `import.meta.env.DEV`、未禁用 fixture、且不在 Wails runtime。
  - 保持现有 `currentUser` effect：只有存在登录用户时才刷新 billing/orders。
  - conversation 工作台的开始会话、发送消息、确认扣次同样先检查 `currentUser?.id`；未登录原生端只提示登录并跳到账户页。
  - 交付物 effect 增加 `currentUser?.id` 依赖和前置检查；下载、分享、取消订单也先检查登录态。
- 数据模型改动：无。
- API 或 binding 改动：无新增。

## 验收标准

- [x] Wails dev 未登录启动后不再持续输出 billing/order/artifact 的 `unauthenticated` binding 错误。
- [x] 未登录状态点击购买次数展示登录提示，不触发 `BillingBinding.StartPurchase`。
- [x] 未登录状态点击确认并支付展示登录提示，不触发 `OrderBinding.CreateOrder`。
- [x] 未登录状态点击 Agent 会话、发送消息或会话确认展示登录提示，不触发 `ConversationBinding`。
- [x] 未登录状态点击工作台或清空用户后不触发 `ArtifactBinding.ListArtifacts`。
- [x] 普通浏览器 Vite 页面仍可加载 fixture 做视觉 QA。
- [x] 登录后余额、订单、下单、交付物链路仍可走真实后端。

## 测试计划

- 自动验证：
  - `cd desktop/oneshot/frontend && npm run build`
  - `go test ./...`
- 联调验证：
  - 启动 `ONESHOT_ADDR=:18080 ONESHOT_AUTH_INSECURE_CALLBACKS=1 go run ./cmd/oneshot-server`。
  - 启动 `ONESHOT_API_BASE_URL=http://127.0.0.1:18080 wails3 dev -config ./build/config.yml -port <free-port>`。
  - 观察未登录首屏无受保护 binding 401 刷屏。
  - 登录后通过桌面 HTTP client 或 Wails UI 验证下单、worker 交付、artifact/share/download。

## 交付记录

- 已新增 Wails runtime 判断：只有 `window._wails.environment.OS` 存在时才视为桌面原生运行时。
- 已将浏览器 fixture 模式改为调用前直接走本地数据，不再先触发 Wails binding 后 fallback。
- 已保留 Wails 原生端真实鉴权：dev mode 下也不会使用 fixture fallback。
- 2026-06-14 回归修复：conversation 工作台的 `StartConversation`、`PostMessage`、`ConfirmCheckout` 已纳入未登录前端 guard。
- 2026-06-14 回归修复：`clients/oneshot` 通过 URL 解析保留 query string，避免订单筛选请求被编码为 `/api/orders%3Fstatus=...`。
- 2026-06-14 回归修复：交付物自动加载、下载、分享和取消订单已纳入未登录前端 guard，避免点击工作台时 stale selected order 触发 `unauthenticated`。
- 已验证：
  - `go test ./clients/oneshot` 通过，覆盖 `ListOrders(status)` 的 path/query 断言。
  - `cd desktop/oneshot/frontend && npm run build` 通过。
  - `go test ./...` 通过。
  - `git diff --check` 通过。
  - `ONESHOT_API_BASE_URL=http://127.0.0.1:18080 wails3 dev -config ./build/config.yml -port 9267` 成功启动并连接前端。
  - Wails dev 未登录首屏静置 5 秒未再出现受保护 binding 的 `unauthenticated` 刷屏。
  - 普通浏览器打开 `http://127.0.0.1:9267/` 可加载 fixture，登录和确认支付走本地预览，无 console error。
  - 桌面 HTTP client 到后端真实链路通过：Google 登录、Agent 列表、余额、下单、worker delivered、artifact 列表。
  - 2026-06-14 复验：本地 server `:18080` + Wails dev `:9267` 启动成功，未登录首屏静置 6 秒无新增受保护 binding 错误日志。
  - 2026-06-14 复验：普通浏览器打开 `http://127.0.0.1:9267/` 后点击 fixture Agent 可进入本地 conversation，不触发登录拦截，说明 fixture 预览路径仍保留。
  - 2026-06-14 复验：主仓库 Wails dev 未登录态实际点击左侧“我的订单”和“工作台”后，Wails/server 日志未再出现 `Binding call failed` 或 `unauthenticated`。
