# Upstream Sync Guide

## 目的

当前仓库是 `memoh` 的 enterprise fork。`main` 分支面向企业版长期维护，仍需要定期从上游同步后端、agent、container runtime、Browser Gateway、provider、memory、MCP、channel 等通用能力。

`.parent-commit` 用来记录当前 enterprise 分支已经对齐过的上游 commit。它不是 Git 子模块，也不是构建输入，只是同步基线文件。

## `.parent-commit` 语义

`.parent-commit` 必须只包含一行完整 commit SHA：

```text
4509b6db06a888e2688103e7ffb7a4e4b9a60863
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

- 接收：Go server、agent、MCP、memory、schedule、providers、models、channels、email、workspace、container runtime、Browser Gateway、PostgreSQL、OpenAPI 后端文档、非交互 CLI。
- 接收：Docker Engine 和 containerd 相关修复。
- 接收：`web-ui` 配置兼容字段。
- 拒绝：Desktop/Electron。
- 拒绝：内置 Web GUI 实现和静态资源嵌入。
- 拒绝：TUI。
- 拒绝：SQLite schema、queries、driver、迁移、配置和文档。
- 拒绝：只服务内置 Web GUI 的 TypeScript SDK、UI package、icon package、frontend build/release 链路。

不要按关键词批量拒绝 `docker`、`browser`、`web`：

- `docker` 可能是 Docker Engine runtime，必须保留。
- `browser` 可能是 Browser Gateway 或 agent browser automation，必须保留。
- `web-ui` 是外部 Web UI 兼容配置，必须保留。
- `web` 如果指内置 GUI 实现、embedded assets、frontend package，则拒绝。

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
  ':!apps/web' \
  ':!packages/ui' \
  ':!packages/sdk' \
  ':!packages/icons' \
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

然后删除不该进入 enterprise 的变更，只保留后端、PostgreSQL、Browser Gateway、Docker Engine、containerd、非交互 CLI 等允许范围。

5. 冲突处理后执行残留审计：

```bash
find . \( -path './.git' -o -path './node_modules' \) -prune -o -type d \( \
  -path './apps/desktop' -o \
  -path './apps/web' -o \
  -path './internal/tui' -o \
  -path './db/sqlite' -o \
  -path './internal/db/sqlite' -o \
  -path './packages/ui' -o \
  -path './packages/sdk' -o \
  -path './packages/icons' -o \
  -path './internal/embedded' \
\) -print

rg -n "apps/(desktop|web)|packages/(ui|sdk|icons)|internal/tui|db/sqlite|internal/db/sqlite|internal/embedded|electron-builder|electron-vite|modernc\\.org/sqlite|golang-migrate/migrate/v4/database/sqlite|@memohai/(desktop|web|ui|sdk|icon)|openapi-ts|build-embedded-assets|sdk-generate|icons-generate|dev:sqlite|desktop:|docker-compose\\.sqlite|Dockerfile\\.web|memohai/web|\\[sqlite\\]|\\[web\\]" \
  -g '!pnpm-lock.yaml' \
  -g '!go.sum' \
  -g '!docs/enterprise-scope.md' \
  -g '!docs/upstream-sync.md' \
  .
```

两个命令都不应该输出需要恢复的活跃支持入口。文档中描述“已移除范围”的文字可以保留。

6. 运行验证：

```bash
go test ./cmd/... ./internal/...
go build ./cmd/agent ./cmd/memoh ./cmd/bridge
sqlc generate
node_modules/.bin/tsc --noEmit -p tsconfig.json
node_modules/.bin/eslint .
pnpm --filter @memohai/docs build
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

- 上游恢复 Desktop、Web GUI、TUI、SQLite 文件时，enterprise 侧继续删除。
- 上游修改 shared config 时，保留 PostgreSQL、Docker Engine、containerd、Browser Gateway、`web-ui`，删除 SQLite、Desktop、内置 Web GUI 分支。
- 上游修改 CLI root 行为时，`memoh` 无参数仍只显示 help，不进入 TUI。
- 上游新增数据库变更时，只迁移到 `db/postgres/` 和 PostgreSQL sqlc。
- 上游新增 Browser Gateway 或 browser automation 能力时，按 agent 工具能力接收。
- 上游新增 Docker Compose Web GUI service 时拒绝；新增 server、postgres、browser、qdrant、sparse、containerd 相关服务时逐项评估。

## 完成标准

- `.parent-commit` 指向本轮已同步且已验证的上游 commit。
- `origin/main` 包含同步结果。
- 残留审计没有恢复 Desktop、内置 Web GUI、TUI、SQLite 支持入口。
- PostgreSQL、server、CLI、Browser Gateway、Docker Engine、containerd 验证通过。
