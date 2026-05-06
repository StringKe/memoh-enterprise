package connectapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/labstack/echo/v4"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/memohai/memoh/internal/accounts"
	"github.com/memohai/memoh/internal/bots"
	privatev1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1"
	"github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1/privatev1connect"
	"github.com/memohai/memoh/internal/handlers"
	memprovider "github.com/memohai/memoh/internal/memory/adapters"
	"github.com/memohai/memoh/internal/settings"
)

const (
	connectSharedMemoryNamespace    = "bot"
	connectDefaultBuiltinProviderID = "__builtin_default__"
)

type memoryProviderRegistry interface {
	Get(id string) (memprovider.Provider, error)
}

type MemoryService struct {
	bots      *bots.Service
	accounts  *accounts.Service
	settings  *settings.Service
	providers memoryProviderRegistry
}

func NewMemoryService(botService *bots.Service, accountService *accounts.Service, settingsService *settings.Service, registry *memprovider.Registry) *MemoryService {
	return &MemoryService{
		bots:      botService,
		accounts:  accountService,
		settings:  settingsService,
		providers: registry,
	}
}

func NewMemoryHandler(service *MemoryService) Handler {
	path, handler := privatev1connect.NewMemoryServiceHandler(service)
	return NewHandler(path, handler)
}

func (s *MemoryService) AddBotMemory(ctx context.Context, req *connect.Request[privatev1.AddBotMemoryRequest]) (*connect.Response[privatev1.AddBotMemoryResponse], error) {
	botID, err := s.requireBotAccess(ctx, req.Msg.GetBotId())
	if err != nil {
		return nil, err
	}
	namespace, err := normalizeConnectMemoryNamespace(req.Msg.GetNamespace())
	if err != nil {
		return nil, err
	}
	provider, err := s.checkService(ctx, botID)
	if err != nil {
		return nil, err
	}
	resp, err := provider.Add(ctx, memprovider.AddRequest{
		Message:          req.Msg.GetContent(),
		Messages:         memoryMessagesFromProto(req.Msg.GetMessages()),
		BotID:            botID,
		RunID:            strings.TrimSpace(req.Msg.GetRunId()),
		Metadata:         structToMap(req.Msg.GetMetadata()),
		Filters:          buildConnectMemoryFiltersWith(namespace, botID, structToMap(req.Msg.GetFilters())),
		Infer:            req.Msg.Infer,
		EmbeddingEnabled: req.Msg.EmbeddingEnabled,
	})
	if err != nil {
		return nil, connectError(err)
	}
	items := memoryItemsToProto(botID, resp.Results)
	out := &privatev1.AddBotMemoryResponse{
		Memories:  items,
		Relations: memoryRelationsToProto(resp.Relations),
	}
	if len(items) > 0 {
		out.Memory = items[0]
	}
	return connect.NewResponse(out), nil
}

func (s *MemoryService) ListBotMemories(ctx context.Context, req *connect.Request[privatev1.ListBotMemoriesRequest]) (*connect.Response[privatev1.ListBotMemoriesResponse], error) {
	botID, err := s.requireBotAccess(ctx, req.Msg.GetBotId())
	if err != nil {
		return nil, err
	}
	provider, err := s.checkService(ctx, botID)
	if err != nil {
		return nil, err
	}
	resp, err := provider.GetAll(ctx, memprovider.GetAllRequest{
		BotID:   botID,
		Filters: buildConnectMemoryFilters(botID),
		NoStats: req.Msg.GetNoStats(),
	})
	if err != nil {
		return nil, connectError(err)
	}
	items := deduplicateConnectMemoryItems(resp.Results)
	return connect.NewResponse(&privatev1.ListBotMemoriesResponse{
		Memories:  memoryItemsToProto(botID, items),
		Relations: memoryRelationsToProto(resp.Relations),
		Page:      &privatev1.PageResponse{},
	}), nil
}

