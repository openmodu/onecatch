# Issue 04-008：DAG 顶部 TUI 操作区

状态：已完成

## 来源

- PRD：`design/prd/04-mac-desktop-ui-redesign.md`
- 技术方案：`design/docs/04-mac-ui-component-system.md`

## 目标

修复 DAG 顶部“节点”和“保存”按钮拼接、挤压和错位问题，让编辑器操作区保持纯文本 TUI 风格。

## 范围

- 自动布局、添加节点和保存 DAG 合并到同一个 toolbar action slot。
- 移除旧 editor footer 的绝对定位覆盖。
- 添加节点和删除节点不使用图标。
- 操作之间保持固定间距且不参与 flex 压缩。

## 非目标

- 不改变 DAG 校验、节点布局或保存协议。
- 不修改画布连接点和边交互。

## 产品需求

- 操作显示为 `[ 自动布局 ]`、`[ 节点 ]`、`[ 保存 DAG ]`。
- 三个按钮必须有清晰间距，不得共享背景或文字重叠。
- Inspector 删除动作显示为 `[ 删除节点 ]`，不使用关闭图标。

## 技术设计

- 前端：DAG JSX 改用共享 `Action`；新增 `dag-actions` flex slot。
- CSS：toolbar 右侧恢复 20px gutter，action gap 10px，设置 `flex: 0 0 auto`。
- 后端、数据模型、binding、错误码：无改动。

## 验收标准

- [x] 三个 toolbar action 文本完整且互不重叠。
- [x] action 之间保持 10px 间距，最右侧保持 20px gutter。
- [x] 节点新增和删除操作不使用图标。
- [x] DAG 画布、Inspector 和保存行为保持可用。

## 测试计划

- Browser：打开 DAG editor，测量三按钮 bounds 与间距。
- Build：frontend test/build 与 Wails production build。

## 交付记录

- 1280px Browser 实测三个 action 宽度约 110/86/119px，间距均为 10px，最右边界为 1260px（20px gutter）。
- 删除节点改为危险语义的共享 `Action`，保存入口不再依赖独立 editor footer。
