# Oneshot Worker

Oneshot Worker 是可独立部署的远端执行服务。它通过 HTTP API 接收桌面端派发的
Workflow 节点，在目标机器调用 Codex、Claude Code 或 Modu Code，并把运行事件与
工作区改动同步回桌面端。

从仓库根目录构建：

```bash
wails3 task build:worker
./bin/oneshot-worker --help
```

生产环境的 TLS、配对和常驻服务配置见仓库根目录
[`README.md`](../../README.md#远端-worker)，launchd 与 systemd 模板位于
[`deploy/oneshot-worker`](../../deploy/oneshot-worker/)。
