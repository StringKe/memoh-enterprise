package connectapi

import (
	"context"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/memohai/memoh/internal/botgroups"
	privatev1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1"
	"github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1/privatev1connect"
)

type BotGroupService struct {
	groups *botgroups.Service
}

func NewBotGroupService(groups *botgroups.Service) *BotGroupService {
	return &BotGroupService{groups: groups}
}

func NewBotGroupHandler(service *BotGroupService) Handler {
	path, handler := privatev1connect.NewBotGroupServiceHandler(service)
	return NewHandler(path, handler)
}

func (s *BotGroupService) CreateBotGroup(ctx context.Context, req *connect.Request[privatev1.CreateBotGroupRequest]) (*connect.Response[privatev1.CreateBotGroupResponse], error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	metadata := structToMap(req.Msg.GetMetadata())
	group, err := s.groups.CreateGroup(ctx, userID, botgroups.CreateGroupRequest{
		Name:        req.Msg.GetName(),
		Description: req.Msg.GetDescription(),
		Metadata:    metadata,
	})
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&privatev1.CreateBotGroupResponse{Group: groupToProto(group)}), nil
}

func (s *BotGroupService) GetBotGroup(ctx context.Context, req *connect.Request[privatev1.GetBotGroupRequest]) (*connect.Response[privatev1.GetBotGroupResponse], error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	group, err := s.groups.GetGroup(ctx, userID, req.Msg.GetId())
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&privatev1.GetBotGroupResponse{Group: groupToProto(group)}), nil
}

func (s *BotGroupService) ListBotGroups(ctx context.Context, _ *connect.Request[privatev1.ListBotGroupsRequest]) (*connect.Response[privatev1.ListBotGroupsResponse], error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	groups, err := s.groups.ListGroups(ctx, userID)
	if err != nil {
		return nil, connectError(err)
	}
	items := make([]*privatev1.BotGroup, 0, len(groups))
	for _, group := range groups {
		items = append(items, groupToProto(group))
	}
	return connect.NewResponse(&privatev1.ListBotGroupsResponse{Groups: items}), nil
}

func (s *BotGroupService) UpdateBotGroup(ctx context.Context, req *connect.Request[privatev1.UpdateBotGroupRequest]) (*connect.Response[privatev1.UpdateBotGroupResponse], error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	group, err := s.groups.UpdateGroup(ctx, userID, req.Msg.GetId(), botgroups.UpdateGroupRequest{
		Name:        req.Msg.GetName(),
		Description: req.Msg.GetDescription(),
		Metadata:    structToMap(req.Msg.GetMetadata()),
	})
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&privatev1.UpdateBotGroupResponse{Group: groupToProto(group)}), nil
}

func (s *BotGroupService) DeleteBotGroup(ctx context.Context, req *connect.Request[privatev1.DeleteBotGroupRequest]) (*connect.Response[privatev1.DeleteBotGroupResponse], error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	if err := s.groups.DeleteGroup(ctx, userID, req.Msg.GetId()); err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&privatev1.DeleteBotGroupResponse{}), nil
}

func (s *BotGroupService) GetBotGroupSettings(ctx context.Context, req *connect.Request[privatev1.GetBotGroupSettingsRequest]) (*connect.Response[privatev1.GetBotGroupSettingsResponse], error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	settings, err := s.groups.GetGroupSettings(ctx, userID, req.Msg.GetGroupId())
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&privatev1.GetBotGroupSettingsResponse{Settings: settingsToProto(settings)}), nil
}

func (s *BotGroupService) UpdateBotGroupSettings(ctx context.Context, req *connect.Request[privatev1.UpdateBotGroupSettingsRequest]) (*connect.Response[privatev1.UpdateBotGroupSettingsResponse], error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	settings := settingsFromProto(req.Msg.GetSettings())
	settings.GroupID = req.Msg.GetGroupId()
	updated, err := s.groups.UpsertGroupSettings(ctx, userID, req.Msg.GetGroupId(), settings)
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&privatev1.UpdateBotGroupSettingsResponse{Settings: settingsToProto(updated)}), nil
}

func (s *BotGroupService) DeleteBotGroupSettings(ctx context.Context, req *connect.Request[privatev1.DeleteBotGroupSettingsRequest]) (*connect.Response[privatev1.DeleteBotGroupSettingsResponse], error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	if err := s.groups.DeleteGroupSettings(ctx, userID, req.Msg.GetGroupId()); err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&privatev1.DeleteBotGroupSettingsResponse{}), nil
}

