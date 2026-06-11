# Issue 00-013: 桌面端三段式交互重构

状态：已完成

## 来源

- PRD：`design/prd/00-agent-marketplace-mvp.md`
- 相关 issue：`design/issues/00-001-desktop-workbench-shell.md`
- 相关 issue：`design/issues/00-004-usage-billing-payment.md`
- 相关 issue：`design/issues/00-005-order-lifecycle.md`
- 相关 issue：`design/issues/00-006-delivery-artifacts.md`

## 目标

将桌面端从复杂仪表盘式工作台重构为 Apple 风格三段式交互：左侧轻导航、中间列表选择、右侧当前对象详情和主操作。

## 范围

- 左侧导航只保留 Agent、订单、账户三个顶层入口。
- Agent 分类从左侧移到中间列表顶部。
- 订单状态筛选从左侧移到订单列表顶部。
- 取消常驻 Inspector，右侧改为当前 Agent、订单或账户详情。
- 主按钮位置统一：
  - Agent 下单主按钮放在右侧需求输入区底部。
  - 购买次数按钮放在账户详情和顶部余额胶囊。
  - 下载、分享放在订单详情的交付物区域。
  - 取消订单放在订单详情的次级操作区。
- 前后端订单筛选对齐，桌面 binding 支持按状态请求订单列表。

## 非目标

- 重做后端订单状态机。
- 重做支付 provider。
- 新增多窗口或多 tab。
- 新增复杂动画。

## 产品需求

- 用户打开应用后先看到清晰的三段式结构。
- 左侧不再承载分类、用量、订单筛选等重信息。
- 用户选择 Agent 后，右侧直接填写需求并开始执行。
- 用户进入订单页后，中间选择订单，右侧查看进度、下载和分享交付物。
- 用户进入账户页后，右侧查看登录、余额、购买次数和用量记录。
- 所有主要操作的位置符合“主动作靠近当前内容，次动作进入详情区域”的规则。

## 技术设计

- 前端：
  - `App.jsx` 增加 `activeSection`，在 Agent、订单、账户之间切换。
  - 中间列表区根据 `activeSection` 展示 Agent 列表、订单列表或账户摘要。
  - 右侧详情区根据当前选中对象展示详情和主操作。
  - 移除常驻 Inspector 布局和五步 tabs。
- 桌面端 / Wails：
  - `OrderBinding.ListOrders(status)` 支持状态筛选。
- 客户端：
  - `oneshot.Client.ListOrders(ctx, status)` 对齐 HTTP `/api/orders?status=`。
- 后端：
  - 复用已有 `GET /api/orders?status=`，不新增接口。

## 验收标准

- [x] 左侧只保留 Agent、订单、账户三个入口。
- [x] Agent 分类在中间列表顶部展示。
- [x] 订单状态筛选在订单页中间列表顶部展示。
- [x] Agent 详情右侧包含需求输入和开始执行按钮。
- [x] 订单详情右侧包含进度、交付物下载和分享。
- [x] 账户详情右侧包含余额、购买次数和登录状态。
- [x] 前端订单筛选通过 binding 传到后端 status 参数。
- [x] `npm run build` 通过。
- [x] `go test ./...` 通过。
- [x] `wails3 build DEV=true` 通过。

## 测试计划

- Go 测试客户端和 handler 编译路径。
- Wails binding 重新生成。
- 前端生产构建。
- Wails DEV 构建。
- 手动使用 `wails3 dev` 检查三段式布局和主操作位置。

## 交付记录

- 已将桌面端重构为三段式布局：
  - 左侧 rail 仅保留 Agent、订单、账户。
  - 中间 pane 承载 Agent 分类列表、订单状态列表或账户摘要。
  - 右侧 pane 承载当前 Agent、订单或账户详情和主操作。
- 已移除常驻 Inspector 和五步 tabs。
- 已将 Agent 下单主按钮放在右侧需求输入区底部。
- 已将订单交付物下载、分享放在订单详情交付物区域。
- 已将账户余额和购买次数放到账户详情。
- 已将 `OrderBinding.ListOrders(status)` 与 HTTP `/api/orders?status=` 对齐。
- 验证结果：
  - `go test ./...` 通过。
  - `npm run build` 通过。
  - `wails3 build DEV=true` 通过；仅有 macOS link target warning。
