# Oneshot Agent Marketplace Prototype

这是 Oneshot 的 Agent 服务市场桌面化原型，当前用于验证产品信息架构和核心交互，不是生产实现。

## 当前范围

- 桌面化三段式布局：左侧导航与账号区，中间任务工作区，右侧可折叠 inspector。
- 模拟登录：微信授权登录、Google 邮箱登录、退出登录。
- Agent 市场：分类筛选、Agent 切换、详情展示。
- 按使用次数计费：购买次数、单次扣减、余额展示、使用记录。
- 订单闭环：需求填写、扣次确认、支付模拟、执行进度、订单信息。
- 交付物：报告预览、下载/分享反馈。

## 本地运行

```bash
npm install
npm run dev
```

默认本地地址：

```text
http://127.0.0.1:5173/
```

构建验证：

```bash
npm run build
```

## 目录说明

```text
prototype/
  src/
    App.jsx              # 单页交互原型
    styles.css           # 桌面化三段式视觉样式
    assets/              # 原型使用的参考图和交付物预览图
  DEVELOPMENT.md         # 原型开发规范
  IMPLEMENTATION.md      # 后续 Go server + Wails v3 落地说明
  PROGRESS.md            # 过程记录
  design-qa.md           # 视觉 QA 状态
```

## 重要产品决策

- 收费模型是按使用次数收费，不是订阅套餐或项目制报价。
- 一次 Agent 执行扣减 1 次。
- 登录入口固定在左下角账号区。
- 整体体验以桌面工作台为主，优先接近 Codex 桌面版的三段式结构。
