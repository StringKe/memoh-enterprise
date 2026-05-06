package runner

import (
	"context"
	"net/http"
	"strings"

	"connectrpc.com/connect"

	runnerv1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/runner/v1"
	"github.com/memohai/memoh/internal/connectapi/gen/memoh/runner/v1/runnerv1connect"
	"github.com/memohai/memoh/internal/connectapi/gen/memoh/workspace/v1/workspacev1connect"
)

type WorkspaceClient struct {
	support    runnerv1connect.RunnerSupportServiceClient
	httpClient connect.HTTPClient
	opts       []connect.ClientOption
}

func NewWorkspaceClient(support runnerv1connect.RunnerSupportServiceClient, httpClient connect.HTTPClient, opts ...connect.ClientOption) *WorkspaceClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &WorkspaceClient{support: support, httpClient: httpClient, opts: opts}
}

func (c *WorkspaceClient) IssueWorkspaceToken(ctx context.Context, lease RunLease, scopes []string) (WorkspaceToken, error) {
	if c == nil || c.support == nil {
		return WorkspaceToken{}, ErrSupportClientMissing
	}
	resp, err := c.support.IssueWorkspaceToken(ctx, connect.NewRequest(&runnerv1.IssueWorkspaceTokenRequest{
		Ref:                     lease.Ref().Proto(),
		WorkspaceId:             lease.WorkspaceID,
		WorkspaceExecutorTarget: lease.WorkspaceExecutorTarget,
		Scopes:                  append([]string(nil), scopes...),
	}))
	if err != nil {
		return WorkspaceToken{}, err
	}
	expiresAt := resp.Msg.GetExpiresAt().AsTime()
	if !expiresAt.IsZero() && expiresAt.After(lease.ExpiresAt) {
		return WorkspaceToken{}, ErrWorkspaceTokenOutlivesLease
	}
	return WorkspaceToken{Token: resp.Msg.GetToken(), ExpiresAt: expiresAt}, nil
}

func (c *WorkspaceClient) WorkspaceExecutorClient(lease RunLease) (workspacev1connect.WorkspaceExecutorServiceClient, error) {
	if strings.TrimSpace(lease.WorkspaceExecutorTarget) == "" {
		return nil, ErrWorkspaceExecutorTargetEmpty
	}
	return workspacev1connect.NewWorkspaceExecutorServiceClient(c.httpClient, lease.WorkspaceExecutorTarget, c.opts...), nil
}

func (c *WorkspaceClient) ExecutorClient(ctx context.Context, lease RunLease, scopes []string) (workspacev1connect.WorkspaceExecutorServiceClient, WorkspaceToken, error) {
	token, err := c.IssueWorkspaceToken(ctx, lease, scopes)
	if err != nil {
		return nil, WorkspaceToken{}, err
	}
	client, err := c.WorkspaceExecutorClient(lease)
	if err != nil {
		return nil, WorkspaceToken{}, err
	}
	return client, token, nil
}
