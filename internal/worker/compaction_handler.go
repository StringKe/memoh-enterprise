package worker

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/memohai/memoh/internal/compaction"
	"github.com/memohai/memoh/internal/eventbus"
)

func (r *Runtime) handleCompactionRun(ctx context.Context, delivery eventbus.Delivery) error {
	if r.deps.Compaction == nil {
		return errors.New("compaction runner is not configured")
	}
	var cfg compaction.TriggerConfig
	if len(delivery.PayloadJSON) > 0 {
		if err := json.Unmarshal(delivery.PayloadJSON, &cfg); err != nil {
			return err
		}
	} else if len(delivery.Payload) > 0 {
		if err := json.Unmarshal(delivery.Payload, &cfg); err != nil {
			return err
		}
	}
	if cfg.BotID == "" || cfg.SessionID == "" {
		return errors.New("compaction event requires bot_id and session_id")
	}
	return r.deps.Compaction.RunCompactionSync(ctx, cfg)
}
