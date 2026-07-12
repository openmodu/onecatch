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
---

# Run Inspector Live Timeline Design QA

- Source visual truth: `/Users/ityike/.codex/generated_images/019f4aee-e179-7293-91d2-814d195f974b/exec-33ca3520-1774-47d6-8df4-db274707ac9a.png`
- Original mismatch evidence: `/var/folders/nz/tjb3cj6s3cb3jrvrp27yf9x00000gn/T/codex-clipboard-df3ebaf4-74a7-4d1e-a2a8-504c8a9bd025.png`
- Implementation screenshot: `/tmp/oneshot-option2-final-v5.png`
- Side-by-side comparison: `/tmp/oneshot-option2-compare-final-v6.jpg`
- Viewport: `1280 × 836`, Inspector width `480px`
- State: light theme, demo paused Run, first tool disclosure open for parity with the source visual

## Findings

No actionable P0/P1/P2 mismatch remains after iteration 4.

- Typography: Agent/user body is `13px`, weight 400, with 1.65 line-height; tool title and metadata use the existing mono hierarchy. Long commands are one-line ellipsized when closed and fully readable after expansion.
- Spacing/layout: the Run summary is `59.9px` high, user and one-line Agent messages are about `75px` and `77px`, and closed tool rows are `50px`, matching the selected source rhythm. Inspector has no horizontal overflow (`clientWidth = scrollWidth = 479px`).
- Colors/tokens: all new foregrounds, hairlines, active cyan, success green and error red are mapped through the existing Mirage tokens in `mirage.css`; no new gradient, shadow or hard-coded dark-only surface was introduced.
- Image/assets: the selected design contains no raster imagery or custom brand assets in the changed region. Disclosure affordances use Phosphor caret icons and existing runtime/status primitives; no replacement imagery is required.
- Copy/content: the compact summary retains task title, Run status, current step/runtime, transition count, Run ID, Workflow steps, sessions and resume commands. Raw `workflow_signal` is intentionally hidden because the visible “等待介入” state already communicates it and the source design does not show raw protocol values.

## Interaction Evidence

- Three demo tool events render as three independent disclosures; all report `open = false` on initial render.
- Opening the first tool changes only that item to `open = true`; the other two remain closed.
- The expanded item exposes its full command content.
- “会话与恢复” opens one combined detail region containing two Workflow steps and two runtime sessions.
- Runtime event DOM order is user → tool → file change → Agent message → tool → Agent messages, matching persisted event order.
- Browser console has no application errors; only the expected Wails browser-preview warning is present.

## Comparison History

### Iteration 1 — blocked

- P1: all tool calls were grouped under one “工具与过程” disclosure, so messages and tools were no longer chronological.
- P1: long commands were expanded together and filled most of the Inspector.
- P1: Run Inspector, current step, Workflow execution and Agent sessions were four separate vertical sections.
- P2: message text was visually subordinate to raw command output.

Fixes applied:

- Changed the derived conversation model from `messages + one activity group` to ordered `message/tool` items.
- Bound an adjacent `tool_result` to its preceding `tool_use` without merging separate tool calls.
- Added one default-closed disclosure per tool/process/file event.
- Replaced four summary sections with one compact sticky Run summary and one default-closed “执行与会话” detail.
- Raised message body text to 14px and constrained closed tool rows to a single line.

### Iteration 2 — passed

- Summary height reduced from `145.5px` to `114px` by removing the redundant raw `workflow_signal` notice.
- All tool items remain `52.9px` closed regardless of command length.
- No horizontal overflow remains in the Inspector, round headers or tool summaries.
- Bottom Run controls remain visible at the viewport boundary.

### Iteration 3 — blocked by source fidelity

- Implementation screenshot: `/tmp/oneshot-run-inspector-polished-v3.png`
- User and Agent messages had distinct identities, but retained the legacy “第 N 轮” header, status rail and card-like message surface that do not exist in option 2.
- The summary remained about twice the source height (`114px` versus about `60px`), and native disclosure markers appeared on the left instead of the source's right-aligned caret.
- Every tool call remains an independent, default-closed disclosure. Its summary shows a readable command stripped from the common `zsh -lc` launcher, while the complete raw event remains available after expansion.
- Expanded tool content is separated into `COMMAND / PROCESS / PATH / RESULT`; hover, keyboard focus, open and running states use shared Mirage tokens and include dark-theme overrides.
- This iteration was functionally correct but visually rejected by the user and is not considered a fidelity pass.
- Regression verification: Node `12/12`, frontend production build and `git diff --check` passed; browser console contains no application error.

