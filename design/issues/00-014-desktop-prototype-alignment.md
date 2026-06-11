# Issue 00-014: 桌面端回归原型与 Codex Desktop 风格纠偏

状态：已完成

## 来源

- PRD：`design/prd/00-agent-marketplace-mvp.md`
- 相关原型：`design/prototype/README.md`
- 相关原型：`design/prototype/src/App.jsx`
- 相关 issue：`design/issues/00-013-desktop-three-pane-redesign.md`
- 技术方案：`design/docs/TECHNICAL_DESIGNS.md#issue-00-014-桌面端回归原型与-codex-desktop-风格纠偏`

## 目标

修正 00-013 的方向偏差：桌面端不再采用 Finder/Mail 式 `rail / list / detail` 结构，而是回归原型定义的桌面工作台，并参考 Codex Desktop 的轻侧栏、主工作区、上下文面板交互。

## 范围

- 回归原型的核心信息架构：左侧导航与账号区、中间任务工作区、右侧可折叠 inspector。
- 左侧减重，只承载全局入口和账号，不承载大量分类、订单筛选和用量明细。
- Agent 分类、订单筛选和流程状态进入中间工作区，以分段控件、筛选条或上下文工具条呈现。
- 取消常驻重型右栏，保留可折叠 inspector；默认不抢占主流程。
- 明确主按钮和次按钮位置，避免按钮漂移到侧栏或右侧详情里。
- 视觉风格收敛到 macOS / Codex Desktop 式克制桌面感，并保留 Claude 风格的温和中性色。
- 建立 Mac 端视觉检查方式，交付前必须有截图或可替代的视觉核对记录。

## 非目标

- 不新增后端业务接口。
- 不改订单状态机、计费事务、OAuth 或支付 provider。
- 不引入多窗口、多工作区或复杂动画。
- 不重写现有领域模型。

## 产品需求

- 用户打开桌面端后，第一视线落在中间工作台，而不是左侧或右侧。
- 用户能在中间完成 Agent 选择、需求填写、扣次确认、执行进度和交付物查看。
- 左侧只帮助用户切换大范围上下文，不承载具体业务筛选。
- 右侧 inspector 只在用户需要查看用量、订单信息、交付物预览等辅助信息时展开。
- 主按钮必须靠近当前任务：
  - `确认并支付 / 开始执行` 位于中间结算区。
  - `购买次数` 位于余额摘要附近和账号/用量入口，不放在主导航。
  - `下载 / 分享` 位于交付物卡片内。
  - `Inspector 展开/收起` 位于工作区右上工具区。
- 视觉上减少蓝色大面积使用，避免管理后台感；采用更接近原型和 Claude 的温和中性色、低饱和强调色、薄分割线和克制阴影。

## 技术设计

- 后端改动：
  - 不新增 API。
  - 继续使用已存在的 Agent、Auth、Billing、Order、Artifact 服务能力。
- 桌面端 / Wails 改动：
  - 不新增 binding。
  - 保留 `OrderBinding.ListOrders(status)`，用于订单筛选。
- 前端改动：
  - 将 00-013 的 `rail / list / detail` 结构改回工作台结构。
  - `sidebar` 只保留工作台、订单、用量/账单、账号等全局入口。
  - 中间主工作区恢复 Agent hero、分类筛选、任务流程、需求、结算、进度和交付物。
  - `inspector` 改为可折叠上下文面板；默认状态待技术方案评审确认，建议为默认收起或窄摘要。
  - 增加清晰的按钮位置规则和状态映射，避免实现时再次跑偏。
  - 若浏览器预览没有 Wails runtime，则增加仅开发环境启用的 UI fixture 适配，保证视觉 QA 可以在浏览器和 Wails 原生窗口中复核。
- 数据模型改动：
  - 无。
