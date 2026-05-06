package integrations

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
)

const (
	IntegrationGatewayServiceName = "memoh.private.v1.IntegrationGatewayService"

	IntegrationGatewayValidateTokenProcedure     = "/memoh.private.v1.IntegrationGatewayService/ValidateToken"
	IntegrationGatewayAuthorizeBotProcedure      = "/memoh.private.v1.IntegrationGatewayService/AuthorizeBot"
	IntegrationGatewayAuthorizeBotGroupProcedure = "/memoh.private.v1.IntegrationGatewayService/AuthorizeBotGroup"
	IntegrationGatewayAuthorizeEventProcedure    = "/memoh.private.v1.IntegrationGatewayService/AuthorizeEvent"
	IntegrationGatewayAckEventProcedure          = "/memoh.private.v1.IntegrationGatewayService/AckEvent"
	IntegrationGatewayCreateSessionProcedure     = "/memoh.private.v1.IntegrationGatewayService/CreateSession"
	IntegrationGatewayGetSessionStatusProcedure  = "/memoh.private.v1.IntegrationGatewayService/GetSessionStatus"
	IntegrationGatewaySendBotMessageProcedure    = "/memoh.private.v1.IntegrationGatewayService/SendBotMessage"
	IntegrationGatewayRequestActionProcedure     = "/memoh.private.v1.IntegrationGatewayService/RequestAction"
	IntegrationGatewayPublishEventProcedure      = "/memoh.private.v1.IntegrationGatewayService/PublishEvent"
)

type GatewayClientOptions struct {
	BaseURL      string
	HTTPClient   connect.HTTPClient
	ServiceToken string
	Header       http.Header
}

type GatewayClient struct {
	header       http.Header
	serviceToken string

	validateToken     *connect.Client[structpb.Struct, structpb.Struct]
	authorizeBot      *connect.Client[structpb.Struct, structpb.Struct]
	authorizeBotGroup *connect.Client[structpb.Struct, structpb.Struct]
	authorizeEvent    *connect.Client[structpb.Struct, structpb.Struct]
	ackEvent          *connect.Client[structpb.Struct, structpb.Struct]
	createSession     *connect.Client[structpb.Struct, structpb.Struct]
	getSessionStatus  *connect.Client[structpb.Struct, structpb.Struct]
	sendBotMessage    *connect.Client[structpb.Struct, structpb.Struct]
	requestAction     *connect.Client[structpb.Struct, structpb.Struct]
	publishEvent      *connect.Client[structpb.Struct, structpb.Struct]
}

func NewGatewayClient(options GatewayClientOptions) *GatewayClient {
	httpClient := options.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	baseURL := strings.TrimRight(options.BaseURL, "/")
	return &GatewayClient{
		header:       cloneHeader(options.Header),
		serviceToken: strings.TrimSpace(options.ServiceToken),

		validateToken:     connect.NewClient[structpb.Struct, structpb.Struct](httpClient, baseURL+IntegrationGatewayValidateTokenProcedure),
		authorizeBot:      connect.NewClient[structpb.Struct, structpb.Struct](httpClient, baseURL+IntegrationGatewayAuthorizeBotProcedure),
		authorizeBotGroup: connect.NewClient[structpb.Struct, structpb.Struct](httpClient, baseURL+IntegrationGatewayAuthorizeBotGroupProcedure),
		authorizeEvent:    connect.NewClient[structpb.Struct, structpb.Struct](httpClient, baseURL+IntegrationGatewayAuthorizeEventProcedure),
		ackEvent:          connect.NewClient[structpb.Struct, structpb.Struct](httpClient, baseURL+IntegrationGatewayAckEventProcedure),
		createSession:     connect.NewClient[structpb.Struct, structpb.Struct](httpClient, baseURL+IntegrationGatewayCreateSessionProcedure),
		getSessionStatus:  connect.NewClient[structpb.Struct, structpb.Struct](httpClient, baseURL+IntegrationGatewayGetSessionStatusProcedure),
		sendBotMessage:    connect.NewClient[structpb.Struct, structpb.Struct](httpClient, baseURL+IntegrationGatewaySendBotMessageProcedure),
		requestAction:     connect.NewClient[structpb.Struct, structpb.Struct](httpClient, baseURL+IntegrationGatewayRequestActionProcedure),
		publishEvent:      connect.NewClient[structpb.Struct, structpb.Struct](httpClient, baseURL+IntegrationGatewayPublishEventProcedure),
	}
}

func (c *GatewayClient) ValidateToken(ctx context.Context, rawToken string) (TokenIdentity, error) {
	res, err := c.call(ctx, c.validateToken, map[string]any{"token": rawToken})
	if err != nil {
		return TokenIdentity{}, err
	}
	token, err := apiTokenFromStruct(res)
	if err != nil {
		return TokenIdentity{}, err
	}
	return TokenIdentity{Token: token}, nil
}

