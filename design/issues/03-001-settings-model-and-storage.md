# Issue 03-001: Settings 模型、持久化与迁移

状态：待开发

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

- [ ] 缺失文件返回完整安全默认值。
- [ ] section update 不覆盖其他 section，revision 冲突明确失败。
- [ ] runtime migration 成功/失败都不损坏原配置。
- [ ] 文件权限为 `0600`，设置 DTO 不含 secret 值。

## 测试计划

- defaults/validation 表驱动、migration fixture、并发 CAS race、重启读取。

## 交付记录

- 待开发。
