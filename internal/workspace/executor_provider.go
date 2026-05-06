package workspace

import (
	"context"

	"github.com/memohai/memoh/internal/workspace/executorclient"
)

type ExecutorProvider interface {
	ExecutorClient(ctx context.Context, botID string) (*executorclient.Client, error)
	WorkspaceInfo(ctx context.Context, botID string) (executorclient.WorkspaceInfo, error)
}
