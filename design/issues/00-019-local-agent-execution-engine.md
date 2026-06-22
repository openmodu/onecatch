# Issue 00-019: 本地 Agent 执行引擎（长程任务真实执行）

状态：已完成

## 来源

产品方向：把工作台从"模拟交付"升级为真正驱动本地 agent CLI（Codex、Claude Code）执行长程任务。此前 worker 为占位实现（`execution` 仅把 running → delivered 并生成假 PDF），agent 回复是模板，没有任何真实执行。

## 目标

订单进入 `running` 后，由 worker 在每个订单独立的工作目录中真实启动本地 agent（codex / claude），把交付物落成真实文件，并把 agent 的执行过程实时流式呈现到桌面会话里。

## 范围

- 新增 `internal/agentrun` 引擎：
  - `Runner` 抽象 + 归一化 `Event` 流（started/reasoning/message/tool_use/tool_result/file_change/usage/result/error）。
  - `CodexRunner`：`codex exec --json --skip-git-repo-check --sandbox <mode> -C <ws>`，解析 JSONL（thread.started / item.completed / turn.completed）。
  - `ClaudeRunner`：`claude -p --output-format stream-json --verbose`，解析 system/assistant/user/result。
  - `Engine` 注册表：按 runtime 选择 runner，检测可用性（`LookPath`），不可用时回退到任意已安装 runtime。
- 改造 `internal/usecase/execution`：
  - 每个 running 订单派发到独立 goroutine（in-flight 去重，避免轮询重复启动），在 `<workspaceRoot>/<orderID>` 中执行。
  - 流式事件写入内存 `RunLog`；成功后 running → delivering，收集工作目录文件为交付物，再 delivering → delivered；失败/无可用 runtime → failed 并附原因。
- 改造 `internal/usecase/artifacts`：
  - `RecordWorkspaceOutput` 把工作目录里的真实文件登记为交付物（`StorageURI` 指向磁盘文件，跳过 dotfile / node_modules 等），并始终写入 `SUMMARY.md`（agent 收尾总结）。
  - `Download` 直接读取真实文件字节，按扩展名返回 MIME。
- 接口：新增 `GET /api/orders/{id}/run`（按订单归属鉴权后返回实时 RunLog，过滤原始 JSONL/隐私字段）。
- Agent 目录：`domain/agents.Agent` 新增 `Runtime/Sandbox/Model/SystemPrompt`，四个 persona 各绑定真实 runtime 与角色提示词。
- 配置：`workspace_root` / `codex_binary` / `claude_binary`（含 `ONESHOT_WORKSPACE_ROOT` 等环境变量）。
- 桌面：SDK + binding 新增 `GetOrderRun`，前端在订单执行中轮询并在会话内渲染"本地 Agent 执行"流式卡片与最终总结；交付物使用真实文件类型。

## 非目标

- 不实现会话 resume / 多轮续跑（runtime 已返回 session id，预留）。
- RunLog 暂为进程内内存态，不跨重启持久化。
- 计费/订单状态机不变。

## 验收标准

- [x] 订单 running 后真实启动本地 agent，在独立工作目录产出真实文件。
- [x] 执行事件可经 `GET /api/orders/{id}/run` 实时获取，按用户鉴权、跨用户 404。
- [x] 交付物为真实文件，下载返回真实字节与正确 MIME。
- [x] 无可用 runtime 时订单 failed 并给出明确原因。
- [x] codex 与 claude 两个 runtime 均通过实机冒烟（产出文件 + 解析最终消息）。
- [x] `go test ./...`、`-race`、`go vet`、前端 `vite build` 全过。

## 验证记录

- 单测：`internal/agentrun`（stub 脚本喂真实 JSONL 格式，覆盖解析/失败/路由/可用性）；execution worker（交付 + 失败 + 无 runtime + 不重复派发）；artifacts（真实文件收集/幂等/磁盘下载）；transport（/run 鉴权与形状）。
- 实机：`ONESHOT_LIVE=1` 跑通 codex 与 claude 各自产出 `hello.txt`。
- 端到端：本地启动 oneshot-server，登录 → 建单（research-analyst/codex）→ 轮询 /run 看到 started/message/file_change/usage → delivered → 列出 `result.md` + `SUMMARY.md` → 下载真实内容。
