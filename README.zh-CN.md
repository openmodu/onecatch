# OneCatch

**[English](README.md) | 简体中文**

OneCatch 是一个 local-first 的桌面 Coding Agent 工作台。它在本机调用 Codex、Claude Code 等 Harness，把一次性的 Agent 会话变成可排队、可观察、可介入、可恢复的任务，以及串行 Loop 和并行 DAG。

OneCatch 不代管模型账号，也不要求把项目复制到它的服务器。本地工作区仍在原目录，OneCatch 的任务、工作流、运行事件和日志默认保存在 `~/.onecatch/`；模型请求是否联网、会向服务商发送哪些内容，取决于你选择的 Harness 及其配置。

> **项目状态：** OneCatch 仍在快速迭代，发布版本以 [`VERSION`](VERSION) 为准。桌面安装包面向 macOS 12+、Windows 10+，以及带 GTK4/WebKitGTK 6.0 的 Linux 系统；配置格式和交互可能在小版本中变化。远端 Worker 的服务端和调度代码已经进入仓库，但桌面端入口目前只作预览，尚不能启用或管理。

## 能做什么

- **直接运行 Agent**：支持 Codex、Claude Code、Modu Code、Pi、Grok Build 和 DeepSeek Harness。OneCatch 从 `PATH` 自动发现 CLI；Modu Code 还可以使用内嵌 SDK。可用时，你可以指定可执行文件、模型、推理强度和允许继承的环境变量。
- **编排工作流**：串行工作流按 Agent 返回的 signal 在步骤间跳转，可以组成“实现 → 审查 → 返工”Loop；DAG 让无依赖节点并行执行，再把结果交给下游节点。
- **在运行中介入**：查看消息、工具调用、命令输出和 token 用量；排队下一轮指令，或打断当前轮次后插入高优先级指令。支持续接的 Harness 会复用原会话。
- **控制访问边界**：每个任务或节点可选择只读、工作区读写或完全访问。完全访问默认关闭；启用后，OneCatch 默认仍会在每次运行前要求确认。
- **管理本地工作区**：浏览和编辑文件，使用多标签/分栏终端，查看 Git 状态与 diff，并让 Agent 只读审查未提交变更。
- **把本地 Agent 接到远端目录**：Remote FS 通过 SSH/SFTP 操作远端工作区，Harness 和模型凭据仍留在本机。当前仅 Codex、Claude Code 和 Modu Code 支持，Windows 桌面端暂不支持这种运行方式。

首次启动会内置三种工作方式：单 Agent 完成、实现与审查 Loop、并行审查 DAG。你也可以在可视化编辑器中修改角色、指令、依赖关系、signal、沙箱和执行上限。

## 安装

### 使用安装包

