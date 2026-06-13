# Oneshot Desktop

Wails v3 桌面端。它是一个纯 API 客户端，所有领域逻辑、持久化和鉴权都在
`oneshot-server`，桌面端通过 HTTP 访问（现在本地，以后换远端域名）。

## 运行

桌面端依赖 `oneshot-server`，需先启动后端：

```bash
# 1. 启动后端（默认 :8080）。本地登录走开发态 OAuth 回调，需开启 insecure 模式。
ONESHOT_AUTH_INSECURE_CALLBACKS=1 go run ./cmd/oneshot-server

# 2. 启动桌面端（默认连 http://127.0.0.1:8080）
wails3 dev -config ./build/config.yml
```

连接远端后端时设置 `ONESHOT_API_BASE_URL`：

```bash
ONESHOT_API_BASE_URL=https://api.oneshot.example wails3 dev -config ./build/config.yml
```

## 构建

```bash
wails3 build DEV=true
```

## 目录

```text
app/          应用元信息
bindings/     Wails bindings，只包装 clients/oneshot
client.go     解析 base URL，返回 oneshot HTTP 客户端
frontend/     React/Vite 前端和生成 bindings
build/        Wails 构建配置
```
