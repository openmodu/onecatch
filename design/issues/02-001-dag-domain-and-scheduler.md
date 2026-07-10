# Issue 02-001: DAG 领域模型与并行调度器

状态：已完成

## 来源

- PRD：`design/prd/02-parallel-dag-and-cluster.md`
- 技术方案：`design/docs/02-parallel-dag-and-cluster.md`

## 目标

在不破坏 serial loop 的前提下，增加无环依赖图、并行 ready 节点和 all-dependencies join。

## 范围

- Definition/Step/Run 增加 DAG 字段并保持旧 JSON 兼容。
- DAG 校验、并行写冲突校验和节点状态机。
- 并行执行、结果汇聚、暂停/恢复和重启恢复。

## 非目标

- 条件边、any-of join、DAG 环。

## 验收标准

- [x] fan-out 两节点并行，join 等待全部依赖。
- [x] 非法 DAG 和并行写冲突拒绝保存。
- [x] serial 回归测试不变。

## 测试计划

- 领域表驱动测试、并发 stub engine 集成测试和 race。

## 交付记录

- Definition/Step/Run 增加兼容字段 `mode/layout/dependsOn/workerId/nodes`，旧 JSON 默认 serial/local。
- Kahn 拓扑校验拒绝环、未知/重复/自依赖；传递可达性检查拒绝潜在并行写冲突。
- DAG scheduler 用中心事件循环串行更新 Run revision，并发 goroutine 只执行节点；ready roots 并行，join prompt 注入直接依赖 outcomes。
- 完成节点在 resume 时保留，running/paused/failed 节点重置后重试；重启恢复会把 running node 和 Run 落为 paused。
- 并发测试观察到两个 root 最大并发度为 2，join 在 root 完成后执行；目标包 race 与 serial 全回归通过。
