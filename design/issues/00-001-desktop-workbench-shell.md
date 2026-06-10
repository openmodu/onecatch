# Issue 00-001: 桌面工作台框架

状态：待开发

## 来源

- PRD：`design/prd/00-agent-marketplace-mvp.md`
- 相关原型：`design/prototype/`

## 目标

实现 Wails v3 桌面工作台框架，承载原型中的三段式产品结构，并为后续 Agent 市场、订单、计费和交付流程提供基础界面。

## 范围

- Wails v3 应用窗口初始化。
- React/Vite 前端迁移目标目录：`desktop/oneshot/frontend`。
- 三段式布局：左侧导航、中间工作区、右侧 Inspector。
- 右侧 Inspector 展开和折叠状态。
- 桌面宽度下稳定布局，各 pane 独立滚动。

## 非目标

- 真实 OAuth 登录。
- 真实支付。
- 生产级订单执行。
- 完整像素级还原 prototype。

## 产品需求

- 首屏必须是可工作的桌面应用，不做营销页。
- 左侧用于导航、Agent 分类、用量和订单入口、账号区域。
- 中间用于 Agent 和订单主流程。
- 右侧用于用量、订单详情和交付物预览。
- Inspector 可折叠为窄轨入口，并能再次展开；折叠和展开不应丢失当前选择上下文。

## 技术设计

- 使用 `desktop/oneshot/main.go` 初始化 Wails 应用。
- 桌面端专属 Go 代码放在 `desktop/oneshot/app` 和 `desktop/oneshot/bindings`。
- 共享业务规则放在 `internal/domain` 和 `internal/service`。
- 框架 UI 放在 `desktop/oneshot/frontend/src`。
- Wails 生成的 bindings 放在 `desktop/oneshot/frontend/bindings`。

## 验收标准

- [ ] 桌面端可通过 `wails3 dev -config ./build/config.yml` 启动。
- [ ] 布局包含左侧导航、中间工作区、右侧 Inspector。
- [ ] 右侧 Inspector 可以折叠和展开。
- [ ] 桌面目标宽度下内容不重叠。
- [ ] 前端通过 `npm run build`。
- [ ] Go 包通过 `go test ./...`。

## 测试计划

- 运行 `go test ./...`。
- 运行 `cd desktop/oneshot/frontend && npm run build`。
- 运行 `cd desktop/oneshot && wails3 build DEV=true`。
- 手动验证 Inspector 展开和折叠状态。

## 交付记录

- 当前已有 `desktop/oneshot` 的 Wails v3 初始骨架。
- 当前前端仍是最小占位界面，后续需要迁移完整 prototype。
