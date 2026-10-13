# Keel 托盘与窗口生命周期

Keel v0.2.3 的托盘菜单提供显示、隐藏、新建任务、最近活跃的 5 个 Session、设置和退出。运行中的会话排在前面，同组按活动时间降序排列；没有更新时间时使用创建时间。隐藏项目的会话不进入托盘。项目名最多 10 个显示列，标题最多 28 个显示列，超出时分别显示省略号。

菜单内容来自现有任务列表刷新通知，窗口关闭后仍更新。只有项目名、标题、状态、顺序或语言改变时才调用 `SetMenu`，不会逐帧重建原生菜单。点击会话时重新查询当前任务；任务已删除则不跳转，点击当前会话也不重置草稿和运行记录。

## 关闭与退出

`window.Options.OnClose` 是窗口销毁后的通知，无法拦截关闭。托盘存在时，关闭主窗口仅清空窗口引用，保留 `app`、后端、消息文档和主界面 `el.RootWidget`。再次显示时创建新原生窗口，并复用界面根节点以保留输入和滚动状态。

应用使用 `window.MainWithOptions`，仅在托盘创建成功时启用 `KeepAlive`。没有窗口时，Keel 仍消费 `core.Update`，后台流、任务列表刷新和托盘操作可以继续执行。托盘创建失败时，主窗口关闭仍触发清理，避免留下用户无法退出的后台进程。

退出托盘、应用菜单或自动更新器时调用 `window.Quit()`；不要用 `a.window.Close()` 代替退出，否则启用托盘后会变成关闭窗口并常驻。清理由应用 `OnQuit` 调用幂等的 `app.close`，移除托盘并关闭终端、订阅、上下文和后端。

## 原生接口边界

托盘回调在 Go goroutine 中串行执行，UI 修改需要进入 `core.Update`，并忽略应用关闭后排队的回调。macOS 的 `tray.New` 要放在 `window.MainWithOptions` 前、UI frame lock 外；持锁创建会有主线程等待风险。

v0.2.3 的 `tray.MenuItem` 只有文字、选中、禁用、分隔线和子菜单接口，没有菜单项图片、颜色或呼吸动画接口。实现运行状态的呼吸小绿点需要扩展 Keel 的菜单项接口，或使用自绘托盘面板。不要通过反复 `SetMenu` 伪造动画：菜单更新会更换原生动作 ID，过期的菜单事件不会触发新动作。

## 回归入口

`internal/app/keel/tray_test.go` 验证最近五条排序、隐藏项目过滤、项目和标题截断、显示/隐藏相邻、菜单增量刷新与失败重试、语言更新、窗口关闭保留业务状态、无托盘时清理、退出清理幂等，以及回调进入 UI 队列。macOS 模板图标测试检查深色底块透明化后仍保留可见标记。

```sh
go test ./internal/app/keel ./cmd/keel -run TestTray -count=1
go test ./internal/app/keel ./cmd/keel -count=1
go vet ./internal/app/keel ./cmd/keel
go build ./cmd/keel
go run github.com/dyike/keel/examples/tray -smoke
```

升级 Keel 时还应运行 [对话选择与复制的回归测试](keel-conversation-selection.md)。业务状态测试、原生托盘菜单截图和完整客户端运行验证是不同证据；报告时分别说明。

2026-10-13，升级后的界面包完整测试、vet 和 macOS 构建通过；Keel 自带的原生托盘 smoke 返回 `PASS`。该 smoke 验证的是 Keel 的托盘创建、更新、无窗口重开与退出，OneCatch 的业务状态保留由本仓库测试验证；尚未用这次构建在用户客户端重新点击托盘菜单验收。
