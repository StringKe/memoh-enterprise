package connectapi

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/labstack/echo/v4"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/memohai/memoh/internal/accounts"
	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/compaction"
	privatev1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1"
	"github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1/privatev1connect"
	"github.com/memohai/memoh/internal/handlers"
	"github.com/memohai/memoh/internal/heartbeat"
	"github.com/memohai/memoh/internal/schedule"
)

const (
	scheduleDefaultLimit = 50
	scheduleMaxLimit     = 100
)

type ScheduleService struct {
	schedules  *schedule.Service
	heartbeat  *heartbeat.Service
	compaction *compaction.Service
	bots       *bots.Service
	accounts   *accounts.Service
}

func NewScheduleService(
	scheduleService *schedule.Service,
	heartbeatService *heartbeat.Service,
	compactionService *compaction.Service,
	botService *bots.Service,
	accountService *accounts.Service,
) *ScheduleService {
	return &ScheduleService{
		schedules:  scheduleService,
		heartbeat:  heartbeatService,
		compaction: compactionService,
		bots:       botService,
		accounts:   accountService,
	}
}

func NewScheduleHandler(service *ScheduleService) Handler {
	path, handler := privatev1connect.NewScheduleServiceHandler(service)
	return NewHandler(path, handler)
}

func (s *ScheduleService) CreateSchedule(ctx context.Context, req *connect.Request[privatev1.CreateScheduleRequest]) (*connect.Response[privatev1.CreateScheduleResponse], error) {
	botID, err := s.requireBotAccess(ctx, req.Msg.GetBotId())
	if err != nil {
		return nil, err
	}
	if s.schedules == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("schedule service not configured"))
	}
	item, err := s.schedules.Create(ctx, botID, schedule.CreateRequest{
		Name:        req.Msg.GetName(),
		Description: scheduleDescriptionFromMetadata(req.Msg.GetMetadata(), req.Msg.GetName()),
		Pattern:     req.Msg.GetCron(),
		Command:     req.Msg.GetPrompt(),
		Enabled:     &req.Msg.Enabled,
		MaxCalls:    scheduleMaxCallsFromMetadata(req.Msg.GetMetadata()),
	})
	if err != nil {
		return nil, scheduleConnectError(err)
	}
	return connect.NewResponse(&privatev1.CreateScheduleResponse{Schedule: scheduleToProto(item)}), nil
}

func (s *ScheduleService) ListSchedules(ctx context.Context, req *connect.Request[privatev1.ListSchedulesRequest]) (*connect.Response[privatev1.ListSchedulesResponse], error) {
	botID, err := s.requireBotAccess(ctx, req.Msg.GetBotId())
	if err != nil {
		return nil, err
	}
	if s.schedules == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("schedule service not configured"))
	}
	items, err := s.schedules.List(ctx, botID)
	if err != nil {
		return nil, scheduleConnectError(err)
	}
	return connect.NewResponse(&privatev1.ListSchedulesResponse{
		Schedules: schedulesToProto(items),
		Page:      &privatev1.PageResponse{},
	}), nil
}

func (s *ScheduleService) GetSchedule(ctx context.Context, req *connect.Request[privatev1.GetScheduleRequest]) (*connect.Response[privatev1.GetScheduleResponse], error) {
	id := strings.TrimSpace(req.Msg.GetId())
	if id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}
	if s.schedules == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("schedule service not configured"))
	}
	item, err := s.schedules.Get(ctx, id)
	if err != nil {
		return nil, scheduleConnectError(err)
	}
	if _, err := s.requireBotAccess(ctx, item.BotID); err != nil {
		return nil, err
	}
	return connect.NewResponse(&privatev1.GetScheduleResponse{Schedule: scheduleToProto(item)}), nil
}

