# 仓库协作规范

本仓库采用“PRD -> Issue -> 设计 -> 开发 -> 验收”的交付方式。后续产品设计、技术设计、编码、测试和交付记录都应沉淀在 repo 内，避免只存在于聊天上下文。

## 1. 文档目录

### 1.1 PRD

PRD 放在 `design/prd/` 目录。

命名规则：

```text
design/prd/00-agent-marketplace-mvp.md
design/prd/01-auth-and-session.md
design/prd/02-billing-and-payment.md
```

规则：

- PRD 使用两位数字编号，从 `00-` 开始。
- 一个 PRD 描述一个完整产品范围或阶段性能力。
- PRD 应包含背景、目标、非目标、用户流程、功能需求、数据对象、接口边界、验收标准和待确认问题。
- PRD 是需求来源，不直接作为开发任务。开发任务必须拆成 issue。

### 1.2 Issue

Issue 放在 `design/issues/` 目录。

命名规则：

```text
design/issues/00-001-auth-login.md
design/issues/00-002-agent-catalog.md
design/issues/00-003-usage-billing.md
```

规则：

- 文件名前缀第一段对应 PRD 编号，例如 `00-001` 表示来自 `design/prd/00-...`。
- 第二段为该 PRD 下的 issue 序号，从 `001` 开始。
- 文件名后半段使用短英文 slug，便于 Git 和命令行使用。
- 一个 issue 必须能独立设计、开发、测试和验收。
- 不允许把多个无关能力塞进同一个 issue。

## 2. Issue 模板

每个 issue 文件应使用以下结构：

```md
# Issue 00-001: 标题

状态：草稿 | 待开发 | 开发中 | 阻塞 | 已完成 | 已取消

## 来源

- PRD：`design/prd/00-agent-marketplace-mvp.md`
- 相关原型：`design/prototype/`

## 目标

描述这个 issue 需要交付的明确结果。

## 范围

- 本次包含的事项 1
- 本次包含的事项 2

## 非目标

- 明确不在本次处理的事项

## 产品需求

- 用户可见行为
- 状态变化
- 错误处理

## 技术设计

- 后端改动
- 桌面端 / Wails 改动
- 前端改动
- 数据模型改动
- API 或 binding 改动

## 验收标准

- [ ] 可观察的验收项
- [ ] 可观察的验收项

## 测试计划

- 单元测试
- 集成测试
- 手动验证

## 交付记录

记录实现决策、取舍、后续任务和验证结果。
```

## 3. 工作流

### 3.1 新需求

1. 先确认是否已有对应 PRD。
2. 如果没有，先在 `design/prd/` 新增或更新 PRD。
3. 从 PRD 拆出一个或多个 `design/issues/` 文件。
4. 每个 issue 必须有清晰验收标准后再开始实现。

### 3.2 开发

开发时以 issue 为最小交付单元。

要求：

- 开始前阅读对应 PRD 和 issue。
- 设计变更先更新 issue 的“技术设计”。
- 范围变化先更新 issue 的“范围”或“非目标”。
- 实现完成后更新“交付记录”。
- 验证完成后更新“测试计划”的实际结果。

### 3.3 验收

交付时必须能回答：

- 对应哪个 PRD。
- 对应哪个 issue。
- 完成了哪些验收标准。
- 跑了哪些测试或手动验证。
- 有哪些未完成事项或后续 issue。

## 4. 编号规范

- PRD 编号：`00`、`01`、`02`。
- Issue 编号：`<PRD编号>-<三位序号>`，例如 `00-001`。
- 同一个 PRD 下 issue 序号递增，不复用。
- 如果一个 issue 被废弃，保留文件并标记状态，不回收编号。

## 5. 状态规范

Issue 文件顶部使用中文状态：

```md
状态：草稿 | 待开发 | 开发中 | 阻塞 | 已完成 | 已取消
```

状态含义：

- `草稿`：范围尚未确认。
- `待开发`：可以进入设计或开发。
- `开发中`：正在实现。
- `阻塞`：被外部问题阻塞。
- `已完成`：已实现并验收。
- `已取消`：不再执行。

## 6. 交付原则

