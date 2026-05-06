package connectapi

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"log/slog"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	"github.com/memohai/memoh/internal/config"
	privatev1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1"
	"github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1/privatev1connect"
	emailpkg "github.com/memohai/memoh/internal/email"
	emailgmail "github.com/memohai/memoh/internal/email/adapters/gmail"
	"github.com/memohai/memoh/internal/iam/rbac"
)

const emailOAuthCallbackPath = "/api/email/oauth/callback"

type EmailProviderService struct {
	service     *emailpkg.Service
	tokenStore  emailpkg.OAuthTokenStore
	callbackURL string
	logger      *slog.Logger
}

type EmailBindingService struct {
	service *emailpkg.Service
	manager *emailpkg.Manager
	bots    *BotService
}

type EmailOutboxService struct {
	outbox *emailpkg.OutboxService
	bots   *BotService
}

func NewEmailProviderService(log *slog.Logger, service *emailpkg.Service, tokenStore *emailpkg.DBOAuthTokenStore, cfg config.Config) *EmailProviderService {
	return &EmailProviderService{
		service:     service,
		tokenStore:  tokenStore,
		callbackURL: defaultEmailOAuthCallbackURL(cfg),
		logger:      log.With(slog.String("connect_service", "email_provider")),
	}
}

func NewEmailProviderHandler(service *EmailProviderService) Handler {
	path, handler := privatev1connect.NewEmailProviderServiceHandler(service)
	return NewHandler(path, handler)
}

func NewEmailBindingService(service *emailpkg.Service, manager *emailpkg.Manager, bots *BotService) *EmailBindingService {
	return &EmailBindingService{service: service, manager: manager, bots: bots}
}

func NewEmailBindingHandler(service *EmailBindingService) Handler {
	path, handler := privatev1connect.NewEmailBindingServiceHandler(service)
	return NewHandler(path, handler)
}

func NewEmailOutboxService(outbox *emailpkg.OutboxService, bots *BotService) *EmailOutboxService {
	return &EmailOutboxService{outbox: outbox, bots: bots}
}

func NewEmailOutboxHandler(service *EmailOutboxService) Handler {
	path, handler := privatev1connect.NewEmailOutboxServiceHandler(service)
	return NewHandler(path, handler)
}

func (s *EmailProviderService) ListEmailProviderMeta(ctx context.Context, _ *connect.Request[privatev1.ListEmailProviderMetaRequest]) (*connect.Response[privatev1.ListEmailProviderMetaResponse], error) {
	items := s.service.ListMeta(ctx)
	out := make([]*privatev1.EmailProviderMeta, 0, len(items))
	for _, item := range items {
		out = append(out, emailProviderMetaToProto(item))
	}
	return connect.NewResponse(&privatev1.ListEmailProviderMetaResponse{Providers: out}), nil
}

func (s *EmailProviderService) CreateEmailProvider(ctx context.Context, req *connect.Request[privatev1.CreateEmailProviderRequest]) (*connect.Response[privatev1.CreateEmailProviderResponse], error) {
	name := strings.TrimSpace(req.Msg.GetName())
	if name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("name is required"))
	}
	providerType := strings.TrimSpace(req.Msg.GetType())
	if providerType == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("provider is required"))
	}
	provider, err := s.service.CreateProvider(ctx, emailpkg.CreateProviderRequest{
		Name:     name,
		Provider: emailpkg.ProviderName(providerType),
		Config:   structToMap(req.Msg.GetConfig()),
	})
	if err != nil {
		return nil, emailConnectError(err)
	}
	return connect.NewResponse(&privatev1.CreateEmailProviderResponse{Provider: emailProviderToProto(provider)}), nil
}

func (s *EmailProviderService) ListEmailProviders(ctx context.Context, _ *connect.Request[privatev1.ListEmailProvidersRequest]) (*connect.Response[privatev1.ListEmailProvidersResponse], error) {
	items, err := s.service.ListProviders(ctx, "")
	if err != nil {
		return nil, emailConnectError(err)
	}
	out := make([]*privatev1.EmailProvider, 0, len(items))
	for _, item := range items {
		out = append(out, emailProviderToProto(item))
	}
	return connect.NewResponse(&privatev1.ListEmailProvidersResponse{
		Providers: out,
		Page:      &privatev1.PageResponse{},
	}), nil
}

