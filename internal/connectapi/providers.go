package connectapi

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/types/known/structpb"

	privatev1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1"
	"github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1/privatev1connect"
	"github.com/memohai/memoh/internal/models"
	"github.com/memohai/memoh/internal/oauthctx"
	"github.com/memohai/memoh/internal/providers"
)

type ProviderService struct {
	providers *providers.Service
	models    *models.Service
}

func NewProviderService(providerService *providers.Service, modelsService *models.Service) *ProviderService {
	return &ProviderService{
		providers: providerService,
		models:    modelsService,
	}
}

func NewProviderHandler(service *ProviderService) Handler {
	path, handler := privatev1connect.NewProviderServiceHandler(service)
	return NewHandler(path, handler)
}

func (s *ProviderService) CreateProvider(ctx context.Context, req *connect.Request[privatev1.CreateProviderRequest]) (*connect.Response[privatev1.CreateProviderResponse], error) {
	name := strings.TrimSpace(req.Msg.GetName())
	if name == "" {
		name = strings.TrimSpace(req.Msg.GetDisplayName())
	}
	if name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("name is required"))
	}

	provider, err := s.providers.Create(ctx, providers.CreateRequest{
		Name:       name,
		ClientType: strings.TrimSpace(req.Msg.GetClientType()),
		Config:     providerConfigFromCreate(req.Msg),
	})
	if err != nil {
		return nil, providerConnectError(err)
	}
	return connect.NewResponse(&privatev1.CreateProviderResponse{Provider: providerToProto(provider)}), nil
}

func (s *ProviderService) ListProviders(ctx context.Context, _ *connect.Request[privatev1.ListProvidersRequest]) (*connect.Response[privatev1.ListProvidersResponse], error) {
	items, err := s.providers.List(ctx)
	if err != nil {
		return nil, providerConnectError(err)
	}
	out := make([]*privatev1.Provider, 0, len(items))
	for _, item := range items {
		out = append(out, providerToProto(item))
	}
	return connect.NewResponse(&privatev1.ListProvidersResponse{
		Providers: out,
		Page:      &privatev1.PageResponse{},
	}), nil
}

func (s *ProviderService) GetProvider(ctx context.Context, req *connect.Request[privatev1.GetProviderRequest]) (*connect.Response[privatev1.GetProviderResponse], error) {
	id := strings.TrimSpace(req.Msg.GetId())
	name := strings.TrimSpace(req.Msg.GetName())
	if id == "" && name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id or name is required"))
	}

	var (
		provider providers.GetResponse
		err      error
	)
	if id != "" {
		provider, err = s.providers.Get(ctx, id)
	} else {
		provider, err = s.providers.GetByName(ctx, name)
	}
	if err != nil {
		return nil, providerConnectError(err)
	}
	return connect.NewResponse(&privatev1.GetProviderResponse{Provider: providerToProto(provider)}), nil
}

func (s *ProviderService) UpdateProvider(ctx context.Context, req *connect.Request[privatev1.UpdateProviderRequest]) (*connect.Response[privatev1.UpdateProviderResponse], error) {
	id := strings.TrimSpace(req.Msg.GetId())
	if id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}

	update := providers.UpdateRequest{
		Config: providerConfigFromUpdate(req.Msg),
	}
	if req.Msg.DisplayName != nil {
		name := strings.TrimSpace(req.Msg.GetDisplayName())
		update.Name = &name
	}
	if req.Msg.ClientType != nil {
		clientType := strings.TrimSpace(req.Msg.GetClientType())
		update.ClientType = &clientType
	}
	if req.Msg.Enabled != nil {
		update.Enable = req.Msg.Enabled
	}

	provider, err := s.providers.Update(ctx, id, update)
	if err != nil {
		return nil, providerConnectError(err)
	}
	return connect.NewResponse(&privatev1.UpdateProviderResponse{Provider: providerToProto(provider)}), nil
}

func (s *ProviderService) DeleteProvider(ctx context.Context, req *connect.Request[privatev1.DeleteProviderRequest]) (*connect.Response[privatev1.DeleteProviderResponse], error) {
	id := strings.TrimSpace(req.Msg.GetId())
	if id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}
	if err := s.providers.Delete(ctx, id); err != nil {
		return nil, providerConnectError(err)
	}
	return connect.NewResponse(&privatev1.DeleteProviderResponse{}), nil
}

