# Oneshot

Oneshot 是一个 local-first 的桌面 Agent 调度编排工具。用户可以在自己的 Workspace 中调用 Codex、Claude Code 等本地 CLI，把“实现 → 审查 → 返工”之类的工作方法保存为可观察、可暂停、可恢复的自定义 loop。

当前产品主线见 [`design/prd/01-custom-agent-workflows.md`](design/prd/01-custom-agent-workflows.md)。仓库中已有的服务市场、登录、计费和订单代码属于早期探索，保留作历史参考，不约束新的本地编排实现。

本地数据默认以 JSON/JSONL 文件保存在 `~/.oneshot/`，runtime 事件流位于 `~/.oneshot/runs/`。测试和开发可以显式覆盖数据根目录。

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
