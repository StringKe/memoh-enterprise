package bots

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/acl"
	"github.com/memohai/memoh/internal/botgroups"
	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
	"github.com/memohai/memoh/internal/iam/rbac"
	tzutil "github.com/memohai/memoh/internal/timezone"
)

type PermissionService interface {
	HasPermission(ctx context.Context, check rbac.Check) (bool, error)
}

// Service provides bot CRUD and membership management.
type Service struct {
	queries               dbstore.Queries
	logger                *slog.Logger
	rbac                  PermissionService
	containerLifecycle    ContainerLifecycle
	checkers              []RuntimeChecker
	containerReachability func(ctx context.Context, botID string) error
}

const (
	botLifecycleOperationTimeout = 5 * time.Minute
)

var defaultSettingsOverrideMask = []byte(`{"timezone":true,"language":true,"reasoning_enabled":true,"reasoning_effort":true,"chat_model_id":true,"search_provider_id":true,"memory_provider_id":true,"heartbeat_enabled":true,"heartbeat_interval":true,"heartbeat_prompt":true,"heartbeat_model_id":true,"compaction_enabled":true,"compaction_threshold":true,"compaction_ratio":true,"compaction_model_id":true,"title_model_id":true,"image_model_id":true,"discuss_probe_model_id":true,"tts_model_id":true,"transcription_model_id":true,"persist_full_tool_results":true,"show_tool_calls_in_im":true,"tool_approval_config":true,"overlay_provider":true,"overlay_enabled":true,"overlay_config":true}`)

var inheritedSettingsOverrideMask = []byte(`{"timezone":false,"language":false,"reasoning_enabled":false,"reasoning_effort":false,"chat_model_id":false,"search_provider_id":false,"memory_provider_id":false,"heartbeat_enabled":false,"heartbeat_interval":false,"heartbeat_prompt":false,"heartbeat_model_id":false,"compaction_enabled":false,"compaction_threshold":false,"compaction_ratio":false,"compaction_model_id":false,"title_model_id":false,"image_model_id":false,"discuss_probe_model_id":false,"tts_model_id":false,"transcription_model_id":false,"persist_full_tool_results":false,"show_tool_calls_in_im":false,"tool_approval_config":false,"overlay_provider":false,"overlay_enabled":false,"overlay_config":false}`)

var (
	ErrBotNotFound        = errors.New("bot not found")
	ErrBotAccessDenied    = errors.New("bot access denied")
	ErrOwnerUserNotFound  = errors.New("owner user not found")
	ErrBotGroupNotFound   = errors.New("bot group not found")
	ErrBotGroupNotAllowed = errors.New("bot group access denied")
)

// NewService creates a new bot service.
func NewService(log *slog.Logger, queries dbstore.Queries) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		queries: queries,
		logger:  log.With(slog.String("service", "bots")),
	}
}

// SetContainerLifecycle registers a container lifecycle handler for bot operations.
func (s *Service) SetContainerLifecycle(lc ContainerLifecycle) {
	s.containerLifecycle = lc
}

func (s *Service) SetRBACService(service PermissionService) {
	s.rbac = service
}

// SetContainerReachability registers a function that checks whether a bot's
// container is reachable via gRPC. Returns nil on success, error otherwise.
func (s *Service) SetContainerReachability(fn func(ctx context.Context, botID string) error) {
	s.containerReachability = fn
}

// AddRuntimeChecker registers an additional runtime checker.
func (s *Service) AddRuntimeChecker(c RuntimeChecker) {
	if c != nil {
		s.checkers = append(s.checkers, c)
	}
}

// AuthorizeAccess checks whether userID may read the given bot.
func (s *Service) AuthorizeAccess(ctx context.Context, userID, botID string, isAdmin bool) (Bot, error) {
	if s.queries == nil {
		return Bot{}, errors.New("bot queries not configured")
	}
	_ = isAdmin
	bot, err := s.Get(ctx, botID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Bot{}, ErrBotNotFound
		}
		return Bot{}, err
	}
	allowed, err := s.hasBotPermission(ctx, userID, bot.ID, rbac.PermissionBotRead)
	if err != nil {
		return Bot{}, err
	}
	if allowed {
		return bot, nil
	}
	return Bot{}, ErrBotAccessDenied
}