func (s *ProviderService) CountProviders(ctx context.Context, _ *connect.Request[privatev1.CountProvidersRequest]) (*connect.Response[privatev1.CountProvidersResponse], error) {
	count, err := s.providers.Count(ctx)
	if err != nil {
		return nil, providerConnectError(err)
	}
	return connect.NewResponse(&privatev1.CountProvidersResponse{Count: count}), nil
}

func (s *ProviderService) ListProviderModels(ctx context.Context, req *connect.Request[privatev1.ListProviderModelsRequest]) (*connect.Response[privatev1.ListProviderModelsResponse], error) {
	if s.models == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("models service not configured"))
	}
	id := strings.TrimSpace(req.Msg.GetProviderId())
	if id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("provider_id is required"))
	}
	items, err := s.models.ListByProviderID(ctx, id)
	if err != nil {
		return nil, providerConnectError(err)
	}
	out := make([]*privatev1.ProviderModelSummary, 0, len(items))
	for _, item := range items {
		out = append(out, providerModelToProto(item))
	}
	return connect.NewResponse(&privatev1.ListProviderModelsResponse{
		Models: out,
		Page:   &privatev1.PageResponse{},
	}), nil
}

func (s *ProviderService) TestProvider(ctx context.Context, req *connect.Request[privatev1.TestProviderRequest]) (*connect.Response[privatev1.TestProviderResponse], error) {
	id := strings.TrimSpace(req.Msg.GetId())
	if id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}
	result, err := s.providers.Test(providerOAuthContext(ctx), id)
	if err != nil {
		return nil, providerConnectError(err)
	}
	return connect.NewResponse(&privatev1.TestProviderResponse{
		Ok:      result.Reachable,
		Message: result.Message,
		Metadata: mapToStruct(map[string]any{
			"latency_ms": result.LatencyMs,
		}),
	}), nil
}

func (s *ProviderService) ImportProviderModels(ctx context.Context, req *connect.Request[privatev1.ImportProviderModelsRequest]) (*connect.Response[privatev1.ImportProviderModelsResponse], error) {
	if s.models == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("models service not configured"))
	}
	id := strings.TrimSpace(req.Msg.GetId())
	if id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}

	provider, err := s.providers.Get(ctx, id)
	if err != nil {
		return nil, providerConnectError(fmt.Errorf("provider not found: %w", err))
	}
	if !models.IsLLMClientType(models.ClientType(provider.ClientType)) {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("import models is not supported for speech providers"))
	}

	remoteModels, err := s.providers.FetchRemoteModels(providerOAuthContext(ctx), id)
	if err != nil {
		return nil, providerConnectError(fmt.Errorf("fetch remote models: %w", err))
	}

	allowed := stringSet(req.Msg.GetModelIds())
	out := make([]*privatev1.ProviderModelSummary, 0, len(remoteModels))
	for _, remote := range remoteModels {
		if len(allowed) > 0 {
			if _, ok := allowed[remote.ID]; !ok {
				continue
			}
		}
		summary := remoteProviderModelToProto(id, remote)
		if !req.Msg.GetDryRun() {
			if _, err := s.models.Create(ctx, remoteModelCreateRequest(id, remote)); err != nil {
				if errors.Is(err, models.ErrModelIDAlreadyExists) {
					continue
				}
				return nil, providerConnectError(err)
			}
		}
		out = append(out, summary)
	}

	return connect.NewResponse(&privatev1.ImportProviderModelsResponse{Models: out}), nil
}

func providerConfigFromCreate(req *privatev1.CreateProviderRequest) map[string]any {
	config := structToMap(req.GetConfig())
	if config == nil {
		config = map[string]any{}
	}
	if baseURL := strings.TrimSpace(req.GetBaseUrl()); baseURL != "" {
		config["base_url"] = baseURL
	}
	if apiKey := strings.TrimSpace(req.GetApiKey()); apiKey != "" {
		config["api_key"] = apiKey
	}
	return config
}

func providerConfigFromUpdate(req *privatev1.UpdateProviderRequest) map[string]any {
	config := structToMap(req.GetConfig())
	if config == nil {
		config = map[string]any{}
	}
	if req.BaseUrl != nil {
		config["base_url"] = strings.TrimSpace(req.GetBaseUrl())
	}
	if req.ApiKey != nil {
		config["api_key"] = strings.TrimSpace(req.GetApiKey())
	}
	if len(config) == 0 {
		return nil
	}
	return config
}

