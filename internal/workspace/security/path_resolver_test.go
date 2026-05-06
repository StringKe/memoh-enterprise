package security

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestPathResolverRejectsParentEscape(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	resolver, err := NewPathResolver(PathResolverOptions{WorkspaceRoot: root})
	if err != nil {
		t.Fatalf("NewPathResolver: %v", err)
	}

	_, err = resolver.Resolve("../outside.txt")
	if !errors.Is(err, ErrPathEscapesWorkspace) {
		t.Fatalf("Resolve error = %v, want ErrPathEscapesWorkspace", err)
	}
}

func TestPathResolverRejectsAbsoluteEscape(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")
	resolver, err := NewPathResolver(PathResolverOptions{
		WorkspaceRoot:     root,
		AllowHostAbsolute: true,
	})
	if err != nil {
		t.Fatalf("NewPathResolver: %v", err)
	}

	_, err = resolver.Resolve(outside)
	if !errors.Is(err, ErrPathEscapesWorkspace) {
		t.Fatalf("Resolve error = %v, want ErrPathEscapesWorkspace", err)
	}
}

func TestPathResolverRejectsSymlinkEscape(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o600); err != nil {
		t.Fatalf("write outside file: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	resolver, err := NewPathResolver(PathResolverOptions{WorkspaceRoot: root})
	if err != nil {
		t.Fatalf("NewPathResolver: %v", err)
	}

	_, err = resolver.Resolve("link/secret.txt")
	if !errors.Is(err, ErrPathEscapesWorkspace) {
		t.Fatalf("Resolve error = %v, want ErrPathEscapesWorkspace", err)
	}
}

func TestPathResolverMapsDataMount(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	resolver, err := NewPathResolver(PathResolverOptions{
		WorkspaceRoot: root,
		DataMount:     "/data",
	})
	if err != nil {
		t.Fatalf("NewPathResolver: %v", err)
	}

	got, err := resolver.Resolve("/data/notes/demo.txt")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("EvalSymlinks root: %v", err)
	}
	want := filepath.Join(canonicalRoot, "notes", "demo.txt")
	if got != want {
		t.Fatalf("Resolve = %q, want %q", got, want)
	}
}
