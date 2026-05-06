package worker

import (
	"context"
	"errors"

	"github.com/memohai/memoh/internal/eventbus"
)

func (r *Runtime) handleScheduleStartup(ctx context.Context, _ eventbus.Delivery) error {
	if r.deps.Schedule == nil {
		return errors.New("schedule bootstrapper is not configured")
	}
	return r.deps.Schedule.Bootstrap(ctx)
}
