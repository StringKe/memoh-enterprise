package connectapi

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/db"
	dbsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
	memprovider "github.com/memohai/memoh/internal/memory/adapters"
	messagepkg "github.com/memohai/memoh/internal/message"
	"github.com/memohai/memoh/internal/providers"
	"github.com/memohai/memoh/internal/toolapproval"
)

type RunnerSupportBackendDeps struct {
	Queries       RunnerSupportBackendQueries
	Messages      messagepkg.Service
	Memory        *memprovider.Registry
	Providers     *providers.Service
	ToolApprovals *toolapproval.Service
	ChannelSender interface {
		Send(ctx context.Context, botID string, channelType channel.ChannelType, req channel.SendRequest) error
	}
}

type RunnerSupportBackendQueries interface {
	GetBotByID(ctx context.Context, id pgtype.UUID) (dbsqlc.GetBotByIDRow, error)
	GetModelByID(ctx context.Context, id pgtype.UUID) (dbsqlc.Model, error)
	GetProviderByID(ctx context.Context, id pgtype.UUID) (dbsqlc.Provider, error)
	GetProviderByName(ctx context.Context, name string) (dbsqlc.Provider, error)
	CreateSessionEvent(ctx context.Context, arg dbsqlc.CreateSessionEventParams) (pgtype.UUID, error)
}

type RunnerSupportBackend struct {
	queries       RunnerSupportBackendQueries
	messages      messagepkg.Service
	memory        *memprovider.Registry
	providers     *providers.Service
	toolApprovals *toolapproval.Service
	channelSender channelSender
}

type channelSender interface {
	Send(ctx context.Context, botID string, channelType channel.ChannelType, req channel.SendRequest) error
}

func NewRunnerSupportBackend(deps RunnerSupportBackendDeps) *RunnerSupportBackend {
	return &RunnerSupportBackend{
		queries:       deps.Queries,
		messages:      deps.Messages,
		memory:        deps.Memory,
		providers:     deps.Providers,
		toolApprovals: deps.ToolApprovals,
		channelSender: deps.ChannelSender,
	}
}

func (b *RunnerSupportBackend) ResolveRunContext(ctx context.Context, req ResolveRunContextRequest) (ResolveRunContextResponse, error) {
	if b == nil || b.queries == nil {
		return ResolveRunContextResponse{}, ErrRunnerSupportDependencyMissing
	}
	botID, err := db.ParseUUID(req.Lease.BotID)
	if err != nil {
		return ResolveRunContextResponse{}, err
	}
	bot, err := b.queries.GetBotByID(ctx, botID)
	if err != nil {
		return ResolveRunContextResponse{}, err
	}
	modelContext := map[string]any{}
	if bot.ChatModelID.Valid {
		model, err := b.queries.GetModelByID(ctx, bot.ChatModelID)
		if err != nil {
			return ResolveRunContextResponse{}, err
		}
		modelConfig := jsonObject(model.Config)
		modelContext = map[string]any{
			"id":                uuidString(model.ID),
			"model_id":          model.ModelID,
			"name":              pgText(model.Name),
			"provider_id":       uuidString(model.ProviderID),
			"type":              model.Type,
			"compatibilities":   modelConfig["compatibilities"],
			"reasoning_enabled": bot.ReasoningEnabled,
			"reasoning_effort":  bot.ReasoningEffort,
			"prompt_cache_ttl":  modelConfig["prompt_cache_ttl"],
			"config":            modelConfig,
		}
	}
	return ResolveRunContextResponse{
		Context: map[string]any{
			"bot": map[string]any{
				"id":                  bot.ID.String(),
				"owner_user_id":       bot.OwnerUserID.String(),
				"group_id":            bot.GroupID.String(),
				"display_name":        pgText(bot.DisplayName),
				"timezone":            pgText(bot.Timezone),
				"status":              bot.Status,
				"language":            bot.Language,
				"reasoning_enabled":   bot.ReasoningEnabled,
				"reasoning_effort":    bot.ReasoningEffort,
				"chat_model_id":       bot.ChatModelID.String(),
				"memory_provider_id":  bot.MemoryProviderID.String(),
				"search_provider_id":  bot.SearchProviderID.String(),
				"heartbeat_enabled":   bot.HeartbeatEnabled,
				"heartbeat_interval":  bot.HeartbeatInterval,
				"compaction_enabled":  bot.CompactionEnabled,
				"compaction_model_id": bot.CompactionModelID.String(),
				"metadata":            jsonObject(bot.Metadata),
				"session_type":        "chat",
			},
			"model": modelContext,
		},
	}, nil
}

