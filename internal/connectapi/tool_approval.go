package connectapi

import (
	"context"
	"errors"
	"strings"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	privatev1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1"
	"github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1/privatev1connect"
	"github.com/memohai/memoh/internal/iam/rbac"
	"github.com/memohai/memoh/internal/toolapproval"
)

type ToolApprovalService struct {
	bots      *BotService
	approvals *toolapproval.Service
}

func NewToolApprovalService(bots *BotService, approvals *toolapproval.Service) *ToolApprovalService {
	return &ToolApprovalService{bots: bots, approvals: approvals}
}

func NewToolApprovalHandler(service *ToolApprovalService) Handler {
	path, handler := privatev1connect.NewToolApprovalServiceHandler(service)
	return NewHandler(path, handler)
}

func (s *ToolApprovalService) ApproveTool(ctx context.Context, req *connect.Request[privatev1.ApproveToolRequest]) (*connect.Response[privatev1.ApproveToolResponse], error) {
	if _, err := s.respond(ctx, req.Msg.GetBotId(), req.Msg.GetConversationId(), req.Msg.GetRequestId(), "approve", toolApprovalReasonFromPayload(req.Msg.GetPayload())); err != nil {
		return nil, err
	}
	return connect.NewResponse(&privatev1.ApproveToolResponse{}), nil
}

func (s *ToolApprovalService) RejectTool(ctx context.Context, req *connect.Request[privatev1.RejectToolRequest]) (*connect.Response[privatev1.RejectToolResponse], error) {
	if _, err := s.respond(ctx, req.Msg.GetBotId(), req.Msg.GetConversationId(), req.Msg.GetRequestId(), "reject", req.Msg.GetReason()); err != nil {
		return nil, err
	}
	return connect.NewResponse(&privatev1.RejectToolResponse{}), nil
}

func (s *ToolApprovalService) ListToolApprovalRequests(ctx context.Context, req *connect.Request[privatev1.ListToolApprovalRequestsRequest]) (*connect.Response[privatev1.ListToolApprovalRequestsResponse], error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	botID := strings.TrimSpace(req.Msg.GetBotId())
	conversationID := strings.TrimSpace(req.Msg.GetConversationId())
	if botID == "" || conversationID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("bot_id and conversation_id are required"))
	}
	if s.bots == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("bot service not configured"))
	}
	if err := s.bots.requireBotPermission(ctx, userID, botID, rbac.PermissionBotRead); err != nil {
		return nil, botConnectError(err)
	}
	if s.approvals == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("tool approval service not configured"))
	}
	var requests []toolapproval.Request
	if strings.TrimSpace(req.Msg.GetStatus()) == toolapproval.StatusPending {
		requests, err = s.approvals.ListPendingBySession(ctx, botID, conversationID)
	} else {
		requests, err = s.approvals.ListBySession(ctx, botID, conversationID)
	}
	if err != nil {
		return nil, toolApprovalConnectError(err)
	}
	statusFilter := strings.TrimSpace(req.Msg.GetStatus())
	out := make([]*privatev1.ToolApprovalItem, 0, len(requests))
	for _, item := range requests {
		if statusFilter != "" && item.Status != statusFilter {
			continue
		}
		out = append(out, toolApprovalItemToProto(item))
	}
	return connect.NewResponse(&privatev1.ListToolApprovalRequestsResponse{
		Requests: out,
		Page:     &privatev1.PageResponse{},
	}), nil
}

func (s *ToolApprovalService) GetToolApprovalRequest(ctx context.Context, req *connect.Request[privatev1.GetToolApprovalRequestRequest]) (*connect.Response[privatev1.GetToolApprovalRequestResponse], error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	botID := strings.TrimSpace(req.Msg.GetBotId())
	requestID := strings.TrimSpace(req.Msg.GetRequestId())
	if botID == "" || requestID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("bot_id and request_id are required"))
	}
	if s.bots == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("bot service not configured"))
	}
	if err := s.bots.requireBotPermission(ctx, userID, botID, rbac.PermissionBotRead); err != nil {
		return nil, botConnectError(err)
	}
	if s.approvals == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("tool approval service not configured"))
	}
	item, err := s.approvals.ResolveTarget(ctx, toolapproval.ResolveInput{
		BotID:      botID,
		ExplicitID: requestID,
	})
	if err != nil {
		return nil, toolApprovalConnectError(err)
	}
	return connect.NewResponse(&privatev1.GetToolApprovalRequestResponse{Request: toolApprovalItemToProto(item)}), nil
}

