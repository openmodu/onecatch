# Issue 00-017: Mac native 桌面视觉系统收敛

状态：已完成

## 来源

- PRD：`design/prd/00-agent-marketplace-mvp.md`
- 相关 issue：
  - `design/issues/00-014-desktop-prototype-alignment.md`
  - `design/issues/00-016-inspector-menu-drawer.md`
- 技术方案：`design/docs/TECHNICAL_DESIGNS.md#issue-00-017-mac-native-桌面视觉系统收敛`
- 基线提交：`e6be354 chore(checkpoint): preserve current desktop UI state`
- 视觉参考：
  - macOS 原生 source list、toolbar、popover、inspector panel
  - Claude Code 桌面端的左侧导航、顶部工具入口和右侧内容面板

## 目标

在保留当前 Inspector 菜单和右侧 drawer 交互的前提下，将桌面端从“轻量 Web app 风格”收敛到更接近 Mac 原生应用的视觉和基础行为。

## 范围

- 调整全局视觉 token：
  - 从大面积米色/陶土色转向 macOS 中性灰。
  - 品牌橙只用于主按钮、关键金额和轻量选中态。
  - 本 issue 只验收 light mode；dark mode 不在本次完成标准内。
- 左侧 sidebar 收敛为 macOS source list：
  - 更轻的背景和选中态。
  - 图标和文字间距收紧。
  - 底部登录区降低卡片感。
- 顶部 toolbar 收敛为原生工具栏：
  - 保留 Wails draggable / no-drag 区域。
  - 返回、帮助和 Inspector 入口降低按钮重量。
  - Inspector popover 更接近 native menu 的行高、圆角、阴影和快捷键排版。
- 主工作区降低 Web 卡片感：
  - 减少粗边框和高饱和背景。
  - 保留当前三段式流程，但让 hero、分类、流程 tabs、表单和 checkout 更像 grouped sections。
- Inspector drawer 收敛为 native inspector panel：
  - header 更像 panel titlebar。
  - `明细` 内容从彩色大卡片改为 table/list/diff-row 风格。
  - 关闭按钮保持右上角，面板内部滚动。
- 恢复 Mac 基础行为：
  - 不全局禁用文本选择。
  - 不全局拦截右键菜单。
  - 只在导航、按钮、拖拽区等控件上禁用选择。

## 非目标

- 不改变 00-016 已确认的 Inspector 菜单 -> drawer 交互模型。
- 不新增后端 API。
- 不新增 Wails binding。
- 不改登录、订单、计费、交付物业务流程。
- 不实现 dark mode 完整适配。
- 不引入新的桌面多窗口、系统设置页或菜单命令体系。

## 产品需求

- 用户看到的整体界面应更像 Mac 桌面工具，而不是网页后台。
- 左侧导航应更轻，不应像按钮列表或重卡片。
- 右上角 Inspector 菜单打开后应像原生 popover/menu，不能挤压主内容。
- 右侧 drawer 应像 inspector panel，信息可扫读，不应像一组彩色营销卡片。
- 用户能选择和复制订单号、金额、说明文本等关键内容。
- 1280px 宽桌面窗口下，主流程按钮、流程 tabs、分类条和 drawer 内容不应裁切或横向溢出。

## 技术设计

- 后端改动：
  - 无。
- 桌面端 / Wails 改动：
  - 保留当前 Wails 原生菜单角色。
  - 将窗口背景色调整为 macOS 中性 light 背景，避免原生窗口边缘与前端背景色不一致。
- 前端改动：
  - `styles.css`：
    - 重建 light mode token，优先使用 `#f5f5f7`、`#ffffff`、系统分隔线和低饱和 selection。
    - 移除本期未验收的 dark token 入口，避免半成品 dark mode。
    - 将 `body { user-select: none; }` 改为控件级禁用选择。
    - 调整 sidebar、toolbar、popover、main sections、drawer 的背景、边框、阴影、字号和间距。
    - 恢复 860px 以下的保护断点，避免后续窗口缩小时布局崩坏。
  - `App.jsx`：
    - 移除全局 `contextmenu` 拦截。
    - 保留现有 inline `Icon`，本 issue 不新增 icon 依赖。
    - 如需要，微调 drawer detail DOM 结构，使 `明细` 更像列表/table，而不改变数据来源。
