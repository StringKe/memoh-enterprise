package agent

import (
	"log/slog"

	"github.com/memohai/memoh/internal/workspace/executorclient"
)

// Deps holds all service dependencies for the Agent.
type Deps struct {
	WorkspaceExecutorProvider executorclient.Provider
	Logger                    *slog.Logger
}
