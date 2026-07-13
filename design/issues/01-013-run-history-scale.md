# Issue 01-013：Run 历史搜索、筛选与分页

状态：已完成

## 来源

- PRD：`design/prd/01-custom-agent-workflows.md`
- 技术方案：`design/docs/01-custom-agent-workflows.md`

## 目标

让当前 Workspace 的 Run 历史在百级、千级数据下仍能快速检索、增量加载并保持 Inspector 数据一致。

## 范围

- Run repo 增加可自动重建的派生摘要索引，以及稳定排序、keyword/status 与 cursor 分页查询。
- 应用服务按 Workspace 聚合 Task 摘要与 Run，返回分页 DTO。
- 运行列表增加搜索、状态筛选、总数和加载更多状态。
- 前端使用固定行高窗口化渲染，接近底部时加载下一页。
- 搜索、筛选与 Workspace 变化时清理不再匹配的 Run 详情。

## 非目标

- 不迁移到数据库。
- 不删除或归档历史 Run。
- 不搜索完整消息、工具输出或事件 JSONL。

## 产品需求

- 搜索覆盖 Task 标题、Run ID、Task ID、Workflow ID 和 runtime session/thread ID。
- 状态支持全部、运行中、等待介入、已完成、失败、已终止。
- 默认按更新时间倒序；同时间使用 Run ID 稳定排序。
- 首页 50 条，继续滚动增量加载；UI 不一次渲染全部已加载行。
- 筛选后没有匹配项时显示明确空状态，右侧不保留旧详情。
- 搜索和清空入口使用 `/`、`[ x ]` 等纯文本 TUI 标记，不新增图标按钮。

## 技术设计

- 后端：workflows repo 新增 `runs/index.json`、`RunListQuery/RunPage`；localapp 新增 `ListRunsInput/RunListPage`。索引不是权威数据，损坏时从 `run.json` 重建。
- 桌面端 / Wails：TaskRunBinding 新增 `ListRuns`，保留旧 `ListRunsByTask` 兼容内部流程。
- 前端：以分页 items 替换 Task -> Runs N+1 状态；虚拟列表按固定 row height 计算可见窗口。
- 数据模型：不修改 Run 文件；cursor 为 opaque string。
- API/binding：`ListRuns(input) -> {items,nextCursor,total}`。
- 错误码：`run_query_invalid_cursor`、`run_query_invalid_status`。

## 验收标准

- [x] repo 查询正确处理 Workspace Task 范围、状态、keyword、稳定 cursor 和 total。
- [x] 前端不再为每个 Task 单独调用 `ListRunsByTask`。
- [x] 搜索和状态筛选会重置分页，并清空不匹配的 Inspector。
- [x] 滚动可加载下一页且不会重复已有 Run。
- [x] 窗口化列表只渲染可见区域附近的行。
- [x] 运行中 Run 的轮询仍会刷新当前已加载窗口。

## 测试计划

- Repo：排序、cursor、keyword、status、session ID 与非法 cursor。
- App：Workspace 范围与 Task 标题聚合。
- Frontend：筛选参数、去重和虚拟窗口计算。
- Build：Go test/race/vet、frontend test/build、Wails build。

## 交付记录

- `runs/index.json` 保存可派生摘要；缺失、损坏或目录集合变化会自动重建，Run 状态写入不依赖索引成功。
- `ListRuns` 按 Workspace Task 范围、状态、Task 标题、Run/Task/Workflow ID 和 session/thread ID 查询，默认 50、最大 200，并返回 opaque cursor 与 total。
- 工作台移除 Task -> `ListRunsByTask` 的 N+1 调用，改为 Workspace 级 page DTO；现有 binding 保留给兼容调用。
- 列表按今天/昨天/更早分组；固定高度窗口化在 1000 个 Run 的单元测试中只保留视口附近少量行。
- 千条记录测试曾发现同组标题被重复插入并放大虚拟总高度；改为显式记录 lastGroup 后，同一日期只生成一个标题。
- Browser 验证搜索无结果时列表和 Inspector 同时清空；Go test/race/vet、frontend test/build 与 Wails production build 通过。