func (s *ScheduleService) UpdateSchedule(ctx context.Context, req *connect.Request[privatev1.UpdateScheduleRequest]) (*connect.Response[privatev1.UpdateScheduleResponse], error) {
	id := strings.TrimSpace(req.Msg.GetId())
	if id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}
	if s.schedules == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("schedule service not configured"))
	}
	item, err := s.schedules.Get(ctx, id)
	if err != nil {
		return nil, scheduleConnectError(err)
	}
	if _, err := s.requireBotAccess(ctx, item.BotID); err != nil {
		return nil, err
	}
	update := schedule.UpdateRequest{
		Name:     req.Msg.Name,
		Pattern:  req.Msg.Cron,
		Command:  req.Msg.Prompt,
		Enabled:  req.Msg.Enabled,
		MaxCalls: scheduleMaxCallsFromMetadata(req.Msg.GetMetadata()),
	}
	if description, ok := scheduleOptionalDescriptionFromMetadata(req.Msg.GetMetadata()); ok {
		update.Description = &description
	}
	updated, err := s.schedules.Update(ctx, id, update)
	if err != nil {
		return nil, scheduleConnectError(err)
	}
	return connect.NewResponse(&privatev1.UpdateScheduleResponse{Schedule: scheduleToProto(updated)}), nil
}

func (s *ScheduleService) DeleteSchedule(ctx context.Context, req *connect.Request[privatev1.DeleteScheduleRequest]) (*connect.Response[privatev1.DeleteScheduleResponse], error) {
	id := strings.TrimSpace(req.Msg.GetId())
	if id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}
	if s.schedules == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("schedule service not configured"))
	}
	item, err := s.schedules.Get(ctx, id)
	if err != nil {
		return nil, scheduleConnectError(err)
	}
	if _, err := s.requireBotAccess(ctx, item.BotID); err != nil {
		return nil, err
	}
	if err := s.schedules.Delete(ctx, id); err != nil {
		return nil, scheduleConnectError(err)
	}
	return connect.NewResponse(&privatev1.DeleteScheduleResponse{}), nil
}

func (s *ScheduleService) ListScheduleLogs(ctx context.Context, req *connect.Request[privatev1.ListScheduleLogsRequest]) (*connect.Response[privatev1.ListScheduleLogsResponse], error) {
	botID, err := s.requireBotAccess(ctx, req.Msg.GetBotId())
	if err != nil {
		return nil, err
	}
	if s.schedules == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("schedule service not configured"))
	}
	limit, offset, err := schedulePagination(req.Msg.GetPage())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	items, total, err := s.schedules.ListLogs(ctx, botID, limit, offset)
	if err != nil {
		return nil, scheduleConnectError(err)
	}
	return connect.NewResponse(&privatev1.ListScheduleLogsResponse{
		Logs: scheduleLogsToProto(items),
		Page: schedulePageResponse(total, limit, offset),
	}), nil
}

func (s *ScheduleService) ListScheduleLogsBySchedule(ctx context.Context, req *connect.Request[privatev1.ListScheduleLogsByScheduleRequest]) (*connect.Response[privatev1.ListScheduleLogsByScheduleResponse], error) {
	scheduleID := strings.TrimSpace(req.Msg.GetScheduleId())
	if scheduleID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("schedule_id is required"))
	}
	if s.schedules == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("schedule service not configured"))
	}
	item, err := s.schedules.Get(ctx, scheduleID)
	if err != nil {
		return nil, scheduleConnectError(err)
	}
	if _, err := s.requireBotAccess(ctx, item.BotID); err != nil {
		return nil, err
	}
	limit, offset, err := schedulePagination(req.Msg.GetPage())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	items, total, err := s.schedules.ListLogsBySchedule(ctx, scheduleID, limit, offset)
	if err != nil {
		return nil, scheduleConnectError(err)
	}
	return connect.NewResponse(&privatev1.ListScheduleLogsByScheduleResponse{
		Logs: scheduleLogsToProto(items),
		Page: schedulePageResponse(total, limit, offset),
	}), nil
}