func (s *EmailProviderService) GetEmailProvider(ctx context.Context, req *connect.Request[privatev1.GetEmailProviderRequest]) (*connect.Response[privatev1.GetEmailProviderResponse], error) {
	id := strings.TrimSpace(req.Msg.GetId())
	if id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}
	provider, err := s.service.GetProvider(ctx, id)
	if err != nil {
		return nil, emailConnectError(err)
	}
	return connect.NewResponse(&privatev1.GetEmailProviderResponse{Provider: emailProviderToProto(provider)}), nil
}

func (s *EmailProviderService) UpdateEmailProvider(ctx context.Context, req *connect.Request[privatev1.UpdateEmailProviderRequest]) (*connect.Response[privatev1.UpdateEmailProviderResponse], error) {
	id := strings.TrimSpace(req.Msg.GetId())
	if id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}
	update := emailpkg.UpdateProviderRequest{Config: structToMap(req.Msg.GetConfig())}
	if req.Msg.Name != nil {
		name := strings.TrimSpace(req.Msg.GetName())
		update.Name = &name
	}
	if req.Msg.Type != nil {
		provider := emailpkg.ProviderName(strings.TrimSpace(req.Msg.GetType()))
		update.Provider = &provider
	}
	provider, err := s.service.UpdateProvider(ctx, id, update)
	if err != nil {
		return nil, emailConnectError(err)
	}
	return connect.NewResponse(&privatev1.UpdateEmailProviderResponse{Provider: emailProviderToProto(provider)}), nil
}

func (s *EmailProviderService) DeleteEmailProvider(ctx context.Context, req *connect.Request[privatev1.DeleteEmailProviderRequest]) (*connect.Response[privatev1.DeleteEmailProviderResponse], error) {
	id := strings.TrimSpace(req.Msg.GetId())
	if id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}
	if err := s.service.DeleteProvider(ctx, id); err != nil {
		return nil, emailConnectError(err)
	}
	return connect.NewResponse(&privatev1.DeleteEmailProviderResponse{}), nil
}

func (s *EmailProviderService) StartEmailOauth(ctx context.Context, req *connect.Request[privatev1.StartEmailOauthRequest]) (*connect.Response[privatev1.StartEmailOauthResponse], error) {
	providerID := strings.TrimSpace(req.Msg.GetProviderId())
	if providerID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("provider_id is required"))
	}
	provider, err := s.service.GetProvider(ctx, providerID)
	if err != nil {
		return nil, emailConnectError(err)
	}
	if !supportsConnectEmailOAuth(emailpkg.ProviderName(provider.Provider)) {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("provider does not support OAuth2"))
	}
	callbackURL := strings.TrimSpace(req.Msg.GetRedirectUri())
	if callbackURL == "" {
		callbackURL = s.callbackURL
	}
	state, err := generateConnectEmailState(callbackURL)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if err := s.tokenStore.SetPendingState(ctx, providerID, state); err != nil {
		return nil, emailConnectError(err)
	}
	clientID, _ := provider.Config["client_id"].(string)
	if strings.TrimSpace(clientID) == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("client_id is not configured for this provider"))
	}
	adapter := emailgmail.New(s.logger, s.tokenStore)
	return connect.NewResponse(&privatev1.StartEmailOauthResponse{
		AuthorizeUrl: adapter.AuthorizeURL(clientID, callbackURL, state),
		State:        state,
	}), nil
}

