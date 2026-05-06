package connectapi

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	runnerv1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/runner/v1"
	"github.com/memohai/memoh/internal/connectapi/gen/memoh/runner/v1/runnerv1connect"
	"github.com/memohai/memoh/internal/serviceauth"
)

type runnerSupportHandler struct {
	service *RunnerSupportService
}

func NewRunnerSupportHandler(service *RunnerSupportService) Handler {
	path, handler := runnerv1connect.NewRunnerSupportServiceHandler(&runnerSupportHandler{service: service})
	return NewHandler(path, handler)
}

func (h *runnerSupportHandler) ResolveRunContext(ctx context.Context, req *connect.Request[runnerv1.ResolveRunContextRequest]) (*connect.Response[runnerv1.ResolveRunContextResponse], error) {
	if h == nil || h.service == nil {
		return nil, connect.NewError(connect.CodeInternal, ErrRunnerSupportDependencyMissing)
	}
	ref := runSupportRefFromProto(req.Msg.GetRef())
	lease, err := h.service.ValidateRunLease(ctx, ValidateRunLeaseRequest{Lease: ref})
	if err != nil {
		return nil, runnerSupportConnectError(err)
	}
	resolved, err := h.service.ResolveRunContext(ctx, ResolveRunContextRequest{Lease: ref})
	if err != nil && !errors.Is(err, ErrRunnerSupportDependencyMissing) {
		return nil, runnerSupportConnectError(err)
	}
	botContext, _ := resolved.Context["bot"].(map[string]any)
	botGroupContext, _ := resolved.Context["bot_group"].(map[string]any)
	modelContext, _ := resolved.Context["model"].(map[string]any)
	return connect.NewResponse(&runnerv1.ResolveRunContextResponse{
		Lease:    runLeaseToProto(lease),
		Bot:      mapToStruct(botContext),
		BotGroup: mapToStruct(botGroupContext),
		Model:    mapToStruct(modelContext),
	}), nil
}

func (h *runnerSupportHandler) ValidateRunLease(ctx context.Context, req *connect.Request[runnerv1.ValidateRunLeaseRequest]) (*connect.Response[runnerv1.ValidateRunLeaseResponse], error) {
	if h == nil || h.service == nil {
		return nil, connect.NewError(connect.CodeInternal, ErrRunnerSupportDependencyMissing)
	}
	ref := runSupportRefFromProto(req.Msg.GetRef())
	ref.BotID = strings.TrimSpace(req.Msg.GetBotId())
	ref.SessionID = strings.TrimSpace(req.Msg.GetSessionId())
	ref.WorkspaceExecutorTarget = strings.TrimSpace(req.Msg.GetWorkspaceExecutorTarget())
	lease, err := h.service.ValidateRunLease(ctx, ValidateRunLeaseRequest{Lease: ref})
	if err != nil {
		return nil, runnerSupportConnectError(err)
	}
	return connect.NewResponse(&runnerv1.ValidateRunLeaseResponse{
		Lease: runLeaseToProto(lease),
		Valid: true,
	}), nil
}

func (h *runnerSupportHandler) IssueWorkspaceToken(ctx context.Context, req *connect.Request[runnerv1.IssueWorkspaceTokenRequest]) (*connect.Response[runnerv1.IssueWorkspaceTokenResponse], error) {
	if h == nil || h.service == nil {
		return nil, connect.NewError(connect.CodeInternal, ErrRunnerSupportDependencyMissing)
	}
	ref := runSupportRefFromProto(req.Msg.GetRef())
	ref.WorkspaceID = strings.TrimSpace(req.Msg.GetWorkspaceId())
	ref.WorkspaceExecutorTarget = strings.TrimSpace(req.Msg.GetWorkspaceExecutorTarget())
	resp, err := h.service.IssueWorkspaceToken(ctx, RunnerIssueWorkspaceTokenRequest{
		Lease:  ref,
		Scopes: append([]string(nil), req.Msg.GetScopes()...),
		TTL:    serviceauth.MaxWorkspaceTokenTTL,
	})
	if err != nil {
		return nil, runnerSupportConnectError(err)
	}
	return connect.NewResponse(&runnerv1.IssueWorkspaceTokenResponse{
		Token:     resp.Token,
		ExpiresAt: timestamppb.New(resp.ExpiresAt),
	}), nil
}

