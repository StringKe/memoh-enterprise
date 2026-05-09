package tools

import (
	"context"
	"log/slog"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/settings"
	"github.com/memohai/memoh/internal/workspace/executorclient"
)

// BrowserProvider previously exposed browser_action / browser_observe /
// browser_remote_session tools backed by the standalone Browser Gateway
// (apps/browser). The enterprise fork removed Browser Gateway in favour of
// running the agent browser inside the workspace container itself, reached
// over the workspace executor tunnel. The CDP-backed implementation is being
// rebuilt on top of executor.Tunnel and is not wired up yet, so this provider
// intentionally exposes no tools today.
type BrowserProvider struct {
	logger     *slog.Logger
	settings   *settings.Service
	containers executorclient.Provider
}

// NewBrowserProvider constructs a BrowserProvider stub that exposes no tools.
// Once executor Tunnel + CDP browser lands, this constructor will gain back
// its CDP-related parameters.
func NewBrowserProvider(log *slog.Logger, settingsSvc *settings.Service, containers executorclient.Provider) *BrowserProvider {
	if log == nil {
		log = slog.Default()
	}
	return &BrowserProvider{
		logger:     log.With(slog.String("tool", "browser")),
		settings:   settingsSvc,
		containers: containers,
	}
}

// Tools returns no browser tools while the in-workspace CDP implementation is
// pending. The signature matches the other ToolProvider implementations so
// agent runtime wiring can keep referencing this provider unchanged.
func (*BrowserProvider) Tools(_ context.Context, _ SessionContext) ([]sdk.Tool, error) {
	return nil, nil
}
