# Issue 01-012：Workspace 搜索、置顶与最近使用

状态：已完成

## 来源

- PRD：`design/prd/01-custom-agent-workflows.md`
- 技术方案：`design/docs/01-custom-agent-workflows.md`

## 目标

让个人工作台在 CWD 数量增长后仍能快速定位和切换项目，并持久保留置顶与最近使用顺序。

## 范围

- Workspace 增加 `pinned` 持久化字段。
- 新增置顶和打开 Workspace 的本地应用服务 / Wails binding。
- 支持从列表隐藏 Workspace 引用，不删除目录和历史数据。
- 侧栏默认展示置顶、最近和当前 Workspace，支持搜索与展开全部。
- 当前 Workspace 始终可见，切换后更新 `lastOpenedAt`。

## 非目标

- 不删除磁盘目录。
- 不实现云同步、Workspace 分组或团队共享。
- 不扫描未主动加入的本地目录。

## 产品需求

- 默认紧凑列表最多展示 8 项，优先级为置顶、最近使用、当前选中。
- 用户可按名称和路径模糊搜索全部 Workspace。
- 用户可切换置顶状态，置顶操作不触发 Workspace 切换。
- 搜索为空时可以在紧凑列表与全部列表之间切换。
- 搜索、置顶、清空和展开入口使用纯文本 TUI 标记，不新增图标按钮。

## 技术设计

- 后端：复用 tasks repo 的 Workspace JSON 原子保存，应用服务提供 `OpenWorkspace` 和 `SetWorkspacePinned`。
- 桌面端 / Wails：WorkspaceBinding 暴露两个偏好更新方法。
- 前端：新增纯函数计算紧凑/搜索结果；侧栏使用独立选择按钮与置顶按钮，避免嵌套 button。
- 数据模型：Workspace 新增 `pinned/hidden`，旧 JSON 缺失时按 false 兼容。
- 错误码：沿用 `workspace_not_found`。

## 验收标准

- [x] 置顶和取消置顶在重启后保留。
- [x] 切换 Workspace 更新最近使用顺序。
- [x] 超过 8 个 Workspace 时默认列表受控，搜索和全部列表可访问其余项目。
- [x] 当前 Workspace 不会因紧凑列表截断而消失。
- [x] Workspace 切换继续立即清空旧 Run Inspector。
- [x] 从列表移除后项目文件和历史数据仍保留，重新加入同一路径可恢复。

## 测试计划

- Repo/domain：旧 JSON 兼容、排序与偏好保存。
- Frontend：紧凑结果、搜索、当前项保留。
- 手动：侧栏搜索、置顶、全部/收起和切换。

## 交付记录

- Workspace JSON 兼容新增 `pinned`，tasks repo 统一按 pinned、lastOpenedAt、name 排序。
- `OpenWorkspace` 与 `SetWorkspacePinned` binding 已生成；打开应用、切换项目和置顶操作会更新同一份 Workspace JSON。
- `RemoveWorkspace` 只设置 `hidden=true`；tasks repo 的列表跳过隐藏项，但 GetWorkspace 和历史 Run 解析仍可使用该记录。
- 侧栏默认最多 8 项，当前项不在头部集合时会替换最后一项；`[ / ]` 搜索、`*` 置顶和 `[ all cwd ]` 全部使用纯文本 TUI 标记。
- 前端 helper 覆盖置顶/最近/当前项与路径搜索；Browser 验证搜索无结果和清空状态可用。
