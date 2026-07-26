# Oneshot

Oneshot 是一个 local-first 的桌面 Agent 调度编排工具。用户可以在自己的 Workspace 中调用 Codex、Claude Code 等本地 CLI，把“实现 → 审查 → 返工”之类的工作方法保存为可观察、可暂停、可恢复的自定义 loop。

当前产品主线见 [`design/prd/01-custom-agent-workflows.md`](design/prd/01-custom-agent-workflows.md)。仓库中已有的服务市场、登录、计费和订单代码属于早期探索，保留作历史参考，不约束新的本地编排实现。

本地数据默认以 JSON/JSONL 文件保存在 `~/.oneshot/`，runtime 事件流位于 `~/.oneshot/runs/`。测试和开发可以显式覆盖数据根目录。

## 目录

```text
cmd/                    远端 Worker 进程入口
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

运行桌面端：

```bash
cd desktop/oneshot
wails3 dev -config ./build/config.yml
```

## 远端 Worker

远端 Worker 适合受信任的局域网或 VPN。它不复制项目文件：协调端和远端机器必须各自准备代码副本，并使用相同的 Workspace ID。远端步骤目前只允许 `read-only`，写入和合并仍由协调端执行。

在远端机器启动 Worker：

```bash
go build -o oneshot-worker ./cmd/oneshot-worker
ONESHOT_WORKER_TOKEN='<shared-token>' ./oneshot-worker \
  --listen 0.0.0.0:9231 \
  --id mac-mini \
  --name 'Build Mac mini' \
  --workspace workspace-id=/absolute/path/to/clone
```

`--workspace` 可以重复指定。Codex、Claude Code 和 Modu Code 默认从 `PATH` 查找，也可以分别用 `--codex-binary`、`--claude-binary` 和 `--modu-binary` 覆盖。

在桌面端打开“设置 → 远端 Worker”，启用远端调度，保存后注册 Worker 地址和相同的 Token。健康检查通过后，串行 Workflow 和 DAG 节点都可以选择该 Worker；切换到远端时，编辑器会把节点调整为 `read-only`。

生产安装包把 macOS Worker 放在：

```text
Oneshot.app/Contents/Resources/bin/oneshot-worker
```

不要把 Worker 的明文 HTTP 端口直接暴露到公网。需要跨网络连接时，使用 Tailscale、WireGuard 等受信任隧道。当前协议没有 TLS/mTLS、设备配对和文件同步；远端 Claude 权限请求会明确拒绝并回传事件，不会等待桌面端审批。

检查：

```bash
go test ./...
cd desktop/oneshot && wails3 build DEV=true
```
