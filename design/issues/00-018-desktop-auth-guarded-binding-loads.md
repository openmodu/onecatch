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

## 技术设计

- 后端改动：无。
- 桌面端 / Wails 改动：无。
- 前端改动：
  - 增加 Wails runtime 检测，只以 `window._wails.environment.OS` 作为原生桌面运行时信号。
  - 不用 `/wails/runtime` HTTP transport 可用性判断，因为普通浏览器打开 Vite dev server 时也可能尝试访问 Wails runtime；该场景应继续走 fixture。
  - `canUseFallback()` 必须同时满足 `import.meta.env.DEV`、未禁用 fixture、且不在 Wails runtime。
  - 保持现有 `currentUser` effect：只有存在登录用户时才刷新 billing/orders。
- 数据模型改动：无。
- API 或 binding 改动：无新增。

## 验收标准

- [x] Wails dev 未登录启动后不再持续输出 billing/order/artifact 的 `unauthenticated` binding 错误。
- [x] 未登录状态点击购买次数展示登录提示，不触发 `BillingBinding.StartPurchase`。
- [x] 未登录状态点击确认并支付展示登录提示，不触发 `OrderBinding.CreateOrder`。
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
- 已验证：
  - `cd desktop/oneshot/frontend && npm run build` 通过。
  - `go test ./...` 通过。
  - `git diff --check` 通过。
  - `ONESHOT_API_BASE_URL=http://127.0.0.1:18080 wails3 dev -config ./build/config.yml -port 9267` 成功启动并连接前端。
  - Wails dev 未登录首屏静置 5 秒未再出现受保护 binding 的 `unauthenticated` 刷屏。
  - 普通浏览器打开 `http://127.0.0.1:9267/` 可加载 fixture，登录和确认支付走本地预览，无 console error。
  - 桌面 HTTP client 到后端真实链路通过：Google 登录、Agent 列表、余额、下单、worker delivered、artifact 列表。
