package connectapi

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/protobuf/types/known/timestamppb"

	privatev1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1"
	"github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1/privatev1connect"
	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
	"github.com/memohai/memoh/internal/iam/rbac"
)

const (
	usageRecordsDefaultLimit = 20
	usageRecordsMaxLimit     = 100
)

type UsageService struct {
	queries dbstore.Queries
	bots    *BotService
}

func NewUsageService(queries dbstore.Queries, bots *BotService) *UsageService {
	return &UsageService{queries: queries, bots: bots}
}

func NewUsageHandler(service *UsageService) Handler {
	path, handler := privatev1connect.NewUsageServiceHandler(service)
	return NewHandler(path, handler)
}

func (s *UsageService) GetTokenUsage(ctx context.Context, req *connect.Request[privatev1.GetTokenUsageRequest]) (*connect.Response[privatev1.GetTokenUsageResponse], error) {
	_, pgBotID, from, to, err := s.authorizeAndParseRange(ctx, req.Msg.GetBotId(), req.Msg.GetFrom(), req.Msg.GetTo())
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.GetTokenUsageByDayAndType(ctx, sqlc.GetTokenUsageByDayAndTypeParams{
		BotID:    pgBotID,
		FromTime: from,
		ToTime:   to,
		ModelID:  pgtype.UUID{},
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	var summary privatev1.TokenUsageSummary
	summary.BotId = req.Msg.GetBotId()
	summary.Currency = "USD"
	for _, row := range rows {
		summary.PromptTokens += row.InputTokens + row.CacheReadTokens + row.CacheWriteTokens
		summary.CompletionTokens += row.OutputTokens + row.ReasoningTokens
	}
	summary.TotalTokens = summary.PromptTokens + summary.CompletionTokens
	return connect.NewResponse(&privatev1.GetTokenUsageResponse{Summary: &summary}), nil
}

func (s *UsageService) ListTokenUsageRecords(ctx context.Context, req *connect.Request[privatev1.ListTokenUsageRecordsRequest]) (*connect.Response[privatev1.ListTokenUsageRecordsResponse], error) {
	_, pgBotID, from, to, err := s.authorizeAndParseRange(ctx, req.Msg.GetBotId(), req.Msg.GetFrom(), req.Msg.GetTo())
	if err != nil {
		return nil, err
	}
	limit, offset, err := usagePagination(req.Msg.GetPage())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	rows, err := s.queries.ListTokenUsageRecords(ctx, sqlc.ListTokenUsageRecordsParams{
		BotID:       pgBotID,
		FromTime:    from,
		ToTime:      to,
		ModelID:     pgtype.UUID{},
		SessionType: pgtype.Text{},
		PageOffset:  offset,
		PageLimit:   limit,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	items := make([]*privatev1.TokenUsageRecord, 0, len(rows))
	for _, row := range rows {
		items = append(items, tokenUsageRecordToProto(req.Msg.GetBotId(), row))
	}
	nextPageToken := ""
	if len(items) == int(limit) {
		nextPageToken = strconv.FormatInt(int64(offset+limit), 10)
	}
	return connect.NewResponse(&privatev1.ListTokenUsageRecordsResponse{
		Records: items,
		Page:    &privatev1.PageResponse{NextPageToken: nextPageToken},
	}), nil
}

func (s *UsageService) authorizeAndParseRange(ctx context.Context, botID string, fromValue, toValue *timestamppb.Timestamp) (string, pgtype.UUID, pgtype.Timestamptz, pgtype.Timestamptz, error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return "", pgtype.UUID{}, pgtype.Timestamptz{}, pgtype.Timestamptz{}, connect.NewError(connect.CodeUnauthenticated, err)
	}
	botID = strings.TrimSpace(botID)
	if botID == "" {
		return "", pgtype.UUID{}, pgtype.Timestamptz{}, pgtype.Timestamptz{}, connect.NewError(connect.CodeInvalidArgument, errors.New("bot_id is required"))
	}
	if s.bots == nil {
		return "", pgtype.UUID{}, pgtype.Timestamptz{}, pgtype.Timestamptz{}, connect.NewError(connect.CodeInternal, errors.New("bot service not configured"))
	}
	if err := s.bots.requireBotPermission(ctx, userID, botID, rbac.PermissionBotRead); err != nil {
		return "", pgtype.UUID{}, pgtype.Timestamptz{}, pgtype.Timestamptz{}, botConnectError(err)
	}
	if s.queries == nil {
		return "", pgtype.UUID{}, pgtype.Timestamptz{}, pgtype.Timestamptz{}, connect.NewError(connect.CodeInternal, errors.New("usage queries not configured"))
	}
	pgBotID, err := db.ParseUUID(botID)
	if err != nil {
		return "", pgtype.UUID{}, pgtype.Timestamptz{}, pgtype.Timestamptz{}, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid bot_id"))
	}
	if fromValue == nil || toValue == nil {
		return "", pgtype.UUID{}, pgtype.Timestamptz{}, pgtype.Timestamptz{}, connect.NewError(connect.CodeInvalidArgument, errors.New("from and to are required"))
	}
	fromTime := fromValue.AsTime()
	toTime := toValue.AsTime()
	if fromTime.IsZero() || toTime.IsZero() || !toTime.After(fromTime) {
		return "", pgtype.UUID{}, pgtype.Timestamptz{}, pgtype.Timestamptz{}, connect.NewError(connect.CodeInvalidArgument, errors.New("to must be after from"))
	}
	return userID,
		pgBotID,
		pgtype.Timestamptz{Time: fromTime, Valid: true},
		pgtype.Timestamptz{Time: toTime, Valid: true},
		nil
}

func usagePagination(page *privatev1.PageRequest) (int32, int32, error) {
	limit := int32(usageRecordsDefaultLimit)
	offset := int32(0)
	if page != nil {
		if page.GetPageSize() > 0 {
			limit = page.GetPageSize()
		}
		if limit > usageRecordsMaxLimit {
			limit = usageRecordsMaxLimit
		}
		if token := strings.TrimSpace(page.GetPageToken()); token != "" {
			parsed, err := strconv.ParseInt(token, 10, 32)
			if err != nil || parsed < 0 {
				return 0, 0, errors.New("page_token must be a non-negative offset")
			}
			offset = int32(parsed)
		}
	}
	return limit, offset, nil
}

func tokenUsageRecordToProto(botID string, row sqlc.ListTokenUsageRecordsRow) *privatev1.TokenUsageRecord {
	inputTokens := row.InputTokens + row.CacheReadTokens + row.CacheWriteTokens
	outputTokens := row.OutputTokens + row.ReasoningTokens
	return &privatev1.TokenUsageRecord{
		Id:               usageOptionalUUID(row.ID),
		BotId:            botID,
		ModelId:          usageOptionalUUID(row.ModelID),
		PromptTokens:     inputTokens,
		CompletionTokens: outputTokens,
		TotalTokens:      inputTokens + outputTokens,
		Currency:         "USD",
		CreatedAt:        usageTimeToProto(row.CreatedAt),
		Metadata: mapToStruct(map[string]any{
			"session_id":         usageOptionalUUID(row.SessionID),
			"session_type":       row.SessionType,
			"model_slug":         row.ModelSlug,
			"model_name":         row.ModelName,
			"provider_name":      row.ProviderName,
			"cache_read_tokens":  row.CacheReadTokens,
			"cache_write_tokens": row.CacheWriteTokens,
			"reasoning_tokens":   row.ReasoningTokens,
		}),
	}
}

func usageTimeToProto(value pgtype.Timestamptz) *timestamppb.Timestamp {
	if !value.Valid || value.Time.IsZero() {
		return nil
	}
	return timestamppb.New(value.Time.UTC())
}

func usageOptionalUUID(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	return value.String()
}
