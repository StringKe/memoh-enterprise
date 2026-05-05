# Docker Installation

Docker Compose is the default way to run Memoh Enterprise. The stack includes PostgreSQL, the Go server, and optional Qdrant, Browser Gateway, and sparse memory services. It does not include a bundled Web GUI.

## Services

| Service | Profile | Description |
|---------|---------|-------------|
| `postgres` | core | PostgreSQL database |
| `migrate` | core | Runs PostgreSQL migrations |
| `server` | core | Main Memoh server and in-process agent |
| `qdrant` | `qdrant`, `sparse` | Vector database |
| `browser` | `browser` | Playwright Browser Gateway for agent browser automation |
| `sparse` | `sparse` | Neural sparse encoding service |

## Start

```bash
docker compose up -d
```

Enable optional services:

```bash
docker compose --profile qdrant --profile browser --profile sparse up -d
```

The API listens on `http://localhost:8080`. Browser Gateway listens on `http://localhost:8083` when the `browser` profile is enabled.

## Configuration

Copy `conf/app.docker.toml` to `config.toml` and adjust credentials before starting:

```bash
cp conf/app.docker.toml config.toml
```

PostgreSQL is the only supported relational database backend. The Compose stack uses the `containerd` workspace backend inside the server container by default. Docker Engine remains supported for host or binary deployments where workspace bind mounts are valid on the Docker host.

The `[web-ui]` section is retained as compatibility configuration for an external Web UI. This repository does not ship a bundled Web GUI.
