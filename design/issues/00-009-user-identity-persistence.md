# Issue 00-009: 用户与身份持久化

状态：待开发

## 来源

- PRD：`design/prd/00-agent-marketplace-mvp.md`
- 相关 issue：`design/issues/00-002-auth-session.md`

## 目标

实现 `User` 和 `AuthIdentity` 的 MySQL 持久化能力，让微信和 Google 身份可以稳定映射到同一个用户，并为订单、余额、流水和交付物的用户归属提供生产数据基础。

## 范围

- `users` 表设计与迁移。
- `auth_identities` 表设计与迁移。
- `internal/repo/users` 包，定义用户与身份仓储接口和实现。
- auth usecase 通过 repo 查询或创建用户。
- 登录回调按 provider 身份查找用户，首次登录自动创建用户和身份映射。
- `GET /api/me` 返回持久化用户。

## 非目标

- 真实微信 OAuth 流程。
- 真实 Google OAuth 流程。
- 系统 keychain 或加密本地 token 存储。
- 组织、角色、权限。
- 后台用户管理。

## 产品需求

- 同一个微信账号重复登录应识别为同一个用户。
- 同一个 Google 账号重复登录应识别为同一个用户。
- 用户退出后再次登录，用户 ID 不应变化。
- 用户信息至少包含展示名、头像和邮箱。
- 微信无邮箱时，系统仍应允许创建用户。

## 技术设计

- 新增 `internal/domain/users.AuthIdentity` 领域对象。
- 新增 `internal/repo/users`：
  - `UsersRepo` 接口放在 repo 包内。
  - 实现结构体使用未导出命名。
  - 构造函数返回接口。
- 建议表结构：
  - `users(id, display_name, email, avatar_url, status, created_at, updated_at)`。
  - `auth_identities(id, user_id, provider, provider_subject, email, display_name, avatar_url, created_at, updated_at)`。
- 唯一约束：
  - `auth_identities(provider, provider_subject)` 唯一。
  - `users.email` 可为空；非空唯一是否启用需结合微信无邮箱场景确认。
- `internal/data` 只负责组合 repo，不写用户查询逻辑。
- Wire 组装 users repo 并注入 auth usecase。

## 验收标准

- [ ] MySQL schema 包含 `users` 表。
- [ ] MySQL schema 包含 `auth_identities` 表。
- [ ] 重复微信登录返回同一个用户 ID。
- [ ] 重复 Google 登录返回同一个用户 ID。
- [ ] 微信和 Google 身份可以分别映射到用户。
- [ ] `GET /api/me` 返回持久化用户。
- [ ] 未登录 `GET /api/me` 仍返回 401。
- [ ] repo 层不依赖 `internal/data`。

## 测试计划

- users repo 单元测试或集成测试。
- auth usecase 身份查找和首次创建测试。
- HTTP 登录 callback + `/api/me` 测试。
- MySQL schema 集成测试。
- 手动联调微信/Google 开发回调与桌面账号区状态。

## 交付记录

- 当前 `00-002` 只实现开发用内存会话和固定本地用户。
- 本 issue 补齐用户表、身份映射表和持久化 repo。
