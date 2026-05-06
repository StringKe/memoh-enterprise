package integrations

import "time"

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
