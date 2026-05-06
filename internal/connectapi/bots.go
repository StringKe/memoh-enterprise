package connectapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/memohai/memoh/internal/bots"
	privatev1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1"
	"github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1/privatev1connect"
	dbsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
	"github.com/memohai/memoh/internal/eventbus"
	"github.com/memohai/memoh/internal/iam/rbac"
	messagepkg "github.com/memohai/memoh/internal/message"
)

type BotService struct {
	bots               *bots.Service
	permissions        botPermissionChecker
	messages           messagepkg.Service
	events             botEventPublisher
	workerConsumerName string
	now                func() time.Time
}

type botPermissionChecker interface {
	HasBotPermission(ctx context.Context, userID, botID string, permission rbac.PermissionKey) (bool, error)
}

type botEventPublisher interface {
	Publish(ctx context.Context, event eventbus.Event, consumers []string) (dbsqlc.EventOutbox, error)
}

const (
	defaultWorkerConsumerName    = "memoh-worker"
	botSessionCompactionTopic    = "worker.compaction.run"
	botSessionCompactionPayload  = "memoh.compaction.TriggerConfig"
	botSessionCompactionConsumer = defaultWorkerConsumerName
)

func NewBotService(bots *bots.Service, messages *messagepkg.DBService, events *eventbus.Producer) *BotService {
	return &BotService{
		bots:               bots,
		permissions:        bots,
		messages:           messages,
		events:             events,
		workerConsumerName: defaultWorkerConsumerName,
		now:                time.Now,
	}
}

func (s *BotService) SetWorkerConsumerName(name string) {
	name = strings.TrimSpace(name)
	if name != "" {
		s.workerConsumerName = name
	}
}

func (s *BotService) SetNow(now func() time.Time) {
	if now != nil {
		s.now = now
	}
}

func NewBotHandler(service *BotService) Handler {
	path, handler := privatev1connect.NewBotServiceHandler(service)
	return NewHandler(path, handler)
}

func (s *BotService) CreateBot(ctx context.Context, req *connect.Request[privatev1.CreateBotRequest]) (*connect.Response[privatev1.CreateBotResponse], error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	bot, err := s.bots.Create(ctx, userID, bots.CreateBotRequest{
		DisplayName: req.Msg.GetDisplayName(),
		GroupID:     req.Msg.GetGroupId(),
		AvatarURL:   req.Msg.GetAvatarUrl(),
		Timezone:    req.Msg.Timezone,
		IsActive:    req.Msg.IsActive,
		AclPreset:   req.Msg.GetAclPreset(),
		Metadata:    structToMap(req.Msg.GetMetadata()),
	})
	if err != nil {
		return nil, botConnectError(err)
	}
	return connect.NewResponse(&privatev1.CreateBotResponse{Bot: botToProto(bot)}), nil
}

func (s *BotService) GetBot(ctx context.Context, req *connect.Request[privatev1.GetBotRequest]) (*connect.Response[privatev1.GetBotResponse], error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	bot, err := s.bots.AuthorizeAccess(ctx, userID, req.Msg.GetId(), false)
	if err != nil {
		return nil, botConnectError(err)
	}
	return connect.NewResponse(&privatev1.GetBotResponse{Bot: botToProto(bot)}), nil
}

func (s *BotService) ListBots(ctx context.Context, _ *connect.Request[privatev1.ListBotsRequest]) (*connect.Response[privatev1.ListBotsResponse], error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	items, err := s.bots.ListAccessible(ctx, userID)
	if err != nil {
		return nil, botConnectError(err)
	}
	out := make([]*privatev1.Bot, 0, len(items))
	for _, item := range items {
		out = append(out, botToProto(item))
	}
	return connect.NewResponse(&privatev1.ListBotsResponse{
		Bots: out,
		Page: &privatev1.PageResponse{},
	}), nil
}

