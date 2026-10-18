# OneCatch

**[English](README.md) | 简体中文**

OneCatch 是一个 local-first 的桌面 Coding Agent 工作台。它在本机调用 Codex、Claude Code 等 Harness，把一次性的 Agent 会话变成可排队、可观察、可介入、可恢复的任务和多 Agent 工作流。

OneCatch 不代管模型账号，也不要求把项目复制到它的服务器。本地工作区仍在原目录，OneCatch 的任务、工作流、运行事件和日志默认保存在 `~/.onecatch/`；模型请求是否联网、会向服务商发送哪些内容，取决于你选择的 Harness 及其配置。

> **项目状态：** OneCatch 仍在快速迭代，发布版本和说明以 [`CHANGELOG.md`](CHANGELOG.md) 最上方的版本小节为准。桌面安装包面向 macOS 12+、Windows 10+，以及带 GTK4/WebKitGTK 6.0 的 Linux 系统；配置格式和交互可能在小版本中变化。远端 Worker 的服务端和调度代码已经进入仓库，但桌面端入口目前只作预览，尚不能启用或管理。

## 能做什么

- **直接运行 Agent**：支持 Codex、Claude Code、Modu Code、Pi、Grok Build 和 DeepSeek Harness。OneCatch 从 `PATH` 自动发现 CLI；Modu Code 还可以使用内嵌 SDK。可用时，你可以指定可执行文件、模型、推理强度和允许继承的环境变量。
- **编排工作流**：工作流是一张步骤图。依赖连线决定顺序，互不依赖的步骤并行执行；审查类步骤可以沿返工连线返回 `rework`，意见会交回给上游步骤原来的会话，只重跑两者之间的步骤。步骤可以是 Agent（各自选择 Harness 和模型），也可以是 E2E 测试之类的 shell 命令。运行中随时可以暂停、给某个步骤留言，或者从任意步骤重跑。
- **在运行中介入**：查看消息、工具调用、命令输出和 token 用量；排队下一轮指令，或打断当前轮次后插入高优先级指令。支持续接的 Harness 会复用原会话。
- **控制访问边界**：每个任务或节点可选择只读、工作区读写或完全访问。完全访问默认关闭；启用后，OneCatch 默认仍会在每次运行前要求确认。
- **管理本地工作区**：浏览和编辑文件，使用多标签/分栏终端，查看 Git 状态与 diff，并让 Agent 只读审查未提交变更。
- **把本地 Agent 接到远端目录**：Remote FS 通过 SSH/SFTP 操作远端工作区，Harness 和模型凭据仍留在本机。当前仅 Codex、Claude Code 和 Modu Code 支持，Windows 桌面端暂不支持这种运行方式。

工作流也可以作为代码放在项目里：把 YAML 文件放进 `.onecatch/workflows/`，OneCatch 会自动读取并画出流程图，文件一改就更新，出错时指出具体行。`after` 是依赖，`rework` 是不通过时可以退回的上游步骤，`agent` 写作“运行环境/模型”，`run` 表示命令步骤：

```yaml
name: 设计到 PR
max_rework: 3
steps:
  design:    { agent: claude/opus, sandbox: read-only, prompt: 给出实现方案，不写代码 }
  implement: { agent: codex, after: design }
  review:    { agent: claude/sonnet, sandbox: read-only, after: implement, rework: [implement, design] }
  e2e:       { run: npm run e2e, after: review, rework: implement }
  pr:        { agent: modu, after: e2e, prompt: 提交并创建 PR }
```

在设置的工作流列表里，任意工作流都可以“导出到项目”生成这样的文件。

首次启动内置单 Agent 完成、实现与审查、设计到 PR（Opus 设计、Codex 实现、Sonnet 审查、Modu 跑 E2E 并提交 PR）和并行审查。你可以在画布编辑器里增加步骤、连接依赖和返工连线，并为每个步骤设置 Harness、模型、沙箱和说明或命令。

## 安装与开发

桌面端使用 Keel v0.3.3，界面和业务代码均由 Go 构建。依赖来自远端版本，Gio 和 typesetting 使用 `go.mod` 中固定的 Keel 分支，不需要本地 Keel 仓库。

