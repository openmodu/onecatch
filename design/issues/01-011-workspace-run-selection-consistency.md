# Issue 01-011: Workspace 与 Run 选中态一致性

状态：已完成

## 来源

- PRD：`design/prd/01-custom-agent-workflows.md`
- 技术方案：`design/docs/01-custom-agent-workflows.md`

## 目标

保证任务工作台的 Workspace、最近运行列表和右侧 Run Inspector 始终属于同一个工作目录，消除切换 CWD 后残留其他目录 Run 详情的状态串台。

## 范围

- 切换 Workspace 时立即重置当前任务、运行列表和 Run 详情。
- 当前 Workspace 没有运行记录时，右侧展示对应空状态。
- 异步加载任务或 Run 详情时忽略已经过期的响应。
- 运行列表刷新后校验当前选中的 Run 是否仍属于该列表。

## 非目标

- 不修改 Workspace、Task 或 Run 的持久化结构。
- 不改变 Run 调度、恢复或终止语义。
- 不新增跨 Workspace 聚合运行视图。

## 产品需求

- 用户点击另一个 CWD 后，中间“最近运行”与右侧详情必须同步切换数据范围。
- 新 CWD 没有 Run 时，右侧不得继续显示上一个 CWD 的任务、消息或工具调用。
- 快速连续切换 CWD 时，较早请求晚返回也不得覆盖当前 CWD 的界面。
- 当前运行列表刷新后，如果原选中 Run 已不存在，Inspector 应恢复为空状态。

## 技术设计

- 后端改动：无。
- 桌面端 / Wails 改动：无 binding 变化。
- 前端改动：统一使用 `selectWorkspace` 作为切换入口并同步清空 Workspace 派生状态；`loadTasks` 与 `loadRun` 使用请求版本号拒绝过期响应；通过 `runPairsContain` 校验选中 Run 是否仍在当前 Workspace 的任务运行集合中。
- 数据模型改动：无，仅调整 React UI 状态生命周期。
- API 或 binding 改动：无。
- 错误码：无。

## 验收标准

- [x] 切换到无运行记录的 Workspace 后，中间显示 0 runs，右侧显示空状态。
- [x] 右侧不再保留上一个 Workspace 的 Run 摘要、消息或工具记录。
- [x] 切换到有运行记录的 Workspace 后，需要用户从当前列表选择 Run 才展示详情。
- [x] 快速切换 Workspace 时，过期任务列表和 Run 详情响应不会恢复旧数据。
- [x] 当前列表不包含已选 Run 时，选中态、详情和恢复指令一起清空。

## 测试计划

- Node：验证运行集合只接受当前列表内的 Run ID。
- Build：执行前端单元测试和 production build。
- 手动：在有 Run 与无 Run 的两个 Workspace 之间连续切换，检查中间列表、右侧空状态与异步响应一致性。

## 交付记录

- 新增统一的 Workspace 选择入口，切换时主动失效进行中的任务与详情请求。
- 任务列表和 Run 详情加载分别使用递增版本号，过期响应不再写入 React 状态。
- 新增 `workspaceRunSelection` 纯函数及测试，用于刷新后核对选中 Run 的归属。
- Inspector 空状态按当前 Workspace 的运行数量显示，不复用旧 Run 内容。
