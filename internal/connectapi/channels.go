package connectapi

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"connectrpc.com/connect"

	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/channel"
	privatev1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1"
	"github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1/privatev1connect"
	"github.com/memohai/memoh/internal/iam/rbac"
)

type ChannelService struct {
	store     *channel.Store
	registry  *channel.Registry
	lifecycle *channel.Lifecycle
	bots      *bots.Service
}

func NewChannelService(store *channel.Store, registry *channel.Registry, lifecycle *channel.Lifecycle, bots *bots.Service) *ChannelService {
	return &ChannelService{store: store, registry: registry, lifecycle: lifecycle, bots: bots}
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
	if s.bots == nil {
		return errors.New("bot service not configured")
	}
	allowed, err := s.bots.HasBotPermission(ctx, userID, strings.TrimSpace(botID), permission)
	if err != nil {
		return err
	}
	if !allowed {
		return bots.ErrBotAccessDenied
	}
	return nil
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
