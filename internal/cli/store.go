package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	DefaultProdServerURL = "http://127.0.0.1:26810"
	DefaultDevServerURL  = "http://127.0.0.1:26810"
)

type State struct {
	ServerURL string `json:"server_url"`
}

func DefaultState() State {
	return State{ServerURL: DefaultProdServerURL}
}

func LoadState() (State, error) {
	path, err := statePath()
	if err != nil {
		return State{}, err
	}

	data, err := os.ReadFile(path) //nolint:gosec // path is derived from the user's config directory, not arbitrary input
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return DefaultState(), nil
		}
		return State{}, fmt.Errorf("read state: %w", err)
	}

	state := DefaultState()
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, fmt.Errorf("decode state: %w", err)
	}
	state.ServerURL = NormalizeServerURL(state.ServerURL)
	return state, nil
}

func SaveState(state State) error {
	path, err := statePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	if strings.TrimSpace(state.ServerURL) == "" {
		state.ServerURL = DefaultProdServerURL
	}
	state.ServerURL = NormalizeServerURL(state.ServerURL)
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write state: %w", err)
	}
	return nil
}

func NormalizeServerURL(raw string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(raw), "/")
	if trimmed == "" {
		return DefaultProdServerURL
	}
	if !strings.Contains(trimmed, "://") {
		return "http://" + trimmed
	}
	return trimmed
}

func statePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}
	return filepath.Join(dir, "memoh", "cli.json"), nil
}
