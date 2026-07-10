# Issue 01-005: Workflow 桌面编辑与运行监控

状态：已完成

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
- 旧市场、登录、计费和订单 UI 从桌面入口移除，不保留双模式。

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

- [x] 用户能从模板创建并保存 review loop。
- [x] 运行时能看到步骤切换和回边。
- [x] 用户能暂停、补充指令并恢复。

## 测试计划

- 前端状态测试、Wails 集成测试、视觉回归和手动 loop 冒烟。

## 交付记录

- 完整替换旧服务市场 UI，新增本地工作目录、runtime 健康状态、Task composer、最近 Run 与 Run inspector。
- Workflow 页面提供单 Agent / 实现审查模板、步骤表单、runtime/sandbox、role/instruction、signal/target 和 policy 编辑；领域 validation issues 原样显示。
- Run inspector 展示当前步骤、转移计数、步骤执行记录和 runtime stream，支持运行中打断、暂停态补充指令恢复以及永久终止。
- Full sandbox 在保存 Workflow 和启动 Run 前二次确认；空状态、错误 toast、后台轮询与 `~/.oneshot/` 本地存储提示已接入。
- 浏览器开发模式仅在 Wails runtime 不存在时展示 demo 数据，便于 UI 开发；生产桌面使用生成 bindings。
- 验证：前端 production/dev build 通过；1280×720 实际浏览器视觉检查通过，Workflow 编辑器打开、校验、保存交互通过；Wails 桌面 build 和进程启动冒烟通过。