func (b *RunnerSupportBackend) ReadSessionHistory(ctx context.Context, req ReadSessionHistoryRequest) (ReadSessionHistoryResponse, error) {
	if b == nil || b.messages == nil {
		return ReadSessionHistoryResponse{}, ErrRunnerSupportDependencyMissing
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 50
	}
	rows, err := b.messages.ListLatestBySession(ctx, req.Lease.SessionID, limit)
	if err != nil {
		return ReadSessionHistoryResponse{}, err
	}
	out := make([]SessionMessage, 0, len(rows))
	for _, row := range rows {
		out = append(out, SessionMessage{
			Role:      row.Role,
			Content:   messageText(row.Content),
			Metadata:  marshalMap(row.Metadata),
			CreatedAt: row.CreatedAt,
		})
	}
	return ReadSessionHistoryResponse{Messages: out}, nil
}

func (b *RunnerSupportBackend) AppendRunEvent(ctx context.Context, req AppendRunEventRequest) error {
	if b == nil || b.queries == nil {
		return ErrRunnerSupportDependencyMissing
	}
	botID, err := db.ParseUUID(req.Lease.BotID)
	if err != nil {
		return err
	}
	sessionID, err := db.ParseUUID(req.Lease.SessionID)
	if err != nil {
		return err
	}
	_, err = b.queries.CreateSessionEvent(ctx, dbsqlc.CreateSessionEventParams{
		BotID:             botID,
		SessionID:         sessionID,
		EventKind:         "agent_run." + strings.TrimSpace(req.EventType),
		EventData:         req.Payload,
		ExternalMessageID: pgtype.Text{String: strings.TrimSpace(req.Idempotency), Valid: strings.TrimSpace(req.Idempotency) != ""},
		ReceivedAtMs:      time.Now().UTC().UnixMilli(),
	})
	return err
}

func (b *RunnerSupportBackend) AppendSessionMessage(ctx context.Context, req AppendSessionMessageRequest) error {
	if b == nil || b.messages == nil {
		return ErrRunnerSupportDependencyMissing
	}
	content := req.Message.Content
	if strings.TrimSpace(content) == "" {
		content = "{}"
	}
	rawContent, err := json.Marshal(content)
	if err != nil {
		return err
	}
	_, err = b.messages.Persist(ctx, messagepkg.PersistInput{
		BotID:        req.Lease.BotID,
		SessionID:    req.Lease.SessionID,
		SenderUserID: req.Lease.UserID,
		Role:         strings.TrimSpace(req.Message.Role),
		Content:      rawContent,
		Metadata:     jsonObject(req.Message.Metadata),
		DisplayText:  strings.TrimSpace(req.Message.Content),
	})
	return err
}

func (*RunnerSupportBackend) ResolveOutboundTarget(_ context.Context, req ResolveOutboundTargetRequest) (ResolveOutboundTargetResponse, error) {
	channelType := strings.TrimSpace(req.ChannelType)
	conversationID := strings.TrimSpace(req.ConversationID)
	return ResolveOutboundTargetResponse{
		Target: map[string]any{
			"channel_type":    channelType,
			"conversation_id": conversationID,
			"target":          conversationID,
		},
	}, nil
}

