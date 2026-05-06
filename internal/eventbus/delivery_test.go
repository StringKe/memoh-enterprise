package eventbus

import (
	"context"
	"errors"
	"testing"

	dbsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
)

var errTestHandler = errors.New("handler failed")

func TestConsumerAcksEachConsumerIndependently(t *testing.T) {
	queriesA := &fakeConsumerQueries{
		claims: []dbsqlc.ClaimEventDeliveriesRow{{
			EventID:        testUUID(3),
			ConsumerName:   "worker-a",
			Topic:          "runner.run.event",
			Attempts:       1,
			MaxAttempts:    10,
			PayloadType:    "json",
			Payload:        []byte(`{"n":1}`),
			IdempotencyKey: "event-1",
		}},
	}
	consumerA := NewConsumer(queriesA, "worker-a", "instance-a")
	consumerA.RegisterHandler("runner.run.event", func(context.Context, Delivery) error { return nil })
	if _, err := consumerA.ProcessOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if queriesA.ack.ConsumerName != "worker-a" {
		t.Fatalf("ack consumer = %q", queriesA.ack.ConsumerName)
	}

	queriesB := &fakeConsumerQueries{
		claims: []dbsqlc.ClaimEventDeliveriesRow{{
			EventID:        testUUID(3),
			ConsumerName:   "worker-b",
			Topic:          "runner.run.event",
			Attempts:       1,
			MaxAttempts:    10,
			PayloadType:    "json",
			Payload:        []byte(`{"n":1}`),
			IdempotencyKey: "event-1",
		}},
	}
	consumerB := NewConsumer(queriesB, "worker-b", "instance-b")
	consumerB.RegisterHandler("runner.run.event", func(context.Context, Delivery) error { return nil })
	if _, err := consumerB.ProcessOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if queriesB.ack.ConsumerName != "worker-b" {
		t.Fatalf("ack consumer = %q", queriesB.ack.ConsumerName)
	}
}
