package integrations

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	integrationv1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/integration/v1"
)

const (
	WebSocketPath      = "/integration/v1/ws"
	wsHeartbeatTimeout = 45 * time.Second
)

type WebSocketHandler struct {
	backend          GatewayBackend
	hub              *Hub
	logger           *slog.Logger
	upgrader         websocket.Upgrader
	heartbeatTimeout time.Duration
}

func NewWebSocketHandler(log *slog.Logger, service *Service) *WebSocketHandler {
	if log == nil {
		log = slog.Default()
	}
	hub := NewHub()
	return NewWebSocketHandlerWithBackend(log, NewLocalGatewayBackend(service, hub), hub)
}

func NewGatewayWebSocketHandler(log *slog.Logger, backend GatewayBackend) *WebSocketHandler {
	return NewWebSocketHandlerWithBackend(log, backend, NewHub())
}

func NewWebSocketHandlerWithBackend(log *slog.Logger, backend GatewayBackend, hub *Hub) *WebSocketHandler {
	if log == nil {
		log = slog.Default()
	}
	if hub == nil {
		hub = NewHub()
	}
	return &WebSocketHandler{
		backend:          backend,
		hub:              hub,
		logger:           log.With(slog.String("handler", "integration_ws")),
		heartbeatTimeout: wsHeartbeatTimeout,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(*http.Request) bool { return true },
		},
	}
}

func (h *WebSocketHandler) HTTPHandler() http.Handler {
	return h
}

func (h *WebSocketHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != WebSocketPath {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	h.handle(w, r)
}

func (h *WebSocketHandler) handle(w http.ResponseWriter, r *http.Request) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.logger.Warn("integration websocket upgrade failed", slog.Any("error", err))
		return
	}
	defer func() { _ = conn.Close() }()

	ctx := r.Context()
	identity, err := h.authenticate(ctx, conn)
	if err != nil {
		_ = h.writeError(conn, nil, "", "unauthenticated", err.Error())
		return
	}
	if err := h.writeEnvelope(conn, &integrationv1.Envelope{
		Version:       wsProtocolVersion,
		MessageId:     uuid.NewString(),
		CorrelationId: "",
		Payload: &integrationv1.Envelope_AuthResponse{AuthResponse: &integrationv1.AuthResponse{
			IntegrationId:   identity.Token.ID,
			ScopeType:       identity.Token.ScopeType,
			ScopeBotId:      identity.Token.ScopeBotID,
			ScopeBotGroupId: identity.Token.ScopeBotGroupID,
		}},
	}); err != nil {
		return
	}
	var writeMu sync.Mutex
	connectionID := h.hub.Register(identity, func(envelope *integrationv1.Envelope) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return h.writeEnvelope(conn, envelope)
	})
	defer h.hub.Unregister(connectionID)
	_ = h.readLoop(ctx, conn, identity, connectionID, &writeMu)
}

func (h *WebSocketHandler) authenticate(ctx context.Context, conn *websocket.Conn) (TokenIdentity, error) {
	var envelope integrationv1.Envelope
	if err := readEnvelope(conn, &envelope); err != nil {
		return TokenIdentity{}, err
	}
	authReq := envelope.GetAuthRequest()
	if authReq == nil {
		return TokenIdentity{}, errors.New("first frame must be auth_request")
	}
	if h.backend == nil {
		return TokenIdentity{}, errors.New("integration gateway backend is not configured")
	}
	return h.backend.ValidateToken(ctx, authReq.GetToken())
}

