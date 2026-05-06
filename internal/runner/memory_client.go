package runner

import (
	"context"

	"connectrpc.com/connect"

	runnerv1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/runner/v1"
	"github.com/memohai/memoh/internal/connectapi/gen/memoh/runner/v1/runnerv1connect"
)

type MemoryClient struct {
	client runnerv1connect.RunnerSupportServiceClient
}

func NewMemoryClient(client runnerv1connect.RunnerSupportServiceClient) *MemoryClient {
	return &MemoryClient{client: client}
}

func (c *MemoryClient) ReadMemory(ctx context.Context, lease RunLease, query string, scopes []string, limit int32) ([]*runnerv1.MemoryRecord, error) {
	if c == nil || c.client == nil {
		return nil, ErrSupportClientMissing
	}
	resp, err := c.client.ReadMemory(ctx, connect.NewRequest(&runnerv1.ReadMemoryRequest{
		Ref:    lease.Ref().Proto(),
		Query:  query,
		Scopes: append([]string(nil), scopes...),
		Limit:  limit,
	}))
	if err != nil {
		return nil, err
	}
	return resp.Msg.GetMemories(), nil
}

func (c *MemoryClient) WriteMemory(ctx context.Context, lease RunLease, memory *runnerv1.MemoryRecord) (string, error) {
	if c == nil || c.client == nil {
		return "", ErrSupportClientMissing
	}
	resp, err := c.client.WriteMemory(ctx, connect.NewRequest(&runnerv1.WriteMemoryRequest{
		Ref:    lease.Ref().Proto(),
		Memory: memory,
	}))
	if err != nil {
		return "", err
	}
	return resp.Msg.GetMemoryId(), nil
}
