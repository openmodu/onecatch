# 对话选区与后台更新

后台某条回复更新时，不能清空其他回复中已经建立的选区。正文同步必须区分内容未变、末尾追加和全文修正。

这条经验来自 2026-10-13 的复制故障排查，依赖版本为 `github.com/dyike/keel v0.2.2`。升级 Keel 时应重新核对下面的行为。

## 已复现的原因

用户录屏显示：Agent 仍在运行，下面有工具调用；上面的多行中文回复拖选后，出现复制失败或内容不完整。

单独让工具结果逐帧更新，测试仍能完整复制正文。明确失败的场景是：选中一条已完成回复，再更新另一条没有 `streaming` 标记的消息快照，随后按 Cmd/Ctrl+C，剪贴板没有写入选中的文字。

旧的 `syncConversationSource` 只在消息正在流式输出时使用 `Doc.Append`。普通快照更新走 `Doc.SetSource`，而 Keel v0.2.2 的 `SetSource` 会清空该文档所属的整个 `SelectionGroup`。一条后台消息的更新因此影响了另一条回复的选区。

故障测试是 `TestConversationCopySurvivesUnrelatedMessageSnapshot`，修复前实际失败，修复后通过。它验证后台消息追加、全文修正、再次追加后，选中的回复身份不变，剪贴板文字与预期全文相等。

## 当前实现约定

正文统一通过 `internal/app/keel/conversation.go` 的 `app.conversationSource` 同步，普通回复、用户消息、工作流结果正文和可读摘要都使用这个入口。

- 内容未变：复用原文档。
- 新内容以前文为前缀：复用文档并调用 `Append`，包括没有流式标记的持久化快照。
- 内容被改写：为这一条消息创建新文档。随后由 `SelectionGroup.SetDocuments` 按文档身份保留其他回复中的选区。

已经加入对话 `SelectionGroup` 的文档不能直接用 `SetSource` 更新后台快照，否则会重新引入跨消息清空选区的问题。这条限制针对加入选区组的正文，不代表所有独立工具文档都禁止使用 `SetSource`。

如果被改写的是正在选中的文档，替换文档可能清除该选区；当前修复保证其他未改写回复的选区不受影响。文档身份变化后，检查当前的 `a.docs[key]`，不要拿已经被替换的旧文档判断折叠状态。

复制按钮仍从原始消息文档读取全文。拖选复制跟随实际显示的文字；工作流结果卡片的可读摘要与原始 JSON 分开保存，不能把显示摘要覆盖回原始内容。

## 验证方法

核心回归测试位于 `internal/app/keel/conversation_copy_regression_test.go`：

- `TestConversationCopySurvivesUnrelatedMessageSnapshot`：后台消息快照更新不清空另一条回复的选区。
- `TestConversationCopiesWrappedRepliesBetweenLiveTools`：用录屏中的两段回复和中间七条工具调用，逐帧刷新工具结果；在 1×、2× 下反复正向、反向及快速拖选，检查剪贴板全文相等。
- `TestConversationRepeatedDragCopyAfterScrolling`：滚动后反复拖选复制，包含从输入框切换焦点的情况。

同时检查工作流卡片选择复制、跨虚拟消息选择，以及长用户消息的展开、收起和完整复制。

```sh
go test ./internal/app/keel -run 'TestConversationCop|TestConversationRepeatedDrag|TestConversationSelection|TestWorkflowOutcome|TestLongUserMessageDisclosure' -count=1
go test ./internal/app/keel -count=1
git diff --check
```

2026-10-13 修复后，上述相关测试及界面包完整测试通过。录屏已检查，测试使用原生指针和键盘事件读取剪贴板；未在用户正在运行的客户端中重新操作验证。后续报告应分别说明这三类证据。

同日升级 Keel v0.2.3 时，重新核对了 `Doc.SetSource` 清空 `SelectionGroup` 的行为，仍需保留上述同步约定；复制相关回归与界面包完整测试再次通过。
