# Memoh Enterprise

Memoh Enterprise is an enterprise-focused, fully open-source fork of [Memoh](https://github.com/memohai/Memoh). The upstream project is a self-hosted, always-on AI agent platform that runs bots in containers with long-term memory and chat integrations.

This fork keeps the core agent platform and web management experience, but narrows the product to enterprise server deployments. It removes desktop and terminal UI surfaces, removes SQLite, standardizes on PostgreSQL, and replaces the OpenAPI management surface with ConnectRPC.

## Upstream

- Upstream repository: [memohai/Memoh](https://github.com/memohai/Memoh)
- Current upstream alignment commit: `beae0041b9f0e009509e6411673175e672b6f163`
- Alignment marker: [`.parent-commit`](./.parent-commit)

This project is built on top of Memoh. Thanks to the Memoh maintainers and contributors for creating and maintaining the original self-hosted AI agent platform. Memoh Enterprise exists because that upstream work provides the agent runtime, container workspace model, memory system, channel integrations, and web management foundation.

This fork is expected to keep syncing useful upstream server, agent, memory, provider, channel, workspace, and web management improvements. Upstream changes for Desktop GUI, TUI, SQLite, Windows packaging, Docker Hub publishing, npmjs publishing, and China-specific optimizations are intentionally excluded.

## Enterprise Scope

Included:

- Go server with ConnectRPC management APIs under `/connect`.
- Vue web management UI in `apps/web`.
- PostgreSQL as the only relational database backend.
- containerd, Docker Engine, Podman, and Kubernetes workspace runtimes.
- Browser Gateway for headless browser automation used by agent tools.
- Split runtime services: `memoh-server`, `memoh-agent-runner`, `memoh-connector`, `memoh-integration-gateway`, `memoh-worker`, and `workspace-executor`.
- Agent tools, MCP, memory, schedule, providers, models, channels, email, workspace, and container management.
- Bot Groups for enterprise-level configuration inheritance and grouped bot operations.
- Enterprise integration API tokens for global, bot-scoped, and bot-group-scoped access.
- WebSocket-based external integration protocol with Go and TypeScript SDKs.
- GHCR-based publishing for Docker images and packages.
- Non-interactive `memoh` CLI for server operations and administrative maintenance.

Removed:

- Electron/Desktop application.
- Terminal TUI.
- SQLite database backend.
- Docker Hub publishing.
- npmjs publishing.
- Windows-specific packaging.
- China-specific installation and runtime optimizations.

## ConnectRPC And Integration APIs

The internal management API is ConnectRPC-first. Protocol definitions live in [`proto/memoh/private/v1`](./proto/memoh/private/v1), generated Go handlers live under [`internal/connectapi`](./internal/connectapi), and generated TypeScript client types live under [`packages/sdk`](./packages/sdk).

External enterprise integrations use a separate WebSocket protocol. Its protobuf definitions live in [`proto/memoh/integration/v1`](./proto/memoh/integration/v1). SDKs are provided for:

- Go: [`sdk/go/integration`](./sdk/go/integration)
- TypeScript: [`packages/integration-sdk-ts`](./packages/integration-sdk-ts)

Other languages are not supported at this stage.

## Runtime Architecture

- `memoh-server`: control plane, ConnectRPC management API, auth, RBAC, settings, token admin, and health checks.
- `memoh-agent-runner`: agent run lifecycle and tool orchestration.
- `memoh-connector`: platform channel adapters and long-lived connector leases.
- `memoh-integration-gateway`: external enterprise WebSocket integration API.
- `memoh-worker`: schedule, heartbeat, compaction, cleanup, and PostgreSQL outbox consumers.
- `workspace-executor`: in-workspace file, exec, PTY, and MCP server execution boundary.

## Runtime Targets

Supported deployment targets:

- Linux `amd64`
- Linux `arm64`

macOS is supported for local development compatibility, especially on Apple Silicon. Desktop GUI support is not part of this fork.

## Quick Start

Server installer:

```bash
curl -fsSL https://raw.githubusercontent.com/StringKe/memoh-enterprise/main/scripts/install.sh | sh -s -- --runtime containerd --version latest
```

Supported deployment runtimes:

```bash
scripts/install.sh --runtime containerd --version latest
scripts/install.sh --runtime docker --version latest
scripts/install.sh --runtime podman --version latest
```

The installer installs missing server prerequisites automatically, including the selected runtime CLI.

Manual Compose start:

```bash
cp conf/app.docker.toml config.toml
docker compose up -d
```

Enable optional services:

```bash
docker compose --profile qdrant --profile browser --profile sparse up -d
```

Default local endpoints:

- Server API: `http://localhost:26810`
- Web UI: `http://localhost:26811`
- Browser Gateway: `http://localhost:26812` when the `browser` profile is enabled
- Agent Runner: `http://localhost:26813`
- Integration Gateway: `http://localhost:26815`
- PostgreSQL: `localhost:26817`
- Qdrant HTTP: `http://localhost:26818`
- Qdrant gRPC: `localhost:26819`
- Sparse memory service: `http://localhost:26820`

Default admin account in templates: `admin` / `admin123`. Change it before production use.

## Local Development

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

## Documentation

- Enterprise scope: [`docs/enterprise-scope.md`](./docs/enterprise-scope.md)
- Upstream sync process: [`docs/upstream-sync.md`](./docs/upstream-sync.md)
- Deployment guide: [`DEPLOYMENT.md`](./DEPLOYMENT.md)
