# Issue 04-003：Action Hover 对比度修复

状态：已完成

## 来源

- PRD：`design/prd/04-mac-desktop-ui-redesign.md`
- 技术方案：`design/docs/04-mac-ui-component-system.md`
- 问题截图：暂停 Run 的“终止”操作在 hover 后文字不可见。

## 目标

修复共享 Action 在 hover 时文字与背景变成同色的问题，并保证所有语义 variant 使用对应背景色。

## 范围

- accent、cyan、danger、muted 和 primary Action hover。
- 暂停 Run 的“终止”按钮视觉验证。

## 非目标

- 修改按钮文案、布局、点击行为或运行状态机。

## 产品需求

- hover 后文字与背景必须保持清晰对比。
- 不同 Action variant 保持各自语义色。
- disabled Action 不应呈现可点击的 hover 反馈。

## 技术设计

- 删除依赖最终 `currentColor` 的通用背景规则。
- 为每个 Action variant 显式指定 hover background。
- 保留统一的 hover 前景色和 primary 特例。

## 验收标准

- [x] “终止”按钮 hover 使用 danger 背景与 canvas 前景，不再同色。
- [x] accent、cyan、danger、muted、primary hover 显式映射语义色。
- [x] frontend production build 通过。

## 测试计划

- 浏览器悬停暂停 Run 的“终止”按钮并截图。
- 运行 frontend production build 和 `git diff --check`。

## 交付记录

- 将通用 `background: currentColor` 拆分为各 Action variant 的显式 hover background。
- 通用 hover 仅负责设置 canvas 前景色，disabled variant 通过 `:not(:disabled)` 排除。
- frontend production build、Wails dev build 和 `git diff --check` 通过。
- 自动化浏览器在本轮验证时无法重新连接本地预览，因此没有生成 hover 截图；CSS cascade 与 Wails bundle 已验证。
