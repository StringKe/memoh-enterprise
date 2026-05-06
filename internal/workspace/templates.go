package workspace

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed templates/*
var executorTemplates embed.FS

func seedExecutorTemplates(dstDir string) error {
	if err := os.MkdirAll(dstDir, 0o750); err != nil {
		return err
	}
	entries, err := fs.ReadDir(executorTemplates, "templates")
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		dst := filepath.Join(dstDir, entry.Name())
		if _, err := os.Stat(dst); err == nil {
			continue
		}
		data, err := executorTemplates.ReadFile("templates/" + entry.Name())
		if err != nil {
			return err
		}
		if err := os.WriteFile(dst, data, 0o600); err != nil {
			return err
		}
	}
	return nil
}