func (h *WebSocketHandler) readLoop(ctx context.Context, conn *websocket.Conn, identity TokenIdentity, connectionID string, writeMu *sync.Mutex) error {
	if h.heartbeatTimeout > 0 {
		if err := conn.SetReadDeadline(time.Now().Add(h.heartbeatTimeout)); err != nil {
			return nil
		}
		conn.SetPongHandler(func(string) error {
			return conn.SetReadDeadline(time.Now().Add(h.heartbeatTimeout))
		})
	}
	for {
		var envelope integrationv1.Envelope
		if err := readEnvelope(conn, &envelope); err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				return nil
			}
			h.logger.Warn("integration websocket read failed", slog.Any("error", err))
			return nil
		}
		switch payload := envelope.GetPayload().(type) {
		case *integrationv1.Envelope_Ping:
			if h.heartbeatTimeout > 0 {
				_ = conn.SetReadDeadline(time.Now().Add(h.heartbeatTimeout))
			}
			if err := h.safeWrite(conn, writeMu, responseEnvelope(envelope.GetCorrelationId(), &integrationv1.Envelope_Pong{Pong: &integrationv1.Pong{}})); err != nil {
				return nil
			}
		case *integrationv1.Envelope_SubscribeRequest:
			if err := h.handleSubscribe(ctx, conn, writeMu, identity, connectionID, envelope.GetCorrelationId(), payload.SubscribeRequest); err != nil {
				return nil
			}
		case *integrationv1.Envelope_AckRequest:
			if err := h.backend.AckEvent(ctx, identity, payload.AckRequest.GetEventId()); err != nil {
				return h.writeError(conn, writeMu, envelope.GetCorrelationId(), "internal", err.Error())
			}
			if err := h.safeWrite(conn, writeMu, responseEnvelope(envelope.GetCorrelationId(), &integrationv1.Envelope_AckResponse{AckResponse: &integrationv1.AckResponse{EventId: payload.AckRequest.GetEventId()}})); err != nil {
				return nil
			}
		case *integrationv1.Envelope_SendBotMessageRequest:
			if err := h.handleSendBotMessage(ctx, conn, writeMu, identity, envelope.GetCorrelationId(), payload.SendBotMessageRequest); err != nil {
				return nil
			}
		case *integrationv1.Envelope_CreateSessionRequest:
			if err := h.handleCreateSession(ctx, conn, writeMu, identity, envelope.GetCorrelationId(), payload.CreateSessionRequest); err != nil {
				return nil
			}
		case *integrationv1.Envelope_GetSessionStatusRequest:
			if err := h.handleGetSessionStatus(ctx, conn, writeMu, identity, envelope.GetCorrelationId(), payload.GetSessionStatusRequest); err != nil {
				return nil
			}
		case *integrationv1.Envelope_GetBotStatusRequest:
			if err := h.handleGetBotStatus(ctx, conn, writeMu, identity, envelope.GetCorrelationId(), payload.GetBotStatusRequest); err != nil {
				return nil
			}
		case *integrationv1.Envelope_RequestActionRequest:
			if err := h.handleRequestAction(ctx, conn, writeMu, identity, envelope.GetCorrelationId(), payload.RequestActionRequest); err != nil {
				return nil
			}
		default:
			if err := h.writeError(conn, writeMu, envelope.GetCorrelationId(), "unsupported", "unsupported integration frame"); err != nil {
				return nil
			}
		}
	}
}

func (h *WebSocketHandler) handleSubscribe(ctx context.Context, conn *websocket.Conn, writeMu *sync.Mutex, identity TokenIdentity, connectionID string, correlationID string, req *integrationv1.SubscribeRequest) error {
	for _, botID := range req.GetBotIds() {
		if err := h.backend.AuthorizeBot(ctx, identity, botID, "subscribe"); err != nil {
			return h.writeError(conn, writeMu, correlationID, "permission_denied", err.Error())
		}
	}
	for _, botGroupID := range req.GetBotGroupIds() {
		if err := h.backend.AuthorizeBotGroup(ctx, identity, botGroupID); err != nil {
			return h.writeError(conn, writeMu, correlationID, "permission_denied", err.Error())
		}
	}
	events := req.GetEventTypes()
	if len(events) == 0 {
		events = identity.Token.AllowedEventTypes
	}
	for _, eventType := range events {
		if err := h.backend.AuthorizeEvent(ctx, identity, eventType); err != nil {
			return h.writeError(conn, writeMu, correlationID, "permission_denied", err.Error())
		}
	}
	replay, err := h.hub.Subscribe(connectionID, req.GetBotIds(), req.GetBotGroupIds(), events)
	if err != nil {
		return h.writeError(conn, writeMu, correlationID, "internal", err.Error())
	}
	if err := h.safeWrite(conn, writeMu, responseEnvelope(correlationID, &integrationv1.Envelope_SubscribeResponse{SubscribeResponse: &integrationv1.SubscribeResponse{EventTypes: events}})); err != nil {
		return err
	}
	for _, event := range replay {
		if err := h.safeWrite(conn, writeMu, responseEnvelope("", &integrationv1.Envelope_Event{Event: event})); err != nil {
			return err
		}
	}
	return nil
}

func (h *WebSocketHandler) handleSendBotMessage(ctx context.Context, conn *websocket.Conn, writeMu *sync.Mutex, identity TokenIdentity, correlationID string, req *integrationv1.SendBotMessageRequest) error {
	botID := strings.TrimSpace(req.GetBotId())
	if botID == "" {
		return h.writeError(conn, writeMu, correlationID, "invalid_argument", "bot_id is required")
	}
	if strings.TrimSpace(req.GetText()) == "" {
		return h.writeError(conn, writeMu, correlationID, "invalid_argument", "text is required")
	}
	result, err := h.backend.SendBotMessage(ctx, identity, SendBotMessageGatewayRequest{
		BotID:     botID,
		SessionID: strings.TrimSpace(req.GetSessionId()),
		Text:      req.GetText(),
		Metadata:  req.GetMetadata(),
	})
	if err != nil {
		return h.writeError(conn, writeMu, correlationID, "permission_denied", err.Error())
	}
	return h.safeWrite(conn, writeMu, responseEnvelope(correlationID, &integrationv1.Envelope_SendBotMessageResponse{SendBotMessageResponse: &integrationv1.SendBotMessageResponse{
		MessageId: result.MessageID,
		SessionId: result.SessionID,
	}}))
}

