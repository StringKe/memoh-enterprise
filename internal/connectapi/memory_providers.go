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
	memprovider "github.com/memohai/memoh/internal/memory/adapters"
)

type memoryProviderAdmin interface {
	ListMeta(ctx context.Context) []memprovider.ProviderMeta
	Create(ctx context.Context, req memprovider.ProviderCreateRequest) (memprovider.ProviderGetResponse, error)
	List(ctx context.Context) ([]memprovider.ProviderGetResponse, error)
	Get(ctx context.Context, id string) (memprovider.ProviderGetResponse, error)
	Update(ctx context.Context, id string, req memprovider.ProviderUpdateRequest) (memprovider.ProviderGetResponse, error)
	Delete(ctx context.Context, id string) error
	Status(ctx context.Context, id string) (memprovider.ProviderStatusResponse, error)
}

type MemoryProviderService struct {
	providers memoryProviderAdmin
}

func NewMemoryProviderService(providers *memprovider.Service) *MemoryProviderService {
	return &MemoryProviderService{providers: providers}
}

func NewMemoryProviderHandler(service *MemoryProviderService) Handler {
	path, handler := privatev1connect.NewMemoryProviderServiceHandler(service)
	return NewHandler(path, handler)
}

func (s *MemoryProviderService) ListMemoryProviderMeta(ctx context.Context, _ *connect.Request[privatev1.ListMemoryProviderMetaRequest]) (*connect.Response[privatev1.ListMemoryProviderMetaResponse], error) {
	items := s.providers.ListMeta(ctx)
	out := make([]*privatev1.MemoryProviderMeta, 0, len(items))
	for _, item := range items {
		out = append(out, memoryProviderMetaToProto(item))
	}
	return connect.NewResponse(&privatev1.ListMemoryProviderMetaResponse{Providers: out}), nil
}

func (s *MemoryProviderService) CreateMemoryProvider(ctx context.Context, req *connect.Request[privatev1.CreateMemoryProviderRequest]) (*connect.Response[privatev1.CreateMemoryProviderResponse], error) {
	if strings.TrimSpace(req.Msg.GetName()) == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("name is required"))
	}
	if strings.TrimSpace(req.Msg.GetType()) == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("provider is required"))
	}
	provider, err := s.providers.Create(ctx, memprovider.ProviderCreateRequest{
		Name:     req.Msg.GetName(),
		Provider: memprovider.ProviderType(req.Msg.GetType()),
		Config:   structToMap(req.Msg.GetConfig()),
	})
	if err != nil {
		return nil, memoryProviderConnectError(err)
	}
	return connect.NewResponse(&privatev1.CreateMemoryProviderResponse{Provider: memoryProviderToProto(provider)}), nil
}

func (s *MemoryProviderService) ListMemoryProviders(ctx context.Context, _ *connect.Request[privatev1.ListMemoryProvidersRequest]) (*connect.Response[privatev1.ListMemoryProvidersResponse], error) {
	items, err := s.providers.List(ctx)
	if err != nil {
		return nil, memoryProviderConnectError(err)
	}
	out := make([]*privatev1.MemoryProvider, 0, len(items))
	for _, item := range items {
		out = append(out, memoryProviderToProto(item))
	}
	return connect.NewResponse(&privatev1.ListMemoryProvidersResponse{
		Providers: out,
		Page:      &privatev1.PageResponse{},
	}), nil
}

func (s *MemoryProviderService) GetMemoryProvider(ctx context.Context, req *connect.Request[privatev1.GetMemoryProviderRequest]) (*connect.Response[privatev1.GetMemoryProviderResponse], error) {
	id := strings.TrimSpace(req.Msg.GetId())
	if id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}
	provider, err := s.providers.Get(ctx, id)
	if err != nil {
		return nil, memoryProviderConnectError(err)
	}
	return connect.NewResponse(&privatev1.GetMemoryProviderResponse{Provider: memoryProviderToProto(provider)}), nil
}

func (s *MemoryProviderService) UpdateMemoryProvider(ctx context.Context, req *connect.Request[privatev1.UpdateMemoryProviderRequest]) (*connect.Response[privatev1.UpdateMemoryProviderResponse], error) {
	id := strings.TrimSpace(req.Msg.GetId())
	if id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}
	provider, err := s.providers.Update(ctx, id, memprovider.ProviderUpdateRequest{
		Name:   req.Msg.Name,
		Config: structToMap(req.Msg.GetConfig()),
	})
	if err != nil {
		return nil, memoryProviderConnectError(err)
	}
	return connect.NewResponse(&privatev1.UpdateMemoryProviderResponse{Provider: memoryProviderToProto(provider)}), nil
}

func (s *MemoryProviderService) DeleteMemoryProvider(ctx context.Context, req *connect.Request[privatev1.DeleteMemoryProviderRequest]) (*connect.Response[privatev1.DeleteMemoryProviderResponse], error) {
	id := strings.TrimSpace(req.Msg.GetId())
	if id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}
	if err := s.providers.Delete(ctx, id); err != nil {
		return nil, memoryProviderConnectError(err)
	}
	return connect.NewResponse(&privatev1.DeleteMemoryProviderResponse{}), nil
}

func (s *MemoryProviderService) GetMemoryProviderStatus(ctx context.Context, req *connect.Request[privatev1.GetMemoryProviderStatusRequest]) (*connect.Response[privatev1.GetMemoryProviderStatusResponse], error) {
	id := strings.TrimSpace(req.Msg.GetId())
	if id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}
	status, err := s.providers.Status(ctx, id)
	if err != nil {
		return nil, memoryProviderConnectError(err)
	}
	return connect.NewResponse(&privatev1.GetMemoryProviderStatusResponse{
		Status: memoryProviderStatusToProto(id, status),
	}), nil
}

func memoryProviderMetaToProto(meta memprovider.ProviderMeta) *privatev1.MemoryProviderMeta {
	return &privatev1.MemoryProviderMeta{
		Type:        meta.Provider,
		DisplayName: meta.DisplayName,
		Schema:      memoryProviderSchemaToStruct(meta.ConfigSchema),
		Defaults:    mapToStruct(map[string]any{}),
	}
}

func memoryProviderSchemaToStruct(schema memprovider.ProviderConfigSchema) *structpb.Struct {
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

func memoryProviderToProto(provider memprovider.ProviderGetResponse) *privatev1.MemoryProvider {
	return &privatev1.MemoryProvider{
		Id:      provider.ID,
		Name:    provider.Name,
		Type:    provider.Provider,
		Enabled: true,
		Config:  mapToStruct(provider.Config),
		Audit: &privatev1.AuditFields{
			CreatedAt: timeToProto(provider.CreatedAt),
			UpdatedAt: timeToProto(provider.UpdatedAt),
		},
	}
}

func memoryProviderStatusToProto(id string, status memprovider.ProviderStatusResponse) *privatev1.MemoryProviderStatus {
	return &privatev1.MemoryProviderStatus{
		Id:       id,
		Ok:       memoryProviderStatusOK(status),
		Metadata: mapToStruct(memoryProviderStatusMetadata(status)),
	}
}

func memoryProviderStatusOK(status memprovider.ProviderStatusResponse) bool {
	for _, collection := range status.Collections {
		if !collection.Qdrant.OK {
			return false
		}
	}
	return true
}

func memoryProviderStatusMetadata(status memprovider.ProviderStatusResponse) map[string]any {
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

func memoryProviderConnectError(err error) error {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return connect.NewError(connect.CodeNotFound, err)
	default:
		return connectError(err)
	}
}
