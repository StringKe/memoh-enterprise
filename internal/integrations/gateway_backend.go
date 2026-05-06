package integrations

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

type LocalGatewayBackend struct {
	service *Service
	hub     *Hub
}

func NewLocalGatewayBackend(service *Service, hub *Hub) *LocalGatewayBackend {
	return &LocalGatewayBackend{service: service, hub: hub}
}

func (b *LocalGatewayBackend) ValidateToken(ctx context.Context, rawToken string) (TokenIdentity, error) {
	if b.service == nil {
		return TokenIdentity{}, errors.New("integration service is not configured")
	}
	return b.service.ValidateToken(ctx, rawToken)
}

func (b *LocalGatewayBackend) AuthorizeBot(ctx context.Context, identity TokenIdentity, botID string, action string) error {
	if b.service == nil {
		return errors.New("integration service is not configured")
	}
	return b.service.AuthorizeBot(ctx, identity, botID, action)
}

func (b *LocalGatewayBackend) AuthorizeBotGroup(_ context.Context, identity TokenIdentity, botGroupID string) error {
	if b.service == nil {
		return errors.New("integration service is not configured")
	}
	return b.service.AuthorizeBotGroup(identity, botGroupID)
}

func (b *LocalGatewayBackend) AuthorizeEvent(_ context.Context, identity TokenIdentity, eventType string) error {
	if b.service == nil {
		return errors.New("integration service is not configured")
	}
	return b.service.AuthorizeEvent(identity, eventType)
}

func (b *LocalGatewayBackend) AckEvent(_ context.Context, identity TokenIdentity, eventID string) error {
	if b.hub == nil {
		return errors.New("integration hub is not configured")
	}
	b.hub.Ack(identity.Token.ID, eventID)
	return nil
}

func (b *LocalGatewayBackend) CreateSession(_ context.Context, identity TokenIdentity, botID string, externalSessionID string, metadata map[string]string) (IntegrationSession, error) {
	if b.hub == nil {
		return IntegrationSession{}, errors.New("integration hub is not configured")
	}
	return b.hub.CreateOrBindSession(identity.Token.ID, botID, externalSessionID, metadata), nil
}

func (b *LocalGatewayBackend) GetSessionStatus(ctx context.Context, identity TokenIdentity, sessionID string) (SessionStatus, error) {
	if b.hub == nil {
		return SessionStatus{}, errors.New("integration hub is not configured")
	}
	session, ok := b.hub.Session(sessionID)
	if !ok {
		return SessionStatus{}, errors.New("integration session not found")
	}
	if err := b.AuthorizeBot(ctx, identity, session.BotID, "get_session_status"); err != nil {
		return SessionStatus{}, err
	}
	return SessionStatus{
		SessionID: session.ID,
		BotID:     session.BotID,
		Status:    "active",
	}, nil
}

func (b *LocalGatewayBackend) SendBotMessage(ctx context.Context, identity TokenIdentity, req SendBotMessageGatewayRequest) (SendBotMessageResult, error) {
	if err := b.AuthorizeBot(ctx, identity, req.BotID, "send_message"); err != nil {
		return SendBotMessageResult{}, err
	}
	sessionID := req.SessionID
	if sessionID == "" {
		session, err := b.CreateSession(ctx, identity, req.BotID, "", nil)
		if err != nil {
			return SendBotMessageResult{}, err
		}
		sessionID = session.ID
	}
	return SendBotMessageResult{
		MessageID: uuid.NewString(),
		SessionID: sessionID,
	}, nil
}

func (b *LocalGatewayBackend) RequestAction(ctx context.Context, identity TokenIdentity, req RequestActionGatewayRequest) (RequestActionResult, error) {
	if err := b.AuthorizeBot(ctx, identity, req.BotID, req.ActionType); err != nil {
		return RequestActionResult{}, err
	}
	return RequestActionResult{
		ActionID:   uuid.NewString(),
		BotID:      req.BotID,
		ActionType: req.ActionType,
		Status:     "accepted",
	}, nil
}
