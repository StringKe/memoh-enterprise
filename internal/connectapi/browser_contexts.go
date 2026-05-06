package connectapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/memohai/memoh/internal/browsercontexts"
	"github.com/memohai/memoh/internal/config"
	privatev1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1"
	"github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1/privatev1connect"
)

type BrowserContextService struct {
	contexts          *browsercontexts.Service
	logger            *slog.Logger
	browserGatewayURL string
	httpClient        *http.Client
}

func NewBrowserContextService(log *slog.Logger, service *browsercontexts.Service, cfg config.Config) *BrowserContextService {
	if log == nil {
		log = slog.Default()
	}
	return &BrowserContextService{
		contexts:          service,
		logger:            log.With(slog.String("service", "connect_browser_contexts")),
		browserGatewayURL: cfg.BrowserGateway.BaseURL(),
		httpClient:        http.DefaultClient,
	}
}

func NewBrowserContextHandler(service *BrowserContextService) Handler {
	path, handler := privatev1connect.NewBrowserContextServiceHandler(service)
	return NewHandler(path, handler)
}

func (s *BrowserContextService) CreateBrowserContext(ctx context.Context, req *connect.Request[privatev1.CreateBrowserContextRequest]) (*connect.Response[privatev1.CreateBrowserContextResponse], error) {
	var enabled *bool
	if req.Msg.GetEnabled() {
		value := true
		enabled = &value
	}
	config, err := browserContextRequestConfig(req.Msg.GetConfig(), req.Msg.GetCore(), enabled)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	item, err := s.contexts.Create(ctx, browsercontexts.CreateRequest{
		Name:   req.Msg.GetName(),
		Config: config,
	})
	if err != nil {
		return nil, browserContextConnectError(err)
	}
	return connect.NewResponse(&privatev1.CreateBrowserContextResponse{Context: browserContextToProto(item)}), nil
}

func (s *BrowserContextService) ListBrowserContexts(ctx context.Context, _ *connect.Request[privatev1.ListBrowserContextsRequest]) (*connect.Response[privatev1.ListBrowserContextsResponse], error) {
	items, err := s.contexts.List(ctx)
	if err != nil {
		return nil, browserContextConnectError(err)
	}
	response := &privatev1.ListBrowserContextsResponse{
		Contexts: make([]*privatev1.BrowserContext, 0, len(items)),
		Page:     &privatev1.PageResponse{},
	}
	for _, item := range items {
		response.Contexts = append(response.Contexts, browserContextToProto(item))
	}
	return connect.NewResponse(response), nil
}

func (s *BrowserContextService) GetBrowserContext(ctx context.Context, req *connect.Request[privatev1.GetBrowserContextRequest]) (*connect.Response[privatev1.GetBrowserContextResponse], error) {
	id := strings.TrimSpace(req.Msg.GetId())
	if id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}
	item, err := s.contexts.GetByID(ctx, id)
	if err != nil {
		return nil, browserContextConnectError(err)
	}
	return connect.NewResponse(&privatev1.GetBrowserContextResponse{Context: browserContextToProto(item)}), nil
}

func (s *BrowserContextService) UpdateBrowserContext(ctx context.Context, req *connect.Request[privatev1.UpdateBrowserContextRequest]) (*connect.Response[privatev1.UpdateBrowserContextResponse], error) {
	id := strings.TrimSpace(req.Msg.GetId())
	if id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}
	config, err := browserContextRequestConfig(req.Msg.GetConfig(), optionalRequestString(req.Msg.Core), req.Msg.Enabled)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	item, err := s.contexts.Update(ctx, id, browsercontexts.UpdateRequest{
		Name:   optionalRequestString(req.Msg.Name),
		Config: config,
	})
	if err != nil {
		return nil, browserContextConnectError(err)
	}
	return connect.NewResponse(&privatev1.UpdateBrowserContextResponse{Context: browserContextToProto(item)}), nil
}