// Create creates a new bot owned by owner user.
func (s *Service) Create(ctx context.Context, ownerUserID string, req CreateBotRequest) (Bot, error) {
	if s.queries == nil {
		return Bot{}, errors.New("bot queries not configured")
	}
	ownerID := strings.TrimSpace(ownerUserID)
	if ownerID == "" {
		return Bot{}, errors.New("owner user id is required")
	}
	ownerUUID, err := db.ParseUUID(ownerID)
	if err != nil {
		return Bot{}, err
	}
	if err := s.ensureUserExists(ctx, ownerUUID); err != nil {
		return Bot{}, err
	}
	aclPresetKey := acl.NormalizePresetKey(req.AclPreset)
	if _, err := acl.ResolvePreset(aclPresetKey); err != nil {
		return Bot{}, err
	}
	displayName := strings.TrimSpace(req.DisplayName)
	if displayName == "" {
		displayName = "bot-" + uuid.NewString()
	}
	avatarURL := strings.TrimSpace(req.AvatarURL)
	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}
	displayEnabled := false
	if req.DisplayEnabled != nil {
		displayEnabled = *req.DisplayEnabled
	}
	timezoneValue, err := normalizeOptionalTimezone(req.Timezone)
	if err != nil {
		return Bot{}, err
	}
	groupID, err := normalizeOptionalUUID(req.GroupID)
	if err != nil {
		return Bot{}, fmt.Errorf("invalid group id: %w", err)
	}
	if err := s.ensureGroupBelongsToOwner(ctx, ownerUUID, groupID); err != nil {
		if err := s.ensureGroupCanAttach(ctx, ownerID, groupID); err != nil {
			return Bot{}, err
		}
	}
	settingsOverrideMask, err := createSettingsOverrideMask(groupID.Valid, timezoneValue.Valid)
	if err != nil {
		return Bot{}, err
	}
	metadata := req.Metadata
	if metadata == nil {
		metadata = map[string]any{}
	}
	payload, err := json.Marshal(metadata)
	if err != nil {
		return Bot{}, err
	}
	row, err := s.queries.CreateBot(ctx, sqlc.CreateBotParams{
		OwnerUserID:          ownerUUID,
		GroupID:              groupID,
		DisplayName:          pgtype.Text{String: displayName, Valid: displayName != ""},
		AvatarUrl:            pgtype.Text{String: avatarURL, Valid: avatarURL != ""},
		Timezone:             timezoneValue,
		IsActive:             isActive,
		DisplayEnabled:       displayEnabled,
		Metadata:             payload,
		Status:               BotStatusCreating,
		SettingsOverrideMask: settingsOverrideMask,
	})
	if err != nil {
		return Bot{}, err
	}
	bot, err := toBot(asSQLCBot(row))
	if err != nil {
		return Bot{}, err
	}
	if err := acl.ApplyPreset(ctx, s.queries, bot.ID, ownerID, aclPresetKey); err != nil {
		if cleanupErr := s.queries.DeleteBotByID(ctx, row.ID); cleanupErr != nil {
			return Bot{}, errors.Join(
				fmt.Errorf("apply acl preset: %w", err),
				fmt.Errorf("cleanup bot after acl preset failure: %w", cleanupErr),
			)
		}
		return Bot{}, fmt.Errorf("apply acl preset: %w", err)
	}
	if err := s.assignBotOwnerRole(ctx, ownerUUID, row.ID); err != nil {
		if cleanupErr := s.queries.DeleteBotByID(ctx, row.ID); cleanupErr != nil {
			return Bot{}, errors.Join(
				fmt.Errorf("assign bot owner role: %w", err),
				fmt.Errorf("cleanup bot after owner role failure: %w", cleanupErr),
			)
		}
		return Bot{}, fmt.Errorf("assign bot owner role: %w", err)
	}
	if err := s.attachCheckSummary(ctx, &bot, asSQLCBot(row)); err != nil {
		return Bot{}, err
	}
	s.enqueueCreateLifecycle(ctx, bot.ID)
	return bot, nil
}

// Get returns a bot by its ID.
func (s *Service) Get(ctx context.Context, botID string) (Bot, error) {
	if s.queries == nil {
		return Bot{}, errors.New("bot queries not configured")
	}
	botUUID, err := db.ParseUUID(botID)
	if err != nil {
		return Bot{}, err
	}
	row, err := s.queries.GetBotByID(ctx, botUUID)
	if err != nil {
		return Bot{}, err
	}
	bot, err := toBot(asSQLCBot(row))
	if err != nil {
		return Bot{}, err
	}
	if err := s.attachCheckSummary(ctx, &bot, asSQLCBot(row)); err != nil {
		return Bot{}, err
	}
	return bot, nil
}

