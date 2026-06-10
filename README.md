# Oneshot

Oneshot 是一个 Go monorepo，包含后端服务和 Wails v3 桌面端。

## 目录

```text
cmd/                    进程入口，目前只有 server
clients/oneshot/        Oneshot HTTP SDK
desktop/oneshot/        Wails v3 桌面端
internal/domain/        领域对象和值对象
internal/data/          数据连接和生命周期管理
internal/repo/          业务数据封装
internal/usecase/       领域业务组合
internal/service/       上层应用组合
internal/transport/     HTTP 适配层
pkg/                    通用基础封装
design/                 PRD、issue、设计文档和 prototype
```

## 开发

运行后端：

```bash
go run ./cmd/oneshot-server
```

配置 MySQL 后运行后端：

```bash
ONESHOT_MYSQL_ADDR=192.168.5.250:43029 \
ONESHOT_MYSQL_USER=root \
ONESHOT_MYSQL_PASSWORD='<password>' \
go run ./cmd/oneshot-server
```

也可以直接提供 `ONESHOT_MYSQL_DSN`。未配置 MySQL 时，后端保持当前内存 repo 开发模式。

运行桌面端：

```bash
cd desktop/oneshot
wails3 dev -config ./build/config.yml
```

检查：

```bash
go test ./...
cd desktop/oneshot && wails3 build DEV=true
```
