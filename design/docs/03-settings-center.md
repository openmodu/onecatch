# PRD 03 设置中心技术设计

## 1. 设计结论

新增独立 settings 领域，不把应用配置塞进 `internal/data` 或现有 workflow repo：

```text
Wails SettingsBinding -> localapp Settings service -> internal/repo/settings -> pkg/localfile
                                                |-> runtime registry reload
                                                |-> cleanup/diagnostics services
```

设置是本机应用配置；Workflow/Run 仍是业务快照。设置保存成功后通过明确的 reload 方法更新 runtime registry，不让 repo 反向依赖运行资源。

## 2. 数据模型

```go
type Settings struct {
    SchemaVersion int
    Revision      int64
    Runtimes      map[string]RuntimeSettings
    Execution     ExecutionSettings
    Security      SecuritySettings
    Storage       StorageSettings
    Experimental  ExperimentalSettings
    UpdatedAt     time.Time
}
```

所有读取先执行 `NormalizeSettings`，补齐新字段默认值；保存执行 `ValidateSettings` 并要求 `expectedRevision`。磁盘写入使用 `localfile.WriteJSONAtomic`，文件权限 `0600`。

### 2.1 默认值

- Runtime binary/model/env allowlist：空。
- Execution：20 transitions、3 failures、1800 秒 timeout、4 DAG concurrency、10 秒 interrupt grace、workspace-write。
- Security：禁止 full、full 每次确认、诊断不含 Prompt/raw events。
- Storage：永久保留 completed Run、info、20 MB、5 backups、14 天。
- Experimental：远端 Worker关闭。

## 3. 迁移

启动顺序：

1. 如果 `settings.json` 存在，按 schema version 迁移并读取。
2. 如果不存在且 `runtime.json` 存在，读取 binary 字段生成 Settings revision 1。
3. 原子写入 `settings.json` 后把 `runtime.json` 重命名为 `runtime.v0.backup.json`。
4. 任一步失败则保留原文件并返回 `settings_migration_failed`，不部分覆盖。

Worker token 不迁入 settings；`workers.json` 保持独立权限和生命周期。

## 4. Wails Binding

`SettingsBinding`：

- `GetSettings() SettingsView`
- `UpdateRuntimeSettings(input, expectedRevision) SettingsView`
- `UpdateExecutionSettings(input, expectedRevision) SettingsView`
- `UpdateSecuritySettings(input, expectedRevision) SettingsView`
- `UpdateStorageSettings(input, expectedRevision) SettingsView`
- `UpdateExperimentalSettings(input, expectedRevision) SettingsView`
- `ResetSettingsSection(section, expectedRevision) SettingsView`
- `CheckRuntimeDraft(input) RuntimeDraftStatus`
- `GetStorageUsage() StorageUsage`
- `RevealDataRoot()`
- `PreviewCleanup(input) CleanupPreview`
- `ExecuteCleanup(previewToken) CleanupResult`
- `ExportDiagnostics(input) DiagnosticsExport`

SettingsView 不返回环境变量值、Worker token 或完整诊断内容。

## 5. 保存事务与并发

- repo 临界区内重新读取当前 revision。
- revision 不匹配返回 `settings_state_conflict`。
- 写入成功后 revision +1，再触发资源 reload。
- reload 失败时设置文件仍是事实来源；返回 `settings_reload_failed` 并允许应用重启恢复，日志只记录 section 和 revision。
- 每次只更新一个 section，服务端忽略客户端对其他 section 的值。

## 6. Runtime 应用

RuntimeRegistry 改为接收完整 RuntimeSettings：

- binary 空值走 PATH。
- default model 仅在 Step 没有显式 model 时使用。
- env allowlist 生成新子进程环境；值从父进程按 key 读取，不进入持久化或日志。
- draft check 使用 `exec.LookPath` 和 `--version`，不调用 Agent execute。