func providerToProto(provider providers.GetResponse) *privatev1.Provider {
	config := provider.Config
	baseURL, _ := config["base_url"].(string)
	return &privatev1.Provider{
		Id:          provider.ID,
		Name:        provider.Name,
		DisplayName: provider.Name,
		BaseUrl:     baseURL,
		ClientType:  provider.ClientType,
		Enabled:     provider.Enable,
		Config:      mapToStruct(config),
		Audit: &privatev1.AuditFields{
			CreatedAt: timeToProto(provider.CreatedAt),
			UpdatedAt: timeToProto(provider.UpdatedAt),
		},
	}
}

func providerModelToProto(model models.GetResponse) *privatev1.ProviderModelSummary {
	return &privatev1.ProviderModelSummary{
		Id:          model.ID,
		ProviderId:  model.ProviderID,
		ModelId:     model.ModelID,
		DisplayName: firstNonEmptyString(model.Name, model.ModelID),
		Type:        string(model.Type),
		Modalities:  append([]string(nil), model.Config.Compatibilities...),
		Metadata:    modelConfigToStruct(model.Config),
	}
}

func remoteProviderModelToProto(providerID string, model providers.RemoteModel) *privatev1.ProviderModelSummary {
	modelType := strings.TrimSpace(model.Type)
	if modelType == "" {
		modelType = string(models.ModelTypeChat)
	}
	return &privatev1.ProviderModelSummary{
		ProviderId:  providerID,
		ModelId:     model.ID,
		DisplayName: firstNonEmptyString(model.DisplayName, model.Name, model.ID),
		Type:        modelType,
		Modalities:  append([]string(nil), model.Compatibilities...),
		Metadata: mapToStruct(map[string]any{
			"object":            model.Object,
			"created":           model.Created,
			"owned_by":          model.OwnedBy,
			"reasoning_efforts": model.ReasoningEfforts,
		}),
	}
}

func remoteModelCreateRequest(providerID string, remote providers.RemoteModel) models.AddRequest {
	modelType := models.ModelTypeChat
	if strings.TrimSpace(remote.Type) == string(models.ModelTypeEmbedding) {
		modelType = models.ModelTypeEmbedding
	}
	compatibilities := remote.Compatibilities
	if len(compatibilities) == 0 {
		compatibilities = []string{models.CompatVision, models.CompatToolCall, models.CompatReasoning}
	}
	return models.AddRequest{
		ModelID:    remote.ID,
		Name:       firstNonEmptyString(remote.Name, remote.DisplayName, remote.ID),
		ProviderID: providerID,
		Type:       modelType,
		Config: models.ModelConfig{
			Compatibilities:  compatibilities,
			ReasoningEfforts: remote.ReasoningEfforts,
		},
	}
}

func modelConfigToStruct(config models.ModelConfig) *structpb.Struct {
	value := map[string]any{
		"compatibilities":   config.Compatibilities,
		"reasoning_efforts": config.ReasoningEfforts,
	}
	if config.Dimensions != nil {
		value["dimensions"] = *config.Dimensions
	}
	if config.ContextWindow != nil {
		value["context_window"] = *config.ContextWindow
	}
	return mapToStruct(value)
}

func providerOAuthContext(ctx context.Context) context.Context {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return ctx
	}
	return oauthctx.WithUserID(ctx, userID)
}

func providerConnectError(err error) error {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return connect.NewError(connect.CodeNotFound, err)
	case errors.Is(err, models.ErrModelIDAlreadyExists):
		return connect.NewError(connect.CodeAlreadyExists, err)
	case errors.Is(err, models.ErrModelIDAmbiguous):
		return connect.NewError(connect.CodeFailedPrecondition, err)
	case strings.Contains(err.Error(), "invalid") || strings.Contains(err.Error(), "required") || strings.Contains(err.Error(), "not supported"):
		return connect.NewError(connect.CodeInvalidArgument, err)
	default:
		return connect.NewError(connect.CodeInternal, err)
	}
}

func stringSet(values []string) map[string]struct{} {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out[value] = struct{}{}
		}
	}
	return out
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
