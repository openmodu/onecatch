# Issue 00-015: 用户端与后台管理接口安全边界

状态：已完成

## 来源

- PRD：`design/prd/00-agent-marketplace-mvp.md`
- 技术方案：`design/docs/TECHNICAL_DESIGNS.md#issue-00-015-用户端与后台管理接口安全边界`

## 目标

明确并实现用户端 API 与后台管理 API 的安全边界，避免用户端数据接口暴露隐私数据，避免后台管理能力混入用户端接口。

## 范围

- 梳理现有 `/api/*` 用户端接口的鉴权和数据返回字段。
- 明确用户端接口不得通过 `userId` 参数访问其他用户数据。
- 为后续后台管理接口预留独立路由、handler、service 和 DTO 规范。
- 定义隐私字段、敏感字段和日志脱敏规则。
- 增加接口测试或审计测试，覆盖跨用户访问、敏感字段泄漏和未授权访问。

## 非目标

- 本次不实现完整后台管理端。
- 本次不实现后台用户管理 UI。
- 本次不实现组织、角色或企业权限体系。
- 本次不改变现有用户端业务流程。

## 产品需求

- 用户只能看到自己的账号、余额、流水、订单和交付物。
- 用户端接口不得返回其他用户的邮箱、身份映射、支付细节、内部存储地址或后台备注。
- 管理员能力必须与用户端产品能力分离，不能通过桌面端 binding 或用户端 client 调用。
- 敏感原文查看必须有明确权限和审计记录。
- 未授权、越权和权限不足必须返回明确但不泄露资源存在性的错误。

## 技术设计

- 用户端 API：
  - 保持 `/api/*` 作为用户端产品接口。
  - handler 只能从当前 session / Bearer token 解析当前用户，不接受客户端传入的任意 `userId` 作为查询归属。
  - response 使用用户端 DTO，只包含当前界面需要展示的字段。
- 后台管理 API：
  - 后续统一放在 `/admin/api/*` 或独立管理服务。
  - 使用独立 router、handler、service 入口和 admin DTO。
  - 不通过 Wails 用户端 binding 暴露。
  - 必须具备管理员身份、权限校验和审计日志。
- 隐私与敏感字段：
  - 用户端不得返回 `providerSubject`、session token、OAuth token、支付密钥、支付原始回调、内部 `storageUri`、内部错误堆栈、后台备注。
  - 邮箱只允许当前用户自己的账号页返回；订单、流水、交付物列表不冗余返回邮箱。
  - 用户需求原文、交付物内容和下载内容不得进入普通日志。
- 错误处理：
  - 未登录返回 `401`。
  - 越权访问其他用户资源返回 `404` 或统一权限错误，避免泄露资源是否存在。
  - 后台权限不足返回 `403`。

## 验收标准

- [x] `/api/*` 用户端接口没有后台管理能力。
- [x] 用户端订单、余额、流水和交付物接口都按当前用户鉴权。
- [x] 用户端接口不会返回 `providerSubject`、token、内部存储 URI、支付原始回调或后台备注。
- [x] 跨用户访问订单和交付物被拒绝，且不泄露资源归属。
- [x] 后续后台管理接口只能通过独立路由和独立鉴权进入。
- [x] 日志不记录 token、支付密钥、用户需求全文或交付物原文。

## 测试计划

- 单元测试 response DTO，不包含隐私字段。
- HTTP handler 测试未登录访问返回 `401`。
- HTTP handler 测试跨用户访问订单、交付物、余额和流水被拒绝。
- 日志脱敏测试覆盖 Authorization、OAuth callback、支付 callback 和订单需求字段。
- 手动审计 `/api/*` 路由清单，确认没有后台管理接口。

## 交付记录

- 已将用户端与后台管理接口分离、安全和隐私最小披露要求写入 PRD。
- 已新增本 issue 作为后续实现或安全审计的独立交付单元。
- 2026-06-11 接口冒烟审计：
  - `GET /healthz`、`GET /api/agents`、`GET /api/agents/{id}` 可直接返回数据。
  - `POST /api/auth/google/callback` 开发降级登录可返回 session；记录中不得泄露 token。
  - 携带 Bearer token 后，`GET /api/me`、`GET /api/billing/balance`、`GET /api/billing/ledger`、`POST /api/billing/purchases`、`POST /api/orders`、`GET /api/orders`、`GET /api/orders/{id}`、`GET /api/orders/{id}/artifacts`、`POST /api/artifacts/{id}/share`、`GET /api/artifacts/{id}/download` 均有数据返回。
  - 发现待修复安全问题：登录后即使不带 `Authorization` header，`GET /api/me` 和 `GET /api/orders` 仍会通过进程内 fallback session 返回数据；HTTP 用户端接口应要求显式 token 或明确的安全 cookie/session 机制。
  - 发现待评估最小披露问题：用户端订单、流水、交付物响应当前仍返回 `userId`，订单响应返回用户需求原文；后续需要按用户端 DTO 规则确认哪些字段可返回。
- 2026-06-13 实现：
  - Bearer-only 鉴权已在 issue 00-015 之前的传输层加固中落地（移除进程内 fallback session），未授权返回 `401`。
  - 用户端 DTO（`internal/transport/http/dto.go`）：`/api/me`、余额、流水、购买、订单、交付物、分享全部改用用户端 DTO 输出，砍掉 `userId`、`paymentId`、交付物内部 `preview`、分享 `token`（仅 URL 内嵌）；邮箱只在 `/api/me` 返回；订单需求原文作为属主自有数据保留在响应中。
  - 管理端边界（`internal/transport/http/admin.go`）：独立 `/admin/api` 路由 + `requireAdmin`（`X-Admin-Token`，常量时间比较，未配置即默认关闭返回 `403`）+ slog 审计；不挂在 `/api` 下，桌面 oneshot client 不可达。
  - 日志脱敏（`internal/transport/http/redact.go`）：`redactToken` 掩码、`promptDigest` 仅记录长度；订单创建审计日志用需求摘要而非全文；交付物原文不写日志。
  - 跨用户访问订单/交付物/取消均返回 `404`，不泄露资源归属。
  - 测试：`dto`/`boundary`/`redact` 三组测试覆盖 DTO 无隐私字段（黑盒扫 `userId`/`providerSubject`/`paymentId`/`storageUri`）、跨用户 404、未授权 401、admin 403/200、`/api` 下无 admin 路由、脱敏。`go test ./...`、`-race`、gofmt、vet 全过。
