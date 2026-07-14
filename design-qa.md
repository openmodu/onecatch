# Design QA

## Comparison Target

- Source visual truth：`/var/folders/nz/tjb3cj6s3cb3jrvrp27yf9x00000gn/T/codex-clipboard-39403eb7-70cb-429f-b547-b8936e1013e2.png`
- Implementation paused-state screenshot：`/tmp/oneshot-resume-paused.png`
- Implementation after one click：`/tmp/oneshot-resume-single-click.png`
- Combined comparison：`/tmp/oneshot-resume-qa-comparison.png`
- Source viewport：1557 × 1130；对照时裁出 1052 × 1130 的应用区域并等比缩放到 720px 高。
- Implementation viewport：1280 × 720。
- State：Run paused，底部显示补充指令、终止和恢复操作；另验证单击恢复后的 running 状态。

## Findings

- 未发现本次恢复控制区域存在可执行的 P0/P1/P2 差异。
- Source 与 implementation 的窗口比例和内容数据不同，因此不对全屏栏宽和文本换行做像素级结论；同状态的底部操作区在组合对照中保持同一 TUI 层级、按钮顺序、颜色语义和右对齐关系。
- 单击一次“恢复运行”后，预览中恢复按钮消失并出现“打断并暂停”，确认主交互无需第二次点击。
- Wails 的异步 paused → running 间隙无法在同步 demo 中稳定截图；该间隙由 `恢复中` disabled 状态的单元测试和 production 代码路径覆盖。

## Required Fidelity Surfaces

- Fonts and typography：沿用现有等宽字体、字号、字重和 bracket button 文案；本次未引入新的字体或层级。
- Spacing and layout rhythm：textarea、按钮容器、按钮间距和 Inspector 底部边界均复用现有结构；pending 只替换文字和 disabled 状态，不造成布局跳动。
- Colors and visual tokens：恢复按钮继续使用既有 primary 绿色，终止按钮继续使用 danger 红色；disabled 由既有 Action 与 form token 处理。
- Image quality and asset fidelity：该区域没有图片、品牌图形或新增资产；不适用额外图片质量检查。
- Copy and content：默认文案为“恢复运行”，请求中为“恢复中”，中断尚未落盘时为“等待停止”，状态含义明确且符合当前 TUI 风格。

## Focused Region Evidence

- 组合对照已经将 source 的 Inspector 底部操作区保留在可读尺寸，恢复按钮和输入区域无需额外裁切即可判断；因此没有再生成单独 focused crop。

## Comparison History

- 第 1 次对照：未发现 changed area 的 P0/P1/P2 问题，因此没有视觉修复迭代。
- 交互补充验证：单击恢复后 `恢复运行` 数量从 1 变为 0，`打断并暂停` 数量变为 1；浏览器 console error 为 0。

## Implementation Checklist

- [x] 单击提交期间立即显示“恢复中”并禁用重复点击。
- [x] binding 已受理但持久化状态仍 paused 时继续保持 pending。
- [x] Run 尚在停止时显示“等待停止”并禁止恢复。
- [x] 延迟刷新只更新当前仍被选中的 Run。
- [x] 前端 22 项测试、production build、`go test ./...` 和 `wails3 build` 通过。

## Follow-up Polish

- 无本次交付必须处理的 P3 项。

final result: passed
