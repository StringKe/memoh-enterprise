package connectapi

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	privatev1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1"
)

func TestHealthService(t *testing.T) {
	t.Parallel()

	service := NewHealthService()
	ping, err := service.Ping(context.Background(), connect.NewRequest(&privatev1.PingRequest{}))
	if err != nil {
		t.Fatalf("Ping() error = %v", err)
	}
	if ping.Msg.GetStatus() != "ok" {
		t.Fatalf("Ping status = %q, want ok", ping.Msg.GetStatus())
	}

	health, err := service.Health(context.Background(), connect.NewRequest(&privatev1.HealthRequest{}))
	if err != nil {
		t.Fatalf("Health() error = %v", err)
	}
	if health.Msg.GetStatus() != "ok" {
		t.Fatalf("Health status = %q, want ok", health.Msg.GetStatus())
	}
}
