package security

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestListenUnixSocketModes(t *testing.T) {
	t.Parallel()

	socketDir := filepath.Join("/tmp", fmt.Sprintf("memoh-uds-%d-%d", os.Getpid(), time.Now().UnixNano()))
	t.Cleanup(func() { _ = os.RemoveAll(socketDir) })
	socketPath := filepath.Join(socketDir, "workspace-executor.sock")
	listener, err := ListenUnixSocket(socketPath)
	if err != nil {
		t.Fatalf("ListenUnixSocket: %v", err)
	}
	defer func() { _ = listener.Close() }()

	dirInfo, err := os.Stat(filepath.Dir(socketPath))
	if err != nil {
		t.Fatalf("stat socket dir: %v", err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("socket dir mode = %#o, want 0700", got)
	}

	socketInfo, err := os.Stat(socketPath)
	if err != nil {
		t.Fatalf("stat socket: %v", err)
	}
	if got := socketInfo.Mode().Perm(); got != 0o600 {
		t.Fatalf("socket mode = %#o, want 0600", got)
	}
}
