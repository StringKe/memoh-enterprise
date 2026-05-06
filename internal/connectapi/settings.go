package connectapi

import (
	"context"
	"encoding/json"
	"errors"

	"connectrpc.com/connect"

	privatev1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1"
	"github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1/privatev1connect"
	"github.com/memohai/memoh/internal/heartbeat"
	"github.com/memohai/memoh/internal/iam/rbac"
	settingspkg "github.com/memohai/memoh/internal/settings"
)

type SettingsService struct {
	settings  *settingspkg.Service
	bots      *BotService
	heartbeat *heartbeat.Service
}

func NewSettingsService(settings *settingspkg.Service, bots *BotService, heartbeat *heartbeat.Service) *SettingsService {
	return &SettingsService{settings: settings, bots: bots, heartbeat: heartbeat}
}

func NewSettingsHandler(service *SettingsService) Handler {
	path, handler := privatev1connect.NewSettingsServiceHandler(service)
	return NewHandler(path, handler)
}

func (s *SettingsService) GetBotSettings(ctx context.Context, req *connect.Request[privatev1.GetBotSettingsRequest]) (*connect.Response[privatev1.GetBotSettingsResponse], error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	if err := s.bots.requireBotPermission(ctx, userID, req.Msg.GetBotId(), rbac.PermissionBotRead); err != nil {
		return nil, botConnectError(err)
	}
	settings, err := s.settings.GetBot(ctx, req.Msg.GetBotId())
	if err != nil {
		return nil, settingsConnectError(err)
	}
	return connect.NewResponse(&privatev1.GetBotSettingsResponse{Settings: effectiveSettingsToProto(settings)}), nil
}

func (s *SettingsService) UpdateBotSettings(ctx context.Context, req *connect.Request[privatev1.UpdateBotSettingsRequest]) (*connect.Response[privatev1.UpdateBotSettingsResponse], error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	if err := s.bots.requireBotPermission(ctx, userID, req.Msg.GetBotId(), rbac.PermissionBotUpdate); err != nil {
		return nil, botConnectError(err)
	}
	update, err := upsertRequestFromProto(req.Msg.GetSettings(), req.Msg.GetOverrideMask())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	settings, err := s.settings.UpsertBot(ctx, req.Msg.GetBotId(), update)
	if err != nil {
		return nil, settingsConnectError(err)
	}
	if update.HeartbeatEnabled != nil || update.HeartbeatInterval != nil {
		if err := s.heartbeat.Reschedule(ctx, req.Msg.GetBotId()); err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
	}
	return connect.NewResponse(&privatev1.UpdateBotSettingsResponse{Settings: effectiveSettingsToProto(settings)}), nil
}

func (s *SettingsService) DeleteBotSettings(ctx context.Context, req *connect.Request[privatev1.DeleteBotSettingsRequest]) (*connect.Response[privatev1.DeleteBotSettingsResponse], error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	if err := s.bots.requireBotPermission(ctx, userID, req.Msg.GetBotId(), rbac.PermissionBotUpdate); err != nil {
		return nil, botConnectError(err)
	}
	if err := s.settings.Delete(ctx, req.Msg.GetBotId()); err != nil {
		return nil, settingsConnectError(err)
	}
	return connect.NewResponse(&privatev1.DeleteBotSettingsResponse{}), nil
}

func (s *SettingsService) RestoreBotSettingsInheritance(ctx context.Context, req *connect.Request[privatev1.RestoreBotSettingsInheritanceRequest]) (*connect.Response[privatev1.RestoreBotSettingsInheritanceResponse], error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	if err := s.bots.requireBotPermission(ctx, userID, req.Msg.GetBotId(), rbac.PermissionBotUpdate); err != nil {
		return nil, botConnectError(err)
	}
	settings, err := s.settings.RestoreInheritance(ctx, req.Msg.GetBotId(), req.Msg.GetFields())
	if err != nil {
		return nil, settingsConnectError(err)
	}
	return connect.NewResponse(&privatev1.RestoreBotSettingsInheritanceResponse{Settings: effectiveSettingsToProto(settings)}), nil
}

func upsertRequestFromProto(value *privatev1.BotSettings, mask *privatev1.SettingsOverrideMask) (settingspkg.UpsertRequest, error) {
	if value == nil {
		value = &privatev1.BotSettings{}
	}
	req := settingspkg.UpsertRequest{
		ChatModelID:            value.ChatModelId,
		ImageModelID:           value.ImageModelId,
		SearchProviderID:       value.SearchProviderId,
		MemoryProviderID:       value.MemoryProviderId,
		TtsModelID:             value.TtsModelId,
		TranscriptionModelID:   value.TranscriptionModelId,
		BrowserContextID:       value.BrowserContextId,
		Language:               value.Language,
		AclDefaultEffect:       value.AclDefaultEffect,
		Timezone:               value.Timezone,
		ReasoningEnabled:       value.ReasoningEnabled,
		ReasoningEffort:        value.ReasoningEffort,
		HeartbeatEnabled:       value.HeartbeatEnabled,
		HeartbeatInterval:      intPtrFromInt32(value.HeartbeatInterval),
		HeartbeatPrompt:        value.HeartbeatPrompt,
		HeartbeatModelID:       value.HeartbeatModelId,
		TitleModelID:           value.TitleModelId,
		CompactionEnabled:      value.CompactionEnabled,
		CompactionThreshold:    intPtrFromInt32(value.CompactionThreshold),
		CompactionRatio:        intPtrFromInt32(value.CompactionRatio),
		CompactionModelID:      value.CompactionModelId,
		DiscussProbeModelID:    value.DiscussProbeModelId,
		PersistFullToolResults: value.PersistFullToolResults,
		ShowToolCallsInIM:      value.ShowToolCallsInIm,
		OverlayEnabled:         value.OverlayEnabled,
		OverlayProvider:        value.OverlayProvider,
		OverlayConfig:          structToMap(value.GetOverlayConfig()),
	}
	if value.GetToolApprovalConfig() != nil {
		toolApproval, err := toolApprovalConfigFromMap(structToMap(value.GetToolApprovalConfig()))
		if err != nil {
			return settingspkg.UpsertRequest{}, err
		}
		req.ToolApprovalConfig = &toolApproval
	}
	if mask != nil {
		override := settingspkg.OverrideMask(mask.GetFields())
		req.OverrideMask = &override
	}
	return req, nil
}