func (s *EmailProviderService) GetEmailOauthStatus(ctx context.Context, req *connect.Request[privatev1.GetEmailOauthStatusRequest]) (*connect.Response[privatev1.GetEmailOauthStatusResponse], error) {
	providerID := strings.TrimSpace(req.Msg.GetProviderId())
	if providerID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("provider_id is required"))
	}
	provider, err := s.service.GetProvider(ctx, providerID)
	if err != nil {
		return nil, emailConnectError(err)
	}
	if !supportsConnectEmailOAuth(emailpkg.ProviderName(provider.Provider)) {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("provider does not support OAuth2"))
	}
	metadata := map[string]any{
		"provider":   provider.Provider,
		"configured": isConnectEmailProviderConfigured(provider),
	}
	token, err := s.tokenStore.Get(ctx, providerID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return connect.NewResponse(&privatev1.GetEmailOauthStatusResponse{Metadata: mapToStruct(metadata)}), nil
		}
		return nil, emailConnectError(err)
	}
	expired := !token.ExpiresAt.IsZero() && time.Now().After(token.ExpiresAt)
	metadata["has_token"] = token.AccessToken != ""
	metadata["expired"] = expired
	if !token.ExpiresAt.IsZero() {
		metadata["expires_at"] = token.ExpiresAt.Format(time.RFC3339)
	}
	return connect.NewResponse(&privatev1.GetEmailOauthStatusResponse{
		Authorized: token.AccessToken != "",
		Account:    token.EmailAddress,
		Metadata:   mapToStruct(metadata),
	}), nil
}

func (s *EmailProviderService) RevokeEmailOauth(ctx context.Context, req *connect.Request[privatev1.RevokeEmailOauthRequest]) (*connect.Response[privatev1.RevokeEmailOauthResponse], error) {
	providerID := strings.TrimSpace(req.Msg.GetProviderId())
	if providerID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("provider_id is required"))
	}
	provider, err := s.service.GetProvider(ctx, providerID)
	if err != nil {
		return nil, emailConnectError(err)
	}
	if !supportsConnectEmailOAuth(emailpkg.ProviderName(provider.Provider)) {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("provider does not support OAuth2"))
	}
	if err := s.tokenStore.Delete(ctx, providerID); err != nil {
		return nil, emailConnectError(err)
	}
	return connect.NewResponse(&privatev1.RevokeEmailOauthResponse{}), nil
}

func (s *EmailBindingService) CreateEmailBinding(ctx context.Context, req *connect.Request[privatev1.CreateEmailBindingRequest]) (*connect.Response[privatev1.CreateEmailBindingResponse], error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	botID := strings.TrimSpace(req.Msg.GetBotId())
	if botID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("bot_id is required"))
	}
	if err := s.bots.requireBotPermission(ctx, userID, botID, rbac.PermissionBotUpdate); err != nil {
		return nil, botConnectError(err)
	}
	config := structToMap(req.Msg.GetConfig())
	binding, err := s.service.CreateBinding(ctx, botID, emailpkg.CreateBindingRequest{
		EmailProviderID: strings.TrimSpace(req.Msg.GetProviderId()),
		EmailAddress:    strings.TrimSpace(req.Msg.GetAddress()),
		CanRead:         emailConfigBoolPtr(config, "can_read", req.Msg.GetEnabled()),
		CanWrite:        emailConfigBoolPtr(config, "can_write", req.Msg.GetEnabled()),
		CanDelete:       emailConfigBoolPtr(config, "can_delete", false),
		Config:          config,
	})
	if err != nil {
		return nil, emailConnectError(err)
	}
	if s.manager != nil {
		_ = s.manager.RefreshProvider(ctx, binding.EmailProviderID)
	}
	return connect.NewResponse(&privatev1.CreateEmailBindingResponse{Binding: emailBindingToProto(binding)}), nil
}

func (s *EmailBindingService) ListEmailBindings(ctx context.Context, req *connect.Request[privatev1.ListEmailBindingsRequest]) (*connect.Response[privatev1.ListEmailBindingsResponse], error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	botID := strings.TrimSpace(req.Msg.GetBotId())
	if botID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("bot_id is required"))
	}
	if err := s.bots.requireBotPermission(ctx, userID, botID, rbac.PermissionBotRead); err != nil {
		return nil, botConnectError(err)
	}
	items, err := s.service.ListBindings(ctx, botID)
	if err != nil {
		return nil, emailConnectError(err)
	}
	out := make([]*privatev1.EmailBinding, 0, len(items))
	for _, item := range items {
		out = append(out, emailBindingToProto(item))
	}
	return connect.NewResponse(&privatev1.ListEmailBindingsResponse{
		Bindings: out,
		Page:     &privatev1.PageResponse{},
	}), nil
}

