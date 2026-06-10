# Issue 00-002: 登录与会话

状态：已完成

## 来源

- PRD：`design/prd/00-agent-marketplace-mvp.md`
- 相关原型：`design/prototype/`

## 目标

实现用户登录和会话基础能力，确保用户在创建订单或消耗次数前已完成登录。

## 范围

- 微信登录入口。
- Google 邮箱登录入口。
- 当前用户查询。
- 退出登录。
- 桌面端会话持久化。
- 后端登录回调接口边界。
- 未登录用户禁止创建订单。

## 非目标

- 组织账号。
- 角色权限。
- 企业邮箱域名限制。
- 多账号切换。

## 产品需求

- 登录入口固定在左下角账号区。
- 未登录用户看到微信登录和 Google 登录按钮。
- 已登录用户看到登录来源、账号状态和退出登录按钮。
- 未登录用户不能创建订单或扣减次数。
- 登录和退出登录都需要有明确反馈。

## 技术设计

- 后端接口：
  - `POST /api/auth/wechat/start`
  - `POST /api/auth/wechat/callback`
  - `POST /api/auth/google/callback`
  - `POST /api/auth/logout`
  - `GET /api/me`
- 桌面端 bindings：
  - `AuthBinding.LoginWithGoogle()`
  - `AuthBinding.Logout()`
  - `AuthBinding.CurrentUser()`
- 会话 token 通过桌面端服务保存；生产实现应使用系统 keychain 或加密本地存储。
- 服务端通过 `User` 和 `AuthIdentity` 管理身份映射。
- 当前 MVP 使用开发用内存会话实现微信和 Google 登录回调边界；真实 OAuth provider、token 校验和 keychain 存储按后续生产化任务处理。
- 桌面端默认使用本进程内 shared services client，设置 `ONESHOT_API_BASE_URL` 时切换到外部 HTTP API。
- 桌面端未登录状态下 `AuthBinding.CurrentUser()` 返回空用户，不作为 binding 错误；HTTP `/api/me` 仍返回 401。
- 本 issue 按“后端 HTTP API + 共享 Go client + Wails binding + 前端账号区”一起交付和联调，避免只完成单侧实现。

## 验收标准

- [x] 用户可以从账号区发起微信登录。
- [x] 用户可以从账号区发起 Google 登录。
- [x] 登录状态在侧边栏可见。
- [x] 用户可以退出登录。
- [x] 未登录创建订单会被阻止并展示明确提示。
- [x] 后端接口和桌面 binding 都可以获取当前用户。

## 测试计划

- 已新增并运行 auth usecase 单元测试，覆盖未登录、Google 登录、微信登录和 logout。
- 已新增并运行 HTTP handler 测试，覆盖 `/api/me`、微信 start/callback、Google callback、logout 和未登录创建订单 401。
- 已运行 `env GOCACHE=/private/tmp/oneshot-go-build go test ./...`，通过。
- 已运行 `cd desktop/oneshot/frontend && npm run build`，通过。
- 已运行 `cd desktop/oneshot && env GOCACHE=/private/tmp/oneshot-go-build wails3 build DEV=true`，通过。
- 已运行 `cd desktop/oneshot && wails3 dev -config ./build/config.yml -port 9246`，成功构建、启动 Vite dev server 并连接前端 dev server；未启动外部后端时不再出现 binding 连接错误。
- 已启动后端 `ONESHOT_ADDR=:18080 go run ./cmd/oneshot-server` 并用 curl 联调真实 HTTP API：
  - 未登录 `GET /api/me` 返回 401。
  - 未登录 `POST /api/orders` 返回 401。
  - `POST /api/auth/wechat/start` 返回微信 session。
  - `POST /api/auth/google/callback` 返回 Google session。
  - 登录后 `GET /api/me` 返回当前用户。
  - `POST /api/auth/logout` 后 `GET /api/me` 恢复 401。
- 已用 in-app Browser 验证未登录账号区展示微信和 Google 登录入口、未登录点击“确认并支付”展示“请先登录后再创建订单”、流程 Tab 不进入执行中。
- 已用 in-app Browser 验证 960px 桌面宽度下账号区、主工作区和 Inspector 无横向溢出或裁剪。

## 交付记录

- 当前仓库已有开发用 auth service，会返回本地开发用户。
- 真实 OAuth provider 接入尚未实现。
- 已将 auth usecase 改为开发用内存会话，默认未登录，登录后可通过 `CurrentUser` 获取当前用户，logout 后回到未登录。
- 已新增后端接口边界：`POST /api/auth/wechat/start`、`POST /api/auth/wechat/callback`、`POST /api/auth/google/callback`、`POST /api/auth/logout`、`GET /api/me`。
- 已让未登录访问 `/api/me` 和创建订单返回 401，避免未登录用户扣次或创建订单。
- 已扩展 Go client 和 Wails AuthBinding，支持微信登录、Google 登录、当前用户和退出登录。
- 已将桌面默认 client 改为本进程内 shared services，保留 `ONESHOT_API_BASE_URL` 外部服务模式。
- 已接入前端账号区登录/退出状态和未登录下单提示。
- 已完成后端 HTTP API 与桌面前端 binding 两条路径的联调验证；后续 issue 继续保持前后端一起交付。
- 已在前端本地保存登录来源，用于刷新后继续展示微信或 Google 登录来源；生产 token 持久化仍需后续 keychain/加密存储实现。
- 真实 OAuth、token 校验、系统 keychain/加密存储、第三方账号身份映射仍未实现，需后续 issue 继续细化。
