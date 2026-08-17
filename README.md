# OneCatch

OneCatch 是一个 local-first 的桌面 Agent 调度编排工具。用户可以在自己的 Workspace 中调用 Codex、Claude Code 等本地 CLI，把“实现 → 审查 → 返工”之类的工作方法保存为可观察、可暂停、可恢复的自定义 loop。

当前产品主线见 [`design/prd/01-custom-agent-workflows.md`](design/prd/01-custom-agent-workflows.md)。仓库中已有的服务市场、登录、计费和订单代码属于早期探索，保留作历史参考，不约束新的本地编排实现。

本地数据默认以 JSON/JSONL 文件保存在 `~/.onecatch/`，runtime 事件流位于 `~/.onecatch/runs/`。测试和开发可以显式覆盖数据根目录。

## 工程结构

```text
cmd/
├── app/                Wails 桌面与移动端统一入口
├── onecatchfs/          macOS 远端文件系统挂载工具
└── worker/             远端执行服务入口
frontend/               React/Vite 前端、测试和生成的 Wails bindings
internal/
├── app/                Desktop、Worker 和 Server 的启动装配
├── domain/             领域模型与业务规则
├── repo/               SQL、文件、Git 等数据访问
├── usecase/            按业务能力拆分的用例
├── service/            Desktop、Worker 和 Server 的对外服务
└── transport/          Wails 与 HTTP 适配
clients/onecatch/        OneCatch Go HTTP SDK
pkg/                    可被外部项目引用的通用 Go 包
build/                  桌面、iOS、Android 构建配置和共享依赖任务
deploy/                 Worker 的 launchd、systemd 部署模板
design/                 PRD、issue、设计文档和 prototype
web/landing/            落地页，单文件静态站点，无构建步骤
tools/                  Go 工具依赖声明
```

仓库只维护一个根 `go.mod`。`cmd` 只保留进程入口，不承载可复用业务逻辑；
仓库内共享代码放进 `internal`，确实需要向仓库外暴露的包才放进 `pkg` 或
`clients`。React/Vite 源码统一放在 `frontend`，桌面端和移动端共用同一份
`package.json`、lockfile、bindings 和构建产物，再由 Go 嵌入对应二进制。
`internal` 的分层边界和新代码归类判据见
[`internal/README.md`](internal/README.md)。

## 常用命令

以下命令都从仓库根目录执行：

```bash
go tool wails3 task deps              # 按 lockfile 安装 Go 和 JavaScript 依赖
go tool wails3 task dev:desktop       # 启动 Wails 和 Vite 开发环境
go tool wails3 task build:desktop     # 构建桌面开发版本
go tool wails3 task build:ios         # 构建 iOS 模拟器版本
go tool wails3 task build:android     # 构建 Android 版本，需要 Android NDK
go tool wails3 task build:worker      # 输出 bin/onecatch-worker
go tool wails3 task build:onecatchfs   # 输出 bin/onecatchfs，仅支持 macOS
go tool wails3 task test              # 运行 Go 和前端测试
```

`go.mod` 与 `frontend/package.json` 仍由各自工具管理，但安装入口只有
`go tool wails3 task deps`。构建过程执行 `go mod download` 和 `npm ci`，不会用
`go mod tidy` 或 `npm install` 隐式改写 lockfile。

也可以直接调用底层命令：

```bash
go tool wails3 dev -config ./build/desktop/config.yml
```

## 把 Linux 目录挂载到 macOS

`onecatchfs` 通过一条持久 SSH/SFTP 连接，把 Linux 上的项目显示为 macOS 本地目录。Codex、编辑器和普通文件工具可以直接读写挂载点；Linux 只需运行 `sshd`，不需要安装 Codex 或 OneCatch。文件内容不会复制到 Mac，本地只缓存文件属性和目录列表，默认缓存 1 秒。

安装 macFUSE，确认 `ssh <host>` 可以使用密钥或 ssh-agent 无交互登录，然后执行：

```bash
wails3 task build:onecatchfs
mkdir -p "$HOME/Volumes/linux-project"
./bin/onecatchfs \
  --host dev-linux \
  --root /home/dev/project \
  --mount "$HOME/Volumes/linux-project"
```

`--host` 可以直接使用 `~/.ssh/config` 中的别名、`ProxyJump` 和密钥设置。保持进程运行，在 Codex 中打开挂载目录；按 `Ctrl-C` 会卸载文件系统。完整参数、边界和测试方法见 [`docs/remote-filesystem.md`](docs/remote-filesystem.md)。

