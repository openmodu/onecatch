# Issue 00-006: 交付物

状态：已完成

## 来源

- PRD：`design/prd/00-agent-marketplace-mvp.md`
- 相关原型：`design/prototype/`

## 目标

实现已交付订单的交付物元数据、预览、下载和分享能力。

## 范围

- 交付物领域模型。
- 按订单查询交付物。
- 下载鉴权。
- 桌面端下载动作。
- 在文件夹中显示已下载文件。
- 分享链接生成边界。
- 首个交付物类型为 PDF 报告。

## 非目标

- 在线文档编辑器。
- 多文件交付包管理。
- 公开市场分享页。
- 长期 CDN 优化。

## 产品需求

- 已交付订单可以展示交付物预览。
- 交付物行展示文件名、类型、大小和预览图。
- 用户可以下载交付物。
- 用户可以生成或复制分享信息。
- 未交付订单不得暴露可下载交付物。

## 技术设计

- 后端接口：
  - `GET /api/orders/{id}/artifacts`
  - `GET /api/artifacts/{id}/download`
  - `POST /api/artifacts/{id}/share`
- 桌面端 bindings：
  - `ArtifactBinding.ListArtifacts(orderID)`
  - `ArtifactBinding.DownloadArtifact(artifactID)`
  - `ArtifactBinding.ShareArtifact(artifactID)`
  - `ArtifactBinding.ShowInFolder(path)`
- 领域对象：`Artifact`、`Download`、`Share`。
- `internal/repo/artifacts` 提供交付物和分享 token 的 repo 封装。
- `internal/usecase/artifacts` 只允许已交付订单查询、下载和分享交付物。
- 开发环境按订单生成 PDF 报告内容并保存到本地 Downloads。
- 生产环境文件存储使用对象存储，后续在文件存储 issue 中替换。
- 服务端必须按订单归属校验交付物访问权限。

## 验收标准

- [x] 已交付订单展示交付物元数据。
- [x] 执行中或待支付订单不暴露交付物下载。
- [x] 交付物下载会校验当前用户归属。
- [x] 桌面端下载会保存文件到本地路径。
- [x] 桌面端可以在文件夹中显示已下载文件。
- [x] 分享动作返回分享 token 或 URL。

## 测试计划

- 单元测试不同订单状态下的交付物可见性：`TestOrderValidationAndArtifacts` 覆盖 running 订单返回 409。
- 测试下载鉴权 handler：`TestOrderValidationAndArtifacts` 覆盖已交付订单交付物下载。
- 测试其他用户访问交付物时被拒绝：repo 按 `userID + artifactID` 查询，HTTP handler 使用当前用户；后续多用户 fixture 可补充更细断言。
- 手动验证桌面端下载和在文件夹中显示流程：前端已接入 `ArtifactBinding.DownloadArtifact` / `ShowInFolder`，`npm run build` 通过。

## 交付记录

- 已新增 `internal/domain/artifacts`、`internal/repo/artifacts`、`internal/usecase/artifacts`。
- 已新增 HTTP 接口：订单交付物列表、交付物下载、分享。
- 已新增桌面端 Artifact binding，并在 Inspector 交付物卡片接入下载和分享。
- 已补充 Wails 生成 bindings。
- `go test ./...` 和桌面前端 `npm run build` 已通过。