func (b *RunnerSupportBackend) RequestOutboundDispatch(ctx context.Context, req RequestOutboundDispatchRequest) error {
	if b == nil || b.channelSender == nil {
		return ErrRunnerSupportDependencyMissing
	}
	channelType := strings.TrimSpace(req.ChannelType)
	if channelType == "" {
		channelType, _ = req.Target["channel_type"].(string)
	}
	target := strings.TrimSpace(req.ConversationID)
	if target == "" {
		target, _ = req.Target["target"].(string)
	}
	if strings.TrimSpace(channelType) == "" || strings.TrimSpace(target) == "" {
		return errors.New("channel_type and target are required")
	}
	text := strings.TrimSpace(req.Text)
	if text == "" {
		text = strings.TrimSpace(string(req.Payload))
	}
	return b.channelSender.Send(ctx, req.Lease.BotID, channel.ChannelType(channelType), channel.SendRequest{
		Target:  target,
		Message: channel.Message{Text: text},
	})
}

func (b *RunnerSupportBackend) ReadMemory(ctx context.Context, req ReadMemoryRequest) (ReadMemoryResponse, error) {
	provider, err := b.memoryProvider(ctx, req.Lease.BotID)
	if err != nil || provider == nil {
		return ReadMemoryResponse{}, err
	}
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 6
	}
	resp, err := provider.Search(ctx, memprovider.SearchRequest{
		Query: req.Query,
		BotID: req.Lease.BotID,
		RunID: req.Lease.RunID,
		Limit: limit,
	})
	if err != nil {
		return ReadMemoryResponse{}, err
	}
	items := make([]map[string]any, 0, len(resp.Results))
	for _, item := range resp.Results {
		items = append(items, map[string]any{
			"memory_id": item.ID,
			"scope":     "bot",
			"content":   item.Memory,
			"score":     item.Score,
			"metadata":  item.Metadata,
		})
	}
	return ReadMemoryResponse{Items: items}, nil
}

func (b *RunnerSupportBackend) WriteMemory(ctx context.Context, req WriteMemoryRequest) error {
	provider, err := b.memoryProvider(ctx, req.Lease.BotID)
	if err != nil || provider == nil {
		return err
	}
	for _, entry := range req.Entries {
		content, _ := entry["content"].(string)
		if strings.TrimSpace(content) == "" {
			content, _ = entry["memory"].(string)
		}
		if strings.TrimSpace(content) == "" {
			continue
		}
		if _, err := provider.Add(ctx, memprovider.AddRequest{
			Message:  content,
			BotID:    req.Lease.BotID,
			RunID:    req.Lease.RunID,
			Metadata: mapFromAny(entry["metadata"]),
		}); err != nil {
			return err
		}
	}
	return nil
}

func (b *RunnerSupportBackend) ResolveScopedSecret(ctx context.Context, req ResolveScopedSecretRequest) (ResolveScopedSecretResponse, error) {
	name := strings.TrimSpace(req.Name)
	if strings.HasPrefix(name, "provider:") {
		creds, err := b.ResolveProviderCredentials(ctx, ResolveProviderCredentialsRequest{
			Lease:      req.Lease,
			ProviderID: strings.TrimPrefix(name, "provider:"),
		})
		if err != nil {
			return ResolveScopedSecretResponse{}, err
		}
		return ResolveScopedSecretResponse{Value: firstStringFromMap(creds.Credentials, "api_key")}, nil
	}
	return ResolveScopedSecretResponse{}, ErrRunnerSupportDependencyMissing
}

func (b *RunnerSupportBackend) ResolveProviderCredentials(ctx context.Context, req ResolveProviderCredentialsRequest) (ResolveProviderCredentialsResponse, error) {
	if b == nil || b.queries == nil || b.providers == nil {
		return ResolveProviderCredentialsResponse{}, ErrRunnerSupportDependencyMissing
	}
	provider, err := b.resolveProvider(ctx, req.ProviderID)
	if err != nil {
		return ResolveProviderCredentialsResponse{}, err
	}
	creds, err := b.providers.ResolveModelCredentials(ctx, provider)
	if err != nil {
		return ResolveProviderCredentialsResponse{}, err
	}
	return ResolveProviderCredentialsResponse{
		Credentials: map[string]any{
			"provider_id":      provider.ID.String(),
			"provider_name":    provider.Name,
			"client_type":      provider.ClientType,
			"base_url":         providers.ProviderConfigString(provider, "base_url"),
			"api_key":          creds.APIKey,
			"codex_account_id": creds.CodexAccountID,
		},
	}, nil
}

