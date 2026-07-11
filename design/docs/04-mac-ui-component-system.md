# 04 Mac UI 组件系统技术方案

## 1. 问题

第一轮通过 `mirage.css` 覆盖旧页面类名，视觉方向已经切换，但基础控件仍由旧 CSS、覆盖 CSS 和局部 JSX 共同决定。相同按钮、字段和 section 存在 1–4px 的规格漂移，也无法保证修改一处全局生效。

## 2. Token 分层

- Foundation：颜色、字体族、字号、字重、line-height、hairline。
- Geometry：38px titlebar、216px sidebar、44px command strip、400px run inspector、176px settings rail、316px editor inspector。
- Spacing：4/6/8/10/12/14/16/18/20/24px，仅通过语义别名消费。
- Control：32px field、12px action type、7×10/12px action padding、2px badge radius。
- Section：14×16px panel、14×20px row、18×24px settings content。

Token 写入 `src/ui/tokens.css`，组件和页面不再直接维护重复魔法数字。

## 3. UI Primitive

- `Action`：primary、accent、cyan、danger、muted variant，统一括号、字号、padding、disabled/focus。
- `Kicker`：统一大写 caption、tracking 和颜色。
- `StatusBadge` / `ModeBadge`：统一 badge geometry 和 tone。
- `Field` / `NumberField`：统一 label、control、hint、error 和 aria 关系。
- `Panel`：统一 section hairline、背景和 14×16px 内容 padding。
- `Toolbar`：统一 46px editor toolbar 与左右 action slot。
- `ToggleRow`：统一安全设置行和 on/off 命令状态。

primitive 只负责表现和可访问性，不持有业务状态。

## 4. 页面迁移

- App shell 使用 geometry token。
- Tasks、Workflow rows 和 Run Inspector 使用共享 badge/action/kicker。
- serial/DAG editor 共享 toolbar、field 和 action。
- Settings 删除本地 `SettingCard`、`Field`、`NumberField`、`Toggle` 实现，改用 `src/ui/primitives.jsx`。

## 5. API、Binding 与数据

无变化。所有组件继续接收现有 props，事件回调仍由页面调用现有 Wails bindings。不存在新错误码、鉴权、幂等或持久化边界。

Wails 默认窗口由 1440×900 调整为原型基准 1280×800，`InvisibleTitleBarHeight` 由 48px 调整为 38px；最小窗口仍保持 1080×720。

## 6. 验证

- 以原型内部 1280×800 app frame 为真值，实施页面使用同尺寸 viewport。
- 全屏比较比例和节奏；对 Workflow row、settings card/field、editor toolbar、DAG inspector 做局部放大。
- 修复所有 P0/P1/P2 后更新 `design-qa.md`。
- 运行 frontend build、Wails build、Go test/race/vet 与 `git diff --check`。
