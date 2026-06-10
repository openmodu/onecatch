# Issue 00-007: MySQL 连接初始化

状态：已完成

## 来源

- PRD：`design/prd/00-agent-marketplace-mvp.md`
- 相关原型：`design/prototype/`

## 目标

接入后端 MySQL 基础连接，让服务启动时可以根据环境配置初始化 SQL 资源，并在数据库不可用时明确失败。

## 范围

- MySQL 连接配置读取。
- `internal/data` 初始化 SQL 资源。
- Wire 注入 MySQL 资源、服务集合和 cleanup。
- MySQL 连通性验证测试入口。

## 非目标

- 创建业务数据库和表结构。
- 将 Agent、订单、计费仓储迁移为真实 SQL 查询。
- 引入 migration 工具。

## 产品需求

- 本次不改变用户可见行为。
- 未配置 MySQL 时保留当前开发期内存仓储行为。
- 配置 MySQL 后，服务启动前必须验证连接可用。
- MySQL 不可连接时，服务端启动失败并暴露明确错误。

## 技术设计

- 后端配置支持 `ONESHOT_MYSQL_DSN`。
- 未提供 DSN 时，可通过 `ONESHOT_MYSQL_ADDR`、`ONESHOT_MYSQL_USER`、`ONESHOT_MYSQL_PASSWORD`、`ONESHOT_MYSQL_DATABASE` 拼接 DSN。
- `internal/data` 负责 MySQL 生命周期管理，连接失败向入口返回错误。
- Wire injector 返回 `services`、`cleanup` 和 `error`。

## 验收标准

- [x] 未配置 MySQL 时 `go test ./...` 通过。
- [x] 配置 MySQL 后可以完成 MySQL ping。
- [x] MySQL 连接失败时服务启动失败并返回明确错误。
- [x] Wire 生成文件与声明同步。
- [x] 未创建业务库时，可以不指定 database 完成连接验证。

## 测试计划

- 单元/编译测试：`go test ./...`，通过。
- Wire 生成验证：`go run github.com/google/wire/cmd/wire ./cmd/oneshot-server ./desktop/oneshot`，通过。
- 手动连通性：`nc -vz 192.168.5.250 43029`，通过。
- MySQL ping 集成测试：`ONESHOT_MYSQL_TEST_DSN='root:<password>@tcp(192.168.5.250:43029)/?charset=utf8mb4&parseTime=true&loc=Local' go test ./pkg/sql -run TestMySQLPingWithDSN -count=1 -v`，使用用户提供的凭据验证通过。
- 后端启动验证：使用 `ONESHOT_MYSQL_ADDR=192.168.5.250:43029`、`ONESHOT_MYSQL_USER=root`、`ONESHOT_MYSQL_PASSWORD=<password>` 启动后端，服务成功进入监听。

## 交付记录

- 已接入后端 MySQL 配置读取、`internal/data` 资源初始化、Wire cleanup 和 ping 验证。
- 未配置 MySQL 时仍返回空 SQL 资源，保持当前内存 repo 开发模式。
- 修复 `pkg/sql.NewMySQL` 在未提供 `gorm.Config` 时向 `gorm.Open` 传 nil option 导致 panic 的问题。
- 当前仅完成连接层对接，业务数据仍未迁移到 MySQL 表；后续需要单独 issue 处理 schema、migration 和 repo 持久化。