func (s *ScheduleService) DeleteScheduleLogs(ctx context.Context, req *connect.Request[privatev1.DeleteScheduleLogsRequest]) (*connect.Response[privatev1.DeleteScheduleLogsResponse], error) {
	botID := strings.TrimSpace(req.Msg.GetBotId())
	scheduleID := strings.TrimSpace(req.Msg.GetScheduleId())
	if botID == "" && scheduleID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("bot_id or schedule_id is required"))
	}
	if s.schedules == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("schedule service not configured"))
	}
	if scheduleID != "" {
		item, err := s.schedules.Get(ctx, scheduleID)
		if err != nil {
			return nil, scheduleConnectError(err)
		}
		if botID != "" && item.BotID != botID {
			return nil, connect.NewError(connect.CodePermissionDenied, errors.New("bot mismatch"))
		}
		botID = item.BotID
	}
	if _, err := s.requireBotAccess(ctx, botID); err != nil {
		return nil, err
	}
	if err := s.schedules.DeleteLogs(ctx, botID); err != nil {
		return nil, scheduleConnectError(err)
	}
	return connect.NewResponse(&privatev1.DeleteScheduleLogsResponse{}), nil
}

func (s *ScheduleService) ListHeartbeatLogs(ctx context.Context, req *connect.Request[privatev1.ListHeartbeatLogsRequest]) (*connect.Response[privatev1.ListHeartbeatLogsResponse], error) {
	botID, err := s.requireBotAccess(ctx, req.Msg.GetBotId())
	if err != nil {
		return nil, err
	}
	if s.heartbeat == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("heartbeat service not configured"))
	}
	limit, offset, err := schedulePagination(req.Msg.GetPage())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	items, total, err := s.heartbeat.ListLogs(ctx, botID, limit, offset)
	if err != nil {
		return nil, scheduleConnectError(err)
	}
	return connect.NewResponse(&privatev1.ListHeartbeatLogsResponse{
		Logs:       heartbeatLogsToProto(items),
		Page:       schedulePageResponse(total, limit, offset),
		TotalCount: int32(total), //nolint:gosec // service clamps logs pagination to bounded rows
	}), nil
}

func (s *ScheduleService) DeleteHeartbeatLogs(ctx context.Context, req *connect.Request[privatev1.DeleteHeartbeatLogsRequest]) (*connect.Response[privatev1.DeleteHeartbeatLogsResponse], error) {
	botID, err := s.requireBotAccess(ctx, req.Msg.GetBotId())
	if err != nil {
		return nil, err
	}
	if s.heartbeat == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("heartbeat service not configured"))
	}
	if err := s.heartbeat.DeleteLogs(ctx, botID); err != nil {
		return nil, scheduleConnectError(err)
	}
	return connect.NewResponse(&privatev1.DeleteHeartbeatLogsResponse{}), nil
}

func (s *ScheduleService) ListCompactionLogs(ctx context.Context, req *connect.Request[privatev1.ListCompactionLogsRequest]) (*connect.Response[privatev1.ListCompactionLogsResponse], error) {
	botID, err := s.requireBotAccess(ctx, req.Msg.GetBotId())
	if err != nil {
		return nil, err
	}
	if s.compaction == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("compaction service not configured"))
	}
	limit, offset, err := schedulePagination(req.Msg.GetPage())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	items, total, err := s.compaction.ListLogs(ctx, botID, limit, offset)
	if err != nil {
		return nil, scheduleConnectError(err)
	}
	return connect.NewResponse(&privatev1.ListCompactionLogsResponse{
		Logs:       compactionLogsToProto(items),
		Page:       schedulePageResponse(total, limit, offset),
		TotalCount: int32(total), //nolint:gosec // service clamps logs pagination to bounded rows
	}), nil
}

