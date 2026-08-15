# oneshotfs：把 Linux 项目挂载到 macOS

`oneshotfs` 让 macOS 程序通过普通文件 API 直接访问 Linux 项目。Mac 运行 Codex 和 `oneshotfs`；Linux 只提供 OpenSSH 的 SFTP 子系统，不需要安装 Codex、Oneshot 或 FUSE。

```text
Codex / 编辑器 / Finder
          │ macOS 文件 API
        macFUSE
          │
      oneshotfs
          │ 一条持久 SSH/SFTP 连接
       Linux 项目目录
```

文件内容始终留在 Linux。打开文件后，读写请求直接走 SFTP；Mac 只缓存文件属性和目录列表，默认有效期为 1 秒。这不是 Mutagen 式双向同步，没有本地项目副本，也不能离线工作。

## 准备环境

Mac 需要安装 [macFUSE](https://macfuse.github.io/)，Linux 需要运行带 SFTP 子系统的 OpenSSH Server。先确认下面的命令不会要求输入密码或二次确认：

```bash
ssh -o BatchMode=yes dev-linux true
```

`oneshotfs` 调用 Mac 自带的 `ssh`，因此会继承 `~/.ssh/config` 中的 Host 别名、IdentityFile、ProxyJump 和 host key 策略。它不会接收或保存 SSH 密码。

## 构建和挂载

```bash
wails3 task build:oneshotfs
mkdir -p "$HOME/Volumes/linux-project"
./bin/oneshotfs \
  --host dev-linux \
  --root /home/dev/project \
  --mount "$HOME/Volumes/linux-project"
```

`--root` 必须是 Linux 上已经存在的绝对目录；`--mount` 必须是 Mac 上已经存在的空目录。挂载完成后，可以把 `$HOME/Volumes/linux-project` 当作普通工作区交给 Codex 或编辑器。

常用参数：

- `--cache-ttl 2s`：减少高延迟网络上的属性查询；代价是外部修改最多延迟 2 秒可见。
- `--ssh-option ProxyJump=bastion`：补充一项 OpenSSH 配置，可以重复传入。
- `--allow-other`：允许 Mac 上的其他用户访问挂载点，默认关闭。
- `--debug`：输出 FUSE 协议日志，只在排障时打开。

进程收到 `SIGINT` 或 `SIGTERM` 时会主动卸载。正常使用按 `Ctrl-C`；异常退出后可以执行：

```bash
diskutil unmount "$HOME/Volumes/linux-project"
```

## 文件语义和边界

当前实现支持目录遍历、随机偏移读写、创建、删除、重命名、截断、权限和时间修改，以及符号链接。远端路径在每次跟随符号链接前都会规范化，解析结果不能越出 `--root`。

以下行为不在当前版本范围内：

- 扩展属性、AppleDouble 文件、文件锁和硬链接；挂载参数会关闭 Finder 常见的 AppleDouble 和 Apple 扩展属性写入。
- 断网写入和离线编辑；SSH 中断后应卸载并重新挂载。
- 把本地进程自动转移到 Linux。FUSE 只能接管文件操作，无法拦截 Codex 启动的 shell 进程。依赖 Linux 容器、GPU、内核或工具链的命令仍要在远端运行，例如：

```bash
ssh dev-linux 'cd /home/dev/project && go test ./...'
```

因此，这个工具解决的是“远端文件像本地文件一样使用”。如果还要求命令也完全透明地在 Linux 执行，需要 Codex 支持远端 workspace/exec provider，或另外实现进程代理；单靠 FUSE 无法做到。

## 验证

```bash
go test ./internal/remotefs ./internal/fusefs
go test -race ./internal/remotefs ./internal/fusefs
go vet ./internal/remotefs ./internal/fusefs ./cmd/oneshotfs
go build -o /tmp/oneshotfs ./cmd/oneshotfs
```

`internal/remotefs` 的集成测试会启动本机 OpenSSH `sftp-server`，验证真实协议读写和路径隔离。macFUSE 已安装时，`internal/fusefs` 还会自动执行一次真实挂载、写入和卸载；未安装时该用例会显示 `SKIP`。
