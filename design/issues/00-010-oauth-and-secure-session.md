# Issue 00-010: 真实 OAuth 与安全会话

状态：待开发

## 来源

- PRD：`design/prd/00-agent-marketplace-mvp.md`
- 相关 issue：`design/issues/00-002-auth-session.md`
- 前置 issue：`design/issues/00-009-user-identity-persistence.md`

## 目标

接入真实微信和 Google OAuth 登录，并实现桌面端安全会话保存，让用户可以在真实账号体系下完成登录、退出和当前用户查询。

## 范围

- 微信 OAuth 启动和回调流程。
- Google OAuth 启动和回调流程。
- OAuth state 防 CSRF。
- provider token 换取身份信息。
- 服务端签发 Oneshot 会话 token。
- 桌面端保存会话 token。
- HTTP client 和 Wails binding 自动携带会话 token。
- logout 清理服务端会话和桌面端本地 token。

## 非目标

- 企业域名限制。
- 多账号切换。
- 组织权限。
- 支付账号绑定。
- 后台用户风控。

## 产品需求

- 用户可以从账号区完成真实微信登录。
- 用户可以从账号区完成真实 Google 登录。
- 登录完成后侧边栏展示真实账号信息。
- 应用重启后，在 token 未过期时保持登录状态。
- 退出登录后本地和服务端会话都被清理。
- OAuth 失败时展示明确错误反馈。

## 技术设计

- 后端接口沿用：
  - `POST /api/auth/wechat/start`
  - `POST /api/auth/wechat/callback`
  - `POST /api/auth/google/callback`
  - `POST /api/auth/logout`
  - `GET /api/me`
- `start` 接口返回授权 URL、state 或桌面可用启动信息。
- 服务端校验 OAuth state 和 provider 返回结果。
- 服务端通过 `auth_identities(provider, provider_subject)` 查找用户。
- 会话 token 不记录在日志中。
- 桌面端 token 生产实现优先使用系统 keychain；临时实现必须避免明文敏感凭证长期落盘。
- HTTP client 增加 Authorization header 支持。

## 验收标准

- [ ] 微信真实登录可完成授权并返回当前用户。
- [ ] Google 真实登录可完成授权并返回当前用户。
- [ ] OAuth state 校验失败时拒绝登录。
- [ ] 应用重启后可恢复登录状态。
- [ ] logout 后 `/api/me` 返回 401。
- [ ] token 不出现在服务端日志中。
- [ ] 桌面端未配置 token 时不会误判为已登录。

## 测试计划

- OAuth state 单元测试。
- provider callback 错误路径测试。
- 会话 token 签发、校验、过期测试。
- HTTP client Authorization header 测试。
- 桌面端登录、重启、退出手动联调。

## 交付记录

- 当前 `00-002` 只实现开发用登录边界。
- 当前没有真实 OAuth provider 和安全 token 持久化。
