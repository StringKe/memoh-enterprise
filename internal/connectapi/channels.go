package connectapi

import (
	"context"
	"encoding/base64"
	"errors"
	"sort"
	"strings"
	"time"

	"connectrpc.com/connect"

	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/channel/adapters/weixin"
	privatev1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1"
	"github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1/privatev1connect"
	"github.com/memohai/memoh/internal/iam/rbac"
)

type ChannelService struct {
	store       *channel.Store
	registry    *channel.Registry
	lifecycle   *channel.Lifecycle
	bots        *bots.Service
	permissions channelBotPermissionChecker
	weixinQR    weixinQRClient
}

func NewChannelService(store *channel.Store, registry *channel.Registry, lifecycle *channel.Lifecycle, bots *bots.Service) *ChannelService {
	return &ChannelService{
		store:       store,
		registry:    registry,
		lifecycle:   lifecycle,
		bots:        bots,
		permissions: bots,
		weixinQR:    weixin.NewClient(nil),
	}
}

type channelBotPermissionChecker interface {
	HasBotPermission(ctx context.Context, userID, botID string, permission rbac.PermissionKey) (bool, error)
}

type weixinQRClient interface {
	FetchQRCode(ctx context.Context, apiBaseURL string) (*weixin.QRCodeResponse, error)
	PollQRStatus(ctx context.Context, apiBaseURL, qrcode string) (*weixin.QRStatusResponse, error)
}

const weixinQRBaseURL = "https://ilinkai.weixin.qq.com"

func (s *ChannelService) SetWeixinQRClient(client weixinQRClient) {
	s.weixinQR = client
}

func NewChannelHandler(service *ChannelService) Handler {
	path, handler := privatev1connect.NewChannelServiceHandler(service)
	return NewHandler(path, handler)
}

func (s *ChannelService) ListChannels(ctx context.Context, _ *connect.Request[privatev1.ListChannelsRequest]) (*connect.Response[privatev1.ListChannelsResponse], error) {
	if _, err := UserIDFromContext(ctx); err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	if s.registry == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("channel registry not configured"))
	}
	descs := s.registry.ListDescriptors()
	sort.Slice(descs, func(i, j int) bool {
		return descs[i].Type.String() < descs[j].Type.String()
	})
	items := make([]*privatev1.Channel, 0, len(descs))
	for _, desc := range descs {
		items = append(items, channelDescriptorToProto(s.registry, desc))
	}
	return connect.NewResponse(&privatev1.ListChannelsResponse{Channels: items}), nil
}

func (s *ChannelService) GetChannel(ctx context.Context, req *connect.Request[privatev1.GetChannelRequest]) (*connect.Response[privatev1.GetChannelResponse], error) {
	if _, err := UserIDFromContext(ctx); err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	if s.registry == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("channel registry not configured"))
	}
	channelType, err := s.registry.ParseChannelType(req.Msg.GetId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	desc, ok := s.registry.GetDescriptor(channelType)
	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("channel not found"))
	}
	return connect.NewResponse(&privatev1.GetChannelResponse{Channel: channelDescriptorToProto(s.registry, desc)}), nil
}

func (s *ChannelService) GetChannelIdentityConfig(ctx context.Context, req *connect.Request[privatev1.GetChannelIdentityConfigRequest]) (*connect.Response[privatev1.GetChannelIdentityConfigResponse], error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	identityID, err := channelIdentityIDForRequest(userID, req.Msg.GetIdentityId())
	if err != nil {
		return nil, connect.NewError(connect.CodePermissionDenied, err)
	}
	channelType, err := s.parseChannelType(req.Msg.GetChannel())
	if err != nil {
		return nil, err
	}
	item, err := s.store.GetChannelIdentityConfig(ctx, identityID, channelType)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&privatev1.GetChannelIdentityConfigResponse{Config: channelIdentityConfigToProto(item)}), nil
}

