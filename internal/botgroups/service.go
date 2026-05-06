package botgroups

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
)

var (
	ErrGroupNotFound     = errors.New("bot group not found")
	ErrGroupAccessDenied = errors.New("bot group access denied")
)

type Service struct {
	queries dbstore.Queries
	logger  *slog.Logger
}

func NewService(log *slog.Logger, queries dbstore.Queries) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		queries: queries,
		logger:  log.With(slog.String("service", "botgroups")),
	}
}

func (s *Service) CreateGroup(ctx context.Context, ownerID string, req CreateGroupRequest) (Group, error) {
	if s.queries == nil {
		return Group{}, errors.New("bot group queries not configured")
	}
	ownerUUID, err := db.ParseUUID(strings.TrimSpace(ownerID))
	if err != nil {
		return Group{}, err
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return Group{}, errors.New("name is required")
	}
	metadata, err := encodeMap(req.Metadata)
	if err != nil {
		return Group{}, err
	}
	row, err := s.queries.CreateBotGroup(ctx, sqlc.CreateBotGroupParams{
		OwnerUserID: ownerUUID,
		Name:        name,
		Description: strings.TrimSpace(req.Description),
		Metadata:    metadata,
	})
	if err != nil {
		return Group{}, err
	}
	return s.toGroup(ctx, row)
}

func (s *Service) ListGroups(ctx context.Context, ownerID string) ([]Group, error) {
	if s.queries == nil {
		return nil, errors.New("bot group queries not configured")
	}
	ownerUUID, err := db.ParseUUID(strings.TrimSpace(ownerID))
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListBotGroupsByOwner(ctx, ownerUUID)
	if err != nil {
		return nil, err
	}
	groups := make([]Group, 0, len(rows))
	for _, row := range rows {
		group, err := s.toGroup(ctx, row)
		if err != nil {
			return nil, err
		}
		groups = append(groups, group)
	}
	return groups, nil
}

func (s *Service) GetGroup(ctx context.Context, requesterID, groupID string) (Group, error) {
	row, err := s.requireOwnedGroup(ctx, requesterID, groupID)
	if err != nil {
		return Group{}, err
	}
	return s.toGroup(ctx, row)
}

func (s *Service) UpdateGroup(ctx context.Context, requesterID, groupID string, req UpdateGroupRequest) (Group, error) {
	row, err := s.requireOwnedGroup(ctx, requesterID, groupID)
	if err != nil {
		return Group{}, err
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return Group{}, errors.New("name is required")
	}
	metadata, err := encodeMap(req.Metadata)
	if err != nil {
		return Group{}, err
	}
	updated, err := s.queries.UpdateBotGroup(ctx, sqlc.UpdateBotGroupParams{
		ID:          row.ID,
		Name:        name,
		Description: strings.TrimSpace(req.Description),
		Metadata:    metadata,
	})
	if err != nil {
		return Group{}, err
	}
	return s.toGroup(ctx, updated)
}

func (s *Service) DeleteGroup(ctx context.Context, requesterID, groupID string) error {
	row, err := s.requireOwnedGroup(ctx, requesterID, groupID)
	if err != nil {
		return err
	}
	if err := s.queries.DeleteBotGroup(ctx, row.ID); err != nil {
		return err
	}
	return nil
}

func (s *Service) GetGroupSettings(ctx context.Context, requesterID, groupID string) (GroupSettings, error) {
	group, err := s.requireOwnedGroup(ctx, requesterID, groupID)
	if err != nil {
		return GroupSettings{}, err
	}
	row, err := s.queries.GetBotGroupSettings(ctx, group.ID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return GroupSettings{GroupID: group.ID.String()}, nil
		}
		return GroupSettings{}, err
	}
	return toGroupSettings(row)
}

