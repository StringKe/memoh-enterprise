package connectapi

import (
	"context"
	"errors"
	"strings"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/structpb"

	privatev1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1"
	"github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1/privatev1connect"
	"github.com/memohai/memoh/internal/conversation/flow"
	"github.com/memohai/memoh/internal/iam/rbac"
)

type ToolApprovalService struct {
	bots     *BotService
	resolver *flow.Resolver
}

func NewToolApprovalService(bots *BotService, resolver *flow.Resolver) *ToolApprovalService {
	return &ToolApprovalService{bots: bots, resolver: resolver}
}

func NewToolApprovalHandler(service *ToolApprovalService) Handler {
	path, handler := privatev1connect.NewToolApprovalServiceHandler(service)
	return NewHandler(path, handler)
}

func (s *ToolApprovalService) ApproveTool(ctx context.Context, req *connect.Request[privatev1.ApproveToolRequest]) (*connect.Response[privatev1.ApproveToolResponse], error) {
	if err := s.respond(ctx, req.Msg.GetBotId(), req.Msg.GetConversationId(), req.Msg.GetRequestId(), "approve", toolApprovalReasonFromPayload(req.Msg.GetPayload())); err != nil {
		return nil, err
	}
	return connect.NewResponse(&privatev1.ApproveToolResponse{}), nil
}

func (s *ToolApprovalService) RejectTool(ctx context.Context, req *connect.Request[privatev1.RejectToolRequest]) (*connect.Response[privatev1.RejectToolResponse], error) {
	if err := s.respond(ctx, req.Msg.GetBotId(), req.Msg.GetConversationId(), req.Msg.GetRequestId(), "reject", req.Msg.GetReason()); err != nil {
		return nil, err
	}
	return connect.NewResponse(&privatev1.RejectToolResponse{}), nil
}

func (s *ToolApprovalService) respond(ctx context.Context, botID, conversationID, requestID, decision, reason string) error {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return connect.NewError(connect.CodeUnauthenticated, err)
	}
	if strings.TrimSpace(botID) == "" || strings.TrimSpace(requestID) == "" {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("bot_id and request_id are required"))
	}
	if s.bots == nil {
		return connect.NewError(connect.CodeInternal, errors.New("bot service not configured"))
	}
	if err := s.bots.requireBotPermission(ctx, userID, botID, rbac.PermissionBotUpdate); err != nil {
		return botConnectError(err)
	}
	if s.resolver == nil {
		return connect.NewError(connect.CodeInternal, errors.New("tool approval resolver not configured"))
	}
	if err := s.resolver.RespondToolApproval(context.WithoutCancel(ctx), flow.ToolApprovalResponseInput{
		BotID:                  strings.TrimSpace(botID),
		SessionID:              strings.TrimSpace(conversationID),
		ActorChannelIdentityID: userID,
		ApprovalID:             strings.TrimSpace(requestID),
		Decision:               decision,
		Reason:                 strings.TrimSpace(reason),
	}, nil); err != nil {
		return connect.NewError(connect.CodeInternal, err)
	}
	return nil
}

func toolApprovalReasonFromPayload(payload *structpb.Struct) string {
	value, _ := structToMap(payload)["reason"].(string)
	return strings.TrimSpace(value)
}
