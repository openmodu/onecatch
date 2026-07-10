# Issue 01-005: Workflow 桌面编辑与运行监控

状态：待开发

## 来源

- PRD：`design/prd/01-custom-agent-workflows.md`
- 技术方案：`design/docs/01-custom-agent-workflows.md`
- 相关原型：待为 PRD 01 新增；不复用旧服务市场原型的产品规则。

## 目标

让用户在桌面端创建串行可成环工作流，并观察当前步骤、转移历史和人工介入状态。

## 范围

- 工作流列表和内置模板复制。
- Workspace 选择、runtime 可用状态、步骤表单、转移编辑和本地域校验结果展示。
- run 启动、当前步骤、历史、暂停/恢复/终止和补充指令。
- 用线性步骤列表加回边标记表达首版 loop，不引入重型画布依赖。

## 非目标

- 不支持自由拖拽 DAG 画布和并行分支。

## 产品需求

- 保存前明确展示所有校验错误。
- 当前运行步骤、下一转移和暂停原因可观察。
- 危险 sandbox 必须二次确认。

## 技术设计

- 后端改动：无新增业务规则。
- 桌面端 / Wails 改动：使用 `01-004-local-workflow-bindings.md` 的 bindings。
- 前端改动：新增 workflow editor 和 run inspector。
- 数据模型改动：无。
- API 或 binding 改动：无新增。

## 验收标准

- [ ] 用户能从模板创建并保存 review loop。
- [ ] 运行时能看到步骤切换和回边。
- [ ] 用户能暂停、补充指令并恢复。

## 测试计划

- 前端状态测试、Wails 集成测试、视觉回归和手动 loop 冒烟。

## 交付记录

- 待开发。
