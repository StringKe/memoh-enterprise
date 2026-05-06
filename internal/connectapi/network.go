package connectapi

import (
	"context"
	"errors"
	"strings"

	"connectrpc.com/connect"

	privatev1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1"
	"github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1/privatev1connect"
	"github.com/memohai/memoh/internal/iam/rbac"
	netctl "github.com/memohai/memoh/internal/network"
)

type NetworkService struct {
	network *netctl.Service
	bots    *BotService
}

func NewNetworkService(network *netctl.Service, bots *BotService) *NetworkService {
	return &NetworkService{network: network, bots: bots}
}

func NewNetworkHandler(service *NetworkService) Handler {
	path, handler := privatev1connect.NewNetworkServiceHandler(service)
	return NewHandler(path, handler)
}

func (s *NetworkService) ListNetworkMeta(ctx context.Context, _ *connect.Request[privatev1.ListNetworkMetaRequest]) (*connect.Response[privatev1.ListNetworkMetaResponse], error) {
	if _, err := UserIDFromContext(ctx); err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	items := s.network.ListMeta(ctx)
	out := make([]*privatev1.NetworkMeta, 0, len(items))
	for _, item := range items {
		out = append(out, networkMetaToProto(item))
	}
	return connect.NewResponse(&privatev1.ListNetworkMetaResponse{Actions: out}), nil
}

func (s *NetworkService) GetBotNetworkStatus(ctx context.Context, req *connect.Request[privatev1.GetBotNetworkStatusRequest]) (*connect.Response[privatev1.GetBotNetworkStatusResponse], error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	botID := strings.TrimSpace(req.Msg.GetBotId())
	if botID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("bot_id is required"))
	}
	if err := s.bots.requireBotPermission(ctx, userID, botID, rbac.PermissionBotRead); err != nil {
		return nil, botConnectError(err)
	}
	status, err := s.network.StatusBot(ctx, botID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	return connect.NewResponse(&privatev1.GetBotNetworkStatusResponse{
		Status: networkBotStatusToProto(botID, status),
	}), nil
}

func (s *NetworkService) ListBotNetworkNodes(ctx context.Context, req *connect.Request[privatev1.ListBotNetworkNodesRequest]) (*connect.Response[privatev1.ListBotNetworkNodesResponse], error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	botID := strings.TrimSpace(req.Msg.GetBotId())
	if botID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("bot_id is required"))
	}
	if err := s.bots.requireBotPermission(ctx, userID, botID, rbac.PermissionBotRead); err != nil {
		return nil, botConnectError(err)
	}
	nodes, err := s.network.ListBotNodes(ctx, botID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	out := make([]*privatev1.BotNetworkNode, 0, len(nodes.Items))
	for _, item := range nodes.Items {
		out = append(out, networkNodeToProto(botID, nodes.Provider, item))
	}
	return connect.NewResponse(&privatev1.ListBotNetworkNodesResponse{Nodes: out}), nil
}

func (s *NetworkService) ExecuteBotNetworkAction(ctx context.Context, req *connect.Request[privatev1.ExecuteBotNetworkActionRequest]) (*connect.Response[privatev1.ExecuteBotNetworkActionResponse], error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	botID := strings.TrimSpace(req.Msg.GetBotId())
	if botID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("bot_id is required"))
	}
	actionID := strings.TrimSpace(req.Msg.GetActionId())
	if actionID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("action_id is required"))
	}
	if err := s.bots.requireBotPermission(ctx, userID, botID, rbac.PermissionBotUpdate); err != nil {
		return nil, botConnectError(err)
	}
	result, err := s.network.ExecuteActionBot(ctx, botID, actionID, structToMap(req.Msg.GetPayload()))
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	return connect.NewResponse(&privatev1.ExecuteBotNetworkActionResponse{
		Status: providerStatusToNetworkProto(botID, result.Status),
		Result: mapToStruct(map[string]any{
			"action_id": result.ActionID,
			"output":    result.Output,
		}),
	}), nil
}

func networkMetaToProto(value netctl.ProviderDescriptor) *privatev1.NetworkMeta {
	return &privatev1.NetworkMeta{
		ActionId:    value.Kind,
		DisplayName: value.DisplayName,
		Schema:      valueToStruct(value),
	}
}

func networkBotStatusToProto(botID string, value netctl.BotStatus) *privatev1.BotNetworkStatus {
	return &privatev1.BotNetworkStatus{
		BotId:    botID,
		Status:   value.State,
		Message:  firstNonEmpty(value.Description, value.Message),
		Metadata: valueToStruct(value),
	}
}

func providerStatusToNetworkProto(botID string, value netctl.ProviderStatus) *privatev1.BotNetworkStatus {
	metadata := map[string]any{
		"state":       string(value.State),
		"title":       value.Title,
		"description": value.Description,
		"details":     value.Details,
	}
	return &privatev1.BotNetworkStatus{
		BotId:    botID,
		Status:   string(value.State),
		Message:  value.Description,
		Metadata: mapToStruct(metadata),
	}
}

func networkNodeToProto(botID, provider string, value netctl.NodeOption) *privatev1.BotNetworkNode {
	status := "offline"
	if value.Online {
		status = "online"
	}
	return &privatev1.BotNetworkNode{
		Id:       firstNonEmpty(value.ID, value.Value),
		BotId:    botID,
		Kind:     provider,
		Status:   status,
		Metadata: valueToStruct(value),
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
