package connectapi

import (
	"context"
	"errors"
	"strings"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/types/known/structpb"

	privatev1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1"
	"github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1/privatev1connect"
	"github.com/memohai/memoh/internal/models"
)

type ModelService struct {
	models *models.Service
}

func NewModelService(models *models.Service) *ModelService {
	return &ModelService{models: models}
}

func NewModelHandler(service *ModelService) Handler {
	path, handler := privatev1connect.NewModelServiceHandler(service)
	return NewHandler(path, handler)
}

func (s *ModelService) CreateModel(ctx context.Context, req *connect.Request[privatev1.CreateModelRequest]) (*connect.Response[privatev1.CreateModelResponse], error) {
	model, err := s.models.Create(ctx, models.AddRequest{
		ModelID:    strings.TrimSpace(req.Msg.GetModelId()),
		Name:       strings.TrimSpace(req.Msg.GetDisplayName()),
		ProviderID: strings.TrimSpace(req.Msg.GetProviderId()),
		Type:       models.ModelType(strings.TrimSpace(req.Msg.GetType())),
		Config:     modelConfigFromCreateRequest(req.Msg),
	})
	if err != nil {
		return nil, modelConnectError(err)
	}
	created, err := s.models.GetByID(ctx, model.ID)
	if err != nil {
		return nil, modelConnectError(err)
	}
	return connect.NewResponse(&privatev1.CreateModelResponse{Model: modelToProto(created)}), nil
}

func (s *ModelService) ListModels(ctx context.Context, req *connect.Request[privatev1.ListModelsRequest]) (*connect.Response[privatev1.ListModelsResponse], error) {
	providerID := strings.TrimSpace(req.Msg.GetProviderId())
	modelType := models.ModelType(strings.TrimSpace(req.Msg.GetType()))

	var items []models.GetResponse
	var err error
	switch {
	case providerID != "" && modelType != "":
		items, err = s.models.ListByProviderIDAndType(ctx, providerID, modelType)
	case providerID != "":
		items, err = s.models.ListByProviderID(ctx, providerID)
	case modelType != "":
		items, err = s.models.ListEnabledByType(ctx, modelType)
	default:
		items, err = s.models.ListEnabled(ctx)
	}
	if err != nil {
		return nil, modelConnectError(err)
	}

	out := make([]*privatev1.Model, 0, len(items))
	for _, item := range items {
		out = append(out, modelToProto(item))
	}
	return connect.NewResponse(&privatev1.ListModelsResponse{
		Models: out,
		Page:   &privatev1.PageResponse{},
	}), nil
}

func (s *ModelService) GetModel(ctx context.Context, req *connect.Request[privatev1.GetModelRequest]) (*connect.Response[privatev1.GetModelResponse], error) {
	model, err := s.getModel(ctx, req.Msg.GetId(), req.Msg.GetModelId())
	if err != nil {
		return nil, modelConnectError(err)
	}
	return connect.NewResponse(&privatev1.GetModelResponse{Model: modelToProto(model)}), nil
}

func (s *ModelService) UpdateModel(ctx context.Context, req *connect.Request[privatev1.UpdateModelRequest]) (*connect.Response[privatev1.UpdateModelResponse], error) {
	update := models.UpdateRequest{
		ModelID: strings.TrimSpace(req.Msg.GetModelId()),
		Type:    models.ModelType(strings.TrimSpace(req.Msg.GetType())),
		Config:  modelConfigFromUpdateRequest(req.Msg),
	}
	if req.Msg.DisplayName != nil {
		update.Name = strings.TrimSpace(req.Msg.GetDisplayName())
	}

	var model models.GetResponse
	var err error
	id := strings.TrimSpace(req.Msg.GetId())
	modelID := strings.TrimSpace(req.Msg.GetModelId())
	switch {
	case id != "":
		current, currentErr := s.models.GetByID(ctx, id)
		if currentErr != nil {
			return nil, modelConnectError(currentErr)
		}
		applyModelUpdateDefaults(&update, current)
		model, err = s.models.UpdateByID(ctx, id, update)
	case modelID != "":
		current, currentErr := s.models.GetByModelID(ctx, modelID)
		if currentErr != nil {
			return nil, modelConnectError(currentErr)
		}
		applyModelUpdateDefaults(&update, current)
		model, err = s.models.UpdateByModelID(ctx, modelID, update)
	default:
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id or model_id is required"))
	}
	if err != nil {
		return nil, modelConnectError(err)
	}
	return connect.NewResponse(&privatev1.UpdateModelResponse{Model: modelToProto(model)}), nil
}

