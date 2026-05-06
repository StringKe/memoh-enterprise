package connectapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"

	integrationv1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/integration/v1"
	"github.com/memohai/memoh/internal/integrations"
	"github.com/memohai/memoh/internal/serviceauth"
)

type integrationGatewayTokenResolver func(context.Context, string) (integrations.APIToken, error)

type IntegrationGatewayService struct {
	backend  integrations.GatewayBackend
	hub      *integrations.Hub
	resolve  integrationGatewayTokenResolver
	verifier *serviceauth.Verifier
	now      func() time.Time
}

func NewIntegrationGatewayService(service *integrations.Service, verifier *serviceauth.Verifier) *IntegrationGatewayService {
	hub := integrations.NewHub()
	return NewIntegrationGatewayServiceWithBackend(
		integrations.NewLocalGatewayBackend(service, hub),
		hub,
		service.GetAPIToken,
		verifier,
	)
}

func NewIntegrationGatewayServiceWithBackend(backend integrations.GatewayBackend, hub *integrations.Hub, resolve integrationGatewayTokenResolver, verifier *serviceauth.Verifier) *IntegrationGatewayService {
	if hub == nil {
		hub = integrations.NewHub()
	}
	return &IntegrationGatewayService{
		backend:  backend,
		hub:      hub,
		resolve:  resolve,
		verifier: verifier,
		now:      time.Now,
	}
}

func NewIntegrationGatewayHandler(service *IntegrationGatewayService) Handler {
	mux := http.NewServeMux()
	registerIntegrationGatewayProcedure(mux, integrations.IntegrationGatewayValidateTokenProcedure, service.ValidateToken)
	registerIntegrationGatewayProcedure(mux, integrations.IntegrationGatewayAuthorizeBotProcedure, service.AuthorizeBot)
	registerIntegrationGatewayProcedure(mux, integrations.IntegrationGatewayAuthorizeBotGroupProcedure, service.AuthorizeBotGroup)
	registerIntegrationGatewayProcedure(mux, integrations.IntegrationGatewayAuthorizeEventProcedure, service.AuthorizeEvent)
	registerIntegrationGatewayProcedure(mux, integrations.IntegrationGatewayAckEventProcedure, service.AckEvent)
	registerIntegrationGatewayProcedure(mux, integrations.IntegrationGatewayCreateSessionProcedure, service.CreateSession)
	registerIntegrationGatewayProcedure(mux, integrations.IntegrationGatewayGetSessionStatusProcedure, service.GetSessionStatus)
	registerIntegrationGatewayProcedure(mux, integrations.IntegrationGatewaySendBotMessageProcedure, service.SendBotMessage)
	registerIntegrationGatewayProcedure(mux, integrations.IntegrationGatewayRequestActionProcedure, service.RequestAction)
	registerIntegrationGatewayProcedure(mux, integrations.IntegrationGatewayPublishEventProcedure, service.PublishEvent)
	return NewHandler("/"+integrations.IntegrationGatewayServiceName+"/", mux)
}

func registerIntegrationGatewayProcedure(mux *http.ServeMux, procedure string, fn func(context.Context, *structpb.Struct) (*structpb.Struct, error)) {
	mux.Handle(procedure, connect.NewUnaryHandlerSimple(procedure, fn))
}

func (s *IntegrationGatewayService) ValidateToken(ctx context.Context, req *structpb.Struct) (*structpb.Struct, error) {
	if err := s.requireServiceToken(ctx); err != nil {
		return nil, err
	}
	if s.backend == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("integration gateway backend is not configured"))
	}
	identity, err := s.backend.ValidateToken(ctx, structString(req, "token"))
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	return structpb.NewStruct(map[string]any{"token": integrationAPITokenMap(identity.Token)})
}

func (s *IntegrationGatewayService) AuthorizeBot(ctx context.Context, req *structpb.Struct) (*structpb.Struct, error) {
	identity, err := s.identityFromRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	if err := s.backend.AuthorizeBot(ctx, identity, structString(req, "bot_id", "botId"), structString(req, "action")); err != nil {
		return nil, connect.NewError(connect.CodePermissionDenied, err)
	}
	return structpb.NewStruct(map[string]any{})
}

func (s *IntegrationGatewayService) AuthorizeBotGroup(ctx context.Context, req *structpb.Struct) (*structpb.Struct, error) {
	identity, err := s.identityFromRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	if err := s.backend.AuthorizeBotGroup(ctx, identity, structString(req, "bot_group_id", "botGroupId")); err != nil {
		return nil, connect.NewError(connect.CodePermissionDenied, err)
	}
	return structpb.NewStruct(map[string]any{})
}

