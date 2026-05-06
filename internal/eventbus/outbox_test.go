package eventbus

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	dbsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
)

func TestProducerPublishesIdempotentEventAndDeliveries(t *testing.T) {
	now := time.Date(2026, 5, 6, 10, 0, 0, 0, time.UTC)
	queries := &fakeProducerQueries{
		outbox: dbsqlc.EventOutbox{
			ID:             testUUID(1),
			Topic:          "runner.run.started",
			PayloadType:    "memoh.event.v1.AgentRunEvent",
			Payload:        []byte("payload"),
			IdempotencyKey: "run-1:start",
			PartitionKey:   pgtype.Text{String: "run-1", Valid: true},
		},
	}
	producer := NewProducer(queries)
	producer.SetNow(func() time.Time { return now })

	outbox, err := producer.Publish(context.Background(), Event{
		Topic:          "runner.run.started",
		PayloadType:    "memoh.event.v1.AgentRunEvent",
		Payload:        []byte("payload"),
		IdempotencyKey: "run-1:start",
		PartitionKey:   "run-1",
	}, []string{"worker", "worker", "audit"})
	if err != nil {
		t.Fatal(err)
	}
	if outbox.IdempotencyKey != "run-1:start" {
		t.Fatalf("idempotency key = %q", outbox.IdempotencyKey)
	}
	if queries.enqueue.IdempotencyKey != "run-1:start" {
		t.Fatalf("enqueue idempotency key = %q", queries.enqueue.IdempotencyKey)
	}
	if got, want := queries.create.ConsumerNames, []string{"worker", "audit"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("consumer names = %#v", got)
	}
	if !queries.create.AvailableAt.Time.Equal(now) {
		t.Fatalf("available_at = %s", queries.create.AvailableAt.Time)
	}
	if queries.create.MaxAttempts != DefaultMaxAttempts {
		t.Fatalf("max_attempts = %d", queries.create.MaxAttempts)
	}
}

func TestConsumerRetryAndDeadLetter(t *testing.T) {
	now := time.Date(2026, 5, 6, 10, 0, 0, 0, time.UTC)
	queries := &fakeConsumerQueries{
		claims: []dbsqlc.ClaimEventDeliveriesRow{{
			EventID:        testUUID(2),
			ConsumerName:   "worker",
			Topic:          "worker.cleanup.run",
			Attempts:       3,
			MaxAttempts:    10,
			PayloadType:    "json",
			Payload:        []byte(`{}`),
			IdempotencyKey: "cleanup-1",
		}},
	}
	consumer := NewConsumer(queries, "worker", "worker-1")
	consumer.SetNow(func() time.Time { return now })
	consumer.RegisterHandler("worker.cleanup.run", func(context.Context, Delivery) error {
		return errTestHandler
	})

	processed, err := consumer.ProcessOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if processed != 1 {
		t.Fatalf("processed = %d", processed)
	}
	if queries.fail.LastError.String != errTestHandler.Error() {
		t.Fatalf("last_error = %q", queries.fail.LastError.String)
	}
	if got, want := queries.fail.NextAvailableAt.Time, now.Add(4*time.Second); !got.Equal(want) {
		t.Fatalf("next_available_at = %s, want %s", got, want)
	}

	queries.claims[0].Attempts = 10
	queries.fail = dbsqlc.FailEventDeliveryParams{}
	if _, err := consumer.ProcessOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !queries.deadLettered {
		t.Fatal("delivery was not dead-lettered at max attempts")
	}
}

type fakeProducerQueries struct {
	outbox  dbsqlc.EventOutbox
	enqueue dbsqlc.EnqueueEventParams
	create  dbsqlc.CreateEventDeliveriesParams
}

func (f *fakeProducerQueries) EnqueueEvent(_ context.Context, arg dbsqlc.EnqueueEventParams) (dbsqlc.EventOutbox, error) {
	f.enqueue = arg
	return f.outbox, nil
}

func (f *fakeProducerQueries) CreateEventDeliveries(_ context.Context, arg dbsqlc.CreateEventDeliveriesParams) ([]dbsqlc.EventDelivery, error) {
	f.create = arg
	return nil, nil
}
