package conversation

import (
	"context"
	"encoding/json"

	"github.com/memohai/memoh/internal/schedule"
)

type Runner interface {
	Chat(ctx context.Context, req ChatRequest) (ChatResponse, error)
	StreamChat(ctx context.Context, req ChatRequest) (<-chan StreamChunk, <-chan error)
	TriggerSchedule(ctx context.Context, botID string, payload schedule.TriggerPayload, token string) (schedule.TriggerResult, error)
}

type WSStreamEvent = json.RawMessage

type ToolApprovalResponseInput struct {
	BotID                  string
	SessionID              string
	ActorChannelIdentityID string
	ApprovalID             string
	ExplicitID             string
	ReplyExternalMessageID string
	Decision               string
	Reason                 string
	ChatToken              string
}