- 数据模型改动：
  - 无。
- API 或 binding 改动：
  - 无新增。

## 验收标准

- [x] 默认工作台截图接近 macOS native：中性灰背景、轻 sidebar、轻 toolbar、低卡片感。
- [x] Inspector 菜单态接近 native popover/menu，菜单不挤压主工作区。
- [x] `明细` drawer 接近 native inspector panel，内容以列表/table/diff row 展示。
- [x] 可选择并复制订单号、需求文本、金额和 drawer 明细文本。
- [x] 非输入区域右键不再被全局阻止。
- [x] 1280x800 和 1440x900 下无横向溢出、无主按钮裁切、无流程 Tab 高度塌陷。
- [x] `npm run build` 通过。
- [x] `go test ./...` 通过。
- [x] `wails3 build DEV=true` 通过。
- [x] `wails3 dev` 原生窗口完成启动和连接检查；视觉截图用同一前端页面的浏览器视口留证。

## 测试计划

- 自动验证：
  - `cd desktop/oneshot/frontend && npm run build`
  - `go test ./...`
  - `cd desktop/oneshot && wails3 build DEV=true`
- 浏览器视觉验证：
  - 1280x800 默认态截图。
  - 1280x800 Inspector 菜单态截图。
  - 1280x800 `明细` drawer 态截图。
  - 1440x900 默认态和 drawer 态截图。
  - 检查 `document.documentElement.scrollWidth <= window.innerWidth`。
- Mac 原生验证：
  - `cd desktop/oneshot && wails3 dev -port <free-port>`
  - 检查窗口控制点、toolbar、popover 和 drawer 不重叠。
  - 检查原生窗口边缘背景色与前端背景一致。
- 行为验证：
  - 选择订单号和需求文本。
  - 右键非输入区域不被应用层阻止。
  - Inspector 菜单点击外部关闭。
  - `⌘1` 到 `⌘5` 可打开对应面板。

## 交付记录

- 基于 `e6be354` 实现 Mac native 视觉收敛，没有改后端 API 或 Wails binding。
- 已将 Wails window background 从暖色改为中性灰，匹配 light mode 前端背景。
- 已移除全局 `contextmenu` 拦截，非输入区域右键不再被应用层阻止。
- 已移除全局 `body user-select: none`，改为按钮、导航、toolbar 等控件级不可选；正文、订单号、需求和 drawer 明细保持可选择。
- 已将全局 token 收敛为 light-only macOS neutral，移除未验收的 dark mode token。
- 已将 sidebar、toolbar、Inspector popover、主工作区 grouped sections、drawer 明细列表做轻量化。
- 已将导航和 Inspector 图标调为更接近 macOS template symbol 的轻笔画、单色和统一尺寸。
- 2026-06-14 修复：Inspector popover 展开时不再被下方工作台内容遮挡，菜单锚定按钮下方并保持在顶部工具层上方。
- 2026-06-14 修复：错误/状态提示从右下角网页 toast 调整为顶部居中的 macOS native-ish 磨砂浮层，并按提示类型显示系统色状态点。
- 浏览器视觉验证：
  - 1280 默认态：`/tmp/oneshot-017-default-1280.png`
  - 1280 菜单态：`/tmp/oneshot-017-menu-1280.png`
  - 1280 明细 drawer 态：`/tmp/oneshot-017-detail-1280.png`
  - 1440 默认态：`/tmp/oneshot-017-default-1440.png`
  - 1440 明细 drawer 态：`/tmp/oneshot-017-detail-1440.png`
- DOM 检查：
  - 1280 下 `scrollWidth = innerWidth = 1280`，无横向溢出。
  - 1440 下 `scrollWidth = innerWidth = 1440`，无横向溢出。
  - drawer 打开时 1280 布局为 `630px 430px`，1440 布局为 `790px 430px`。
  - `body` 和 `.main-column` 的 `user-select` 为 `auto`，导航按钮为 `none`。
- 自动验证：
  - `cd desktop/oneshot/frontend && npm run build` 通过。
  - `go test ./...` 通过。
  - `cd desktop/oneshot && wails3 build DEV=true` 通过；仅有 macOS SDK 链接版本 warning。
  - `cd desktop/oneshot && wails3 dev -port 9262` 通过，Wails 连接 `http://localhost:9262` 前端 dev server。