func (c *GatewayClient) AuthorizeBot(ctx context.Context, identity TokenIdentity, botID string, action string) error {
	_, err := c.call(ctx, c.authorizeBot, map[string]any{
		"token_id": identity.Token.ID,
		"bot_id":   botID,
		"action":   action,
	})
	return err
}

func (c *GatewayClient) AuthorizeBotGroup(ctx context.Context, identity TokenIdentity, botGroupID string) error {
	_, err := c.call(ctx, c.authorizeBotGroup, map[string]any{
		"token_id":     identity.Token.ID,
		"bot_group_id": botGroupID,
	})
	return err
}

func (c *GatewayClient) AuthorizeEvent(ctx context.Context, identity TokenIdentity, eventType string) error {
	_, err := c.call(ctx, c.authorizeEvent, map[string]any{
		"token_id":   identity.Token.ID,
		"event_type": eventType,
	})
	return err
}

func (c *GatewayClient) AckEvent(ctx context.Context, identity TokenIdentity, eventID string) error {
	_, err := c.call(ctx, c.ackEvent, map[string]any{
		"token_id": identity.Token.ID,
		"event_id": eventID,
	})
	return err
}

func (c *GatewayClient) CreateSession(ctx context.Context, identity TokenIdentity, botID string, externalSessionID string, metadata map[string]string) (IntegrationSession, error) {
	res, err := c.call(ctx, c.createSession, map[string]any{
		"token_id":            identity.Token.ID,
		"bot_id":              botID,
		"external_session_id": externalSessionID,
		"metadata":            stringMapToAny(metadata),
	})
	if err != nil {
		return IntegrationSession{}, err
	}
	return integrationSessionFromStruct(res)
}

func (c *GatewayClient) GetSessionStatus(ctx context.Context, identity TokenIdentity, sessionID string) (SessionStatus, error) {
	res, err := c.call(ctx, c.getSessionStatus, map[string]any{
		"token_id":   identity.Token.ID,
		"session_id": sessionID,
	})
	if err != nil {
		return SessionStatus{}, err
	}
	return SessionStatus{
		SessionID: stringStructField(res, "session_id", "sessionId"),
		BotID:     stringStructField(res, "bot_id", "botId"),
		Status:    stringStructField(res, "status"),
	}, nil
}

func (c *GatewayClient) SendBotMessage(ctx context.Context, identity TokenIdentity, req SendBotMessageGatewayRequest) (SendBotMessageResult, error) {
	res, err := c.call(ctx, c.sendBotMessage, map[string]any{
		"token_id":   identity.Token.ID,
		"bot_id":     req.BotID,
		"session_id": req.SessionID,
		"text":       req.Text,
		"metadata":   stringMapToAny(req.Metadata),
	})
	if err != nil {
		return SendBotMessageResult{}, err
	}
	return SendBotMessageResult{
		MessageID: stringStructField(res, "message_id", "messageId"),
		SessionID: stringStructField(res, "session_id", "sessionId"),
	}, nil
}

func (c *GatewayClient) RequestAction(ctx context.Context, identity TokenIdentity, req RequestActionGatewayRequest) (RequestActionResult, error) {
	res, err := c.call(ctx, c.requestAction, map[string]any{
		"token_id":     identity.Token.ID,
		"bot_id":       req.BotID,
		"action_type":  req.ActionType,
		"payload_json": req.PayloadJSON,
		"metadata":     stringMapToAny(req.Metadata),
	})
	if err != nil {
		return RequestActionResult{}, err
	}
	return RequestActionResult{
		ActionID:   stringStructField(res, "action_id", "actionId"),
		BotID:      stringStructField(res, "bot_id", "botId"),
		ActionType: stringStructField(res, "action_type", "actionType"),
		Status:     stringStructField(res, "status"),
	}, nil
}

func (c *GatewayClient) PublishEvent(ctx context.Context, event *integrationv1.IntegrationEvent) error {
	payload, err := protojson.MarshalOptions{EmitUnpopulated: false}.Marshal(event)
	if err != nil {
		return err
	}
	_, err = c.call(ctx, c.publishEvent, map[string]any{"event_json": string(payload)})
	return err
}

func (c *GatewayClient) call(ctx context.Context, client *connect.Client[structpb.Struct, structpb.Struct], fields map[string]any) (*structpb.Struct, error) {
	msg, err := structpb.NewStruct(fields)
	if err != nil {
		return nil, err
	}
	req := connect.NewRequest(msg)
	for key, values := range c.header {
		for _, value := range values {
			req.Header().Add(key, value)
		}
	}
	if c.serviceToken != "" {
		req.Header().Set("Authorization", "Bearer "+c.serviceToken)
	}
	res, err := client.CallUnary(ctx, req)
	if err != nil {
		return nil, err
	}
	if res.Msg == nil {
		return nil, errors.New("integration gateway returned nil response")
	}
	return res.Msg, nil
}

