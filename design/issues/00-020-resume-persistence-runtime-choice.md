# Issue 00-020: 续跑、RunLog 持久化、直选 runtime + 工作目录

状态：已完成

## 来源

在 00-019 真实本地执行的基础上，补齐三项增强：任务级 resume（多轮续跑）、RunLog 持久化（重启可见）、用户直选 runtime 并指定工作目录（而非仅 persona 预设）。

## 范围

### 1. RunLog 持久化
- `execution/runstore.go`：每个订单的 RunLog 在内存缓存并镜像到磁盘 `<workspaceRoot>/<orderID>/.oneshot-run.json`（原子写）。逐条事件只留内存，终态/会话变更时落盘。
- `RunLog` 增加 `SessionID`、`Turns`。`Snapshot` 在内存缺失时从磁盘水合 → 服务重启后历史与可续会话仍可见。

### 2. 任务级 resume（多轮续跑）
- 引擎 `Request.ResumeSessionID`；codex 用 `exec resume <id>`（注意 resume 仅接受 `--json/--skip-git-repo-check/-m/-c`，sandbox 经 `-c sandbox_mode=...`，cwd 经进程 cwd），claude 用 `-p --resume <id>`。
- `orders.Continue(userID, orderID, prompt)`：对 delivered/失败的订单，取上轮 `SessionID`，把订单 Requirement 换成后续指令、置 `ResumeSessionID`、状态回 running（续跑不额外扣次）。新增 `POST /api/orders/{id}/continue`。
- worker 检测 `order.ResumeSessionID`：复用同一工作目录、附带 resume，事件追加到原 RunLog（`Turns++`），成功后清除 `ResumeSessionID`。
- `artifacts.RecordRunOutput` 改为按文件名去重：多轮只登记新增交付物，不重复。

### 3. 直选 runtime + 工作目录
- 目录新增两个通用开发 agent：`codex`（Codex 工程师）、`claude-code`（Claude Code 工程师），无 persona 约束。
- 订单/会话支持可选 `workspace`：`CreateInput.Workspace`、会话 `Start(..., workspace)`、confirm 时透传。worker 在该目录直接工作；对外部目录只登记 `SUMMARY.md`（写在受管理 metaDir），绝不扫描用户整个项目。
- 桌面：`StartConversation(agentID, workspace)`、`ContinueOrder(orderID, prompt)`；前端为开发 agent 显示「工作目录」输入、运行时标识，订单完成后在对话框输入即「续跑」。

## 验收标准

- [x] RunLog 落盘并在新进程 `Snapshot` 水合（含 SessionID）。
- [x] delivered/失败订单可 `continue`，复用会话与工作目录，多轮事件累积、仅登记新交付物。
- [x] 无可续会话时 `continue` 被拒。
- [x] 指定外部工作目录时 agent 在其中工作、用户原文件不被采集，仅 SUMMARY 入库。
- [x] `go test ./... -race`、vet、gofmt、前端 build 全过。

## 验证记录

- 单测：runstore 跨 worker 持久化、worker 多轮 resume（引擎收到 session id、Turns=2、新增 turn2 文件）、orders.Continue（成功/无会话拒绝）、artifacts 多轮新增 + 外部目录仅 SUMMARY。
- 实机（codex）：
  - 续跑：turn1 建 step1.txt → delivered；continue 后 turn2 复用会话建 step2.txt 且保留 step1.txt，事件 6→12，交付物增量为 step2.txt。
  - 自定义目录：在 /tmp/myproj 建 NOTES.md，用户原有 main.go/README.md 未被采集，仅 SUMMARY.md 入库。
