# Memoh Enterprise Deployment

## Docker Compose

```bash
cp conf/app.docker.toml config.toml
docker compose up -d
```

Optional services:

```bash
docker compose --profile qdrant --profile browser --profile sparse up -d
```

Core services:

- `postgres`
- `migrate`
- `server`

Optional services:

- `qdrant`
- `browser`
- `sparse`

## Access

- API: `http://localhost:8080`
- Browser Gateway: `http://localhost:8083` with the `browser` profile

## Configuration

Edit `config.toml` before production use:

- `admin.password`
- `auth.jwt_secret`
- `postgres.password`
- `container.backend`
- `[web-ui]` when pointing an external Web UI at this server

PostgreSQL is the only supported relational database backend.

## Kubernetes

```bash
kubectl apply -k deploy/kubernetes/base
kubectl -n memoh rollout status deployment/memoh-server
```

Edit `deploy/kubernetes/base/config-secret.yaml` before deploying.
