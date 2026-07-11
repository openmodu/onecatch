# Issue 03-001: Settings 模型、持久化与迁移

状态：已完成

## 来源

- PRD：`design/prd/03-settings-center.md`
- 技术方案：`design/docs/03-settings-center.md`

## 目标

建立统一、可版本化、原子更新的 `~/.oneshot/settings.json`，并安全迁移现有 runtime 配置。

## 范围

- Settings 各 section 模型、defaults、normalize、validation。
- `internal/repo/settings` revision CAS 和原子文件写入。
- `runtime.json` 到 settings schema v1 的一次性迁移和备份。
- section update/reset application service 与 SettingsBinding 基础方法。

## 非目标

- Runtime reload、清理、诊断和前端页面。

## 验收标准

- [x] 缺失文件返回完整安全默认值。
- [x] section update 不覆盖其他 section，revision 冲突明确失败。
- [x] runtime migration 成功/失败都不损坏原配置。
- [x] 文件权限为 `0600`，设置 DTO 不含 secret 值。

## 测试计划

- defaults/validation 表驱动、migration fixture、并发 CAS race、重启读取。
- 实际结果：domain/repo 单测覆盖默认值、危险 env key、迁移成功与失败回滚、`0600`、并发 CAS；全量 race 通过。

## 交付记录

- 新增 `internal/domain/settings` 和 `internal/repo/settings`，实现 schema v1、安全默认值、字段校验、revision CAS 与 `0600` 原子快照。
- `runtime.json` 首次读取时迁移 binary 配置，写入失败会回滚原文件，成功后保留 `runtime.v0.backup.json`。
- 新增 Settings application service、Wails binding、section update/reset 与稳定错误码。
- 验证：defaults/validation、迁移、权限、并发 CAS、全量 `go test ./...` 与 `go test -race ./...` 均通过。