func (s *BotService) UpdateBot(ctx context.Context, req *connect.Request[privatev1.UpdateBotRequest]) (*connect.Response[privatev1.UpdateBotResponse], error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	if err := s.requireBotPermission(ctx, userID, req.Msg.GetId(), rbac.PermissionBotUpdate); err != nil {
		return nil, botConnectError(err)
	}
	bot, err := s.bots.Update(ctx, req.Msg.GetId(), bots.UpdateBotRequest{
		DisplayName: req.Msg.DisplayName,
		GroupID:     req.Msg.GroupId,
		AvatarURL:   req.Msg.AvatarUrl,
		Timezone:    req.Msg.Timezone,
		IsActive:    req.Msg.IsActive,
		Metadata:    structToMap(req.Msg.GetMetadata()),
	})
	if err != nil {
		return nil, botConnectError(err)
	}
	return connect.NewResponse(&privatev1.UpdateBotResponse{Bot: botToProto(bot)}), nil
}

func (s *BotService) DeleteBot(ctx context.Context, req *connect.Request[privatev1.DeleteBotRequest]) (*connect.Response[privatev1.DeleteBotResponse], error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	if err := s.requireBotPermission(ctx, userID, req.Msg.GetId(), rbac.PermissionBotDelete); err != nil {
		return nil, botConnectError(err)
	}
	if err := s.bots.Delete(ctx, req.Msg.GetId()); err != nil {
		return nil, botConnectError(err)
	}
	return connect.NewResponse(&privatev1.DeleteBotResponse{}), nil
}

func (s *BotService) ListBotChecks(ctx context.Context, req *connect.Request[privatev1.ListBotChecksRequest]) (*connect.Response[privatev1.ListBotChecksResponse], error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	if err := s.requireBotPermission(ctx, userID, req.Msg.GetBotId(), rbac.PermissionBotRead); err != nil {
		return nil, botConnectError(err)
	}
	checks, err := s.bots.ListChecks(ctx, req.Msg.GetBotId())
	if err != nil {
		return nil, botConnectError(err)
	}
	out := make([]*privatev1.BotCheck, 0, len(checks))
	for _, check := range checks {
		out = append(out, botCheckToProto(check))
	}
	return connect.NewResponse(&privatev1.ListBotChecksResponse{Checks: out}), nil
}

func (s *BotService) ReadBotSessionHistory(ctx context.Context, req *connect.Request[privatev1.ReadBotSessionHistoryRequest]) (*connect.Response[privatev1.ReadBotSessionHistoryResponse], error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	if err := s.requireBotPermission(ctx, userID, req.Msg.GetBotId(), rbac.PermissionBotRead); err != nil {
		return nil, botConnectError(err)
	}
	messages, err := s.listSessionMessages(ctx, req.Msg.GetBotId(), req.Msg.GetSessionId(), req.Msg.GetLimit(), req.Msg.GetBeforeMessageId())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&privatev1.ReadBotSessionHistoryResponse{Messages: botSessionMessagesToProto(messages)}), nil
}

func (s *BotService) CompactBotSession(ctx context.Context, req *connect.Request[privatev1.CompactBotSessionRequest]) (*connect.Response[privatev1.CompactBotSessionResponse], error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	if strings.TrimSpace(req.Msg.GetSessionId()) == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("session_id is required"))
	}
	if err := s.requireBotPermission(ctx, userID, req.Msg.GetBotId(), rbac.PermissionBotUpdate); err != nil {
		return nil, botConnectError(err)
	}
	if s.events == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("eventbus producer is not configured"))
	}
	botID := strings.TrimSpace(req.Msg.GetBotId())
	sessionID := strings.TrimSpace(req.Msg.GetSessionId())
	payload, err := json.Marshal(struct {
		BotID     string
		SessionID string
	}{
		BotID:     botID,
		SessionID: sessionID,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if _, err := s.events.Publish(ctx, eventbus.Event{
		Topic:          botSessionCompactionTopic,
		PayloadType:    botSessionCompactionPayload,
		Payload:        payload,
		PayloadJSON:    payload,
		IdempotencyKey: fmt.Sprintf("bot-session-compact:%s", uuid.NewString()),
		AggregateType:  "bot_session",
		PartitionKey:   sessionID,
	}, []string{s.workerConsumer()}); err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&privatev1.CompactBotSessionResponse{
		SessionId:   sessionID,
		Summary:     "queued",
		CompactedAt: timeToProto(s.now().UTC()),
	}), nil
}

