# Memoh Enterprise

Memoh Enterprise is a backend-first fork of Memoh for enterprise deployments. It keeps the containerized AI agent platform and removes bundled Desktop, Web GUI, TUI, and SQLite support.

Supported runtime targets: Linux `amd64` and Linux `arm64`. macOS remains supported for local development compatibility.

## Scope

Kept:

- Go server and REST API.
- Non-interactive `memoh` CLI.
- PostgreSQL as the only relational database backend.
- Docker Engine and containerd workspace backends.
- Browser Gateway for agent browser automation.
- Agent, MCP, memory, schedule, providers, models, channels, email, workspace, and container management.
- `web-ui` configuration for external Web UI compatibility.

Removed:

- Electron/Desktop app.
- Bundled Web GUI implementation.
- Terminal TUI.
- SQLite support.
- TypeScript SDK generation for the removed bundled GUI.

## Quick Start

```bash
cp conf/app.docker.toml config.toml
docker compose up -d
```

Enable optional services:

```bash
docker compose --profile qdrant --profile browser --profile sparse up -d
```

API: `http://localhost:8080`

Browser Gateway: `http://localhost:8083` when the `browser` profile is enabled.

Default admin account in templates: `admin` / `admin123`. Change it before production use.

## Development

```bash
mise install
mise run setup
mise run dev
```

Useful commands:

```bash
mise run sqlc-generate
mise run swagger-generate
mise run build-unified
go test ./cmd/... ./internal/...
```

## Upstream Alignment

`.parent-commit` records the upstream Memoh commit this fork is aligned with.
