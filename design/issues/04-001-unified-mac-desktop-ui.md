# Issue 04-001：统一 Mac 桌面端 UI

状态：已完成

## 来源

- PRD：`design/prd/04-mac-desktop-ui-redesign.md`
- 技术方案：`design/docs/04-mac-desktop-ui-redesign.md`
- 视觉原型：`design/Mac应用UI设计优化/Oneshot.dc.html`

## 目标

把现有 React/Wails 桌面端完整迁移到用户确认的 Mac/TUI 视觉与交互系统，同时保留所有真实业务能力。

## 范围

- App shell、Mac titlebar、cwd rail、command strip、导航和 Runtime footer。
- Task/Run composer、运行列表和 Inspector。
- Workflow 行式列表、串行编辑器、DAG 编辑器。
- Settings 五分区、危险确认、冲突和 dirty 状态。
- 浅色/深色 token、响应式、键盘焦点和视觉 QA。

## 非目标

- 后端协议、调度语义和持久化格式变化。
- 新增业务页面或远端 Worker 能力。

## 产品需求

- 页面结构和主要文案以视觉原型为准，真实内容来自现有 state/binding。
- 核心动作维持现有行为，改版不能引入只可看不可用的控件。
- 编辑器从 modal 改为主工作区内页面，返回后保留 Workflow 列表上下文。
- 保存、运行、危险操作和 validation 必须提供明确状态反馈。

## 技术设计

- 后端：无新增接口。
- Wails：保留现有 bindings。
- 前端：重构 App shell 和主要视图 JSX；以 `mirage-tokens.css` 为基准重写样式 token 与组件样式。
- 数据模型：无变化。
- 联调：现有 demo 和 Wails 模式共用新 UI。

## 验收标准

- [x] Tasks、Workflows、serial editor、DAG editor、Settings 与原型同一视觉语言。
- [x] 所有核心交互与现有 binding 联调通过。
- [x] 浅色/深色 token、键盘焦点、滚动和基准窗口通过验收。
- [x] `design-qa.md` 为 passed。

## 测试计划

- frontend production build。
- Wails dev build。
- `go test ./...`、`go test -race ./internal/...`、`go vet ./...`。
- 浏览器逐页交互和同视口截图对照。

## 交付记录

- 新增 `mirage.css`，以原型 token 统一浅色/深色主题、Shell、页面、编辑器、DAG 和设置样式。
- 重构 `App.jsx` 的 Mac titlebar、sidebar、command strip、Tasks、Workflow library 和两个编辑器布局；保留真实 bindings 与状态机。
- 调整 `SettingsPage.jsx` 的远端 Worker 与命令式动作文案。
- 通过 frontend production build、Wails dev build、`go test ./...`、`go test -race ./internal/...`、`go vet ./...` 和 `git diff --check`。
- 在 1360 × 860 对照五个核心页面，浏览器主流程和 console 检查通过；详见根目录 `design-qa.md`。
