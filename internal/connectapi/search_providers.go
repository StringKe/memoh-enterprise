package connectapi

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/types/known/structpb"

	privatev1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1"
	"github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1/privatev1connect"
	"github.com/memohai/memoh/internal/searchproviders"
)

type SearchProviderService struct {
	providers *searchproviders.Service
}

func NewSearchProviderService(providers *searchproviders.Service) *SearchProviderService {
	return &SearchProviderService{providers: providers}
}

func NewSearchProviderHandler(service *SearchProviderService) Handler {
	path, handler := privatev1connect.NewSearchProviderServiceHandler(service)
	return NewHandler(path, handler)
}

func (s *SearchProviderService) ListSearchProviderMeta(ctx context.Context, _ *connect.Request[privatev1.ListSearchProviderMetaRequest]) (*connect.Response[privatev1.ListSearchProviderMetaResponse], error) {
	items := s.providers.ListMeta(ctx)
	out := make([]*privatev1.SearchProviderMeta, 0, len(items))
	for _, item := range items {
		out = append(out, searchProviderMetaToProto(item))
	}
	return connect.NewResponse(&privatev1.ListSearchProviderMetaResponse{Providers: out}), nil
}

func (s *SearchProviderService) CreateSearchProvider(ctx context.Context, req *connect.Request[privatev1.CreateSearchProviderRequest]) (*connect.Response[privatev1.CreateSearchProviderResponse], error) {
	if strings.TrimSpace(req.Msg.GetName()) == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("name is required"))
	}
	if strings.TrimSpace(req.Msg.GetType()) == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("provider is required"))
	}
	provider, err := s.providers.Create(ctx, searchproviders.CreateRequest{
		Name:     req.Msg.GetName(),
		Provider: searchproviders.ProviderName(req.Msg.GetType()),
		Config:   structToMap(req.Msg.GetConfig()),
	})
	if err != nil {
		return nil, searchProviderConnectError(err)
	}
	return connect.NewResponse(&privatev1.CreateSearchProviderResponse{Provider: searchProviderToProto(provider)}), nil
}

func (s *SearchProviderService) ListSearchProviders(ctx context.Context, _ *connect.Request[privatev1.ListSearchProvidersRequest]) (*connect.Response[privatev1.ListSearchProvidersResponse], error) {
	items, err := s.providers.List(ctx, "")
	if err != nil {
		return nil, searchProviderConnectError(err)
	}
	out := make([]*privatev1.SearchProvider, 0, len(items))
	for _, item := range items {
		out = append(out, searchProviderToProto(item))
	}
	return connect.NewResponse(&privatev1.ListSearchProvidersResponse{
		Providers: out,
		Page:      &privatev1.PageResponse{},
	}), nil
}

func (s *SearchProviderService) GetSearchProvider(ctx context.Context, req *connect.Request[privatev1.GetSearchProviderRequest]) (*connect.Response[privatev1.GetSearchProviderResponse], error) {
	id := strings.TrimSpace(req.Msg.GetId())
	if id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}
	provider, err := s.providers.Get(ctx, id)
	if err != nil {
		return nil, searchProviderConnectError(err)
	}
	return connect.NewResponse(&privatev1.GetSearchProviderResponse{Provider: searchProviderToProto(provider)}), nil
}

func (s *SearchProviderService) UpdateSearchProvider(ctx context.Context, req *connect.Request[privatev1.UpdateSearchProviderRequest]) (*connect.Response[privatev1.UpdateSearchProviderResponse], error) {
	id := strings.TrimSpace(req.Msg.GetId())
	if id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}
	update := searchproviders.UpdateRequest{
		Name:   req.Msg.Name,
		Config: structToMap(req.Msg.GetConfig()),
		Enable: req.Msg.Enabled,
	}
	if req.Msg.Type != nil {
		provider := searchproviders.ProviderName(req.Msg.GetType())
		update.Provider = &provider
	}
	provider, err := s.providers.Update(ctx, id, update)
	if err != nil {
		return nil, searchProviderConnectError(err)
	}
	return connect.NewResponse(&privatev1.UpdateSearchProviderResponse{Provider: searchProviderToProto(provider)}), nil
}

func (s *SearchProviderService) DeleteSearchProvider(ctx context.Context, req *connect.Request[privatev1.DeleteSearchProviderRequest]) (*connect.Response[privatev1.DeleteSearchProviderResponse], error) {
	id := strings.TrimSpace(req.Msg.GetId())
	if id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}
	if err := s.providers.Delete(ctx, id); err != nil {
		return nil, searchProviderConnectError(err)
	}
	return connect.NewResponse(&privatev1.DeleteSearchProviderResponse{}), nil
}

func searchProviderMetaToProto(meta searchproviders.ProviderMeta) *privatev1.SearchProviderMeta {
	return &privatev1.SearchProviderMeta{
		Type:        meta.Provider,
		DisplayName: meta.DisplayName,
		Schema:      searchProviderSchemaToStruct(meta.ConfigSchema),
		Defaults:    mapToStruct(map[string]any{}),
	}
}

func searchProviderSchemaToStruct(schema searchproviders.ProviderConfigSchema) *structpb.Struct {
	var value map[string]any
	payload, err := json.Marshal(schema)
	if err != nil {
		return mapToStruct(map[string]any{})
	}
	if err := json.Unmarshal(payload, &value); err != nil {
		return mapToStruct(map[string]any{})
	}
	return mapToStruct(value)
}

func searchProviderToProto(provider searchproviders.GetResponse) *privatev1.SearchProvider {
	return &privatev1.SearchProvider{
		Id:      provider.ID,
		Name:    provider.Name,
		Type:    provider.Provider,
		Enabled: provider.Enable,
		Config:  mapToStruct(provider.Config),
		Audit: &privatev1.AuditFields{
			CreatedAt: timeToProto(provider.CreatedAt),
			UpdatedAt: timeToProto(provider.UpdatedAt),
		},
	}
}

func searchProviderConnectError(err error) error {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return connect.NewError(connect.CodeNotFound, err)
	default:
		return connectError(err)
	}
}