func (s *MemoryService) DeleteBotMemories(ctx context.Context, req *connect.Request[privatev1.DeleteBotMemoriesRequest]) (*connect.Response[privatev1.DeleteBotMemoriesResponse], error) {
	botID, err := s.requireBotAccess(ctx, req.Msg.GetBotId())
	if err != nil {
		return nil, err
	}
	provider, err := s.checkService(ctx, botID)
	if err != nil {
		return nil, err
	}
	if len(req.Msg.GetMemoryIds()) > 0 {
		resp, err := provider.DeleteBatch(ctx, req.Msg.GetMemoryIds())
		if err != nil {
			return nil, connectError(err)
		}
		return connect.NewResponse(&privatev1.DeleteBotMemoriesResponse{Message: resp.Message}), nil
	}
	if _, err := provider.DeleteAll(ctx, memprovider.DeleteAllRequest{
		BotID:   botID,
		Filters: buildConnectMemoryFilters(botID),
	}); err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&privatev1.DeleteBotMemoriesResponse{Message: "All memories deleted successfully!"}), nil
}

func (s *MemoryService) DeleteBotMemory(ctx context.Context, req *connect.Request[privatev1.DeleteBotMemoryRequest]) (*connect.Response[privatev1.DeleteBotMemoryResponse], error) {
	botID, err := s.requireBotAccess(ctx, req.Msg.GetBotId())
	if err != nil {
		return nil, err
	}
	memoryID := strings.TrimSpace(req.Msg.GetMemoryId())
	if memoryID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("memory_id is required"))
	}
	provider, err := s.checkService(ctx, botID)
	if err != nil {
		return nil, err
	}
	if _, err := provider.Delete(ctx, memoryID); err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&privatev1.DeleteBotMemoryResponse{}), nil
}

func (s *MemoryService) SearchBotMemory(ctx context.Context, req *connect.Request[privatev1.SearchBotMemoryRequest]) (*connect.Response[privatev1.SearchBotMemoryResponse], error) {
	botID, err := s.requireBotAccess(ctx, req.Msg.GetBotId())
	if err != nil {
		return nil, err
	}
	query := strings.TrimSpace(req.Msg.GetQuery())
	if query == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("query is required"))
	}
	provider, err := s.checkService(ctx, botID)
	if err != nil {
		return nil, err
	}
	resp, err := provider.Search(ctx, memprovider.SearchRequest{
		Query:            query,
		BotID:            botID,
		RunID:            strings.TrimSpace(req.Msg.GetRunId()),
		Limit:            int(req.Msg.GetLimit()),
		Filters:          buildConnectMemoryFiltersWith(connectSharedMemoryNamespace, botID, structToMap(req.Msg.GetFilters())),
		Sources:          req.Msg.GetSources(),
		EmbeddingEnabled: req.Msg.EmbeddingEnabled,
		NoStats:          req.Msg.GetNoStats(),
	})
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&privatev1.SearchBotMemoryResponse{
		Memories:  memoryItemsToProto(botID, deduplicateConnectMemoryItems(resp.Results)),
		Relations: memoryRelationsToProto(resp.Relations),
	}), nil
}

func (s *MemoryService) CompactBotMemory(ctx context.Context, req *connect.Request[privatev1.CompactBotMemoryRequest]) (*connect.Response[privatev1.CompactBotMemoryResponse], error) {
	botID, err := s.requireBotAccess(ctx, req.Msg.GetBotId())
	if err != nil {
		return nil, err
	}
	ratio := req.Msg.GetRatio()
	if ratio <= 0 || ratio > 1 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("ratio is required and must be in range (0, 1]"))
	}
	decayDays := 0
	if req.Msg.DecayDays != nil && req.Msg.GetDecayDays() > 0 {
		decayDays = int(req.Msg.GetDecayDays())
	}
	provider, err := s.checkService(ctx, botID)
	if err != nil {
		return nil, err
	}
	result, err := provider.Compact(ctx, buildConnectMemoryFilters(botID), ratio, decayDays)
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&privatev1.CompactBotMemoryResponse{
		Status: memoryStatusToProto(botID, memprovider.MemoryStatusResponse{}),
		Result: compactResultToProto(botID, result),
	}), nil
}