func (s *ToolApprovalService) RespondToolApproval(ctx context.Context, req *connect.Request[privatev1.RespondToolApprovalRequest]) (*connect.Response[privatev1.RespondToolApprovalResponse], error) {
	decision := strings.TrimSpace(req.Msg.GetDecision())
	if decision != "approve" && decision != "reject" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("decision must be approve or reject"))
	}
	item, err := s.respond(ctx, req.Msg.GetBotId(), req.Msg.GetConversationId(), req.Msg.GetRequestId(), decision, firstNonEmptyString(req.Msg.GetReason(), toolApprovalReasonFromPayload(req.Msg.GetPayload())))
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&privatev1.RespondToolApprovalResponse{
		Request: toolApprovalItemToProto(item),
	}), nil
}

func (s *ToolApprovalService) respond(ctx context.Context, botID, conversationID, requestID, decision, reason string) (toolapproval.Request, error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return toolapproval.Request{}, connect.NewError(connect.CodeUnauthenticated, err)
	}
	if strings.TrimSpace(botID) == "" || strings.TrimSpace(requestID) == "" {
		return toolapproval.Request{}, connect.NewError(connect.CodeInvalidArgument, errors.New("bot_id and request_id are required"))
	}
	if s.bots == nil {
		return toolapproval.Request{}, connect.NewError(connect.CodeInternal, errors.New("bot service not configured"))
	}
	if err := s.bots.requireBotPermission(ctx, userID, botID, rbac.PermissionBotUpdate); err != nil {
		return toolapproval.Request{}, botConnectError(err)
	}
	if s.approvals == nil {
		return toolapproval.Request{}, connect.NewError(connect.CodeInternal, errors.New("tool approval service not configured"))
	}
	target, err := s.approvals.ResolveTarget(ctx, toolapproval.ResolveInput{
		BotID:      strings.TrimSpace(botID),
		SessionID:  strings.TrimSpace(conversationID),
		ExplicitID: strings.TrimSpace(requestID),
	})
	if err != nil {
		return toolapproval.Request{}, toolApprovalConnectError(err)
	}
	switch decision {
	case "approve":
		return s.approvals.Approve(context.WithoutCancel(ctx), target.ID, userID, strings.TrimSpace(reason))
	case "reject":
		return s.approvals.Reject(context.WithoutCancel(ctx), target.ID, userID, strings.TrimSpace(reason))
	default:
		return toolapproval.Request{}, connect.NewError(connect.CodeInvalidArgument, errors.New("decision must be approve or reject"))
	}
}

func toolApprovalItemToProto(item toolapproval.Request) *privatev1.ToolApprovalItem {
	return &privatev1.ToolApprovalItem{
		Id:             item.ID,
		BotId:          item.BotID,
		ConversationId: item.SessionID,
		RequestId:      firstNonEmptyString(item.ToolCallID, item.ID),
		ToolName:       item.ToolName,
		Status:         item.Status,
		Payload: mapToStruct(map[string]any{
			"tool_input":        item.ToolInput,
			"short_id":          item.ShortID,
			"decision_reason":   item.DecisionReason,
			"source_platform":   item.SourcePlatform,
			"reply_target":      item.ReplyTarget,
			"conversation_type": item.ConversationType,
		}),
		CreatedAt: timeToProto(item.CreatedAt),
		DecidedAt: toolApprovalTimePtrToProto(item.DecidedAt),
	}
}

func toolApprovalTimePtrToProto(value *time.Time) *timestamppb.Timestamp {
	if value == nil {
		return nil
	}
	return timeToProto(*value)
}

func toolApprovalConnectError(err error) error {
	switch {
	case errors.Is(err, toolapproval.ErrNotFound):
		return connect.NewError(connect.CodeNotFound, err)
	case errors.Is(err, toolapproval.ErrForbidden):
		return connect.NewError(connect.CodePermissionDenied, err)
	case errors.Is(err, toolapproval.ErrAlreadyDecided):
		return connect.NewError(connect.CodeFailedPrecondition, err)
	default:
		return connectError(err)
	}
}

func toolApprovalReasonFromPayload(payload *structpb.Struct) string {
	value, _ := structToMap(payload)["reason"].(string)
	return strings.TrimSpace(value)
}
