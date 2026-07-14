# Issue 01-016: Run 单击恢复反馈

状态：已完成

## 来源

- PRD：`design/prd/01-custom-agent-workflows.md`
- 技术方案：`design/docs/01-custom-agent-workflows.md`

## 目标

暂停 Run 的“恢复运行”在一次点击后立即给出已受理反馈，并在后台状态真正推进前阻止重复提交，消除需要点击两次才像是生效的体验。

## 范围

- 恢复请求提交期间提供“恢复中”状态。
- 后端异步接收恢复请求后，前端保持 pending 状态并按 active Run 频率刷新。
- Run 尚在停止时禁用恢复和终止操作。
- 阻止同一个 Inspector 在 React 重绘前重复提交运行控制请求。

## 非目标

- 不改变 Orchestrator 的异步执行和持久化时序。
- 不增加新的 Wails binding 或后端状态。
- 不改变恢复后再次进入 `$pause` 的业务语义。

## 产品需求

- 用户单击“恢复运行”后，按钮立即变为“恢复中”且不可再次点击。
- 恢复请求已受理但 Run 快照仍为 paused 时，不重新显示可点击的“恢复运行”。
- Run 进入 running、结束、再次暂停或异步失败后，界面以最新持久化状态为准。
- 用户在请求完成前切换 Run 或 Workspace 时，旧 Run 的延迟刷新不能覆盖当前 Inspector。

## 技术设计

- 后端改动：无；继续由 application service 先登记 active run，再异步执行 `ResumeRun`。
- 桌面端 / Wails 改动：无 binding 变化。
- 前端改动：记录当前 pending resume Run ID；恢复请求返回后把匹配的详情乐观标记为 active，沿用 active 轮询确认持久化状态；使用同步 ref 屏蔽双击；延迟刷新只允许更新仍被选中的 Run。
- 数据模型改动：无。
- API 或 binding 改动：无。
- 错误码：沿用现有 `run_invalid_state` 等错误，失败时清理 pending 并展示通知。

## 验收标准

- [x] 单击一次即可进入“恢复中”，无需第二次点击。
- [x] pending 或尚在停止的 Run 不会发送第二个恢复请求。
- [x] 后端完成状态推进后，Inspector 自动切换到最新状态。
- [x] 切换 Run/Workspace 后，旧请求不会覆盖右侧详情。
- [x] 前端单元测试和 production build 通过。

## 测试计划

- 单元测试覆盖恢复已受理、等待停止和非当前 Run 的乐观更新。
- production build 检查 React hook、组件属性和打包结果。
- 在 Wails 本地应用中输入补充指令，单击一次“恢复运行”，确认按钮立即反馈且 Run 自动推进。

## 交付记录

- 恢复请求使用同步 ref 防重；Wails binding 返回后把当前 Run 乐观标记 active，并由既有 900ms active 轮询收敛到持久化状态。
- 恢复 pending 与“中断尚未完全停止”分开显示为“恢复中”和“等待停止”，两个阶段都禁用重复操作。
- 延迟刷新在执行前检查当前 selected Run ID，避免用户已切换 Run 或 Workspace 后回写旧 Inspector。
- 验证通过：前端 22 项单元测试、production build、`go test ./...`、`wails3 build` 与浏览器单击冒烟；Wails 仅保留既有 macOS deployment target linker warning。
