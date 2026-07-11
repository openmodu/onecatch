# Issue 04-002：Mac UI 组件系统与像素收敛

状态：已完成

## 来源

- PRD：`design/prd/04-mac-desktop-ui-redesign.md`
- 技术方案：`design/docs/04-mac-ui-component-system.md`
- 视觉原型：`design/Mac应用UI设计优化/Oneshot.dc.html`

## 目标

将第一轮页面级 CSS 覆盖重构为统一的 token 和 UI primitive，并按原型的 1280×800 内容区域重新校准尺寸、间距与排版。

## 范围

- 提取壳层、排版、间距、控件、分区、表单和编辑器尺寸 token。
- 建立 Action、Kicker、Badge、Field、Panel、Toolbar 等共享 primitive。
- 迁移 Tasks、Workflow、serial editor、DAG editor、Settings 的重复结构。
- 以原型实际内容区域做同视口截图和重点区域对照。

## 非目标

- 改变调度、持久化、Workflow 数据模型或 Wails binding。
- 新增业务页面、远端 Worker 协议或新的编辑器能力。

## 产品需求

- 同类按钮、字段、状态标签、section 和 toolbar 在所有页面使用相同高度、内边距、字号和交互态。
- 所有页面间距只能使用命名 spacing token，局部例外必须有明确语义。
- 1280×800 实际窗口内的区域比例与原型一致，不使用外围演示画布参与尺寸比较。
- 修改共享 token 或 primitive 后，其所有消费者同步生效。

## 技术设计

- 新增 `src/ui/`：组件 primitive 与 token 样式。
- `mirage.css` 只保留页面布局和组件组合，不重复声明基础控件规格。
- `App.jsx`、`SettingsPage.jsx` 迁移到共享组件，业务 state 与 handlers 保持不变。
- 不新增后端、Wails、数据模型和错误码。

## 验收标准

- [x] 共享 token 覆盖原型壳层、间距、字体和控件规格。
- [x] 五个核心页面的重复控件使用 UI primitive 或共享 control token。
- [x] 1280×800 同视口对照无 P0/P1/P2 尺寸和间距差异。
- [x] 核心交互、frontend build、Wails build 和 Go 测试通过。
- [x] 根目录 `design-qa.md` 更新并为 passed。

## 测试计划

- 在 in-app browser 用 1280×800 分别捕获原型内容和实现。
- 对壳层、Workflow row、表单字段、editor toolbar、DAG Inspector 做局部对照。
- 运行 frontend production build、Wails dev build、Go test/race/vet。

## 交付记录

- 新增 `src/ui/tokens.css`，集中 ACP 色彩、字体、spacing、app geometry、control 和 section 规格。
- 新增 `src/ui/primitives.jsx`，提供 Action、Kicker、Status/Mode Badge、Panel、Field、NumberField、ToggleRow 和 Toolbar。
- Settings 删除页面内重复的 Field/Panel/Toggle 实现；Tasks、Workflow、Run controls 和 serial toolbar 迁移到共享 primitive。
- 默认 Wails 窗口改为 1280×800，原生隐藏标题栏高度改为 38px，与原型 app frame 一致。
- Workflow row、serial step panel、DAG node/layout 和 Settings Runtime panel 完成第二轮尺寸对照。
- frontend/Wails build、Go test/race/vet 和 diff check 通过，视觉验收见根目录 `design-qa.md`。
