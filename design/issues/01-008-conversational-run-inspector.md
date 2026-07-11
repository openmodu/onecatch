# Issue 01-008: 会话式 Run Inspector

状态：已完成

## 来源

- PRD：`design/prd/01-custom-agent-workflows.md`
- 技术方案：`design/docs/01-custom-agent-workflows.md`
- 视觉证据：用户提供的 2026-07-11 工作台 Run Inspector 截图。

## 目标

把 Run 详情从小字号“步骤摘要 + 原始日志”改成可阅读的会话时间线，让用户能按轮次看清自己提出的目标、人工补充、Agent 消息和工具活动。

## 范围

- 合并“步骤记录”和“Agent 动态”为一个按时间排序的会话区。
- 初始 Task 与后续人工恢复指令显示为用户消息。
- 每个 StepRun 显示轮次头、Runtime、状态、时间和完整 Agent 消息。
- 工具调用、文件变化、推理和工具结果按连续活动分组并默认折叠。
- 去掉最后 8 条事件截断；读取当前 RunDetail 已加载的全部 runtime events。
- 提升 Inspector 标题、元信息、正文、命令和操作区字号，并适当加宽右栏。

## 非目标

- 不改变 Codex/Claude 的原始 JSONL 格式。
- 不把隐藏的模型思维过程伪装成聊天消息。
- 不实现跨 Run 的云端对话同步。

## 产品需求

- 两次 StepRun 必须明确显示为“第 1 轮 / 第 2 轮”，且每轮包含对应 Agent 消息。
- outcome JSON 在 UI 中只展示可读的 `content`，不把协议外壳当正文。
- 工具活动可展开查看完整命令，但默认不压过 Agent 回复。
- 正文不得继续使用 8–9px 字号；Inspector 主要正文不低于 12px。
- 老 Run 没有人工指令正文时仍可展示已有任务和 Agent 消息。

## 技术设计

- 后端改动：`run.resumed` 本地 workflow event payload 增加经过 trim 的 `instruction`，保留 `hasInstruction`。
- 桌面端 / Wails 改动：不新增 binding；继续复用 `RunDetail.events/runtimeEvents/stepRuns`。
- 前端改动：新增纯函数构造按时间排序的会话条目，解析 outcome JSON、按 StepRun 分组并折叠连续工具活动。
- 数据模型改动：WorkflowEvent payload 向后兼容增加可选字段，无 schema 迁移。
- API 或 binding 改动：无。
- 错误码：无。
- 前后端联调点：旧事件没有 `instruction` 时忽略用户补充气泡；Task prompt 始终作为第一条用户消息。

## 验收标准

- [x] 两轮执行在会话区中显示为两张独立轮次卡片。
- [x] 初始任务和新产生的恢复指令显示为用户气泡。
- [x] 所有 Agent message/result/error 均按原顺序展示，outcome JSON 转为可读正文。
- [x] 工具活动默认折叠且可展开。
- [x] 1280×720 下主要正文清晰可读，前端/Wails 构建通过。

## 测试计划

- Node：轮次分组、outcome 正文解析、消息去重、工具活动分组、恢复指令事件解析。
- Go：Workflow usecase 回归与全量测试。
- 视觉：浏览器 demo 状态检查 1280×720 布局和展开交互。
- 集成：production build 与 Wails DEV build。

## 交付记录

- Run Inspector 移除分离的“小字号步骤摘要 + 最后 8 条倒序日志”，改为 Task/人工指令用户气泡与按 StepRun 分组的 Agent 回合。
- 新增 `runConversation.js`：加载 RunDetail 的全部 runtime events，解析 outcome JSON content、去重 Claude message/result、按轮次汇总消息，并把工具/推理/文件事件收进默认折叠组。
- `run.resumed` payload 向后兼容增加 trim 后的 `instruction`，新产生的人工补充可在重启后恢复为用户消息；旧 Run 保持可读。
- Inspector 宽度从 400px 提升至 480px；标题、当前步骤、流程、会话正文、命令和操作区字号统一提升，主要消息正文为 13px。
- 真实浏览器预览验证：任务用户气泡、两张轮次卡片、三条 Agent 回复和完整 signal 均可见；工具组默认折叠，展开后完整命令可见。验收图保存于 `/tmp/oneshot-run-inspector-audit/`。
- 验证：`go test ./...`、`npm test`（10 项）、`npm run build`、`wails3 build DEV=true`、`git diff --check` 全部通过；Wails 仅输出既有 macOS link target 版本警告。
