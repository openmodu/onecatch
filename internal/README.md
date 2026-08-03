# `internal` 分层

`internal` 只保留六类职责：启动装配、领域模型、数据访问、业务用例、服务门面和传输适配。新增代码先判断职责，再选目录；不要因为某个包“大家都在用”就继续往顶层堆包。

正常请求路径如下：

```text
cmd → app → transport → service → usecase → repo → domain
```

`app` 是组合根，可以直接构造任意内层对象。其他层只能依赖自己或右侧的层，不能让 `domain`、`repo`、`usecase` 反向依赖 Wails、HTTP 或进程启动代码。

## 目录职责

```text
internal/
├── app/
│   ├── desktop/          Desktop 进程装配、Wails 生命周期和嵌入资源
│   ├── worker/           Worker 进程装配、参数解析和信号处理
│   └── server/           Server 的依赖注入定义
├── domain/               实体、值对象、状态机和领域校验
├── repo/
│   ├── store/            SQL 与 local-first Store 的创建和生命周期
│   ├── git/              Git 数据访问
│   ├── workspacelock/    Workspace 锁
│   └── ...               按领域拆分的持久化实现
├── usecase/
│   ├── agentrun/         Agent runtime 执行业务模块
│   ├── workflows/        Workflow 编排
│   └── ...               按业务能力拆分的用例
├── service/
│   ├── desktop/          Desktop 对外服务及运行状态
│   ├── worker/           Worker 对外服务
│   └── server/           Server 对外服务
└── transport/
    ├── http/             HTTP handler 与 DTO
    ├── router/           HTTP 路由适配
    └── wails/            Wails bindings
```

### `domain`

放不依赖存储和协议的业务对象与规则，例如 Workflow 状态机、Task 状态和 Settings 校验。这里不能读取文件、访问数据库、启动进程或引用其他上层目录。

### `repo`

只处理数据和外部状态：SQL、JSON/JSONL、本地文件、Git、Workspace 锁。Repo 返回领域对象，不负责“先查 A，再根据 A 创建 B”一类业务编排。

Repo 接口定义在使用它的 usecase 中，具体实现放在 `repo`。这样 usecase 依赖的是能力契约，不是 Gorm、文件布局或 Git 命令。

### `usecase`

一个 usecase 对应一项业务能力，可以组合多个 repo，也可以调用其他 usecase。例如 Workflow 执行会组合 TaskRepo、WorkflowRepo、Agent runtime、Git inspector 和 Workspace locker。

业务判断应落在这里。判断方法很直接：如果换掉 Wails、HTTP 或存储实现，这段规则仍然成立，它就不属于 transport、service 或 repo。

### `service`

Service 是进程对外提供的功能门面，负责组合多个 usecase、管理跨请求生命周期和运行期状态。Desktop 的取消句柄、事件 Hub、远端 Worker 连接等状态放在这里；Wails 和 HTTP 的协议细节不能进入 service。

新增功能先实现 usecase，再由 service 暴露。不要把新的业务规则直接写进 binding 或 handler。

### `transport`

Transport 只做协议转换：读取参数、创建 `context`、调用 service、转换返回值和错误。`transport/wails` 不持有业务状态，`transport/http` 不直接操作数据库。

### `app`

App 负责一次性装配和进程生命周期，包括配置、日志、依赖注入、信号处理、Wails Window 和 HTTP Server 启停。可复用逻辑必须下沉到 service、usecase 或 repo；`cmd` 只调用这里的 `Run`。

## 新代码归类判据

- 只描述业务对象和不变量：放 `domain`。
- 只读写某种数据或外部状态：放 `repo`。
- 组合 repo 完成一项业务目标：放 `usecase`。
- 组合多个 usecase 并对一个进程提供完整能力：放 `service`。
- 把 HTTP/Wails 输入输出转成服务调用：放 `transport`。
- 创建依赖、启动或停止进程：放 `app`。
