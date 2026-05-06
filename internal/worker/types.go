package worker

import (
	"context"
	"log/slog"

	"github.com/memohai/memoh/internal/compaction"
	"github.com/memohai/memoh/internal/eventbus"
)

const (
	TopicScheduleStartup  = "worker.schedule.startup"
	TopicHeartbeatStartup = "worker.heartbeat.startup"
	TopicCompactionRun    = "worker.compaction.run"
	TopicCleanupRun       = "worker.cleanup.run"
)

type EventConsumer interface {
	RegisterHandler(topic string, handler eventbus.Handler)
	Run(ctx context.Context) error
}

type Bootstrapper interface {
	Bootstrap(ctx context.Context) error
}

type CompactionRunner interface {
	RunCompactionSync(ctx context.Context, cfg compaction.TriggerConfig) error
}

type CleanupRunner interface {
	Cleanup(ctx context.Context) error
}

type Dependencies struct {
	Consumer   EventConsumer
	Schedule   Bootstrapper
	Heartbeat  Bootstrapper
	Compaction CompactionRunner
	Cleanup    CleanupRunner
	Logger     *slog.Logger
}
