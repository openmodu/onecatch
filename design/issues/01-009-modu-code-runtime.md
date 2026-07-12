# Issue 01-009: Modu Code 自定义 Runtime

状态：已完成

## 来源

- PRD：`design/prd/01-custom-agent-workflows.md`
- 技术方案：`design/docs/01-custom-agent-workflows.md`

## 目标

把本机 `modu-code` 作为一等 Runtime 接入 Oneshot，让用户可在设置中配置命令、Provider、模型和允许继承的环境变量，并在串行 Workflow 与本地 DAG 节点中选择它。

## 范围

- Runtime 注册表、设置模型、设置 UI 和 Workflow runtime 选择支持 `modu`。
- 通过 ACP JSON-RPC 2.0 LDJSON 完成 initialize、session/new、session/prompt 和流式消息归一化。
- 支持 Provider、默认模型、OpenAI-compatible base URL/API Key 等环境变量配置边界。
- 明确当前 Modu Code session 只在子进程内有效，不生成无效的恢复命令。

## 非目标

- 不修改 Modu Code 本身增加持久会话。
- 不实现通用任意 shell Runtime 模板。
- 不在 Oneshot 设置文件中保存 API Key 值。
- 不为尚未实现的 ACP 文件系统、终端或权限反向请求伪造支持。

## 产品需求

- 设置页显示 Modu Code 卡片，可配置 binary、Provider、默认模型和环境变量 Key 白名单。
- Provider 支持自动检测、OpenAI、Anthropic、Gemini。
- 测试配置只检查 binary 是否可执行，不发起模型请求。
- Workflow/DAG 可选择 Modu Code；运行时把任务 prompt 作为 ACP text prompt 发送，并持久化归一化 Agent 消息与结果。
- Modu Code 当前不支持跨进程恢复时，人工恢复或 Loop 再入创建新 ACP session，并依靠 Oneshot prompt 中的任务、最近 outcome 和人工指令衔接上下文。

## 技术设计

- 后端改动：新增 `RuntimeModu` 与 `ModuRunner`，使用 stdio ACP JSON-RPC；Engine/Registry 增加 `ModuBinary`。
- 桌面端 / Wails 改动：沿用现有 Settings/Runtime bindings，DTO 的 RuntimeSettings 向后兼容增加 `provider`。
- 前端改动：设置页增加 Modu Code 卡片与 Provider 下拉框，demo/runtime 选择同步增加 `modu`。
- 数据模型改动：`RuntimeSettings.Provider`；schemaVersion 保持 1，旧配置 Normalize 自动补齐 `modu`。
- API 或 binding 改动：无新增方法，现有 runtime map 新增 `modu` key。
- 错误码：沿用 `runtime_unknown`、`runtime_unavailable`、`runtime_draft_unavailable`。
- 前后端联调点：Modu `version` 展示 ACP 可用状态；不返回短生命周期 session ID。

## 验收标准

- [x] 设置页可保存并测试 Modu binary、Provider、model 和环境变量白名单。
- [x] Modu runtime 可被 Workflow 和 DAG 选择，ACP stub 能完整执行并输出最终消息。
- [x] Provider/model 以环境变量传递，API Key 只从用户允许的宿主环境继承。
- [x] ACP 错误、进程错误和取消能转换为明确失败，不误推进 Workflow。
- [x] Modu 不展示不可用的 resume command，Codex/Claude 行为不回归。

## 测试计划

- Go：ACP 握手、prompt、chunk 聚合、RPC error、环境变量、Engine 路由与 settings normalize/validate。
- 前端：Settings runtime 校验与 production build。
- 回归：`go test ./...`、`npm test`、`npm run build`、`git diff --check`。

## 交付记录

- 新增 `RuntimeModu` 和 ACP stdio runner，完成 initialize、session/new、session/prompt、agent_message_chunk 聚合、RPC error 与 context 进程退出处理。
- Runtime settings 向后兼容新增 `modu` 和 `provider`；Provider/环境白名单随 Run 快照冻结，避免运行中修改全局设置改变既有 Run。
- 设置页增加 Modu Code 卡片、Provider 下拉框、binary/model/API Key allowlist 配置与 ACP session 边界说明；Workflow 和 DAG 的现有 runtime 列表自动出现 Modu Code。
- Modu 默认命令为 `modu-code`，本机可配置 `/Users/ityike/Code/go/bin/modu-code`；配置测试仅检查 executable，避免 `--version` 启动 ACP provider 或消耗额度。
- 当前 Modu session 是子进程内短生命周期 ID，Result 不持久化该 ID，因此工作台不会展示无效恢复命令；恢复依靠新 session 加 Oneshot prompt 上下文。
- 验证：`go test ./...`、前端 `npm test`（10 项）、`npm run build`、`wails3 build DEV=true`、`git diff --check` 全部通过。Wails 只有既有的 macOS 12 object / macOS 11 link target 警告。
