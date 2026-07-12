# Issue 04-006：macOS 原生窗口圆角

状态：已完成

## 来源

- PRD：`design/prd/04-mac-desktop-ui-redesign.md`
- 技术方案：`design/docs/04-mac-ui-component-system.md`
- 视觉参照：Codex 等现代 macOS 应用窗口。

## 目标

增大 Oneshot 普通窗口的原生圆角，使窗口轮廓更接近现代 macOS 应用，同时保持标题栏、系统阴影、放大和全屏行为正确。

## 范围

- 在 macOS 原生窗口 frame 上应用 26pt continuous corner。
- 放大和全屏时取消圆角。
- 退出放大或全屏后恢复圆角。
- 非 macOS 或禁用 cgo 的构建保持原行为。

## 非目标

- 不给 Web 根节点添加模拟窗口圆角。
- 不启用 Liquid Glass 或改变窗口背景材质。
- 不自绘窗口阴影和交通灯。

## 产品需求

- 普通窗口四角使用比 Wails 默认值更大的连续圆角。
- 圆角裁切必须包含标题栏和页面内容，不能只裁 Web 背景。
- 最大化和全屏不得在屏幕边缘留下圆角缺口。
- 窗口恢复后圆角和系统阴影同步恢复。

## 技术设计

- 后端改动：无业务后端改动。
- 桌面端 / Wails 改动：macOS cgo helper 设置 NSWindow theme frame layer 的 `cornerRadius`、`cornerCurve` 和 `masksToBounds`；监听 Wails 窗口状态事件切换半径。
- 前端改动：无。
- 数据模型改动：无。
- API 或 binding 改动：无。
- 错误码：无。

## 验收标准

- [x] 普通 macOS 窗口使用 26pt continuous corner。
- [x] 标题栏和内容共享同一原生裁切轮廓。
- [x] 放大、还原、全屏状态切换时圆角符合对应窗口状态。
- [x] 系统阴影和交通灯继续正常显示。
- [x] Go tests、前端 build 和 Wails DEV build 通过。

## 测试计划

- Build：执行前端测试/build、桌面 Go tests 与 Wails DEV build。
- 原生窗口：捕获普通窗口左上角，检查 26pt 连续圆角与阴影。
- 交互：双击标题栏放大和还原，检查最大化边缘与恢复后的圆角。

## 交付记录

- 新增 darwin+cgo 原生窗口圆角 helper 和其他平台空实现。
- 在 runtime ready 后应用 26pt continuous corner，并随 maximise/fullscreen 事件切换为 0/26pt。
- 原生窗口捕获确认标题栏与内容使用同一连续圆角轮廓，窗口阴影和交通灯保持系统绘制；双击放大与还原后普通窗口圆角恢复。
- 验证：桌面 Go tests 与 Wails DEV build 通过；Wails build 仅输出既有 macOS link target 警告。