func (s *ScheduleService) DeleteCompactionLogs(ctx context.Context, req *connect.Request[privatev1.DeleteCompactionLogsRequest]) (*connect.Response[privatev1.DeleteCompactionLogsResponse], error) {
	botID, err := s.requireBotAccess(ctx, req.Msg.GetBotId())
	if err != nil {
		return nil, err
	}
	if s.compaction == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("compaction service not configured"))
	}
	if err := s.compaction.DeleteLogs(ctx, botID); err != nil {
		return nil, scheduleConnectError(err)
	}
	return connect.NewResponse(&privatev1.DeleteCompactionLogsResponse{}), nil
}

func (s *ScheduleService) requireBotAccess(ctx context.Context, botID string) (string, error) {
	botID = strings.TrimSpace(botID)
	if botID == "" {
		return "", connect.NewError(connect.CodeInvalidArgument, errors.New("bot_id is required"))
	}
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return "", connect.NewError(connect.CodeUnauthenticated, err)
	}
	if _, err := handlers.AuthorizeBotAccess(ctx, s.bots, s.accounts, userID, botID); err != nil {
		return "", echoToConnectError(err)
	}
	return botID, nil
}

func scheduleConnectError(err error) error {
	var httpErr *echo.HTTPError
	if errors.As(err, &httpErr) {
		return echoToConnectError(err)
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "not found"), strings.Contains(msg, "no rows"):
		return connect.NewError(connect.CodeNotFound, err)
	case strings.Contains(msg, "required"), strings.Contains(msg, "invalid"), strings.Contains(msg, "out of range"):
		return connect.NewError(connect.CodeInvalidArgument, err)
	default:
		return connect.NewError(connect.CodeInternal, err)
	}
}

func schedulePagination(page *privatev1.PageRequest) (int, int, error) {
	limit := int32(scheduleDefaultLimit)
	offset := int32(0)
	if page != nil {
		if page.GetPageSize() > 0 {
			limit = page.GetPageSize()
		}
		if limit > scheduleMaxLimit {
			limit = scheduleMaxLimit
		}
		if token := strings.TrimSpace(page.GetPageToken()); token != "" {
			parsed, err := strconv.ParseInt(token, 10, 32)
			if err != nil || parsed < 0 {
				return 0, 0, errors.New("page_token must be a non-negative offset")
			}
			offset = int32(parsed)
		}
	}
	return int(limit), int(offset), nil
}

func schedulePageResponse(total int64, limit, offset int) *privatev1.PageResponse {
	resp := &privatev1.PageResponse{}
	if int64(offset+limit) < total {
		resp.NextPageToken = strconv.Itoa(offset + limit)
	}
	return resp
}

func schedulesToProto(items []schedule.Schedule) []*privatev1.ScheduleTask {
	out := make([]*privatev1.ScheduleTask, 0, len(items))
	for _, item := range items {
		out = append(out, scheduleToProto(item))
	}
	return out
}

func scheduleToProto(item schedule.Schedule) *privatev1.ScheduleTask {
	metadata := map[string]any{
		"description":   item.Description,
		"current_calls": item.CurrentCalls,
	}
	if item.MaxCalls != nil {
		metadata["max_calls"] = *item.MaxCalls
	}
	return &privatev1.ScheduleTask{
		Id:       item.ID,
		BotId:    item.BotID,
		Name:     item.Name,
		Cron:     item.Pattern,
		Enabled:  item.Enabled,
		Prompt:   item.Command,
		Metadata: mapToStruct(metadata),
		Audit: &privatev1.AuditFields{
			CreatedAt: timeToProto(item.CreatedAt),
			UpdatedAt: timeToProto(item.UpdatedAt),
		},
	}
}

func scheduleLogsToProto(items []schedule.Log) []*privatev1.ScheduleLog {
	out := make([]*privatev1.ScheduleLog, 0, len(items))
	for _, item := range items {
		out = append(out, scheduleLogToProto(item))
	}
	return out
}

