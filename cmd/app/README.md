# OneCatch App

`cmd/app` 是桌面、iOS 和 Android 的统一 Wails 入口。桌面构建组装本地
store、Workflow orchestrator 与 Agent CLI；移动构建组装远端 Worker
工作台，通过 Worker API 操作远端 Workspace 和 runtime。

## 流式输出

- Codex 通过 app-server 输出消息与命令 delta；本地 Codex CLI 必须支持 app-server。
- Claude Code 使用 `stream-json --include-partial-messages`，Modu Code 使用 print-mode NDJSON。
- provider token 会在后端合并成约 80ms 的增量帧，通过 Wails 事件发送；JSONL 仍是持久化事实源，页面重连时用 revision 快照补偿。
- Agent 与用户正文按 GFM Markdown 渲染。原始 HTML 不执行、远程图片不自动加载，工具日志保持原始等宽文本。
- Token 面板显示步骤累计输入/输出，并拆分缓存读取、缓存写入和推理输出；旧运行会从已保存的 provider usage 只读补齐明细。

## 运行

安装至少一个受支持的本地 Agent CLI 后，在仓库根目录启动：

```bash
go tool wails3 task dev:desktop
```

Workspace、Task、Workflow、Run 和事件默认持久化在 `~/.onecatch/`。

iOS 和 Android 不启动本地 CLI，也不直接操作设备文件系统中的 Git 仓库：

```bash
go tool wails3 task run:ios
go tool wails3 task run:android
```

## 构建

```bash
go tool wails3 task build:desktop
```

生产构建并生成 macOS `.app` ZIP：

```bash
go tool wails3 task package:desktop
```

安装包输出到 `bin/OneCatch-<version>-<commit>-<arch>.zip`。应用内同时包含
`Contents/Resources/bin/onecatch-worker`，可以复制到另一台同架构 Mac 上运行。
默认使用 ad-hoc 签名，适合内部测试；没有 Apple 开发者证书时，首次启动仍需在
Finder 中右键选择“打开”。

使用 Developer ID 证书签名：

```bash
SIGN_IDENTITY="Developer ID Application: Example Corp (TEAMID)" go tool wails3 task package:desktop
```

可通过环境变量覆盖版本号和输出路径：

```bash
VERSION=0.1.0 OUTPUT_ZIP=bin/OneCatch-0.1.0-arm64.zip go tool wails3 task package:desktop
```

脚本只负责构建、组装、签名和压缩。公开分发前还需使用 Apple notary service
完成公证；仅使用 Developer ID 签名不能替代公证。

## 代码位置

```text
cmd/app/                    可执行入口，仅保留 main.go
internal/app/               按 ios/android build tag 选择应用装配
internal/app/desktop/       Desktop 启动装配、平台适配和嵌入资源
internal/app/mobile/        移动工作台装配与远端 Worker 服务
internal/service/desktop/   Desktop 对外服务与运行状态
internal/service/mobile/    移动端远程执行服务
internal/transport/wails/   Wails bindings
frontend/                   React/Vite 前端
build/                      桌面、iOS、Android 构建与打包配置
```
