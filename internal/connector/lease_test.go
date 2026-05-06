package connector

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/memohai/memoh/internal/channel"
)

func TestAcquireLeaseRaceSingleWinner(t *testing.T) {
	t.Parallel()

	service := NewService(NewMemoryLeaseStore(), WithClock(func() time.Time {
		return time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	}))
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, ownerInstanceID := range []string{"connector-a", "connector-b"} {
		wg.Add(1)
		go func(ownerInstanceID string) {
			defer wg.Done()
			<-start
			_, err := service.AcquireLease(context.Background(), AcquireLeaseRequest{
				ChannelConfigID: "cfg-1",
				ChannelType:     channel.ChannelTypeTelegram,
				OwnerID:         "owner",
				OwnerInstanceID: ownerInstanceID,
			})
			results <- err
		}(ownerInstanceID)
	}
	close(start)
	wg.Wait()
	close(results)

	winners := 0
	held := 0
	for err := range results {
		if err == nil {
			winners++
			continue
		}
		if errors.Is(err, ErrLeaseHeld) {
			held++
			continue
		}
		t.Fatalf("unexpected acquire error: %v", err)
	}
	if winners != 1 || held != 1 {
		t.Fatalf("winners=%d held=%d, want 1/1", winners, held)
	}
}

func TestLeaseRenewReleaseAndExpiredReacquire(t *testing.T) {
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	service := NewService(NewMemoryLeaseStore(), WithClock(func() time.Time { return now }))

	lease, err := service.AcquireLease(context.Background(), AcquireLeaseRequest{
		ChannelConfigID: "cfg-1",
		ChannelType:     channel.ChannelTypeTelegram,
		OwnerID:         "owner",
		OwnerInstanceID: "connector-a",
	})
	if err != nil {
		t.Fatalf("acquire failed: %v", err)
	}
	now = now.Add(DefaultRenewInterval)
	renewed, err := service.RenewLease(context.Background(), lease)
	if err != nil {
		t.Fatalf("renew failed: %v", err)
	}
	if renewed.LeaseVersion != lease.LeaseVersion {
		t.Fatalf("renewed version = %d, want %d", renewed.LeaseVersion, lease.LeaseVersion)
	}
	stale := lease
	stale.OwnerInstanceID = "connector-b"
	if err := service.ReleaseLease(context.Background(), stale); !errors.Is(err, ErrLeaseStale) {
		t.Fatalf("non-owner release err = %v, want stale", err)
	}
	now = renewed.ExpiresAt.Add(time.Second)
	reacquired, err := service.AcquireLease(context.Background(), AcquireLeaseRequest{
		ChannelConfigID: "cfg-1",
		ChannelType:     channel.ChannelTypeTelegram,
		OwnerID:         "owner",
		OwnerInstanceID: "connector-b",
	})
	if err != nil {
		t.Fatalf("expired reacquire failed: %v", err)
	}
	if reacquired.LeaseVersion != renewed.LeaseVersion+1 {
		t.Fatalf("reacquired version = %d, want %d", reacquired.LeaseVersion, renewed.LeaseVersion+1)
	}
}