func (s *MemoryService) RebuildBotMemory(ctx context.Context, req *connect.Request[privatev1.RebuildBotMemoryRequest]) (*connect.Response[privatev1.RebuildBotMemoryResponse], error) {
	botID, err := s.requireBotAccess(ctx, req.Msg.GetBotId())
	if err != nil {
		return nil, err
	}
	provider, err := s.checkService(ctx, botID)
	if err != nil {
		return nil, err
	}
	syncProvider, ok := provider.(memprovider.SourceSyncProvider)
	if !ok {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("selected memory provider does not support rebuild from markdown source"))
	}
	status, err := syncProvider.Status(ctx, botID)
	if err != nil {
		return nil, connectError(err)
	}
	if !status.CanManualSync {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("manual sync is not available for the selected memory provider"))
	}
	result, err := syncProvider.Rebuild(ctx, botID)
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&privatev1.RebuildBotMemoryResponse{
		Status: memoryStatusToProto(botID, status),
		Result: rebuildResultToProto(result),
	}), nil
}

func (s *MemoryService) GetBotMemoryStatus(ctx context.Context, req *connect.Request[privatev1.GetBotMemoryStatusRequest]) (*connect.Response[privatev1.GetBotMemoryStatusResponse], error) {
	botID, err := s.requireBotAccess(ctx, req.Msg.GetBotId())
	if err != nil {
		return nil, err
	}
	provider, err := s.checkService(ctx, botID)
	if err != nil {
		return nil, err
	}
	syncProvider, ok := provider.(memprovider.SourceSyncProvider)
	if !ok {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("selected memory provider does not expose runtime status"))
	}
	status, err := syncProvider.Status(ctx, botID)
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&privatev1.GetBotMemoryStatusResponse{
		Status: memoryStatusToProto(botID, status),
	}), nil
}

func (s *MemoryService) GetBotMemoryUsage(ctx context.Context, req *connect.Request[privatev1.GetBotMemoryUsageRequest]) (*connect.Response[privatev1.GetBotMemoryUsageResponse], error) {
	botID, err := s.requireBotAccess(ctx, req.Msg.GetBotId())
	if err != nil {
		return nil, err
	}
	provider, err := s.checkService(ctx, botID)
	if err != nil {
		return nil, err
	}
	usage, err := provider.Usage(ctx, buildConnectMemoryFilters(botID))
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&privatev1.GetBotMemoryUsageResponse{
		Usage: &privatev1.BotMemoryUsage{
			BotId:          botID,
			ItemCount:      int64(usage.Count),
			EstimatedBytes: usage.EstimatedStorageBytes,
			TotalTextBytes: usage.TotalTextBytes,
			AvgTextBytes:   usage.AvgTextBytes,
			Metadata:       mapToStruct(memoryUsageMetadata(usage)),
		},
	}), nil
}

func (s *MemoryService) checkService(ctx context.Context, botID string) (memprovider.Provider, error) {
	if p := s.resolveProvider(ctx, botID); p != nil {
		return p, nil
	}
	return nil, connect.NewError(connect.CodeUnavailable, errors.New("memory service not available"))
}

func (s *MemoryService) resolveProvider(ctx context.Context, botID string) memprovider.Provider {
	if s.providers == nil {
		return nil
	}
	if s.settings != nil {
		botSettings, err := s.settings.GetBot(ctx, botID)
		if err == nil {
			if providerID := strings.TrimSpace(botSettings.MemoryProviderID); providerID != "" {
				if p, getErr := s.providers.Get(providerID); getErr == nil {
					return p
				}
			}
		}
	}
	p, err := s.providers.Get(connectDefaultBuiltinProviderID)
	if err != nil {
		return nil
	}
	return p
}

func (s *MemoryService) requireBotAccess(ctx context.Context, botID string) (string, error) {
	botID = strings.TrimSpace(botID)
	if botID == "" {
		return "", connect.NewError(connect.CodeInvalidArgument, errors.New("bot_id is required"))
	}
	channelIdentityID, err := UserIDFromContext(ctx)
	if err != nil {
		return "", connect.NewError(connect.CodeUnauthenticated, err)
	}
	if _, err := handlers.AuthorizeBotAccess(ctx, s.bots, s.accounts, channelIdentityID, botID); err != nil {
		return "", echoToConnectError(err)
	}
	return botID, nil
}

