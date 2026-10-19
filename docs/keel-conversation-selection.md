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

## 并行任务固定视口与滚动位置（2026-10-14）

并行步骤的正文列现在固定为 420 dp，每列使用独立的 `MessageScroller`。内容在列内滚动，滚到底部时跟随新增输出，向上阅读时保留本列位置。步骤标题仍在正文区外；短列保留空白，不再由长列撑高。每列也按消息虚拟化，避免把整段并行历史全部构建为一行。相关实现位于 `conversation_lanes.go`。

外层并行行的高度不随正文变化，因此正文更新不再使外层高度缓存失效。展开工具等操作的失效请求转发给对应列。消息顺序没有变化时，外层和列内都不再调用 `SetKeys`；该调用会重建索引并设置阅读位置恢复标记。

另一个已在测试中复现的跳动来自可见长消息的 `Invalidate`：Keel v0.2.3 会先将其高度重置为 140 dp，再恢复锚点，消息内部的阅读偏移因此被截短。现在记录上一帧已挂载的消息，只对未挂载的变化消息主动失效；已挂载消息由 Keel 每帧自动测量。这里的“已挂载”包含预渲染区域，不仅是屏幕上可见的文字。

新增回归：

- `TestParallelLanesFixedHeightAndIndependentScrolling`：1×、2× 下两列等高，左列滚动不移动右列，右列追加不移动外层和左列阅读位置。
- `TestConversationGrowingVisibleReplyKeepsReadingPosition`：普通对话与并行列中，滚入长回复后追加正文，逐帧核对阅读标记。临时撤销高度保留修复后，两种场景均失败；普通对话的标记在第一帧下移 600 像素。恢复修复后通过。
- `TestParallelLaneCopySurvivesOtherLaneUpdate`：在一列拖选文字，另一列追加长内容后，剪贴板仍包含完整选区。

Apple M5 Pro、2× 布局下，`-benchtime=1s -count=3` 的平均耗时取三次中位数：

| 场景 | 修改前 | 修改后 |
| --- | ---: | ---: |
| 并行两列，每列 4 条消息 | 0.881 ms/帧 | 0.587 ms/帧 |
| 并行两列，每列 60 条消息 | 10.015 ms/帧 | 0.590 ms/帧 |
| 普通多消息滚动 | 0.632 ms/帧 | 0.650 ms/帧 |
| 单条超长回复滚动 | 4.297 ms/帧 | 4.443 ms/帧 |
| 单个流式增量 | 1.106 ms/次 | 1.117 ms/次 |
| 30 个增量合批 | 1.313 ms/次 | 1.339 ms/次 |

60 条消息的并行场景，每帧内存分配从约 6.20 MB 降到 0.44 MB。普通对话和单条超长回复未测到性能收益；不能将并行场景的提升推广到所有对话。这些基准测量布局与输入路由，不包含实际窗口的 GPU 呈现，也不代表客户端帧率或尾延迟。

```sh
go test ./internal/app/keel -count=1
go test ./internal/app/keel -run '^$' -bench 'BenchmarkConversation(ParallelScroll|Scroll|StreamToken|StreamBurst|IdleFrame)$' -benchtime=1s -count=3
ONECATCH_KEEL_PREVIEWS=/tmp/onecatch-parallel-preview go test ./internal/app/keel -run '^TestRenderFixedParallelLanes$' -count=1
```

界面包完整测试通过。独立渲染截图用于检查固定列布局；尚未在用户正在运行的客户端中操作验证，也没有新的用户录屏证据。

## 用户消息换行与 Markdown（2026-10-14）

用户气泡保留输入中的普通换行，同时继续渲染 Markdown 标题、列表、引用、加粗和代码块。Keel v0.2.3 默认把 Markdown 软换行转为空格，中文行尾则直接合并，因此原先气泡中的多行内容会连在一起。

`conversationSource` 在创建用户消息文档时安装局部 AST 转换器，将软换行转为硬换行。原始消息内容不变，复制按钮仍读取原文；助手回复保留默认 Markdown 换行规则。插件只在新文档创建时配置，追加继续使用 `Append`，全文修正继续替换文档，避免重设插件影响已有选区。

