package security

import (
	"context"
	"net"
	"os"
	"path/filepath"
)

const (
	SocketDirMode  os.FileMode = 0o700
	SocketFileMode os.FileMode = 0o600
)

func PrepareUnixSocket(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), SocketDirMode); err != nil {
		return err
	}
	if err := os.Chmod(filepath.Dir(path), SocketDirMode); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func ListenUnixSocket(path string) (net.Listener, error) {
	if err := PrepareUnixSocket(path); err != nil {
		return nil, err
	}
	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "unix", path)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, SocketFileMode); err != nil {
		_ = listener.Close()
		return nil, err
	}
	return listener, nil
}
