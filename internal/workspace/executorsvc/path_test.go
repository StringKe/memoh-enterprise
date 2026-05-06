package executorsvc

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"connectrpc.com/connect"

	workspacev1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/workspace/v1"
)

func TestLocalPathResolverMapsDataMountToWorkspaceRoot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	srv := New(Options{
		DefaultWorkDir:    root,
		WorkspaceRoot:     root,
		DataMount:         "/data",
		AllowHostAbsolute: true,
	})

	if _, err := srv.WriteFile(context.Background(), connect.NewRequest(&workspacev1.WriteFileRequest{
		Path:             "/data/notes/demo.txt",
		Content:          []byte("demo"),
		CreateParentDirs: true,
	})); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(root, "notes", "demo.txt")) //nolint:gosec // test path is under t.TempDir
	if err != nil {
		t.Fatalf("read mapped file failed: %v", err)
	}
	if string(got) != "demo" {
		t.Fatalf("mapped file = %q, want demo", string(got))
	}
}

func TestLocalPathResolverRejectsHostAbsoluteEscape(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")
	srv := New(Options{
		DefaultWorkDir:    root,
		WorkspaceRoot:     root,
		DataMount:         "/data",
		AllowHostAbsolute: true,
	})

	if _, err := srv.WriteFile(context.Background(), connect.NewRequest(&workspacev1.WriteFileRequest{
		Path:             outside,
		Content:          []byte("outside"),
		CreateParentDirs: true,
	})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("WriteFile status = %s, want %s", connect.CodeOf(err), connect.CodePermissionDenied)
	}
}