func effectiveSettingsToProto(value settingspkg.Settings) *privatev1.EffectiveBotSettings {
	return &privatev1.EffectiveBotSettings{
		Settings:     botSettingsToProto(value),
		OverrideMask: &privatev1.SettingsOverrideMask{Fields: map[string]bool(value.OverrideMask)},
		Sources:      fieldSourcesToProto(value.Sources),
		GroupId:      value.GroupID,
	}
}

func botSettingsToProto(value settingspkg.Settings) *privatev1.BotSettings {
	return &privatev1.BotSettings{
		Timezone:               stringPtr(value.Timezone),
		Language:               stringPtr(value.Language),
		ReasoningEnabled:       &value.ReasoningEnabled,
		ReasoningEffort:        stringPtr(value.ReasoningEffort),
		ChatModelId:            stringPtr(value.ChatModelID),
		SearchProviderId:       stringPtr(value.SearchProviderID),
		MemoryProviderId:       stringPtr(value.MemoryProviderID),
		HeartbeatEnabled:       &value.HeartbeatEnabled,
		HeartbeatInterval:      int32Ptr(value.HeartbeatInterval),
		HeartbeatPrompt:        stringPtr(value.HeartbeatPrompt),
		HeartbeatModelId:       stringPtr(value.HeartbeatModelID),
		CompactionEnabled:      &value.CompactionEnabled,
		CompactionThreshold:    int32Ptr(value.CompactionThreshold),
		CompactionRatio:        int32Ptr(value.CompactionRatio),
		CompactionModelId:      stringPtr(value.CompactionModelID),
		TitleModelId:           stringPtr(value.TitleModelID),
		ImageModelId:           stringPtr(value.ImageModelID),
		DiscussProbeModelId:    stringPtr(value.DiscussProbeModelID),
		TtsModelId:             stringPtr(value.TtsModelID),
		TranscriptionModelId:   stringPtr(value.TranscriptionModelID),
		BrowserContextId:       stringPtr(value.BrowserContextID),
		PersistFullToolResults: &value.PersistFullToolResults,
		ShowToolCallsInIm:      &value.ShowToolCallsInIM,
		ToolApprovalConfig:     mapToStruct(toolApprovalConfigToMap(value.ToolApprovalConfig)),
		OverlayProvider:        stringPtr(value.OverlayProvider),
		OverlayEnabled:         &value.OverlayEnabled,
		OverlayConfig:          mapToStruct(value.OverlayConfig),
		AclDefaultEffect:       stringPtr(value.AclDefaultEffect),
	}
}

func fieldSourcesToProto(values []settingspkg.FieldSource) []*privatev1.FieldSource {
	out := make([]*privatev1.FieldSource, 0, len(values))
	for _, value := range values {
		out = append(out, &privatev1.FieldSource{
			Field:    value.Field,
			Source:   value.Source,
			SourceId: value.SourceID,
		})
	}
	return out
}

func toolApprovalConfigFromMap(value map[string]any) (settingspkg.ToolApprovalConfig, error) {
	var cfg settingspkg.ToolApprovalConfig
	payload, err := json.Marshal(value)
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(payload, &cfg); err != nil {
		return cfg, err
	}
	return settingspkg.NormalizeToolApprovalConfig(cfg), nil
}

func toolApprovalConfigToMap(value settingspkg.ToolApprovalConfig) map[string]any {
	payload, err := json.Marshal(value)
	if err != nil {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal(payload, &out); err != nil {
		return map[string]any{}
	}
	return out
}

func intPtrFromInt32(value *int32) *int {
	if value == nil {
		return nil
	}
	out := int(*value)
	return &out
}

func int32Ptr(value int) *int32 {
	out := int32FromInt(value)
	return &out
}

func stringPtr(value string) *string {
	return &value
}

func settingsConnectError(err error) error {
	switch {
	case errors.Is(err, settingspkg.ErrInvalidModelRef):
		return connect.NewError(connect.CodeInvalidArgument, err)
	case errors.Is(err, settingspkg.ErrModelIDAmbiguous):
		return connect.NewError(connect.CodeFailedPrecondition, err)
	default:
		return botConnectError(err)
	}
}