func (s *IntegrationGatewayService) AuthorizeEvent(ctx context.Context, req *structpb.Struct) (*structpb.Struct, error) {
	identity, err := s.identityFromRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	if err := s.backend.AuthorizeEvent(ctx, identity, structString(req, "event_type", "eventType")); err != nil {
		return nil, connect.NewError(connect.CodePermissionDenied, err)
	}
	return structpb.NewStruct(map[string]any{})
}

func (s *IntegrationGatewayService) AckEvent(ctx context.Context, req *structpb.Struct) (*structpb.Struct, error) {
	identity, err := s.identityFromRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	if err := s.backend.AckEvent(ctx, identity, structString(req, "event_id", "eventId")); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return structpb.NewStruct(map[string]any{})
}

func (s *IntegrationGatewayService) CreateSession(ctx context.Context, req *structpb.Struct) (*structpb.Struct, error) {
	identity, err := s.identityFromRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	session, err := s.backend.CreateSession(ctx, identity, structString(req, "bot_id", "botId"), structString(req, "external_session_id", "externalSessionId"), structStringMap(req, "metadata"))
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return structpb.NewStruct(map[string]any{"session": integrationSessionMap(session)})
}

func (s *IntegrationGatewayService) GetSessionStatus(ctx context.Context, req *structpb.Struct) (*structpb.Struct, error) {
	identity, err := s.identityFromRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	status, err := s.backend.GetSessionStatus(ctx, identity, structString(req, "session_id", "sessionId"))
	if err != nil {
		return nil, connect.NewError(connect.CodePermissionDenied, err)
	}
	return structpb.NewStruct(map[string]any{
		"session_id": status.SessionID,
		"bot_id":     status.BotID,
		"status":     status.Status,
	})
}

func (s *IntegrationGatewayService) SendBotMessage(ctx context.Context, req *structpb.Struct) (*structpb.Struct, error) {
	identity, err := s.identityFromRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	result, err := s.backend.SendBotMessage(ctx, identity, integrations.SendBotMessageGatewayRequest{
		BotID:     structString(req, "bot_id", "botId"),
		SessionID: structString(req, "session_id", "sessionId"),
		Text:      structString(req, "text"),
		Metadata:  structStringMap(req, "metadata"),
	})
	if err != nil {
		return nil, connect.NewError(connect.CodePermissionDenied, err)
	}
	return structpb.NewStruct(map[string]any{
		"message_id": result.MessageID,
		"session_id": result.SessionID,
	})
}

func (s *IntegrationGatewayService) RequestAction(ctx context.Context, req *structpb.Struct) (*structpb.Struct, error) {
	identity, err := s.identityFromRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	result, err := s.backend.RequestAction(ctx, identity, integrations.RequestActionGatewayRequest{
		BotID:       structString(req, "bot_id", "botId"),
		ActionType:  structString(req, "action_type", "actionType"),
		PayloadJSON: structString(req, "payload_json", "payloadJson"),
		Metadata:    structStringMap(req, "metadata"),
	})
	if err != nil {
		return nil, connect.NewError(connect.CodePermissionDenied, err)
	}
	return structpb.NewStruct(map[string]any{
		"action_id":   result.ActionID,
		"bot_id":      result.BotID,
		"action_type": result.ActionType,
		"status":      result.Status,
	})
}

func (s *IntegrationGatewayService) PublishEvent(ctx context.Context, req *structpb.Struct) (*structpb.Struct, error) {
	if err := s.requireServiceToken(ctx); err != nil {
		return nil, err
	}
	eventJSON := structString(req, "event_json", "eventJson")
	if strings.TrimSpace(eventJSON) == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("event_json is required"))
	}
	var event integrationv1.IntegrationEvent
	if err := protojson.Unmarshal([]byte(eventJSON), &event); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	s.hub.Publish(&event)
	return structpb.NewStruct(map[string]any{})
}