挂载只改变文件访问路径，不会把 Mac 上启动的进程转移到 Linux。`go test`、`npm test` 等依赖 Linux 环境的命令仍需通过 SSH 在远端执行。

## 远端 Worker

远端 Worker 可以运行串行 Workflow 或 DAG 节点，实时回传事件和 Claude 权限请求。`workspace-write` 的改动通过 Git binary patch 同步回桌面端；`full` 涉及工作区外文件，仍只允许本地运行。

### 安全启动

先在远端机器准备证书：

```bash
install -d -m 700 ~/.config/onecatch-worker
openssl req -x509 -newkey rsa:3072 -nodes -days 365 \
  -subj '/CN=onecatch-worker' \
  -keyout ~/.config/onecatch-worker/server-key.pem \
  -out ~/.config/onecatch-worker/server.pem
chmod 600 ~/.config/onecatch-worker/server-key.pem
```

构建并启动 Worker。首次启动会在 `~/.onecatch-worker/` 生成持久 Token，并打印一个 10 分钟内有效、只能使用一次的配对码：

```bash
go build -o onecatch-worker ./cmd/worker
./onecatch-worker \
  --listen 0.0.0.0:9231 \
  --id mac-mini \
  --name 'Build Mac mini' \
  --tls-cert ~/.config/onecatch-worker/server.pem \
  --tls-key ~/.config/onecatch-worker/server-key.pem
```

桌面端打开“设置 → 远端 Worker”，填写 `https://<远端地址>:9231` 和启动日志中的配对码。桌面端会换取 Token，并自动记录本次连接看到的服务端证书 SHA-256 指纹；之后每次连接都固定到这张证书。配对码过期或已使用时，在 B 侧用相同参数增加 `--pair` 重启即可生成新码。

选择项目后，在 Worker 卡片点击“准备到 B”。Worker 会使用 A 侧项目的 `origin` 自动 clone 到 `<data-dir>/projects/<workspace-id>`（默认是 `~/.onecatch-worker/projects/...`），或对已有干净 clone 执行 fetch，再以 detached HEAD 切换到 A 当前提交；映射保存在 `<data-dir>/workspaces.json`，重启不会丢失。A 当前提交必须已推送到 B 能访问的远端，B 也需要预先配置相应 SSH 或 HTTPS 仓库凭证。

“准备到 B”是可选的提前检查。工作流派发远端节点时也会自动完成同样的准备；同一 Worker、项目和提交上的并发节点会合并准备请求，避免重复 clone/fetch。桌面设置页每 15 秒刷新 Worker 在线状态和连接延迟。

使用私有 CA 时可在手动配置中填写 CA 文件；需要 mTLS 时再填写客户端证书和私钥，并给 Worker 增加 `--client-ca <ca.pem>`。Bearer Token 在 TLS 和 mTLS 模式下仍会校验。非回环明文 HTTP 默认拒绝启动；只在已有加密隧道承担传输安全时使用 `--allow-insecure-http`。

Codex、Claude Code 和 Modu Code 默认从 `PATH` 查找，也可以分别用 `--codex-binary`、`--claude-binary` 和 `--modu-binary` 覆盖。

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
OneCatch.app/Contents/Resources/bin/onecatch-worker
```

macOS `launchd` 和 Linux `systemd` 模板位于 [`deploy/onecatch-worker`](deploy/onecatch-worker/)。复制后替换二进制、数据目录、证书和用户名路径；首次启动的配对码会写入服务日志。进程收到 `SIGTERM`/`SIGINT` 时会取消在途 Agent，最多等待 30 秒关闭 HTTP 服务。

也可以直接安装当前用户的常驻服务，不需要手工复制模板：

```bash
./onecatch-worker \
  --install-service \
  --pair \
  --listen 0.0.0.0:9231 \
  --id mac-mini \
  --name 'Build Mac mini' \
  --tls-cert /absolute/path/server.pem \
  --tls-key /absolute/path/server-key.pem
```

命令会安装并立即启动 macOS LaunchAgent 或 Linux user systemd unit，并打印查看服务日志的命令。请先把 `onecatch-worker` 放到不会被移动或删除的固定绝对路径。

检查：

```bash
go tool wails3 task test
go tool wails3 task build:desktop
```