func (s *Service) UpsertGroupSettings(ctx context.Context, requesterID, groupID string, req GroupSettings) (GroupSettings, error) {
	group, err := s.requireOwnedGroup(ctx, requesterID, groupID)
	if err != nil {
		return GroupSettings{}, err
	}
	params, err := groupSettingsParams(group.ID, req)
	if err != nil {
		return GroupSettings{}, err
	}
	row, err := s.queries.UpsertBotGroupSettings(ctx, params)
	if err != nil {
		return GroupSettings{}, err
	}
	return toGroupSettings(row)
}

func (s *Service) DeleteGroupSettings(ctx context.Context, requesterID, groupID string) error {
	group, err := s.requireOwnedGroup(ctx, requesterID, groupID)
	if err != nil {
		return err
	}
	return s.queries.DeleteBotGroupSettings(ctx, group.ID)
}

func (s *Service) requireOwnedGroup(ctx context.Context, requesterID, groupID string) (sqlc.BotGroup, error) {
	if s.queries == nil {
		return sqlc.BotGroup{}, errors.New("bot group queries not configured")
	}
	requesterUUID, err := db.ParseUUID(strings.TrimSpace(requesterID))
	if err != nil {
		return sqlc.BotGroup{}, err
	}
	groupUUID, err := db.ParseUUID(strings.TrimSpace(groupID))
	if err != nil {
		return sqlc.BotGroup{}, err
	}
	row, err := s.queries.GetBotGroupByOwnerAndID(ctx, sqlc.GetBotGroupByOwnerAndIDParams{
		OwnerUserID: requesterUUID,
		ID:          groupUUID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			if _, getErr := s.queries.GetBotGroupByID(ctx, groupUUID); getErr == nil {
				return sqlc.BotGroup{}, ErrGroupAccessDenied
			}
			return sqlc.BotGroup{}, ErrGroupNotFound
		}
		return sqlc.BotGroup{}, err
	}
	return row, nil
}

func (s *Service) toGroup(ctx context.Context, row sqlc.BotGroup) (Group, error) {
	metadata, err := decodeMap(row.Metadata)
	if err != nil {
		return Group{}, err
	}
	count, err := s.queries.CountBotsInGroup(ctx, row.ID)
	if err != nil {
		return Group{}, err
	}
	return Group{
		ID:          row.ID.String(),
		OwnerUserID: row.OwnerUserID.String(),
		Name:        row.Name,
		Description: row.Description,
		Metadata:    metadata,
		BotCount:    count,
		CreatedAt:   timeFromPg(row.CreatedAt),
		UpdatedAt:   timeFromPg(row.UpdatedAt),
	}, nil
}

