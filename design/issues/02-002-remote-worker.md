# Issue 02-002: 远端 Worker 与多机派发

状态：已完成

## 来源

- PRD：`design/prd/02-parallel-dag-and-cluster.md`
- 技术方案：`design/docs/02-parallel-dag-and-cluster.md`

## 目标

让 DAG 节点可以按 worker ID 在受信任 LAN/VPN 的另一台机器执行。

## 范围

- Worker registry 纯文件持久化与脱敏 DTO。
- Bearer token health/execute 协议。
- `oneshot-worker` headless 入口、Workspace 映射和 agentrun 调用。
- Coordinator 远端 executor 与 Wails bindings。

## 非目标

- 文件同步、TLS 终止、自动合并。

## 验收标准

- [x] 注册、健康检查、更新和删除 worker。
- [x] 远端 stub worker 能执行节点并返回 events/result。
- [x] token 不通过读接口暴露；未授权请求返回 401。

## 测试计划

- `httptest` 协议测试、registry 权限测试、超时和错误映射。

## 交付记录

- 新增 `internal/worker` registry、HTTP client/server 和脱敏 DTO；`workers.json` 原子写入且权限 `0600`。
- 新增 `cmd/oneshot-worker`，支持 token env、重复 Workspace ID 映射、Codex/Claude binary override 和安全 HTTP timeouts。
- Worker 使用 Bearer token 常量时间比较；Workspace 内 read-only 请求共享锁，写请求独占锁。
- Coordinator 按节点 workerId 派发，远端 events/result 回写现有 StepRun event stream；不可用/未映射/runtime 缺失暂停而不回退。
- 因首版不做文件同步，远端 DAG 节点强制 read-only；本机 join 节点负责汇总落地。
- registry/鉴权/health/execute/unmapped 测试和远端 scheduler dispatch 测试通过。
