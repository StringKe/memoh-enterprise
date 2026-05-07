# Upstream Sync Guide

## 目的

当前仓库是 `memoh` 的 enterprise fork。`main` 分支面向企业版长期维护，仍需要定期从上游同步后端、agent、container runtime、Browser Gateway、Web 管理后台、provider、memory、MCP、channel 等通用能力。

`.parent-commit` 用来记录当前 enterprise 分支已经对齐过的上游 commit。它不是 Git 子模块，也不是构建输入，只是同步基线文件。

## `.parent-commit` 语义

`.parent-commit` 必须只包含一行完整 commit SHA：

```text
ce7bd4adf9a9bf9e7e44eafb0ea3e9cce8bdac93
```

含义：

- 这个 SHA 是上一次完成同步时的上游基线。
- 下一次同步时，从这个 SHA 到新的上游目标 SHA 之间的 diff 就是待评估范围。
- 同步完成并通过验证后，才把 `.parent-commit` 更新成新的上游目标 SHA。
- 如果同步中止、回滚、未验证通过，不更新 `.parent-commit`。

## 远端约定

本仓库 `origin` 指向 enterprise 仓库：

```bash
git remote set-url origin git@github.com:StringKe/memoh-enterprise.git
```

建议把原始 `memoh` 仓库配置为 `upstream`：

```bash
git remote add upstream <UPSTREAM_MEMOH_REPO_URL>
git fetch upstream
```

`<UPSTREAM_MEMOH_REPO_URL>` 必须替换成实际上游仓库地址。

## 同步前检查

```bash
git status --short
cat .parent-commit
git fetch upstream
git log --oneline "$(cat .parent-commit)"..upstream/main
```

同步前工作区必须干净。`git log` 输出就是本轮要评估的上游提交范围。

## 同步原则

同步时按 enterprise scope 过滤上游变化：

- 接收：Go server、agent、MCP、memory、schedule、providers、models、channels、email、workspace、container runtime、Browser Gateway、Web 管理后台、PostgreSQL、ConnectRPC proto、TypeScript SDK、非交互 CLI。
- 接收：containerd、Docker Engine、Podman 相关修复。
- 接收：`apps/web`、`packages/ui`、`packages/sdk`、`packages/icons`、`packages/config` 中服务 Web 管理后台和 Browser Gateway 的改动。
- 接收：`web-ui` 配置字段。
- 接收：容器内 workspace display、Xvnc、WebRTC 等服务 Web 管理后台和 workspace 操作的能力；同步时使用 display/session 语义，不恢复 Electron Desktop 产品形态。
- 拒绝：Desktop/Electron。
- 拒绝：TUI。
- 拒绝：SQLite schema、queries、driver、迁移、配置和文档。

不要按关键词批量拒绝 `docker`、`browser`、`web`：

- `docker` 可能是 Docker Engine runtime，必须保留。
- `browser` 可能是 Browser Gateway 或 agent browser automation，必须保留。
- `web-ui` 是 Web 管理后台配置，必须保留。
- `web` 如果指 `apps/web` 管理后台、SDK、UI package、icon package 或 Web image，则必须保留。
- `desktop` 才是 Desktop GUI 删除范围。

## 推荐同步流程

1. 新建同步分支：

```bash
git switch main
git pull --ff-only origin main
git switch -c sync/upstream-<YYYYMMDD>
git fetch upstream
```

2. 确认同步范围：

```bash
BASE="$(cat .parent-commit)"
TARGET="upstream/main"
git log --oneline "$BASE..$TARGET"
git diff --name-status "$BASE..$TARGET"
```

3. 先评估文件路径：

```bash
git diff --name-status "$BASE..$TARGET" -- \
  ':!apps/desktop' \
  ':!db/sqlite' \
  ':!internal/db/sqlite'
```

这个命令只用于缩小评估范围，不代表可以直接套用全部 diff。每个冲突都要按 enterprise scope 判断。

4. 应用上游变化。

优先使用 cherry-pick 小批量同步：

```bash
git cherry-pick <commit-sha>
```

遇到包含已删除范围的提交时，拆分处理：

```bash
git show --name-status <commit-sha>
git cherry-pick --no-commit <commit-sha>
```

然后删除不该进入 enterprise 的变更，只保留后端、PostgreSQL、Browser Gateway、containerd、Docker Engine、Podman、非交互 CLI 等允许范围。

5. 冲突处理后执行残留审计：

```bash
find . \( -path './.git' -o -path './node_modules' \) -prune -o -type d \( \
  -path './apps/desktop' -o \
  -path './internal/tui' -o \
  -path './db/sqlite' -o \
  -path './internal/db/sqlite' -o \
  -path './internal/embedded' \
\) -print

rg -n "apps/desktop|internal/tui|db/sqlite|internal/db/sqlite|internal/embedded|electron-builder|electron-vite|modernc\\.org/sqlite|golang-migrate/migrate/v4/database/sqlite|@stringke/desktop|@memohai/desktop|dev:sqlite|desktop:|docker-compose\\.sqlite|\\[sqlite\\]" \
  -g '!pnpm-lock.yaml' \
  -g '!go.sum' \
  -g '!docs/enterprise-scope.md' \
  -g '!docs/upstream-sync.md' \
  .
```

两个命令都不应该输出需要恢复的活跃支持入口。`apps/web`、`packages/ui`、`packages/sdk`、`packages/icons`、`packages/config` 是保留对象，不属于残留删除审计。

6. 运行验证：

```bash
go test ./cmd/... ./internal/...
go build ./cmd/server ./cmd/memoh ./cmd/workspace-executor
sqlc generate
vp check
vp test
vp run --filter @stringke/web build
vp run --filter @stringke/docs build
```

Browser Gateway 改动后增加：

```bash
cd apps/browser
bun build src/index.ts --outfile dist/index.js --target bun --minify --external playwright --external playwright-core
```

7. 更新 `.parent-commit`：

```bash
git rev-parse upstream/main > .parent-commit
```

只在同步完成、残留审计通过、验证通过后执行。

8. 提交同步结果：

```bash
git add -A
git commit -m "chore: 同步上游变更"
git push origin sync/upstream-<YYYYMMDD>
```

## 冲突处理规则

- 上游恢复 Desktop、TUI、SQLite 文件时，enterprise 侧继续删除。
- 上游修改 `apps/web` 管理后台时，按企业版 Web 管理后台能力同步。
- 上游修改 shared config 时，保留 PostgreSQL、containerd、Docker Engine、Podman、Browser Gateway、Web 管理后台、`web-ui`，删除 SQLite、Desktop 分支。
- 上游修改 CLI root 行为时，`memoh` 无参数仍只显示 help，不进入 TUI。
- 上游新增数据库变更时，只迁移到 `db/postgres/` 和 PostgreSQL sqlc。
- 上游新增 Browser Gateway 或 browser automation 能力时，按 agent 工具能力接收。
- 上游新增容器内 VNC/display 能力时，按 workspace display 接收，保留 Browser Gateway，并把 OpenAPI/REST 入口转写为 ConnectRPC。
- 上游新增 Docker Compose Web 管理后台 service 时按 GHCR 发布要求同步；新增 server、postgres、browser、qdrant、sparse、containerd 相关服务时逐项评估。

## 完成标准

- `.parent-commit` 指向本轮已同步且已验证的上游 commit。
- `origin/main` 包含同步结果。
- 残留审计没有恢复 Desktop GUI、TUI、SQLite 支持入口。
- PostgreSQL、server、CLI、Web 管理后台、Browser Gateway、containerd、Docker Engine、Podman 验证通过。
