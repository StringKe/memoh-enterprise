package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestServerRuntimeDoesNotStartSplitServices(t *testing.T) {
	source := readRepoFile(t, "internal/serverruntime/module.go")
	forbidden := []string{
		"integrations.New" + "WebSocketHandler",
		"channel.NewWebhookServerHandler",
		"weixin.NewQRServerHandler",
		"startChannelManager",
		"startScheduleService",
		"startHeartbeatService",
		"startBackgroundTaskCleanup",
	}
	for _, item := range forbidden {
		if strings.Contains(source, item) {
			t.Fatalf("server runtime must not register %s", item)
		}
	}
}

func readRepoFile(t *testing.T, name string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime caller unavailable")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
	data, err := fs.ReadFile(os.DirFS(root), filepath.ToSlash(name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
