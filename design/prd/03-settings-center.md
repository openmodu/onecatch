# PRD 03: 本地设置中心

状态：设计完成，待开发

## 1. 背景

Oneshot 已具备本地 Runtime、Workspace、串行 Loop、并行 DAG 和实验性远端 Worker，但配置入口分散：Workspace 只在添加时选择 sandbox，Runtime binary 有 binding 没有 UI，Workflow policy 只暴露部分字段，数据与日志只能从文件系统观察。

设置中心需要把“影响所有新任务的默认值”和“只影响本机应用行为的配置”集中起来，同时保持 local-first、安全默认和 Run 快照不可变。

## 2. 目标

- 用户可以集中检查和配置 Codex、Claude Code 的 binary、默认 model 和环境变量白名单。
- 用户可以设置新 Workflow/Run 的执行默认值，包括保护上限、超时、并发数和默认 sandbox。
- 用户可以控制 Full sandbox、危险操作确认和远端 Worker 实验开关。
- 用户可以查看 `~/.oneshot/` 用量、日志策略，预览并执行历史清理。
- 设置变更有字段校验、revision 冲突保护和清晰的生效范围。

## 3. 非目标

- 不允许在首版修改数据根目录；继续固定为 `~/.oneshot/`。
- 不做账号、云同步、团队权限、计费或远端配置同步。
- 不在设置文件中保存环境变量 secret 值；只保存允许继承的变量名。
- 不让全局设置追溯修改已保存 Workflow 或已启动 Run。
- 不在本阶段实现自动更新、插件市场和任意 CLI 参数注入。

## 4. 信息架构

设置作为左侧主导航底部的独立入口，页面内部使用二级 section rail：

1. **Runtime**：Codex、Claude Code 可用性、binary、默认 model、环境变量白名单。
2. **执行策略**：默认 policy、DAG 并发、默认 sandbox、中断宽限时间。
3. **安全**：Full sandbox 总开关、每次确认、诊断脱敏策略。
4. **存储与日志**：数据根目录、空间占用、保留策略、日志级别、导出诊断。
5. **实验性**：远端 Worker 总开关和 Worker 管理入口，默认关闭。

原有“多机 Worker”从主导航移入实验性 section。关闭实验性开关时保留 `workers.json`，但 UI 不允许给新节点选择远端 worker，启动包含远端节点的 Workflow 时返回明确错误。

## 5. 保存与生效规则

- 每个 section 独立保存；编辑后出现 sticky action bar：`放弃更改` / `保存设置`。
- 保存使用 `expectedRevision`，防止多个窗口覆盖。
- Runtime binary 保存前执行可执行文件检查和 `--version` 探测。
- 安全降权立即影响后续启动；不会强制终止 active Run。
- 执行默认值只用于新建 Workflow 或未显式设置的字段；已有 Workflow 保持自己的 policy。
- Run 启动时复制 resolved settings/security grant 到 Run 快照，之后不受设置变化影响。
- 每个 section 提供“恢复默认值”，危险设置恢复前二次确认。

## 6. Runtime 设置

每个 Runtime 使用独立状态卡：

- 安装状态、版本、最近检查时间。
- Binary：空值表示从 PATH 自动发现；支持系统目录选择器或手工输入。
- Default model：空值表示 runtime 默认；自由文本但去除首尾空格和控制字符。
- Environment allowlist：只存变量名，例如 `HTTP_PROXY`、`GITHUB_TOKEN`；值从 Oneshot 进程环境继承，UI 永不显示值。
- `测试配置` 使用未保存草稿启动只读 version check，不执行 Agent，不消耗模型额度。

首版不允许自定义任意 CLI flags，避免绕过 sandbox 和协议参数。

## 7. 执行策略

默认值：

| 字段 | 默认 | 范围 | 生效对象 |
| --- | ---: | ---: | --- |
| 最大转移次数 | 20 | 1–10000 | 新 Workflow |
| 最大连续失败 | 3 | 1–100 | 新 Workflow |
| 单节点超时 | 1800 秒 | 30–86400 | 新 Workflow |
| 本地 DAG 最大并发 | 4 | 1–16 | 新 Run |
| 中断宽限时间 | 10 秒 | 1–60 | 后续打断 |
| 默认 Sandbox | workspace-write | read-only/workspace-write | 新 Workspace/节点 |

优先级：节点显式配置 > Workflow policy > 全局默认值。`full` 不作为默认 sandbox 选项。