func (s *ModelService) DeleteModel(ctx context.Context, req *connect.Request[privatev1.DeleteModelRequest]) (*connect.Response[privatev1.DeleteModelResponse], error) {
	id := strings.TrimSpace(req.Msg.GetId())
	modelID := strings.TrimSpace(req.Msg.GetModelId())
	switch {
	case id != "":
		if err := s.models.DeleteByID(ctx, id); err != nil {
			return nil, modelConnectError(err)
		}
	case modelID != "":
		if err := s.models.DeleteByModelID(ctx, modelID); err != nil {
			return nil, modelConnectError(err)
		}
	default:
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id or model_id is required"))
	}
	return connect.NewResponse(&privatev1.DeleteModelResponse{}), nil
}

func (s *ModelService) CountModels(ctx context.Context, req *connect.Request[privatev1.CountModelsRequest]) (*connect.Response[privatev1.CountModelsResponse], error) {
	providerID := strings.TrimSpace(req.Msg.GetProviderId())
	modelType := models.ModelType(strings.TrimSpace(req.Msg.GetType()))
	if providerID != "" {
		var items []models.GetResponse
		var err error
		if modelType != "" {
			items, err = s.models.ListByProviderIDAndType(ctx, providerID, modelType)
		} else {
			items, err = s.models.ListByProviderID(ctx, providerID)
		}
		if err != nil {
			return nil, modelConnectError(err)
		}
		return connect.NewResponse(&privatev1.CountModelsResponse{Count: int64(len(items))}), nil
	}

	var count int64
	var err error
	if modelType != "" {
		count, err = s.models.CountByType(ctx, modelType)
	} else {
		count, err = s.models.Count(ctx)
	}
	if err != nil {
		return nil, modelConnectError(err)
	}
	return connect.NewResponse(&privatev1.CountModelsResponse{Count: count}), nil
}

func (s *ModelService) TestModel(ctx context.Context, req *connect.Request[privatev1.TestModelRequest]) (*connect.Response[privatev1.TestModelResponse], error) {
	id := strings.TrimSpace(req.Msg.GetId())
	if id == "" {
		model, err := s.getModel(ctx, "", req.Msg.GetModelId())
		if err != nil {
			return nil, modelConnectError(err)
		}
		id = model.ID
	}
	result, err := s.models.Test(ctx, id)
	if err != nil {
		return nil, modelTestConnectError(err)
	}
	return connect.NewResponse(&privatev1.TestModelResponse{
		Ok:       result.Status == models.TestStatusOK,
		Message:  result.Message,
		Metadata: mapToStruct(modelTestMetadata(result)),
	}), nil
}

func (s *ModelService) GetModelCapabilities(ctx context.Context, req *connect.Request[privatev1.GetModelCapabilitiesRequest]) (*connect.Response[privatev1.GetModelCapabilitiesResponse], error) {
	model, err := s.getModel(ctx, req.Msg.GetId(), req.Msg.GetModelId())
	if err != nil {
		return nil, modelConnectError(err)
	}
	return connect.NewResponse(&privatev1.GetModelCapabilitiesResponse{
		Capabilities: modelCapabilitiesToProto(model),
	}), nil
}

func (s *ModelService) getModel(ctx context.Context, id string, modelID string) (models.GetResponse, error) {
	id = strings.TrimSpace(id)
	modelID = strings.TrimSpace(modelID)
	switch {
	case id != "":
		return s.models.GetByID(ctx, id)
	case modelID != "":
		return s.models.GetByModelID(ctx, modelID)
	default:
		return models.GetResponse{}, connect.NewError(connect.CodeInvalidArgument, errors.New("id or model_id is required"))
	}
}

func modelConfigFromCreateRequest(req *privatev1.CreateModelRequest) models.ModelConfig {
	config := modelConfigFromMap(structToMap(req.GetMetadata()))
	applyModelRequestCapabilities(&config, req.GetModalities(), structToMap(req.GetReasoning()))
	return config
}

func modelConfigFromUpdateRequest(req *privatev1.UpdateModelRequest) models.ModelConfig {
	config := modelConfigFromMap(structToMap(req.GetMetadata()))
	applyModelRequestCapabilities(&config, req.GetModalities(), structToMap(req.GetReasoning()))
	return config
}

func modelConfigFromMap(value map[string]any) models.ModelConfig {
	if value == nil {
		return models.ModelConfig{}
	}
	config := models.ModelConfig{
		Compatibilities:  stringSliceFromAny(value["compatibilities"]),
		ReasoningEfforts: stringSliceFromAny(value["reasoning_efforts"]),
	}
	if dimensions := positiveIntFromAny(value["dimensions"]); dimensions != nil {
		config.Dimensions = dimensions
	}
	if contextWindow := positiveIntFromAny(value["context_window"]); contextWindow != nil {
		config.ContextWindow = contextWindow
	}
	return config
}

func applyModelRequestCapabilities(config *models.ModelConfig, modalities []string, reasoning map[string]any) {
	if len(modalities) > 0 {
		config.Compatibilities = normalizedModelCapabilities(modalities)
	}
	if len(reasoning) == 0 {
		return
	}
	if efforts := stringSliceFromAny(reasoning["reasoning_efforts"]); len(efforts) > 0 {
		config.ReasoningEfforts = efforts
		return
	}
	if efforts := stringSliceFromAny(reasoning["efforts"]); len(efforts) > 0 {
		config.ReasoningEfforts = efforts
	}
}

