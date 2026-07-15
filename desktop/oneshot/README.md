# Oneshot Desktop

Wails v3 本地 Agent 调度桌面端。应用进程直接组装本地 store、Workflow
orchestrator 与 Codex / Claude Code / Modu Code runtime，不依赖 `oneshot-server`、登录或远端 API。

## 流式输出

- Codex 优先使用 app-server 的消息与命令输出 delta，旧版 CLI 自动回退到 `codex exec --json`。
- Claude Code 使用 `stream-json --include-partial-messages`，Modu Code 使用 print-mode NDJSON。
- provider token 会在后端合并成约 80ms 的增量帧，通过 Wails 事件发送；JSONL 仍是持久化事实源，页面重连时用 revision 快照补偿。
- Agent 与用户正文按 GFM Markdown 渲染。原始 HTML 不执行、远程图片不自动加载，工具日志保持原始等宽文本。
- Token 面板显示步骤累计输入/输出，并拆分缓存读取、缓存写入和推理输出；旧运行会从已保存的 provider usage 只读补齐明细。

## 运行

安装至少一个受支持的本地 Agent CLI 后直接启动：

```bash
wails3 dev -config ./build/config.yml
```

Workspace、Task、Workflow、Run 和事件默认持久化在 `~/.oneshot/`。

## 构建

```bash
wails3 build DEV=true
```

## 目录

```text
app/          应用元信息
bindings/     Runtime、Settings、Workspace、Workflow、TaskRun、Worker Wails bindings
frontend/     React/Vite 前端和生成 bindings
build/        Wails 构建配置
```