func (s *ChannelService) UpsertChannelIdentityConfig(ctx context.Context, req *connect.Request[privatev1.UpsertChannelIdentityConfigRequest]) (*connect.Response[privatev1.UpsertChannelIdentityConfigResponse], error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	identityID, err := channelIdentityIDForRequest(userID, req.Msg.GetIdentityId())
	if err != nil {
		return nil, connect.NewError(connect.CodePermissionDenied, err)
	}
	channelType, err := s.parseChannelType(req.Msg.GetChannel())
	if err != nil {
		return nil, err
	}
	item, err := s.store.UpsertChannelIdentityConfig(ctx, identityID, channelType, channel.UpsertChannelIdentityConfigRequest{
		Config: structToMap(req.Msg.GetConfig()),
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&privatev1.UpsertChannelIdentityConfigResponse{Config: channelIdentityConfigToProto(item)}), nil
}

func (s *ChannelService) GetBotChannelConfig(ctx context.Context, req *connect.Request[privatev1.GetBotChannelConfigRequest]) (*connect.Response[privatev1.GetBotChannelConfigResponse], error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	if err := s.requireBotPermission(ctx, userID, req.Msg.GetBotId(), rbac.PermissionBotRead); err != nil {
		return nil, botConnectError(err)
	}
	channelType, err := s.parseChannelType(req.Msg.GetChannel())
	if err != nil {
		return nil, err
	}
	item, err := s.store.ResolveEffectiveConfig(ctx, strings.TrimSpace(req.Msg.GetBotId()), channelType)
	if err != nil {
		if errors.Is(err, channel.ErrChannelConfigNotFound) || strings.Contains(err.Error(), "not found") {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&privatev1.GetBotChannelConfigResponse{Config: botChannelConfigToProto(item)}), nil
}

func (s *ChannelService) UpsertBotChannelConfig(ctx context.Context, req *connect.Request[privatev1.UpsertBotChannelConfigRequest]) (*connect.Response[privatev1.UpsertBotChannelConfigResponse], error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	if err := s.requireBotPermission(ctx, userID, req.Msg.GetBotId(), rbac.PermissionBotUpdate); err != nil {
		return nil, botConnectError(err)
	}
	channelType, err := s.parseChannelType(req.Msg.GetChannel())
	if err != nil {
		return nil, err
	}
	if s.lifecycle == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("channel lifecycle not configured"))
	}
	channelReq := channel.UpsertConfigRequest{
		Credentials:      structToMap(req.Msg.GetCredentials()),
		ExternalIdentity: strings.TrimSpace(req.Msg.GetExternalIdentity()),
		SelfIdentity:     structToMap(req.Msg.GetSelfIdentity()),
		Routing:          structToMap(req.Msg.GetRouting()),
		Disabled:         req.Msg.Disabled,
		VerifiedAt:       timestampPtr(req.Msg.GetVerifiedAt()),
	}
	if channelReq.Credentials == nil {
		channelReq.Credentials = map[string]any{}
	}
	item, err := s.lifecycle.UpsertBotChannelConfig(ctx, strings.TrimSpace(req.Msg.GetBotId()), channelType, channelReq)
	if err != nil {
		if errors.Is(err, channel.ErrEnableChannelFailed) {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&privatev1.UpsertBotChannelConfigResponse{Config: botChannelConfigToProto(item)}), nil
}

func (s *ChannelService) UpdateBotChannelStatus(ctx context.Context, req *connect.Request[privatev1.UpdateBotChannelStatusRequest]) (*connect.Response[privatev1.UpdateBotChannelStatusResponse], error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	if err := s.requireBotPermission(ctx, userID, req.Msg.GetBotId(), rbac.PermissionBotUpdate); err != nil {
		return nil, botConnectError(err)
	}
	channelType, err := s.parseChannelType(req.Msg.GetChannel())
	if err != nil {
		return nil, err
	}
	if s.lifecycle == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("channel lifecycle not configured"))
	}
	item, err := s.lifecycle.SetBotChannelStatus(ctx, strings.TrimSpace(req.Msg.GetBotId()), channelType, req.Msg.GetDisabled())
	if err != nil {
		if errors.Is(err, channel.ErrChannelConfigNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		if errors.Is(err, channel.ErrEnableChannelFailed) {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&privatev1.UpdateBotChannelStatusResponse{Config: botChannelConfigToProto(item)}), nil
}

func (s *ChannelService) DeleteBotChannelConfig(ctx context.Context, req *connect.Request[privatev1.DeleteBotChannelConfigRequest]) (*connect.Response[privatev1.DeleteBotChannelConfigResponse], error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	if err := s.requireBotPermission(ctx, userID, req.Msg.GetBotId(), rbac.PermissionBotUpdate); err != nil {
		return nil, botConnectError(err)
	}
	channelType, err := s.parseChannelType(req.Msg.GetChannel())
	if err != nil {
		return nil, err
	}
	if s.lifecycle == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("channel lifecycle not configured"))
	}
	if err := s.lifecycle.DeleteBotChannelConfig(ctx, strings.TrimSpace(req.Msg.GetBotId()), channelType); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&privatev1.DeleteBotChannelConfigResponse{}), nil
}

