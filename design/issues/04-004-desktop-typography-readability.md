# Issue 04-004：桌面端字号可读性统一

状态：已完成

## 来源

- PRD：`design/prd/04-mac-desktop-ui-redesign.md`
- 技术方案：`design/docs/04-mac-ui-component-system.md`

## 目标

整体放大桌面端过小字体，并让任务、Workflow、Run Inspector、设置和 DAG 编辑器共用一套可维护的语义字号体系。

## 范围

- 建立从 micro 到 display 的共享字号 token。
- 将旧页面中 7–16px 的直接字号声明迁移到共享 token。
- 提高侧栏、表单、列表、状态、对话、工具调用、设置与 DAG 辅助信息的可读性。
- 调整 Run Inspector 的状态列与时间列宽度，保持事件时间右对齐。

## 非目标

- 不改变页面信息架构、色彩主题或业务交互。
- 不提供用户自定义缩放设置。
- 不修改 Wails binding、后端或持久化数据。

## 产品需求

- 用户可见辅助文字最小为 11px，不再出现 7–10px 难以辨认的正文或元数据。
- 常规正文使用 14px，Agent 对话正文使用 15px，并保持舒适行高。
- 相同语义文字必须从同一个 token 取值，后续调整一处即可全局生效。
- 字号增大后，事件时间仍需在 Inspector 右侧纵向对齐，列表和控件不得出现明显裁切。

## 技术设计

- 后端改动：无。
- 桌面端 / Wails 改动：无。
- 前端改动：在 `ui/tokens.css` 增加语义字号、事件状态列和时间列 token；`styles.css` 与 `mirage.css` 的字号声明统一消费这些 token。
- 数据模型改动：无。
- API 或 binding 改动：无。
- 错误码：无。

## 验收标准

- [x] 全局不存在 11px 以下的用户可见字号规格。
- [x] 侧栏、任务列表、Workflow、Run Inspector、设置和 DAG 编辑器字号整体提升且层级一致。
- [x] 按钮、输入框、状态标签与说明文字使用共享字号 token。
- [x] Inspector 中用户、Agent、Tool Use 的时间列在放大后继续右对齐。
- [x] 前端单元测试、production build 与 CSS diff check 通过。

## 测试计划

- 静态检查：扫描 CSS，确认页面样式不再直接声明小于 11px 的字号。
- 前端：执行 `npm test` 与 `npm run build`。
- 手动：检查任务工作台、Run Inspector、Workflow、设置和 DAG 编辑器的文字层级、裁切与滚动。

## 交付记录

- 新增 11–29px 的语义字号层级，并保留现有 primitive 的兼容别名。
- 旧版和当前主题样式中的直接字号统一迁移到共享 token，最小字号从 7–10px 提升到 11px。
- 默认正文从 12px 提高到 14px，对话正文从 13px 提高到 15px。
- Inspector 时间列由固定 55px 改为 64px token，工具状态列改为 50px token，放大后继续保持列对齐。
- 本地 1280×720 预览验证任务页、Run Inspector 与设置页无横向溢出；DAG 二次校准为节点标题 14px、说明 12px、Inspector 字段 13px，未发现文字裁切。
