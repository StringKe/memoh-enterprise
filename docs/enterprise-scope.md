# Memoh Enterprise Scope

## 背景

当前仓库是 `memoh` 的 enterprise fork，目标是面向企业领域维护一个完全开源版本。后续仍会从上游 `memoh` 同步相关功能，因此边界必须清晰，降低长期同步冲突。

上游同步基线记录在仓库根目录 `.parent-commit`。同步流程见 `docs/upstream-sync.md`。

## 总目标

将项目收敛为企业版平台：

- 保留 Go server、ConnectRPC 管理 API、Web 管理后台、Browser Gateway、agent tools、MCP、memory、schedule、providers、models、channels、email、workspace 和 containers。
- 保留 `apps/web`，它是企业版管理后台，不属于 Desktop GUI。
- 保留 `packages/ui`、`packages/sdk`、`packages/icons`、`packages/config`，它们服务 Web 管理后台和 Browser Gateway。
- 保留 ConnectRPC protobuf 和 TypeScript SDK 生成链路。
- 保留 PostgreSQL，且 PostgreSQL 是唯一关系数据库后端。
- 保留 containerd、Docker Engine、Podman、Kubernetes runtime 相关能力。
- 保留非交互 CLI。
- 移除 Desktop GUI、终端 TUI、SQLite、Windows 发布/配置、Docker Hub 发布、npmjs 发布和中国特化优化。
- 运行和发布目标只覆盖 Linux `amd64` / `arm64`，macOS 只保留本地开发兼容。

## 删除范围

### Desktop GUI

- 删除 Electron desktop app。
- 删除 `apps/desktop/`。
- 删除 `@stringke/desktop`、`@memohai/desktop`、`electron`、`electron-vite`、`electron-builder` 等 desktop 专用依赖。
- 删除 desktop dev/build/release/docs 配置。
- 删除 `apps/web` 中仅服务 Electron shell 的适配代码。

### TUI

- 删除终端 TUI。
- `memoh` 不带子命令时只显示 help，不进入 TUI。
- 删除 `memoh tui` 子命令。
- 删除 `internal/tui/`。
- 删除 Bubble Tea、Glamour、Lipgloss 等 TUI 专用依赖。

### SQLite

- 删除 `db/sqlite/`。
- 删除 `internal/db/sqlite/`。
- 删除 SQLite driver `modernc.org/sqlite`。
- 删除配置里的 `[sqlite]`。
- 删除 database backend switch 中的 SQLite 分支。
- 删除 SQLite migration/dev 命令与文档。
- 后续 SQL schema/query 只维护 PostgreSQL。

### Windows 和发布渠道

- 不提供 Windows 配置模板、发布目标或安装说明。
- Docker images 只发布到 GHCR。
- npm packages 只发布到 GitHub Packages。
- 不发布二进制 release artifacts。
- 不保留 Docker Hub 发布配置。
- 不保留中国镜像源、CN compose、CN install 优化。

## 保留范围

### Web 管理后台

- 保留 `apps/web/`。
- 保留 Vue、Vite+、Tailwind、Pinia、Pinia Colada、i18n、Web routes、Web build/dev 任务。
- 保留 `packages/ui/`、`packages/sdk/`、`packages/icons/`、`packages/config/`。
- 保留 `proto/`、`buf.gen.ts.yaml` 和 ConnectRPC `mise run sdk-generate`。
- 保留生产 Web image `ghcr.io/stringke/web`。
- Web 管理后台监听 `26811`。

### PostgreSQL

- PostgreSQL 是唯一数据库后端。
- 保留 `db/postgres/`。
- 保留 PostgreSQL sqlc 生成链路。
- 迁移逻辑固定走 PostgreSQL。

### Browser Gateway

- 保留 `apps/browser/`。
- 保留 browser automation。
- 保留 `internal/agent/tools/browser.go`。
- 保留 browser context、browser gateway 配置、Playwright 自动化链路。
- Browser Gateway 是 agent 工具能力，不属于 GUI。

### Docker Engine

- 保留 Docker Engine runtime adapter。
- 保留 Docker 配置。
- 保留 Docker Compose 运维命令。
- 不按 `docker` 关键词批量删除。

### containerd

- 保留 containerd runtime adapter。
- 保留 `memoh bots ctr ...`。
- 保留 containerd、OCI image、registry、snapshotter、CNI 等能力。

### 非交互 CLI

- 保留 `cmd/memoh`。
- 保留以下明确子命令：
  - `memoh migrate`
  - `memoh install`
  - `memoh serve`
  - `memoh docker ...`
  - `memoh admin ...`
  - `memoh support ...`
  - `memoh version`
- CLI 不提供 GUI/TUI 交互界面。

### `web-ui` 配置

- 保留配置项 `web-ui`。
- `web-ui` 用于 Web 管理后台监听地址和端口。
- Web Vite dev server 读取 `web-ui` 并代理到 server API。

## 文档与同步原则

- 不盲删 `docker`、`browser`、`web` 字样，逐项确认含义。
- `apps/web` 是保留对象。
- `apps/desktop`、`internal/tui`、`db/sqlite`、`internal/db/sqlite` 是删除对象。
- 上游对 `apps/web` 的管理后台功能改动需要评估并同步。
- 上游 OpenAPI 管理 API 改动同步时需要转写到 ConnectRPC proto、Connect handler 和 `packages/sdk`。
- 上游对 Desktop、TUI、SQLite、Windows、Docker Hub、npmjs、CN 特化的改动继续拒绝。

## 验收标准

- 仓库中不存在 Desktop/Electron 支持入口、依赖、构建任务和文档。
- 仓库中不存在 TUI 入口、TUI 子命令、TUI 依赖和 `internal/tui`。
- 仓库中不存在 SQLite schema、queries、sqlc 生成包、driver 依赖和配置分支。
- `apps/web`、`packages/ui`、`packages/sdk`、`packages/icons`、`packages/config` 存在并通过 Vite+ 检查。
- PostgreSQL 迁移、sqlc、服务启动和 CLI migration 正常工作。
- Browser Gateway 和 agent browser automation 保持可用。
- Docker Engine runtime adapter 和 Docker Compose 运维命令保持可用。
- Podman runtime adapter 保持可用，复用 Podman Docker-compatible API。
- containerd runtime adapter 和 `memoh bots ctr ...` 保持可用。
- `web-ui` 配置项保留。
- Go 编译通过。
