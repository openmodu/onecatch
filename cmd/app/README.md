# OneCatch 入口

`cmd/app` 和 `cmd/keel` 共用原生 Keel 桌面装配。启动时先分发 Worker、远程 shell、SSH askpass 和更新器角色；没有子命令时打开桌面。

```bash
go run ./cmd/app
go run ./cmd/app worker --help
go build -trimpath -ldflags='-s -w' -o bin/onecatch ./cmd/app
go build -tags onecatch_headless,onecatch_worker -trimpath -ldflags='-s -w' -o bin/headless/onecatch ./cmd/app
```

打包执行 `node build/scripts/package-keel.mjs`。数据默认保存在 `~/.onecatch/`，桌面可用 `-data-dir` 指定其他目录。界面代码位于 `internal/app/keel`，共享业务服务位于 `internal/service/desktop`。
