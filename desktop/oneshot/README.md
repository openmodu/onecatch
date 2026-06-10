# Oneshot Desktop

Wails v3 桌面端。

## 运行

```bash
wails3 dev -config ./build/config.yml
```

## 构建

```bash
wails3 build DEV=true
```

## 目录

```text
app/          应用元信息
bindings/     Wails bindings，只包装 clients/oneshot
frontend/     React/Vite 前端和生成 bindings
build/        Wails 构建配置
```
