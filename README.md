# Memoh Enterprise

Memoh Enterprise is an enterprise-focused fork of Memoh. It keeps the containerized AI agent platform and the web management UI, while removing Desktop GUI, TUI, and SQLite support.

Supported runtime targets: Linux `amd64` and Linux `arm64`. macOS remains supported for local development compatibility.

## Scope

Kept:

- Go server with ConnectRPC management API.
- Non-interactive `memoh` CLI.
- PostgreSQL as the only relational database backend.
- Docker Engine and containerd workspace backends.
- Browser Gateway for agent browser automation.
- Web management UI in `apps/web`.
- Agent, MCP, memory, schedule, providers, models, channels, email, workspace, and container management.
- `web-ui` configuration for the bundled web management UI.

Removed:

- Electron/Desktop app.
- Terminal TUI.
- SQLite support.

## Quick Start

```bash
cp conf/app.docker.toml config.toml
docker compose up -d
```

Enable optional services:

```bash
docker compose --profile qdrant --profile browser --profile sparse up -d
```

API: `http://localhost:26810`

Web UI: `http://localhost:26811`

Browser Gateway: `http://localhost:26812` when the `browser` profile is enabled.

Default admin account in templates: `admin` / `admin123`. Change it before production use.

## Development

```bash
mise install
mise run setup
mise run local:dev
```

Useful commands:

```bash
mise run local:browser
mise run web:dev
mise run dev
mise run dev:infra
mise run e2e:smoke
mise run sqlc-generate
mise run proto-generate
mise run sdk-generate
mise run build-unified
go test ./cmd/... ./internal/...
```

## Upstream Alignment

`.parent-commit` records the upstream Memoh commit this fork is aligned with.
