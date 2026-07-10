# Issue 03-003: 存储清理、日志与诊断导出

状态：待开发

## 来源

- PRD：`design/prd/03-settings-center.md`
- 技术方案：`design/docs/03-settings-center.md`
- 前置：`design/issues/03-001-settings-model-and-storage.md`

## 目标

让用户看清本地数据占用，并以可预览、可恢复、默认脱敏的方式清理和导出诊断。

## 范围

- StorageUsage 目录统计和 Finder reveal。
- completed/cancelled Run cleanup preview token、trash 和执行结果。
- 日志策略设置 reload。
- 脱敏 diagnostics ZIP 导出和敏感内容二次确认。

## 非目标

- 移动数据根目录、云端上传诊断。

## 验收标准

- [ ] 不跟随 symlink，分类大小和时间可观察。
- [ ] 未 Preview、过期 token 和状态变化不能误删。
- [ ] active/paused Run 永不清理。
- [ ] 默认诊断不含 token、环境值、Prompt、raw events 和完整路径。

## 测试计划

- 临时目录 fixture、清理故障恢复、zip 内容审计和路径脱敏测试。

## 交付记录

- 待开发。