`TestUserMessageMarkdownPreservesLineBreaks` 覆盖普通及长文本换行、CRLF、Markdown 标记解析、消息追加和全文修正；`TestAssistantMessageKeepsMarkdownSoftBreaks` 检查助手回复仍使用默认软换行。界面包完整测试通过；新增长文本用例另行通过专项测试。尚未在实际运行的客户端中操作验证。

## 粘贴图片后的中文输入（2026-10-14）

用户反馈粘贴图片后，中文候选一闪即消失，输入变成英文。测试复现了两处会影响输入法的状态变化：附件栏插入后，无 ID 的主题容器按位置重新分配状态，内部编辑器丢失焦点；固定容器身份后，粘贴完成时排队的 `SelectInput` 仍会覆盖同一帧的拼音编辑，将 `ni` 后的光标拉回输入前的位置。Keel 的 macOS `shouldCancelComposition` 会在编辑器报告的选区与平台状态不一致时取消组合输入。

输入框主题容器现在固定为 `composer-editor`。粘贴完成后，已聚焦的输入框保留选区；失焦时通过文档选区和一次焦点请求恢复输入。只有编辑器尚未创建时才保留待处理请求，不再逐帧排队重设光标。

`TestPastedImageKeepsChineseComposition` 检查粘贴完成同一帧的拼音光标、候选区域及中文提交，修复前失败。必须检查第一帧，等待多帧后的正确状态会掩盖瞬间的选区错误。`TestAttachmentRowsPreserveActiveComposition` 检查组合输入期间附件加载、完成和移除；原有测试继续检查失焦恢复、重复粘贴和过期回调。界面包完整测试通过。这些测试注入输入法事件，不等同于真实 macOS 中文输入法的客户端验证。

2026-10-14 升级至 Keel 主分支固定提交 `74ea9c5` 后，界面包完整测试再次通过。分配量对比及真实客户端内存采样见 [Keel 内存排查](keel-memory.md)。

## 空输入框光标不可见与首次聚焦（2026-10-19）

用户反馈输入框再次没有光标，首次找不到焦点。在发布依赖 Keel v0.3.3 上，测试分别复现了两个问题：启动后直接输入没有接收者；点击空输入框后，焦点已在 `prompt`，但像素中没有光标，不能把后者认定为失焦。

调用路径是 `views.go:composer` → Keel `el` 输入绘制 → `editorstyle.Caret.LayoutDecorated`。空编辑器把光标放在提示文字前 2dp，光标宽 1.5dp；输入框原来只有 2dp 左内边距，光标完全落在控件裁剪范围外。现在左右内边距为 4dp，容纳提示文字前的光标。启动和显式新建对话各请求一次焦点，在首次绘制后生效；后续重绘不重复抢焦点。编辑器创建前已有粘贴请求时，优先走粘贴的选区恢复路径，避免提前聚焦使草稿末尾定位被跳过。

`TestComposerInitiallyAcceptsTyping` 检查启动直接输入、新建对话恢复输入，以及切换到其他输入框后重绘不抢焦点。`TestEmptyComposerFirstClickPaintsCaret` 在浅色/深色、1×/2× 下检查第一次点击后的实际光标像素和文字输入；修复前四组均已聚焦但像素变化为零，修复后通过。原有粘贴恢复、中文组合输入和主题表面测试也通过；主题表面测试显式关闭启动聚焦，以继续比较失焦与聚焦两种外观。

`go test ./internal/app/keel ./cmd/app -count=1`、这两个包的 `go vet`、macOS 构建和 `git diff --check` 均通过。

原生候选客户端使用 `/tmp/onecatch-composer-focus/data` 独立目录，已检查首次打开后无需点击即可输入，以及清空文字后提示文字前的光标可见。未发送消息或执行工具；没有替换用户正在运行的安装客户端。该窗口检查不等同于真实中文输入法验证，组合输入的证据仍来自事件注入测试。文档记录与本轮构建使用 go.mod 中的 v0.3.3，没有本地 go.work 或 replace。

## macOS 系统字体统一（2026-10-14）

界面与 Markdown 正文统一使用 `theme.Material.Face`；代码保留等宽字体。此前正文直接使用 `theme.Face`，没有跟随 OneCatch 的界面字体设置。底层 Markdown 的块缓存同时纳入正文和代码字体，避免字体切换后仍使用旧的换行与测量结果；不重写文档内容。