需要 Go 1.26.1、Git，以及已配置的 Agent CLI 或 Modu provider。macOS 需要 Xcode Command Line Tools；Linux 的原生图形库可用 `go tool keel doctor` 检查。Node.js 仅用于打包和发布元数据，运行桌面端不需要 npm。

```bash
go mod download
go run ./cmd/keel
go build -trimpath -ldflags='-s -w' -o bin/onecatch ./cmd/app
go test ./...
```

应用启动后，在“设置 → Harness”配置 Agent，添加项目，再创建任务或选择工作流。数据默认保存在 `~/.onecatch/`；可用 `go run ./cmd/keel -data-dir /tmp/onecatch-preview` 指定独立目录。

安装了 Task 命令后，也可以使用 `task dev:desktop`、`task build`、`task test`。原来的 Wails 桌面端、移动端及 React 前端已移除。

各 Harness 的接入差异见 [接入说明](docs/harness-integrations.md)，数据兼容说明见 [数据兼容](docs/data-compatibility.md)。

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

## 手机连接

电脑端“设置 → 手机连接”保留 Worker 服务与配对能力，供兼容客户端接入。本仓库已移除旧移动客户端和移动端打包配置；共享远程协议仍保留。

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
task build:headless
./bin/headless/onecatch worker --pair
```

监听非回环地址时必须通过 `--tls-cert` 和 `--tls-key` 配置 TLS；mTLS 再增加 `--client-ca`。只有传输安全已由受信任隧道承担时，才应显式使用 `--allow-insecure-http`。当前用户的常驻服务可用 `--install-service` 安装，所有参数以 `./bin/headless/onecatch worker --help` 为准。Worker 入口说明见 [`internal/app/worker/README.md`](internal/app/worker/README.md)，部署模板位于 [`deploy/onecatch-worker/`](deploy/onecatch-worker/)。

## 桌面打包发布

```bash
node build/scripts/release-info.mjs --check
node build/scripts/package-keel.mjs
```

版本和发布说明取自 `CHANGELOG.md`。脚本使用 `go tool keel build` 生成原生桌面包，再输出到 `dist/`：macOS 为应用 ZIP，Windows 为便携 EXE ZIP，Linux 为包含安装脚本的 tar.gz。新构建不再生成 DMG、NSIS、AppImage 或移动端安装包；已发布的历史版本不受影响。

macOS 默认 ad-hoc 签名，可通过 `SIGN_IDENTITY` 指定 Developer ID；公证需要另行执行。macOS 更新包继续使用固定公钥校验 Ed25519 签名，Windows 和 Linux 的便携包需手动替换。推送与 CHANGELOG 一致的版本标签后，CI 构建三个平台并发布产物、校验文件和 macOS 更新源。

## 工程结构

```text
cmd/
└── app/                桌面、Worker 与内部辅助进程统一入口
internal/
├── app/                桌面、Worker 与辅助进程装配
├── domain/             领域模型与业务规则
├── repo/               文件、Git 和历史数据访问
├── usecase/            Agent 适配器与工作流用例
└── service/            桌面、Worker 等功能服务
pkg/                    可供仓库外引用的通用 Go 包
build/                  原生桌面构建与打包配置
deploy/                 Worker 的 launchd、systemd 部署模板
docs/                   Harness 等开发文档
```

仓库只维护一个根 `go.mod`。`cmd` 只保留进程入口；仓库内共享代码放在 `internal`，确实需要向外暴露的包才放进 `pkg`。桌面界面位于 `internal/app/keel`。分层边界和新代码归类判据见 [`internal/README.md`](internal/README.md)。

## 参与贡献

Issue 和 Pull Request 都欢迎。提交前请至少运行：

```bash
task test
task build:desktop
```

新增 Harness 时，以 `internal/domain/harnesses/catalog.go` 作为能力目录，并同步补充适配器、配置检查和测试；不要在前端维护第二份运行时事实。

## 许可证

OneCatch 基于 [MIT License](LICENSE) 开源，版权所有 © 2026 OpenModu。