func (h *runnerSupportHandler) ReadSessionHistory(ctx context.Context, req *connect.Request[runnerv1.ReadSessionHistoryRequest]) (*connect.Response[runnerv1.ReadSessionHistoryResponse], error) {
	if h == nil || h.service == nil {
		return nil, connect.NewError(connect.CodeInternal, ErrRunnerSupportDependencyMissing)
	}
	ref := runSupportRefFromProto(req.Msg.GetRef())
	ref.SessionID = strings.TrimSpace(req.Msg.GetSessionId())
	resp, err := h.service.ReadSessionHistory(ctx, ReadSessionHistoryRequest{
		Lease: ref,
		Limit: req.Msg.GetLimit(),
	})
	if err != nil {
		return nil, runnerSupportConnectError(err)
	}
	messages := make([]*runnerv1.SessionMessage, 0, len(resp.Messages))
	for _, item := range resp.Messages {
		messages = append(messages, sessionMessageToProto(ref, item))
	}
	return connect.NewResponse(&runnerv1.ReadSessionHistoryResponse{Messages: messages}), nil
}

func (h *runnerSupportHandler) AppendRunEvent(ctx context.Context, req *connect.Request[runnerv1.AppendRunEventRequest]) (*connect.Response[runnerv1.AppendRunEventResponse], error) {
	if h == nil || h.service == nil {
		return nil, connect.NewError(connect.CodeInternal, ErrRunnerSupportDependencyMissing)
	}
	event := req.Msg.GetEvent()
	if event == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("event is required"))
	}
	payload, _ := protojson.Marshal(event)
	if err := h.service.AppendRunEvent(ctx, AppendRunEventRequest{
		Lease:       runSupportRefFromProto(req.Msg.GetRef()),
		EventType:   event.GetEventType(),
		Payload:     payload,
		Idempotency: event.GetEventId(),
	}); err != nil {
		return nil, runnerSupportConnectError(err)
	}
	return connect.NewResponse(&runnerv1.AppendRunEventResponse{EventId: event.GetEventId()}), nil
}

func (h *runnerSupportHandler) AppendSessionMessage(ctx context.Context, req *connect.Request[runnerv1.AppendSessionMessageRequest]) (*connect.Response[runnerv1.AppendSessionMessageResponse], error) {
	if h == nil || h.service == nil {
		return nil, connect.NewError(connect.CodeInternal, ErrRunnerSupportDependencyMissing)
	}
	msg := req.Msg.GetMessage()
	if msg == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("message is required"))
	}
	payload, _ := json.Marshal(structToMap(msg.GetPayload()))
	if err := h.service.AppendSessionMessage(ctx, AppendSessionMessageRequest{
		Lease: runSupportRefFromProto(req.Msg.GetRef()),
		Message: SessionMessage{
			Role:      msg.GetRole(),
			Content:   msg.GetText(),
			Metadata:  payload,
			CreatedAt: protoTime(msg.GetCreatedAt()),
		},
	}); err != nil {
		return nil, runnerSupportConnectError(err)
	}
	return connect.NewResponse(&runnerv1.AppendSessionMessageResponse{MessageId: msg.GetMessageId()}), nil
}

func (h *runnerSupportHandler) ResolveOutboundTarget(ctx context.Context, req *connect.Request[runnerv1.ResolveOutboundTargetRequest]) (*connect.Response[runnerv1.ResolveOutboundTargetResponse], error) {
	if h == nil || h.service == nil {
		return nil, connect.NewError(connect.CodeInternal, ErrRunnerSupportDependencyMissing)
	}
	resp, err := h.service.ResolveOutboundTarget(ctx, ResolveOutboundTargetRequest{
		Lease:          runSupportRefFromProto(req.Msg.GetRef()),
		ChannelType:    req.Msg.GetChannelType(),
		ConversationID: req.Msg.GetConversationId(),
	})
	if err != nil {
		return nil, runnerSupportConnectError(err)
	}
	return connect.NewResponse(&runnerv1.ResolveOutboundTargetResponse{
		ChannelConfigId: stringFromMap(resp.Target, "channel_config_id"),
		ChannelType:     firstNonEmptyString(stringFromMap(resp.Target, "channel_type"), req.Msg.GetChannelType()),
		ConversationId:  firstNonEmptyString(stringFromMap(resp.Target, "conversation_id"), req.Msg.GetConversationId()),
		Target:          mapToStruct(resp.Target),
	}), nil
}

func (h *runnerSupportHandler) RequestOutboundDispatch(ctx context.Context, req *connect.Request[runnerv1.RequestOutboundDispatchRequest]) (*connect.Response[runnerv1.RequestOutboundDispatchResponse], error) {
	if h == nil || h.service == nil {
		return nil, connect.NewError(connect.CodeInternal, ErrRunnerSupportDependencyMissing)
	}
	payload, _ := json.Marshal(structToMap(req.Msg.GetPayload()))
	if err := h.service.RequestOutboundDispatch(ctx, RequestOutboundDispatchRequest{
		Lease:           runSupportRefFromProto(req.Msg.GetRef()),
		ChannelConfigID: req.Msg.GetChannelConfigId(),
		ChannelType:     req.Msg.GetChannelType(),
		ConversationID:  req.Msg.GetConversationId(),
		Text:            req.Msg.GetText(),
		Target:          structToMap(req.Msg.GetPayload()),
		Payload:         payload,
	}); err != nil {
		return nil, runnerSupportConnectError(err)
	}
	return connect.NewResponse(&runnerv1.RequestOutboundDispatchResponse{Status: "queued"}), nil
}

