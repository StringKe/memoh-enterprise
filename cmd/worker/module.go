package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"

	"github.com/memohai/memoh/internal/agent/background"
	"github.com/memohai/memoh/internal/boot"
	"github.com/memohai/memoh/internal/compaction"
	"github.com/memohai/memoh/internal/config"
	"github.com/memohai/memoh/internal/db"
	postgresstore "github.com/memohai/memoh/internal/db/postgres/store"
	"github.com/memohai/memoh/internal/eventbus"
	"github.com/memohai/memoh/internal/heartbeat"
	"github.com/memohai/memoh/internal/logger"
	"github.com/memohai/memoh/internal/schedule"
	sessionpkg "github.com/memohai/memoh/internal/session"
	"github.com/memohai/memoh/internal/worker"
)

func run(parent context.Context) error {
	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load(os.Getenv("CONFIG_PATH"))
	if err != nil {
		return err
	}
	logger.Init(cfg.Log.Level, cfg.Log.Format)

	conn, err := db.Open(ctx, cfg)
	if err != nil {
		return err
	}
	defer conn.Close()

	store, err := postgresstore.New(conn)
	if err != nil {
		return err
	}
	queries := postgresstore.NewQueries(store.SQLC())
	runtimeConfig, err := boot.ProvideRuntimeConfig(cfg)
	if err != nil {
		return err
	}
	producer := eventbus.NewProducer(queries)
	sessionService := sessionpkg.NewService(logger.L, queries)
	sessionCreator := &workerSessionCreator{svc: sessionService}
	triggerer := &workerRunTriggerer{producer: producer}
	scheduleService := schedule.NewService(logger.L, queries, triggerer, sessionCreator, runtimeConfig)
	heartbeatService := heartbeat.NewService(logger.L, queries, triggerer, sessionCreator, runtimeConfig)
	compactionService := compaction.NewService(logger.L, queries)
	cleanup := &backgroundCleanupRunner{manager: background.New(logger.L)}
	consumerName := cfg.Internal.WorkerName
	if consumerName == "" {
		consumerName = "memoh-worker"
	}
	consumer := eventbus.NewConsumer(queries, consumerName, uuid.NewString())
	runtime := worker.NewRuntime(worker.Dependencies{
		Consumer:   consumer,
		Schedule:   scheduleService,
		Heartbeat:  heartbeatService,
		Compaction: compactionService,
		Cleanup:    cleanup,
		Logger:     logger.L,
	})
	if runtime == nil {
		return errors.New("worker runtime is not configured")
	}
	logger.L.InfoContext(ctx, "worker runtime starting")
	return runtime.Run(ctx)
}

type workerSessionCreator struct {
	svc *sessionpkg.Service
}

func (c *workerSessionCreator) CreateSession(ctx context.Context, botID, sessionType string) (string, error) {
	session, err := c.svc.Create(ctx, sessionpkg.CreateInput{
		BotID: botID,
		Type:  sessionType,
	})
	if err != nil {
		return "", err
	}
	return session.ID, nil
}

type workerRunTriggerer struct {
	producer *eventbus.Producer
}

func (t *workerRunTriggerer) TriggerSchedule(ctx context.Context, botID string, payload schedule.TriggerPayload, token string) (schedule.TriggerResult, error) {
	if err := t.publish(ctx, "schedule", botID, token, payload); err != nil {
		return schedule.TriggerResult{}, err
	}
	return schedule.TriggerResult{Status: "queued"}, nil
}

func (t *workerRunTriggerer) TriggerHeartbeat(ctx context.Context, botID string, payload heartbeat.TriggerPayload, token string) (heartbeat.TriggerResult, error) {
	if err := t.publish(ctx, "heartbeat", botID, token, payload); err != nil {
		return heartbeat.TriggerResult{}, err
	}
	return heartbeat.TriggerResult{Status: "queued", SessionID: payload.SessionID}, nil
}

func (t *workerRunTriggerer) publish(ctx context.Context, source, botID, token string, payload any) error {
	if t == nil || t.producer == nil {
		return errors.New("worker run producer is not configured")
	}
	body, err := json.Marshal(map[string]any{
		"source":  source,
		"bot_id":  botID,
		"token":   token,
		"payload": payload,
	})
	if err != nil {
		return err
	}
	_, err = t.producer.Publish(ctx, eventbus.Event{
		Topic:          "runner.run.requested",
		PayloadType:    "memoh.runner.RunRequest",
		Payload:        body,
		PayloadJSON:    body,
		IdempotencyKey: "runner-run:" + uuid.NewString(),
		AggregateType:  "bot",
		PartitionKey:   botID,
	}, []string{"memoh-agent-runner"})
	return err
}

type backgroundCleanupRunner struct {
	manager *background.Manager
}

func (r *backgroundCleanupRunner) Cleanup(context.Context) error {
	if r == nil || r.manager == nil {
		return nil
	}
	r.manager.Cleanup(24 * time.Hour)
	return nil
}
