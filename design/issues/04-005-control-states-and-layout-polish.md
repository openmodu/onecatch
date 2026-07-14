# Issue 04-005：控件状态补全与布局美学优化

状态：进行中

## 来源

- 技术方案：`design/docs/04-mac-ui-component-system.md`
- 前置修复：`design/issues/04-003-action-hover-contrast.md`
- 依据：实测浏览器控件状态审计（CSSOM 扫描 + 浅色/深色截图）与布局评审。

## 目标

补齐交互控件缺失的状态反馈，修正遗留按钮的语义色，并按设计评审结论优化 Tasks / Workflow / Settings / Inspector 的布局节奏。

## 范围

### 控件（6 项）

1. 全应用按钮增加按下（`:active`）反馈——此前仅 `.dag-node-card` 有。
2. 显式定义 `::placeholder` 颜色（`--acp-text-soft`），替换 WebKit 默认灰，保证深浅主题一致。
3. Composer 与恢复指令输入支持 Cmd/Ctrl+Enter 提交。
4. WorkerPage 迁移到 Action/StatusBadge primitives；遗留 `.text-button.danger` 等 hover 修为语义色背景（与 04-003 同理）。
5. ToggleRow 增加 hover 反馈。
6. 补齐 DAG 连接端口等图标按钮的 aria-label。

### 布局（6 项）

1. Tasks 页渐进披露：未选中 Run 时不渲染右侧 Inspector 列，composer 与列表用全宽；全宽时输入控件限宽保证行长。
2. 删除与列表头计数重复的“已显示 N 条”底栏，仅在加载中/可继续加载时显示状态行。
3. Settings 内容列限宽（≤820px），避免字段与说明拉满全宽。
4. Inspector 事件流节奏：同秒时间戳去重；tool use / file change 行降一级视觉权重；会话块之间加入间隔建立章节感。
5. Composer 标题输入改为无盒底线样式，与描述框拉开层级。
6. Workflow 页 hero 压缩：口号式大标题改为工具页语气，缩小页头留白。

## 非目标

- 不改变任何业务行为、binding 或数据模型（Cmd+Enter 仅复用现有提交入口）。
- 不调整 DAG 编辑器画布交互。

## 验收标准

- [ ] 按钮按下有可感知的明度变化；placeholder 双主题下可读。
- [ ] Cmd+Enter 可从标题/描述/恢复指令输入直接提交。
- [ ] WorkerPage 无遗留按钮类；危险操作 hover 为 danger 背景。
- [ ] 未选中 Run 时 Tasks 页为全宽单列；选中后 Inspector 出现。
- [ ] Settings 字段列限宽；Workflow hero 高度收敛。
- [ ] 事件流中相同秒时间戳只显示一次；tool 行视觉权重低于 agent 消息。
- [ ] frontend production build 通过；浅色/深色截图核验。

## 交付记录

（完成后补充）
