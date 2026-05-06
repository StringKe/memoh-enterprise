package architecture

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unicode/utf8"
)

func repoRoot(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve caller")
	}

	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

func walkTextFiles(t *testing.T, roots ...string) map[string]string {
	t.Helper()

	root := repoRoot(t)
	files := make(map[string]string)
	for _, scanRoot := range roots {
		absRoot := filepath.Join(root, scanRoot)
		if _, err := os.Stat(absRoot); os.IsNotExist(err) {
			continue
		}
		err := filepath.WalkDir(absRoot, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if shouldSkipDir(d.Name()) {
					return filepath.SkipDir
				}
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			data, err := fs.ReadFile(os.DirFS(root), rel)
			if err != nil {
				return err
			}
			if !utf8.Valid(data) || strings.Contains(string(data), "\x00") {
				return nil
			}
			files[filepath.ToSlash(rel)] = string(data)
			return nil
		})
		if err != nil {
			t.Fatalf("scan %s: %v", scanRoot, err)
		}
	}
	return files
}

func shouldSkipDir(name string) bool {
	switch name {
	case ".git", "node_modules", "dist", "tmp", ".turbo", ".vite", "coverage":
		return true
	default:
		return false
	}
}

func assertNoTextMatches(t *testing.T, roots []string, forbidden []string, allow func(path string) bool) {
	t.Helper()

	for path, content := range walkTextFiles(t, roots...) {
		if allow != nil && allow(path) {
			continue
		}
		for _, token := range forbidden {
			if strings.Contains(content, token) {
				t.Fatalf("%s contains forbidden token %q", path, token)
			}
		}
	}
}

func assertNoImports(t *testing.T, roots []string, forbidden []string) {
	t.Helper()

	assertNoTextMatches(t, roots, forbidden, func(path string) bool {
		return !strings.HasSuffix(path, ".go")
	})
}

func TestAgentRunnerImportBoundary(t *testing.T) {
	assertNoImports(t, []string{"internal/runner", "cmd/agent-runner"}, []string{
		"github.com/memohai/memoh/internal/db/",
		"github.com/memohai/memoh/internal/container/",
		"github.com/memohai/memoh/internal/channel/adapters/",
		"github.com/memohai/memoh/internal/serverruntime",
	})
}

func TestServerConnectorImportBoundary(t *testing.T) {
	assertNoImports(t, []string{"cmd/server", "internal/serverruntime"}, []string{
		"github.com/memohai/memoh/internal/channel/adapters/dingtalk",
		"github.com/memohai/memoh/internal/channel/adapters/discord",
		"github.com/memohai/memoh/internal/channel/adapters/feishu",
		"github.com/memohai/memoh/internal/channel/adapters/matrix",
		"github.com/memohai/memoh/internal/channel/adapters/misskey",
		"github.com/memohai/memoh/internal/channel/adapters/qq",
		"github.com/memohai/memoh/internal/channel/adapters/slack",
		"github.com/memohai/memoh/internal/channel/adapters/telegram",
		"github.com/memohai/memoh/internal/channel/adapters/wechatoa",
		"github.com/memohai/memoh/internal/channel/adapters/wecom",
		"github.com/memohai/memoh/internal/channel/adapters/weixin",
		"integrations.NewWebSocketHandler",
	})
}

func TestServerRuntimeDoesNotOwnAgentLoop(t *testing.T) {
	assertNoTextMatches(t, []string{"cmd/server", "internal/serverruntime"}, []string{
		"provideAgent",
		"provideChatResolver",
		"provideToolProviders",
		"flow.NewScheduleGateway",
		"flow.NewHeartbeatGateway",
		"flow.NewEmailChatGateway",
		".StreamChatWS(",
	}, func(path string) bool {
		return !strings.HasSuffix(path, ".go")
	})
}

func TestConnectChatDoesNotDependOnFlowResolver(t *testing.T) {
	assertNoTextMatches(t, []string{"internal/connectapi"}, []string{
		"github.com/memohai/memoh/internal/conversation/flow",
		"resolver *flow.Resolver",
		"resolver.StreamChat(",
	}, func(path string) bool {
		return !strings.HasSuffix(path, ".go") || strings.Contains(path, "tool_approval")
	})
}

func TestChannelInboundDoesNotDependOnFlowRuntime(t *testing.T) {
	assertNoImports(t, []string{"internal/channel/inbound"}, []string{
		"github.com/memohai/memoh/internal/conversation/flow",
	})
}

func TestWorkspaceExecutorImportBoundary(t *testing.T) {
	assertNoImports(t, []string{"cmd/workspace-executor", "internal/workspace/executorsvc", "internal/workspace/executorclient"}, []string{
		"github.com/memohai/memoh/internal/agent/",
		"github.com/memohai/memoh/internal/channel/",
		"github.com/memohai/memoh/internal/db/",
		"github.com/memohai/memoh/internal/providers/",
		"github.com/memohai/memoh/internal/integrations",
	})
}

func TestWebUIStreamingBoundary(t *testing.T) {
	assertNoTextMatches(t, []string{"apps/web/src"}, []string{
		"new WebSocket",
		"EventSource",
		"text/event-stream",
	}, nil)
}

func TestWorkspaceExecutorNamingBoundary(t *testing.T) {
	assertNoTextMatches(t, []string{"cmd", "internal", "deploy", "conf", "scripts", "docs", "README.md"}, []string{
		"bridge" + "pb",
		"bridge" + "svc",
		"cmd/" + "bridge",
		"/opt/memoh/" + "bridge",
		"/run/memoh/" + "bridge.sock",
		"/run/memoh/" + "executorclient.sock",
		"BRIDGE" + "_SOCKET_PATH",
	}, func(path string) bool {
		return strings.HasPrefix(path, "docs/sdlc/")
	})
}
