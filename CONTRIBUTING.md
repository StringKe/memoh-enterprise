# Contributing Guide

## Prerequisites

- [Docker](https://docs.docker.com/get-docker/) (for containerized dev environment)
- [mise](https://mise.jdx.dev/) (task runner & toolchain manager)

### Install mise

```bash
curl https://mise.run | sh
# or
brew install mise
```

## Quick Start

```bash
mise install       # Install toolchains (Go, Node, Bun, pnpm, sqlc)
./deploy/docker/toolkit/install.sh  # Install toolkit used by the nested workspace runtime
mise run setup     # Install deps and prepare local tooling
mise run local:dev # Start infra in Docker and run the server on macOS
```

The default local workflow is macOS arm64 first:

1. PostgreSQL, Qdrant, and Sparse run in a Docker-compatible engine.
2. The Go server runs on macOS with `container.backend = "docker"`.
3. Browser Gateway runs on macOS with Bun when browser automation is needed.
4. External Web UI runs outside this repository.

OrbStack is supported as the Docker-compatible engine on macOS. OrbStack Linux machines are optional and are not required for the default workflow.

The dev stack uses `deploy/config/dev/app.dev.toml` directly and no longer overwrites the repo root `config.toml`.
Default host ports start at `26810`: API `26810`, Web UI `26811`, Browser Gateway `26812`, Postgres `26813`, Qdrant HTTP `26814`, Qdrant gRPC `26815`, Sparse `26816`, and OAuth callback/server alternate `26817`.

## Daily Development

```bash
mise run local:dev       # Start infra and run server on macOS
mise run local:browser   # Start Browser Gateway on macOS
mise run dev             # Start full containerized environment
mise run dev:infra       # Start only Postgres, Qdrant, and Sparse
mise run dev:selinux     # Start full environment on SELinux hosts
mise run dev:down        # Stop all services
mise run dev:down:selinux # Stop SELinux dev environment
mise run dev:logs        # View logs
mise run dev:logs:selinux # View logs on SELinux hosts
mise run dev:restart -- server  # Restart a specific service
mise run dev:restart:selinux -- server  # Restart a service on SELinux hosts
mise run bridge:build:selinux  # Rebuild bridge binary on SELinux hosts
```

## More Commands

| Command | Description |
| ------- | ----------- |
| `mise run local:dev` | Start local infra and run the server on the host |
| `mise run local:browser` | Start Browser Gateway on the host |
| `mise run dev` | Start full containerized dev environment |
| `mise run dev:infra` | Start only Postgres, Qdrant, and Sparse |
| `mise run dev:selinux` | Start dev environment with SELinux compose overrides |
| `mise run dev:down` | Stop dev environment |
| `mise run dev:down:selinux` | Stop SELinux dev environment |
| `mise run dev:logs` | View dev logs |
| `mise run dev:logs:selinux` | View dev logs on SELinux hosts |
| `mise run dev:restart` | Restart a service (e.g. `-- server`) |
| `mise run dev:restart:selinux` | Restart a service on SELinux hosts |
| `mise run bridge:build:selinux` | Rebuild bridge binary in SELinux dev container |
| `mise run setup` | Install deps and prepare local tooling |
| `mise run db-up` | Run database migrations |
| `mise run db-down` | Roll back database migrations |
| `mise run proto-generate` | Generate ConnectRPC protobuf code |
| `mise run sqlc-generate` | Generate SQL code |
| `mise run e2e:smoke` | Smoke-test a running deployment |

## Project Layout

```
conf/       — Configuration templates (app.example.toml, app.docker.toml)
deploy/     — Compose, Docker, Kubernetes, and dev deployment resources
cmd/        — Go application entry points
internal/   — Go backend core code
apps/       — Application services (Browser Gateway, etc.)
  browser/  — Browser Gateway (Bun/Elysia/Playwright)
packages/   — Frontend monorepo (web, ui, sdk, config)
db/         — Database migrations and queries
scripts/    — Utility scripts
proto/      — ConnectRPC protobuf contracts
```