func (s *IntegrationGatewayService) identityFromRequest(ctx context.Context, req *structpb.Struct) (integrations.TokenIdentity, error) {
	if err := s.requireServiceToken(ctx); err != nil {
		return integrations.TokenIdentity{}, err
	}
	if s.backend == nil {
		return integrations.TokenIdentity{}, connect.NewError(connect.CodeInternal, errors.New("integration gateway backend is not configured"))
	}
	if s.resolve == nil {
		return integrations.TokenIdentity{}, connect.NewError(connect.CodeInternal, errors.New("integration token resolver is not configured"))
	}
	tokenID := structString(req, "token_id", "tokenId")
	if tokenID == "" {
		return integrations.TokenIdentity{}, connect.NewError(connect.CodeInvalidArgument, errors.New("token_id is required"))
	}
	token, err := s.resolve(ctx, tokenID)
	if err != nil {
		return integrations.TokenIdentity{}, connect.NewError(connect.CodeUnauthenticated, err)
	}
	return integrations.TokenIdentity{Token: token}, nil
}

func (s *IntegrationGatewayService) requireServiceToken(ctx context.Context) error {
	if s.verifier == nil {
		return connect.NewError(connect.CodeUnauthenticated, errors.New("service auth verifier is not configured"))
	}
	rawToken := bearerTokenFromContext(ctx)
	if rawToken == "" {
		return connect.NewError(connect.CodeUnauthenticated, errors.New("missing service token"))
	}
	claims, err := s.verifier.Verify(rawToken)
	if err != nil {
		return connect.NewError(connect.CodeUnauthenticated, err)
	}
	if err := serviceauth.RequireScope(claims, serviceauth.AudienceServer, serviceauth.ScopeIntegrationGateway, s.now().UTC()); err != nil {
		if errors.Is(err, serviceauth.ErrPermissionDenied) {
			return connect.NewError(connect.CodePermissionDenied, err)
		}
		return connect.NewError(connect.CodeUnauthenticated, err)
	}
	return nil
}

func bearerTokenFromContext(ctx context.Context) string {
	callInfo, ok := connect.CallInfoForHandlerContext(ctx)
	if !ok {
		return ""
	}
	value := strings.TrimSpace(callInfo.RequestHeader().Get("Authorization"))
	if value == "" {
		return ""
	}
	parts := strings.Fields(value)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

func integrationAPITokenMap(token integrations.APIToken) map[string]any {
	return map[string]any{
		"id":                   token.ID,
		"name":                 token.Name,
		"scope_type":           token.ScopeType,
		"scope_bot_id":         token.ScopeBotID,
		"scope_bot_group_id":   token.ScopeBotGroupID,
		"allowed_event_types":  stringSliceAny(token.AllowedEventTypes),
		"allowed_action_types": stringSliceAny(token.AllowedActionTypes),
		"expires_at":           optionalTimeString(token.ExpiresAt),
		"disabled_at":          optionalTimeString(token.DisabledAt),
		"last_used_at":         optionalTimeString(token.LastUsedAt),
		"created_by_user_id":   token.CreatedByUserID,
		"created_at":           timeString(token.CreatedAt),
		"updated_at":           timeString(token.UpdatedAt),
	}
}

func integrationSessionMap(session integrations.IntegrationSession) map[string]any {
	return map[string]any{
		"session_id":          session.ID,
		"bot_id":              session.BotID,
		"external_session_id": session.ExternalSessionID,
		"metadata":            stringMapAny(session.Metadata),
		"created_at":          timeString(session.CreatedAt),
	}
}

func structString(msg *structpb.Struct, names ...string) string {
	if msg == nil {
		return ""
	}
	for _, name := range names {
		if value := msg.Fields[name]; value != nil {
			return strings.TrimSpace(value.GetStringValue())
		}
	}
	return ""
}

func structStringMap(msg *structpb.Struct, names ...string) map[string]string {
	if msg == nil {
		return nil
	}
	for _, name := range names {
		value := msg.Fields[name]
		if value == nil || value.GetStructValue() == nil {
			continue
		}
		result := make(map[string]string, len(value.GetStructValue().Fields))
		for key, field := range value.GetStructValue().Fields {
			result[key] = field.GetStringValue()
		}
		return result
	}
	return nil
}

func stringSliceAny(items []string) []any {
	if len(items) == 0 {
		return []any{}
	}
	result := make([]any, 0, len(items))
	for _, item := range items {
		result = append(result, item)
	}
	return result
}

func stringMapAny(input map[string]string) map[string]any {
	if len(input) == 0 {
		return map[string]any{}
	}
	result := make(map[string]any, len(input))
	for key, value := range input {
		result[key] = value
	}
	return result
}

func optionalTimeString(value *time.Time) string {
	if value == nil || value.IsZero() {
		return ""
	}
	return timeString(*value)
}

func timeString(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}