本机探测发现 SFNS.ttf 的 OpenType 字体族为 `System Font`，旧配置中的 `SF Pro Text`、`SF Pro` 和 `.AppleSystemUIFont` 均未命中。macOS 的苹方位于 `FontServices.framework/Resources/Reserved/PingFangUI.ttc`，不在原字体扫描目录中。加入系统目录后确认苹方能够命中。直接使用 SFNS 的 `System Font` 在本机像素测试中出现英文缺字，因此普通文本采用系统自带的 Helvetica Neue，中文采用苹方，代码保留系统等宽字体；不将当前引擎的 SF 可变字体支持描述为已完成，也不等同于使用 AppKit 完整排版。

底层变更暂通过本机忽略的 `go.work` 接入相邻 Keel 仓库，尚未包含在已发布的 v0.2.4 中。

## 文件链接分流（2026-10-14）

用户点击 Markdown 中的本地 `.app` 路径时，原处理器统一调用 `core.OpenURL`，报 `unsupported scheme ""`。现在 `conversation_links.go` 区分网页链接与文件路径：HTTP、HTTPS、mailto 保留原处理器；本地文件、目录和应用通过系统文件处理器打开。相对路径按当前任务 worktree 或工作区解析，`~/` 展开为用户目录，支持 `file://` 和编码后的空格。远端工作区的相对路径不会交给本机打开。

打开前检查文件存在；优先保留完整文件名，找不到时再识别 `:行号[:列号]` 或 `#L行号` 后缀。这些后缀目前只用于定位文件，不表示编辑器行列跳转。传给系统的是独立路径参数，不经 shell 拼接。

`TestConversationFileLinkClick` 通过指针点击带空格的 `.app` 链接，验证进入文件处理器；`TestConversationFileLinkResolution` 覆盖工作区、worktree、文件 URI、中文与空格、行号、缺失文件及启动失败。已有网页链接点击和拖选复制回归继续通过。测试替换了系统启动函数，未实际启动附件应用。

## 重试分组与叠页（2026-10-14）

用户截图显示六个狭窄并排列，而设置中的并发上限为 4。核对本机对应工作流的 91 次步骤执行记录，配置确为 4，执行时间的重叠峰值为 2。截图中的六列是两个步骤交错失败、重试产生的六次执行；不能据此认定调度器启动了六个并发任务。

`buildWorkflowTimeline` 原先把时间区间间接重叠的所有执行都分配为独立列。现在按开始时间分配空闲执行位置：前一次结束后，后一次可以复用该位置，页面数对应实际重叠峰值。后续执行在正文中保留步骤名、执行次数和状态，历史输出不会丢失。`TestWorkflowTimelineRetriesReuseConcurrentLanes` 用六次执行、峰值为二的区间复现并验证这一修复。调度器逻辑未改；`TestDAGSixRootsRespectFourConcurrentSteps` 阻塞前四个任务，确认第五个不能提前启动，释放后六个任务均完成。

并行区域改为水平方向错开 36 dp 的叠页，上下对齐；数字标签位于各页露出的左右边缘，点击切换前后页；正文固定 420 dp，每页独立滚动。页头和正文通过稳定的 `pageGroup` 共享当前页。每页除保留 `MessageScroller` 外，还保留独立的嵌入视图根，否则卸载再挂载会丢失底层滚动状态。嵌入根使用固定尺寸容器和 `LayoutIn`，保留父级滚动坐标与选择上下文。删除整个区域时一起释放页码和视图状态。

`TestParallelLanesFixedHeightAndIndependentScrolling` 在 1×、2× 下点击数字标签，检查固定高度、切页返回后的阅读位置，以及隐藏页追加时当前页不移动。原有正文追加和拖选复制测试继续覆盖叠页。界面包（含独立渲染截图）和工作流用例包完整测试通过；本次测试使用当前本机 `go.work` 依赖。截图用于检查布局，尚未在实际运行的客户端中操作验证。

## 字体集合与原生字重错配（2026-10-14）

