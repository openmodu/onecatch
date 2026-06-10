# Issue 00-002: 登录与会话

状态：待开发

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

## 验收标准

- [ ] 用户可以从账号区发起微信登录。
- [ ] 用户可以从账号区发起 Google 登录。
- [ ] 登录状态在侧边栏可见。
- [ ] 用户可以退出登录。
- [ ] 未登录创建订单会被阻止并展示明确提示。
- [ ] 后端接口和桌面 binding 都可以获取当前用户。

## 测试计划

- 单元测试 auth service 行为。
- 测试 `/api/me` handler。
- 测试 logout handler。
- 手动验证桌面端未登录和已登录状态。

## 交付记录

- 当前仓库已有开发用 auth service，会返回本地开发用户。
- 真实 OAuth provider 接入尚未实现。
