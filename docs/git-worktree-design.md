# Git 项目展示与 worktree 会话设计

状态：设计草案；已实现本机已有 worktree 选择和项目级自动创建。2026-09-23。

实现范围：输入区显示分支和未提交文件数，支持为本机已有 worktree 新建任务。项目可开启“新 session 自动创建 Git worktree”：首次发送时从项目 HEAD 创建独立分支和目录，继续原 session 复用绑定。新 session 可单次选择原目录、新建或已有 worktree；单次选择在成功创建任务后重置为跟随项目设置。关闭项目开关不改变已有任务。新目录不携带未提交改动和忽略文件，创建失败不会回退到项目目录；创建后发生附件或持久化错误时保留目录并返回其位置。目录绑定暂存于 Task.Worktree，统一解析供 Agent、Git、文件、终端和附件使用；保留项目归属，尚未建立独立 Repository/Workdir 存储。不同 checkout 的立即运行使用不同目录锁；项目队列仍保持原有串行顺序。目录丢失或 Git 目录身份变化会拒绝执行，历史记录仍可查看。

尚未实现：删除 worktree、远程 worktree 选择、会话迁移、外部分支漂移处理，以及侧栏分支标签。浏览器预览使用演示数据；桌面版本读取真实 Git。以下为完整目标设计。

建议保留现有 Workspace 作为用户添加的项目，在任务上增加固定的工作目录绑定。Git 项目显示当前任务的分支、改动数量和 worktree；新任务可以选择当前目录、已有 worktree 或新建 worktree。普通目录继续按现有方式工作。

