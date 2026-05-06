package runner

import (
	"context"

	"connectrpc.com/connect"

	runnerv1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/runner/v1"
	"github.com/memohai/memoh/internal/connectapi/gen/memoh/runner/v1/runnerv1connect"
)

type SecretClient struct {
	client runnerv1connect.RunnerSupportServiceClient
}

func NewSecretClient(client runnerv1connect.RunnerSupportServiceClient) *SecretClient {
	return &SecretClient{client: client}
}

func (c *SecretClient) ResolveScopedSecret(ctx context.Context, lease RunLease, secretRef, purpose string) (*runnerv1.ResolveScopedSecretResponse, error) {
	if c == nil || c.client == nil {
		return nil, ErrSupportClientMissing
	}
	resp, err := c.client.ResolveScopedSecret(ctx, connect.NewRequest(&runnerv1.ResolveScopedSecretRequest{
		Ref:       lease.Ref().Proto(),
		SecretRef: secretRef,
		Purpose:   purpose,
	}))
	if err != nil {
		return nil, err
	}
	return resp.Msg, nil
}
