# Issue 01-007: Runtime 会话恢复信息

状态：已完成

## 来源

- PRD：`design/prd/01-custom-agent-workflows.md`
- 技术方案：`design/docs/01-custom-agent-workflows.md`

## 目标

在 Run 工作台中直接展示每个本地 Agent 步骤的 thread/session ID 与可复制恢复命令，让用户可以快速在对应 Runtime 客户端中接管会话。

## 范围

- 按 Workflow 步骤汇总当前可恢复的 Runtime session。
- 展示完整 session ID、Runtime、步骤名称和恢复命令。
- Codex 生成 `codex resume <session-id>`；Claude Code 生成 `claude --resume <session-id>`。
- 支持分别复制 session ID 和恢复命令，并给出成功/失败提示。
- 浏览器 demo 数据覆盖 Codex 与 Claude Code 两类会话。

## 非目标

- 不从 OneShot 自动启动外部 Terminal 或 Codex App。
- 不改变 OneShot 自身的暂停、恢复和 session 复用语义。
- 不为尚未支持的 Runtime 猜测恢复命令。

## 产品需求

- 一个步骤多次 attempt 复用同一 session 时只展示一次当前会话。
- session ID 必须完整可见，不得只显示短 ID。
- 没有 session 的 Run 不展示空区块。
- 复制失败时保留可手动选择的命令文本并提示错误。

## 技术设计

- 后端改动：无；复用 `Run.sessions` 与 `StepRun.sessionIdAfter/sessionIdBefore`。
- 桌面端 / Wails 改动：无新增 binding。
- 前端改动：新增纯函数汇总步骤会话并生成 Runtime 专属 shell 命令；Run Inspector 新增“Agent 会话”区块。
- 数据模型改动：无。
- API 或 binding 改动：无。
- 错误码：无；剪贴板失败走现有 toast。
- 前后端联调点：优先使用 `Run.sessions[stepId]`，旧数据回退到该步骤最新 StepRun 的 session ID。

## 验收标准

- [x] Codex 步骤显示 thread ID 和 `codex resume` 命令。
- [x] Claude Code 步骤显示 session ID 和 `claude --resume` 命令。
- [x] 多 attempt 不重复显示同一步骤的当前 session。
- [x] ID 与命令可以复制，且前端与 Wails 构建通过。

## 测试计划

- Node 单元测试：Runtime 命令、Run.sessions 优先级、旧 StepRun 回退和无 session 场景。
- 前端 production build。
- Wails DEV build。

## 交付记录

- 新增 `sessionCommands.js`，按 Workflow 步骤汇总 `Run.sessions`，并兼容从最新 StepRun 恢复旧数据。
- Run Inspector 新增“Agent 会话”区块，完整展示 Runtime、步骤、thread/session ID 和恢复命令，支持分别复制 ID 与命令。
- Clipboard API 被 WebView 权限拒绝时回退到临时 textarea + `execCommand("copy")`；失败会保留可选择文本并显示 toast。
- Node 测试覆盖 Codex/Claude 命令、Run session 优先级、多 attempt 去重、旧数据回退与空 session。
- 验证：`npm test`（7 项）、`npm run build`、`wails3 build DEV=true`、`git diff --check` 均通过；Wails 仅有既有 macOS link target 版本警告。
- 真实浏览器预览检查通过：1280×720 下两个会话卡片未溢出，长 ID/命令可读，点击复制命令出现成功提示。
