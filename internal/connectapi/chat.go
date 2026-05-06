package connectapi

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/memohai/memoh/internal/channel"
	privatev1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1"
	"github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1/privatev1connect"
	"github.com/memohai/memoh/internal/conversation"
)

type chatStreamer interface {
	StreamChat(ctx context.Context, req conversation.ChatRequest) (<-chan conversation.StreamChunk, <-chan error)
}

type ChatService struct {
	runner chatStreamer
}

func NewChatService(runner chatStreamer) *ChatService {
	return &ChatService{runner: runner}
}

func NewChatHandler(service *ChatService) Handler {
	path, handler := privatev1connect.NewChatServiceHandler(service)
	return NewHandler(path, handler)
}

func (s *ChatService) StreamChat(ctx context.Context, req *connect.Request[privatev1.StreamChatRequest], stream *connect.ServerStream[privatev1.StreamChatResponse]) error {
	if s.runner == nil {
		return connect.NewError(connect.CodeInternal, errors.New("chat runner is not configured"))
	}
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return connect.NewError(connect.CodeUnauthenticated, err)
	}
	botID := strings.TrimSpace(req.Msg.GetBotId())
	if botID == "" {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("bot_id is required"))
	}
	text := strings.TrimSpace(req.Msg.GetMessage())
	attachments := streamChatAttachments(req.Msg.GetAttachmentIds())
	if text == "" && len(attachments) == 0 {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("message or attachment_ids is required"))
	}

	token := strings.TrimSpace(req.Header().Get("Authorization"))
	chunks, errs := s.runner.StreamChat(ctx, conversation.ChatRequest{
		BotID:                   botID,
		ChatID:                  botID,
		SessionID:               strings.TrimSpace(req.Msg.GetSessionId()),
		Token:                   token,
		UserID:                  userID,
		SourceChannelIdentityID: userID,
		ConversationType:        channel.ConversationTypePrivate,
		Query:                   text,
		CurrentChannel:          channel.ChannelTypeLocal.String(),
		Channels:                []string{channel.ChannelTypeLocal.String()},
		Attachments:             attachments,
	})

	for chunks != nil || errs != nil {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case chunk, ok := <-chunks:
			if !ok {
				chunks = nil
				continue
			}
			msg, err := streamChatResponseFromChunk(chunk)
			if err != nil {
				return connect.NewError(connect.CodeInternal, err)
			}
			if err := stream.Send(msg); err != nil {
				return err
			}
		case err, ok := <-errs:
			if !ok {
				errs = nil
				continue
			}
			if err != nil {
				return connect.NewError(connect.CodeInternal, err)
			}
		}
	}
	return nil
}

func streamChatAttachments(ids []string) []conversation.ChatAttachment {
	out := make([]conversation.ChatAttachment, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		out = append(out, conversation.ChatAttachment{ContentHash: id})
	}
	return out
}

func streamChatResponseFromChunk(chunk conversation.StreamChunk) (*privatev1.StreamChatResponse, error) {
	payload, err := structFromJSON(chunk)
	if err != nil {
		return nil, err
	}
	fields := payload.AsMap()
	return &privatev1.StreamChatResponse{
		Id:        stringValue(fields, "id"),
		Type:      stringValue(fields, "type"),
		Text:      firstStringValue(fields, "text", "delta", "error", "message"),
		Payload:   payload,
		CreatedAt: timestamppb.New(time.Now()),
	}, nil
}

func structFromJSON(data []byte) (*structpb.Struct, error) {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	return structpb.NewStruct(raw)
}

func stringValue(fields map[string]any, key string) string {
	value, _ := fields[key].(string)
	return value
}

func firstStringValue(fields map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := stringValue(fields, key); strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
