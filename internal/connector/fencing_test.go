package connector

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/memohai/memoh/internal/channel"
)

type recordingInboundSink struct {
	events []InboundEvent
}

func (s *recordingInboundSink) AcceptInbound(_ context.Context, event InboundEvent) error {
	s.events = append(s.events, event)
	return nil
}

func TestStaleInboundRejected(t *testing.T) {
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	sink := &recordingInboundSink{}
	service := NewService(NewMemoryLeaseStore(), WithClock(func() time.Time { return now }), WithInboundSink(sink))
	first, err := service.AcquireLease(context.Background(), AcquireLeaseRequest{
		ChannelConfigID: "cfg-1",
		ChannelType:     channel.ChannelTypeTelegram,
		OwnerID:         "owner",
		OwnerInstanceID: "connector-a",
	})
	if err != nil {
		t.Fatalf("acquire first failed: %v", err)
	}

	now = first.ExpiresAt.Add(time.Second)
	second, err := service.AcquireLease(context.Background(), AcquireLeaseRequest{
		ChannelConfigID: "cfg-1",
		ChannelType:     channel.ChannelTypeTelegram,
		OwnerID:         "owner",
		OwnerInstanceID: "connector-b",
	})
	if err != nil {
		t.Fatalf("acquire second failed: %v", err)
	}

	cfg := channel.ChannelConfig{ID: "cfg-1", BotID: "bot-1", ChannelType: channel.ChannelTypeTelegram}
	err = service.AcceptInbound(context.Background(), InboundEvent{
		Token:  first,
		Config: cfg,
		Message: channel.InboundMessage{
			Channel: channel.ChannelTypeTelegram,
			BotID:   "bot-1",
			Message: channel.Message{Text: "stale"},
		},
	})
	if !errors.Is(err, ErrLeaseStale) {
		t.Fatalf("stale inbound err = %v, want stale", err)
	}

	if err := service.AcceptInbound(context.Background(), InboundEvent{
		Token:  second,
		Config: cfg,
		Message: channel.InboundMessage{
			Channel: channel.ChannelTypeTelegram,
			BotID:   "bot-1",
			Message: channel.Message{Text: "fresh"},
		},
	}); err != nil {
		t.Fatalf("fresh inbound failed: %v", err)
	}
	if len(sink.events) != 1 {
		t.Fatalf("sink events = %d, want 1", len(sink.events))
	}
	if sink.events[0].Message.Message.PlainText() != "fresh" {
		t.Fatalf("unexpected forwarded message: %q", sink.events[0].Message.Message.PlainText())
	}
}