func groupSettingsParams(groupID pgtype.UUID, settings GroupSettings) (sqlc.UpsertBotGroupSettingsParams, error) {
	toolApprovalConfig, err := encodeNullableMap(settings.ToolApprovalConfig)
	if err != nil {
		return sqlc.UpsertBotGroupSettingsParams{}, err
	}
	overlayConfig, err := encodeNullableMap(settings.OverlayConfig)
	if err != nil {
		return sqlc.UpsertBotGroupSettingsParams{}, err
	}
	metadata, err := encodeMap(settings.Metadata)
	if err != nil {
		return sqlc.UpsertBotGroupSettingsParams{}, err
	}
	chatModelID, err := uuidPtr(settings.ChatModelID)
	if err != nil {
		return sqlc.UpsertBotGroupSettingsParams{}, fmtFieldUUID(FieldChatModelID, err)
	}
	searchProviderID, err := uuidPtr(settings.SearchProviderID)
	if err != nil {
		return sqlc.UpsertBotGroupSettingsParams{}, fmtFieldUUID(FieldSearchProviderID, err)
	}
	memoryProviderID, err := uuidPtr(settings.MemoryProviderID)
	if err != nil {
		return sqlc.UpsertBotGroupSettingsParams{}, fmtFieldUUID(FieldMemoryProviderID, err)
	}
	heartbeatModelID, err := uuidPtr(settings.HeartbeatModelID)
	if err != nil {
		return sqlc.UpsertBotGroupSettingsParams{}, fmtFieldUUID(FieldHeartbeatModelID, err)
	}
	compactionModelID, err := uuidPtr(settings.CompactionModelID)
	if err != nil {
		return sqlc.UpsertBotGroupSettingsParams{}, fmtFieldUUID(FieldCompactionModelID, err)
	}
	titleModelID, err := uuidPtr(settings.TitleModelID)
	if err != nil {
		return sqlc.UpsertBotGroupSettingsParams{}, fmtFieldUUID(FieldTitleModelID, err)
	}
	imageModelID, err := uuidPtr(settings.ImageModelID)
	if err != nil {
		return sqlc.UpsertBotGroupSettingsParams{}, fmtFieldUUID(FieldImageModelID, err)
	}
	discussProbeModelID, err := uuidPtr(settings.DiscussProbeModelID)
	if err != nil {
		return sqlc.UpsertBotGroupSettingsParams{}, fmtFieldUUID(FieldDiscussProbeModelID, err)
	}
	ttsModelID, err := uuidPtr(settings.TTSModelID)
	if err != nil {
		return sqlc.UpsertBotGroupSettingsParams{}, fmtFieldUUID(FieldTTSModelID, err)
	}
	transcriptionModelID, err := uuidPtr(settings.TranscriptionModelID)
	if err != nil {
		return sqlc.UpsertBotGroupSettingsParams{}, fmtFieldUUID(FieldTranscriptionModelID, err)
	}
	browserContextID, err := uuidPtr(settings.BrowserContextID)
	if err != nil {
		return sqlc.UpsertBotGroupSettingsParams{}, fmtFieldUUID(FieldBrowserContextID, err)
	}
	params := sqlc.UpsertBotGroupSettingsParams{
		GroupID:                groupID,
		Timezone:               textPtr(settings.Timezone),
		Language:               textPtr(settings.Language),
		ReasoningEnabled:       boolPtr(settings.ReasoningEnabled),
		ReasoningEffort:        textPtr(settings.ReasoningEffort),
		ChatModelID:            chatModelID,
		SearchProviderID:       searchProviderID,
		MemoryProviderID:       memoryProviderID,
		HeartbeatEnabled:       boolPtr(settings.HeartbeatEnabled),
		HeartbeatInterval:      int4Ptr(settings.HeartbeatInterval),
		HeartbeatPrompt:        textPtr(settings.HeartbeatPrompt),
		HeartbeatModelID:       heartbeatModelID,
		CompactionEnabled:      boolPtr(settings.CompactionEnabled),
		CompactionThreshold:    int4Ptr(settings.CompactionThreshold),
		CompactionRatio:        int4Ptr(settings.CompactionRatio),
		CompactionModelID:      compactionModelID,
		TitleModelID:           titleModelID,
		ImageModelID:           imageModelID,
		DiscussProbeModelID:    discussProbeModelID,
		TtsModelID:             ttsModelID,
		TranscriptionModelID:   transcriptionModelID,
		BrowserContextID:       browserContextID,
		PersistFullToolResults: boolPtr(settings.PersistFullToolResults),
		ShowToolCallsInIm:      boolPtr(settings.ShowToolCallsInIM),
		ToolApprovalConfig:     toolApprovalConfig,
		OverlayProvider:        textPtr(settings.OverlayProvider),
		OverlayEnabled:         boolPtr(settings.OverlayEnabled),
		OverlayConfig:          overlayConfig,
		Metadata:               metadata,
	}
	return params, nil
}