func (b *RunnerSupportBackend) EvaluateToolApprovalPolicy(ctx context.Context, req EvaluateToolApprovalPolicyRequest) (EvaluateToolApprovalPolicyResponse, error) {
	if b == nil || b.toolApprovals == nil {
		return EvaluateToolApprovalPolicyResponse{}, nil
	}
	eval, err := b.toolApprovals.EvaluatePolicy(ctx, toolapproval.CreatePendingInput{
		BotID:     req.Lease.BotID,
		SessionID: req.Lease.SessionID,
		ToolName:  req.ToolName,
		ToolInput: jsonObject(req.Input),
	})
	if err != nil {
		return EvaluateToolApprovalPolicyResponse{}, err
	}
	return EvaluateToolApprovalPolicyResponse{
		RequiresApproval: eval.Decision == toolapproval.DecisionNeedsApproval,
		Reason:           eval.Decision,
	}, nil
}

func (b *RunnerSupportBackend) RequestToolApproval(ctx context.Context, req RequestToolApprovalRequest) (RequestToolApprovalResponse, error) {
	if b == nil || b.toolApprovals == nil {
		return RequestToolApprovalResponse{}, ErrRunnerSupportDependencyMissing
	}
	created, err := b.toolApprovals.CreatePending(ctx, toolapproval.CreatePendingInput{
		BotID:     req.Lease.BotID,
		SessionID: req.Lease.SessionID,
		ToolName:  req.ToolName,
		ToolInput: jsonObject(req.ToolInput),
	})
	if err != nil {
		return RequestToolApprovalResponse{}, err
	}
	return RequestToolApprovalResponse{RequestID: created.ID}, nil
}

func (b *RunnerSupportBackend) resolveProvider(ctx context.Context, value string) (dbsqlc.Provider, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return dbsqlc.Provider{}, errors.New("provider id or name is required")
	}
	if id, err := db.ParseUUID(trimmed); err == nil {
		return b.queries.GetProviderByID(ctx, id)
	}
	return b.queries.GetProviderByName(ctx, trimmed)
}

func (b *RunnerSupportBackend) memoryProvider(ctx context.Context, botID string) (memprovider.Provider, error) {
	if b == nil || b.queries == nil || b.memory == nil {
		return nil, nil
	}
	pgBotID, err := db.ParseUUID(botID)
	if err != nil {
		return nil, err
	}
	bot, err := b.queries.GetBotByID(ctx, pgBotID)
	if err != nil {
		return nil, err
	}
	providerID := uuidString(bot.MemoryProviderID)
	if providerID == "" {
		return nil, nil
	}
	provider, err := b.memory.Get(providerID)
	if err != nil {
		return nil, err
	}
	return provider, nil
}

func pgText(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return strings.TrimSpace(value.String)
}

func jsonObject(data []byte) map[string]any {
	if len(data) == 0 {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return map[string]any{}
	}
	return out
}

func mapFromAny(value any) map[string]any {
	if out, ok := value.(map[string]any); ok {
		return out
	}
	return map[string]any{}
}

func firstStringFromMap(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if text, ok := values[key].(string); ok && strings.TrimSpace(text) != "" {
			return strings.TrimSpace(text)
		}
	}
	return ""
}

func marshalMap(value map[string]any) []byte {
	if len(value) == 0 {
		return nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	return data
}

func messageText(data []byte) string {
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		return text
	}
	return strings.TrimSpace(string(data))
}

var (
	_ RunContextResolver        = (*RunnerSupportBackend)(nil)
	_ SessionHistoryReader      = (*RunnerSupportBackend)(nil)
	_ RunEventAppender          = (*RunnerSupportBackend)(nil)
	_ SessionMessageAppender    = (*RunnerSupportBackend)(nil)
	_ OutboundSupport           = (*RunnerSupportBackend)(nil)
	_ MemorySupport             = (*RunnerSupportBackend)(nil)
	_ SecretSupport             = (*RunnerSupportBackend)(nil)
	_ ProviderCredentialSupport = (*RunnerSupportBackend)(nil)
	_ ToolApprovalSupport       = (*RunnerSupportBackend)(nil)
)
