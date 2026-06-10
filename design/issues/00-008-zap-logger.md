# Issue 00-008: Zap 日志封装

状态：已完成

## 来源

- PRD：`design/prd/00-agent-marketplace-mvp.md`
- 相关原型：`design/prototype/`

## 目标

在 `pkg/` 下封装统一 zap logger，让后端和桌面入口使用同一套日志能力，并支持文件日志轮转。

## 范围

- 新增 `pkg/logger` 基础日志封装。
- 支持 zap console/json 输出。
- 支持 stdout 和文件输出。
- 使用 lumberjack 管理文件日志轮转。
- 后端通过配置文件读取日志配置。
- 环境变量可以覆盖配置文件中的日志配置。
- 替换当前项目中的 `slog` 和标准库 `log.Fatal` 使用点。

## 非目标

- 接入远程日志平台。
- 接入 trace/span 上下文。
- 为每个 usecase/repo 注入独立 logger。

## 产品需求

- 本次不改变用户可见行为。
- 服务端日志格式、级别和轮转参数可配置。
- 服务端配置文件应包含 logger 配置段。
- 未配置文件日志时，开发环境仍能在 stdout 看到日志。

## 技术设计

- 后端改动：
  - `pkg/logger` 暴露 `Config`、`New`、`MustNew`、`Sync`。
  - `internal/app/config` 读取 `config/oneshot-server.yaml` 中的 `logger` 配置段。
  - `ONESHOT_LOG_*` 环境变量覆盖配置文件。
  - `pkg/server` 接收 `*zap.Logger`。
  - `cmd/oneshot-server` 使用统一 logger 初始化和错误输出。
- 桌面端 / Wails 改动：
  - `desktop/oneshot/main.go` 使用 `pkg/logger`。
- 前端改动：无。
- 数据模型改动：无。
- API 或 binding 改动：无。

## 验收标准

- [x] 项目中不再使用 `internal/app/logger`。
- [x] 服务端入口和 `pkg/server` 使用 `*zap.Logger`。
- [x] 桌面入口使用统一 logger。
- [x] 服务端配置文件包含 logger 配置段。
- [x] 环境变量可以覆盖配置文件中的 logger 配置。
- [x] 配置 `ONESHOT_LOG_FILE` 后由 lumberjack 管理文件轮转。
- [x] `go test ./...` 通过。

## 测试计划

- `go test ./...`：通过。
- `internal/app/config` 单元测试覆盖默认值、YAML 读取、env 覆盖和显式坏路径报错。
- `rg -n "log/slog|internal/app/logger|\"log\"" . -g '*.go'`：无残留。
- `ONESHOT_ADDR=127.0.0.1:0 ONESHOT_LOG_FILE=/private/tmp/oneshot-zap-test.log ONESHOT_LOG_FORMAT=json go run ./cmd/oneshot-server`：stdout 和文件日志均输出正常。
- `ONESHOT_CONFIG=/private/tmp/oneshot-config-file-test.yaml go run ./cmd/oneshot-server`：配置文件中的 `logger.service`、`logger.format` 和 `logger.file` 生效，stdout 和文件日志均输出正常。

## 交付记录

- 新增 `pkg/logger`，封装 zap logger、stdout/file sink、console/json encoder 和 lumberjack 文件轮转。
- 后端配置新增 `config/oneshot-server.yaml` 文件读取，`logger` 段用于配置日志。
- 保留 `ONESHOT_LOG_*` 环境变量读取，用于覆盖配置文件。
- 服务端入口、`pkg/server`、桌面入口已统一使用 `pkg/logger`。
- 删除旧的 `internal/app/logger` slog 封装。