func (h *WebSocketHandler) handleCreateSession(ctx context.Context, conn *websocket.Conn, writeMu *sync.Mutex, identity TokenIdentity, correlationID string, req *integrationv1.CreateSessionRequest) error {
	botID := strings.TrimSpace(req.GetBotId())
	if botID == "" {
		return h.writeError(conn, writeMu, correlationID, "invalid_argument", "bot_id is required")
	}
	if err := h.backend.AuthorizeBot(ctx, identity, botID, "create_session"); err != nil {
		return h.writeError(conn, writeMu, correlationID, "permission_denied", err.Error())
	}
	session, err := h.backend.CreateSession(ctx, identity, botID, req.GetExternalSessionId(), req.GetMetadata())
	if err != nil {
		return h.writeError(conn, writeMu, correlationID, "internal", err.Error())
	}
	return h.safeWrite(conn, writeMu, responseEnvelope(correlationID, &integrationv1.Envelope_CreateSessionResponse{CreateSessionResponse: &integrationv1.CreateSessionResponse{
		SessionId: session.ID,
		BotId:     botID,
	}}))
}

func (h *WebSocketHandler) handleGetSessionStatus(ctx context.Context, conn *websocket.Conn, writeMu *sync.Mutex, identity TokenIdentity, correlationID string, req *integrationv1.GetSessionStatusRequest) error {
	sessionID := strings.TrimSpace(req.GetSessionId())
	if sessionID == "" {
		return h.writeError(conn, writeMu, correlationID, "invalid_argument", "session_id is required")
	}
	status, err := h.backend.GetSessionStatus(ctx, identity, sessionID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return h.writeError(conn, writeMu, correlationID, "not_found", err.Error())
		}
		return h.writeError(conn, writeMu, correlationID, "permission_denied", err.Error())
	}
	return h.safeWrite(conn, writeMu, responseEnvelope(correlationID, &integrationv1.Envelope_GetSessionStatusResponse{GetSessionStatusResponse: &integrationv1.GetSessionStatusResponse{
		SessionId: sessionID,
		BotId:     status.BotID,
		Status:    status.Status,
	}}))
}

func (h *WebSocketHandler) handleGetBotStatus(ctx context.Context, conn *websocket.Conn, writeMu *sync.Mutex, identity TokenIdentity, correlationID string, req *integrationv1.GetBotStatusRequest) error {
	botID := strings.TrimSpace(req.GetBotId())
	if botID == "" {
		return h.writeError(conn, writeMu, correlationID, "invalid_argument", "bot_id is required")
	}
	if err := h.backend.AuthorizeBot(ctx, identity, botID, "get_bot_status"); err != nil {
		return h.writeError(conn, writeMu, correlationID, "permission_denied", err.Error())
	}
	return h.safeWrite(conn, writeMu, responseEnvelope(correlationID, &integrationv1.Envelope_GetBotStatusResponse{GetBotStatusResponse: &integrationv1.GetBotStatusResponse{
		BotId:  botID,
		Status: "available",
	}}))
}

func (h *WebSocketHandler) handleRequestAction(ctx context.Context, conn *websocket.Conn, writeMu *sync.Mutex, identity TokenIdentity, correlationID string, req *integrationv1.RequestActionRequest) error {
	botID := strings.TrimSpace(req.GetBotId())
	if botID == "" {
		return h.writeError(conn, writeMu, correlationID, "invalid_argument", "bot_id is required")
	}
	actionType := strings.TrimSpace(req.GetActionType())
	if actionType == "" {
		return h.writeError(conn, writeMu, correlationID, "invalid_argument", "action_type is required")
	}
	result, err := h.backend.RequestAction(ctx, identity, RequestActionGatewayRequest{
		BotID:       botID,
		ActionType:  actionType,
		PayloadJSON: req.GetPayloadJson(),
		Metadata:    req.GetMetadata(),
	})
	if err != nil {
		return h.writeError(conn, writeMu, correlationID, "permission_denied", err.Error())
	}
	return h.safeWrite(conn, writeMu, responseEnvelope(correlationID, &integrationv1.Envelope_RequestActionResponse{RequestActionResponse: &integrationv1.RequestActionResponse{
		ActionId:   result.ActionID,
		BotId:      result.BotID,
		ActionType: result.ActionType,
		Status:     result.Status,
	}}))
}

func (h *WebSocketHandler) writeError(conn *websocket.Conn, writeMu *sync.Mutex, correlationID, code, message string) error {
	return h.safeWrite(conn, writeMu, responseEnvelope(correlationID, &integrationv1.Envelope_Error{Error: &integrationv1.Error{
		Code:    strings.TrimSpace(code),
		Message: strings.TrimSpace(message),
	}}))
}

func (h *WebSocketHandler) safeWrite(conn *websocket.Conn, writeMu *sync.Mutex, envelope *integrationv1.Envelope) error {
	if writeMu != nil {
		writeMu.Lock()
		defer writeMu.Unlock()
	}
	return h.writeEnvelope(conn, envelope)
}

func (*WebSocketHandler) writeEnvelope(conn *websocket.Conn, envelope *integrationv1.Envelope) error {
	payload, err := marshalEnvelope(envelope)
	if err != nil {
		return err
	}
	if err := conn.SetWriteDeadline(time.Now().Add(wsWriteTimeout)); err != nil {
		return err
	}
	return conn.WriteMessage(websocket.TextMessage, payload)
}
