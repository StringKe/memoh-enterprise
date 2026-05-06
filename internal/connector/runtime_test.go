package connector

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/memohai/memoh/internal/channel"
)

type runtimeTestAdapter struct {
	stopOnce sync.Once
	stopped  chan struct{}
}

func (*runtimeTestAdapter) Type() channel.ChannelType {
	return channel.ChannelTypeTelegram
}

func (*runtimeTestAdapter) Descriptor() channel.Descriptor {
	return channel.Descriptor{
		Type:        channel.ChannelTypeTelegram,
		DisplayName: "runtime-test",
		Capabilities: channel.ChannelCapabilities{
			Text: true,
		},
	}
}

func (a *runtimeTestAdapter) Connect(_ context.Context, cfg channel.ChannelConfig, _ channel.InboundHandler) (channel.Connection, error) {
	return channel.NewConnection(cfg, func(context.Context) error {
		a.stopOnce.Do(func() {
			close(a.stopped)
		})
		return nil
	}), nil
}

type failingRuntimeRenewer struct{}

func (failingRuntimeRenewer) RenewLease(context.Context, LeaseToken) (LeaseToken, error) {
	return LeaseToken{}, ErrLeaseStale
}

func (failingRuntimeRenewer) ReleaseLease(context.Context, LeaseToken) error {
	return nil
}

func TestRuntimeStopsAdapterWhenRenewFails(t *testing.T) {
	t.Parallel()

	registry := channel.NewRegistry()
	adapter := &runtimeTestAdapter{stopped: make(chan struct{})}
	if err := registry.Register(adapter); err != nil {
		t.Fatalf("register adapter failed: %v", err)
	}
	runtime := NewRuntime(
		slog.New(slog.DiscardHandler),
		registry,
		failingRuntimeRenewer{},
		nil,
		WithRenewInterval(10*time.Millisecond),
	)
	token := LeaseToken{
		ChannelConfigID: "cfg-1",
		ChannelType:     channel.ChannelTypeTelegram,
		OwnerID:         "owner",
		OwnerInstanceID: "connector-a",
		LeaseVersion:    1,
		AcquiredAt:      time.Now().UTC(),
		ExpiresAt:       time.Now().Add(DefaultLeaseTTL).UTC(),
	}
	cfg := channel.ChannelConfig{
		ID:          "cfg-1",
		BotID:       "bot-1",
		ChannelType: channel.ChannelTypeTelegram,
		UpdatedAt:   time.Now().UTC(),
	}
	if err := runtime.Start(context.Background(), token, cfg); err != nil {
		t.Fatalf("runtime start failed: %v", err)
	}
	select {
	case <-adapter.stopped:
	case <-time.After(time.Second):
		t.Fatal("adapter was not stopped after renewal failure")
	}
}
