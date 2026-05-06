package worker

import (
	"context"
	"errors"

	"github.com/memohai/memoh/internal/eventbus"
)

func (r *Runtime) handleHeartbeatStartup(ctx context.Context, _ eventbus.Delivery) error {
	if r.deps.Heartbeat == nil {
		return errors.New("heartbeat bootstrapper is not configured")
	}
	return r.deps.Heartbeat.Bootstrap(ctx)
}
