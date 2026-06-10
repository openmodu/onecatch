# Issue 00-003: Agent 目录

状态：待开发

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

## 验收标准

- [ ] 左侧导航展示 Agent 分类。
- [ ] 切换分类时 Agent 列表变化。
- [ ] 用户可以选择 Agent。
- [ ] 中间工作区展示选中 Agent 详情。
- [ ] 选中 Agent 的价格展示在扣次确认和 Inspector 中。
- [ ] 未知 Agent ID 返回 not found 错误。

## 测试计划

- 单元测试内存目录仓储。
- 测试 Agent 列表和详情 API。
- 桌面 binding 冒烟测试。
- 手动验证分类切换和 Agent 切换。

## 交付记录

- 当前 prototype 中有 4 个 mock Agent。
- 当前 Go domain 里只有 1 个种子 Agent，需要扩展到 MVP 范围。
