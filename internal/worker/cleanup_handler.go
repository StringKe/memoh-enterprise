package worker

import (
	"context"
	"errors"

	"github.com/memohai/memoh/internal/eventbus"
)

func (r *Runtime) handleCleanupRun(ctx context.Context, _ eventbus.Delivery) error {
	if r.deps.Cleanup == nil {
		return errors.New("cleanup runner is not configured")
	}
	return r.deps.Cleanup.Cleanup(ctx)
}