// ListByOwner returns bots owned by the given user.
func (s *Service) ListByOwner(ctx context.Context, ownerUserID string) ([]Bot, error) {
	if s.queries == nil {
		return nil, errors.New("bot queries not configured")
	}
	ownerUUID, err := db.ParseUUID(ownerUserID)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListBotsByOwner(ctx, ownerUUID)
	if err != nil {
		return nil, err
	}
	items := make([]Bot, 0, len(rows))
	for _, row := range rows {
		item, err := toBot(asSQLCBot(row))
		if err != nil {
			return nil, err
		}
		if err := s.attachCheckSummary(ctx, &item, asSQLCBot(row)); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

// ListAccessible returns all bots readable by the user through ownership, bot RBAC, or bot-group RBAC.
func (s *Service) ListAccessible(ctx context.Context, userID string) ([]Bot, error) {
	if s.queries == nil {
		return nil, errors.New("bot queries not configured")
	}
	userUUID, err := db.ParseUUID(userID)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListAccessibleBots(ctx, userUUID)
	if err != nil {
		return nil, err
	}
	items := make([]Bot, 0, len(rows))
	for _, row := range rows {
		item, err := toBot(asSQLCBot(row))
		if err != nil {
			return nil, err
		}
		if err := s.attachCheckSummary(ctx, &item, asSQLCBot(row)); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

// Update updates bot profile fields.
func (s *Service) Update(ctx context.Context, botID string, req UpdateBotRequest) (Bot, error) {
	if s.queries == nil {
		return Bot{}, errors.New("bot queries not configured")
	}
	botUUID, err := db.ParseUUID(botID)
	if err != nil {
		return Bot{}, err
	}
	existing, err := s.queries.GetBotByID(ctx, botUUID)
	if err != nil {
		return Bot{}, err
	}
	displayName := strings.TrimSpace(existing.DisplayName.String)
	avatarURL := strings.TrimSpace(existing.AvatarUrl.String)
	isActive := existing.IsActive
	displayEnabled := existing.DisplayEnabled
	metadata, err := decodeMetadata(existing.Metadata)
	if err != nil {
		return Bot{}, err
	}
	if req.DisplayName != nil {
		displayName = strings.TrimSpace(*req.DisplayName)
	}
	if req.AvatarURL != nil {
		avatarURL = strings.TrimSpace(*req.AvatarURL)
	}
	if req.IsActive != nil {
		isActive = *req.IsActive
	}
	if req.DisplayEnabled != nil {
		displayEnabled = *req.DisplayEnabled
	}
	timezoneValue := existing.Timezone
	if req.Timezone != nil {
		timezoneValue, err = normalizeOptionalTimezone(req.Timezone)
		if err != nil {
			return Bot{}, err
		}
	}
	if req.Metadata != nil {
		metadata = req.Metadata
	}
	groupID := existing.GroupID
	if req.GroupID != nil {
		groupID, err = normalizeOptionalUUID(*req.GroupID)
		if err != nil {
			return Bot{}, fmt.Errorf("invalid group id: %w", err)
		}
		if err := s.ensureGroupBelongsToOwner(ctx, existing.OwnerUserID, groupID); err != nil {
			return Bot{}, err
		}
	}
	if displayName == "" {
		displayName = "bot-" + uuid.NewString()
	}
	payload, err := json.Marshal(metadata)
	if err != nil {
		return Bot{}, err
	}
	row, err := s.queries.UpdateBotProfile(ctx, sqlc.UpdateBotProfileParams{
		ID:             botUUID,
		DisplayName:    pgtype.Text{String: displayName, Valid: displayName != ""},
		AvatarUrl:      pgtype.Text{String: avatarURL, Valid: avatarURL != ""},
		Timezone:       timezoneValue,
		IsActive:       isActive,
		GroupID:        groupID,
		DisplayEnabled: displayEnabled,
		Metadata:       payload,
	})
	if err != nil {
		return Bot{}, err
	}
	bot, err := toBot(asSQLCBot(row))
	if err != nil {
		return Bot{}, err
	}
	if err := s.attachCheckSummary(ctx, &bot, asSQLCBot(row)); err != nil {
		return Bot{}, err
	}
	return bot, nil
}

func (s *Service) AssignGroup(ctx context.Context, userID, botID, groupID string) (Bot, error) {
	if s.queries == nil {
		return Bot{}, errors.New("bot queries not configured")
	}
	allowed, err := s.hasBotPermission(ctx, userID, botID, rbac.PermissionBotUpdate)
	if err != nil {
		return Bot{}, err
	}
	if !allowed {
		return Bot{}, ErrBotAccessDenied
	}
	botUUID, err := db.ParseUUID(botID)
	if err != nil {
		return Bot{}, err
	}
	existing, err := s.queries.GetBotByID(ctx, botUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Bot{}, ErrBotNotFound
		}
		return Bot{}, err
	}
	targetGroupID, err := normalizeOptionalUUID(groupID)
	if err != nil {
		return Bot{}, fmt.Errorf("invalid group id: %w", err)
	}
	if !targetGroupID.Valid {
		return Bot{}, errors.New("group id is required")
	}
	if err := s.ensureGroupCanAttach(ctx, userID, targetGroupID); err != nil {
		return Bot{}, err
	}
	row, err := s.queries.UpdateBotProfile(ctx, sqlc.UpdateBotProfileParams{
		ID:             botUUID,
		DisplayName:    existing.DisplayName,
		AvatarUrl:      existing.AvatarUrl,
		Timezone:       existing.Timezone,
		IsActive:       existing.IsActive,
		GroupID:        targetGroupID,
		DisplayEnabled: existing.DisplayEnabled,
		Metadata:       existing.Metadata,
	})
	if err != nil {
		return Bot{}, err
	}
	bot, err := toBot(asSQLCBot(row))
	if err != nil {
		return Bot{}, err
	}
	if err := s.attachCheckSummary(ctx, &bot, asSQLCBot(row)); err != nil {
		return Bot{}, err
	}
	return bot, nil
}

func (s *Service) ClearGroup(ctx context.Context, userID, botID string) (Bot, error) {
	if s.queries == nil {
		return Bot{}, errors.New("bot queries not configured")
	}
	allowed, err := s.hasBotPermission(ctx, userID, botID, rbac.PermissionBotUpdate)
	if err != nil {
		return Bot{}, err
	}
	if !allowed {
		return Bot{}, ErrBotAccessDenied
	}
	botUUID, err := db.ParseUUID(botID)
	if err != nil {
		return Bot{}, err
	}
	existing, err := s.queries.GetBotByID(ctx, botUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Bot{}, ErrBotNotFound
		}
		return Bot{}, err
	}
	row, err := s.queries.UpdateBotProfile(ctx, sqlc.UpdateBotProfileParams{
		ID:             botUUID,
		DisplayName:    existing.DisplayName,
		AvatarUrl:      existing.AvatarUrl,
		Timezone:       existing.Timezone,
		IsActive:       existing.IsActive,
		GroupID:        pgtype.UUID{},
		DisplayEnabled: existing.DisplayEnabled,
		Metadata:       existing.Metadata,
	})
	if err != nil {
		return Bot{}, err
	}
	bot, err := toBot(asSQLCBot(row))
	if err != nil {
		return Bot{}, err
	}
	if err := s.attachCheckSummary(ctx, &bot, asSQLCBot(row)); err != nil {
		return Bot{}, err
	}
	return bot, nil
}

// TransferOwner transfers bot ownership to another user.
func (s *Service) TransferOwner(ctx context.Context, botID string, ownerUserID string) (Bot, error) {
	if s.queries == nil {
		return Bot{}, errors.New("bot queries not configured")
	}
	botUUID, err := db.ParseUUID(botID)
	if err != nil {
		return Bot{}, err
	}
	ownerUUID, err := db.ParseUUID(ownerUserID)
	if err != nil {
		return Bot{}, err
	}
	if err := s.ensureUserExists(ctx, ownerUUID); err != nil {
		return Bot{}, err
	}
	current, err := s.queries.GetBotByID(ctx, botUUID)
	if err != nil {
		return Bot{}, err
	}
	oldOwnerID := current.OwnerUserID
	row, err := s.queries.UpdateBotOwner(ctx, sqlc.UpdateBotOwnerParams{
		ID:          botUUID,
		OwnerUserID: ownerUUID,
	})
	if err != nil {
		return Bot{}, err
	}
	bot, err := toBot(asSQLCBot(row))
	if err != nil {
		return Bot{}, err
	}
	if err := s.attachCheckSummary(ctx, &bot, asSQLCBot(row)); err != nil {
		return Bot{}, err
	}
	if err := s.assignBotOwnerRole(ctx, ownerUUID, botUUID); err != nil {
		return Bot{}, fmt.Errorf("assign bot owner role: %w", err)
	}
	if oldOwnerID.Valid && oldOwnerID != ownerUUID {
		if err := s.deleteBotOwnerRole(ctx, oldOwnerID, botUUID); err != nil {
			return Bot{}, fmt.Errorf("delete previous bot owner role: %w", err)
		}
	}
	return bot, nil
}

func (s *Service) HasBotPermission(ctx context.Context, userID, botID string, permission rbac.PermissionKey) (bool, error) {
	return s.hasBotPermission(ctx, userID, botID, permission)
}

func (s *Service) hasBotPermission(ctx context.Context, userID, botID string, permission rbac.PermissionKey) (bool, error) {
	if s.rbac == nil {
		return false, errors.New("bot rbac service not configured")
	}
	userID = strings.TrimSpace(userID)
	botID = strings.TrimSpace(botID)
	if userID == "" || botID == "" {
		return false, nil
	}
	allowed, err := s.rbac.HasPermission(ctx, rbac.Check{
		UserID:        userID,
		PermissionKey: permission,
		ResourceType:  rbac.ResourceBot,
		ResourceID:    botID,
	})
	if err != nil || allowed {
		return allowed, err
	}
	botUUID, err := db.ParseUUID(botID)
	if err != nil {
		return false, err
	}
	row, err := s.queries.GetBotByID(ctx, botUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, ErrBotNotFound
		}
		return false, err
	}
	if !row.GroupID.Valid {
		return false, nil
	}
	group, err := s.queries.GetBotGroupByID(ctx, row.GroupID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	if permission == rbac.PermissionBotRead || permission == rbac.PermissionBotChat {
		if group.Visibility == botgroups.VisibilityOrganization || group.Visibility == botgroups.VisibilityPublic {
			return true, nil
		}
	}
	groupPermission, ok := inheritedBotGroupPermission(permission)
	if !ok {
		return false, nil
	}
	return s.rbac.HasPermission(ctx, rbac.Check{
		UserID:        userID,
		PermissionKey: groupPermission,
		ResourceType:  rbac.ResourceBotGroup,
		ResourceID:    row.GroupID.String(),
	})
}

func inheritedBotGroupPermission(permission rbac.PermissionKey) (rbac.PermissionKey, bool) {
	switch permission {
	case rbac.PermissionBotRead:
		return rbac.PermissionBotGroupRead, true
	case rbac.PermissionBotChat:
		return rbac.PermissionBotGroupUse, true
	case rbac.PermissionBotUpdate, rbac.PermissionBotDelete:
		return rbac.PermissionBotGroupBotsManage, true
	case rbac.PermissionBotPermissionsManage:
		return rbac.PermissionBotGroupPermissionsManage, true
	default:
		return "", false
	}
}

func (s *Service) ensureGroupCanAttach(ctx context.Context, userID string, groupID pgtype.UUID) error {
	if !groupID.Valid {
		return nil
	}
	group, err := s.queries.GetBotGroupByID(ctx, groupID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrBotGroupNotFound
		}
		return err
	}
	userUUID, err := db.ParseUUID(strings.TrimSpace(userID))
	if err != nil {
		return err
	}
	if group.OwnerUserID == userUUID {
		return nil
	}
	if s.rbac == nil {
		return ErrBotGroupNotAllowed
	}
	allowed, err := s.rbac.HasPermission(ctx, rbac.Check{
		UserID:        userID,
		PermissionKey: rbac.PermissionBotGroupBotsManage,
		ResourceType:  rbac.ResourceBotGroup,
		ResourceID:    groupID.String(),
	})
	if err != nil {
		return err
	}
	if allowed {
		return nil
	}
	return ErrBotGroupNotAllowed
}

func (s *Service) assignBotOwnerRole(ctx context.Context, ownerID pgtype.UUID, botID pgtype.UUID) error {
	role, err := s.queries.GetRoleByKey(ctx, string(rbac.RoleBotOwner))
	if err != nil {
		return err
	}
	_, err = s.queries.AssignPrincipalRole(ctx, sqlc.AssignPrincipalRoleParams{
		PrincipalType: string(rbac.PrincipalUser),
		PrincipalID:   ownerID,
		RoleID:        role.ID,
		ResourceType:  string(rbac.ResourceBot),
		ResourceID:    botID,
		Source:        string(rbac.SourceSystem),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	return err
}

func (s *Service) deleteBotOwnerRole(ctx context.Context, ownerID pgtype.UUID, botID pgtype.UUID) error {
	return s.queries.DeletePrincipalRoleAssignment(ctx, sqlc.DeletePrincipalRoleAssignmentParams{
		PrincipalType: string(rbac.PrincipalUser),
		PrincipalID:   ownerID,
		ResourceType:  string(rbac.ResourceBot),
		ResourceID:    botID,
		RoleKey:       string(rbac.RoleBotOwner),
	})
}

// Delete removes a bot and its associated resources.
func (s *Service) Delete(ctx context.Context, botID string) error {
	if s.queries == nil {
		return errors.New("bot queries not configured")
	}
	botUUID, err := db.ParseUUID(botID)
	if err != nil {
		return err
	}
	row, err := s.queries.GetBotByID(ctx, botUUID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(row.Status) == BotStatusDeleting {
		return nil
	}
	if err := s.queries.UpdateBotStatus(ctx, sqlc.UpdateBotStatusParams{
		ID:     botUUID,
		Status: BotStatusDeleting,
	}); err != nil {
		return err
	}
	s.enqueueDeleteLifecycle(ctx, botID)
	return nil
}

// ListChecks evaluates runtime resource checks for a bot.
func (s *Service) ListChecks(ctx context.Context, botID string) ([]BotCheck, error) {
	if s.queries == nil {
		return nil, errors.New("bot queries not configured")
	}
	botUUID, err := db.ParseUUID(botID)
	if err != nil {
		return nil, err
	}
	row, err := s.queries.GetBotByID(ctx, botUUID)
	if err != nil {
		return nil, err
	}
	return s.buildRuntimeChecks(ctx, asSQLCBot(row), true)
}

func (s *Service) enqueueCreateLifecycle(ctx context.Context, botID string) {
	go func() {
		lifecycleCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), botLifecycleOperationTimeout)
		defer cancel()

		if s.containerLifecycle != nil {
			if err := s.containerLifecycle.SetupBotContainer(lifecycleCtx, botID); err != nil {
				s.logger.Error("bot container setup failed",
					slog.String("bot_id", botID),
					slog.Any("error", err),
				)
			}
		}

		if err := s.updateStatus(lifecycleCtx, botID, BotStatusReady); err != nil {
			s.logger.Error("failed to update bot status to ready after create",
				slog.String("bot_id", botID),
				slog.Any("error", err),
			)
		}
	}()
}

func (s *Service) enqueueDeleteLifecycle(ctx context.Context, botID string) {
	go func() {
		lifecycleCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), botLifecycleOperationTimeout)
		defer cancel()

		if s.containerLifecycle != nil {
			if err := s.containerLifecycle.CleanupBotContainer(lifecycleCtx, botID, false); err != nil {
				s.logger.Error("bot container cleanup failed",
					slog.String("bot_id", botID),
					slog.Any("error", err),
				)
			}
		}

		botUUID, err := db.ParseUUID(botID)
		if err != nil {
			s.logger.Error("invalid bot id while finalizing delete",
				slog.String("bot_id", botID),
				slog.Any("error", err),
			)
			if err := s.updateStatus(lifecycleCtx, botID, BotStatusReady); err != nil {
				s.logger.Error("revert bot status failed", slog.String("bot_id", botID), slog.Any("error", err))
			}
			return
		}
		if err := s.queries.DeleteBotByID(lifecycleCtx, botUUID); err != nil {
			s.logger.Error("failed to delete bot after cleanup",
				slog.String("bot_id", botID),
				slog.Any("error", err),
			)
			if err := s.updateStatus(lifecycleCtx, botID, BotStatusReady); err != nil {
				s.logger.Error("revert bot status failed", slog.String("bot_id", botID), slog.Any("error", err))
			}
			return
		}
	}()
}

