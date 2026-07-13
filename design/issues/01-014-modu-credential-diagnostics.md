# Issue 01-014: Modu Code 默认配置与错误诊断

状态：已完成

## 来源

- PRD：`design/prd/01-custom-agent-workflows.md`
- 技术方案：`design/docs/01-custom-agent-workflows.md`

## 目标

让 Oneshot 默认复用终端中的 Modu Code 配置，并避免把 ACP 的 `unexpected EOF` 当成主要错误。

## 范围

- 默认启动当前 `modu_code --acp` 并读取 `~/.modu/config.toml`。
- 当前命令不存在时兼容旧 `modu-code`。
- ACP 子进程提前退出时优先展示退出状态和 stderr。

## 非目标

- 不在 `~/.oneshot/` 保存 API Key 值。
- 不接入 macOS Keychain 或第三方凭据管理器。
- 不改变已启动 Run 的 Runtime 设置快照。

## 产品需求

- binary 留空时无需在 Oneshot 重复填写终端已经配置的 Provider、模型或 Key。
- 用户手动填写旧 `modu-code` 路径时保持兼容。
- 纯 binary 检测仍不发起模型请求。

## 技术设计

- 后端改动：Modu runner 默认解析 `modu_code`、传入 `--acp`，并回退旧 `modu-code`。
- 桌面端 / Wails 改动：无 binding 变化。
- 前端改动：Modu Runtime 卡片说明默认复用 `~/.modu/config.toml`。
- 数据模型改动：无。
- API 或 binding 改动：无。
- 错误码：无新增；保留 Runtime 退出错误中的 stderr。

## 验收标准

- [x] 未配置 binary 和白名单时优先使用 `modu_code --acp` 及用户现有 Modu 配置。
- [x] 当前命令缺失时回退旧 `modu-code`。
- [x] ACP 子进程提前失败时不再以 `unexpected EOF` 开头。

## 测试计划

- Go 单元测试覆盖默认命令选择、`--acp` 参数和 ACP 提前退出。
- 前端运行测试与 production build。
- 回归执行 `go test ./...`、`npm test`、`npm run build`、`git diff --check`。

## 交付记录

- 默认命令从旧 `modu-code` 切换为终端实际使用的 `modu_code --acp`；不会复制或导出 `~/.modu/config.toml` 的凭据。
- 用户显式配置 binary 时始终优先于默认命令探测。
