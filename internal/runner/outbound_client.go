package runner

import (
	"context"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/structpb"

	runnerv1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/runner/v1"
	"github.com/memohai/memoh/internal/connectapi/gen/memoh/runner/v1/runnerv1connect"
)

type OutboundClient struct {
	client runnerv1connect.RunnerSupportServiceClient
}

type OutboundDispatch struct {
	ChannelConfigID string
	ChannelType     string
	ConversationID  string
	Text            string
	Payload         *structpb.Struct
}

func NewOutboundClient(client runnerv1connect.RunnerSupportServiceClient) *OutboundClient {
	return &OutboundClient{client: client}
}

func (c *OutboundClient) ResolveOutboundTarget(ctx context.Context, lease RunLease, channelType, conversationID string) (*runnerv1.ResolveOutboundTargetResponse, error) {
	if c == nil || c.client == nil {
		return nil, ErrSupportClientMissing
	}
	resp, err := c.client.ResolveOutboundTarget(ctx, connect.NewRequest(&runnerv1.ResolveOutboundTargetRequest{
		Ref:            lease.Ref().Proto(),
		ChannelType:    channelType,
		ConversationId: conversationID,
	}))
	if err != nil {
		return nil, err
	}
	return resp.Msg, nil
}

func (c *OutboundClient) RequestOutboundDispatch(ctx context.Context, lease RunLease, dispatch OutboundDispatch) (*runnerv1.RequestOutboundDispatchResponse, error) {
	if c == nil || c.client == nil {
		return nil, ErrSupportClientMissing
	}
	resp, err := c.client.RequestOutboundDispatch(ctx, connect.NewRequest(&runnerv1.RequestOutboundDispatchRequest{
		Ref:             lease.Ref().Proto(),
		ChannelConfigId: dispatch.ChannelConfigID,
		ChannelType:     dispatch.ChannelType,
		ConversationId:  dispatch.ConversationID,
		Text:            dispatch.Text,
		Payload:         dispatch.Payload,
	}))
	if err != nil {
		return nil, err
	}
	return resp.Msg, nil
}
