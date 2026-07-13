# PRD 04：Mac 桌面端统一 UI 改版

状态：已完成

## 背景

当前桌面端以卡片、圆角面板和大面积侧栏模块为主，任务、Workflow、编辑器、DAG 与设置之间的视觉语言不统一，信息密度也不适合本地 Agent 编排工具。`design/Mac应用UI设计优化/Oneshot.dc.html` 已给出新的可交互视觉基准。

## 目标

- 将桌面端统一为接近原生 Mac 工具的 TUI 风格：等宽字体、矩形分区、细分隔线、命令行式动作和克制的语义色。
- 让任务运行、Workflow 管理、串行编辑器、DAG 画布和设置中心保持一致的信息架构与交互反馈。
- 保留现有 Wails binding、真实数据、运行控制、Workflow 编辑和设置持久化能力，不把改版退化成静态原型。
- 支持浅色与系统深色模式，保证核心信息和状态在两种主题下可辨识。

## 非目标

- 不改变 Agent 调度、Workflow 定义、Run 状态机和本地持久化协议。
- 不新增云同步、账号、插件市场或远端 Worker 协议。
- 不复刻原型中的 mock 数据和仅用于展示的版本号。

## 用户流程

1. 用户从左侧 `cwd` 区选择或加入工作目录。
2. 用户在“任务与运行”中创建 Run，并在右侧 Inspector 观察、暂停、恢复或终止。
3. 用户在 Workflow 列表中创建或编辑串行 Loop / 并行 DAG。
4. 编辑器在主工作区内打开，并持续显示 flow preview、policy 和 validation。
5. 用户在设置页按 section 修改本机 Runtime、执行、安全、存储和远端 Worker 配置。

## 功能需求

- Shell 包含 Mac titlebar、216px rail、44px command strip 和 local 状态。
- 主导航只保留任务、Workflow、设置；Runtime 与本地数据位置合并到 rail 底部。
- 产品定位为个人工作台，品牌副标题使用 `personal workspace`，不再以 `local agents` 把产品描述成 Agent 列表。
- macOS 窗口使用接近 Codex 的系统 unified toolbar：保留原生交通灯及工具栏留白，不使用 CSS 仿制窗口控制按钮；双击标题栏切换窗口放大与还原。
- 标题栏文字不可选择，双击或拖动时不得出现文字选区和底部灰白分隔闪线。
- macOS 普通窗口使用接近现代原生应用的 26pt continuous corner；放大或全屏时取消圆角，恢复窗口时重新应用。
- 页面主要依赖分隔线、留白和排版分组，不使用卡片墙、渐变背景和大面积阴影。
- 主动作采用 `[ action ]` 文案形式；危险、警告、成功和运行状态使用固定语义色。
- 操作按钮默认使用纯文本 TUI 文案，不使用 plus、close、search、pin 等图标；对话事件圆点和 disclosure caret 作为状态/展开标记保留。
- Workflow 列表为横向信息行，串行和 DAG 编辑器不再以遮罩 modal 呈现。
- DAG 保留拖拽、连接、删除依赖、节点 Inspector 与校验能力。
- DAG 顶部操作使用独立、有间距、不可压缩的纯文本 TUI Action；保存动作与节点动作不得重叠或拼接成一块，Inspector 删除动作不使用图标按钮。
- 设置中心保留独立 draft/save/reset、inline validation、冲突恢复和危险确认。
- 核心输入、按钮、导航和编辑器必须支持键盘焦点状态。
- 原型中重复出现的尺寸、间距、字体、控件和分区结构必须由共享 token 与 UI primitive 管理，禁止页面各自复制一套近似值。
- 同类型组件只允许通过 variant 表达语义差异；基础尺寸或间距调整应能在一个位置全局生效。
- 所有选择控件使用统一的 TUI dropdown，不调用不可定制的系统原生 select 弹层；闭合态只保留一条底线，展开态使用矩形列表、文本标记和一致的键盘交互。
- 桌面端文字必须使用统一的可读性字号层级；辅助信息不得小于 11px，标签、元数据、正文和标题需要保持稳定层级，不能因页面不同退回 7–10px 的局部规格。

## 数据对象

不新增领域对象。继续使用现有 Workspace、Task、Run、WorkflowDefinition、Worker、SettingsView DTO。

## 接口边界

- 不新增后端 API 或 Wails binding。
- 前端继续调用现有 RuntimeBinding、WorkspaceBinding、WorkflowBinding、TaskRunBinding、WorkerBinding 和 SettingsBinding。
- UI 改版不得改变 binding 入参、响应和错误码。

## 验收标准

- [x] 五个核心页面与 `design/Mac应用UI设计优化/Oneshot.dc.html` 的布局、密度、颜色和排版保持同一设计语言。
- [x] 任务创建/运行控制、Workflow 编辑、DAG 画布与设置保存仍可用。
- [x] 基准窗口无关键操作裁切；较窄窗口可滚动而不丢失主操作。
- [x] 浅色与系统深色主题 token 均已覆盖。
- [x] frontend production build、Wails dev build、Go test/race/vet 通过。
- [x] 同视口 design QA 无 P0/P1/P2 差异，`design-qa.md` 结果为 passed。
- [x] 任务、Workflow、Inspector、设置与 DAG 编辑器共用可读性字号 token，最小可见字号不低于 11px。

## 待确认问题

- 首版不提供显式主题切换，跟随系统主题；后续如需手动切换另拆 issue。