- API 或 binding 改动：
  - 无新增。
  - 前端只消费现有 binding：
    - `AuthBinding.CurrentUser/LoginWithWechat/LoginWithGoogle/Logout`
    - `AgentBinding.ListAgents/GetAgent`
    - `BillingBinding.GetBalance/ListLedger/StartPurchase`
    - `OrderBinding.CreateOrder/ListOrders/GetOrder/CancelOrder`
    - `ArtifactBinding.ListArtifacts/DownloadArtifact/ShareArtifact`

## 验收标准

- [x] 左侧不再是重型分类/订单/用量导航，主导航项数量受控。
- [x] 中间工作区恢复为主任务流，用户不需要先在列表和详情之间来回跳转才能下单。
- [x] Agent 分类和订单筛选不占据左侧主导航。
- [x] 右侧 inspector 可收起，且收起后不影响下单、查看进度和查看交付物。
- [x] `确认并支付 / 开始执行` 位于中间结算区，且靠近价格、扣次和余额信息。
- [x] `购买次数` 位于余额摘要或账号/用量上下文中，不作为左侧主导航强入口。
- [x] `下载 / 分享` 位于交付物卡片内。
- [x] 视觉风格不再呈现大面积蓝色管理后台感。
- [x] Mac 端 `wails3 dev` 原生窗口完成视觉检查，并记录截图或无法截图原因。
- [x] `go test ./...` 通过。
- [x] `npm run build` 通过。
- [x] `wails3 build DEV=true` 通过。

## 测试计划

- 方案评审：
  - 对照 `design/prototype/README.md` 和 `design/prototype/src/App.jsx` 核对信息架构。
  - 对照 Codex Desktop 参考原则核对左侧、主工作区、上下文面板和操作位置。
- 自动验证：
  - `go test ./...`
  - `cd desktop/oneshot/frontend && npm run build`
  - `wails3 build DEV=true`
- 前后端联调验证：
  - 未登录点击主按钮必须走登录提示。
  - 登录后创建订单必须刷新余额和订单状态。
  - 订单筛选必须通过 `OrderBinding.ListOrders(status)` 进入后端。
  - 交付后交付物列表、下载和分享必须仍走 Artifact binding。
- Mac 端视觉验证：
  - 启动 `wails3 dev`。
  - 检查 1280x800、1440x900 两个桌面尺寸。
  - 对照原型与本 issue 验收标准做截图核对。
  - 若 macOS 截图权限阻止自动截图，交付记录必须说明原因，并使用浏览器 fixture 截图加原生窗口人工核对补足。

## 交付记录

- 已按原型工作台结构重构桌面端 UI：
  - 左侧保留轻量全局入口：工作台、我的订单、用量账单、账户。
  - 中间恢复 Agent hero、分类筛选、Agent 快捷切换、流程步骤、需求、扣次结算、进度和交付物。
  - 右侧 Inspector 默认收起，不再常驻占位；通过顶部按钮展开。
  - Inspector 展开后展示用量、使用记录、订单信息和交付物预览。
- 已保留现有 Wails binding 联调路径，不新增后端 API。
- 已增加浏览器开发态 fixture：当浏览器缺少 Wails runtime 时仍可做视觉 QA；Wails 原生运行仍优先使用真实 binding。
- 已修复视觉 QA 中发现的两处布局问题：
  - Agent 快捷切换条父容器高度塌陷。
  - Inspector 展开态流程 Tab 高度塌陷、主按钮被右栏裁切。
- 验证结果：
  - `cd desktop/oneshot/frontend && npm run build` 通过。
  - `go test ./...` 通过。
  - `cd desktop/oneshot && wails3 build DEV=true` 通过，仅有 macOS link target warning。
  - 浏览器 fixture 视觉检查通过：
    - `/tmp/oneshot-ui-1440x900-fixed.png`
    - `/tmp/oneshot-ui-1280x800-final.png`
  - Mac 原生窗口检查通过：
    - `wails3 dev -config ./build/config.yml -port 9255`
    - 窗口：`Oneshot, 320, 119, 1279, 819`
    - 截图：`/tmp/oneshot-wails-dev.png`