func (s *BotService) ListBotSessionMessages(ctx context.Context, req *connect.Request[privatev1.ListBotSessionMessagesRequest]) (*connect.Response[privatev1.ListBotSessionMessagesResponse], error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	if err := s.requireBotPermission(ctx, userID, req.Msg.GetBotId(), rbac.PermissionBotRead); err != nil {
		return nil, botConnectError(err)
	}
	pageSize := req.Msg.GetPage().GetPageSize()
	if pageSize <= 0 || pageSize > 200 {
		pageSize = 100
	}
	offset, err := parsePageOffset(req.Msg.GetPage().GetPageToken())
	if err != nil {
		return nil, err
	}
	messages, err := s.listSessionMessages(ctx, req.Msg.GetBotId(), req.Msg.GetSessionId(), 0, "")
	if err != nil {
		return nil, err
	}
	allMessages := messages
	start := int(offset)
	end := start + int(pageSize)
	if start > len(allMessages) {
		messages = nil
	} else {
		if end > len(allMessages) {
			end = len(allMessages)
		}
		messages = allMessages[start:end]
	}
	nextPageToken := ""
	if end < len(allMessages) {
		nextPageToken = strconv.Itoa(end)
	}
	return connect.NewResponse(&privatev1.ListBotSessionMessagesResponse{
		Messages: botSessionMessagesToProto(messages),
		Page:     &privatev1.PageResponse{NextPageToken: nextPageToken},
	}), nil
}

func (s *BotService) AssignBotGroup(ctx context.Context, req *connect.Request[privatev1.AssignBotGroupRequest]) (*connect.Response[privatev1.AssignBotGroupResponse], error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	bot, err := s.bots.AssignGroup(ctx, userID, req.Msg.GetBotId(), req.Msg.GetGroupId())
	if err != nil {
		return nil, botConnectError(err)
	}
	return connect.NewResponse(&privatev1.AssignBotGroupResponse{Bot: botToProto(bot)}), nil
}

func (s *BotService) ClearBotGroup(ctx context.Context, req *connect.Request[privatev1.ClearBotGroupRequest]) (*connect.Response[privatev1.ClearBotGroupResponse], error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	bot, err := s.bots.ClearGroup(ctx, userID, req.Msg.GetBotId())
	if err != nil {
		return nil, botConnectError(err)
	}
	return connect.NewResponse(&privatev1.ClearBotGroupResponse{Bot: botToProto(bot)}), nil
}

func (s *BotService) requireBotPermission(ctx context.Context, userID, botID string, permission rbac.PermissionKey) error {
	checker := s.permissions
	if checker == nil {
		checker = s.bots
	}
	if checker == nil {
		return errors.New("bot permission checker is not configured")
	}
	allowed, err := checker.HasBotPermission(ctx, userID, botID, permission)
	if err != nil {
		return err
	}
	if !allowed {
		return bots.ErrBotAccessDenied
	}
	return nil
}

func (s *BotService) workerConsumer() string {
	if s != nil && strings.TrimSpace(s.workerConsumerName) != "" {
		return strings.TrimSpace(s.workerConsumerName)
	}
	return botSessionCompactionConsumer
}

func botToProto(bot bots.Bot) *privatev1.Bot {
	return &privatev1.Bot{
		Id:                   bot.ID,
		OwnerUserId:          bot.OwnerUserID,
		GroupId:              bot.GroupID,
		DisplayName:          bot.DisplayName,
		AvatarUrl:            bot.AvatarURL,
		Timezone:             bot.Timezone,
		IsActive:             bot.IsActive,
		Status:               bot.Status,
		CheckState:           bot.CheckState,
		CheckIssueCount:      bot.CheckIssueCount,
		SettingsOverrideMask: &privatev1.SettingsOverrideMask{Fields: bot.SettingsOverrideMask},
		Metadata:             mapToStruct(bot.Metadata),
		Audit: &privatev1.AuditFields{
			CreatedAt: timeToProto(bot.CreatedAt),
			UpdatedAt: timeToProto(bot.UpdatedAt),
		},
	}
}

