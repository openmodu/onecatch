# Issue 02-003: DAG 拖拽画布与运行监控

状态：已完成

## 来源

- PRD：`design/prd/02-parallel-dag-and-cluster.md`
- 技术方案：`design/docs/02-parallel-dag-and-cluster.md`
- 前置：`design/issues/02-001-dag-domain-and-scheduler.md`、`design/issues/02-002-remote-worker.md`

## 目标

提供无需重型依赖的节点画布，并展示 DAG 并发运行状态。

## 范围

- 节点拖动、SVG 依赖边、连接/删除和自动布局。
- 节点 inspector 编辑 runtime、worker、sandbox、prompt 和 signals。
- DAG validation issues、保存恢复与运行态覆盖。
- Worker 管理面板和健康状态。

## 非目标

- 无限画布协作、多人光标、复杂条件编辑器。

## 验收标准

- [x] 拖动和边编辑后可保存并恢复。
- [x] 画布实时表达 ready/running/completed/paused/failed。
- [x] 用户可以选择 local 或远端 worker。

## 测试计划

- 前端 build、交互冒烟、视觉检查和 Wails build。

## 交付记录

- 新增无第三方图依赖的 HTML/SVG DAG 画布：pointer drag 节点、贝塞尔依赖边、端口两步连接、点击边删除和自动分层布局。
- 节点 inspector 支持 ID/名称/runtime/worker/sandbox/role/instruction/terminal signals；坐标写入 Workflow layout。
- 新增多机 Worker 页面：注册、编辑、删除、健康检查、runtime 状态和 headless 启动命令。
- Run inspector 对 DAG nodes 展示 pending/running/completed/paused/failed 和 worker。
- 浏览器实测新增节点、创建依赖边、节点拖动、Worker 健康检查均成功；frontend production/dev build 与 Wails build 通过。