func echoToConnectError(err error) error {
	var httpErr *echo.HTTPError
	if !errors.As(err, &httpErr) {
		return connectError(err)
	}
	code := connect.CodeInternal
	switch httpErr.Code {
	case http.StatusBadRequest:
		code = connect.CodeInvalidArgument
	case http.StatusForbidden:
		code = connect.CodePermissionDenied
	case http.StatusNotFound:
		code = connect.CodeNotFound
	case http.StatusServiceUnavailable:
		code = connect.CodeUnavailable
	case http.StatusConflict:
		code = connect.CodeFailedPrecondition
	}
	return connect.NewError(code, errors.New(strings.TrimSpace(toString(httpErr.Message))))
}

func buildConnectMemoryFilters(botID string) map[string]any {
	return buildConnectMemoryFiltersWith(connectSharedMemoryNamespace, botID, nil)
}

func buildConnectMemoryFiltersWith(namespace, botID string, extra map[string]any) map[string]any {
	filters := make(map[string]any, len(extra)+2)
	for key, value := range extra {
		filters[key] = value
	}
	filters["namespace"] = namespace
	filters["scopeId"] = botID
	return filters
}

func normalizeConnectMemoryNamespace(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", connectSharedMemoryNamespace:
		return connectSharedMemoryNamespace, nil
	default:
		return "", connect.NewError(connect.CodeInvalidArgument, errors.New("invalid namespace: "+raw))
	}
}

func memoryMessagesFromProto(items []*privatev1.MemoryMessage) []memprovider.Message {
	if len(items) == 0 {
		return nil
	}
	out := make([]memprovider.Message, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		out = append(out, memprovider.Message{
			Role:    item.GetRole(),
			Content: item.GetContent(),
		})
	}
	return out
}

func memoryItemsToProto(botID string, items []memprovider.MemoryItem) []*privatev1.BotMemory {
	out := make([]*privatev1.BotMemory, 0, len(items))
	for _, item := range items {
		out = append(out, memoryItemToProto(botID, item))
	}
	return out
}

func memoryRelationsToProto(items []any) []*privatev1.MemoryRelation {
	if len(items) == 0 {
		return nil
	}
	out := make([]*privatev1.MemoryRelation, 0, len(items))
	for _, item := range items {
		out = append(out, &privatev1.MemoryRelation{Value: anyToStruct(item)})
	}
	return out
}

func anyToStruct(item any) *structpb.Struct {
	if item == nil {
		return mapToStruct(map[string]any{})
	}
	if value, ok := item.(map[string]any); ok {
		return mapToStruct(value)
	}
	payload, err := json.Marshal(item)
	if err != nil {
		return mapToStruct(map[string]any{"value": item})
	}
	var mapped map[string]any
	if err := json.Unmarshal(payload, &mapped); err == nil {
		return mapToStruct(mapped)
	}
	return mapToStruct(map[string]any{"value": item})
}

func deduplicateConnectMemoryItems(items []memprovider.MemoryItem) []memprovider.MemoryItem {
	if len(items) == 0 {
		return items
	}
	seen := make(map[string]struct{}, len(items))
	result := make([]memprovider.MemoryItem, 0, len(items))
	for _, item := range items {
		if _, ok := seen[item.ID]; ok {
			continue
		}
		seen[item.ID] = struct{}{}
		result = append(result, item)
	}
	return result
}

func memoryItemToProto(botID string, item memprovider.MemoryItem) *privatev1.BotMemory {
	if item.BotID != "" {
		botID = item.BotID
	}
	return &privatev1.BotMemory{
		Id:          item.ID,
		BotId:       botID,
		Content:     item.Memory,
		Score:       item.Score,
		Metadata:    mapToStruct(item.Metadata),
		CreatedAt:   parseMemoryTime(item.CreatedAt),
		UpdatedAt:   parseMemoryTime(item.UpdatedAt),
		Hash:        item.Hash,
		AgentId:     item.AgentID,
		RunId:       item.RunID,
		TopKBuckets: memoryTopKBucketsToProto(item.TopKBuckets),
		CdfCurve:    memoryCDFCurveToProto(item.CDFCurve),
	}
}

