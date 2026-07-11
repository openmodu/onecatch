# Issue 03-003: 存储清理、日志与诊断导出

状态：已完成

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

- [x] 不跟随 symlink，分类大小和时间可观察。
- [x] 未 Preview、过期 token 和状态变化不能误删。
- [x] active/paused Run 永不清理。
- [x] 默认诊断不含 token、环境值、Prompt、raw events 和完整路径。

## 测试计划

- 临时目录 fixture、清理故障恢复、zip 内容审计和路径脱敏测试。
- 实际结果：覆盖 symlink 不跟随、cleanup token 单次消费与目录移除、ZIP 环境值审计；全量 Go/race 通过。

## 交付记录

- 新增不跟随 symlink 的目录用量分类和 Finder reveal。
- 清理只选择过期 completed/cancelled Run；5 分钟 token 单次消费，执行前复核 revision/status/active，并逐个原子移动到 `.trash` 后删除。
- logger 支持稳定 logger 指针下的 level 和 lumberjack writer 热切换。
- 诊断 ZIP 默认只含脱敏 settings、版本、用量、Run 状态与日志；环境值会从日志二次脱敏，Prompt/runtime events 需设置授权和本次确认。
- 验证：symlink、preview/execute/复用 token、诊断环境值审计及全量 race 通过。