安装包发布在 [GitHub Releases](https://github.com/openmodu/onecatch/releases)：

- macOS：`OneCatch-<version>-macOS-<arch>.dmg`
- Windows：`OneCatch-<version>-Windows-<arch>-Setup.exe`
- Linux：`OneCatch-<version>-Linux-<arch>.deb` 或 `.AppImage`

每个安装包旁边都有 `.sha256` 校验文件。当前 macOS 包使用 ad-hoc 签名、Windows 包未做 Authenticode 签名，系统可能在首次启动时显示安全提示。公开分发前仍需补齐正式签名和 macOS 公证。

### 从源码运行

你需要：

- Go `1.26.1`，版本以 [`go.mod`](go.mod) 为准；
- Node.js `24.14.0` 和 npm `11.9.0`，版本以 [`frontend/package.json`](frontend/package.json) 为准；
- Git；
- [Wails 3 要求的平台依赖](https://v3.wails.io/quick-start/installation/)；
- 至少一个可用的 Harness：CLI 接入需要先安装并认证相应命令，Modu Code 的内嵌 SDK 接入需要有效的 provider 配置。

仓库通过 Go 的 `tool` 指令固定 Wails CLI，不需要另外执行 `go install wails3`。从仓库根目录运行：

```bash
git clone https://github.com/openmodu/onecatch.git
cd onecatch
go tool wails3 task deps
go tool wails3 task dev:desktop
```

`deps` 会执行 `go mod download` 和 `npm ci`，不会用 `go mod tidy` 或 `npm install` 改写 lockfile。

应用启动后：

1. 在“设置 → Harness”确认至少一个 Agent 显示为可用；
2. 添加本地项目，选择默认文件访问权限；
3. 新建任务，选择单个 Agent 或工作流；
4. 运行任务，在时间线中查看输出、介入或续接会话。

各 Harness 的协议、续接能力和沙箱差异见 [`docs/harness-integrations.md`](docs/harness-integrations.md)。

## 常用开发命令

以下命令都从仓库根目录执行：

```bash
go tool wails3 task deps              # 安装 Go 和前端依赖
go tool wails3 task dev:desktop       # 启动桌面开发环境
go tool wails3 task build:desktop     # 构建桌面开发版本
go tool wails3 task package:desktop   # 生成当前系统的桌面安装包
go tool wails3 task build:worker      # 输出 bin/onecatch-worker
go tool wails3 task test              # 运行 Go 和前端测试
```

iOS 和 Android 客户端仍属实验能力，定位是远端 Worker 工作台，不会在移动设备上运行本地 Agent CLI：

```bash
go tool wails3 task run:ios
go tool wails3 task run:android
```

移动端需要额外的 Xcode 或 Android SDK/NDK 环境。

## 本地数据与安全边界

OneCatch 的主要持久化数据默认写入 `~/.onecatch/`：

```text
~/.onecatch/
├── workspaces/     工作区索引
├── tasks/          任务
├── workflows/      工作流定义
├── runs/           运行快照与 JSONL 事件流
├── locks/          工作区锁
└── logs/           应用日志
```

这里的 local-first 指 OneCatch 自己的数据和调度状态保存在本机，并不代表 Agent 离线运行。Harness 仍会按各自的认证和隐私策略连接模型服务；把任务交给 Agent 前，请确认项目内容允许发送给对应服务商。

任务附件会复制到项目内的 `.onecatch/attachments/`，OneCatch 会尝试把 `.onecatch/` 加入该项目的 `.git/info/exclude`。诊断包默认脱敏，不导出 Worker Token、环境变量值或完整本地路径；任务提示和原始事件必须由用户在导出时单独授权。

## 远端 Worker（开发中）

远端 Worker 在另一台机器上运行 Harness，并通过 Git patch 把 `workspace-write` 的改动同步回桌面端。它与 Remote FS 不同：Remote FS 只把文件和命令操作转发到 SSH 目标，Worker 则把 Harness 进程、工作区和模型环境都放到远端。

**当前桌面版本没有可用的 Worker 配置入口。** 以下命令只用于开发、API 联调和后续功能验证，不能视为已经交付的用户功能。完整入口启用后还必须满足这些约束：

- 桌面端和 Worker 的 Git 工作区都干净，且 `HEAD` 完全相同；
- 桌面端当前提交已推送到 Worker 能访问的 `origin`；
- 单次最多同步 24 MiB 的 tracked 和非 ignored untracked 改动；
- ignored 文件不会同步，包含 Git submodule 的仓库不能执行可写远端任务；
- `full` 访问只允许本地运行。

构建并在回环地址启动开发 Worker：

```bash
go tool wails3 task build:worker
./bin/onecatch-worker --pair
```

监听非回环地址时必须通过 `--tls-cert` 和 `--tls-key` 配置 TLS；mTLS 再增加 `--client-ca`。只有传输安全已由受信任隧道承担时，才应显式使用 `--allow-insecure-http`。当前用户的常驻服务可用 `--install-service` 安装，所有参数以 `./bin/onecatch-worker --help` 为准。Worker 入口说明见 [`cmd/worker/README.md`](cmd/worker/README.md)，部署模板位于 [`deploy/onecatch-worker/`](deploy/onecatch-worker/)。

## 桌面打包与发布

根目录 [`VERSION`](VERSION) 是安装包版本来源，只接受 `X.Y.Z` 三段数字：

```bash
go tool wails3 task package:desktop
```

Windows 打包前需要安装 [NSIS](https://nsis.sourceforge.io/)：

```powershell
winget install NSIS.NSIS
```

Linux 使用 GTK4 和 WebKitGTK 6.0；`.deb` 与 AppImage 都包含桌面端调用的 worker、shell 和 SSH askpass。

正式发布时，先修改并提交 `VERSION`，再推送同版本标签：

```bash
git tag v0.2.0
git push origin v0.2.0
```

标签必须等于 `v` 加 `VERSION` 的内容。GitHub Actions 会构建 macOS DMG、Windows Setup、Linux `.deb` 和 AppImage，校验后把安装包及 SHA-256 文件发布到对应的 GitHub Release。

macOS 使用 Developer ID 签名时设置 `SIGN_IDENTITY`；已经通过 `notarytool store-credentials` 保存公证凭据时，再设置 `NOTARY_PROFILE`：

```bash
SIGN_IDENTITY="Developer ID Application: Example Corp (TEAMID)" \
NOTARY_PROFILE="onecatch-notary" \
  go tool wails3 task package:desktop
```

## 工程结构

```text
cmd/
├── app/                Wails 桌面与移动端统一入口
├── worker/             远端执行服务入口
├── onecatchsh/         Remote FS 命令代理
└── onecatch-askpass/   SSH 密码辅助进程
frontend/               React、Vite、测试和生成的 Wails bindings
internal/
├── app/                Desktop、Mobile 和 Worker 启动装配
├── domain/             领域模型与业务规则
├── repo/               文件、Git 和历史数据访问
├── usecase/            Agent 适配器与工作流用例
├── service/            Desktop、Mobile 和 Worker 服务
└── transport/          Wails 与 HTTP 适配
clients/onecatch/        OneCatch Go HTTP SDK
pkg/                    可供仓库外引用的通用 Go 包
build/                  桌面、iOS、Android 构建与打包配置
deploy/                 Worker 的 launchd、systemd 部署模板
docs/                   Harness 等开发文档
tools/                  Go 工具依赖声明
```

仓库只维护一个根 `go.mod`。`cmd` 只保留进程入口；仓库内共享代码放在 `internal`，确实需要向外暴露的包才放进 `pkg` 或 `clients`。桌面端和移动端共用 `frontend` 中的 React 源码、lockfile、bindings 和构建产物。分层边界和新代码归类判据见 [`internal/README.md`](internal/README.md)。

## 参与贡献

Issue 和 Pull Request 都欢迎。提交前请至少运行：

```bash
go tool wails3 task test
go tool wails3 task build:desktop
```

新增 Harness 时，以 `internal/domain/harnesses/catalog.go` 作为能力目录，并同步补充适配器、配置检查和测试；不要在前端维护第二份运行时事实。

## 许可证

OneCatch 基于 [MIT License](LICENSE) 开源，版权所有 © 2026 OpenModu。