- PRD 管“为什么做、做什么、验收什么”。
- Issue 管“这次交付什么、怎么做、如何验证”。
- 代码提交应尽量对应一个 issue。
- 大 issue 应拆小，不用一个提交解决整个 PRD。
- 原型里的 mock 行为不能直接视为生产规则，生产规则必须写入 PRD 或 issue。
- 涉及登录、支付、扣次、订单、交付物的实现必须有明确验收标准和测试计划。

## 7. 后端架构硬约束

后端采用改良 DDD，后续所有实现必须遵守下面的依赖方向。

### 7.1 分层职责

- `pkg/`：基础能力封装，例如 `pkg/sql` 封装 MySQL/GORM。这里不能出现具体业务语义。
- `internal/data`：底层资源初始化和生命周期管理，例如 SQL、Redis、对象存储等连接的创建、关闭和组合。
- `internal/repo`：业务数据封装，例如用户数据、订单数据、Agent 数据、计费流水数据。
- `internal/usecase`：领域业务组合，例如下单依赖用户、订单、计费等 repo。
- `internal/service`：上层应用能力组合，对外暴露 API 所需的服务集合。
- `internal/api` 和 `internal/transport`：API 抽象、路由、请求解析和响应输出。

### 7.2 依赖方向

允许的核心依赖方向：

```text
transport/api -> service -> usecase -> repo -> pkg
data -> pkg
data -> repo
```

规则：

- `repo/*` 不要依赖 `internal/data`，避免把生命周期管理层塞进业务数据实现里。
- `repo/*` 可以直接依赖 `pkg/sql.Sql`、Redis client 等基础资源句柄。
- `internal/data` 可以负责创建资源句柄，并可以聚合 `repo/*` 中定义的业务 repo 接口。
- `usecase` 只依赖接口，不依赖 repo 的具体实现。
- `service` 依赖 usecase，对外组合 API 能力，不写数据访问逻辑。

### 7.3 Repo 写法

每个业务 repo 包自己定义接口和实现：

```go
type AgentsRepo interface {
    // business data methods
}

type agentsImpl struct {
    sql *pkgsql.Sql
}

func NewAgentsRepo(sql *pkgsql.Sql) AgentsRepo {
    return &agentsImpl{sql: sql}
}
```

规则：

- 接口定义放在对应业务 repo 包内，例如 `internal/repo/agents` 定义 `AgentsRepo`。
- 实现结构体使用未导出命名，例如 `agentsImpl`、`ordersImpl`、`billingImpl`。
- 构造函数返回接口，不把实现类型暴露给上层。
- 不要在 `internal/data` 里重复定义 `AgentRepo`、`BillingRepo`、`OrderRepo` 这类接口。
- 业务数据封装必须在 `internal/repo/*`，不要放到 `internal/data`。

### 7.4 Data 写法

`internal/data` 只做资源和生命周期管理。

可以有：

```go
type Data struct {
    Sql   *pkgsql.Sql
    Redis *pkg.Redis
}
```

可以有：

```go
type OneShotRepo struct {
    Agents  agents.AgentsRepo
    Orders  orders.OrdersRepo
    Billing billing.BillingRepo
}
```

前提是 `repo/*` 不 import `internal/data`，否则会形成循环依赖。

不允许：

- 在 `data` 里写用户、订单、Agent、计费等业务查询方法。
- 在 `data` 里放 `memory.Store` 这类混合业务数据访问的结构。
- 在 `data` 里重复声明业务 repo 接口。

### 7.5 Wire 规则

- Wire 文件放在入口同级，例如 `cmd/oneshot-server/wire.go`。
- Wire 负责组装 `pkg` 资源、`data` 生命周期、`repo`、`usecase`、`service`。
- 不创建全局 `internal/di`。

## 8. 当前已存在文档

- MVP PRD：`design/prd/00-agent-marketplace-mvp.md`
- 项目结构设计：`design/docs/PROJECT_STRUCTURE.md`
- 原型说明：`design/prototype/README.md`
- 原型本地规范：`design/prototype/AGENTS.md`

根目录 `AGENTS.md` 约束整个仓库；子目录中的 `AGENTS.md` 可以补充局部规则，但不能覆盖 PRD 和 issue 的交付规范。
