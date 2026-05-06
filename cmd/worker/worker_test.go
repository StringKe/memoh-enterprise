package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWorkerDoesNotRegisterHTTPServer(t *testing.T) {
	source := readCommandFile(t, "module.go")
	forbidden := []string{
		"net/http",
		"ListenAndServe",
		"ServeHTTP",
		"/health",
	}
	for _, item := range forbidden {
		if strings.Contains(source, item) {
			t.Fatalf("worker command must not register HTTP server code: %s", item)
		}
	}
}

func readCommandFile(t *testing.T, name string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime caller unavailable")
	}
	data, err := fs.ReadFile(os.DirFS(filepath.Dir(file)), filepath.ToSlash(name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
