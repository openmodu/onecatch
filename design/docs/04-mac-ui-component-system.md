# 04 Mac UI 组件系统技术方案

## 1. 问题

第一轮通过 `mirage.css` 覆盖旧页面类名，视觉方向已经切换，但基础控件仍由旧 CSS、覆盖 CSS 和局部 JSX 共同决定。相同按钮、字段和 section 存在 1–4px 的规格漂移，也无法保证修改一处全局生效。

## 2. Token 分层

- Foundation：颜色、字体族、字号、字重、line-height、hairline。
- Geometry：52px unified native titlebar、216px sidebar、44px command strip、400px run inspector、176px settings rail、316px editor inspector。
- Spacing：4/6/8/10/12/14/16/18/20/24px，仅通过语义别名消费。
- Control：32px field、12px action type、7×10/12px action padding、2px badge radius。
- Section：14×16px panel、14×20px row、18×24px settings content。

Token 写入 `src/ui/tokens.css`，组件和页面不再直接维护重复魔法数字。

Issue 04-004 将字号进一步收敛为语义可读性层级：micro 11px、caption 12px、meta 13px、body 14px、emphasis 15px、subtitle 16px、title 17px、heading 18px，并为大标题与指标保留独立层级。旧 CSS 中 7–16px 的字号声明迁移到这些 token，避免任务页可读而 DAG、Settings 或 Run 恢复详情仍然偏小。Inspector 的状态列与时间列也改用共享宽度 token，字号增大后继续保持右侧纵向对齐。

## 3. UI Primitive

- `Action`：primary、accent、cyan、danger、muted variant，统一括号、字号、padding、disabled/focus。
- `Kicker`：统一大写 caption、tracking 和颜色。
- `StatusBadge` / `ModeBadge`：统一 badge geometry 和 tone。
- `Field` / `NumberField`：统一 label、control、hint、error 和 aria 关系。
- `Panel`：统一 section hairline、背景和 14×16px 内容 padding。
- `Toolbar`：统一 46px editor toolbar 与左右 action slot。
- `ToggleRow`：统一安全设置行和 on/off 命令状态。
- `TUISelect`：统一任务、设置和编辑器选择控件。触发器使用单底线与文本 caret；菜单通过 portal 避免被滚动容器裁切，使用无阴影矩形 TUI 列表和 `>` 当前项标记；支持 ArrowUp/ArrowDown、Home/End、Enter/Space、Escape 和外部点击关闭。

primitive 只负责表现和可访问性，不持有业务状态。

Issue 04-007 起，产品代码不再渲染原生 `<select>`。macOS 原生 popup 无法可靠修改圆角、蓝色选中态和阴影，也会与页面 TUI 视觉冲突；所有选项数据统一传给 `TUISelect`，选择结果仍以原字符串值交给现有业务 handler，不改变 DTO 或 binding。

Action hover 不使用 `background: currentColor`，因为同一规则内修改前景色会让背景跟随最终前景色，造成文字与背景同色。各 variant 必须显式映射 hover background，前景统一使用 canvas 色。

## 4. 页面迁移

- App shell 使用 geometry token。
- Tasks、Workflow rows 和 Run Inspector 使用共享 badge/action/kicker。
- serial/DAG editor 共享 toolbar、field 和 action。
- Settings 删除本地 `SettingCard`、`Field`、`NumberField`、`Toggle` 实现，改用 `src/ui/primitives.jsx`。

## 5. API、Binding 与数据

无变化。所有组件继续接收现有 props，事件回调仍由页面调用现有 Wails bindings。不存在新错误码、鉴权、幂等或持久化边界。

Wails 默认窗口由 1440×900 调整为原型基准 1280×800；最小窗口仍保持 1080×720。Issue 04-005 最终以 Codex 的现代 Mac 窗口为参照，使用 `MacTitleBarHiddenInsetUnified` 与 52px titlebar token，让系统交通灯在 unified toolbar 内获得舒展且对称的上下留白。

Issue 04-005 将 macOS `TitleBar` 改为 `MacTitleBarHiddenInsetUnified`。系统继续负责关闭、最小化、缩放按钮及 hover 图标，Web 前端不绘制交通灯。标题栏使用 `--wails-draggable: drag`，不再用 `InvisibleTitleBarHeight` 覆盖整个顶部点击区；macOS 平台在窗口顶部 80pt 区域安装原生双击手势并调用 `NSWindow zoom:`，放大/还原不依赖 WebView 事件。品牌副标题同步改为 `// personal workspace`，明确 Oneshot 是个人工作台，而不是本地 Agent 清单。

标题栏的文本和占位节点统一 `pointer-events: none`、`user-select: none` 与 `-webkit-user-select: none`，鼠标事件始终落在同一个 draggable header 上，不产生 WebKit 文字选区。unified toolbar 不渲染 Web `border-bottom`，并与 app shell 共用 `--acp-canvas` 背景，避免双击和缩放动画中出现非原生的灰白分隔横带。

Issue 04-006 在 macOS 平台层设置 NSWindow theme frame 的 CALayer：普通窗口使用 26pt `kCACornerCurveContinuous`，不通过 Web 内容的 `border-radius` 模拟，也不启用会改变背景材质的 Liquid Glass。窗口进入 maximise/fullscreen 时半径归零，unmaximise/unfullscreen 时恢复 26pt；阴影由 NSWindow 继续负责并在半径变化后失效重算。平台调用隔离在 `window_corner_darwin.go`，非 macOS 或未启用 cgo 的构建使用空实现。

## 6. 验证

- 以原型内部 1280×800 app frame 为真值，实施页面使用同尺寸 viewport。
- 全屏比较比例和节奏；对 Workflow row、settings card/field、editor toolbar、DAG inspector 做局部放大。
- 修复所有 P0/P1/P2 后更新 `design-qa.md`。
- 运行 frontend build、Wails build、Go test/race/vet 与 `git diff --check`。
