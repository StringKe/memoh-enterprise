# Memoh Enterprise Scope

## 背景

当前仓库是 `memoh` 的 enterprise fork，目标是面向企业领域维护一个完全开源版本。后续仍会从上游 `memoh` 同步相关功能，因此本次收敛必须保持边界清晰，降低长期同步冲突。

上游同步基线记录在仓库根目录 `.parent-commit`。同步流程见 `docs/upstream-sync.md`。

## 总目标

将项目收敛为 enterprise 后端版：

- 移除 Desktop 支持。
- 移除内置 Web GUI 实现。
- 移除终端 TUI。
- 移除 SQLite 支持。
- 运行目标只支持 Linux `amd64` / `arm64`，macOS 保留本地开发兼容。
- 保留 PostgreSQL、Docker Engine、containerd、Browser Gateway、browser automation、非交互 CLI 和核心后端能力。
- 保留配置中的 `web-ui`，但仓库不再内置 Web GUI 实现。

## 删除范围

### Desktop

- 删除 Electron desktop app。
- 删除 `apps/desktop/`。
- 删除 `@memohai/desktop`、`electron`、`electron-vite`、`electron-builder` 等 desktop 专用依赖。
- 删除 desktop dev/build/release/docs 配置。

### Web GUI 实现

- 删除内置 Web 管理端 GUI 代码。
- 删除 `apps/web/`。
- 删除只服务 GUI 的 frontend packages、构建配置、Vite/Vue/Tailwind/Pinia 等依赖链。
- 删除 web embedded assets 构建链路。
- 删除服务端内置 Web 静态资源 handler。
- 删除内置 Web GUI 的开发、构建、发布和文档说明。

### TUI

- 删除终端 TUI。
- `memoh` 不带子命令时不再进入 TUI。
- 删除 `memoh tui` 子命令。
- 删除 `internal/tui/`。
- 删除 Bubble Tea、Glamour、Lipgloss 等 TUI 专用依赖。
- CLI 仍需的 server URL/token state 迁移到非交互 CLI state 包。

### SQLite

- 删除 `db/sqlite/`。
- 删除 `internal/db/sqlite/`。
- 删除 SQLite driver `modernc.org/sqlite`。
- 删除配置里的 `[sqlite]`。
- 删除 database backend switch 中的 SQLite 分支。
- 删除 SQLite migration/dev 命令与文档。
- 后续 SQL schema/query 只维护 PostgreSQL。

## 保留范围

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
- 只删除明确属于 Desktop/Web GUI 打包或运行的 Docker 内容。

### containerd

- 保留 containerd runtime adapter。
- 保留 `memoh bots ctr ...`。
- 保留 containerd、OCI image、registry、snapshotter、CNI 等能力。

### 非交互 CLI

- 保留 `cmd/memoh`。
- 保留以下明确子命令：
  - `memoh migrate`
  - `memoh install`
  - `memoh login`
  - `memoh chat`
  - `memoh bots`
  - `memoh compose ...`
  - `memoh version`
- CLI 不提供 GUI/TUI 交互界面。

### 核心后端

- 保留 `cmd/agent` / `memoh-server`。
- 保留 Go server。
- 保留 agent、MCP、memory、schedule、providers、models、channels、email、workspace、containers、browser automation。
- 保留 Swagger/OpenAPI 后端 API 文档。
- 删除只服务内置 Web GUI 的 TypeScript SDK 生成链路。

### 平台目标

- 发布和部署目标只覆盖 Linux `amd64` / `arm64`。
- macOS 只作为本地开发兼容目标保留。
- 不提供 Windows 配置模板、发布目标或安装说明。

### `web-ui` 配置

- 保留配置项 `web-ui`。
- 保留配置结构、配置文件模板、环境变量和文档中用于声明 `web-ui` 的字段。
- 仓库不再内置 Web GUI 实现。
- `web-ui` 用作外部 Web UI 地址、开关、反向代理目标或兼容配置，具体语义以现有代码读取结果为准。

## 配置与命令收敛

- 删除 desktop、内置 web 实现、tui、sqlite 相关配置和任务。
- 保留 `web-ui` 配置。
- 保留 server、auth、postgres、docker、containerd、qdrant、sparse、browser_gateway、registry、supermarket 等后端配置。
- 删除 `mise` 中 desktop/web 构建/sqlite/tui 专用任务。
- 保留 Docker Engine 和 Docker Compose 相关任务。

## 文档收敛

- 删除 Desktop App 章节。
- 删除内置 Web GUI 开发/构建说明。
- 删除 SQLite 说明。
- 删除 TUI 说明。
- 保留 `web-ui` 配置说明。
- 保留 server、CLI、PostgreSQL、Docker Engine、containerd、Browser Gateway、agent tools 说明。

## 实施原则

- 先全库搜索引用，读文件再改文件。
- 不盲删 `docker`、`browser`、`web` 字样，逐项确认含义。
- 分阶段删除：
  1. Desktop/Web GUI 实现
  2. TUI
  3. SQLite
  4. 配置/命令/文档
  5. 依赖清理
  6. 编译与测试修复
- 每阶段运行编译检查，修复所有错误。
- 不做未请求的架构重写。

## 验收标准

- 仓库中不存在 Desktop/Electron 支持入口、依赖、构建任务和文档。
- 仓库中不存在内置 Web GUI 实现、构建任务和静态资源服务入口。
- 仓库中不存在 TUI 入口、TUI 子命令、TUI 依赖和 `internal/tui`。
- 仓库中不存在 SQLite schema、queries、sqlc 生成包、driver 依赖和配置分支。
- PostgreSQL 迁移、sqlc、服务启动和 CLI migration 正常工作。
- Browser Gateway 和 agent browser automation 保持可用。
- Docker Engine runtime adapter 和 Docker Compose 运维命令保持可用。
- containerd runtime adapter 和 `memoh bots ctr ...` 保持可用。
- `web-ui` 配置项保留。
- Go 编译通过。
