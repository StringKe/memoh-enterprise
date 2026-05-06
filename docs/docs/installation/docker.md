# Docker Installation

The server installer is the default way to run Memoh Enterprise on Linux servers. The stack includes PostgreSQL, the Go server, the Web management UI, and optional Qdrant, Browser Gateway, and sparse memory services.

```bash
curl -fsSL https://raw.githubusercontent.com/StringKe/memoh-enterprise/main/scripts/install.sh | sh -s -- --runtime containerd --version latest
```

Supported deployment runtimes:

```bash
scripts/install.sh --runtime containerd --version latest
scripts/install.sh --runtime docker --version latest
scripts/install.sh --runtime podman --version latest
```

The installer installs missing server prerequisites automatically:

- `containerd` runtime: `containerd`, `nerdctl`, CNI/runtime files from the `nerdctl-full` release.
- `docker` runtime: Docker Engine and Docker Compose v2.
- `podman` runtime: Podman and `podman-compose`.
- Common tools: `git`, `curl` or `wget`, `openssl`, `tar`, and `gzip`.

Manual Docker Compose remains supported when Docker Engine is already installed.

## Services

| Service | Profile | Description |
|---------|---------|-------------|
| `postgres` | core | PostgreSQL database |
| `migrate` | core | Runs PostgreSQL migrations |
| `server` | core | Main Memoh server and in-process agent |
| `web` | core | Web management UI |
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

The server listens on `http://localhost:26810`. The Web management UI listens on `http://localhost:26811`. Browser Gateway listens on `http://localhost:26812` when the `browser` profile is enabled.

Reverse proxies in front of the Web UI must route `/connect/*` to the server without response buffering and must preserve WebSocket upgrade headers for `/integration/v1/ws`.

## Configuration

Copy `conf/app.docker.toml` to `config.toml` and adjust credentials before starting:

```bash
cp conf/app.docker.toml config.toml
```

PostgreSQL is the only supported relational database backend. The Compose stack uses the `containerd` workspace backend inside the server container by default. Docker Engine remains supported for host or binary deployments where workspace bind mounts are valid on the Docker host.

The `[web-ui]` section configures the bundled Web management UI.
