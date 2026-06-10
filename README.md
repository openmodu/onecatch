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

后端默认会尝试读取 `config/oneshot-server.yaml`；文件不存在时使用内置默认值。可以从示例创建本地配置：

```bash
cp config/oneshot-server.example.yaml config/oneshot-server.yaml
```

也可以通过 `ONESHOT_CONFIG` 指定配置文件路径：

```bash
ONESHOT_CONFIG=/etc/oneshot/server.yaml go run ./cmd/oneshot-server
```

配置文件示例：

```yaml
http:
  addr: ":8080"

mysql:
  addr: "192.168.5.250:43029"
  user: "root"
  password: "<password>"
  database: ""

logger:
  service: "oneshot-server"
  level: "info"
  format: "json"
  file: "logs/oneshot-server.log"
  max_size_mb: 100
  max_backups: 7
  max_age_days: 30
  compress: true
  development: false
  disable_stdout: false
```

环境变量会覆盖配置文件。配置 MySQL 后也可以这样运行后端：

```bash
ONESHOT_MYSQL_ADDR=192.168.5.250:43029 \
ONESHOT_MYSQL_USER=root \
ONESHOT_MYSQL_PASSWORD='<password>' \
go run ./cmd/oneshot-server
```

也可以直接提供 `ONESHOT_MYSQL_DSN`。未配置 MySQL 时，后端保持当前内存 repo 开发模式。

日志也可以通过环境变量覆盖：

```bash
ONESHOT_LOG_LEVEL=debug \
ONESHOT_LOG_FORMAT=json \
ONESHOT_LOG_FILE=logs/oneshot-server.log \
ONESHOT_LOG_MAX_SIZE_MB=100 \
ONESHOT_LOG_MAX_BACKUPS=7 \
ONESHOT_LOG_MAX_AGE_DAYS=30 \
ONESHOT_LOG_COMPRESS=true \
go run ./cmd/oneshot-server
```

未配置 `ONESHOT_LOG_FILE` 时只输出到 stdout；配置后会同时输出到 stdout 和轮转文件。

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