func apiTokenFromStruct(msg *structpb.Struct) (APIToken, error) {
	tokenMsg := nestedStructField(msg, "token")
	if tokenMsg == nil {
		tokenMsg = msg
	}
	if tokenMsg == nil {
		return APIToken{}, errors.New("integration token response is missing")
	}
	return APIToken{
		ID:                 stringStructField(tokenMsg, "id"),
		Name:               stringStructField(tokenMsg, "name"),
		ScopeType:          stringStructField(tokenMsg, "scope_type", "scopeType"),
		ScopeBotID:         stringStructField(tokenMsg, "scope_bot_id", "scopeBotId"),
		ScopeBotGroupID:    stringStructField(tokenMsg, "scope_bot_group_id", "scopeBotGroupId"),
		AllowedEventTypes:  stringListStructField(tokenMsg, "allowed_event_types", "allowedEventTypes"),
		AllowedActionTypes: stringListStructField(tokenMsg, "allowed_action_types", "allowedActionTypes"),
		ExpiresAt:          timeStructField(tokenMsg, "expires_at", "expiresAt"),
		DisabledAt:         timeStructField(tokenMsg, "disabled_at", "disabledAt"),
		LastUsedAt:         timeStructField(tokenMsg, "last_used_at", "lastUsedAt"),
		CreatedByUserID:    stringStructField(tokenMsg, "created_by_user_id", "createdByUserId"),
		CreatedAt:          timeValueStructField(tokenMsg, "created_at", "createdAt"),
		UpdatedAt:          timeValueStructField(tokenMsg, "updated_at", "updatedAt"),
	}, nil
}

func integrationSessionFromStruct(msg *structpb.Struct) (IntegrationSession, error) {
	sessionMsg := nestedStructField(msg, "session")
	if sessionMsg == nil {
		sessionMsg = msg
	}
	if sessionMsg == nil {
		return IntegrationSession{}, errors.New("integration session response is missing")
	}
	return IntegrationSession{
		ID:                stringStructField(sessionMsg, "session_id", "sessionId", "id"),
		BotID:             stringStructField(sessionMsg, "bot_id", "botId"),
		ExternalSessionID: stringStructField(sessionMsg, "external_session_id", "externalSessionId"),
		Metadata:          stringMapStructField(sessionMsg, "metadata"),
		CreatedAt:         timeValueStructField(sessionMsg, "created_at", "createdAt"),
	}, nil
}

func nestedStructField(msg *structpb.Struct, names ...string) *structpb.Struct {
	if msg == nil {
		return nil
	}
	for _, name := range names {
		if value := msg.Fields[name]; value != nil {
			return value.GetStructValue()
		}
	}
	return nil
}

func stringStructField(msg *structpb.Struct, names ...string) string {
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

func stringListStructField(msg *structpb.Struct, names ...string) []string {
	if msg == nil {
		return nil
	}
	for _, name := range names {
		value := msg.Fields[name]
		if value == nil || value.GetListValue() == nil {
			continue
		}
		items := value.GetListValue().GetValues()
		result := make([]string, 0, len(items))
		for _, item := range items {
			if text := strings.TrimSpace(item.GetStringValue()); text != "" {
				result = append(result, text)
			}
		}
		return result
	}
	return nil
}

func stringMapStructField(msg *structpb.Struct, names ...string) map[string]string {
	value := nestedStructField(msg, names...)
	if value == nil {
		return nil
	}
	result := make(map[string]string, len(value.Fields))
	for key, field := range value.Fields {
		result[key] = field.GetStringValue()
	}
	return result
}

func timeStructField(msg *structpb.Struct, names ...string) *time.Time {
	value := timeValueStructField(msg, names...)
	if value.IsZero() {
		return nil
	}
	return &value
}

func timeValueStructField(msg *structpb.Struct, names ...string) time.Time {
	raw := stringStructField(msg, names...)
	if raw == "" {
		return time.Time{}
	}
	value, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}
	}
	return value
}

func stringMapToAny(values map[string]string) map[string]any {
	if len(values) == 0 {
		return map[string]any{}
	}
	result := make(map[string]any, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

func cloneHeader(header http.Header) http.Header {
	if len(header) == 0 {
		return nil
	}
	cloned := make(http.Header, len(header))
	for key, values := range header {
		cloned[key] = append([]string(nil), values...)
	}
	return cloned
}