func scheduleLogToProto(item schedule.Log) *privatev1.ScheduleLog {
	message := item.ResultText
	if message == "" {
		message = item.ErrorMessage
	}
	metadata := map[string]any{}
	if item.SessionID != "" {
		metadata["session_id"] = item.SessionID
	}
	if item.ResultText != "" {
		metadata["result_text"] = item.ResultText
	}
	if item.ErrorMessage != "" {
		metadata["error_message"] = item.ErrorMessage
	}
	if item.Usage != nil {
		metadata["usage"] = item.Usage
	}
	return &privatev1.ScheduleLog{
		Id:         item.ID,
		ScheduleId: item.ScheduleID,
		BotId:      item.BotID,
		Status:     item.Status,
		Message:    message,
		StartedAt:  timeToProto(item.StartedAt),
		FinishedAt: scheduleOptionalTimeToProto(item.CompletedAt),
		Metadata:   mapToStruct(metadata),
	}
}

func heartbeatLogsToProto(items []heartbeat.Log) []*privatev1.HeartbeatLog {
	out := make([]*privatev1.HeartbeatLog, 0, len(items))
	for _, item := range items {
		out = append(out, heartbeatLogToProto(item))
	}
	return out
}

func heartbeatLogToProto(item heartbeat.Log) *privatev1.HeartbeatLog {
	return &privatev1.HeartbeatLog{
		Id:           item.ID,
		BotId:        item.BotID,
		SessionId:    item.SessionID,
		Status:       item.Status,
		ResultText:   item.ResultText,
		ErrorMessage: item.ErrorMessage,
		Usage:        anyToStruct(item.Usage),
		StartedAt:    timeToProto(item.StartedAt),
		CompletedAt:  scheduleOptionalTimeToProto(item.CompletedAt),
	}
}

func compactionLogsToProto(items []compaction.Log) []*privatev1.CompactionLog {
	out := make([]*privatev1.CompactionLog, 0, len(items))
	for _, item := range items {
		out = append(out, compactionLogToProto(item))
	}
	return out
}

func compactionLogToProto(item compaction.Log) *privatev1.CompactionLog {
	return &privatev1.CompactionLog{
		Id:           item.ID,
		BotId:        item.BotID,
		SessionId:    item.SessionID,
		Status:       item.Status,
		Summary:      item.Summary,
		MessageCount: int32(item.MessageCount), //nolint:gosec // persisted count is bounded by message rows
		ErrorMessage: item.ErrorMessage,
		Usage:        anyToStruct(item.Usage),
		ModelId:      item.ModelID,
		StartedAt:    timeToProto(item.StartedAt),
		CompletedAt:  scheduleOptionalTimeToProto(item.CompletedAt),
	}
}

func scheduleOptionalTimeToProto(value *time.Time) *timestamppb.Timestamp {
	if value == nil || value.IsZero() {
		return nil
	}
	return timestamppb.New(*value)
}

func scheduleDescriptionFromMetadata(metadata *structpb.Struct, fallback string) string {
	if description, ok := scheduleOptionalDescriptionFromMetadata(metadata); ok && description != "" {
		return description
	}
	return fallback
}

func scheduleOptionalDescriptionFromMetadata(metadata *structpb.Struct) (string, bool) {
	fields := metadata.GetFields()
	if fields == nil {
		return "", false
	}
	value, ok := fields["description"]
	if !ok {
		return "", false
	}
	return strings.TrimSpace(value.GetStringValue()), true
}

func scheduleMaxCallsFromMetadata(metadata *structpb.Struct) schedule.NullableInt {
	fields := metadata.GetFields()
	if fields == nil {
		return schedule.NullableInt{}
	}
	value, ok := fields["max_calls"]
	if !ok {
		return schedule.NullableInt{}
	}
	if _, ok := value.GetKind().(*structpb.Value_NullValue); ok {
		return schedule.NullableInt{Set: true}
	}
	maxCalls := int(value.GetNumberValue())
	return schedule.NullableInt{Set: true, Value: &maxCalls}
}

var _ privatev1connect.ScheduleServiceHandler = (*ScheduleService)(nil)
