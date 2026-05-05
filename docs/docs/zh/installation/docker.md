# Docker 安装

Docker Compose 是 Memoh Enterprise 的默认运行方式。默认栈包含 PostgreSQL、Go server，以及可选的 Qdrant、Browser Gateway、sparse memory 服务。本仓库不内置 Web GUI。

## 服务

| 服务 | Profile | 说明 |
|------|---------|------|
| `postgres` | core | PostgreSQL 数据库 |
| `migrate` | core | 执行 PostgreSQL 迁移 |
| `server` | core | Memoh 主服务和进程内 agent |
| `qdrant` | `qdrant`, `sparse` | 向量数据库 |
| `browser` | `browser` | 用于 agent browser automation 的 Playwright Browser Gateway |
| `sparse` | `sparse` | 神经稀疏编码服务 |

## 启动

```bash
docker compose up -d
```

启用可选服务：

```bash
docker compose --profile qdrant --profile browser --profile sparse up -d
```

API 监听 `http://localhost:8080`。启用 `browser` profile 后，Browser Gateway 监听 `http://localhost:8083`。

## 配置

启动前复制 `conf/app.docker.toml` 到 `config.toml` 并调整凭据：

```bash
cp conf/app.docker.toml config.toml
```

PostgreSQL 是唯一支持的关系数据库后端。Compose 栈默认使用 server 容器内部的 `containerd` workspace backend。Docker Engine 仍支持用于 host 或 binary 部署，前提是 workspace bind mount 路径在 Docker host 上有效。

`[web-ui]` 配置作为外部 Web UI 兼容配置保留。本仓库不内置 Web GUI。
