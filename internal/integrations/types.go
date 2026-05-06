package integrations

import (
	"context"
	"time"
)

const (
	ScopeGlobal   = "global"
	ScopeBot      = "bot"
	ScopeBotGroup = "bot_group"
)

type APIToken struct {
	ID                 string     `json:"id"`
	Name               string     `json:"name"`
	ScopeType          string     `json:"scope_type"`
	ScopeBotID         string     `json:"scope_bot_id,omitempty"`
	ScopeBotGroupID    string     `json:"scope_bot_group_id,omitempty"`
	AllowedEventTypes  []string   `json:"allowed_event_types"`
	AllowedActionTypes []string   `json:"allowed_action_types"`
	ExpiresAt          *time.Time `json:"expires_at,omitempty"`
	DisabledAt         *time.Time `json:"disabled_at,omitempty"`
	LastUsedAt         *time.Time `json:"last_used_at,omitempty"`
	CreatedByUserID    string     `json:"created_by_user_id,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

type CreateAPITokenRequest struct {
	Name               string     `json:"name"`
	ScopeType          string     `json:"scope_type"`
	ScopeBotID         string     `json:"scope_bot_id,omitempty"`
	ScopeBotGroupID    string     `json:"scope_bot_group_id,omitempty"`
	AllowedEventTypes  []string   `json:"allowed_event_types,omitempty"`
	AllowedActionTypes []string   `json:"allowed_action_types,omitempty"`
	ExpiresAt          *time.Time `json:"expires_at,omitempty"`
}

type CreateAPITokenResult struct {
	Token    APIToken `json:"token"`
	RawToken string   `json:"raw_token"`
}

type TokenIdentity struct {
	Token APIToken
}

type GatewayBackend interface {
	ValidateToken(ctx context.Context, rawToken string) (TokenIdentity, error)
	AuthorizeBot(ctx context.Context, identity TokenIdentity, botID string, action string) error
	AuthorizeBotGroup(ctx context.Context, identity TokenIdentity, botGroupID string) error
	AuthorizeEvent(ctx context.Context, identity TokenIdentity, eventType string) error
	AckEvent(ctx context.Context, identity TokenIdentity, eventID string) error
	CreateSession(ctx context.Context, identity TokenIdentity, botID string, externalSessionID string, metadata map[string]string) (IntegrationSession, error)
	GetSessionStatus(ctx context.Context, identity TokenIdentity, sessionID string) (SessionStatus, error)
	SendBotMessage(ctx context.Context, identity TokenIdentity, req SendBotMessageGatewayRequest) (SendBotMessageResult, error)
	RequestAction(ctx context.Context, identity TokenIdentity, req RequestActionGatewayRequest) (RequestActionResult, error)
}

type SessionStatus struct {
	SessionID string
	BotID     string
	Status    string
}

type SendBotMessageGatewayRequest struct {
	BotID     string
	SessionID string
	Text      string
	Metadata  map[string]string
}

type SendBotMessageResult struct {
	MessageID string
	SessionID string
}

type RequestActionGatewayRequest struct {
	BotID       string
	ActionType  string
	PayloadJSON string
	Metadata    map[string]string
}

type RequestActionResult struct {
	ActionID   string
	BotID      string
	ActionType string
	Status     string
}
