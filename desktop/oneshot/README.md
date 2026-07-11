# Oneshot Desktop

Wails v3 本地 Agent 调度桌面端。应用进程直接组装本地 store、Workflow
orchestrator 与 Codex / Claude Code runtime，不依赖 `oneshot-server`、登录或远端 API。

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
