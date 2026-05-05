# Memoh Enterprise

Memoh Enterprise 是 Memoh 的后端优先企业版 fork。保留容器化 AI agent 平台核心能力，移除内置 Desktop、Web GUI、TUI 和 SQLite 支持。

支持的运行目标：Linux `amd64` 和 Linux `arm64`。macOS 保留本地开发兼容。

## 范围

保留：

- Go server 和 REST API。
- 非交互 `memoh` CLI。
- PostgreSQL 作为唯一关系数据库后端。
- Docker Engine 和 containerd workspace backend。
- Browser Gateway，用于 agent browser automation。
- agent、MCP、memory、schedule、providers、models、channels、email、workspace、container 管理。
- `web-ui` 配置，作为外部 Web UI 兼容配置。

移除：

- Electron/Desktop app。
- 内置 Web GUI 实现。
- 终端 TUI。
- SQLite 支持。
- 仅服务已移除 GUI 的 TypeScript SDK 生成链路。

## 快速启动

```bash
cp conf/app.docker.toml config.toml
docker compose up -d
```

启用可选服务：

```bash
docker compose --profile qdrant --profile browser --profile sparse up -d
```

API: `http://localhost:8080`

Browser Gateway: 启用 `browser` profile 后监听 `http://localhost:8083`。

模板中的默认管理员账号是 `admin` / `admin123`，生产使用前必须修改。

## 开发

```bash
mise install
mise run setup
mise run local:dev
```

常用命令：

```bash
mise run local:browser
mise run dev
mise run dev:infra
mise run e2e:smoke
mise run sqlc-generate
mise run swagger-generate
mise run build-unified
go test ./cmd/... ./internal/...
```

## 上游对齐

`.parent-commit` 记录当前 fork 对齐的上游 Memoh commit。