func (s *ChannelService) StartChannelQrLogin(ctx context.Context, req *connect.Request[privatev1.StartChannelQrLoginRequest]) (*connect.Response[privatev1.StartChannelQrLoginResponse], error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	if err := s.requireBotPermission(ctx, userID, req.Msg.GetBotId(), rbac.PermissionBotUpdate); err != nil {
		return nil, botConnectError(err)
	}
	channelType, err := s.parseChannelType(req.Msg.GetChannel())
	if err != nil {
		return nil, err
	}
	if channelType != channel.ChannelTypeWeixin {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("channel does not support QR login"))
	}
	if s.weixinQR == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("weixin QR client is not configured"))
	}
	qr, err := s.weixinQR.FetchQRCode(ctx, weixinQRBaseURL)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if qr == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("weixin QR login returned empty response"))
	}
	loginID := strings.TrimSpace(qr.QRCode)
	qrURL := strings.TrimSpace(qr.QRCode)
	image, mimeType := decodeQRCodeImage(qr.QRCodeImgContent)
	if loginID == "" {
		loginID = strings.TrimSpace(qr.QRCodeImgContent)
	}
	if qrURL == "" {
		qrURL = loginID
	}
	if loginID == "" {
		return nil, connect.NewError(connect.CodeInternal, errors.New("weixin QR login did not return a login id"))
	}
	return connect.NewResponse(&privatev1.StartChannelQrLoginResponse{
		LoginId:   loginID,
		QrUrl:     qrURL,
		QrImage:   image,
		MimeType:  mimeType,
		ExpiresAt: timeToProto(time.Now().UTC().Add(2 * time.Minute)),
	}), nil
}

func (s *ChannelService) PollChannelQrLogin(ctx context.Context, req *connect.Request[privatev1.PollChannelQrLoginRequest]) (*connect.Response[privatev1.PollChannelQrLoginResponse], error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	if err := s.requireBotPermission(ctx, userID, req.Msg.GetBotId(), rbac.PermissionBotUpdate); err != nil {
		return nil, botConnectError(err)
	}
	channelType, err := s.parseChannelType(req.Msg.GetChannel())
	if err != nil {
		return nil, err
	}
	if channelType != channel.ChannelTypeWeixin {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("channel does not support QR login"))
	}
	if strings.TrimSpace(req.Msg.GetLoginId()) == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("login_id is required"))
	}
	if s.weixinQR == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("weixin QR client is not configured"))
	}
	status, err := s.weixinQR.PollQRStatus(ctx, weixinQRBaseURL, strings.TrimSpace(req.Msg.GetLoginId()))
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if status == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("weixin QR polling returned empty response"))
	}
	statusValue := strings.TrimSpace(status.Status)
	if statusValue == "" {
		statusValue = "wait"
	}
	resp := &privatev1.PollChannelQrLoginResponse{
		Status: statusValue,
		Metadata: mapToStruct(map[string]any{
			"ilink_bot_id":  strings.TrimSpace(status.ILinkBotID),
			"ilink_user_id": strings.TrimSpace(status.ILinkUserID),
			"base_url":      strings.TrimSpace(status.BaseURL),
		}),
	}
	if statusValue == "confirmed" && strings.TrimSpace(status.BotToken) != "" {
		if s.lifecycle == nil {
			return nil, connect.NewError(connect.CodeInternal, errors.New("channel lifecycle not configured"))
		}
		baseURL := strings.TrimSpace(status.BaseURL)
		if baseURL == "" {
			baseURL = weixinQRBaseURL
		}
		disabled := false
		cfg, err := s.lifecycle.UpsertBotChannelConfig(ctx, strings.TrimSpace(req.Msg.GetBotId()), channelType, channel.UpsertConfigRequest{
			Credentials: map[string]any{
				"token":   strings.TrimSpace(status.BotToken),
				"baseUrl": baseURL,
			},
			Disabled: &disabled,
		})
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		resp.Config = botChannelConfigToProto(cfg)
	}
	return connect.NewResponse(resp), nil
}

