# seam — 本地 agent 操作远端代码的接缝

本地跑 Claude Code / Codex、让它们的工具作用在远端机器上，靠的是各家 harness
提供的一个"接缝"：Claude Code 是 `CLAUDE_CODE_SHELL_PREFIX`，Codex 是
`environments.toml` 里的 exec-server。

这两个接缝都没有稳定性承诺。它们随版本变，而**变了之后的失败是从 agent 内部
看不见的**：模型照常报告"我在目标机上执行了命令"，命令实际跑在用户自己的
机器上。这是这套设计能产生的最坏结果，也是这个包存在的唯一理由。

生产代码：

| | |
|---|---|
| `envelope.go` | 拆开一次 shell 调用，判断它该去哪台机器 |
| `session.go` | 跨调用共享的状态。harness 每次工具调用起一个新进程，`cd` 只能活在这个文件里 |
| `executor.go` | 本地 / SSH 执行，一次往返同时取回退出码和结果目录 |
| `dispatch.go` | 把上面三者接起来：解析 → 判路由 → 执行 → 回写 cwd |
| `execserver*.go` | Codex remote-environment JSON-RPC：远端进程与原生 `fs/*` |

调用链：

- Claude：`ClaudeRunner.Run`（`Request.Remote` 非空）→ `agentrun/remote.go`
  → `onecatchsh` shell prefix → `Dispatcher`。
- Codex：`CodexRunner.Run`（`Request.Remote` 非空）→ 私有 `CODEX_HOME` 的
  `environments.toml` → `onecatchsh exec-server` → SSH 命令与 SFTP `fs/*`。

这里没有挂载点，也不挂载远端文件系统。Agent 看到的是 harness 自己的远端
environment；Codex 的原生文件工具通过 JSON-RPC/SFTP 操作目标机，Claude 因原生
文件工具没有可重定向接缝而禁用它们，改走远端 shell。

其余是 conformance 套件：拿真实安装的 harness 验证接缝还是我们以为的形状。

## 怎么跑

```bash
go test ./internal/usecase/agentrun/...    # 离线单测，快，不碰 harness
task test:conformance                      # 加上真实 harness 验证
```

远端运行需要 `bin/onecatchsh`：`task build:shell`。开发时也可以用
`ONECATCH_SHELL_BINARY` 指向别处。conformance 套件自己编译它，不依赖先 build。

conformance 套件**不花任何 API 额度**：内嵌一个 mock model server，脚本化地让
harness 发起恰好一次工具调用。也**不需要远端机器**：recorder 拿到命令后在本地
执行——被测的是拦截的形状，不是执行的目的地。

harness 没装就自动 skip，所以挂 CI 是安全的。整套约 2 秒。

## 每个测试锁住了什么

| 测试 | 锁住的事实 | 它变红意味着 |
|---|---|---|
| `TestParse*` | 解析器对已抓获的真实信封仍然正确 | 解析器退化了 |
| `TestClaudeCodeShellSeam` | `-p` 模式认 shell prefix；argv 恰好 1 个元素；命令逐字 round-trip；`cd` 重定向还在 | Claude Code 换了封装形状，适配层必须跟着改 |
| `TestClaudeCodeDeniesLocalFileTools` | `permissions.deny` 仍然把 Read/Edit/Write/Glob/Grep 从模型的工具表里**删掉** | exec 模式不再安全，启动器必须拒绝启动 |
| `TestClaudeRunnerRunsCommandsOnTheTarget` | 整条链路：`ClaudeRunner` 配了 target 之后，模型的命令真的在 target 上执行，且本地文件工具没被提供给模型 | 接缝在某一环断了；错误信息会说断在哪 |
| `TestCodexExecServerSeam` | `environments.toml` 的 stdio(`program`) 路线仍然生效；命令以 `process/start` 到达；argv 是 `[shell, -lc, script]`；路径是 `file://` URI；输出能回到模型 | Codex 换了 exec-server 协议 |

## 已验证的事实（2026-08，claude 2.1.239 / codex 0.149.0）

这些是实测结论，不是从文档抄的。写适配层时按这个来：

**Claude Code**

1. `CLAUDE_CODE_SHELL_PREFIX` 在 `-p` 非交互模式下生效。
2. 调用形状是 `<prefix> <整个信封>` —— **单个 argv 元素，没有 `-c`**。按
   `bash -c <envelope>` 去写会直接挂。
3. 信封可能是**多行的**，中间会被插件的 SessionStart hook 注入 `export`
   语句（带本地用户名和插件目录）。因此解析器锚定在
   `eval '...' < /dev/null` 上，而不是逐段剥离已知前缀——后者遇到注入就漏。
4. 单引号转义用的是双引号夹心 `'"'"'`，不是 `'\''`。两种都要认。
5. `pwd -P >| <file>` 里的文件名**每次调用都不同**，必须每次解析，不能缓存。
   不复现这个文件，`cd` 就在调用之间静默失效。
6. Claude Code 自己的 shell 调用（hook、plugin 启动）**也走同一个 prefix**，
   且**不带信封**。把它们转发到远端会让每个 hook 都失败。

**Codex**

7. `environments.toml` 的 `program` + `args`（stdio）在 0.149 仍然有效。
   schema：`{id, url, program, args, env, connect_timeout_sec,
   initialize_timeout_sec}`，外层 `{default, include_local, environments}`。
8. 模型的命令以 `process/start` 到达，argv 为 `["/bin/bash","-lc",<脚本>]`，
   脚本部分原样保留。
9. **输出走服务端推送**：exec-server 线协议使用 `process/output` +
   `process/exited` 通知；app-server 再将前者转换成对 UI 的
   `process/outputDelta`。
   **不是** `process/read` 轮询。只实现轮询的话，codex 会等满
   `yield_time_ms` 然后告诉模型"Process running / Output:（空）"，
   接着 `process/terminate`——一个看起来在工作、只是什么都不返回的 agent，
   比直接报错难查得多。
10. 关掉 `unified_exec`（`-c features.unified_exec=false`）之后 0.149
    不再提供任何 shell 工具，所以 `exec_command` 是唯一路径。

## 还没定的事

- 输出推送的字段名（`processId` / `stream` / `chunk` / `exitCode`）是从二进制
  里挖出来再实测确认的，不是官方文档。codex 升级时第一个该怀疑它。
- 只覆盖了 stdio 传输。0.149 另有 WebSocket 路线
  （`codex exec-server forward --remote --environment-id --connect`），没测。
- Claude Code 会拿 cwd 文件的内容比对 allowed directories，远端路径不在列表里
  就会在每次工具结果后追加 `Shell cwd was reset to <本地工作区>`。启动时用
  `--add-dir <远端根>` 消除——实测本地不存在的路径也接受。这条没写成测试，
  因为它只影响模型看到的文本，不影响命令去哪执行。

## 升级 harness 之后

先跑 `task test:conformance`。红了就看是哪一条断言，对照上表判断影响范围。
**不要**因为测试红了就放宽断言——每一条都对应一个静默在用户本机上执行命令的
路径。