### Iteration 4 — option 2 fidelity passed

- Removed the StepRun round header, message card rail and tinted user surface; events now form the same flat chronological stream as the source.
- Rebuilt the summary as two compact rows: title/status/count/time, then current step/runtime and “会话与恢复”. Measured height is `59.9px`.
- Rebuilt tool summaries as five fixed columns with a right-aligned Phosphor caret. Closed rows measure `50px`; one expanded row leaves its siblings closed and shows a bordered `COMMAND` body with a cyan active rail.
- Added readable action titles for common `sed`, package runner, git, search and find commands while preserving the full raw command after expansion.
- Side-by-side evidence `/tmp/oneshot-option2-compare-final-v6.jpg` confirms matching hierarchy, spacing rhythm, flat borders, body weight, tool-row density and expanded-body treatment. Remaining visible differences are demo event content, runtime/status values and unavailable tool durations rather than design drift.
- Browser verification: no horizontal overflow, no legacy round headers, controls remain reachable, and default tool state remains closed on initial render. Node `12/12`, production build and `git diff --check` pass.

### Iteration 5 — icon-only event markers passed

- User override: replace visible event-type text with a white user dot, blue Agent dot and the existing tool disclosure arrow while retaining option 2's layout and density.
- Implementation screenshot: `/tmp/oneshot-event-dot-markers-v6.png`; comparison evidence: `/tmp/oneshot-event-dot-compare-v6.jpg`.
- Removed all visible `USER / AGENT / TOOL USE / FILE CHANGE` strings from the conversation surface. Runtime names, state and timestamps remain because they carry task context rather than repeat the event type.
- User and Agent markers are 9px Phosphor `Circle` icons. The user marker resolves to the dedicated white `--ui-event-user` token with a token-based one-pixel visual edge, while the Agent marker resolves to the cyan token in both themes.
- Tool summaries now use `minmax(0, 1fr) / 45px / 55px / 16px` tracks, so the removed label column becomes command-title space; the right caret retains the same open/closed behavior.
- Browser verification at `1280 × 800`: no visible type-label strings, all four markers are 9px, every tool starts closed, Inspector remains `479px / 479px` with no horizontal overflow, and tool title/state/time/caret alignment remains stable.
- Accessibility: user and Agent circles expose `aria-label`; tool summaries retain kind-aware accessible names even though the kind text is no longer visible.
- Regression verification: Node `12/12`, production build and `git diff --check` pass.

### Iteration 6 — right-aligned time and round metadata passed

- User annotation source: `/var/folders/nz/tjb3cj6s3cb3jrvrp27yf9x00000gn/T/codex-clipboard-6d3bfa5f-0a32-468d-a89e-a5bc3337589d.png`.
- Implementation screenshot: `/tmp/oneshot-round-time-alignment-v8.png`; normalized comparison: `/tmp/oneshot-round-time-compare-v8.jpg`.
- User, Agent and tool timestamps now share an exact right edge at browser x=`1243`; all message timestamps use the same 55px track as tool timestamps and preserve the 23px caret gutter.
- Agent messages show `第 1 轮`, `第 2 轮`, etc. immediately before the timestamp. Multiple messages from the same StepRun repeat the same round number, while user messages remain unnumbered.
- The initial user message now falls back to task/run `updatedAt` only when created/started timestamps are absent, avoiding a visible em dash for compatible older records.
- Browser verification at `1280 × 800`: four message timestamps and three tool timestamps are vertically aligned on the same right edge, round labels do not collide with runtime or time, Inspector overflow delta is zero, and no legacy round header was reintroduced.
- Regression verification: Node `13/13`, production build and `git diff --check` pass.

### Iteration 7 — tool state spacing passed

- Tool state remains in its fixed 45px grid track but now uses `justify-self: end`, moving short state labels toward the timestamp without shifting the shared 55px time column or disclosure caret.
- This is a spacing-only adjustment; event order, disclosure behavior, time alignment and responsive width are unchanged.

### Iteration 8 — leading tool caret passed