func memoryStatusToProto(botID string, status memprovider.MemoryStatusResponse) *privatev1.BotMemoryStatus {
	return &privatev1.BotMemoryStatus{
		BotId:             botID,
		Ready:             memoryRuntimeReady(status),
		Message:           memoryRuntimeMessage(status),
		Metadata:          mapToStruct(memoryStatusMetadata(status)),
		ProviderType:      status.ProviderType,
		MemoryMode:        status.MemoryMode,
		CanManualSync:     status.CanManualSync,
		SourceDir:         status.SourceDir,
		OverviewPath:      status.OverviewPath,
		MarkdownFileCount: int32FromInt(status.MarkdownFileCount),
		SourceCount:       int32FromInt(status.SourceCount),
		IndexedCount:      int32FromInt(status.IndexedCount),
		QdrantCollection:  status.QdrantCollection,
		Encoder:           memoryHealthStatusToProto(status.Encoder),
		Qdrant:            memoryHealthStatusToProto(status.Qdrant),
	}
}

func memoryHealthStatusToProto(status memprovider.HealthStatus) *privatev1.MemoryHealthStatus {
	return &privatev1.MemoryHealthStatus{
		Ok:    status.OK,
		Error: status.Error,
	}
}

func memoryTopKBucketsToProto(items []memprovider.TopKBucket) []*privatev1.MemoryTopKBucket {
	if len(items) == 0 {
		return nil
	}
	out := make([]*privatev1.MemoryTopKBucket, 0, len(items))
	for _, item := range items {
		out = append(out, &privatev1.MemoryTopKBucket{
			Index: item.Index,
			Value: item.Value,
		})
	}
	return out
}

func memoryCDFCurveToProto(items []memprovider.CDFPoint) []*privatev1.MemoryCDFPoint {
	if len(items) == 0 {
		return nil
	}
	out := make([]*privatev1.MemoryCDFPoint, 0, len(items))
	for _, item := range items {
		out = append(out, &privatev1.MemoryCDFPoint{
			K:          int32FromInt(item.K),
			Cumulative: item.Cumulative,
		})
	}
	return out
}

func compactResultToProto(botID string, result memprovider.CompactResult) *privatev1.CompactBotMemoryResult {
	return &privatev1.CompactBotMemoryResult{
		BeforeCount: int32FromInt(result.BeforeCount),
		AfterCount:  int32FromInt(result.AfterCount),
		Ratio:       result.Ratio,
		Memories:    memoryItemsToProto(botID, result.Results),
	}
}

func rebuildResultToProto(result memprovider.RebuildResult) *privatev1.RebuildBotMemoryResult {
	return &privatev1.RebuildBotMemoryResult{
		FsCount:       int32FromInt(result.FsCount),
		StorageCount:  int32FromInt(result.StorageCount),
		MissingCount:  int32FromInt(result.MissingCount),
		RestoredCount: int32FromInt(result.RestoredCount),
	}
}

func memoryRuntimeReady(status memprovider.MemoryStatusResponse) bool {
	return status.Encoder.Error == "" && status.Qdrant.Error == ""
}

func memoryRuntimeMessage(status memprovider.MemoryStatusResponse) string {
	if status.Encoder.Error != "" {
		return status.Encoder.Error
	}
	if status.Qdrant.Error != "" {
		return status.Qdrant.Error
	}
	return ""
}

func memoryStatusMetadata(status memprovider.MemoryStatusResponse) map[string]any {
	payload, err := json.Marshal(status)
	if err != nil {
		return map[string]any{}
	}
	var metadata map[string]any
	if err := json.Unmarshal(payload, &metadata); err != nil {
		return map[string]any{}
	}
	return metadata
}

func memoryUsageMetadata(usage memprovider.UsageResponse) map[string]any {
	return map[string]any{
		"count":                   usage.Count,
		"total_text_bytes":        usage.TotalTextBytes,
		"avg_text_bytes":          usage.AvgTextBytes,
		"estimated_storage_bytes": usage.EstimatedStorageBytes,
	}
}

func parseMemoryTime(value string) *timestamppb.Timestamp {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return nil
	}
	return timeToProto(t)
}

func toString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case error:
		return v.Error()
	default:
		return "request failed"
	}
}