禁止 allowlist key：`PATH`、`HOME`、`SHELL`、`DYLD_*`、`LD_*`、`BASH_ENV`、`ENV`、`ZDOTDIR`。其余 key 使用 `^[A-Z_][A-Z0-9_]{0,127}$`。

## 7. 设置解析优先级

Run 启动前生成 `ResolvedExecutionSettings`：

```text
step explicit sandbox/model
        > workflow explicit policy
        > global execution/runtime defaults
        > compiled safe defaults
```

Run 快照新增 resolved policy/security grant，但旧 Run 缺失时按启动时已有字段恢复，不读取当前设置补写。

`maxLocalDAGConcurrency` 在 scheduler 每一批 ready nodes 前通过 semaphore 限制；不改变 DAG readiness 和 join 语义。

## 8. 安全授权

- `allowFullSandbox=false` 时，Definition save 和 Run start 都校验，防止旧定义绕过。
- `confirmFullSandboxEveryRun=true` 时 StartRun 接收短时 confirmation token；token 绑定 workflow revision、workspace 和过期时间。
- SettingsBinding 不生成 Full sandbox token；由 TaskRunBinding 的 preview/confirm 流程负责。
- active Run 不因设置降权被异步杀死，但 resume 前重新检查当前安全授权；不通过则保持 paused。

## 9. 存储用量与清理

StorageUsage 通过遍历已知 `~/.oneshot/` 子目录计算，不跟随 symlink。结果带 calculatedAt，可由 UI 手动刷新。

Cleanup preview：

- 只选择 completed/cancelled 且超过 retention 的 Run。
- 排除 active、paused、running、ready。
- 返回 run IDs 的服务端摘要、数量、估算 bytes 和 5 分钟有效 token。
- token 保存于进程内，绑定候选 ID/hash；execute 前重新检查状态，发生变化的 Run 自动跳过。
- 删除逐个 Run 原子移动到 `.trash/<cleanup-id>/`，全部完成后再删除 trash；失败可以重试。

## 10. Diagnostics

导出 ZIP 到用户选择路径：

- 默认包含 app/runtime 版本、脱敏 Settings、目录用量、选中 Run 状态和 logs。
- 路径使用 `$HOME`/`<workspace>` 替换。
- token、环境变量值、Worker token 永不包含。
- Prompt/raw events 需要 Settings 允许且本次导出再次确认。

## 11. UI 联调

左侧主导航新增底部 Settings 入口；Settings 页面内部 section rail 固定宽度约 180px，表单内容最大宽度约 760px。沿用现有 panel、status pill、runtime dot、primary/secondary button 和危险确认样式。

每个 section 维护独立 draft：

- 初始 draft = server view。
- dirty 时离开 section弹出“放弃/留在此处”。
- Save disabled 条件：无 dirty、字段错误、请求中。
- 保存成功替换 view/draft 和 revision，不整页 reload。
- revision conflict 显示 inline banner + `重新加载设置`。

Worker UI 移入 Experimental；开关关闭时 DAG worker select 只显示 Local。

## 12. 错误码

- `settings_invalid`
- `settings_state_conflict`
- `settings_migration_failed`
- `settings_reload_failed`
- `runtime_draft_unavailable`
- `security_full_sandbox_disabled`
- `cleanup_preview_expired`
- `cleanup_state_changed`
- `diagnostics_export_failed`

## 13. 验证计划

- Domain/repo：defaults、validation、revision、atomic write、runtime migration、并发更新 race。
- Runtime：binary draft check、model precedence、env key validation和 secret 不落盘。
- Security：Definition save、Run start/resume 三条边界。
- Cleanup：preview/execute、状态变化跳过、symlink、失败恢复和 active Run 保护。
- Binding：DTO 脱敏和稳定错误码。
- Desktop：dirty guard、section save、conflict、danger confirmation、Worker experimental visibility。
- 回归：serial/DAG snapshots、全量 Go/race、frontend/Wails build。