用户截图显示统一字体后的中文明显偏细。进一步核对实际字体身份：本机 `PingFangUI.ttc` 的 sfnt 第 20 个字体是 `PingFangSC-Medium`，CoreText 描述符第 20 项却是 `.PingFangUITextHK-Thin`。CoreText 展开可变字体的命名实例，不能把两个数组的位置视为相同编号。此前只验证字体族解析和布局，未检查原生绘制字体身份，遗漏了这个故障。

Keel 现在读取所选 sfnt 字体的 PostScript 名称，在相同文件的 CoreText 描述符中精确匹配，并再次核对创建结果；没有精确匹配时使用轮廓回退。新增原生测试检查实际创建的苹方简体 Medium 与 SF Regular 对象，而不只检查请求的字体名称。保留 OneCatch 的 Helvetica Neue + 苹方配置，没有全局加粗。随后确认正文请求 Normal（400）仍会选到文件默认的 Medium（500）：字体扫描没有收录 `fvar` 命名字重，且解析器误把轴数量当成实例记录步长。Keel 现已修复记录解析、收录其他轴保持默认值的命名字重、应用实例坐标，并将实例身份传递到 CoreText。字体缓存版本升级后会自动重建。原生回归测试验证正文实际创建 `PingFangSC-Regular`，较重字重创建 `PingFangSC-Semibold`。这不代表支持任意可变字体轴，也不能替代实际客户端验证。

Regular 修复的离屏 Metal 截图已检查：正文与 Markdown 加粗可区分。流式渐显像素测试同时暴露空格前的裁剪边界会切掉前一个字形的外伸笔画，Keel 已把边界移到空格内部。消息气泡的自然宽度测量也跟随 `theme.Material.Face`。这些检查与原生字体身份测试均不等同于正在运行的客户端验证。

## 无变化轮询与重绘（2026-10-14）

项目工作流每两秒调用 `core.Update`，即使签名未改变，也请求整窗绘制；工作区 Git 检查也会因无变化结果重绘。本次改为 `core.UpdateIf`，后台仍在同一帧锁下读写状态，只有可见结果改变才请求绘制。运行状态灯改为静态绿点，状态切换仍更新。

这修复的是无变化轮询带来的绘制和内存回收延迟，不能据此认定所有后台重绘都会破坏选区。工作流文件变更仍应及时显示；签名不变时保留文档、滚动与选区。现有对话选区测试通过，真实客户端另行验证滚动后内容和回收情况，见 `keel-memory.md`。

## 混合字体标签末字缺失（2026-10-14）

用户截图中“运行了 1 个工具”只显示到“工”。本机用 Helvetica Neue + PingFang SC、13sp 复现：排版器拿不到苹方的轮廓尺寸，`GlyphExtents` 返回失败，而 `trimTrailingWhitespace` 把零墨迹宽度当成空白，将“具”的前进宽度清零。2× 时标签自然宽度仅 164px，绘制末字超出控件宽度后被裁掉；语义标签仍保留完整字符串，因此只检查文本或点击不能发现。

Keel 的行尾裁剪现在核对源字簇，只有全为空白时才允许清零；多字体单行快速路径与普通换行路径都应用这个条件。没有修改 Regular 字重或给具体标签硬加宽度。`TestMixedLabelNaturalWidth` 覆盖 1×、1.5×、2× 下中文、英文、数字混排，检查非零前进宽度及自然宽度与宽画布的原生渲染像素一致。临时撤销补丁后该回归失败。合成测试另覆盖 LTR、RTL、纵排、真正空白和含组合标记的字簇。

Keel 的 `TestDefaultInputTokenCentersInkWithLineLeading` 在补丁前后均有垂直居中失败，不能报告整个 el 包通过。当前补丁通过本地 `go.work` 接入，尚未发布；离屏验证不能代替用户正在运行的客户端验证。

