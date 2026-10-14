# Keel 桌面代码导读

这个目录承担桌面界面和应用装配。业务流程从 `service/desktop` 进入，工作流调度在 `usecase/workflows`，Agent 适配在 `usecase/agentrun`。完整分层见 [internal 导读](../../README.md)。

## 已独立的组件

| 目录 | 内容 | 与页面的连接方式 |
| --- | --- | --- |
| `assets` | 通用 SVG、运行时图标、解析、缓存和栅格化 | 页面调用图标绘制函数 |
| `terminalview` | 终端模拟器画布、输入法、键鼠输入、颜色、选区和搜索 | 页面注入写入、尺寸变化等回调 |
| `filepicker` | 系统文件选择器 | 页面传入选择请求，读取结果 |

`assets` 和 `terminalview` 不依赖桌面 app、service 或 repo，测试随组件放置。终端进程的启动、远程连接和 PTY 管理仍在 `service/terminal`，没有搬进 UI 组件。

## 根目录按功能阅读

| 文件组 | 职责 |
| --- | --- |
| `backend.go` | 组合根，创建并连接后端对象 |
| `app.go`、`run.go`、`tray.go` | 应用状态、窗口和生命周期 |
| `views.go`、`navigation.go`、`sidebar*.go` | 页面骨架、导航和侧栏 |
| `composer*.go`、`attachments*.go` | 输入框、补全和附件交互 |
| `transcript.go`、`conversation*.go` | 运行事件的显示模型、对话渲染和交互 |
| `workflow*.go` | 工作流编辑、DAG 绘制和运行操作 |
| `terminal.go`、`terminal_views.go`、`terminal_layout.go` | 终端标签页、分屏、会话连接；画布实现位于 `terminalview` |
| `settings*.go`、`skills*.go`、`runtime_profile.go` | 设置、Skills 和运行环境检查界面 |
| `workbench.go`、`files_panel.go`、`git_review.go` | 文件编辑、目录和 Git 审查面板 |
| `task*.go`、`session*.go`、`workers.go` | 任务、会话和 Worker 的界面操作 |

同前缀的多个文件属于同一个功能，不是多个架构层。`*_test.go` 与实现放在一起，便于定位回归覆盖；阅读目录时可以先隐藏测试和资源文件。

## 继续拆分的边界

根包仍然较大，多个页面共享 `app` 状态。后续拆页面时，应把该页面拥有的状态、视图和事件处理一起迁移，并通过小接口或回调连接应用。只把文件挪到子目录会跨越 Go 包边界，迫使大量内部状态公开，反而增加耦合。

后端业务规则应在 service/usecase 中独立执行；页面保留输入草稿、选区、滚动、展开状态和过期回调判断。新增业务行为不要直接放进按钮回调，也不要从页面读写业务存储文件。

修改对话渲染或后台刷新前，阅读 [对话选区与后台更新](../../../docs/keel-conversation-selection.md)；修改窗口和托盘前，阅读 [托盘与窗口生命周期](../../../docs/keel-tray.md)。
