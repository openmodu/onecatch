# Issue 00-016: Inspector 菜单与右侧内容面板

状态：已完成

## 来源

- PRD：`design/prd/00-agent-marketplace-mvp.md`
- 相关原型：`design/prototype/README.md`
- 相关 issue：`design/issues/00-014-desktop-prototype-alignment.md`
- 技术方案：`design/docs/TECHNICAL_DESIGNS.md#issue-00-016-inspector-菜单与右侧内容面板`
- 视觉参考：
  - Claude Code 顶部工具菜单：`/var/folders/nz/tjb3cj6s3cb3jrvrp27yf9x00000gn/T/codex-clipboard-66981bb5-1261-4be5-92c0-f8bbdfd3aa03.png`
  - Claude Code 选择 Diff 后右侧面板：`/var/folders/nz/tjb3cj6s3cb3jrvrp27yf9x00000gn/T/codex-clipboard-dbf054fa-203a-41d4-93f2-d40ddaff7e64.png`

## 目标

将当前 `打开 Inspector` 的直接展开行为，改为 Claude Code 风格的顶部工具菜单：用户点击右上角 Inspector 触发器后出现菜单，选择某个菜单项后再弹出右侧内容面板，面板内展示对应上下文内容并可关闭。

## 范围

- 右上角 Inspector 触发器改为图标/按钮式工具入口。
- 点击触发器时显示轻量浮层菜单。
- 菜单项以 Oneshot 业务语义映射 Claude Code 的工具项：
  - `预览`：交付物预览。
  - `明细`：扣次、价格、订单和流水明细，对应 Claude Code `Diff` 的“查看具体内容”交互。
  - `订单`：当前订单信息。
  - `记录`：使用记录。
  - `进度`：执行进度。
- 选择菜单项后，右侧弹出内容面板。
- 右侧面板顶部展示当前面板标题、上下文路径和关闭按钮。
- 关闭按钮只关闭右侧面板，不影响中间工作台状态。
- 面板打开后仍保持中间工作台可滚动、可继续操作。
- 保留现有 Wails binding 和后端接口，不新增 API。

## 非目标

- 不引入 Claude Code 的 Code/Cowork/Chat 左侧模式。
- 不做真实代码 Diff。
- 不新增 Terminal、Files、Background tasks 等编码工具能力。
- 不改订单、计费、交付物后端模型。
- 不改支付或登录流程。

## 产品需求

- 用户点击右上角 Inspector 入口时，先看到一个小菜单，而不是直接展开重型右栏。
- 用户选择 `明细` 后，右侧弹出类似 Claude Code `Diff` 的内容面板。
- 面板展示具体内容，不再只是一组摘要卡片。
- 用户可通过面板右上角 `X` 关闭面板。
- 用户再次点击 Inspector 入口可切换到其他上下文面板。
- 右侧面板不得遮挡 macOS 窗口控制点、顶部工具条和左侧导航。
- 1280px 宽桌面窗口下，右侧面板打开后不应造成主按钮、流程 Tab 或文本被裁切。

## 技术设计

- 后端改动：
  - 无。
- 桌面端 / Wails 改动：
  - 无新增 binding。
- 前端改动：
  - `App.jsx` 增加：
    - `inspectorMenuOpen`
    - `activeInspectorPanel`
    - `inspectorPanelOpen`
  - 右上角入口点击后切换 `inspectorMenuOpen`。
  - 菜单项点击后：
    - 设置 `activeInspectorPanel`。
    - 设置 `inspectorPanelOpen = true`。
    - 关闭 `inspectorMenuOpen`。
  - 新增右侧面板渲染函数：
    - `renderInspectorMenu()`
    - `renderInspectorPanel()`
    - `renderInspectorDetail(panelId)`
  - `styles.css` 增加：
    - 顶部工具菜单定位、阴影、菜单行样式。
    - 右侧 drawer/panel 的固定宽度、边框、关闭按钮和内容滚动。
    - 1280px 下的响应式宽度和主工作区保护规则。
- 数据模型改动：
  - 无。
- API 或 binding 改动：
  - 无新增。
  - 继续使用现有数据：
    - 余额：`BillingBinding.GetBalance()`
    - 流水：`BillingBinding.ListLedger()`
    - 订单：`OrderBinding.ListOrders(status)` / 当前选中订单
    - 交付物：`ArtifactBinding.ListArtifacts(orderID)`

## 验收标准

- [x] 点击右上角 Inspector 入口时出现浮层菜单。
- [x] 菜单不挤压主工作区，不影响当前工作台布局。
- [x] 点击菜单 `明细` 后，右侧弹出内容面板。
- [x] 右侧面板顶部有标题、上下文路径和关闭按钮。
- [x] 点击关闭按钮后，右侧面板消失，主工作区恢复。
- [x] 面板内 `预览`、`明细`、`订单`、`记录`、`进度` 至少展示各自的真实当前状态或开发态 fixture。
- [x] 1280px 宽桌面窗口下无横向溢出、无主按钮裁切、无流程 Tab 高度塌陷。
- [x] `npm run build` 通过。
- [x] `go test ./...` 通过。
- [x] `wails3 build DEV=true` 通过。
- [x] `wails3 dev` 原生启动并连接前端 dev server；视觉截图使用同一前端页面的浏览器检查留证。

## 测试计划

- 浏览器视觉验证：
  - 打开工作台默认态。
  - 点击 Inspector 入口，截图菜单态。
  - 点击 `明细`，截图右侧面板态。
  - 关闭面板，确认主工作区恢复。
- Wails 原生验证：
  - `wails3 dev -config ./build/config.yml -port <free-port>`
  - 截图确认 macOS 窗口控制点、左侧品牌区、顶部工具菜单和右侧面板不互相遮挡。
- 自动验证：
  - `cd desktop/oneshot/frontend && npm run build`
  - `go test ./...`
  - `cd desktop/oneshot && wails3 build DEV=true`

## 交付记录

- 已按评审后的 Claude Code 参考交互实现右上角 Inspector 菜单。
- `明细` 面板改为右侧 drawer，展示余额、本次扣减、单次价格、订单和最近订单；不再默认常驻右侧重栏。
- `预览`、`订单`、`记录`、`进度` 复用当前页面已加载的交付物、订单、流水和进度状态；未新增后端 API 或 Wails binding。
- 浏览器检查结果：
  - Inspector 入口唯一，菜单项为 `预览`、`明细`、`订单`、`记录`、`进度`。
  - 选择 `明细` 后菜单自动收起，右侧面板标题为 `明细`。
  - 1280px 宽下布局为 `630px 430px`，无横向溢出。
  - 关闭按钮生效，关闭后主工作区恢复单栏。
- 自动验证：
  - `cd desktop/oneshot/frontend && npm run build` 通过。
  - `go test ./...` 通过。
  - `cd desktop/oneshot && wails3 build DEV=true` 通过；仅有 macOS SDK 链接版本 warning。
  - `cd desktop/oneshot && wails3 dev -port 9259` 通过，Wails 连接 `http://localhost:9259` 前端 dev server。
