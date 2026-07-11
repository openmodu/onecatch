# Issue 03-002: Runtime、执行策略与安全授权

状态：已完成

## 来源

- PRD：`design/prd/03-settings-center.md`
- 技术方案：`design/docs/03-settings-center.md`
- 前置：`design/issues/03-001-settings-model-and-storage.md`

## 目标

让 Runtime 和 scheduler 消费统一设置，并在 Definition/Run 边界落实安全授权。

## 范围

- Runtime binary/default model/env allowlist reload 和 draft check。
- 全局 policy defaults、DAG concurrency semaphore、interrupt grace。
- Step/Workflow/global precedence和 resolved Run snapshot。
- Full sandbox 在保存、启动、恢复时的授权与确认。

## 非目标

- Keychain secret 输入、任意 CLI flags。

## 验收标准

- [x] Runtime 草稿检查不执行 Agent。
- [x] model、policy 和 sandbox precedence 与技术方案一致。
- [x] DAG 并发上限生效且不改变 join。
- [x] 禁止 Full sandbox 时旧定义也不能启动或恢复。
- [x] 环境变量值不写入文件和日志。

## 测试计划

- fake binaries、env fixtures、scheduler concurrency、security boundary 和 snapshot 回归。
- 实际结果：覆盖 DAG 1/2 并发与 join、resolved Run snapshot、Full confirmation/downgrade resume、诊断环境值审计；全量 race 通过。

## 交付记录

- Runtime registry 支持 binary/default model/env key allowlist 热更新；草稿检查只执行带 5 秒超时的 `--version`。
- 新 Workflow 使用全局 policy/default sandbox；新 Run 冻结 resolved model、sandbox、环境 key、DAG 并发与中断宽限，恢复不读取新设置覆写快照。
- DAG scheduler 使用快照并发上限分批执行，保留 readiness/join 语义；Agent 取消先发送 interrupt，超时后由进程管理器终止。
- Full access 在 Definition save、Run preview/start、resume 边界校验；短时 token 绑定 Task、Workspace、Workflow 更新时间并单次消费。
- 验证：并发上限/join、Run 快照、环境值不落盘、Full 降权恢复边界及全量 race 通过。
