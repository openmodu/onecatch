# Issue 03-004: 桌面设置中心

状态：待开发

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

- [ ] 五个 section 和字段顺序与 PRD 一致。
- [ ] 每区独立 dirty/save/reset，离开保护和冲突恢复可用。
- [ ] 键盘、label、错误和危险状态不只依赖颜色。
- [ ] Worker 默认关闭，本地 Task/Loop/DAG 主流程不受影响。

## 测试计划

- 前端状态测试、Wails 集成、视觉与键盘验收、production/dev build。

## 交付记录

- 待开发。
