package eventbus

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	dbsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
)

const DefaultMaxAttempts int32 = 10

type ProducerQueries interface {
	EnqueueEvent(ctx context.Context, arg dbsqlc.EnqueueEventParams) (dbsqlc.EventOutbox, error)
	CreateEventDeliveries(ctx context.Context, arg dbsqlc.CreateEventDeliveriesParams) ([]dbsqlc.EventDelivery, error)
}

type Producer struct {
	queries ProducerQueries
	now     func() time.Time
}

type Event struct {
	Topic          string
	PayloadType    string
	Payload        []byte
	PayloadJSON    []byte
	IdempotencyKey string
	AggregateType  string
	AggregateID    pgtype.UUID
	PartitionKey   string
	AvailableAt    time.Time
	MaxAttempts    int32
}

func NewProducer(queries ProducerQueries) *Producer {
	return &Producer{
		queries: queries,
		now:     time.Now,
	}
}

func (p *Producer) SetNow(now func() time.Time) {
	if now != nil {
		p.now = now
	}
}

func (p *Producer) Publish(ctx context.Context, event Event, consumers []string) (dbsqlc.EventOutbox, error) {
	if p == nil || p.queries == nil {
		return dbsqlc.EventOutbox{}, errors.New("eventbus producer queries are not configured")
	}
	if strings.TrimSpace(event.Topic) == "" {
		return dbsqlc.EventOutbox{}, errors.New("event topic is required")
	}
	if strings.TrimSpace(event.PayloadType) == "" {
		return dbsqlc.EventOutbox{}, errors.New("event payload_type is required")
	}
	if len(event.Payload) == 0 {
		return dbsqlc.EventOutbox{}, errors.New("event payload is required")
	}
	if strings.TrimSpace(event.IdempotencyKey) == "" {
		return dbsqlc.EventOutbox{}, errors.New("event idempotency_key is required")
	}
	consumerNames := normalizeConsumers(consumers)
	if len(consumerNames) == 0 {
		return dbsqlc.EventOutbox{}, errors.New("event consumers are required")
	}
	availableAt := event.AvailableAt
	if availableAt.IsZero() {
		availableAt = p.now().UTC()
	}
	maxAttempts := event.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = DefaultMaxAttempts
	}
	outbox, err := p.queries.EnqueueEvent(ctx, dbsqlc.EnqueueEventParams{
		Topic:          strings.TrimSpace(event.Topic),
		PayloadType:    strings.TrimSpace(event.PayloadType),
		Payload:        event.Payload,
		PayloadJson:    event.PayloadJSON,
		IdempotencyKey: strings.TrimSpace(event.IdempotencyKey),
		AggregateType:  textFromString(event.AggregateType),
		AggregateID:    event.AggregateID,
		PartitionKey:   textFromString(event.PartitionKey),
	})
	if err != nil {
		return dbsqlc.EventOutbox{}, err
	}
	if _, err := p.queries.CreateEventDeliveries(ctx, dbsqlc.CreateEventDeliveriesParams{
		EventID:       outbox.ID,
		ConsumerNames: consumerNames,
		Topic:         outbox.Topic,
		PartitionKey:  outbox.PartitionKey,
		AvailableAt:   timestamptz(availableAt),
		MaxAttempts:   maxAttempts,
	}); err != nil {
		return dbsqlc.EventOutbox{}, err
	}
	return outbox, nil
}

func normalizeConsumers(consumers []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(consumers))
	for _, consumer := range consumers {
		consumer = strings.TrimSpace(consumer)
		if consumer == "" {
			continue
		}
		if _, ok := seen[consumer]; ok {
			continue
		}
		seen[consumer] = struct{}{}
		out = append(out, consumer)
	}
	return out
}

func textFromString(value string) pgtype.Text {
	value = strings.TrimSpace(value)
	return pgtype.Text{String: value, Valid: value != ""}
}

func timestamptz(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value.UTC(), Valid: true}
}
