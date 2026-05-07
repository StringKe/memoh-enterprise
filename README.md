# Memoh Enterprise

Memoh Enterprise is a fully open-source, enterprise-focused fork of [Memoh](https://github.com/memohai/Memoh). It keeps Memoh's self-hosted AI agent foundation and reshapes the project into a server-first platform for enterprise deployments.

This fork is not a desktop product. It removes Desktop GUI, terminal TUI, SQLite, Windows packaging, Docker Hub publishing, npmjs publishing, and China-specific runtime optimizations. The supported product surface is the Go server, ConnectRPC APIs, web management UI, PostgreSQL, containerized workspaces, browser automation, enterprise integrations, and operational CLI.

## What Is New In This Fork

- **Enterprise scope**: server-first architecture with Web UI, RBAC, SSO, Bot Groups, API tokens, audit-aware management APIs, and operational CLI.
- **ConnectRPC management API**: the old OpenAPI management surface is replaced by protobuf contracts and ConnectRPC handlers.
- **Bot Groups**: grouped bot ownership, visibility, inherited settings, and principal role assignment for enterprise administration.
- **Structured Data Spaces**: every bot and bot group can own a PostgreSQL schema for relational data, run raw SQL including DDL, and share access across bots or bot groups through platform grants backed by PostgreSQL role permissions.
- **Integration Gateway**: external enterprise integrations use a dedicated WebSocket protocol and separate protobuf contracts.
- **Scoped integration tokens**: integration clients can use global, bot-scoped, or bot-group-scoped API tokens.
- **Split runtime services**: server, agent runner, connector, integration gateway, worker, browser gateway, and workspace executor are separate runtime roles.
- **PostgreSQL only**: PostgreSQL is the only relational database backend. SQLite support is removed.
- **Workspace Executor**: the old bridge role is renamed and narrowed into the in-workspace execution boundary for files, exec, PTY, and MCP server processes.
- **Container runtime focus**: containerd is the default workspace runtime, with Docker Engine, Podman, and Kubernetes runtime support retained where implemented.
- **Browser automation retained**: Browser Gateway and Playwright-based agent tools remain supported because they are agent capabilities, not GUI features.
- **Feishu/Lark WebSocket lifecycle fix**: Feishu/Lark channel connections now close their WebSocket transport on stop and do not leave old subscriptions running after token or config updates.
- **Vite+ frontend tooling**: frontend checks, formatting, tests, and package-manager wrappers are handled through Vite+.
- **GitHub-native publishing**: Docker images publish to GHCR and npm packages publish to GitHub Packages.

## Upstream Relationship

This repository is built on top of upstream Memoh.

- Upstream repository: [memohai/Memoh](https://github.com/memohai/Memoh)
- Enterprise repository: [StringKe/memoh-enterprise](https://github.com/StringKe/memoh-enterprise)
- Upstream alignment marker: [`.parent-commit`](./.parent-commit)
- Current upstream alignment commit: `c674b4921bae885088022ba58fc6bfc832c8c157`

Thanks to the Memoh maintainers and contributors for the original self-hosted AI agent platform, including the agent runtime, long-term memory model, workspace container design, channel integrations, and web management foundation.

This fork continues to sync useful upstream changes for server, agent, memory, MCP, providers, models, channels, email, workspace, container runtime, Browser Gateway, and Web UI. Changes that reintroduce Desktop GUI, TUI, SQLite, Windows packaging, Docker Hub, npmjs, or China-specific optimizations are intentionally excluded.

See [docs/upstream-sync.md](./docs/upstream-sync.md) for the synchronization process.

## Product Scope

Included:

- Go server and `memoh-server`.
- Web management UI in [`apps/web`](./apps/web).
- Browser Gateway in [`apps/browser`](./apps/browser).
- ConnectRPC management API under `/connect`.
- TypeScript management SDK in [`packages/sdk`](./packages/sdk).
- PostgreSQL schema, migrations, and sqlc queries in [`db/postgres`](./db/postgres).
- Bot Groups, RBAC, SSO, auth, settings, models, providers, memory, schedules, channels, email, and workspace management.
- Bot and Bot Group structured data spaces backed by PostgreSQL schema and role isolation.
- External integration WebSocket API and SDKs.
- Non-interactive `memoh` CLI for server operations and break-glass administration.
- Docker Engine, containerd, Podman, and Kubernetes runtime adapters.
- Linux `amd64` and Linux `arm64` deployment targets.
- macOS local development compatibility.

Removed:

- Electron/Desktop app.
- Terminal TUI.
- SQLite schema, migrations, driver, generated code, configuration, and documentation.
- Windows release targets and installation instructions.
- Docker Hub publishing.
- npmjs publishing.
- China-specific mirrors, installation paths, and runtime optimizations.

## Architecture

| Runtime | Default Port | Role |
| --- | ---: | --- |
| `memoh-server` | `26810` | Control plane, ConnectRPC management API, auth, RBAC, settings, token admin, health, and container control |
| `memoh-web` | `26811` | Vue 3 web management UI |
| `memoh-browser` | `26812` | Browser Gateway for Playwright automation used by agent tools |
| `memoh-agent-runner` | `26813` | Agent run lifecycle, tool orchestration, and model execution flow |
| `memoh-connector` | internal | Platform channel adapters and long-lived connector leases |
| `memoh-integration-gateway` | `26815` | External enterprise WebSocket integration API |
| `memoh-worker` | internal | Schedule, heartbeat, compaction, cleanup, and PostgreSQL outbox consumers |
| `workspace-executor` | Unix socket | In-workspace file, exec, PTY, and MCP server execution boundary |
| PostgreSQL | `26817` | Relational database |
| Qdrant HTTP | `26818` | Optional vector storage |
| Qdrant gRPC | `26819` | Optional vector storage gRPC endpoint |
| Sparse memory service | `26820` | Optional sparse retrieval service |

## API Surfaces

### Management API

Management APIs are ConnectRPC-first.

- Protobuf contracts: [`proto/memoh/private/v1`](./proto/memoh/private/v1)
- Generated Go code: [`internal/connectapi/gen`](./internal/connectapi/gen)
- ConnectRPC handlers: [`internal/connectapi`](./internal/connectapi)
- TypeScript SDK: [`packages/sdk`](./packages/sdk)

Generate protocol artifacts:

```bash
mise run proto-generate
```

### Structured Data API

Structured Data Spaces give bots and bot groups relational PostgreSQL storage without relying on memory records or markdown files.

- Protobuf contract: [`proto/memoh/private/v1/structured_data.proto`](./proto/memoh/private/v1/structured_data.proto)
- Service implementation: [`internal/structureddata`](./internal/structureddata)
- ConnectRPC handler: [`internal/connectapi/structured_data.go`](./internal/connectapi/structured_data.go)
- Web UI: Settings -> Structured Data, Bot detail -> Structured Data, and Bot Group detail -> Structured Data
- Agent tools: `structured_data_spaces` and `structured_data_sql`

Each structured data space creates:

- one generated PostgreSQL schema, such as `bot_data_<uuid>` or `bot_group_data_<uuid>`
- one generated PostgreSQL owner role, such as `memoh_bot_<uuid>` or `memoh_group_<uuid>`
- audit rows for ensure, SQL execution, grant, and revoke operations

Bots can execute raw SQL, including DDL, in their own bot space and their bot group's space. Cross-bot and cross-group sharing is available immediately through structured data grants:

- `read`: grants schema usage plus table and sequence read privileges
- `write`: grants read plus insert, update, delete, truncate, and sequence update privileges
- `ddl`: grants schema create privileges

The platform authorization model decides who can manage spaces and grants. PostgreSQL schema and role permissions enforce what bot execution can do at runtime.

### External Integration API

Enterprise external integrations use WebSocket, not webhook. The external integration protocol is intentionally separate from the internal management API.

- Protobuf contracts: [`proto/memoh/integration/v1`](./proto/memoh/integration/v1)
- Integration Gateway: [`cmd/integration-gateway`](./cmd/integration-gateway)
- Go SDK: [`sdk/go/integration`](./sdk/go/integration)
- TypeScript SDK: [`packages/integration-sdk-ts`](./packages/integration-sdk-ts)

Only Go and TypeScript SDKs are maintained at this stage.

## Runtime Services

Memoh Enterprise splits platform responsibilities into explicit services:

- `memoh-server`: control plane, API, auth, RBAC, token administration, container runtime coordination.
- `memoh-agent-runner`: agent execution lifecycle and tool orchestration.
- `memoh-connector`: platform channel long connections and inbound/outbound message flow.
- `memoh-integration-gateway`: external enterprise WebSocket integration clients.
- `memoh-worker`: background schedules, compaction, cleanup, heartbeat, and outbox work.
- `memoh-browser`: Browser Gateway for agent browser automation.
- `workspace-executor`: workspace-local execution service mounted inside workspace containers.

This split keeps platform integrations, model work, browser automation, and workspace execution out of the main control plane process.

## Channel And Integration Notes

The project keeps platform channel adapters for enterprise bot use cases. Telegram, Feishu/Lark, WeCom, DingTalk, Slack, Discord, Matrix, QQ, WeChat, Misskey, and local web channels are represented in the codebase.

Feishu/Lark WebSocket connections use a project-owned transport so `Stop` closes the WebSocket and waits for the read, ping, and reconnect loops to exit. Updating channel credentials now restarts the connection without leaving the old subscription owned by the previous SDK client.

## Installation

Default server install with containerd:

```bash
curl -fsSL https://raw.githubusercontent.com/StringKe/memoh-enterprise/main/scripts/install.sh | sh -s -- --runtime containerd --version latest
```

Supported install runtime flags:

```bash
scripts/install.sh --runtime containerd --version latest
scripts/install.sh --runtime docker --version latest
scripts/install.sh --runtime podman --version latest
```

The installer defaults to:

- image registry: `ghcr.io/stringke`
- database driver: `postgres`
- deployment runtime: `containerd`
- workspace backend: `containerd`

It installs missing host prerequisites through the system package manager when required.

## Manual Compose Deployment

Create a config file and start the stack:

```bash
cp conf/app.docker.toml config.toml
docker compose up -d
```

Enable optional services:

```bash
docker compose --profile browser --profile qdrant --profile sparse up -d
```

Useful compose operations through the CLI:

```bash
memoh start
memoh stop
memoh restart server
memoh status
memoh logs server
memoh update
```

Inspect nested containerd inside the server container:

```bash
memoh ctr images ls
memoh ctr containers ls
memoh ctr --namespace default tasks ls
```

Default admin credentials in templates are `admin` / `admin123`. Change them before exposing a deployment.

## Local Development

Install toolchains:

```bash
mise install
mise run setup
```

Run infrastructure and server on the host:

```bash
mise run local:dev
```

Run Browser Gateway on the host:

```bash
mise run local:browser
```

Run Web UI development server:

```bash
mise run web:dev
```

Run the full containerized development environment:

```bash
mise run dev
```

Common development commands:

```bash
mise run dev:infra
mise run dev:logs
mise run dev:restart -- server
mise run db-up
mise run db-down
mise run sqlc-generate
mise run proto-generate
mise run sdk-generate
mise run build-unified
mise run e2e:smoke
```

## Validation

Run Go tests:

```bash
go test ./...
```

Run repository checks:

```bash
mise run lint
```

Run frontend tests:

```bash
vp test
```

Run structured data integration tests against PostgreSQL:

```bash
TEST_POSTGRES_DSN='postgres://postgres@127.0.0.1:26817/memoh?sslmode=disable' go test ./internal/structureddata -run 'TestServiceIntegration' -count=1 -v
```

Run endpoint smoke tests against a running deployment:

```bash
mise run e2e:smoke
```

## Publishing

Docker images are published to GHCR under `ghcr.io/stringke`.

Primary images:

- `ghcr.io/stringke/server`
- `ghcr.io/stringke/web`
- `ghcr.io/stringke/browser`
- `ghcr.io/stringke/agent-runner`
- `ghcr.io/stringke/connector`
- `ghcr.io/stringke/integration-gateway`
- `ghcr.io/stringke/worker`
- `ghcr.io/stringke/sparse`

npm packages are published to GitHub Packages at `https://npm.pkg.github.com`.

Published package candidates:

- `apps/browser`
- `packages/config`
- `packages/icons`
- `packages/sdk`
- `packages/integration-sdk-ts`
- `packages/ui`

There are no binary release artifacts for this fork.

## Important Directories

| Path | Purpose |
| --- | --- |
| [`cmd/server`](./cmd/server) | `memoh-server` entrypoint |
| [`cmd/memoh`](./cmd/memoh) | Non-interactive operations CLI |
| [`cmd/agent-runner`](./cmd/agent-runner) | Agent runner service |
| [`cmd/connector`](./cmd/connector) | Connector service |
| [`cmd/integration-gateway`](./cmd/integration-gateway) | Enterprise integration gateway |
| [`cmd/worker`](./cmd/worker) | Background worker service |
| [`cmd/workspace-executor`](./cmd/workspace-executor) | Workspace execution boundary |
| [`internal`](./internal) | Go backend packages |
| [`internal/structureddata`](./internal/structureddata) | PostgreSQL-backed bot and bot-group structured data service |
| [`apps/web`](./apps/web) | Web management UI |
| [`apps/browser`](./apps/browser) | Browser Gateway |
| [`packages/sdk`](./packages/sdk) | TypeScript ConnectRPC management SDK |
| [`packages/integration-sdk-ts`](./packages/integration-sdk-ts) | TypeScript integration SDK |
| [`sdk/go/integration`](./sdk/go/integration) | Go integration SDK |
| [`db/postgres`](./db/postgres) | PostgreSQL migrations and queries |
| [`proto`](./proto) | ConnectRPC and integration protobuf contracts |
| [`deploy`](./deploy) | Compose, Docker, and Kubernetes deployment files |
| [`scripts`](./scripts) | Install, release, DB, and E2E scripts |
| [`docs`](./docs) | Project documentation |

## Documentation

- [Enterprise scope](./docs/enterprise-scope.md)
- [Upstream sync guide](./docs/upstream-sync.md)
- [Deployment guide](./DEPLOYMENT.md)

## License

This fork follows the repository license in [LICENSE](./LICENSE).