func (s *Service) updateStatus(ctx context.Context, botID, status string) error {
	if s.queries == nil {
		return errors.New("bot queries not configured")
	}
	botUUID, err := db.ParseUUID(botID)
	if err != nil {
		return err
	}
	return s.queries.UpdateBotStatus(ctx, sqlc.UpdateBotStatusParams{
		ID:     botUUID,
		Status: strings.TrimSpace(status),
	})
}

func (s *Service) ensureUserExists(ctx context.Context, userID pgtype.UUID) error {
	if s.queries == nil {
		return errors.New("bot queries not configured")
	}
	_, err := s.queries.GetUserByID(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrOwnerUserNotFound
		}
		return err
	}
	return nil
}

func (s *Service) ensureGroupBelongsToOwner(ctx context.Context, ownerID pgtype.UUID, groupID pgtype.UUID) error {
	if !groupID.Valid {
		return nil
	}
	_, err := s.queries.GetBotGroupByOwnerAndID(ctx, sqlc.GetBotGroupByOwnerAndIDParams{
		OwnerUserID: ownerID,
		ID:          groupID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrBotGroupNotAllowed
		}
		return err
	}
	return nil
}

func asSQLCBot(v any) sqlc.Bot {
	switch r := v.(type) {
	case sqlc.Bot:
		return r
	case sqlc.CreateBotRow:
		return sqlc.Bot{ID: r.ID, OwnerUserID: r.OwnerUserID, GroupID: r.GroupID, DisplayName: r.DisplayName, AvatarUrl: r.AvatarUrl, Timezone: r.Timezone, IsActive: r.IsActive, Status: r.Status, Language: r.Language, ReasoningEnabled: r.ReasoningEnabled, ReasoningEffort: r.ReasoningEffort, ChatModelID: r.ChatModelID, SearchProviderID: r.SearchProviderID, MemoryProviderID: r.MemoryProviderID, HeartbeatEnabled: r.HeartbeatEnabled, HeartbeatInterval: r.HeartbeatInterval, HeartbeatPrompt: r.HeartbeatPrompt, DisplayEnabled: r.DisplayEnabled, SettingsOverrideMask: r.SettingsOverrideMask, Metadata: r.Metadata, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
	case sqlc.GetBotByIDRow:
		return sqlc.Bot{ID: r.ID, OwnerUserID: r.OwnerUserID, GroupID: r.GroupID, DisplayName: r.DisplayName, AvatarUrl: r.AvatarUrl, Timezone: r.Timezone, IsActive: r.IsActive, Status: r.Status, Language: r.Language, ReasoningEnabled: r.ReasoningEnabled, ReasoningEffort: r.ReasoningEffort, ChatModelID: r.ChatModelID, SearchProviderID: r.SearchProviderID, MemoryProviderID: r.MemoryProviderID, HeartbeatEnabled: r.HeartbeatEnabled, HeartbeatInterval: r.HeartbeatInterval, HeartbeatPrompt: r.HeartbeatPrompt, DisplayEnabled: r.DisplayEnabled, CompactionEnabled: r.CompactionEnabled, CompactionThreshold: r.CompactionThreshold, CompactionModelID: r.CompactionModelID, SettingsOverrideMask: r.SettingsOverrideMask, Metadata: r.Metadata, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
	case sqlc.ListBotsByOwnerRow:
		return sqlc.Bot{ID: r.ID, OwnerUserID: r.OwnerUserID, GroupID: r.GroupID, DisplayName: r.DisplayName, AvatarUrl: r.AvatarUrl, Timezone: r.Timezone, IsActive: r.IsActive, Status: r.Status, Language: r.Language, ReasoningEnabled: r.ReasoningEnabled, ReasoningEffort: r.ReasoningEffort, ChatModelID: r.ChatModelID, SearchProviderID: r.SearchProviderID, MemoryProviderID: r.MemoryProviderID, HeartbeatEnabled: r.HeartbeatEnabled, HeartbeatInterval: r.HeartbeatInterval, HeartbeatPrompt: r.HeartbeatPrompt, DisplayEnabled: r.DisplayEnabled, SettingsOverrideMask: r.SettingsOverrideMask, Metadata: r.Metadata, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
	case sqlc.ListAccessibleBotsRow:
		return sqlc.Bot{ID: r.ID, OwnerUserID: r.OwnerUserID, GroupID: r.GroupID, DisplayName: r.DisplayName, AvatarUrl: r.AvatarUrl, Timezone: r.Timezone, IsActive: r.IsActive, Status: r.Status, Language: r.Language, ReasoningEnabled: r.ReasoningEnabled, ReasoningEffort: r.ReasoningEffort, ChatModelID: r.ChatModelID, SearchProviderID: r.SearchProviderID, MemoryProviderID: r.MemoryProviderID, HeartbeatEnabled: r.HeartbeatEnabled, HeartbeatInterval: r.HeartbeatInterval, HeartbeatPrompt: r.HeartbeatPrompt, DisplayEnabled: r.DisplayEnabled, SettingsOverrideMask: r.SettingsOverrideMask, Metadata: r.Metadata, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
	case sqlc.UpdateBotProfileRow:
		return sqlc.Bot{ID: r.ID, OwnerUserID: r.OwnerUserID, GroupID: r.GroupID, DisplayName: r.DisplayName, AvatarUrl: r.AvatarUrl, Timezone: r.Timezone, IsActive: r.IsActive, Status: r.Status, Language: r.Language, ReasoningEnabled: r.ReasoningEnabled, ReasoningEffort: r.ReasoningEffort, ChatModelID: r.ChatModelID, SearchProviderID: r.SearchProviderID, MemoryProviderID: r.MemoryProviderID, HeartbeatEnabled: r.HeartbeatEnabled, HeartbeatInterval: r.HeartbeatInterval, HeartbeatPrompt: r.HeartbeatPrompt, DisplayEnabled: r.DisplayEnabled, SettingsOverrideMask: r.SettingsOverrideMask, Metadata: r.Metadata, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
	case sqlc.UpdateBotOwnerRow:
		return sqlc.Bot{ID: r.ID, OwnerUserID: r.OwnerUserID, GroupID: r.GroupID, DisplayName: r.DisplayName, AvatarUrl: r.AvatarUrl, Timezone: r.Timezone, IsActive: r.IsActive, Status: r.Status, Language: r.Language, ReasoningEnabled: r.ReasoningEnabled, ReasoningEffort: r.ReasoningEffort, ChatModelID: r.ChatModelID, SearchProviderID: r.SearchProviderID, MemoryProviderID: r.MemoryProviderID, HeartbeatEnabled: r.HeartbeatEnabled, HeartbeatInterval: r.HeartbeatInterval, HeartbeatPrompt: r.HeartbeatPrompt, DisplayEnabled: r.DisplayEnabled, SettingsOverrideMask: r.SettingsOverrideMask, Metadata: r.Metadata, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
	default:
		return sqlc.Bot{}
	}
}

func toBot(row sqlc.Bot) (Bot, error) {
	displayName := ""
	if row.DisplayName.Valid {
		displayName = row.DisplayName.String
	}
	avatarURL := ""
	if row.AvatarUrl.Valid {
		avatarURL = row.AvatarUrl.String
	}
	timezoneName := ""
	if row.Timezone.Valid {
		timezoneName = row.Timezone.String
	}
	metadata, err := decodeMetadata(row.Metadata)
	if err != nil {
		return Bot{}, err
	}
	overrideMask, err := decodeOverrideMask(row.SettingsOverrideMask)
	if err != nil {
		return Bot{}, err
	}
	groupID := ""
	if row.GroupID.Valid {
		groupID = row.GroupID.String()
	}
	createdAt := time.Time{}
	if row.CreatedAt.Valid {
		createdAt = row.CreatedAt.Time
	}
	updatedAt := time.Time{}
	if row.UpdatedAt.Valid {
		updatedAt = row.UpdatedAt.Time
	}
	return Bot{
		ID:                   row.ID.String(),
		OwnerUserID:          row.OwnerUserID.String(),
		GroupID:              groupID,
		DisplayName:          displayName,
		AvatarURL:            avatarURL,
		Timezone:             timezoneName,
		IsActive:             row.IsActive,
		DisplayEnabled:       row.DisplayEnabled,
		Status:               strings.TrimSpace(row.Status),
		CheckState:           BotCheckStateUnknown,
		CheckIssueCount:      0,
		SettingsOverrideMask: overrideMask,
		Metadata:             metadata,
		CreatedAt:            createdAt,
		UpdatedAt:            updatedAt,
	}, nil
}

func decodeOverrideMask(payload []byte) (map[string]bool, error) {
	if len(payload) == 0 {
		return map[string]bool{}, nil
	}
	var data map[string]bool
	if err := json.Unmarshal(payload, &data); err != nil {
		return nil, err
	}
	if data == nil {
		data = map[string]bool{}
	}
	return data, nil
}

func createSettingsOverrideMask(hasGroup bool, hasExplicitTimezone bool) ([]byte, error) {
	if !hasGroup {
		return defaultSettingsOverrideMask, nil
	}
	if !hasExplicitTimezone {
		return inheritedSettingsOverrideMask, nil
	}
	var mask map[string]bool
	if err := json.Unmarshal(inheritedSettingsOverrideMask, &mask); err != nil {
		return nil, err
	}
	mask["timezone"] = true
	return json.Marshal(mask)
}

func normalizeOptionalUUID(raw string) (pgtype.UUID, error) {
	normalized := strings.TrimSpace(raw)
	if normalized == "" {
		return pgtype.UUID{}, nil
	}
	return db.ParseUUID(normalized)
}

func decodeMetadata(payload []byte) (map[string]any, error) {
	if len(payload) == 0 {
		return map[string]any{}, nil
	}
	var data map[string]any
	if err := json.Unmarshal(payload, &data); err != nil {
		return nil, err
	}
	if data == nil {
		data = map[string]any{}
	}
	return data, nil
}

func normalizeOptionalTimezone(raw *string) (pgtype.Text, error) {
	if raw == nil {
		return pgtype.Text{}, nil
	}
	normalized := strings.TrimSpace(*raw)
	if normalized == "" {
		return pgtype.Text{}, nil
	}
	loc, _, err := tzutil.Resolve(normalized)
	if err != nil {
		return pgtype.Text{}, fmt.Errorf("invalid timezone: %w", err)
	}
	return pgtype.Text{String: loc.String(), Valid: true}, nil
}

func (s *Service) attachCheckSummary(ctx context.Context, bot *Bot, row sqlc.Bot) error {
	checks, err := s.buildRuntimeChecks(ctx, row, false)
	if err != nil {
		return err
	}
	checkState, issueCount := summarizeChecks(checks)
	bot.CheckState = checkState
	bot.CheckIssueCount = issueCount
	return nil
}

// buildRuntimeChecks composes builtin checks and optional dynamic checker results.
// includeDynamic is disabled when computing list summary to avoid expensive runtime probes.
func (s *Service) buildRuntimeChecks(ctx context.Context, row sqlc.Bot, includeDynamic bool) ([]BotCheck, error) {
	status := strings.TrimSpace(row.Status)
	checks := make([]BotCheck, 0, 4)

	if status == BotStatusCreating {
		checks = append(checks, BotCheck{
			ID:       BotCheckTypeContainerInit,
			Type:     BotCheckTypeContainerInit,
			TitleKey: "bots.checks.titles.containerInit",
			Status:   BotCheckStatusUnknown,
			Summary:  "Initialization is in progress.",
			Detail:   "Bot resources are still being provisioned.",
		})
		checks = append(checks, BotCheck{
			ID:       BotCheckTypeContainerRecord,
			Type:     BotCheckTypeContainerRecord,
			TitleKey: "bots.checks.titles.containerRecord",
			Status:   BotCheckStatusUnknown,
			Summary:  "Container record is pending.",
			Detail:   "Container record will be checked after initialization.",
		})
		checks = append(checks, BotCheck{
			ID:       BotCheckTypeContainerTask,
			Type:     BotCheckTypeContainerTask,
			TitleKey: "bots.checks.titles.containerTask",
			Status:   BotCheckStatusUnknown,
			Summary:  "Container task state is pending.",
			Detail:   "Task state will be checked after initialization.",
		})
		checks = append(checks, BotCheck{
			ID:       BotCheckTypeContainerData,
			Type:     BotCheckTypeContainerData,
			TitleKey: "bots.checks.titles.containerDataPath",
			Status:   BotCheckStatusUnknown,
			Summary:  "Container reachability check is pending.",
			Detail:   "Reachability will be checked after initialization.",
		})
		if includeDynamic {
			checks = s.appendDynamicChecks(ctx, row.ID.String(), checks)
		}
		return checks, nil
	}
	if status == BotStatusDeleting {
		checks = append(checks, BotCheck{
			ID:       BotCheckTypeDelete,
			Type:     BotCheckTypeDelete,
			TitleKey: "bots.checks.titles.botDelete",
			Status:   BotCheckStatusUnknown,
			Summary:  "Deletion is in progress.",
			Detail:   "Bot resources are being cleaned up.",
		})
		checks = append(checks, BotCheck{
			ID:       BotCheckTypeContainerRecord,
			Type:     BotCheckTypeContainerRecord,
			TitleKey: "bots.checks.titles.containerRecord",
			Status:   BotCheckStatusUnknown,
			Summary:  "Container record check is skipped.",
			Detail:   "Bot is deleting and container checks are paused.",
		})
		checks = append(checks, BotCheck{
			ID:       BotCheckTypeContainerTask,
			Type:     BotCheckTypeContainerTask,
			TitleKey: "bots.checks.titles.containerTask",
			Status:   BotCheckStatusUnknown,
			Summary:  "Container task check is skipped.",
			Detail:   "Bot is deleting and task checks are paused.",
		})
		checks = append(checks, BotCheck{
			ID:       BotCheckTypeContainerData,
			Type:     BotCheckTypeContainerData,
			TitleKey: "bots.checks.titles.containerDataPath",
			Status:   BotCheckStatusUnknown,
			Summary:  "Container reachability check is skipped.",
			Detail:   "Bot is deleting and reachability checks are paused.",
		})
		if includeDynamic {
			checks = s.appendDynamicChecks(ctx, row.ID.String(), checks)
		}
		return checks, nil
	}

	checks = append(checks, BotCheck{
		ID:       BotCheckTypeContainerInit,
		Type:     BotCheckTypeContainerInit,
		TitleKey: "bots.checks.titles.containerInit",
		Status:   BotCheckStatusOK,
		Summary:  "Initialization finished.",
	})

	containerRow, err := s.queries.GetContainerByBotID(ctx, row.ID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			checks = append(checks, BotCheck{
				ID:       BotCheckTypeContainerRecord,
				Type:     BotCheckTypeContainerRecord,
				TitleKey: "bots.checks.titles.containerRecord",
				Status:   BotCheckStatusError,
				Summary:  "Container record is missing.",
				Detail:   "No container is attached to this bot.",
			})
			checks = append(checks, BotCheck{
				ID:       BotCheckTypeContainerTask,
				Type:     BotCheckTypeContainerTask,
				TitleKey: "bots.checks.titles.containerTask",
				Status:   BotCheckStatusUnknown,
				Summary:  "Container task state is unknown.",
				Detail:   "Task state cannot be determined without a container record.",
			})
			checks = append(checks, BotCheck{
				ID:       BotCheckTypeContainerData,
				Type:     BotCheckTypeContainerData,
				TitleKey: "bots.checks.titles.containerDataPath",
				Status:   BotCheckStatusUnknown,
				Summary:  "Container reachability is unknown.",
				Detail:   "Reachability cannot be determined without a container record.",
			})
			if includeDynamic {
				checks = s.appendDynamicChecks(ctx, row.ID.String(), checks)
			}
			return checks, nil
		}
		return nil, err
	}

	checks = append(checks, BotCheck{
		ID:       BotCheckTypeContainerRecord,
		Type:     BotCheckTypeContainerRecord,
		TitleKey: "bots.checks.titles.containerRecord",
		Status:   BotCheckStatusOK,
		Summary:  "Container record exists.",
		Detail:   fmt.Sprintf("container_id=%s", strings.TrimSpace(containerRow.ContainerID)),
		Metadata: map[string]any{
			"container_id": strings.TrimSpace(containerRow.ContainerID),
			"namespace":    strings.TrimSpace(containerRow.Namespace),
			"image":        strings.TrimSpace(containerRow.Image),
		},
	})

	taskStatus := strings.TrimSpace(strings.ToLower(containerRow.Status))
	taskCheck := BotCheck{
		ID:       BotCheckTypeContainerTask,
		Type:     BotCheckTypeContainerTask,
		TitleKey: "bots.checks.titles.containerTask",
		Status:   BotCheckStatusWarn,
		Summary:  "Container task state needs attention.",
	}
	switch taskStatus {
	case "running", "created", "stopped", "paused":
		taskCheck.Status = BotCheckStatusOK
		taskCheck.Summary = "Container task state is reported."
		taskCheck.Detail = fmt.Sprintf("status=%s", taskStatus)
	case "":
		taskCheck.Detail = "status is empty"
	default:
		taskCheck.Detail = fmt.Sprintf("unexpected status=%s", taskStatus)
	}
	taskCheck.Metadata = map[string]any{"status": taskStatus}
	checks = append(checks, taskCheck)

	dataCheck := BotCheck{
		ID:       BotCheckTypeContainerData,
		Type:     BotCheckTypeContainerData,
		TitleKey: "bots.checks.titles.containerDataPath",
		Status:   BotCheckStatusWarn,
		Summary:  "Container reachability needs attention.",
	}
	if s.containerReachability == nil {
		dataCheck.Status = BotCheckStatusUnknown
		dataCheck.Summary = "Container reachability check not configured."
	} else if err := s.containerReachability(ctx, row.ID.String()); err != nil {
		dataCheck.Status = BotCheckStatusError
		dataCheck.Summary = "Container is not reachable via gRPC."
		dataCheck.Detail = err.Error()
	} else {
		dataCheck.Status = BotCheckStatusOK
		dataCheck.Summary = "Container is reachable via gRPC."
	}
	checks = append(checks, dataCheck)
	if includeDynamic {
		checks = s.appendDynamicChecks(ctx, row.ID.String(), checks)
	}

	return checks, nil
}

// appendDynamicChecks appends checks from registered runtime checkers.
func (s *Service) appendDynamicChecks(ctx context.Context, botID string, checks []BotCheck) []BotCheck {
	for _, checker := range s.checkers {
		items := checker.ListChecks(ctx, botID)
		for _, item := range items {
			item.ID = strings.TrimSpace(item.ID)
			item.Type = strings.TrimSpace(item.Type)
			item.Status = strings.TrimSpace(item.Status)
			if item.ID == "" {
				if item.Type != "" {
					item.ID = item.Type
				} else {
					item.ID = "runtime.unknown"
					if s.logger != nil {
						s.logger.Warn("runtime checker returned check without id and type",
							slog.String("bot_id", botID))
					}
				}
			}
			if item.Type == "" {
				item.Type = item.ID
			}
			if item.Status == "" {
				item.Status = BotCheckStatusUnknown
			}
			checks = append(checks, item)
		}
	}
	return checks
}

func summarizeChecks(checks []BotCheck) (string, int32) {
	if len(checks) == 0 {
		return BotCheckStateUnknown, 0
	}
	var issueCount int32
	unknownCount := 0
	for _, check := range checks {
		switch check.Status {
		case BotCheckStatusWarn, BotCheckStatusError:
			issueCount++
		case BotCheckStatusUnknown:
			unknownCount++
		}
	}
	if issueCount > 0 {
		return BotCheckStateIssue, issueCount
	}
	if unknownCount == len(checks) {
		return BotCheckStateUnknown, 0
	}
	return BotCheckStateOK, 0
}