func (s *EmailBindingService) UpdateEmailBinding(ctx context.Context, req *connect.Request[privatev1.UpdateEmailBindingRequest]) (*connect.Response[privatev1.UpdateEmailBindingResponse], error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	id := strings.TrimSpace(req.Msg.GetId())
	if id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}
	current, err := s.service.GetBinding(ctx, id)
	if err != nil {
		return nil, emailConnectError(err)
	}
	if err := s.bots.requireBotPermission(ctx, userID, current.BotID, rbac.PermissionBotUpdate); err != nil {
		return nil, botConnectError(err)
	}
	config := structToMap(req.Msg.GetConfig())
	update := emailpkg.UpdateBindingRequest{
		EmailAddress: req.Msg.Address,
		Config:       config,
	}
	if req.Msg.Enabled != nil {
		update.CanRead = req.Msg.Enabled
		update.CanWrite = req.Msg.Enabled
	}
	if canRead, ok := emailConfigBool(config, "can_read"); ok {
		update.CanRead = &canRead
	}
	if canWrite, ok := emailConfigBool(config, "can_write"); ok {
		update.CanWrite = &canWrite
	}
	if canDelete, ok := emailConfigBool(config, "can_delete"); ok {
		update.CanDelete = &canDelete
	}
	binding, err := s.service.UpdateBinding(ctx, id, update)
	if err != nil {
		return nil, emailConnectError(err)
	}
	if s.manager != nil {
		_ = s.manager.RefreshProvider(ctx, binding.EmailProviderID)
	}
	return connect.NewResponse(&privatev1.UpdateEmailBindingResponse{Binding: emailBindingToProto(binding)}), nil
}

func (s *EmailBindingService) DeleteEmailBinding(ctx context.Context, req *connect.Request[privatev1.DeleteEmailBindingRequest]) (*connect.Response[privatev1.DeleteEmailBindingResponse], error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	id := strings.TrimSpace(req.Msg.GetId())
	if id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}
	binding, err := s.service.GetBinding(ctx, id)
	if err != nil {
		return nil, emailConnectError(err)
	}
	if err := s.bots.requireBotPermission(ctx, userID, binding.BotID, rbac.PermissionBotUpdate); err != nil {
		return nil, botConnectError(err)
	}
	if err := s.service.DeleteBinding(ctx, id); err != nil {
		return nil, emailConnectError(err)
	}
	if s.manager != nil {
		_ = s.manager.RefreshProvider(ctx, binding.EmailProviderID)
	}
	return connect.NewResponse(&privatev1.DeleteEmailBindingResponse{}), nil
}

func (s *EmailOutboxService) ListEmailOutbox(ctx context.Context, req *connect.Request[privatev1.ListEmailOutboxRequest]) (*connect.Response[privatev1.ListEmailOutboxResponse], error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	botID := strings.TrimSpace(req.Msg.GetBotId())
	if botID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("bot_id is required"))
	}
	if err := s.bots.requireBotPermission(ctx, userID, botID, rbac.PermissionBotRead); err != nil {
		return nil, botConnectError(err)
	}
	limit := req.Msg.GetPage().GetPageSize()
	if limit <= 0 {
		limit = 20
	}
	items, _, err := s.outbox.ListByBot(ctx, botID, limit, 0)
	if err != nil {
		return nil, emailConnectError(err)
	}
	out := make([]*privatev1.EmailOutboxItem, 0, len(items))
	for _, item := range items {
		out = append(out, emailOutboxItemToProto(item))
	}
	return connect.NewResponse(&privatev1.ListEmailOutboxResponse{
		Items: out,
		Page:  &privatev1.PageResponse{},
	}), nil
}

func (s *EmailOutboxService) GetEmailOutboxItem(ctx context.Context, req *connect.Request[privatev1.GetEmailOutboxItemRequest]) (*connect.Response[privatev1.GetEmailOutboxItemResponse], error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	id := strings.TrimSpace(req.Msg.GetId())
	if id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}
	item, err := s.outbox.Get(ctx, id)
	if err != nil {
		return nil, emailConnectError(err)
	}
	if err := s.bots.requireBotPermission(ctx, userID, item.BotID, rbac.PermissionBotRead); err != nil {
		return nil, botConnectError(err)
	}
	return connect.NewResponse(&privatev1.GetEmailOutboxItemResponse{Item: emailOutboxItemToProto(item)}), nil
}

