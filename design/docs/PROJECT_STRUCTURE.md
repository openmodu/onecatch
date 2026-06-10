# 项目结构

Oneshot 是一个包含 Go 后端和 Wails v3 桌面端的 monorepo。后端负责 API、持久化、支付回调、订单执行和交付物；桌面端不直接依赖后端内部代码，只通过根目录 `clients/oneshot` SDK 调用后端 HTTP API。

## 当前结构

```text
oneshot/
  cmd/
    oneshot-server/        # 后端 API 入口，main.go 和 wire.go 同级

  clients/
    oneshot/               # Oneshot HTTP SDK，供桌面端和外部 Go client 使用

  desktop/
    oneshot/               # Wails v3 桌面端
      main.go
      wire.go
      wire_gen.go
      client.go            # 桌面端 API base URL
      bindings/            # Wails bindings，只包装 clients/oneshot
      frontend/            # React/Vite 前端
      build/               # Wails 构建资源

  internal/
    api/                   # API 层抽象，目前封装 chi router
    app/
      config/              # 配置
      logger/              # 日志
    domain/                # 领域对象和值对象
    data/                  # 数据连接和生命周期管理
    repo/                  # 业务数据封装
    usecase/               # 领域业务组合
    service/               # 上层应用组合
    transport/
      http/                # HTTP handler 和路由挂载

  pkg/
    sql/                   # MySQL/GORM 基础封装
    httpx/                 # HTTP JSON 请求/响应基础封装
    server/                # HTTP server 启动和优雅退出

  design/
    prd/                   # PRD
    issues/                # 从 PRD 拆出的交付 issue
    docs/                  # 设计文档
    prototype/             # 产品交互原型

```

## 后端分层

后端采用改良 DDD 分层：

```text
transport/http
  -> service
  -> usecase
  -> repo
  -> pkg

data
  -> pkg
  -> repo
```

### `internal/domain`

领域对象、值对象和领域状态定义。

要求：

- 不依赖 HTTP、数据库、Wails、SDK。
- 不放外部平台细节。
- 可被 usecase、repo、transport 使用。

### `internal/data`

数据连接和生命周期管理层。

职责：

- 定义 `Data` 结构，统一管理底层资源句柄，例如 SQL、Redis、对象存储。
- 负责初始化和关闭这些资源的生命周期。
- 定义 `OneShotRepo` 聚合结构，集合各业务 repo 接口，便于 Wire 装配和上层依赖接口。
- 不封装用户、订单、计费等业务数据方法；这些方法必须放在 `internal/repo` 的业务模块里。

### `internal/repo`

业务数据封装层。

职责：

- 封装用户、订单、Agent、计费等业务数据访问。
- 每个业务模块独立建包，例如 `repo/agents`、`repo/orders`、`repo/billing`。
- 每个业务 repo 包内定义接口和未导出实现，例如 `AgentsRepo` + `agentsImpl`。
- repo 可以依赖 `pkg/sql.Sql`、Redis client 等基础资源句柄。
- 不暴露底层 SQL、连接池或存储细节。
- 不依赖 `internal/data`，实现 usecase 所需的接口。

### `internal/usecase`

领域业务组合层。

职责：

- 组合 domain 规则和 repo 操作。
- 表达核心业务动作，例如创建订单、扣减次数、查询 Agent。
- 不依赖 HTTP handler 或 Wails。

### `internal/service`

上层应用组合层。

职责：

- 聚合多个 usecase，形成应用服务集合。
- 给 transport/http 等入口层注入统一依赖。
- 不承载具体数据管理逻辑。

### `internal/transport/http`

HTTP 适配层。

职责：

- 注册 API 路由。
- 解析请求。
- 调用 service/usecase。
- 返回响应。

路由框架使用 `chi`，但 `chi` 被包在 `internal/api` 抽象后面。

## 桌面端分层

桌面端不能直接 import 后端 `internal/...`。

当前依赖方向：

```text
desktop/oneshot/bindings
  -> clients/oneshot
  -> 后端 HTTP API
```

职责划分：

- `desktop/oneshot/bindings`：暴露给 Wails 前端的方法，薄包装 SDK。
- `clients/oneshot`：HTTP SDK，封装后端 API。
- 业务规则仍由后端 `internal/usecase` 和 `internal/service` 承担。

## 依赖注入

依赖注入使用 Wire。

规则：

- Wire 文件放在入口同级。
- 后端 Wire：`cmd/oneshot-server/wire.go`
- 桌面 Wire：`desktop/oneshot/wire.go`
- 生成文件：对应目录下的 `wire_gen.go`

不要再创建全局 `internal/di`。

## 设计资料

设计资料统一放到 `design/`：

- `design/prd/`：PRD。
- `design/issues/`：开发交付 issue。
- `design/docs/`：架构和设计文档。
- `design/prototype/`：交互原型。

## 当前约束

- 不提前创建纯占位目录；需要接数据库、支付、对象存储、后台任务时再新增对应目录。
- `pkg/` 只放真正通用、可被其他模块复用的基础库。
- MySQL/GORM 基础连接封装放在 `pkg/sql`，业务 SQL 操作仍然放在 `internal/repo`。
- Oneshot API SDK 放在根目录 `clients/`，不放 `pkg/sdk`。
- 桌面端通过 `clients/oneshot` 调后端 API。
- 后端内部业务代码不暴露给桌面端直接调用。
- 数据连接和资源生命周期使用 `internal/data`，业务数据封装使用 `internal/repo`。
- 依赖倒置原则不能破坏：usecase 只依赖接口，repo 实现接口，Wire 负责装配。