func (s *ChannelService) parseChannelType(raw string) (channel.ChannelType, error) {
	if s.registry == nil {
		return "", connect.NewError(connect.CodeInternal, errors.New("channel registry not configured"))
	}
	channelType, err := s.registry.ParseChannelType(raw)
	if err != nil {
		return "", connect.NewError(connect.CodeInvalidArgument, err)
	}
	return channelType, nil
}

func (s *ChannelService) requireBotPermission(ctx context.Context, userID, botID string, permission rbac.PermissionKey) error {
	checker := s.permissions
	if checker == nil {
		checker = s.bots
	}
	if checker == nil {
		return errors.New("bot service not configured")
	}
	allowed, err := checker.HasBotPermission(ctx, userID, strings.TrimSpace(botID), permission)
	if err != nil {
		return err
	}
	if !allowed {
		return bots.ErrBotAccessDenied
	}
	return nil
}

func decodeQRCodeImage(raw string) ([]byte, string) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		return nil, ""
	}
	mimeType := "image/png"
	if prefix, body, ok := strings.Cut(raw, ","); ok && strings.Contains(prefix, ";base64") {
		raw = strings.TrimSpace(body)
		if strings.HasPrefix(prefix, "data:") {
			mimeType = strings.TrimPrefix(strings.TrimSuffix(prefix, ";base64"), "data:")
		}
	}
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(raw)
	}
	if err != nil || len(decoded) == 0 {
		return nil, ""
	}
	return decoded, mimeType
}

func channelIdentityIDForRequest(userID, requested string) (string, error) {
	requested = strings.TrimSpace(requested)
	if requested == "" || requested == userID {
		return userID, nil
	}
	return "", errors.New("channel identity id must match current user")
}

func channelDescriptorToProto(registry *channel.Registry, desc channel.Descriptor) *privatev1.Channel {
	_, supportsWebhook := registry.GetWebhookReceiver(desc.Type)
	metadata := map[string]any{
		"configless":         desc.Configless,
		"capabilities":       desc.Capabilities,
		"config_schema":      desc.ConfigSchema,
		"user_config_schema": desc.UserConfigSchema,
		"target_spec":        desc.TargetSpec,
	}
	return &privatev1.Channel{
		Id:                     desc.Type.String(),
		DisplayName:            desc.DisplayName,
		SupportsWebhook:        supportsWebhook,
		SupportsIdentityConfig: true,
		Metadata:               mapToStruct(metadata),
	}
}

func channelIdentityConfigToProto(value channel.ChannelIdentityBinding) *privatev1.ChannelIdentityConfig {
	return &privatev1.ChannelIdentityConfig{
		Channel:    value.ChannelType.String(),
		IdentityId: value.ChannelIdentityID,
		Config:     mapToStruct(value.Config),
		Audit: &privatev1.AuditFields{
			CreatedAt: timeToProto(value.CreatedAt),
			UpdatedAt: timeToProto(value.UpdatedAt),
		},
	}
}

func botChannelConfigToProto(value channel.ChannelConfig) *privatev1.BotChannelConfig {
	return &privatev1.BotChannelConfig{
		Id:               value.ID,
		BotId:            value.BotID,
		Channel:          value.ChannelType.String(),
		Credentials:      mapToStruct(value.Credentials),
		ExternalIdentity: value.ExternalIdentity,
		SelfIdentity:     mapToStruct(value.SelfIdentity),
		Routing:          mapToStruct(value.Routing),
		Disabled:         value.Disabled,
		VerifiedAt:       timeToProto(value.VerifiedAt),
		Audit: &privatev1.AuditFields{
			CreatedAt: timeToProto(value.CreatedAt),
			UpdatedAt: timeToProto(value.UpdatedAt),
		},
	}
}

func timestampPtr(value interface{ AsTime() time.Time }) *time.Time {
	if value == nil {
		return nil
	}
	t := value.AsTime()
	if t.IsZero() {
		return nil
	}
	return &t
}
