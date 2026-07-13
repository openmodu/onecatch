# Issue 01-015: Modu ACP 工具授权

状态：已完成

## 来源

- PRD：`design/prd/01-custom-agent-workflows.md`
- 技术方案：`design/docs/01-custom-agent-workflows.md`

## 目标

支持 `modu_code --acp` 的 `session/request_permission` 反向请求，使 Agent 在需要工具审批时能够按 Workflow Sandbox 继续或拒绝，而不是直接中断 Session。

## 范围

- 解析 ACP permission 的 toolCall 与 options。
- workspace-write/full 启动时追加 `--no-approve`，read-only 不追加。
- 按 read-only / workspace-write / full 选择单次授权结果。
- 将授权请求和决定保存为相邻 Runtime 工具事件。
- 保持未知反向方法的显式协议错误。

## 非目标

- 不实现弹窗等待用户逐次批准。
- 不缓存 `allow_always` 或 `reject_always`。
- 不把 ACP 工具审批声明为操作系统级目录隔离。

## 产品需求

- read-only Run 遇到需要审批的工具时返回 `reject_once`。
- workspace-write 和 full Run 使用 `--no-approve`；兼容版本仍请求授权时返回 `allow_once`。
- 工作台能够按发生顺序展示该工具请求与授权结果。
- 非 permission 的未知反向请求仍应失败，不能静默允许。

## 技术设计

- 后端改动：`ModuRunner` 按 Sandbox 构造 `--acp [--no-approve]`，并为 `readACPResponse` 注入 reverse handler，通过既有 stdin 回写同 ID JSON-RPC response。
- 桌面端 / Wails 改动：无 binding 变化，复用 Runtime event timeline。
- 前端改动：无，复用现有 Tool Use / Tool Result 渲染。
- 数据模型改动：无。
- API 或 binding 改动：无。
- 错误码：协议解析、未知 method 和缺少匹配 option 返回明确 Modu ACP 错误。

## 验收标准

- [x] workspace-write/full 追加 `--no-approve`，read-only 保持审批模式。
- [x] 兼容场景下 workspace-write/full 可完成包含 permission reverse request 的 ACP prompt。
- [x] read-only 返回拒绝选项，不误授予风险工具。
- [x] permission 请求与决定各产生一个 Runtime event。
- [x] 未知反向 method 仍明确失败。

## 测试计划

- 单元测试覆盖三种 Sandbox 的启动参数、option 选择、事件和未知 method。
- Stub ACP 子进程覆盖 reverse request → response → 原 prompt 完成的完整时序。
- 回归执行 `go test ./...`、`npm test`、`npm run build`、`wails3 build` 与 `git diff --check`。

## 交付记录

- `run_11436148cbc55ece` 连续四次失败于 `session/request_permission`，随后因连续失败上限暂停；修复后可从当前步骤恢复，Modu 会创建新的短生命周期 ACP session。
- 权限响应使用 `allow_once/reject_once`，不在不同 Run 或工具参数之间复用授权决定。
- 当前 `modu_code` 的可写 Run 直接使用 `--no-approve`；reverse permission handler 保留给 read-only 和兼容版本。
- 验证通过：`go test ./...`、`go vet ./...`、关键包 race、前端 18 项测试、production build、`wails3 build` 和 `git diff --check`；Wails 仅保留既有 macOS deployment target linker warning。
