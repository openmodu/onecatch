# Issue 00-001: 桌面工作台框架

状态：已完成

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
- Wails dev/build 任务按顺序执行 bindings 生成、前端依赖安装和前端构建，避免 npm install 与 Go module 扫描或 dev server 并发冲突。

## 验收标准

- [x] 桌面端可通过 `wails3 dev -config ./build/config.yml` 启动。
- [x] 布局包含左侧导航、中间工作区、右侧 Inspector。
- [x] 右侧 Inspector 可以折叠和展开。
- [x] 桌面目标宽度下内容不重叠。
- [x] 前端通过 `npm run build`。
- [x] Go 包通过 `go test ./...`。

## 测试计划

- 已运行 `env GOCACHE=/private/tmp/oneshot-go-build go test ./...`，通过。
- 已运行 `cd desktop/oneshot/frontend && npm run build`，通过。
- 已运行 `cd desktop/oneshot && env GOCACHE=/private/tmp/oneshot-go-build wails3 build DEV=true`，通过。
- 已运行 `cd desktop/oneshot && wails3 dev -config ./build/config.yml -port 9246`，成功构建、启动 Vite dev server 并连接前端 dev server。
- 已用 in-app Browser 验证 1280px 和 960px 桌面宽度下无 body 横向溢出，左侧导航、中间工作区、右侧 Inspector 都在 viewport 内。
- 已用 in-app Browser 验证 Inspector 收起后显示窄轨入口，再次展开后仍保留选中 Agent 上下文，控制台无 error。

## 交付记录

- 当前已有 `desktop/oneshot` 的 Wails v3 初始骨架。
- 已将 `desktop/oneshot/frontend/src/app/App.jsx` 从最小占位界面替换为三段式桌面工作台框架。
- 已实现左侧导航、Agent 分类、次数与订单入口、账号区、中间工作区、扣次确认、执行进度和右侧 Inspector 框架。
- 已实现右侧 Inspector 展开和折叠状态，折叠态保留当前选中 Agent 上下文。
- 已调整 `desktop/oneshot/build/Taskfile.yml` 和 `desktop/oneshot/build/config.yml`，避免 Wails dev/build 中前端依赖安装与构建任务并发冲突。
- 已将 `desktop/oneshot/frontend/.node_modules/` 加入 `.gitignore`，配合 task 使用隐藏依赖目录和 `node_modules` symlink，避免 Go 扫描前端依赖树。
- 真实登录、真实支付、生产订单执行和 Agent API 接入仍按后续 issue 交付。
