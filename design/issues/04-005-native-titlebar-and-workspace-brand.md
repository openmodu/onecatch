# Issue 04-005：原生标题栏与个人工作台品牌

状态：已完成

## 来源

- PRD：`design/prd/04-mac-desktop-ui-redesign.md`
- 技术方案：`design/docs/04-mac-ui-component-system.md`

## 目标

让 macOS 窗口交通灯回到普通原生应用的标准位置与外观，并让品牌副标题准确表达 Oneshot 的个人工作台定位。

## 范围

- 将 Wails macOS 标题栏切换为接近 Codex 的原生 unified toolbar 模式。
- 将前端标题栏高度调整为 52px，并使用 Wails CSS 拖拽区域取代顶部隐形覆盖层。
- 双击标题栏通过 macOS 原生手势调用 `NSWindow zoom:`。
- 禁止标题文字选择并移除自定义底部分隔线。
- 保留全尺寸内容、自定义拖拽区和系统原生窗口控制行为。
- 品牌副标题从 `local agents` 改为 `personal workspace`。

## 非目标

- 不使用 CSS 或图片仿制交通灯。
- 不接管关闭、最小化、缩放和全屏行为。
- 不改变应用名称、图标或业务导航。

## 产品需求

- 交通灯使用 macOS 系统原生尺寸、颜色、交互和标准窗口边距。
- 交通灯在 unified toolbar 内上下居中，顶部与底部保留接近 Codex 的舒展留白。
- 标题栏继续支持拖动窗口，不遮挡左侧品牌内容。
- 品牌区域明确显示 `// personal workspace`，与个人任务和 Agent 编排工作台定位一致。

## 技术设计

- 后端改动：无业务后端改动。
- 桌面端 / Wails 改动：使用 `application.MacTitleBarHiddenInsetUnified`，不设置 `InvisibleTitleBarHeight`。
- 前端改动：更新品牌副标题文案；`--ui-titlebar-height` 设为 52px；标题栏使用 `--wails-draggable: drag`，文字和占位节点不参与指针命中。
- macOS 原生改动：窗口顶部 80pt 区域使用 `NSClickGestureRecognizer` 识别双击并调用 `NSWindow zoom:`。
- 数据模型改动：无。
- API 或 binding 改动：无。
- 错误码：无。

## 验收标准

- [x] macOS 窗口继续使用系统原生交通灯。
- [x] 原生交通灯在 52px unified toolbar 中垂直居中且上下留白对称。
- [x] 双击标题栏可以在窗口放大与还原之间切换。
- [x] 连续点击标题栏不会选中 `Oneshot`，缩放过程中不显示灰白底部分隔线。
- [x] 关闭、最小化、缩放和窗口拖动继续由系统处理。
- [x] 品牌副标题显示为 `// personal workspace`。
- [x] 前端和 Go 构建通过。

## 测试计划

- Go：执行桌面端相关测试与编译。
- Frontend：执行单元测试和 production build。
- 手动：重启 Wails dev，检查原生交通灯位置、hover 图标、窗口拖动及三种窗口操作。

## 交付记录

- 使用 Wails unified 隐藏标题栏模式保留原生窗口控制，不引入前端模拟按钮。
- 品牌副标题调整为个人工作台语义。
- 根据 Codex 参考进一步将紧凑 28px 标题栏调整为 52px unified toolbar，并将拖动改为 Wails CSS 区域；双击标题栏由 macOS 原生手势完成放大/还原。
- 标题栏子节点不再接收指针和文字选择，WebKit 连续点击不会高亮标题；移除自定义底部边框并让 titlebar/app shell 共用 canvas 背景，缩放动画不再出现灰白分隔横带。
- 验证：前端 14 项单元测试、production build、桌面 Go tests 与 Wails DEV build 均通过；Wails build 仅输出既有 macOS link target 警告。
- 原生窗口实测：52px unified toolbar 中交通灯上下留白对称；双击后窗口由 1279×799 放大为 2560×1357，再次双击恢复至 1279×799。