DAG 并发数限制同时 running 的 read-only 节点；write 节点仍受 Workspace lock 和依赖关系约束。

## 8. 安全设置

- `允许 Full sandbox`：默认关闭。关闭时保存或启动包含 full 节点的 Workflow 会被拒绝。
- `每次启动 Full sandbox 前确认`：默认开启；仅在允许 full 时可编辑。
- `诊断包包含 Prompt`：默认关闭；开启时每次导出仍需二次确认。
- `诊断包包含原始 Runtime event`：默认关闭。
- 环境变量只允许合法大写 key；拒绝 `PATH`、`HOME`、shell startup 和动态 loader 相关高风险 key 的覆盖，只允许继承既有值。

安全设置降权后，已有定义仍可查看和编辑，但不能新启动违反当前授权的 Run。

## 9. 存储与日志

- 数据根目录只读显示为 `~/.oneshot/`，提供“在 Finder 中显示”。
- 显示 Workflows、Runs、Events、Logs 各自大小和最后计算时间。
- 默认保留全部历史；用户可配置 completed/cancelled Runs 保留天数：`永久/30/90/180`。
- 清理必须先 Preview，返回数量、估算空间和不可逆提示；执行时使用 preview token，避免状态变化后误删。
- active/paused Runs、Workspace、Workflow 模板和 Worker 配置永不被自动清理。
- 日志默认 `info`、单文件 20 MB、5 个备份、保留 14 天；允许选择 `error/warn/info/debug`，debug 明确提示可能增加本地磁盘使用。
- 诊断导出默认包含版本、脱敏 settings、Run 状态、错误码和日志，不包含 token、环境变量值、Prompt 和完整本地路径。

## 10. 实验性设置

- `启用远端 Worker` 默认关闭。
- 开启前展示受信任 LAN/VPN、无项目同步、远端只读节点的边界说明。
- 开启后在同一 section 展示现有 Worker 管理卡片。
- 关闭不会删除 Worker token 或配置；远端 worker 不再出现在 DAG 节点选择器中。

## 11. 空状态、错误和无障碍

- Runtime 未安装：显示安装缺失，不把 binary 输入错误混成未安装。
- 配置无效：字段旁显示错误，Save disabled；顶部不只依赖 toast。
- Revision 冲突：提示设置已在另一窗口变化，提供重新加载，不自动覆盖。
- 所有切换项有完整 label/description；键盘可完成 section 切换、输入和保存。
- 危险项不只用颜色表达，必须包含“危险”文案和确认对话框。

## 12. 数据模型

`~/.oneshot/settings.json`：

- `schemaVersion`、`revision`、`updatedAt`
- `runtimes.codex/claude`：binary、defaultModel、environmentAllowlist
- `execution`：默认 policy、maxLocalDAGConcurrency、interruptGraceSeconds、defaultSandbox
- `security`：allowFullSandbox、confirmFullSandboxEveryRun、diagnosticsIncludePrompt、diagnosticsIncludeRawEvents
- `storage`：completedRunRetentionDays、logLevel、logMaxSizeMB、logMaxBackups、logMaxAgeDays
- `experimental`：remoteWorkersEnabled

现有 `runtime.json` 首次读取时迁移到 `settings.json`；迁移成功后保留原文件为一次性备份，后续只写新设置文件。`workers.json` 继续独立保存 token。

## 13. 验收标准

- [ ] 设置中心五个 section 的字段、默认值和说明与本文一致。
- [ ] 每个 section 独立保存，dirty state、放弃、revision 冲突可观察。
- [ ] Runtime 草稿可以无额度检查 binary/version。
- [ ] 执行默认值只影响新对象，active/historical Run 不变化。
- [ ] Full sandbox 总开关能阻止新定义保存和新 Run 启动。
- [ ] 数据清理必须 Preview 后使用 token 执行，且不会删除 active/paused Run。
- [ ] 设置列表和诊断导出不暴露 token、环境变量值或未授权 Prompt。
- [ ] 远端 Worker 默认关闭，关闭时本地 DAG 正常使用且远端配置保留。
- [ ] `runtime.json` 可安全迁移，旧 Workflow/Run 不受影响。

## 14. 待实现后再评估

- 是否使用 macOS Keychain 保存用户主动录入的 secret 环境变量值。
- 是否允许迁移数据根目录和外置存储。
- 是否增加系统菜单、开机启动、自动更新和通知设置。
