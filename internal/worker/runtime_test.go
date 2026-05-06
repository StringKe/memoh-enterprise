package worker

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/memohai/memoh/internal/compaction"
	"github.com/memohai/memoh/internal/eventbus"
)

func TestRuntimeRegistersHandlersAndDispatches(t *testing.T) {
	consumer := newFakeConsumer()
	schedule := &fakeBootstrapper{}
	heartbeat := &fakeBootstrapper{}
	compactor := &fakeCompactionRunner{}
	cleanup := &fakeCleanupRunner{}
	runtime := NewRuntime(Dependencies{
		Consumer:   consumer,
		Schedule:   schedule,
		Heartbeat:  heartbeat,
		Compaction: compactor,
		Cleanup:    cleanup,
	})
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := consumer.handlers[TopicScheduleStartup](context.Background(), eventbus.Delivery{}); err != nil {
		t.Fatal(err)
	}
	if err := consumer.handlers[TopicHeartbeatStartup](context.Background(), eventbus.Delivery{}); err != nil {
		t.Fatal(err)
	}
	if err := consumer.handlers[TopicCompactionRun](context.Background(), eventbus.Delivery{
		Payload: []byte(`{"BotID":"bot-1","SessionID":"session-1"}`),
	}); err != nil {
		t.Fatal(err)
	}
	if err := consumer.handlers[TopicCleanupRun](context.Background(), eventbus.Delivery{}); err != nil {
		t.Fatal(err)
	}
	if schedule.count != 1 || heartbeat.count != 1 || compactor.count != 1 || cleanup.count != 1 {
		t.Fatalf("handler counts schedule=%d heartbeat=%d compaction=%d cleanup=%d", schedule.count, heartbeat.count, compactor.count, cleanup.count)
	}
	if err := runtime.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeGracefulShutdown(t *testing.T) {
	consumer := newFakeConsumer()
	runtime := NewRuntime(Dependencies{Consumer: consumer})
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := runtime.Stop(stopCtx); err != nil {
		t.Fatal(err)
	}
	if !consumer.stopped {
		t.Fatal("consumer did not observe cancellation")
	}
}

type fakeConsumer struct {
	handlers map[string]eventbus.Handler
	stopped  bool
	mu       sync.Mutex
}

func newFakeConsumer() *fakeConsumer {
	return &fakeConsumer{handlers: map[string]eventbus.Handler{}}
}

func (f *fakeConsumer) RegisterHandler(topic string, handler eventbus.Handler) {
	f.handlers[topic] = handler
}

func (f *fakeConsumer) Run(ctx context.Context) error {
	<-ctx.Done()
	f.mu.Lock()
	f.stopped = true
	f.mu.Unlock()
	return ctx.Err()
}

type fakeBootstrapper struct {
	count int
}

func (f *fakeBootstrapper) Bootstrap(context.Context) error {
	f.count++
	return nil
}

type fakeCompactionRunner struct {
	count int
	cfg   compaction.TriggerConfig
}

func (f *fakeCompactionRunner) RunCompactionSync(_ context.Context, cfg compaction.TriggerConfig) error {
	f.count++
	f.cfg = cfg
	return nil
}

type fakeCleanupRunner struct {
	count int
}

func (f *fakeCleanupRunner) Cleanup(context.Context) error {
	f.count++
	return nil
}
