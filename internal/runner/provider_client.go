package runner

import (
	"context"

	"connectrpc.com/connect"

	runnerv1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/runner/v1"
	"github.com/memohai/memoh/internal/connectapi/gen/memoh/runner/v1/runnerv1connect"
)

type ProviderClient struct {
	client runnerv1connect.RunnerSupportServiceClient
}

func NewProviderClient(client runnerv1connect.RunnerSupportServiceClient) *ProviderClient {
	return &ProviderClient{client: client}
}

func (c *ProviderClient) ResolveProviderCredentials(ctx context.Context, lease RunLease, providerID, providerName string, scopes []string) (*runnerv1.ResolveProviderCredentialsResponse, error) {
	if c == nil || c.client == nil {
		return nil, ErrSupportClientMissing
	}
	resp, err := c.client.ResolveProviderCredentials(ctx, connect.NewRequest(&runnerv1.ResolveProviderCredentialsRequest{
		Ref:          lease.Ref().Proto(),
		ProviderId:   providerID,
		ProviderName: providerName,
		Scopes:       append([]string(nil), scopes...),
	}))
	if err != nil {
		return nil, err
	}
	return resp.Msg, nil
}