func applyModelUpdateDefaults(update *models.UpdateRequest, current models.GetResponse) {
	if update.ModelID == "" {
		update.ModelID = current.ModelID
	}
	if update.ProviderID == "" {
		update.ProviderID = current.ProviderID
	}
	if update.Type == "" {
		update.Type = current.Type
	}
	if update.Name == "" {
		update.Name = current.Name
	}
	if modelConfigIsEmpty(update.Config) {
		update.Config = current.Config
	}
}

func modelToProto(model models.GetResponse) *privatev1.Model {
	config := modelConfigToMap(model.Config)
	return &privatev1.Model{
		Id:          model.ID,
		ProviderId:  model.ProviderID,
		ModelId:     model.ModelID,
		DisplayName: model.Name,
		Type:        string(model.Type),
		Modalities:  append([]string(nil), model.Config.Compatibilities...),
		Enabled:     true,
		Reasoning:   modelReasoningToStruct(model.Config),
		Metadata:    mapToStruct(config),
		Audit:       &privatev1.AuditFields{},
	}
}

func modelCapabilitiesToProto(model models.GetResponse) *privatev1.ModelCapabilities {
	return &privatev1.ModelCapabilities{
		ModelId:           model.ModelID,
		Modalities:        append([]string(nil), model.Config.Compatibilities...),
		SupportsTools:     modelHasCompatibility(model, models.CompatToolCall),
		SupportsVision:    modelHasCompatibility(model, models.CompatVision),
		SupportsReasoning: modelHasCompatibility(model, models.CompatReasoning),
		SupportsStreaming: string(model.Type) == string(models.ModelTypeChat),
		Metadata:          mapToStruct(modelConfigToMap(model.Config)),
	}
}

func modelReasoningToStruct(config models.ModelConfig) *structpb.Struct {
	if len(config.ReasoningEfforts) == 0 {
		return mapToStruct(map[string]any{})
	}
	return mapToStruct(map[string]any{"reasoning_efforts": stringSliceToAny(config.ReasoningEfforts)})
}

func modelConfigToMap(config models.ModelConfig) map[string]any {
	out := map[string]any{}
	if config.Dimensions != nil {
		out["dimensions"] = *config.Dimensions
	}
	if len(config.Compatibilities) > 0 {
		out["compatibilities"] = stringSliceToAny(config.Compatibilities)
	}
	if config.ContextWindow != nil {
		out["context_window"] = *config.ContextWindow
	}
	if len(config.ReasoningEfforts) > 0 {
		out["reasoning_efforts"] = stringSliceToAny(config.ReasoningEfforts)
	}
	return out
}

func stringSliceToAny(values []string) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}

func modelConfigIsEmpty(config models.ModelConfig) bool {
	return config.Dimensions == nil &&
		len(config.Compatibilities) == 0 &&
		config.ContextWindow == nil &&
		len(config.ReasoningEfforts) == 0
}

func normalizedModelCapabilities(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func stringSliceFromAny(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
				out = append(out, strings.TrimSpace(text))
			}
		}
		return out
	default:
		return nil
	}
}

func positiveIntFromAny(value any) *int {
	var out int
	switch typed := value.(type) {
	case int:
		out = typed
	case int32:
		out = int(typed)
	case int64:
		out = int(typed)
	case float64:
		out = int(typed)
	default:
		return nil
	}
	if out <= 0 {
		return nil
	}
	return &out
}

func modelHasCompatibility(model models.GetResponse, capability string) bool {
	for _, value := range model.Config.Compatibilities {
		if value == capability {
			return true
		}
	}
	return false
}

func modelTestMetadata(result models.TestResponse) map[string]any {
	return map[string]any{
		"status":     string(result.Status),
		"reachable":  result.Reachable,
		"latency_ms": result.LatencyMs,
	}
}

func modelConnectError(err error) error {
	var connectErr *connect.Error
	if errors.As(err, &connectErr) {
		return connectErr
	}
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return connect.NewError(connect.CodeNotFound, err)
	case errors.Is(err, models.ErrModelIDAlreadyExists), errors.Is(err, models.ErrModelIDAmbiguous):
		return connect.NewError(connect.CodeAlreadyExists, err)
	case strings.Contains(err.Error(), "required"), strings.Contains(err.Error(), "invalid"), strings.Contains(err.Error(), "validation failed"):
		return connect.NewError(connect.CodeInvalidArgument, err)
	default:
		return connect.NewError(connect.CodeInternal, err)
	}
}

func modelTestConnectError(err error) error {
	if strings.Contains(err.Error(), "invalid") {
		return connect.NewError(connect.CodeInvalidArgument, err)
	}
	return modelConnectError(err)
}