func (h *runnerSupportHandler) ReadMemory(ctx context.Context, req *connect.Request[runnerv1.ReadMemoryRequest]) (*connect.Response[runnerv1.ReadMemoryResponse], error) {
	if h == nil || h.service == nil {
		return nil, connect.NewError(connect.CodeInternal, ErrRunnerSupportDependencyMissing)
	}
	resp, err := h.service.ReadMemory(ctx, ReadMemoryRequest{
		Lease: runSupportRefFromProto(req.Msg.GetRef()),
		Query: req.Msg.GetQuery(),
		Limit: req.Msg.GetLimit(),
	})
	if err != nil {
		return nil, runnerSupportConnectError(err)
	}
	memories := make([]*runnerv1.MemoryRecord, 0, len(resp.Items))
	for _, item := range resp.Items {
		memories = append(memories, &runnerv1.MemoryRecord{
			MemoryId: stringFromMap(item, "memory_id"),
			Scope:    stringFromMap(item, "scope"),
			Content:  stringFromMap(item, "content"),
			Metadata: mapToStruct(item),
		})
	}
	return connect.NewResponse(&runnerv1.ReadMemoryResponse{Memories: memories}), nil
}

func (h *runnerSupportHandler) WriteMemory(ctx context.Context, req *connect.Request[runnerv1.WriteMemoryRequest]) (*connect.Response[runnerv1.WriteMemoryResponse], error) {
	if h == nil || h.service == nil {
		return nil, connect.NewError(connect.CodeInternal, ErrRunnerSupportDependencyMissing)
	}
	memory := req.Msg.GetMemory()
	entry := map[string]any{}
	if memory != nil {
		entry = map[string]any{
			"memory_id": memory.GetMemoryId(),
			"scope":     memory.GetScope(),
			"content":   memory.GetContent(),
			"metadata":  structToMap(memory.GetMetadata()),
		}
	}
	if err := h.service.WriteMemory(ctx, WriteMemoryRequest{
		Lease:   runSupportRefFromProto(req.Msg.GetRef()),
		Entries: []map[string]any{entry},
	}); err != nil {
		return nil, runnerSupportConnectError(err)
	}
	return connect.NewResponse(&runnerv1.WriteMemoryResponse{MemoryId: stringFromMap(entry, "memory_id")}), nil
}

func (h *runnerSupportHandler) ResolveScopedSecret(ctx context.Context, req *connect.Request[runnerv1.ResolveScopedSecretRequest]) (*connect.Response[runnerv1.ResolveScopedSecretResponse], error) {
	if h == nil || h.service == nil {
		return nil, connect.NewError(connect.CodeInternal, ErrRunnerSupportDependencyMissing)
	}
	resp, err := h.service.ResolveScopedSecret(ctx, ResolveScopedSecretRequest{
		Lease: runSupportRefFromProto(req.Msg.GetRef()),
		Name:  req.Msg.GetSecretRef(),
	})
	if err != nil {
		return nil, runnerSupportConnectError(err)
	}
	return connect.NewResponse(&runnerv1.ResolveScopedSecretResponse{SecretValue: resp.Value}), nil
}

func (h *runnerSupportHandler) ResolveProviderCredentials(ctx context.Context, req *connect.Request[runnerv1.ResolveProviderCredentialsRequest]) (*connect.Response[runnerv1.ResolveProviderCredentialsResponse], error) {
	if h == nil || h.service == nil {
		return nil, connect.NewError(connect.CodeInternal, ErrRunnerSupportDependencyMissing)
	}
	resp, err := h.service.ResolveProviderCredentials(ctx, ResolveProviderCredentialsRequest{
		Lease:      runSupportRefFromProto(req.Msg.GetRef()),
		ProviderID: firstNonEmptyString(req.Msg.GetProviderId(), req.Msg.GetProviderName()),
	})
	if err != nil {
		return nil, runnerSupportConnectError(err)
	}
	return connect.NewResponse(&runnerv1.ResolveProviderCredentialsResponse{
		ProviderId:   req.Msg.GetProviderId(),
		ProviderName: req.Msg.GetProviderName(),
		Credentials:  mapToStruct(resp.Credentials),
	}), nil
}