func (s *BrowserContextService) DeleteBrowserContext(ctx context.Context, req *connect.Request[privatev1.DeleteBrowserContextRequest]) (*connect.Response[privatev1.DeleteBrowserContextResponse], error) {
	id := strings.TrimSpace(req.Msg.GetId())
	if id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}
	if err := s.contexts.Delete(ctx, id); err != nil {
		return nil, browserContextConnectError(err)
	}
	return connect.NewResponse(&privatev1.DeleteBrowserContextResponse{}), nil
}

func (s *BrowserContextService) ListBrowserCores(ctx context.Context, _ *connect.Request[privatev1.ListBrowserCoresRequest]) (*connect.Response[privatev1.ListBrowserCoresResponse], error) {
	url := fmt.Sprintf("%s/cores/", strings.TrimRight(s.browserGatewayURL, "/"))
	httpClient := s.httpClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("failed to create request"))
	}
	resp, err := httpClient.Do(req) //nolint:gosec // URL is from trusted internal config
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("browser gateway unreachable", slog.String("error", err.Error()))
		}
		return connect.NewResponse(&privatev1.ListBrowserCoresResponse{Cores: browserCoresToProto([]string{"chromium"})}), nil
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("failed to read browser gateway response"))
	}
	var result struct {
		Cores []string `json:"cores"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("failed to parse browser gateway response"))
	}
	return connect.NewResponse(&privatev1.ListBrowserCoresResponse{Cores: browserCoresToProto(result.Cores)}), nil
}

func browserContextConnectError(err error) error {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return connect.NewError(connect.CodeNotFound, err)
	case strings.Contains(err.Error(), "invalid UUID"):
		return connect.NewError(connect.CodeInvalidArgument, err)
	case strings.Contains(err.Error(), "not valid JSON"):
		return connect.NewError(connect.CodeInvalidArgument, err)
	default:
		return connect.NewError(connect.CodeInternal, err)
	}
}

func browserContextToProto(item browsercontexts.BrowserContext) *privatev1.BrowserContext {
	config := rawConfigToStruct(item.Config)
	configMap := structToMap(config)
	core, _ := configMap["core"].(string)
	enabled, ok := configMap["enabled"].(bool)
	if !ok {
		enabled = true
	}
	return &privatev1.BrowserContext{
		Id:      item.ID,
		Name:    item.Name,
		Core:    core,
		Enabled: enabled,
		Config:  config,
		Audit: &privatev1.AuditFields{
			CreatedAt: browserContextTimeToProto(item.CreatedAt),
			UpdatedAt: browserContextTimeToProto(item.UpdatedAt),
		},
	}
}

func browserContextRequestConfig(config *structpb.Struct, core string, enabled *bool) (json.RawMessage, error) {
	configMap := structToMap(config)
	if configMap == nil {
		configMap = map[string]any{}
	}
	if strings.TrimSpace(core) != "" {
		configMap["core"] = strings.TrimSpace(core)
	}
	if enabled != nil {
		configMap["enabled"] = *enabled
	}
	data, err := json.Marshal(configMap)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(data), nil
}

func rawConfigToStruct(raw json.RawMessage) *structpb.Struct {
	if len(raw) == 0 {
		return mapToStruct(nil)
	}
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		return mapToStruct(nil)
	}
	return mapToStruct(value)
}

func browserContextTimeToProto(value string) *timestamppb.Timestamp {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil
	}
	return timestamppb.New(parsed)
}

func browserCoresToProto(cores []string) []*privatev1.BrowserCore {
	result := make([]*privatev1.BrowserCore, 0, len(cores))
	for _, core := range cores {
		core = strings.TrimSpace(core)
		if core == "" {
			continue
		}
		result = append(result, &privatev1.BrowserCore{
			Id:          core,
			DisplayName: core,
			Metadata:    mapToStruct(nil),
		})
	}
	return result
}

func optionalRequestString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