缺字补丁现已通过 [Keel PR #25](https://github.com/dyike/keel/pull/25) 合入，并发布为 [v0.2.6](https://github.com/dyike/keel/releases/tag/v0.2.6)。OneCatch 的 go.mod/go.sum 已升级到该版本；发布依赖验证使用 `GOWORK=off`，不把相邻 Keel 工作区的其他改动算入本次发布。

## 工作流摘要右侧裁切（2026-10-15）

用户截图中，摘要每行的右端像被白色区域盖住。使用截图中的长中英混排摘要，在 Keel v0.2.8 原生离屏渲染中复现：部分行末字符被文档边界裁掉，原始消息和复制文本仍完整。

调用路径为 `views.go:messageEntry` → `conversation_outcomes.go:workflowOutcomeCard` → `conversation.go:conversationDocument` → Keel Markdown `shapeLine` → `typesetting/shaping.Output.advanceSpaceAware`。卡片的 16dp 内边距和正文约束正常；不是新增遮挡层，也不是摘要数据被截断。

之前的缺字补丁保护了**最终行尾裁剪**，但遗漏**候选换行宽度**。`advanceSpaceAware` 仍在行末字形 `Width == 0`（纵排为 `Height == 0`）时直接扣掉它的前进宽度，没有检查源字簇是否为空白。轮廓尺寸读取失败的可见字符因此被少算，换行时多放一个字符。诊断中，限制 1000px 的一行实际排到 1017px，原生绘制也超出边界；文档按正确宽度裁切，就出现用户看到的缺字。

底层修复为候选行宽度加入源字簇空白判定，与最终行尾裁剪共用 `glyphIsWhitespace`。只有真正的空白字簇才允许扣除零墨迹字形的前进宽度；非空白、含组合标记和无法确认的字簇保留宽度，字间距及反向 run 的规则保持。没有给摘要卡片硬加宽度、调整字号或修改原文。

新增回归：

- `TestConversationWrappedLinesRespectContentWidth`：直接使用 Markdown 的单行排版参数及不可见截断标记，逐行检查实际字形前进宽度。覆盖 4 种字号、1×/2×、340/700/860dp 内容宽度。`GOWORK=off` 的 v0.2.8 在全部 24 组中失败，接入补丁后通过。
- `TestWorkflowOutcomeSummaryWrapsInsideCard`：长中英混排与纯中文、440/800/1200dp 视口、1×/2× 下，核对卡片内容边界和独立约束文档的尺寸。这个边界测试在修复前也通过，说明**语义边界或全文复制正常不能证明行末字形完整**。
- `TestRenderLongWorkflowOutcomeSummary`：生成三种宽度的原生离屏截图；已检查修复前后的 800dp 截图，修复后行末文字在边界内换行。
- Keel `TestWrapMissingOutlineMetricsRespectsWidth`：合成零墨迹非空白字形，覆盖普通换行、截断和启用/禁用空白裁剪。修复前 16px 的行保留 32px 字形，四组均失败，修复后通过。其他合成测试覆盖 LTR/RTL/纵排、字间距、真正空白、组合标记及无效字簇范围。

验证命令：

```sh
go test ./internal/app/keel ./cmd/app -count=1
go vet ./internal/app/keel ./cmd/app
ONECATCH_KEEL_PREVIEWS=/tmp/onecatch-summary-after go test ./internal/app/keel -run '^TestRenderLongWorkflowOutcomeSummary$' -count=1
# 在 Keel 修复源码中：
GOWORK=off go test ./typesetting/... ./gio/text ./ui/markdown ./internal/deps -count=1
GOWORK=off go test -race ./typesetting/shaping -count=1
```

上述完整包测试、相关 race、vet、macOS 构建与两仓 `git diff --check` 通过。没有重新运行整个 Keel el 包；上节记录的既有输入垂直居中失败不属于本次修复。

**交付范围：补丁尚未提交或发布。** 底层源码保留在相邻的 `github.com/dyike/keel-summary-width` 独立工作区，基于 v0.2.8；本机忽略的 `go.work` 临时接入该目录。OneCatch 的 go.mod/go.sum 仍依赖发布版 v0.2.8，关闭工作区后新增排版回归仍会失败，不能把本地修复描述为 v0.2.8 已修好。截图证据在 `/tmp/onecatch-summary-before` 和 `/tmp/onecatch-summary-after`，候选构建为后者的 `onecatch`。发布底层补丁并升级依赖后，应删除本地 go.work，再用 `GOWORK=off` 验证。

没有替换或重启用户正在运行的安装客户端，也没有在真实窗口操作该工作流、缩放或拖选验证；原生离屏截图和测试通过不等同于客户端已修复。

## 思考与工具调用分组（2026-10-15）

用户截图中，“运行了 25 个工具”的卡片里混入多行思考预览，思考与命令交替出现。原因是 `buildConversationEntriesWith` 把同一步骤内相邻的所有过程记录合并为一组，思考记录也作为工具卡片的一行绘制。

现在同一步骤内，思考与工具分别成组：一个组只包含思考，或只包含工具调用、文件变更等记录；两者交替时按原顺序切分。工具组仍用带边框的卡片，计数只统计其中的工具；思考组标题为“思考过程”（运行中的最新组为“思考中…”），展开后直接显示正文，不再嵌套第二层展开。`processItem` 不再绘制思考行。

`TestReasoningSeparatesFromToolGroups` 检查分组形状、标题、展开后的工具卡片中不含思考行，并可通过 `ONECATCH_KEEL_PREVIEWS` 生成 `reasoning-groups.png`；`TestLiveReasoningGroupLabel` 检查思考组出现后，前面的工具组不再标为运行中。界面包完整测试通过，测试使用当前本机 `go.work`。离屏截图已检查；尚未在实际运行的客户端中操作验证。思考正文仍按纯文本显示，`**标题**` 标记原样保留，与修改前一致。

## 独立文件编辑卡片（2026-10-16）

用户截图中，文件修改混在工具组里，展开后只有路径代码块。文件变更现在在 `buildConversationEntriesWith` 中独立成行，由 `conversation_files.go` 渲染；普通工具与思考仍按原顺序分组。首版每个文件默认展示最多 8 行手工绘制的差异预览、旧新行号和增删数量，删除为红色，新增为绿色；展开切换到只读 `CodeEditor`，最大高度 300dp。复制按钮读取保存的完整差异，显示不改原文。预览和展开的样式随后统一，见下节。

`agentrun.FileChange` 在运行事件、历史视图和 transcript 间传递统一差异。Codex/TRAE 使用运行时提供的差异；Pi/DSH 和 ACP 可从编辑参数或前后内容生成片段，未知位置不编造行号。整文件写入只展示写入内容，并提示原内容未记录。ACP 后续更新复用文件事件身份，失败仍标为失败。单文件保存上限为 256 KiB，截断有提示并保持 UTF-8 完整。

旧记录优先从保存的原始运行事件恢复差异，只有路径的记录显示“未保存代码差异”。不得用当前工作区文件或当前 Git diff 推断历史编辑。预览解析及展开编辑器按内容缓存，内容不变不重建；编辑器不加入正文 `SelectionGroup`。

`TestFileChangeSeparateFromToolsAndRestored`、`TestFileChangeCardPreviewExpandCopy`、`TestFileChangeLegacyPaths` 以及服务和运行时测试覆盖分组、历史合并、1×/2× 展开收起、只读输入、包含屏外内容的选择复制、旧事件恢复和协议差异。界面、desktop、agentrun、cmd/app 完整包测试通过（本机 go.work）；新增专项、原有跨消息选区复制回归、相关包 vet 和 macOS 构建另在 `GOWORK=off` 的发布依赖 v0.2.10 下通过。`TestRenderFileChangeCards` 生成 840dp 和 360dp 离屏截图，已检查。尚未替换或重启用户正在运行的客户端，也未在真实窗口中操作验证。

## 文件编辑代码块样式统一（2026-10-16）

用户反馈文件卡片展开后样式不统一。首版收起时手工绘制旧新行号和差异行，展开时换成代码编辑器，并插入第二条完整路径；两套组件的字号、行号列和间距不同。

根据后续反馈，文件卡片的“收起”不是只剩标题，而是默认展示部分内容：每张卡片都保留文件图标、“编辑文件 · 路径”、增删数、复制和右箭头，并在下面露出最多约 8 行 diff。删除行使用淡红背景与红字，新增行使用淡绿背景与绿字。点击后仍使用同一个只读 `CodeEditor` 展示更多内容，最大 300dp，内容多时在内部纵向滚动；再次收起恢复部分预览。长行始终自动折行，不能用横向裁切隐藏行尾。两种状态共享完整差异文档、选区和滚动状态。行号是差异文档的行号；源文件坐标保留在原始 `@@` 段头中，不能把文档行号当成源文件行号。

差异标记解析按内容缓存，以 `CodeDecorationText` 立即为新增、删除及段头设置绿色、红色及弱化色，不依赖异步语法高亮完成。主题颜色变化时更新装饰，不替换文档或清空选区；未变的内容和主题不逐帧重建装饰。完整复制、只读和旧记录的缺失差异提示仍保留，编辑器不加入正文 `SelectionGroup`。

`TestFileChangeCodeStyleSurvivesCollapse` 覆盖浅色/深色、1×/2× 和 360/840dp：原生离屏像素检查代码块背景、首帧红绿文字和淡色行背景，以及从部分预览展开后段头、上下文、增删行和行号的像素一致。`TestFileChangeCardPreviewExpandCopy` 另检查每张卡片默认展示部分内容和增删元数据、展开视口有界、复制读取完整差异、编辑器只读，以及收起再展开保留文档和完整选区。

首版验证使用 `GOWORK=off`，当时的发布依赖为 `github.com/dyike/keel v0.3.1`，没有把本地 `go.work` 中的底层改动算入结果：

```sh
GOWORK=off go test ./internal/app/keel ./internal/service/desktop ./internal/usecase/agentrun ./cmd/app -count=1
GOWORK=off go vet ./internal/app/keel ./internal/service/desktop ./internal/usecase/agentrun ./cmd/app
ONECATCH_KEEL_PREVIEWS=/tmp/onecatch-file-code-style GOWORK=off go test ./internal/app/keel -run 'TestFile(Change|Diff)|TestRenderFileChangeCards' -count=1
GOWORK=off go build -o /tmp/onecatch-file-code-style/onecatch ./cmd/app
git diff --check
```

初版的完整包测试、专项像素与交互回归、vet 和 macOS 构建通过。恢复默认部分预览后，文件变更专项测试通过；重新检查了 840dp 与 360dp 离屏截图，两张卡片都默认露出 diff，增删背景可见，窄窗口标题路径会截断而代码长行会折行。当前工作区 `go.mod` 已并行改为 Keel v0.3.2，但 `go.sum` 仍只有 v0.3.1，故本轮使用本地 `go.work` 中的 Keel 源码验证，不能把结果描述为发布依赖 v0.3.2 已验证。没有替换、重启正在运行的安装客户端，也未在真实窗口操作验证；测试和截图不能代替客户端验证。

## 文件编辑卡片对齐参考图（2026-10-16）

后续对照参考图发现，通用 `CodeEditor` 虽然已经能显示 diff，但仍带有折叠箭头、差异文档自身的连续行号、内嵌圆角和常驻滚动条，与参考图不是同一种组件。文件头也把“编辑文件 · 完整路径”全部加粗，缺少参考图中“文件名加粗、动作和目录弱化”的层次。

文件卡片现在使用专用的 unified diff 表格：标题依次显示文件图标、文件名、动作、父目录、增删数量、复制按钮和展开箭头；不再显示工具调用时间。正文紧贴标题分隔线，hunk 段头保留原文，内容行分别显示旧文件行号、新文件行号和增删标记；新增、删除背景铺满整行。默认最多展示 8 行，长行在小卡片内省略，展开后仍使用同一套表格并在 300dp 内纵向滚动。底部展开按钮回到独立白色栏，不再嵌在第二层圆角代码块内。完整 diff 仍保存在 `fileChangeView` 中，复制按钮不受预览截断影响；这套专用表格不冒充可编辑器。

`TestFileDiffCountsIgnoreHeaders` 同时检查 hunk 坐标到双行号的映射；`TestFileChangeCardPreviewExpandCopy` 检查标题拆分、8 行预览、有界展开和完整复制；`TestFileChangeCodeStyleSurvivesCollapse` 继续在浅色/深色、1×/2×、360/840dp 下比较预览与展开的相同行像素，并检查红绿文字和整行背景。离屏结果已与参考图逐项对照；实际安装客户端仍需另行验证。

## 滚动后的反向回跳（2026-10-16）

用户 GIF 显示文件卡片滚上去后又向下回落。录屏本身约为 12fps，不能据此测客户端帧率；本次定位的是内容位置反向移动。

在发布依赖 Keel v0.3.2 上，以文件差异预览和折叠思考组交错组成历史列表，注入滚轮事件后逐帧检查同一卡片的坐标。`TestConversationScrollDoesNotBounceWhileMeasuringHistory` 在向上滚动的第 13 次输入后复现：没有新输入的下一帧，卡片从 y=78 移到 y=196，回落 118dp；2× 下从 157px 移到 393px。回归还检查实际经过至少 10 张卡片，避免空列表或未滚动时误通过。

调用路径为 `views.go:conversationScroll` → Keel `kit.MessageScroller` → `kit.VariableListView.Render`。列表用估计高度构建屏外占位，绘制后才读取新挂载行的实际高度，再把锚点恢复留到下一帧。新进入预渲染区域的短思考组和长 diff 卡片高度相差较大：当前帧内容先偏移，下一帧恢复位置，于是出现回落。消息数据不变也会触发，不能把这个故障归因于后台快照覆盖滚动状态。

底层补丁在布局完成、绘制开始前读取已挂载行的高度，用当前锚点计算本帧位置补偿；滚轮事件随后照常参与绘制，绘制后再记录新的阅读锚点和高度缓存。补偿按物理像素比较，避免小数 dp 的求和误差持续请求重绘。虚拟化、自动跟随底部和显式消息跳转仍保留。

回归与验证：

- OneCatch 新增测试覆盖 1×/2×、连续双向滚动、停止输入后的前三帧，以及期间内容不变的后台快照；原版失败，补丁版通过。
- Keel `TestVariableListHeightChangesPreserveAnchorBeforePaint` 覆盖横向/纵向、1×/1.5×/2×、前一行增长和缩短，以及高度变化同帧收到滚轮事件。检查第一帧及后续两帧，不能只等待位置稳定后断言。临时撤销底层补丁后六组均在第一帧失败，恢复后通过。
- OneCatch 界面包和入口包，Keel 的 kit、el、markdown、依赖边界完整测试，以及相关 vet、滚动和选区复制 race 测试通过。macOS 候选客户端构建通过。
- 另开原生候选客户端，使用“继续迁移工作”的已完成会话副本，在 2240×1520 像素窗口中操作短距离连续上滚、下滚和停止，检查文件预览及文字。该检查确认真实窗口加载与输入可用，未采集高帧率录屏或端到端帧率；逐帧回跳的定量证据来自上述测试。没有发送消息、执行会话工具或修改原会话数据，也未替换用户正在运行的安装客户端。

复验命令：

```sh
# 在 OneCatch，直接使用 go.mod 中的远端依赖：
GOWORK=off go test ./internal/app/keel ./cmd/app -count=1
GOWORK=off go vet ./internal/app/keel ./cmd/app
GOWORK=off go test -race ./internal/app/keel -run 'TestConversation(ScrollDoesNotBounce|GrowingVisible|CopySurvives|RepeatedDrag|SelectionAcrossVirtual)|TestParallelLane' -count=1
# 在 Keel 补丁源码中：
GOWORK=off go test ./ui/kit ./ui/el ./ui/markdown ./internal/deps -count=1
GOWORK=off go test -race ./ui/kit -run 'Test(VariableList|MessageScroller|VirtualLists)' -count=1
```

交付状态：底层修复已直接提交并推送到 Keel 的 `codex/scroll-anchor` 分支，提交为 `2024cc41edd76b6a6ad2a912f84476ae70b2bfcd`，见 [Keel PR #40](https://github.com/dyike/keel/pull/40)。OneCatch 的 go.mod/go.sum 已锁定远端版本 `v0.3.3-0.20261016113346-2024cc41edd7`。这是可下载的提交版本，尚未合入主分支或发布正式标签；不能归入原来的 v0.3.2。

本机 go.work/go.work.sum 已删除，不使用本地 replace 或手工修改模块缓存；临时补丁副本也已移除。关闭工作区后，已再次通过 OneCatch 界面包和入口包完整测试、相关 race、vet、macOS 构建和 go mod verify；确认下载的模块源码与 Keel 提交逐字节一致。后续构建直接读取 go.mod 中的远端版本。测试日志、隔离会话副本和候选客户端位于 `/tmp/onecatch-scroll-anchor`；临时目录不保证长期保留。
