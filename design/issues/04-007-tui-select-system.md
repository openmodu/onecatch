# Issue 04-007：全局 TUI Select 组件

状态：已完成

## 来源

- PRD：`design/prd/04-mac-desktop-ui-redesign.md`
- 技术方案：`design/docs/04-mac-ui-component-system.md`

## 目标

移除与 TUI 视觉冲突的 macOS 原生选择框和弹出菜单，让任务、设置、Workflow 与 DAG 共用一个可全局维护的 TUI Select。

## 范围

- 新增共享 `TUISelect` primitive。
- 迁移任务 Workflow、Workspace Sandbox、串行步骤、DAG Inspector/transition 和设置页选择框。
- 使用 portal 渲染展开菜单，避免被 Inspector 或滚动容器裁切。
- 支持鼠标、键盘和外部点击关闭。

## 非目标

- 不修改选项值、设置 DTO 或 Workflow 数据结构。
- 不引入第三方 dropdown 组件库。
- 不实现多选、搜索或异步加载选项。

## 产品需求

- 闭合选择框只显示一条底线，不出现上下双横线或双层 focus outline。
- 展开菜单使用无圆角、无投影的矩形面板，不显示 macOS 蓝色原生高亮，也不在底部形成第二层“背板”。
- 当前选项使用 `>` 和语义色标记，hover/active 保持 TUI 风格。
- 菜单支持 ArrowUp/ArrowDown、Home/End、Enter/Space、Escape。
- 禁用选项不能选择，点击外部或滚动时菜单位置保持正确。

## 技术设计

- 后端改动：无。
- 桌面端 / Wails 改动：无。
- 前端改动：`ui/primitives.jsx` 新增 portal dropdown；`ui/tokens.css` 增加 trigger/menu/option 共享规格；所有业务页面改用 options/value/onChange 接口。
- 数据模型改动：无。
- API 或 binding 改动：无。
- 错误码：无。

## 验收标准

- [x] 产品前端不再包含原生 `<select>` / `<option>`。
- [x] 所有选择框闭合态只有一条底线，focus 不出现双重轮廓。
- [x] 展开菜单为矩形 TUI 风格，当前项、活动项和禁用项清晰。
- [x] 任务、设置、串行编辑器与 DAG 编辑器统一生效。
- [x] 键盘选择、Escape 和外部点击关闭可用。
- [x] 前端测试和 production build 通过。

## 测试计划

- 静态：扫描前端源码，确认不存在原生 select/option。
- Browser：检查任务页展开态、键盘选择、外部点击关闭；抽查 Settings 与 DAG 容器裁切。
- Build：执行 `npm test`、`npm run build` 和 `git diff --check`。

## 交付记录

- `TUISelect` 统一归一化字符串/对象 options，保持现有业务值语义。
- 触发器采用单底线和内嵌 cyan focus 状态；菜单采用无圆角、无投影的单层边框。
- 菜单使用 fixed portal，根据 viewport 自动选择向上或向下展开并监听 resize/scroll 更新位置。
- Browser 实测 ArrowDown/Enter、Escape、外部点击关闭均通过；设置页覆盖规则已收敛，展开时 trigger 为 0px 顶边、1px 底边、0px outline，菜单为 fixed/0px radius。
- 视觉对比与修复记录见根目录 `design-qa.md`，最终结果为 passed。
