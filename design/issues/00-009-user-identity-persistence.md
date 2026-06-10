# Issue 00-009: 用户与身份持久化

状态：已完成

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
- 实际实现保留开发降级：未配置 MySQL 时 `internal/repo/users` 使用内存映射；配置 MySQL 时通过 GORM 写入 `users` 和 `auth_identities`。
- auth usecase 不再直接持有固定 dev user 作为唯一来源，而是按 provider 身份 profile 调用 users repo 查找或创建用户。
- 当前仍使用开发 provider subject；真实 provider subject 获取归入 `00-010`。

## 验收标准

- [x] MySQL schema 包含 `users` 表。
- [x] MySQL schema 包含 `auth_identities` 表。
- [x] 重复微信登录返回同一个用户 ID。
- [x] 重复 Google 登录返回同一个用户 ID。
- [x] 微信和 Google 身份可以分别映射到用户。
- [x] `GET /api/me` 返回持久化用户。
- [x] 未登录 `GET /api/me` 仍返回 401。
- [x] repo 层不依赖 `internal/data`。

## 测试计划

- 已新增 users repo 单元测试，覆盖重复 provider subject、微信/Google 映射到同一开发用户、缺少 provider subject 的错误路径。
- 已新增 users repo MySQL schema 集成测试，使用 `ONESHOT_MYSQL_TEST_DSN` 时验证 `users` 和 `auth_identities` 表；本地未配置 DSN 时跳过。
- 已更新 auth usecase 测试，覆盖 repo-backed 登录、logout 和重复微信登录同用户 ID。
- 已更新 HTTP handler 测试，覆盖登录 callback、`/api/me`、logout、未登录创建订单 401。
- 已运行 `env GOCACHE=/private/tmp/oneshot-go-build go test ./...`，通过。
- 已运行 `go run github.com/google/wire/cmd/wire ./cmd/oneshot-server` 重新生成 server wire。

## 交付记录

- 当前 `00-002` 只实现开发用内存会话和固定本地用户。
- 本 issue 补齐用户表、身份映射表和持久化 repo。
- 已新增 `internal/domain/users.AuthIdentity` 和用户状态/时间字段。
- 已新增 `internal/repo/users`，接口定义在 repo 包内，支持无 MySQL 时内存映射、配置 MySQL 时 GORM 自动迁移并写入 `users` 与 `auth_identities`。
- 已将 auth usecase 改为依赖 users repo 接口，通过 provider 身份查找或创建用户后再建立会话。
- 已将 server wire、`internal/data.OneShotRepo` 和桌面本地 client 接入 users repo。
- 当前 provider subject 仍是开发固定值；真实微信/Google subject 获取仍属于 `00-010`。