本稿把“Claude Code 一样的效果”理解为分支旁提供 worktree 入口、会话在独立目录中执行；具体视觉样式尚未确认。Claude Code Desktop 的官方文档描述了这一交互，可作为行为参考：[并行会话](https://code.claude.com/docs/en/desktop#work-in-parallel-with-sessions)。以下数据结构、默认值和清理规则是 OneCatch 的设计建议。

## 现状与需要解决的问题

`internal/domain/workspaces/workspace.go` 的 Workspace 只有一个 Path；Task 通过 WorkspaceID 关联项目。`internal/service/desktop/gitops.go` 中的状态、diff、分支切换和提交都操作 Workspace.Path。Git Inspector 已有分支选择，输入区的 WorkspaceComposerMeta 只有项目和位置。

`internal/usecase/workflows` 同样使用 Workspace.Path 启动 Agent，`internal/repo/workspacelock/lock.go` 按 WorkspaceID 加锁。因此，直接给 Git 面板增加 worktree 下拉框，会出现面板查看目录 A、Agent 修改目录 B 的风险；仅替换全局 Workspace.Path，又会改变其他任务恢复执行的位置。

现有 Git 命令能在合法 linked worktree 中运行，但没有仓库归组、worktree 枚举、任务目录绑定与生命周期管理。实现要补齐这些能力。

## 用户看到什么

输入区保留项目和执行位置，再增加一行紧凑状态：

```text
oneshot  ·  本机
feature/login ▾  ·  Worktree: login ▾  ·  3 个文件改动
```

分支和 worktree 是两个独立操作。分支菜单表示“切换此目录的分支”；worktree 菜单表示“选择另一个工作目录”。分支已被其他 worktree 使用时，显示占用目录并提供“在该目录新建任务”，不强制 checkout。

| 位置 | 展示与行为 |
| --- | --- |
| 项目侧栏 | 保留项目 → 任务两层，任务附带分支；linked worktree 加标签。首版不再增加一层目录树 |
| 新任务输入区 | 工作目录可选“当前项目目录 / 已有 worktree / 新建 worktree”；默认当前目录，记住用户明确选择的模式 |
| 新建 worktree | 展示来源分支或提交、新分支名、目标位置；首次发送时创建，取消草稿不留下目录 |
| 已开始的任务 | 显示实际绑定目录；首版不提供原地迁移，提供“在另一目录新建任务” |
| Git 面板 | 默认跟随任务，标明主机、目录、分支；查看其他来源时明确标记，写操作仅对当前任务目录开放 |
| 终端和文件面板 | 新建终端、文件读取和编辑跟随任务目录；已有终端保持原目录并显示其归属 |
| 普通目录 | 不显示分支与 worktree 控件；探测失败显示“Git 状态不可用”，不能误报为普通目录 |

主 worktree 标记为“主工作目录”，不要把它的分支硬编码成 main。没有 upstream 时不显示同步计数；detached HEAD 显示短提交号；尚无首次提交时显示“暂无提交”，首版禁用新建 worktree。

切换当前目录分支会影响使用该目录的所有任务。应用内有执行中的任务时禁止切换；有本地改动时先处理改动。外部终端仍可能改变分支，因此启动和恢复运行前要检查 HEAD、分支和目录身份，变化时暂停并让用户选择继续或另建任务。

## 数据模型与目录身份

保留 WorkspaceID，新增 Workdir，而不是把每个 worktree 注册成顶层项目。一个 Workspace 可以关联多个 Workdir，一个 Workdir 可以承载多个任务；同时写入受目录锁约束。

| 对象 | 建议字段与职责 |
| --- | --- |
| Workspace | 现有 ID、Path、RemoteFS 保留；Path 表示用户添加的入口，不随任务切换 |
| Repository | 稳定 ID、executionTargetID、commonDir；表示某台执行主机上的一份仓库 |
| Workdir | 稳定 ID、repositoryID（可空）、rootPath、gitDir、kind、ownership、health；kind 区分普通目录、主 worktree、linked worktree |
| WorkspaceWorkdir | workspaceID、workdirID、relativeCwd；支持用户打开仓库子目录，而不把 cwd 扩大到仓库根 |
| Task | 新增 workdirID；首次运行前固定绑定，WorkspaceID 仍用于项目归属 |
| Run | 保存 executionTargetID、workdirID、resolvedCwd、起始 HEAD/branch 和可选 baseCommit；历史记录不随实时分支变化 |

Repository 的发现键使用执行目标身份 + 规范化 commonDir，Workdir 使用目标身份 + 规范化 Git 私有目录或普通目录路径。发现键用于匹配持久化 ID，不用分支名、仓库名或 remote URL 作为身份。不同 clone 即使 origin 相同也不合并；相同路径在不同主机上也不合并。SSH 别名首版允许作为不同目标，只有明确验证或配置后才归并。

使用 Git 自己解析仓库位置：`rev-parse --show-toplevel`、`--absolute-git-dir`、`--path-format=absolute --git-common-dir`，并用 `worktree list --porcelain -z` 枚举目录。最低 Git 能力要在实现时检测，不支持时保留普通 Git 展示并禁用管理功能。不能假定 `.git` 是目录。NUL 分隔解析需要覆盖空格、中文和换行路径，以及 locked、prunable、detached、bare 状态。[Git worktree 文档](https://git-scm.com/docs/git-worktree#_list_output_format)

用户从 linked worktree 或仓库子目录添加项目时仍可发现仓库关系，但保留其项目入口、名称和子目录范围。发现到另一个已注册项目时只提示关联，不自动合并历史任务。移动后的目录通过 Git 元数据验证后更新路径；无法验证时标记 missing，不用同名目录替换。

## 统一解析执行上下文

新增 `ResolveTaskContext(taskID)`，返回目标主机、仓库根、workdir 根、实际 cwd、权限范围和远程配置。Agent、Git、终端、文件访问、ReviewChanges、提交信息生成、附件和运行恢复都使用该结果。修改 Remote FS 路径时，派生配置的 Path 与 RemoteFS.Root 必须一致，不能污染项目原配置。

任务相关 API 传 taskID 或服务端签发的 contextID，由后端验证归属并解析目录，不接受前端任意传入绝对路径。项目级旧接口可暂时保留给未绑定任务的视图，不能用于已隔离任务的写操作。绑定生成代码随服务端接口一起更新。

锁拆为两类：运行写锁按 executionTarget + 规范化 workdir 根识别，同一目录即使通过不同项目或子目录导入也互斥，不同 worktree 可以并行；创建、删除和共享引用变更另用 Repository 范围的短锁。所有入口使用相同锁序。Git 自身的锁仍是最后一道并发约束，应用锁无法控制外部终端。跨客户端远程运行需要执行端租约，不能沿用本机 PID 判断远程任务存活。

前端状态缓存按 contextID 分开。切换任务时清空旧结果，用请求序号丢弃迟到响应；首次进入、窗口恢复焦点、运行完成和 Git 操作完成时刷新。活动目录增加低频刷新或文件事件合并，后台项目不逐个高频扫描。分支、目录身份和轻量状态先加载，完整 diff 按需读取。

## 创建、恢复和回收

创建由应用统一管理，再把目录交给所选 Agent；不要同时让 Claude CLI 再创建第二层 worktree。这样不同 Agent 使用同一套目录语义。

1. 首次发送前，在所选执行目标验证 Git 能力、来源提交、目标位置及分支占用。来源解析成不可变提交；用户选择已有分支时重新检查占用。
2. 默认从选定提交创建，不复制当前目录的未提交改动。在发送前显示这一规则；携带 dirty changes 留作后续显式功能。
3. 先持久化带幂等键的创建记录，再执行 `git worktree add`。新分支建议使用可配置的 `onecatch/<slug>-<short-id>`，目录使用独立 ID，避免把分支名直接拼接成路径。
4. 创建成功后记录归属，再绑定任务并启动。状态至少区分 creating、ready、failed、missing、removing；崩溃后重试先检查 Git 记录和路径，不能重复创建或静默降级到项目原目录。

默认目录建议放在应用数据目录下的 `worktrees/<repository-id>/<workdir-id>`，提供仓库级位置覆盖。这样不会把新目录放进用户源码树；代价是部分依赖相对父目录的工具需要用户另选位置。远程目录必须在远程执行目标上创建。

新 worktree 不自动复制 `.env`、ignored 文件和依赖目录，也不自动执行仓库初始化脚本。用户明确配置的初始化流程可以后续加入。submodule、LFS、稀疏检出需要独立验收；首版对未验证的组合明确提示能力限制，保留当前目录工作方式。

归档任务只改变任务状态，默认保留目录。清理应用创建的 worktree 时，列出受影响任务、未提交文件、未跟踪及 ignored 文件、独有提交、推送状态和活动终端；信息未知时不判定为可安全删除。干净工作区不代表没有待保留提交。使用普通 `git worktree remove`，失败时展示原因，首版不自动强制删除或删除分支。外部创建的 worktree 只取消应用关联，不删除目录；主工作目录永不回收。

任务恢复到 missing 目录时展示“目录已移除”并保留历史记录，提供定位目录或新建任务。prunable 只作为 Git 诊断展示，不在刷新时自动 prune；locked 不自动解锁。

## 实施顺序与验收

第一阶段完成识别、状态展示、Workdir 数据模型和统一上下文，支持为已有 worktree 新建任务。旧任务在首次使用时绑定原项目目录，迁移幂等且不改变 cwd。验收 Agent、文件面板、终端、Git 和恢复行为都指向同一目录之后，再开放创建入口。

第二阶段加入本地 worktree 创建、幂等恢复、清理预览和目录锁。Remote FS 与 Worker 独立 clone 必须返回各自 capability；未完成对应执行端协议前只开放可用操作，不复用本地路径执行。第三阶段补远程创建及执行端租约、显式迁移、改动携带和初始化配置。

| 必须覆盖的场景 | 验收结果 |
| --- | --- |
| 普通目录、普通仓库、linked worktree、仓库子目录 | 识别准确；保留原项目 cwd 和访问范围 |
| 两个任务使用不同 worktree | 可以并行写入；终端、文件、Git 状态和 Review 各自一致 |
| 两个项目指向同一目录或其不同子目录 | 写锁互斥；切分支提示影响范围 |
| 分支已占用、未提交改动、detached、无 HEAD | 入口和提示符合规则，不强制覆盖 |
| 外部切分支、删目录或移动 worktree | 刷新并检测漂移，恢复不误入主目录 |
| 快速切换任务，旧请求后返回 | 不串分支、不串 diff、不向旧目录提交 |
| 创建时崩溃、重复请求、持久化失败 | 可重试恢复；保留可诊断记录，不清理未知目录 |
| 归档、外部 worktree、未推送提交、ignored 文件 | 归档不删目录；清理不丢文件或提交 |
| SSH 与本地同名路径、Worker clone | 身份隔离；不以本地状态代替远程状态 |

首版不实现完整 Git 客户端、自动合并回主分支、会话原地迁移或自动磁盘回收。这些功能引入额外的冲突处理、会话恢复和数据保留语义，应在目录绑定可验证后分别设计。
