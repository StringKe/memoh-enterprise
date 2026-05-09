package connectapi

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWebRuntimeConnectHandlersAreRegistered(t *testing.T) {
	source := readConnectAPIRepoFile(t, "internal/serverruntime/module.go")
	required := []string{
		"provideServerHandler(connectapi.NewBotHandler)",
		"provideServerHandler(connectapi.NewChatHandler)",
		"provideServerHandler(connectapi.NewContainerHandler)",
		"provideServerHandler(connectapi.NewSkillHandler)",
		"provideServerHandler(connectapi.NewChannelHandler)",
		"provideServerHandler(connectapi.NewProviderHandler)",
		"provideServerHandler(connectapi.NewToolApprovalHandler)",
	}
	for _, item := range required {
		if !strings.Contains(source, item) {
			t.Fatalf("server runtime does not register %s", item)
		}
	}
}

func TestWebRuntimeDoesNotUseRemovedRESTStreamingEndpoints(t *testing.T) {
	for _, path := range []string{"apps/web/src", "packages/sdk/src"} {
		for file, content := range readConnectAPITextFiles(t, path) {
			for _, forbidden := range []string{"/local/ws", "/terminal/ws", "/container/fs", "EventSource", "new WebSocket"} {
				if strings.Contains(content, forbidden) {
					t.Fatalf("%s contains removed runtime endpoint %q", file, forbidden)
				}
			}
		}
	}
}

func readConnectAPIRepoFile(t *testing.T, name string) string {
	t.Helper()
	data, err := fs.ReadFile(os.DirFS(connectAPIRepoRoot(t)), filepath.ToSlash(name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func readConnectAPITextFiles(t *testing.T, name string) map[string]string {
	t.Helper()
	root := connectAPIRepoRoot(t)
	out := map[string]string{}
	err := filepath.WalkDir(filepath.Join(root, name), func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case "node_modules", "dist", ".vite", "coverage":
				return filepath.SkipDir
			default:
				return nil
			}
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		data, err := fs.ReadFile(os.DirFS(root), filepath.ToSlash(rel))
		if err != nil {
			return err
		}
		if strings.Contains(string(data), "\x00") {
			return nil
		}
		out[filepath.ToSlash(rel)] = string(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func connectAPIRepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime caller unavailable")
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
