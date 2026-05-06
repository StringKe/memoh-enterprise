package runner

import (
	"context"

	"connectrpc.com/connect"

	runnerv1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/runner/v1"
	"github.com/memohai/memoh/internal/connectapi/gen/memoh/runner/v1/runnerv1connect"
)

type ContextClient struct {
	client runnerv1connect.RunnerSupportServiceClient
}

func NewContextClient(client runnerv1connect.RunnerSupportServiceClient) *ContextClient {
	return &ContextClient{client: client}
}

func (c *ContextClient) ResolveRunContext(ctx context.Context, lease RunLease) (*runnerv1.ResolveRunContextResponse, error) {
	if c == nil || c.client == nil {
		return nil, ErrSupportClientMissing
	}
	resp, err := c.client.ResolveRunContext(ctx, connect.NewRequest(&runnerv1.ResolveRunContextRequest{
		Ref: lease.Ref().Proto(),
	}))
	if err != nil {
		return nil, err
	}
	return resp.Msg, nil
}

func (c *ContextClient) ValidateRunLease(ctx context.Context, lease RunLease) (*runnerv1.ValidateRunLeaseResponse, error) {
	if c == nil || c.client == nil {
		return nil, ErrSupportClientMissing
	}
	resp, err := c.client.ValidateRunLease(ctx, connect.NewRequest(&runnerv1.ValidateRunLeaseRequest{
		Ref:                     lease.Ref().Proto(),
		BotId:                   lease.BotID,
		SessionId:               lease.SessionID,
		WorkspaceExecutorTarget: lease.WorkspaceExecutorTarget,
	}))
	if err != nil {
		return nil, err
	}
	return resp.Msg, nil
}

func (c *ContextClient) ReadSessionHistory(ctx context.Context, lease RunLease, limit int32, beforeMessageID string) ([]*runnerv1.SessionMessage, error) {
	if c == nil || c.client == nil {
		return nil, ErrSupportClientMissing
	}
	resp, err := c.client.ReadSessionHistory(ctx, connect.NewRequest(&runnerv1.ReadSessionHistoryRequest{
		Ref:             lease.Ref().Proto(),
		SessionId:       lease.SessionID,
		Limit:           limit,
		BeforeMessageId: beforeMessageID,
	}))
	if err != nil {
		return nil, err
	}
	return resp.Msg.GetMessages(), nil
}
