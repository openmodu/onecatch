# Issue 00-003: Agent 目录

状态：已完成

## 来源

- PRD：`design/prd/00-agent-marketplace-mvp.md`
- 相关原型：`design/prototype/`

## 目标

实现 Agent 目录能力，让用户可以浏览分类、选择 Agent，并理解价格、能力、成交次数、评分和交付物预期。

## 范围

- Agent 分类列表。
- Agent 列表。
- 分类筛选。
- Agent 详情。
- MVP 初始 Agent 种子数据。
- 后端 API 和桌面端 binding。

## 非目标

- 用户自助上架 Agent。
- Agent 评价审核。
- 动态定价。
- 个性化推荐。

## 产品需求

- 分类包含全部、内容创作、数据分析、市场营销、设计创意、开发与技术、办公效率、行业研究。
- Agent 详情展示名称、分类、标签、简介、价格、评分、成交次数、平均交付时间和交付物说明。
- 选择 Agent 后，中间工作区和扣次确认信息同步更新。
- 切换分类后，Agent 列表同步变化。

## 技术设计

- 后端接口：
  - `GET /api/agents`
  - `GET /api/agents/{id}`
- 桌面端 bindings：
  - `AgentBinding.ListAgents()`
  - `AgentBinding.GetAgent(agentID)`
- 领域对象：`Agent`。
- 仓储可以先使用内存种子数据，后续迁移到 MySQL。
- 前端应把目录数据视为服务端数据，不把 prototype mock 当生产数据源。
- 实际实现会扩展 `Agent` 字段，覆盖标签、评分、成交次数、平均交付时间、展示价格和交付物说明；扣次仍由 `priceUses` 表达。
- 桌面前端通过 `AgentBinding.ListAgents()` 加载目录数据，分类筛选和 Agent 选择基于服务端返回结果。

## 验收标准

- [x] 左侧导航展示 Agent 分类。
- [x] 切换分类时 Agent 列表变化。
- [x] 用户可以选择 Agent。
- [x] 中间工作区展示选中 Agent 详情。
- [x] 选中 Agent 的价格展示在扣次确认和 Inspector 中。
- [x] 未知 Agent ID 返回 not found 错误。

## 测试计划

- 已新增内存 Agent 目录 repo 测试，覆盖 seed 数量、详情字段和未知 ID not found。
- 已新增 HTTP handler 测试，覆盖 `GET /api/agents`、`GET /api/agents/{id}` 和未知 Agent 404。
- 已运行 `env GOCACHE=/private/tmp/oneshot-go-build go test ./...`，通过。
- 已运行 `cd desktop/oneshot/frontend && npm run build`，通过。
- 已运行 `cd desktop/oneshot && env GOCACHE=/private/tmp/oneshot-go-build wails3 build DEV=true`，通过，并重新生成 Agent binding model。
- 已运行 `cd desktop/oneshot && wails3 dev -config ./build/config.yml -port 9246`，Vite dev server 启动并连接成功。
- 当前会话没有可用 in-app Browser/Playwright/Chromium 工具，已用 `curl http://127.0.0.1:9246/` 和 `/src/main.jsx` 做页面可访问冒烟。

## 交付记录

- 当前 prototype 中有 4 个 mock Agent。
- 当前 Go domain 里只有 1 个种子 Agent，需要扩展到 MVP 范围。
- 已将 Go domain seed 扩展为 4 个 MVP Agent：行业研究分析师、内容增长写手、经营数据分析师、新品上市策划。
- 已扩展 Agent 字段：`tags`、`priceCents`、`rating`、`dealCount`、`deliverable`，保留 `priceUses` 作为扣次单位。
- 已让桌面前端通过 `AgentBinding.ListAgents()` 加载目录数据，不再使用 prototype mock Agent。
- 已实现分类筛选、Agent 选择、空分类提示，以及价格在扣次确认和 Inspector 同步展示。