func (h *runnerSupportHandler) EvaluateToolApprovalPolicy(ctx context.Context, req *connect.Request[runnerv1.EvaluateToolApprovalPolicyRequest]) (*connect.Response[runnerv1.EvaluateToolApprovalPolicyResponse], error) {
	if h == nil || h.service == nil {
		return nil, connect.NewError(connect.CodeInternal, ErrRunnerSupportDependencyMissing)
	}
	payload, _ := json.Marshal(structToMap(req.Msg.GetPayload()))
	resp, err := h.service.EvaluateToolApprovalPolicy(ctx, EvaluateToolApprovalPolicyRequest{
		Lease:    runSupportRefFromProto(req.Msg.GetRef()),
		ToolName: req.Msg.GetToolName(),
		Input:    payload,
	})
	if err != nil {
		return nil, runnerSupportConnectError(err)
	}
	decision := "bypass"
	if resp.RequiresApproval {
		decision = "requires_approval"
	}
	return connect.NewResponse(&runnerv1.EvaluateToolApprovalPolicyResponse{
		Decision: decision,
		Reason:   resp.Reason,
	}), nil
}

func (h *runnerSupportHandler) RequestToolApproval(ctx context.Context, req *connect.Request[runnerv1.RequestToolApprovalRequest]) (*connect.Response[runnerv1.RequestToolApprovalResponse], error) {
	if h == nil || h.service == nil {
		return nil, connect.NewError(connect.CodeInternal, ErrRunnerSupportDependencyMissing)
	}
	payload, _ := json.Marshal(structToMap(req.Msg.GetPayload()))
	resp, err := h.service.RequestToolApproval(ctx, RequestToolApprovalRequest{
		Lease:     runSupportRefFromProto(req.Msg.GetRef()),
		ToolName:  req.Msg.GetToolName(),
		ToolInput: payload,
	})
	if err != nil {
		return nil, runnerSupportConnectError(err)
	}
	return connect.NewResponse(&runnerv1.RequestToolApprovalResponse{
		ApprovalRequestId: resp.RequestID,
		Status:            "pending",
	}), nil
}

func runSupportRefFromProto(ref *runnerv1.RunSupportRef) RunLeaseRef {
	if ref == nil {
		return RunLeaseRef{}
	}
	return RunLeaseRef{
		RunID:            strings.TrimSpace(ref.GetRunId()),
		RunnerInstanceID: strings.TrimSpace(ref.GetRunnerInstanceId()),
		LeaseVersion:     ref.GetLeaseVersion(),
	}
}

func runLeaseToProto(lease serviceauth.RunLease) *runnerv1.RunLease {
	return &runnerv1.RunLease{
		RunId:                     lease.RunID,
		RunnerInstanceId:          lease.RunnerInstanceID,
		BotId:                     lease.BotID,
		BotGroupId:                lease.BotGroupID,
		SessionId:                 lease.SessionID,
		UserId:                    lease.UserID,
		PermissionSnapshotVersion: lease.PermissionSnapshotVersion,
		AllowedToolScopes:         append([]string(nil), lease.AllowedToolScopes...),
		WorkspaceExecutorTarget:   lease.WorkspaceExecutorTarget,
		WorkspaceId:               lease.WorkspaceID,
		ExpiresAt:                 timestamppb.New(lease.ExpiresAt),
		LeaseVersion:              lease.LeaseVersion,
	}
}

func sessionMessageToProto(ref RunLeaseRef, msg SessionMessage) *runnerv1.SessionMessage {
	return &runnerv1.SessionMessage{
		SessionId: ref.SessionID,
		BotId:     ref.BotID,
		Role:      msg.Role,
		Text:      msg.Content,
		Payload:   bytesToStruct(msg.Metadata),
		CreatedAt: timestamppb.New(msg.CreatedAt),
	}
}

func bytesToStruct(data []byte) *structpb.Struct {
	if len(data) == 0 {
		return nil
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil
	}
	return mapToStruct(raw)
}

func protoTime(value *timestamppb.Timestamp) time.Time {
	if value == nil {
		return time.Time{}
	}
	return value.AsTime()
}

func runnerSupportConnectError(err error) error {
	switch {
	case errors.Is(err, serviceauth.ErrUnauthenticated):
		return connect.NewError(connect.CodeUnauthenticated, err)
	case errors.Is(err, serviceauth.ErrPermissionDenied):
		return connect.NewError(connect.CodePermissionDenied, err)
	case errors.Is(err, ErrRunnerSupportDependencyMissing):
		return connect.NewError(connect.CodeInternal, err)
	default:
		return connect.NewError(connect.CodeInvalidArgument, err)
	}
}

var _ runnerv1connect.RunnerSupportServiceHandler = (*runnerSupportHandler)(nil)
