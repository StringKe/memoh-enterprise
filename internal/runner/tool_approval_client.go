package runner

import (
	"context"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/structpb"

	runnerv1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/runner/v1"
	"github.com/memohai/memoh/internal/connectapi/gen/memoh/runner/v1/runnerv1connect"
)

type ToolApprovalClient struct {
	client runnerv1connect.RunnerSupportServiceClient
}

func NewToolApprovalClient(client runnerv1connect.RunnerSupportServiceClient) *ToolApprovalClient {
	return &ToolApprovalClient{client: client}
}

func (c *ToolApprovalClient) EvaluateToolApprovalPolicy(ctx context.Context, lease RunLease, toolName, toolScope string, payload *structpb.Struct) (*runnerv1.EvaluateToolApprovalPolicyResponse, error) {
	if c == nil || c.client == nil {
		return nil, ErrSupportClientMissing
	}
	resp, err := c.client.EvaluateToolApprovalPolicy(ctx, connect.NewRequest(&runnerv1.EvaluateToolApprovalPolicyRequest{
		Ref:       lease.Ref().Proto(),
		ToolName:  toolName,
		ToolScope: toolScope,
		Payload:   payload,
	}))
	if err != nil {
		return nil, err
	}
	return resp.Msg, nil
}

func (c *ToolApprovalClient) RequestToolApproval(ctx context.Context, lease RunLease, toolName, toolScope string, payload *structpb.Struct) (*runnerv1.RequestToolApprovalResponse, error) {
	if c == nil || c.client == nil {
		return nil, ErrSupportClientMissing
	}
	resp, err := c.client.RequestToolApproval(ctx, connect.NewRequest(&runnerv1.RequestToolApprovalRequest{
		Ref:       lease.Ref().Proto(),
		ToolName:  toolName,
		ToolScope: toolScope,
		Payload:   payload,
	}))
	if err != nil {
		return nil, err
	}
	return resp.Msg, nil
}