func toGroupSettings(row sqlc.BotGroupSetting) (GroupSettings, error) {
	toolApprovalConfig, err := decodeNullableMap(row.ToolApprovalConfig)
	if err != nil {
		return GroupSettings{}, err
	}
	overlayConfig, err := decodeNullableMap(row.OverlayConfig)
	if err != nil {
		return GroupSettings{}, err
	}
	metadata, err := decodeMap(row.Metadata)
	if err != nil {
		return GroupSettings{}, err
	}
	return GroupSettings{
		GroupID:                row.GroupID.String(),
		Timezone:               textValue(row.Timezone),
		Language:               textValue(row.Language),
		ReasoningEnabled:       boolValue(row.ReasoningEnabled),
		ReasoningEffort:        textValue(row.ReasoningEffort),
		ChatModelID:            uuidValue(row.ChatModelID),
		SearchProviderID:       uuidValue(row.SearchProviderID),
		MemoryProviderID:       uuidValue(row.MemoryProviderID),
		HeartbeatEnabled:       boolValue(row.HeartbeatEnabled),
		HeartbeatInterval:      int4Value(row.HeartbeatInterval),
		HeartbeatPrompt:        textValue(row.HeartbeatPrompt),
		HeartbeatModelID:       uuidValue(row.HeartbeatModelID),
		CompactionEnabled:      boolValue(row.CompactionEnabled),
		CompactionThreshold:    int4Value(row.CompactionThreshold),
		CompactionRatio:        int4Value(row.CompactionRatio),
		CompactionModelID:      uuidValue(row.CompactionModelID),
		TitleModelID:           uuidValue(row.TitleModelID),
		ImageModelID:           uuidValue(row.ImageModelID),
		DiscussProbeModelID:    uuidValue(row.DiscussProbeModelID),
		TTSModelID:             uuidValue(row.TtsModelID),
		TranscriptionModelID:   uuidValue(row.TranscriptionModelID),
		BrowserContextID:       uuidValue(row.BrowserContextID),
		PersistFullToolResults: boolValue(row.PersistFullToolResults),
		ShowToolCallsInIM:      boolValue(row.ShowToolCallsInIm),
		ToolApprovalConfig:     toolApprovalConfig,
		OverlayProvider:        textValue(row.OverlayProvider),
		OverlayEnabled:         boolValue(row.OverlayEnabled),
		OverlayConfig:          overlayConfig,
		Metadata:               metadata,
		UpdatedAt:              timeFromPg(row.UpdatedAt),
	}, nil
}

func encodeMap(value map[string]any) ([]byte, error) {
	if value == nil {
		value = map[string]any{}
	}
	return json.Marshal(value)
}

func encodeNullableMap(value map[string]any) ([]byte, error) {
	if value == nil {
		return nil, nil
	}
	return json.Marshal(value)
}

func decodeMap(payload []byte) (map[string]any, error) {
	if len(payload) == 0 {
		return map[string]any{}, nil
	}
	var value map[string]any
	if err := json.Unmarshal(payload, &value); err != nil {
		return nil, err
	}
	if value == nil {
		value = map[string]any{}
	}
	return value, nil
}

func decodeNullableMap(payload []byte) (map[string]any, error) {
	if len(payload) == 0 {
		return nil, nil
	}
	return decodeMap(payload)
}

func textPtr(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *value, Valid: true}
}

func textValue(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	result := value.String
	return &result
}

func boolPtr(value *bool) pgtype.Bool {
	if value == nil {
		return pgtype.Bool{}
	}
	return pgtype.Bool{Bool: *value, Valid: true}
}

func boolValue(value pgtype.Bool) *bool {
	if !value.Valid {
		return nil
	}
	result := value.Bool
	return &result
}

func int4Ptr(value *int32) pgtype.Int4 {
	if value == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: *value, Valid: true}
}

func int4Value(value pgtype.Int4) *int32 {
	if !value.Valid {
		return nil
	}
	result := value.Int32
	return &result
}

func uuidPtr(value *string) (pgtype.UUID, error) {
	if value == nil || strings.TrimSpace(*value) == "" {
		return pgtype.UUID{}, nil
	}
	return db.ParseUUID(strings.TrimSpace(*value))
}

func fmtFieldUUID(field string, err error) error {
	return errors.New(field + ": invalid uuid: " + err.Error())
}

func uuidValue(value pgtype.UUID) *string {
	if !value.Valid {
		return nil
	}
	result := value.String()
	return &result
}

func timeFromPg(value pgtype.Timestamptz) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time
}
