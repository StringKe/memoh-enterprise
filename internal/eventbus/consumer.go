package eventbus

import (
	"context"
	"errors"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	dbsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
)

const (
	DefaultLockTTL      = 60 * time.Second
	DefaultPollInterval = time.Second
	DefaultBatchSize    = int32(10)
	MaxRetryBackoff     = 5 * time.Minute
)

type ConsumerQueries interface {
	ClaimEventDeliveries(ctx context.Context, arg dbsqlc.ClaimEventDeliveriesParams) ([]dbsqlc.ClaimEventDeliveriesRow, error)
	AckEventDelivery(ctx context.Context, arg dbsqlc.AckEventDeliveryParams) (dbsqlc.EventDelivery, error)
	FailEventDelivery(ctx context.Context, arg dbsqlc.FailEventDeliveryParams) (dbsqlc.EventDelivery, error)
}

type Delivery struct {
	EventID        pgtype.UUID
	ConsumerName   string
	Topic          string
	PartitionKey   string
	Attempts       int32
	MaxAttempts    int32
	PayloadType    string
	Payload        []byte
	PayloadJSON    []byte
	IdempotencyKey string
	AggregateType  string
	AggregateID    pgtype.UUID
	CreatedAt      time.Time
}

type Handler func(context.Context, Delivery) error

type Consumer struct {
	queries      ConsumerQueries
	consumerName string
	instanceID   string
	handlers     map[string]Handler
	lockTTL      time.Duration
	pollInterval time.Duration
	batchSize    int32
	now          func() time.Time
}

func NewConsumer(queries ConsumerQueries, consumerName, instanceID string) *Consumer {
	return &Consumer{
		queries:      queries,
		consumerName: strings.TrimSpace(consumerName),
		instanceID:   strings.TrimSpace(instanceID),
		handlers:     map[string]Handler{},
		lockTTL:      DefaultLockTTL,
		pollInterval: DefaultPollInterval,
		batchSize:    DefaultBatchSize,
		now:          time.Now,
	}
}

func (c *Consumer) SetNow(now func() time.Time) {
	if now != nil {
		c.now = now
	}
}

func (c *Consumer) SetLockTTL(ttl time.Duration) {
	if ttl > 0 {
		c.lockTTL = ttl
	}
}

func (c *Consumer) SetPollInterval(interval time.Duration) {
	if interval > 0 {
		c.pollInterval = interval
	}
}

func (c *Consumer) SetBatchSize(size int32) {
	if size > 0 {
		c.batchSize = size
	}
}

func (c *Consumer) RegisterHandler(topic string, handler Handler) {
	topic = strings.TrimSpace(topic)
	if topic == "" || handler == nil {
		return
	}
	c.handlers[topic] = handler
}

func (c *Consumer) Run(ctx context.Context) error {
	if err := c.validate(); err != nil {
		return err
	}
	ticker := time.NewTicker(c.pollInterval)
	defer ticker.Stop()
	for {
		if _, err := c.ProcessOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (c *Consumer) ProcessOnce(ctx context.Context) (int, error) {
	if err := c.validate(); err != nil {
		return 0, err
	}
	topics := make([]string, 0, len(c.handlers))
	for topic := range c.handlers {
		topics = append(topics, topic)
	}
	sort.Strings(topics)

	processed := 0
	for _, topic := range topics {
		rows, err := c.queries.ClaimEventDeliveries(ctx, dbsqlc.ClaimEventDeliveriesParams{
			ConsumerName: c.consumerName,
			Topic:        topic,
			LimitCount:   c.batchSize,
			LockedBy:     pgtype.Text{String: c.instanceID, Valid: true},
			LockedUntil:  timestamptz(c.now().UTC().Add(c.lockTTL)),
		})
		if err != nil {
			return processed, err
		}
		for _, row := range rows {
			delivery := deliveryFromRow(row)
			if err := c.handlers[topic](ctx, delivery); err != nil {
				if _, failErr := c.queries.FailEventDelivery(ctx, dbsqlc.FailEventDeliveryParams{
					LastError:       pgtype.Text{String: err.Error(), Valid: true},
					NextAvailableAt: timestamptz(c.now().UTC().Add(RetryBackoff(row.Attempts))),
					EventID:         row.EventID,
					ConsumerName:    row.ConsumerName,
					LockedBy:        pgtype.Text{String: c.instanceID, Valid: true},
				}); failErr != nil {
					return processed, failErr
				}
				processed++
				continue
			}
			if _, err := c.queries.AckEventDelivery(ctx, dbsqlc.AckEventDeliveryParams{
				EventID:      row.EventID,
				ConsumerName: row.ConsumerName,
				LockedBy:     pgtype.Text{String: c.instanceID, Valid: true},
			}); err != nil {
				return processed, err
			}
			processed++
		}
	}
	return processed, nil
}

func RetryBackoff(attempts int32) time.Duration {
	if attempts <= 1 {
		return time.Second
	}
	power := math.Min(float64(attempts-1), 10)
	backoff := time.Duration(1<<int(power)) * time.Second
	if backoff > MaxRetryBackoff {
		return MaxRetryBackoff
	}
	return backoff
}

func (c *Consumer) validate() error {
	if c == nil || c.queries == nil {
		return errors.New("eventbus consumer queries are not configured")
	}
	if c.consumerName == "" {
		return errors.New("eventbus consumer name is required")
	}
	if c.instanceID == "" {
		return errors.New("eventbus consumer instance id is required")
	}
	if len(c.handlers) == 0 {
		return errors.New("eventbus consumer handlers are required")
	}
	return nil
}

func deliveryFromRow(row dbsqlc.ClaimEventDeliveriesRow) Delivery {
	return Delivery{
		EventID:        row.EventID,
		ConsumerName:   row.ConsumerName,
		Topic:          row.Topic,
		PartitionKey:   row.PartitionKey.String,
		Attempts:       row.Attempts,
		MaxAttempts:    row.MaxAttempts,
		PayloadType:    row.PayloadType,
		Payload:        row.Payload,
		PayloadJSON:    row.PayloadJson,
		IdempotencyKey: row.IdempotencyKey,
		AggregateType:  row.AggregateType.String,
		AggregateID:    row.AggregateID,
		CreatedAt:      row.EventCreatedAt.Time,
	}
}
