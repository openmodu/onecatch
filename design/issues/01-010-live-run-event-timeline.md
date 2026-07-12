# Issue 01-010: Live Run 事件时间线

状态：已完成

## 来源

- PRD：`design/prd/01-custom-agent-workflows.md`
- 技术方案：`design/docs/01-custom-agent-workflows.md`
- 选定视觉方案：2026-07-12 最新一组 Run Inspector 方案 2。

## 目标

让 Run Inspector 真实反映 Agent 的执行顺序：消息与每一次 Tool Use 按时间交错展示，工具默认折叠；同时合并重复的 Run、当前步骤、执行流程和 Agent 会话信息。

## 范围

- Runtime events 不再汇总为一个“工具与过程”分组。
- 每个 Tool Use / 过程 / 文件事件成为独立、默认折叠的时间线项。
- Tool Result 就近归入对应 Tool Use 的展开内容。
- Agent message 保持展开并与工具项按 seq 排序。
- Run 标题、状态、当前步骤、转移进度、流程步骤和恢复会话合并为一个紧凑摘要。
- 消息区明确区分用户与 Agent 身份；工具折叠标题从 shell launcher 中提取可读命令，并提供清晰的展开、聚焦和运行中状态。

## 非目标

- 不改变 Agent runtime 原始事件协议。
- 不新增后端事件字段或持久化迁移。
- 不推断运行时没有上报的精确工具耗时。

## 产品需求

- 长命令不得在默认状态撑开 Inspector；用户展开单个工具项后才查看完整内容。
- 多个工具调用不得放进同一个折叠面板。
- 消息、工具、文件变化和过程事件保持原始发生顺序。
- 当前 Run 的核心状态首屏可见；流程和会话恢复细节通过同一个摘要 disclosure 查看。
- 终态 Run 仍可浏览完整事件记录和复制恢复信息。

## 技术设计

- 后端改动：无。
- 桌面端 / Wails 改动：无 binding 变化。
- 前端改动：`runConversation.js` 输出交错的 `message/tool` items；相邻 tool result 绑定到前一个 tool use；从常见 `zsh -lc` launcher 提取可读工具标题；`RunInspector` 使用统一摘要、消息身份标识和独立 `<details>` 工具行。
- 数据模型改动：仅前端派生 view model，持久化结构不变。
- API 或 binding 改动：无。
- 错误码：无。
- 验证计划：纯函数顺序/归并测试、折叠交互、1280×800 视觉对比、深色 token 回归、前端/Wails 构建。

## 验收标准

- [x] Message 与 Tool Use 按 runtime event seq 交错展示。
- [x] 每个 Tool Use 独立且默认折叠，长命令不再占满面板。
- [x] Run/当前步骤/执行流程/Agent 会话合并为一个摘要区域。
- [x] 展开工具项可以查看完整命令及对应结果。
- [x] 用户与 Agent 消息身份清晰，工具项具备可读标题、键盘焦点、展开态和运行态反馈。
- [x] 事件类型不依赖重复文字标签：用户为白色圆点、Agent 为蓝色圆点、工具项沿用展开箭头，并保留无障碍名称。
- [x] 用户、Agent 与工具事件的时间共用右侧基准线；Agent 回复显示其所属的真实 StepRun 轮次。
- [x] 1280×800 下主要正文清晰、控制区不被遮挡，设计 QA 通过。

## 测试计划

- Node：事件交错、message 去重、tool result 归并、fallback message、shell launcher 标题提取。
- Browser：默认折叠、单项展开、摘要 disclosure、滚动与 sticky 操作区。
- Build：`npm test`、`npm run build`、`wails3 build DEV=true`。

## 交付记录

- `runConversation.js` 改为按 seq 生成交错的 `message/tool` view model；tool result 只归入相邻 tool use，不合并不同调用。
- Run Inspector 顶部收敛为约 60px 的两行 sticky summary；Workflow steps、Run ID、runtime sessions 和恢复命令统一放入“会话与恢复” disclosure。
- 每个工具、过程和文件事件使用独立 `<details>`，闭合高度 50px；展开后展示完整内容和结果。
- 移除与方案 2 不一致的“第 N 轮”分组标题和 Agent 左侧粗状态线，消息直接按事件顺序平铺；用户/Agent 正文为 13px、400 weight。
- 消息元数据仅保留 time/runtime；工具项补齐 hover、focus、open 与 running 反馈，并使用 Phosphor caret 作为标题前的展开入口。
- 最终视觉标记按用户反馈进一步简化：移除可见的 `USER / AGENT / TOOL USE / FILE CHANGE` 标签；用户消息使用带轻微描边的白色 Phosphor 圆点，Agent 消息使用蓝色圆点，工具标题获得额外横向空间，事件类型继续通过 aria label 暴露。
- 时间列统一为 55px 并贴近内容区右边缘；用户消息、Agent 消息和工具行的时间 right edge 完全一致。Agent 消息在时间左侧显示 `第 N 轮`，N 来自 `buildRunConversation` 已有的 StepRun 顺序；初始任务时间缺少 createdAt/startedAt 时回退到 updatedAt。
- 工具状态在固定 45px 状态列内右对齐，缩短状态文字与时间列之间的视觉距离，同时不改变时间和 caret 的统一基准线。
- Tool disclosure caret 移到工具标题前：折叠为向右、展开为向下。右侧不再预留 caret gutter，用户、Agent 与工具时间列一起贴近内容区右边缘并继续对齐。
- 圆点与 tool caret 共用 `--ui-event-marker-size: 9px` 首列，marker 左边缘和中心线一致；caret 使用正文前景 token，在浅色主题呈黑色、深色主题保持高对比。
- 工具摘要优先展示去除 `zsh -lc` 包装后的可读动作，如“读取文件”“运行 npm”“检查 git diff”；完整原始内容仍保留在展开区，并用 `COMMAND / PROCESS / PATH / RESULT` 分区。
- `workflow_signal` 不再作为原始暂停原因重复显示；其他 pause reason 和 last error 仍保留。
- 验证：Node 13 项测试、前端 production build、`go test ./...`、Wails DEV build、1280×800 Browser 交互与 overflow 检查全部通过。Wails 仅有既有 macOS link target 警告。
- 设计 QA：根目录 `design-qa.md`，`final result: passed`。
