# Issue 03-004: 桌面设置中心

状态：已完成

## 来源

- PRD：`design/prd/03-settings-center.md`
- 技术方案：`design/docs/03-settings-center.md`
- 前置：`design/issues/03-001-settings-model-and-storage.md`、`design/issues/03-002-runtime-and-execution-settings.md`、`design/issues/03-003-storage-cleanup-and-diagnostics.md`

## 目标

按统一信息架构提供 Runtime、执行、安全、存储和实验性五个 section，并把远端 Worker 降级为默认关闭的实验功能。

## 范围

- Settings 主导航入口、二级 section rail、独立 draft/save/reset。
- 字段说明、inline validation、dirty guard、revision conflict 和危险确认。
- Runtime 状态卡、执行/安全表单、StorageUsage/cleanup/diagnostics。
- Worker 管理移入 Experimental，关闭时 DAG 只显示 Local。

## 非目标

- 独立系统窗口、云端同步、自动更新。

## 验收标准

- [x] 五个 section 和字段顺序与 PRD 一致。
- [x] 每区独立 dirty/save/reset，离开保护和冲突恢复可用。
- [x] 键盘、label、错误和危险状态不只依赖颜色。
- [x] Worker 默认关闭，本地 Task/Loop/DAG 主流程不受影响。

## 测试计划

- 前端状态测试、Wails 集成、视觉与键盘验收、production/dev build。
- 实际结果：在 1440×900 同视口捕获并检查五分区、dirty/invalid/confirm 状态、默认 Worker 隐藏和保存后显隐；确认框 Escape、焦点恢复与危险操作文案通过键盘验收；production frontend 与 Wails dev build 通过。

## 交付记录

- 左侧底部新增 Settings，内部提供 Runtime、执行、安全、存储与日志、实验功能五个 section。
- 每区独立 draft/save/reset，包含离开保护、放弃更改、字段校验、sticky action bar、revision conflict 重载和危险确认。
- StorageUsage、cleanup preview/execute、diagnostics、Runtime draft check 均接入真实 binding。
- Worker 主导航已移除；默认关闭时管理 UI 隐藏、DAG 只显示 Local，开启后在 Experimental 内复用管理卡片。
- 设置页完成交互风格收口：提升 section/字段层级与可读性，干净状态不常驻操作条，dirty 时显示分区标记和 sticky save bar，字段错误使用顶部摘要 + 就地提示，依赖项明确 disabled，Storage 自动计算并可视化分类占用。
- 浏览器原生 `confirm` 已替换为统一应用内危险确认框，覆盖 Full access、Worker 删除、Workflow 保存/启动、清理和敏感诊断导出；支持遮罩取消、Escape、Tab 焦点约束和关闭后焦点恢复。
- 验证：浏览器真实前端捕获确认五分区和 Worker 显隐，无前端 error；`npm run build`、`wails3 build DEV=true` 通过。macOS 原生截图因系统录屏权限返回黑屏，Wails binding/桌面能力由构建和集成测试覆盖。
