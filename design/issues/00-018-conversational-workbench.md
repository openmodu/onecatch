# Issue 00-018: 会话式工作台

状态：已完成

## 来源

- 产品方向：工作台从「分类 + 流程标签 + 扣次面板 + 进度时间线 + 交付面板」并列改为对话式：选 agent → 对话提交任务 → 订单流程（扣次确认、进度、交付）在对话内呈现。

## 目标

把工作台改为聊天式：选择一个 agent，进入会话，用对话提交任务；扣次确认、执行进度、交付物都作为会话里的消息/卡片出现。

## 范围

- 后端新增会话（conversation）与消息（message）模型、仓储、用例、接口。
- 复用现有订单/计费/交付后端：确认扣次时调用现有 `Orders.Create`，不重复实现下单与计费。
- 前端工作台重写为「agent 选择 + 会话线程 + 输入框」，保留 `我的订单 / 用量账单 / 账户` 导航。

## 非目标

- 不接入真实 LLM：agent 回复目前是模板规则，worker 仍为占位执行。后续接 LLM 只替换 agent 回复生成。
- 不实现会话历史侧栏、重命名、删除等管理功能。
- 不改动订单状态机、计费一致性、交付物生成逻辑。

## 产品需求

- 选中 agent 后进入会话，agent 先发一条招呼消息。
- 用户用消息描述任务；确认扣次前可继续修改（多轮）。
- agent 在收到任务后回一张「确认扣次」卡片（本次扣减次数 + 当前余额）。
- 用户在会话内点「确认并支付」才真正下单扣次。
- 下单后会话进入执行态，进度与交付物在会话内可见。

## 技术设计

- 领域 `conversations`：
  - `Conversation{ID, UserID, AgentID, AgentName, Status, OrderID, PendingRequirement, CreatedAt, UpdatedAt, Messages}`。
  - `Message{ID, ConversationID, Role(user|agent|system), Kind(text|checkout), Text, CreatedAt}`。
  - 状态：`active -> awaiting_confirm -> running`。
- 仓储 `repo/conversations`：内存 + SQL 双模式，启动 `Migrate`；按 `userID` 鉴权读取，跨用户返回 not found。
- 用例 `usecase/conversations`：
  - `Start(userID, agentID)`：建会话 + 招呼消息。
  - `PostMessage(userID, convID, text)`：追加用户消息；agent 模板回复 + checkout 卡片，暂存 `PendingRequirement`，状态置 `awaiting_confirm`。
  - `Confirm(userID, convID)`：用 `PendingRequirement` 调 `Orders.Create`（真实扣次），链 `OrderID`，状态置 `running`，追加系统消息。
  - 进度与交付物不写入消息：会话携带 `OrderID`，前端用现有 `GET /api/orders/{id}` 与 `/artifacts` 缝入对话线。
- 接口（用户端，Bearer 鉴权）：
  - `POST /api/conversations` `{agentId}`
  - `GET /api/conversations/{id}`
  - `POST /api/conversations/{id}/messages` `{text}`
  - `POST /api/conversations/{id}/confirm`
  - 响应使用会话 DTO，不返回 `userId`、`pendingRequirement` 等内部字段。
- 桌面：`clients/oneshot` 增加会话方法；`bindings` 暴露 `StartConversation/GetConversation/PostMessage/ConfirmCheckout`，仅经 `/api/*`。

## 验收标准

- [x] 选 agent 进入会话并收到招呼消息。
- [x] 发送任务后收到「确认扣次」卡片，确认前可多轮修改。
- [x] 仅在确认后才下单扣次（复用现有计费，扣减一致）。
- [x] 下单后会话内显示进度与交付物。
- [x] 会话按当前用户鉴权，跨用户访问返回 404。
- [x] 会话响应不泄露 `userId`、token 等隐私字段。

## 交付记录

- 2026-06-13 实现：
  - 后端：`domain/conversations`、`repo/conversations`（双模式 + `Migrate` + 消息 `Seq` 排序）、`usecase/conversations`（Start/PostMessage/Confirm，确认才走 `Orders.Create` 扣次）。
  - 接口：`POST /api/conversations`、`GET /api/conversations/{id}`、`POST /api/conversations/{id}/messages`、`POST /api/conversations/{id}/confirm`，会话 DTO 过滤 `userId`/`pendingRequirement`，错误映射 401/404/400/409。
  - 桌面：`clients/oneshot` 会话方法 + `ConversationBinding`，wails3 重新生成前端 bindings。
  - 前端：工作台重写为 agent 选择 + 会话线程 + 输入框；checkout 卡片确认；进度/交付物由现有 order/artifacts 缝入；删除 flow-tabs/brief-grid/checkout-panel 等旧面板，新增聊天样式。
  - 测试：会话用例（确认才扣次/跨用户隔离/空消息）、transport（未登录 401、跨用户 404、DTO 无 `userId`、无暂存确认 409）；`go test ./...`、`-race`、vet、前端 build 全过；浏览器预览验证 选 agent → 对话 → 确认 → 进度 全流程。
  - 局限：agent 回复为模板规则，worker 仍为占位执行；接 LLM 时只替换 `usecase/conversations` 的回复生成。

## 测试计划

- 会话用例测试：start/post/confirm 流转、确认才扣次、跨用户拒绝。
- transport 测试：未登录 401、跨用户 404、会话 DTO 无隐私字段。
- 前端：浏览器预览验证选 agent → 对话 → 确认 → 进度/交付。