func emailProviderMetaToProto(meta emailpkg.ProviderMeta) *privatev1.EmailProviderMeta {
	return &privatev1.EmailProviderMeta{
		Type:        meta.Provider,
		DisplayName: meta.DisplayName,
		Schema:      valueToStruct(meta.ConfigSchema),
	}
}

func emailProviderToProto(provider emailpkg.ProviderResponse) *privatev1.EmailProvider {
	return &privatev1.EmailProvider{
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

func emailBindingToProto(binding emailpkg.BindingResponse) *privatev1.EmailBinding {
	config := cloneAnyMap(binding.Config)
	config["can_read"] = binding.CanRead
	config["can_write"] = binding.CanWrite
	config["can_delete"] = binding.CanDelete
	return &privatev1.EmailBinding{
		Id:         binding.ID,
		BotId:      binding.BotID,
		ProviderId: binding.EmailProviderID,
		Address:    binding.EmailAddress,
		Enabled:    binding.CanRead || binding.CanWrite,
		Config:     mapToStruct(config),
		Audit: &privatev1.AuditFields{
			CreatedAt: timeToProto(binding.CreatedAt),
			UpdatedAt: timeToProto(binding.UpdatedAt),
		},
	}
}

func emailOutboxItemToProto(item emailpkg.OutboxItemResponse) *privatev1.EmailOutboxItem {
	return &privatev1.EmailOutboxItem{
		Id:         item.ID,
		BotId:      item.BotID,
		ProviderId: item.ProviderID,
		To:         strings.Join(item.To, ", "),
		Subject:    item.Subject,
		Status:     item.Status,
		Error:      item.Error,
		SentAt:     timeToProto(item.SentAt),
		Audit: &privatev1.AuditFields{
			CreatedAt: timeToProto(item.CreatedAt),
		},
	}
}

func emailConnectError(err error) error {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return connect.NewError(connect.CodeNotFound, err)
	case strings.Contains(err.Error(), "not found"):
		return connect.NewError(connect.CodeNotFound, err)
	case strings.Contains(err.Error(), "invalid") || strings.Contains(err.Error(), "required") || strings.Contains(err.Error(), "unsupported"):
		return connect.NewError(connect.CodeInvalidArgument, err)
	default:
		return connect.NewError(connect.CodeInternal, err)
	}
}

func defaultEmailOAuthCallbackURL(cfg config.Config) string {
	addr := strings.TrimSpace(cfg.Server.Addr)
	if addr == "" {
		addr = ":8080"
	}
	host := addr
	if strings.HasPrefix(host, ":") {
		host = "localhost" + host
	}
	return "http://" + host + emailOAuthCallbackPath
}

func generateConnectEmailState(callbackURL string) (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	state := hex.EncodeToString(b)
	if callbackURL == "" {
		return state, nil
	}
	return state + "." + base64.RawURLEncoding.EncodeToString([]byte(callbackURL)), nil
}

func supportsConnectEmailOAuth(name emailpkg.ProviderName) bool {
	return name == emailgmail.ProviderName
}

func isConnectEmailProviderConfigured(provider emailpkg.ProviderResponse) bool {
	if emailpkg.ProviderName(provider.Provider) != emailgmail.ProviderName {
		return false
	}
	clientID, _ := provider.Config["client_id"].(string)
	return strings.TrimSpace(clientID) != ""
}

func emailConfigBoolPtr(config map[string]any, key string, fallback bool) *bool {
	value, ok := emailConfigBool(config, key)
	if !ok {
		value = fallback
	}
	return &value
}

func emailConfigBool(config map[string]any, key string) (bool, bool) {
	value, ok := config[key]
	if !ok {
		return false, false
	}
	typed, ok := value.(bool)
	return typed, ok
}

func cloneAnyMap(value map[string]any) map[string]any {
	out := make(map[string]any, len(value))
	for key, item := range value {
		out[key] = item
	}
	return out
}
