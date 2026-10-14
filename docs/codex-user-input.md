# Codex 提问与回答

OneCatch 支持 Codex app-server 的提问请求和异步消息问题。收到问题后展示选项与自由输入；提交成功后，普通回答保留在对话记录中。跳过、服务端关闭和回答成功是不同结果，不能把未回答当成选择了第一项。

## 协议与回答路径

本次依据本机 `codex-cli 0.162.1` 的 `app-server generate-ts --experimental` 导出类型实现。在线参考为 [Codex App Server](https://learn.chatgpt.com/docs/app-server#toolrequestuserinput)。协议中的实验字段可能随版本变化。

| 输入 | 回答方式 |
| --- | --- |
| `item/tool/requestUserInput` | 使用原 JSON-RPC ID 返回 `answers: { questionId: { answers: [text] } }` |
| `agentMessage.questions`，通常伴随 `delivery: "async"` | 根据消息与题目生成卡片，回答通过带 `expectedTurnId` 的 `turn/steer` 送入当前轮次 |

`isBlocking` 决定卡片是否显示“等待你的回答”。字段缺失时按阻塞请求兼容旧版本；不根据已弃用的 `autoResolutionMs` 自动选择答案。异步消息题目使用 `title` 和可空的 `options`，提问 RPC 使用 `id`、`header`、`question`、`options` 和 `isSecret`。

等待用户时不能阻塞协议读取。每个 app-server 连接只有一个读取协程，当前轮次负责串行写入与发布事件；回答处理器在独立协程中等待。异步回答收到 `turn/steer` 成功响应后才报告提交成功。提问 RPC 没有客户端响应的独立确认，写入成功后记录回答。

## 生命周期

- 请求 ID 包含 Codex thread、turn 和原始请求或消息 ID，防止跨轮次冲突；重复消息快照不重复展示卡片。
- 服务端发送 `serverRequest/resolved` 时按 thread 与 request ID 清理对应等待者；另一会话的通知不影响当前请求。
- 轮次完成、失败、取消或连接关闭后，当前轮次未回答的卡片关闭，迟到的回答不能送往下一轮。异步卡片目前也只在所属轮次内有效，结束后可通过普通输入框继续对话。
- 用户跳过 RPC 问题时回传空 `answers`；跳过消息问题只关闭卡片，不发送一条伪造回答。
- 回答提交前校验每题非空；请求关闭或回答提交后拒绝重复提交。服务端拒绝 `turn/steer` 时向用户返回错误，不能记为成功回答。
- `isSecret` 使用密码输入控件，答案会送往 Codex，但不会写入 OneCatch 的回答事件或显示在回答历史中。Codex 自身是否持久化不由 OneCatch 控制。

本地、远端 HTTP worker 和直接连接沙箱的 Codex 共用协议适配器。远端 HTTP worker 通过 `/v1/runs/{runID}/user-input/{requestID}` 接收答案；需要同时升级 worker 与桌面端。

## 回归入口

- `internal/usecase/agentrun/codex_questions_test.go`：双向协议流，覆盖阻塞与非阻塞请求、持续接收消息、异步回答确认与拒绝、服务端关闭、轮次结束、重复快照、无效及过期请求、跳过与秘密答案。
- `internal/service/desktop/runtime_permission_test.go`：回答完整性、跨 run 拒绝、传输确认和关闭竞争。
- `internal/service/worker/user_input_test.go`：通过真实 HTTP 流往返回答，覆盖带斜杠的请求 ID、跨 run、重复与不完整回答。
- `internal/app/keel/transcript_test.go`：先收到回答，再补历史快照时，已关闭卡片不重新出现，回答只保留一份。

界面继续使用原有对话文档同步入口，不直接改写已加入选区组的文档。选区回归要求见 [对话选区与后台更新](keel-conversation-selection.md)。协议测试与界面布局测试不能代替实际客户端验证。

本次已通过运行器、桌面服务、worker 和 Keel 界面包测试；新增协议、回答状态及 HTTP 往返测试通过 `-race`。`TestQuestionCardRequiresAnswersAndPreventsDuplicateClicks` 使用 Gio 原生指针路由验证选择选项、必填校验和提交期间禁用控件。未在实际运行的 OneCatch 窗口中操作验证，也未用真实模型触发提问。
