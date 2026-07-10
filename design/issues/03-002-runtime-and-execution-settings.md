# Issue 03-002: Runtime、执行策略与安全授权

状态：待开发

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

- [ ] Runtime 草稿检查不执行 Agent。
- [ ] model、policy 和 sandbox precedence 与技术方案一致。
- [ ] DAG 并发上限生效且不改变 join。
- [ ] 禁止 Full sandbox 时旧定义也不能启动或恢复。
- [ ] 环境变量值不写入文件和日志。

## 测试计划

- fake binaries、env fixtures、scheduler concurrency、security boundary 和 snapshot 回归。

## 交付记录

- 待开发。
