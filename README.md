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

远端 Worker 可以运行串行 Workflow 或 DAG 节点，实时回传事件和 Claude 权限请求。`workspace-write` 的改动通过 Git binary patch 同步回桌面端；`full` 涉及工作区外文件，仍只允许本地运行。

### 安全启动

先在远端机器准备证书和 32 字节随机 Token：

```bash
install -d -m 700 ~/.config/oneshot-worker
openssl req -x509 -newkey rsa:3072 -nodes -days 365 \
  -subj '/CN=oneshot-worker' \
  -keyout ~/.config/oneshot-worker/server-key.pem \
  -out ~/.config/oneshot-worker/server.pem
chmod 600 ~/.config/oneshot-worker/server-key.pem
openssl rand -hex 32
```

保存最后一条命令输出的 Token，然后构建并启动 Worker：

```bash
go build -o oneshot-worker ./cmd/oneshot-worker
ONESHOT_WORKER_TOKEN='<shared-token>' ./oneshot-worker \
  --listen 0.0.0.0:9231 \
  --id mac-mini \
  --name 'Build Mac mini' \
  --workspace workspace-id=/absolute/path/to/clone \
  --tls-cert ~/.config/oneshot-worker/server.pem \
  --tls-key ~/.config/oneshot-worker/server-key.pem
```

启动日志会打印服务端证书的 SHA-256 指纹。桌面端打开“设置 → 远端 Worker”，填写 `https://<远端地址>:9231`、相同 Token 和这串 64 位指纹。指纹会把桌面端配对到这张证书，自签名证书不需要关闭校验。

使用私有 CA 时填写 CA 文件；需要 mTLS 时再填写客户端证书和私钥，并给 Worker 增加 `--client-ca <ca.pem>`。Bearer Token 在 TLS 和 mTLS 模式下仍会校验。非回环明文 HTTP 默认拒绝启动；只在已有加密隧道承担传输安全时使用 `--allow-insecure-http`。

`--workspace` 可以重复指定。Codex、Claude Code 和 Modu Code 默认从 `PATH` 查找，也可以分别用 `--codex-binary`、`--claude-binary` 和 `--modu-binary` 覆盖。

### 写入同步与恢复

可写步骤启动前必须满足两个条件：

- 桌面端和 Worker 的 clone 都没有 tracked/untracked 改动；
- 两端 `git rev-parse HEAD` 完全相同。

Worker 最多回传 24 MiB 的 tracked 和非 ignored untracked 改动。本地补丁校验并应用成功后，桌面端发送确认，Worker 才重置 tracked 改动并清理这次已同步的新文件。传输中断、HEAD 变化、补丁冲突或确认失败时，远端工作区保持原样；先到远端执行 `git status` 恢复或提交改动，再清理工作区。ignored 文件不会同步，任务不要把交付物写进 ignored 路径。

可写同步暂不支持包含 Git submodule 的仓库：gitlink 补丁不能携带 submodule 工作区里的文件改动，Worker 会在启动 Agent 前拒绝这类任务。只读步骤不受影响。

Claude 权限请求会显示在原任务的审批卡中。允许一次、始终允许和拒绝都会回传到 Worker 上阻塞的 Claude 进程；运行中断或 Worker 离线后，过期审批会返回 `permission_not_pending`。

### 常驻运行

生产安装包把 macOS Worker 放在：

```text
Oneshot.app/Contents/Resources/bin/oneshot-worker
```

macOS `launchd` 和 Linux `systemd` 模板位于 [`deploy/oneshot-worker`](deploy/oneshot-worker/)。复制后替换二进制、工作区、证书和用户名路径；包含 Token 的 plist 或 `/etc/oneshot-worker.env` 权限设为 `0600`。进程收到 `SIGTERM`/`SIGINT` 时会取消在途 Agent，最多等待 30 秒关闭 HTTP 服务。

检查：

```bash
go test ./...
cd desktop/oneshot && wails3 build DEV=true
```
