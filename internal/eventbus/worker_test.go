package eventbus

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	dbsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
)

func TestConsumerUsesDefaultLockTTLAndRetryBackoff(t *testing.T) {
	now := time.Date(2026, 5, 6, 10, 0, 0, 0, time.UTC)
	queries := &fakeConsumerQueries{
		claims: []dbsqlc.ClaimEventDeliveriesRow{{
			EventID:        testUUID(4),
			ConsumerName:   "worker",
			Topic:          "worker.schedule.startup",
			Attempts:       1,
			MaxAttempts:    10,
			PayloadType:    "json",
			Payload:        []byte(`{}`),
			IdempotencyKey: "startup",
		}},
	}
	consumer := NewConsumer(queries, "worker", "worker-1")
	consumer.SetNow(func() time.Time { return now })
	consumer.RegisterHandler("worker.schedule.startup", func(context.Context, Delivery) error {
		return errTestHandler
	})
	if _, err := consumer.ProcessOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got, want := queries.claim.LockedUntil.Time, now.Add(DefaultLockTTL); !got.Equal(want) {
		t.Fatalf("locked_until = %s, want %s", got, want)
	}
	if got, want := queries.fail.NextAvailableAt.Time, now.Add(time.Second); !got.Equal(want) {
		t.Fatalf("retry time = %s, want %s", got, want)
	}
	if RetryBackoff(100) != MaxRetryBackoff {
		t.Fatalf("retry cap = %s", RetryBackoff(100))
	}
}

type fakeConsumerQueries struct {
	claims       []dbsqlc.ClaimEventDeliveriesRow
	claim        dbsqlc.ClaimEventDeliveriesParams
	ack          dbsqlc.AckEventDeliveryParams
	fail         dbsqlc.FailEventDeliveryParams
	deadLettered bool
}

func (f *fakeConsumerQueries) ClaimEventDeliveries(_ context.Context, arg dbsqlc.ClaimEventDeliveriesParams) ([]dbsqlc.ClaimEventDeliveriesRow, error) {
	f.claim = arg
	return f.claims, nil
}

func (f *fakeConsumerQueries) AckEventDelivery(_ context.Context, arg dbsqlc.AckEventDeliveryParams) (dbsqlc.EventDelivery, error) {
	f.ack = arg
	return dbsqlc.EventDelivery{}, nil
}

func (f *fakeConsumerQueries) FailEventDelivery(_ context.Context, arg dbsqlc.FailEventDeliveryParams) (dbsqlc.EventDelivery, error) {
	f.fail = arg
	if len(f.claims) > 0 && f.claims[0].Attempts >= f.claims[0].MaxAttempts {
		f.deadLettered = true
	}
	return dbsqlc.EventDelivery{}, nil
}

func testUUID(seed byte) pgtype.UUID {
	var bytes [16]byte
	bytes[15] = seed
	return pgtype.UUID{Bytes: bytes, Valid: true}
}