- Tool disclosure caret moved from the trailing column to the 16px leading column before the command title.
- Closed tools render `CaretRight`; open tools render `CaretDown` through the existing native `<details>` state selectors.
- The former 23px trailing caret gutter was removed from message headers, so user, Agent and tool timestamps remain aligned at the content area's right edge.

### Iteration 9 — marker alignment and contrast passed

- User/Agent circles and tool carets now share the `--ui-event-marker-size: 9px` leading track, aligning both their left edge and visual center across event rows.
- Tool caret color changed from muted gray to the primary foreground token, rendering black in the light theme and preserving readable contrast in the dark theme.

## Follow-up Polish

- P3: real runtimes do not currently report reliable per-tool duration, so the implementation shows event time rather than inventing duration values from the mock.

final result: passed


---

# Issue 04-007 TUI Select Design QA

- Source visual truth: `/var/folders/nz/tjb3cj6s3cb3jrvrp27yf9x00000gn/T/codex-clipboard-7de83a33-d5b6-4c89-b4a3-e5972d25f2ce.png`（原生 popup 为待消除的反例；页面现有直角、hairline、绿色语义色为目标语言）
- Follow-up annotation: `/var/folders/nz/tjb3cj6s3cb3jrvrp27yf9x00000gn/T/codex-clipboard-851ad4cd-fe03-4890-af98-ed3b343d9fed.png`（菜单底部硬阴影形成额外背板）
- Implementation screenshot: `/tmp/oneshot-select-qa/task-open.jpg`
- Follow-up implementation screenshot: `/tmp/oneshot-select-qa/sandbox-no-shadow.jpg`
- Combined comparison: `/tmp/oneshot-select-qa/source-vs-task-open.jpg`
- Viewport: 1280 × 720 implementation; source focused crop normalized to 720px height for comparison
- State: 工作台 Workflow 选择框展开，当前项为“并行审查 DAG”

## Full-view comparison evidence

- 实现保留页面原有 TUI 字体、hairline、直角和绿色语义色，没有引入 macOS 蓝色选中态或圆角浮层。
- 菜单通过 portal 覆盖列表区域，未被工作台列或滚动容器裁切。
- 触发器与菜单宽度一致，展开层级不会推动周围布局。

## Focused region comparison evidence

- 原生控件的圆角外框、蓝色 focus ring、系统阴影和蓝色选中项已移除。
- 闭合触发器仅保留 1px 底线；展开/键盘焦点以 cyan 内嵌底线表达，没有第二圈 outline。
- 菜单为 0px 圆角矩形，当前项用 `>` 与绿色文本标记，活动项使用低对比背景。

## Required fidelity surfaces

- Fonts and typography: 继承共享 monospace token，字号、字重和行高与工作台元数据一致；长值保持单行省略。
- Spacing and layout rhythm: 触发器使用共享 control height/padding；选项 36px 最小高度；矩形菜单与 trigger 左右对齐。
- Colors and visual tokens: 只使用 canvas、line、cyan、good 与 accent-soft 共享 token。
- Image quality and asset fidelity: 本控件不包含图片资产；未以位图替代 UI 内容。
- Copy and content: 现有 Workflow、Runtime、Sandbox、Worker 与设置选项文案和值保持不变。

## Interaction verification

- ArrowDown + Enter 可切换选项并关闭菜单。
- Escape 可关闭并把焦点留在 trigger。
- 点击外部可关闭菜单。
- 设置页 Provider 展开态实测：trigger 顶边 0px、底边 1px、outline 0px；菜单 fixed、0px radius、4 个选项。

## Comparison history

1. 首轮发现设置页 `.settings-page button:focus-visible` 覆盖共享 Select，产生 3px 绿色外圈（P1）。
2. 提高共享 Select focus/open 规则的组件作用域优先级，同时明确 `outline-offset: 0`。
3. 复验得到 outline 0px、顶边 0px、底边 1px，P1 已关闭。
4. 用户复查发现菜单的 5px 硬阴影会形成底部“背板”（P2）；移除 box-shadow 后菜单只保留单层 hairline 边框。

## Findings

- 无剩余 P0/P1/P2 视觉或交互问题。

## Follow-up polish

- P3：后续可在真实长选项数据出现时补充菜单横向溢出与 tooltip 体验。

final result: passed