func groupToProto(group botgroups.Group) *privatev1.BotGroup {
	return &privatev1.BotGroup{
		Id:          group.ID,
		OwnerUserId: group.OwnerUserID,
		Name:        group.Name,
		Description: group.Description,
		Metadata:    mapToStruct(group.Metadata),
		BotCount:    group.BotCount,
		Audit: &privatev1.AuditFields{
			CreatedAt: timeToProto(group.CreatedAt),
			UpdatedAt: timeToProto(group.UpdatedAt),
		},
	}
}

func settingsFromProto(settings *privatev1.BotSettings) botgroups.GroupSettings {
	if settings == nil {
		return botgroups.GroupSettings{}
	}
	return botgroups.GroupSettings{
		Timezone:               settings.Timezone,
		Language:               settings.Language,
		ReasoningEnabled:       settings.ReasoningEnabled,
		ReasoningEffort:        settings.ReasoningEffort,
		ChatModelID:            settings.ChatModelId,
		SearchProviderID:       settings.SearchProviderId,
		MemoryProviderID:       settings.MemoryProviderId,
		HeartbeatEnabled:       settings.HeartbeatEnabled,
		HeartbeatInterval:      settings.HeartbeatInterval,
		HeartbeatPrompt:        settings.HeartbeatPrompt,
		HeartbeatModelID:       settings.HeartbeatModelId,
		CompactionEnabled:      settings.CompactionEnabled,
		CompactionThreshold:    settings.CompactionThreshold,
		CompactionRatio:        settings.CompactionRatio,
		CompactionModelID:      settings.CompactionModelId,
		TitleModelID:           settings.TitleModelId,
		ImageModelID:           settings.ImageModelId,
		DiscussProbeModelID:    settings.DiscussProbeModelId,
		TTSModelID:             settings.TtsModelId,
		TranscriptionModelID:   settings.TranscriptionModelId,
		BrowserContextID:       settings.BrowserContextId,
		PersistFullToolResults: settings.PersistFullToolResults,
		ShowToolCallsInIM:      settings.ShowToolCallsInIm,
		ToolApprovalConfig:     structToMap(settings.ToolApprovalConfig),
		OverlayProvider:        settings.OverlayProvider,
		OverlayEnabled:         settings.OverlayEnabled,
		OverlayConfig:          structToMap(settings.OverlayConfig),
		Metadata:               structToMap(settings.Metadata),
	}
}

func settingsToProto(settings botgroups.GroupSettings) *privatev1.BotSettings {
	return &privatev1.BotSettings{
		Timezone:               settings.Timezone,
		Language:               settings.Language,
		ReasoningEnabled:       settings.ReasoningEnabled,
		ReasoningEffort:        settings.ReasoningEffort,
		ChatModelId:            settings.ChatModelID,
		SearchProviderId:       settings.SearchProviderID,
		MemoryProviderId:       settings.MemoryProviderID,
		HeartbeatEnabled:       settings.HeartbeatEnabled,
		HeartbeatInterval:      settings.HeartbeatInterval,
		HeartbeatPrompt:        settings.HeartbeatPrompt,
		HeartbeatModelId:       settings.HeartbeatModelID,
		CompactionEnabled:      settings.CompactionEnabled,
		CompactionThreshold:    settings.CompactionThreshold,
		CompactionRatio:        settings.CompactionRatio,
		CompactionModelId:      settings.CompactionModelID,
		TitleModelId:           settings.TitleModelID,
		ImageModelId:           settings.ImageModelID,
		DiscussProbeModelId:    settings.DiscussProbeModelID,
		TtsModelId:             settings.TTSModelID,
		TranscriptionModelId:   settings.TranscriptionModelID,
		BrowserContextId:       settings.BrowserContextID,
		PersistFullToolResults: settings.PersistFullToolResults,
		ShowToolCallsInIm:      settings.ShowToolCallsInIM,
		ToolApprovalConfig:     mapToStruct(settings.ToolApprovalConfig),
		OverlayProvider:        settings.OverlayProvider,
		OverlayEnabled:         settings.OverlayEnabled,
		OverlayConfig:          mapToStruct(settings.OverlayConfig),
		Metadata:               mapToStruct(settings.Metadata),
	}
}

func mapToStruct(value map[string]any) *structpb.Struct {
	if value == nil {
		value = map[string]any{}
	}
	result, err := structpb.NewStruct(value)
	if err != nil {
		return &structpb.Struct{Fields: map[string]*structpb.Value{}}
	}
	return result
}

func structToMap(value *structpb.Struct) map[string]any {
	if value == nil {
		return nil
	}
	return value.AsMap()
}

func timeToProto(value time.Time) *timestamppb.Timestamp {
	if value.IsZero() {
		return nil
	}
	return timestamppb.New(value)
}
