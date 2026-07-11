# Design QA：Mac UI 组件系统与像素收敛

## 基准

- 视觉真值：`design/Mac应用UI设计优化/Oneshot.dc.html`
- 实现：`desktop/oneshot/frontend/src/app/App.jsx`、`SettingsPage.jsx`
- 共享组件：`desktop/oneshot/frontend/src/ui/primitives.jsx`
- 共享 token：`desktop/oneshot/frontend/src/ui/tokens.css`
- 对照视口：原型内部 app frame 与实现均为 1280 × 800
- 状态：Tasks、Workflows、serial editor、DAG editor、Settings / Runtime

## 完整对照证据

| 页面 | 原型 | 实现 | 合并对照 |
| --- | --- | --- | --- |
| Tasks | `/tmp/oneshot-component-system/tasks-reference-1280x800-v2.png` | `/tmp/oneshot-component-system/tasks-1280x800-v2.png` | `/tmp/oneshot-component-system/tasks-compare-v2.png` |
| Workflows | `/tmp/oneshot-component-system/workflows-reference-1280x800-v2.png` | `/tmp/oneshot-component-system/workflows-1280x800-v2.png` | `/tmp/oneshot-component-system/workflows-compare-v2.png` |
| Serial editor | `/tmp/oneshot-component-system/editor-reference-1280x800.png` | `/tmp/oneshot-component-system/editor-1280x800-v2.png` | `/tmp/oneshot-component-system/editor-compare-v2.png` |
| DAG editor | `/tmp/oneshot-component-system/dag-reference-1280x800.png` | `/tmp/oneshot-component-system/dag-1280x800-v2.png` | `/tmp/oneshot-component-system/dag-compare-v2.png` |
| Settings | `/tmp/oneshot-component-system/settings-reference-1280x800-v2.png` | `/tmp/oneshot-component-system/settings-1280x800-v2.png` | `/tmp/oneshot-component-system/settings-compare-v2.png` |

## 局部对照证据

- Workflow header、badge、五列 row：`/tmp/oneshot-component-system/workflows-focus.png`
- Settings panel、Field、Action：`/tmp/oneshot-component-system/settings-focus.png`
- editor Toolbar、step Panel、transition Field：`/tmp/oneshot-component-system/editor-focus.png`

DAG 全图中的节点、连线、toolbar 和 Inspector 已能以原尺寸辨认，因此未再生成 DAG 局部裁切。

## 必查表面

- 字体与排版：统一由 `--ui-font-*`、weight 和 SF Mono/JetBrains Mono fallback 管理；标题、正文、kicker、badge 和 action 与原型层级一致。
- 间距与布局：38px titlebar、216px sidebar、44px command strip、400px Run Inspector、176px settings rail、280px editor Inspector 均已 token 化；row、panel、field 使用统一 spacing。
- 颜色与 token：ACP light/dark palette 已集中到 `ui/tokens.css`，页面不再重复维护主题色。
- 图片与资产：该原型无品牌位图、插画或产品图片；未用占位图替换可见资产。Wails 交通灯继续由 macOS 原生标题栏提供。
- 文案与内容：导航和主要动作与原型一致；Run 数量、版本、运行时间和设置说明来自真实/demo state，允许与 mock 文案不同。

## 对照迭代历史

1. P1：第一轮用页面覆盖 CSS，按钮、字段和 section 仍继承旧样式，无法全局收敛。修复：新增 `ui/tokens.css` 与 `ui/primitives.jsx`，迁移主要 Action、Badge、Field、Panel、Toggle、Toolbar。
2. P2：Workflow badge 宽度和五列 row 与原型不同。修复：固定 68px badge，并采用 `76 / 200 / 1fr / 150 / 76` grid、16px gap、14×20px row padding；复查见 `workflows-focus.png`。
3. P2：Settings card 因逐字段 hint 比原型偏高。修复：Runtime Panel 使用共享 variant 隐藏冗余 hint，并统一 14×16px Panel、18px section gap、32px Field；复查见 `settings-focus.png`。
4. P1：serial editor 第一轮是无背景 section，header input 和 transition 自动拉伸。修复：step 改为共享 Panel 规格，header 和 transition 使用原型固定列宽；复查见 `editor-focus.png`。
5. P2：DAG toolbar 名称偏到中间，节点位置和 runtime badge 尺寸漂移。修复：toolbar 起点、204px node、节点 padding、runtime badge 和默认 layout 与原型收敛；复查见 `dag-compare-v2.png`。

## 有意保留的差异

- 浏览器预览不重复绘制交通灯；Wails 使用 `HiddenInset`，实际窗口由 macOS 提供原生交通灯。
- DAG Inspector 保留可编辑“名称”字段，这是现有功能所需，原型只展示名称标题。
- 原型使用固定 mock Run；实现保留真实/demo Run state，因此列表数量与 Inspector 内容不同。

## 功能与工程检查

- 主导航、Workflow 打开/返回、serial 校验、DAG 打开与 Inspector 可用。
- Settings 修改 `maxTransitions`、dirty/save 流程通过。
- 页面无 React error overlay，主流程未观察到 console error。
- frontend production build、Wails dev build、`go test ./...`、`go test -race ./internal/...`、`go vet ./...`、`git diff --check` 通过。

## 结论

P0：0，P1：0，P2：0。

final result: passed
