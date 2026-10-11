# OneCatch 单二进制

`cmd/app` 与 `cmd/keel` 共用 `internal/app` 装配，默认打开 Keel 原生桌面。`onecatch worker` 启动 Worker；内部 shell、askpass、updater 角色通过 `ONECATCH_INTERNAL_MODE` 分发，进入角色后清除环境变量，避免传给子进程。

```bash
go build -trimpath -ldflags='-s -w' -o bin/onecatch ./cmd/app
go build -tags onecatch_headless,onecatch_worker -trimpath -ldflags='-s -w' -o bin/headless/onecatch ./cmd/app
```

无界面版本不创建窗口，也不链接图形界面包。桌面二进制中的 Worker 仍会加载原生图形库，因此服务器应使用无界面版本。旧 `onecatch-worker` 命令改为 `onecatch worker`；已有常驻服务需要用新路径重新安装，数据目录和配对身份保持原值。

macOS 更新器验证下载文件的 Ed25519 签名，缓存 ZIP，在独立 helper 中替换应用 bundle。新窗口发送 ready 标记前保留旧 bundle，启动失败时回滚。Windows、Linux 新发布的便携包使用手动替换；旧安装器与 AppImage 的 helper 兼容逻辑仍保留。

原来的移动端入口、WebView 和 React 构建已移除。远程连接、Worker HTTP 协议和共享业务服务继续由 Go 实现。