func botCheckToProto(check bots.BotCheck) *privatev1.BotCheck {
	return &privatev1.BotCheck{
		Id:       check.ID,
		Type:     check.Type,
		TitleKey: check.TitleKey,
		Subtitle: check.Subtitle,
		Status:   check.Status,
		Summary:  check.Summary,
		Detail:   check.Detail,
		Metadata: mapToStruct(check.Metadata),
	}
}

func (s *BotService) listSessionMessages(ctx context.Context, botID, sessionID string, limit int32, beforeMessageID string) ([]messagepkg.Message, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("session_id is required"))
	}
	if s.messages == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("message service not configured"))
	}
	if limit < 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("limit must be non-negative"))
	}
	if limit > 500 {
		limit = 500
	}
	items, err := s.messages.ListBySession(ctx, sessionID)
	if err != nil {
		return nil, connectError(err)
	}
	filtered := make([]messagepkg.Message, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.BotID) == strings.TrimSpace(botID) {
			filtered = append(filtered, item)
		}
	}
	if before := strings.TrimSpace(beforeMessageID); before != "" {
		for i, item := range filtered {
			if item.ID == before {
				filtered = filtered[:i]
				break
			}
		}
	}
	if limit > 0 && len(filtered) > int(limit) {
		filtered = filtered[len(filtered)-int(limit):]
	}
	return filtered, nil
}

func botSessionMessagesToProto(items []messagepkg.Message) []*privatev1.BotSessionMessage {
	out := make([]*privatev1.BotSessionMessage, 0, len(items))
	for _, item := range items {
		out = append(out, botSessionMessageToProto(item))
	}
	return out
}

func botSessionMessageToProto(item messagepkg.Message) *privatev1.BotSessionMessage {
	payload := map[string]any{
		"metadata": item.Metadata,
		"assets":   messageAssetsPayload(item.Assets),
	}
	if content := jsonPayload(item.Content); content != nil {
		payload["content"] = content
	}
	if usage := jsonPayload(item.Usage); usage != nil {
		payload["usage"] = usage
	}
	return &privatev1.BotSessionMessage{
		Id:        item.ID,
		SessionId: item.SessionID,
		BotId:     item.BotID,
		Role:      item.Role,
		Text:      firstNonEmptyString(item.DisplayContent, jsonPayloadText(item.Content)),
		Payload:   mapToStruct(payload),
		CreatedAt: timeToProto(item.CreatedAt),
	}
}

func jsonPayload(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return string(raw)
	}
	return value
}

func jsonPayloadText(raw json.RawMessage) string {
	value := jsonPayload(raw)
	switch typed := value.(type) {
	case string:
		return typed
	case map[string]any:
		if text, _ := typed["text"].(string); text != "" {
			return text
		}
	}
	if value == nil {
		return ""
	}
	data, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(data)
}

func messageAssetsPayload(items []messagepkg.MessageAsset) []any {
	out := make([]any, 0, len(items))
	for _, item := range items {
		out = append(out, map[string]any{
			"content_hash": item.ContentHash,
			"role":         item.Role,
			"ordinal":      item.Ordinal,
			"mime":         item.Mime,
			"size_bytes":   item.SizeBytes,
			"storage_key":  item.StorageKey,
			"name":         item.Name,
			"metadata":     item.Metadata,
		})
	}
	return out
}

func parsePageOffset(token string) (int32, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return 0, nil
	}
	offset, err := strconv.Atoi(token)
	if err != nil || offset < 0 {
		return 0, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid page_token"))
	}
	return int32(offset), nil //nolint:gosec // page offsets are bounded by request page size.
}

func botConnectError(err error) error {
	switch {
	case errors.Is(err, bots.ErrBotNotFound), errors.Is(err, pgx.ErrNoRows):
		return connect.NewError(connect.CodeNotFound, err)
	case errors.Is(err, bots.ErrBotAccessDenied), errors.Is(err, bots.ErrBotGroupNotAllowed):
		return connect.NewError(connect.CodePermissionDenied, err)
	default:
		return connectError(err)
	}
}
