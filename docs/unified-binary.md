# OneCatch 单二进制设计

桌面安装包只交付一个业务可执行文件 `onecatch`。桌面、独立 Worker、远程 shell、
SSH askpass 和更新器共享代码与版本，但仍按需要启动独立进程。

## 入口

| 调用 | 行为 |
| --- | --- |
| `onecatch` | 打开桌面端 |
| `onecatch worker --pair` | 启动独立 Worker，输出配对码 |
| `onecatch worker --install-service …` | 安装启动命令为 `onecatch worker …` 的常驻服务 |
| `onecatch --help` | 显示入口帮助 |
| 内部角色 `shell` / `askpass` / `updater` | 执行对应协议，不进入桌面启动流程 |

`cmd/app` 是唯一入口。桌面平台在 `internal/app/command` 完成角色分发，
随后才决定是否调用 `desktop.Run`。iOS / Android 继续使用移动端装配，
不导入桌面角色。内部角色实现分别放在 `internal/app/shell`、`askpass`、
`updatehelper`；Worker 复用原来的 `internal/app/worker`。

## 为什么内部角色使用环境变量

Claude shell prefix 和 OpenSSH askpass 接受可执行文件路径，不能把
`onecatch shell` 当作路径传给它们。因此传入当前可执行文件路径，以
`ONECATCH_INTERNAL_MODE` 选择角色。Codex 的 exec-server 配置同时显式携带
角色和会话 ID，不依赖它是否继承父进程环境。

分发器读取后立即删除角色变量。shell 启动的 SSH 密码请求重新设置
`askpass`，避免继承为 shell；更新器启动的新版本也不会再次进入 updater。
未知角色以状态码 2 失败，缺失远程会话的 shell 继续以 125 退出，不能回退本地。
askpass 保持原来的系统凭据读取方式，stdout 只输出密码，密码不进入环境或参数。

环境变量是进程分发信息，不是认证凭据或权限边界。原有会话校验、凭据 ID 校验、
Worker TLS 与配对机制仍由各角色负责。

## 更新

Windows / Linux 更新时将自身复制到随机临时文件，以 updater 角色启动。
更新进程等待桌面退出，替换安装单元，启动新版本并等待 ready 标记；
失败时沿用原有回滚逻辑。角色分发不会把 GUI 初始化带进更新流程。

Linux AppImage 必须复制完整 AppImage，而不是镜像内的 ELF。临时副本通过
`APPIMAGE_EXTRACT_AND_RUN=1` 建立自己的运行环境，桌面退出和旧挂载消失后，
更新进程仍能访问 GTK/WebKit 等库。代价是更新时多复制一份完整镜像。
macOS 沿用 Wails 的自重启更新协议，本来就复用主程序。

Go 包初始化发生在入口分发之前，动态链接库也在启动前加载。统一可执行文件
不能消除 GUI 运行库依赖；各角色不会创建窗口，但不是纯静态程序。

## 构建与平台边界

`go tool wails3 task build` 构建桌面统一程序；各平台打包脚本不再构建、
复制或单独签名 Worker / shell / askpass / updater。
Windows 安装器自身、卸载器以及系统 WebView 运行库不在业务二进制合并范围内。

无界面服务器可执行 `go tool wails3 task build:headless`，得到
`bin/headless/onecatch`（Windows 为 .exe）。它仍支持 `onecatch worker`，
但通过 `onecatch_headless` 排除 GUI 装配，通过 `onecatch_worker` 排除
Modu 原生 SDK。桌面版本的独立 Worker 同样显式使用 Modu CLI，保持原有行为。
无界面产物是可选构建，不附带在桌面安装包中。

Windows 桌面产物保留 GUI subsystem，Worker / help 模式尝试连接调用者控制台，
保留显式传入的管道句柄。命令行自动化应显式等待进程或使用无界面构建；
GUI subsystem 程序的 shell 等待行为与 console 程序不同。

## 迁移

原来的 `onecatch-worker …` 命令改为 `onecatch worker …`。
已安装的 launchd / systemd 服务需要用新命令和原参数重新执行
`--install-service`，以更新可执行路径和子命令。数据目录、配对身份、
端口和服务名称保持原值，不自动重装用户已有服务。

旧 shell / askpass 独立入口不再构建。开发时取消旧的
`ONECATCH_SHELL_BINARY` / `ONECATCH_SSH_ASKPASS` 覆盖，让程序使用自身路径；
这两个覆盖仍供测试或自定义集成使用。升级前结束正在进行的远程任务，
因为旧任务的配置可能引用旧 helper 路径。

## 验证

启动级测试通过真实 app 入口重新执行测试程序，覆盖帮助、Worker、shell 拒绝
无会话执行、askpass 静默失败和未知角色。另检查角色不向子进程泄漏、
SSH 正确覆盖 askpass 角色、更新副本与当前可执行文件一致、
launchd / systemd 生成的启动命令包含 worker。

合并不改变远程协议；原有 shell / exec-server、SSH 凭据、Worker、更新器
和前端测试继续执行。真实 harness 的 conformance 测试改为构建统一入口。
macOS 可本机验证构建与签名，Windows / Linux 原生更新和安装仍需对应平台验证。

本次本机验证：全量 Go 测试、410 个前端测试、桌面与无界面入口测试通过；
更新角色成功替换测试副本并重启，Worker 实际启动后健康接口与配对正常。
macOS DMG / ZIP 打包及签名通过，ZIP 内只有一个业务可执行文件。
Windows 桌面版、Windows / Linux 无界面版交叉编译通过。

真实 harness conformance 中，当前 Codex 0.153.4 的 recorder 测试报告缺少
`seq`；用未修改的 HEAD 独立复现了相同失败。它是现有测试协议与 CLI 的兼容问题，
不属于本次入口改造。Claude CLI 未安装，相关真实 harness 测试跳过。
本机未安装 NSIS，Windows 安装器编译未执行；Linux AppImage 原生更新尚未验证。
