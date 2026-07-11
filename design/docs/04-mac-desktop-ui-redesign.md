# 04 Mac 桌面端统一 UI 技术方案

## 1. 设计来源

视觉真值为 `design/Mac应用UI设计优化/Oneshot.dc.html`，颜色和排版 token 来源于同目录 `mirage-tokens.css`。`recreation/` 与 `uploads/` 仅用于理解旧页面和改版意图，不作为最终实现代码直接复制。

## 2. 前端结构

```text
AppShell
├── MacTitlebar
├── Sidebar
│   ├── Brand
│   ├── Workspace/CWD
│   ├── PrimaryNav
│   └── RuntimeFooter
└── Main
    ├── CommandStrip
    └── ActiveSurface
        ├── Tasks + RunInspector
        ├── WorkflowLibrary
        ├── SerialWorkflowEditor
        ├── DAGWorkflowEditor
        └── SettingsPage
```

`editor` state仍保存 WorkflowDefinition 草稿，但编辑器直接渲染为 ActiveSurface，不再通过全屏 modal 包裹。Workspace/Worker 新增和危险确认继续使用 modal。

## 3. Token

- 字体：`SFMono-Regular`、`SF Mono`、`Cascadia Mono`、`Menlo`、`Consolas` 等宽 fallback。
- Light：canvas `#F5F5F0`、ink/panel `#EBEBE6`、line `rgba(115,110,94,.36)`、text `#1A1A1A`。
- Dark：canvas `#1C1C1C`、ink/panel `#252525`、line `rgba(255,255,255,.16)`、text `#EFEFEF`。
- 语义色：accent、good、warn、danger、cyan，分别用于操作、成功、暂停/警告、危险、DAG/信号。
- 形状：主页面 0–2px radius；仅 local/status 使用 pill；遮罩 modal 可保留 4px 小圆角。

## 4. API 与 Binding

不新增或修改 API/Wails binding。以下接口继续原样使用：

- RuntimeBinding：Runtime 列表与检测。
- WorkspaceBinding：选择和加入工作目录。
- TaskRunBinding：Task/Run 创建、预览、启动、查询、暂停、恢复、终止。
- WorkflowBinding：定义列表、校验、创建和更新。
- WorkerBinding：Worker 列表、健康检查、保存和删除。
- SettingsBinding：五分区读取、保存、重置、Runtime 检测、存储统计、清理和诊断导出。

错误码和 revision/CAS 处理不变，仍由现有通知、inline banner 和确认框呈现。

## 5. 交互边界

- 导航：切换 Task/Workflow/Settings 时关闭 editor 草稿；若后续需要 dirty guard，单独增强。
- Run：保留轮询、运行状态、暂停说明、补充指令、恢复和终止。
- Workflow：行点击进入编辑器；创建按钮使用现有模板。
- DAG：保留 pointer drag、连接端口和节点 Inspector；只改变画布与节点样式。
- Settings：保留 section draft、dirty guard、save/reset、冲突和危险确认。

## 6. 响应式

- 基准 viewport：1328×848（原型 1280×800 加 24px 外边距）。
- Wails 实际窗口内 AppShell 填满可用区域，不显示原型外层灰色背景。
- 宽度低于 1100px 时 rail 缩至 190px，Inspector 缩至 340px。
- 宽度低于 860px 时主内容允许横向最小宽度并由外层滚动，确保按钮和 Inspector 不重叠。

## 7. 验证计划

- 使用 in-app browser 在 1360×860 捕获视觉原型和实现的 Tasks、Workflows、serial editor、DAG editor、Settings。
- 对每个页面生成同画布对照图；先修复 P0/P1/P2，再记录 P3。
- 检查主导航、创建 Run、打开两个编辑器、设置分区切换与 dirty/save、确认框 Escape。
- 检查 console error、production build、Wails build、Go test/race/vet。
