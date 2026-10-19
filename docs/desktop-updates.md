# 桌面更新与官网下载

Keel 客户端在后台检查更新，但检查成功不会自动产生界面。下载提示、手动检查、签名下载和安装入口都必须显式接入。

## 2026-10-19：官网仍下载旧版、客户端没有更新提示

线上 `https://onecatch.app` 的版本日期已经变化，macOS 下载链接仍指向 v0.3.4 的 DMG。GitHub 的最新正式版本为 v0.5.2，release workflow 成功，macOS appcast 指向该版本的 ZIP。实下载 ZIP 的 SHA-256 与 `SHA256SUMS` 一致，并通过仓库固定 Ed25519 公钥验证。这些结果排除了本轮发布未生成包和包签名错误。

官网代码位于独立仓库 `openmodu/onecatch_app`。页面用 `data-asset` 的文件名后缀匹配最新 release。v0.5.0 起桌面发布改为 `darwin-arm64.zip`、`windows-amd64.zip`、`linux-amd64.tar.gz`，页面仍请求 DMG、EXE 安装器、DEB 和 AppImage。Worker 找不到匹配时保留旧链接，却继续更新版本号和日期；每日同步脚本则直接失败。2026-10-18 的 `Sync download links` 运行记录验证了这条失败路径。

官网补丁更新中英文卡片、校验文件和源码构建命令，并让缺失资产回退到当前版本的 release 文件页，避免把旧包当作新版。Android 不在当前桌面流水线中，保留最后发布的 v0.4.1 APK，并在按钮上明确标版本。Worker 与同步脚本都有回归测试，部署和同步前执行。

客户端的 `initUpdates` 已调用 `Service.Start`，结果只存入 `a.updates.status`，没有任何视图读取可用版本，也没有调用下载、安装的按钮。现在恢复 sidebar 底部、OneCatch 菜单上方的更新按钮，点击打开“软件更新”设置页。设置页显示当前版本、可用版本、更新说明与错误，提供检查、下载和重启安装。下载与安装分别由用户操作触发；后台检查不抢输入焦点。开发构建或缺少固定公钥的包明确显示自动更新未启用，并提供发布页入口。

`Service.Start` 的首次检查仍延迟 12 秒，之后每 6 小时检查一次。`Version=dev` 或没有 `UpdatePublicKey` 时不会开始检查。不要为了让开发包自动更新而从 feed 接受公钥；正式包通过 `package-keel.mjs` 注入版本和固定公钥。macOS 自动更新退出仍调用 `window.Quit`，不能用关闭主窗口代替退出进程。

## 验证与边界

- `TestUpdateControlsCheckDownloadAndInstall` 在 1×、2× 下用指针点击检查、下载和安装，验证状态流转、不会自动安装，以及安装失败后可见错误和重试入口。此测试使用替代更新服务，不替换真实应用。
- `TestBackgroundUpdateNoticePreservesComposerFocus` 使用生产 sidebar，验证按钮位于底部菜单上方、后台发现更新不抢输入焦点、关闭当前提示不屏蔽后续版本。
- `TestRenderUpdateSettings` 输出浅色、深色的更新页和 sidebar 截图，用于检查布局。
- 本轮独立原生候选客户端使用 `/tmp/onecatch-update-qa/data`，注入旧版本 0.5.1 和正式固定公钥，以实际网络读取 v0.5.2 feed。已看到自动发现提示和下载入口，并验证首次无点击输入、清空后的可见光标。最初原生检查的提示位于主区顶部，随后按用户要求移入 sidebar；最终位置由生产视图回归与截图验证。没有重启、替换用户正在运行的安装客户端，也没有执行真实安装回滚。

候选构建使用 go.mod 中的 Keel v0.3.3，无 go.work 或 replace。输入框修复已在 v0.5.2；更新界面和官网补丁仍需提交、发布，不能把本地验证写成线上已修复。

```sh
go test ./internal/app/keel ./internal/service/appupdate ./internal/app/updatehelper ./cmd/app -count=1
go vet ./internal/app/keel ./internal/service/appupdate ./internal/app/updatehelper ./cmd/app
ONECATCH_KEEL_PREVIEWS=/tmp/onecatch-update-previews go test ./internal/app/keel -run '^TestRenderUpdateSettings$' -count=1
# 在 onecatch_app 仓库：
node --test worker.test.mjs
python3 -m unittest test_sync_release.py
python3 sync-release.py
```
