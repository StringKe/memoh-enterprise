package integrations

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	dbstore "github.com/memohai/memoh/internal/db/store"
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
		logger:  log.With(slog.String("service", "integrations")),
	}
}

func (s *Service) CreateAPIToken(ctx context.Context, createdByUserID string, req CreateAPITokenRequest) (CreateAPITokenResult, error) {
	if s.queries == nil {
		return CreateAPITokenResult{}, errors.New("integration token queries not configured")
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return CreateAPITokenResult{}, errors.New("name is required")
	}
	scopeType := strings.TrimSpace(req.ScopeType)
	if scopeType == "" {
		scopeType = ScopeGlobal
	}
	scopeBotID, scopeBotGroupID, err := parseScope(scopeType, req.ScopeBotID, req.ScopeBotGroupID)
	if err != nil {
		return CreateAPITokenResult{}, err
	}
	rawToken, err := generateRawToken()
	if err != nil {
		return CreateAPITokenResult{}, err
	}
	events, err := encodeStringList(req.AllowedEventTypes)
	if err != nil {
		return CreateAPITokenResult{}, err
	}
	actions, err := encodeStringList(req.AllowedActionTypes)
	if err != nil {
		return CreateAPITokenResult{}, err
	}
	createdBy, err := optionalUUID(createdByUserID)
	if err != nil {
		return CreateAPITokenResult{}, err
	}
	expiresAt := pgtype.Timestamptz{}
	if req.ExpiresAt != nil {
		expiresAt = pgtype.Timestamptz{Time: req.ExpiresAt.UTC(), Valid: true}
	}
	row, err := s.queries.CreateIntegrationAPIToken(ctx, sqlc.CreateIntegrationAPITokenParams{
		Name:               name,
		TokenHash:          HashToken(rawToken),
		ScopeType:          scopeType,
		ScopeBotID:         scopeBotID,
		ScopeBotGroupID:    scopeBotGroupID,
		AllowedEventTypes:  events,
		AllowedActionTypes: actions,
		ExpiresAt:          expiresAt,
		CreatedByUserID:    createdBy,
	})
	if err != nil {
		return CreateAPITokenResult{}, err
	}
	token, err := toAPIToken(row)
	if err != nil {
		return CreateAPITokenResult{}, err
	}
	return CreateAPITokenResult{Token: token, RawToken: rawToken}, nil
}

func (s *Service) ListAPITokens(ctx context.Context) ([]APIToken, error) {
	rows, err := s.queries.ListIntegrationAPITokens(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]APIToken, 0, len(rows))
	for _, row := range rows {
		item, err := toAPIToken(row)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *Service) DisableAPIToken(ctx context.Context, id string) (APIToken, error) {
	tokenID, err := db.ParseUUID(id)
	if err != nil {
		return APIToken{}, err
	}
	row, err := s.queries.DisableIntegrationAPIToken(ctx, tokenID)
	if err != nil {
		return APIToken{}, err
	}
	return toAPIToken(row)
}

func (s *Service) DeleteAPIToken(ctx context.Context, id string) error {
	tokenID, err := db.ParseUUID(id)
	if err != nil {
		return err
	}
	return s.queries.DeleteIntegrationAPIToken(ctx, tokenID)
}

func (s *Service) GetAPIToken(ctx context.Context, id string) (APIToken, error) {
	if s.queries == nil {
		return APIToken{}, errors.New("integration token queries not configured")
	}
	tokenID, err := db.ParseUUID(id)
	if err != nil {
		return APIToken{}, err
	}
	row, err := s.queries.GetIntegrationAPITokenByID(ctx, tokenID)
	if err != nil {
		return APIToken{}, err
	}
	token, err := toAPIToken(row)
	if err != nil {
		return APIToken{}, err
	}
	return ensureAPITokenActive(token)
}

func (s *Service) DisableAllAPITokens(ctx context.Context) error {
	return s.queries.DisableAllIntegrationAPITokens(ctx)
}

func (s *Service) ValidateToken(ctx context.Context, rawToken string) (TokenIdentity, error) {
	if s.queries == nil {
		return TokenIdentity{}, errors.New("integration token queries not configured")
	}
	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" {
		return TokenIdentity{}, errors.New("token is required")
	}
	row, err := s.queries.GetIntegrationAPITokenByHash(ctx, HashToken(rawToken))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return TokenIdentity{}, errors.New("invalid integration token")
		}
		return TokenIdentity{}, err
	}
	token, err := toAPIToken(row)
	if err != nil {
		return TokenIdentity{}, err
	}
	token, err = ensureAPITokenActive(token)
	if err != nil {
		return TokenIdentity{}, err
	}
	if err := s.queries.TouchIntegrationAPITokenUsed(ctx, row.ID); err != nil {
		return TokenIdentity{}, err
	}
	return TokenIdentity{Token: token}, nil
}

func ensureAPITokenActive(token APIToken) (APIToken, error) {
	if token.DisabledAt != nil {
		return APIToken{}, errors.New("integration token is disabled")
	}
	if token.ExpiresAt != nil && time.Now().After(token.ExpiresAt.UTC()) {
		return APIToken{}, errors.New("integration token is expired")
	}
	return token, nil
}

func (s *Service) AuthorizeBot(ctx context.Context, identity TokenIdentity, botID string, action string) error {
	token := identity.Token
	if !listAllows(token.AllowedActionTypes, action) {
		return errors.New("integration token action is not allowed")
	}
	switch token.ScopeType {
	case ScopeGlobal:
		return nil
	case ScopeBot:
		if strings.TrimSpace(botID) == strings.TrimSpace(token.ScopeBotID) {
			return nil
		}
		return errors.New("integration token does not allow this bot")
	case ScopeBotGroup:
		botUUID, err := db.ParseUUID(botID)
		if err != nil {
			return err
		}
		row, err := s.queries.GetBotByID(ctx, botUUID)
		if err != nil {
			return err
		}
		if row.GroupID.Valid && row.GroupID.String() == strings.TrimSpace(token.ScopeBotGroupID) {
			return nil
		}
		return errors.New("integration token does not allow this bot group")
	default:
		return errors.New("integration token scope is invalid")
	}
}

func (*Service) AuthorizeBotGroup(identity TokenIdentity, botGroupID string) error {
	botGroupID = strings.TrimSpace(botGroupID)
	if botGroupID == "" {
		return errors.New("bot_group_id is required")
	}
	switch identity.Token.ScopeType {
	case ScopeGlobal:
		return nil
	case ScopeBotGroup:
		if botGroupID == strings.TrimSpace(identity.Token.ScopeBotGroupID) {
			return nil
		}
		return errors.New("integration token does not allow this bot group")
	case ScopeBot:
		return errors.New("integration token does not allow bot group subscriptions")
	default:
		return errors.New("integration token scope is invalid")
	}
}

func (*Service) AuthorizeEvent(identity TokenIdentity, eventType string) error {
	if !listAllows(identity.Token.AllowedEventTypes, eventType) {
		return errors.New("integration token event is not allowed")
	}
	return nil
}

func listAllows(allowed []string, value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(allowed) == 0 {
		return true
	}
	for _, item := range allowed {
		item = strings.TrimSpace(item)
		if item == "*" || item == value {
			return true
		}
	}
	return false
}

func parseScope(scopeType, botID, botGroupID string) (pgtype.UUID, pgtype.UUID, error) {
	switch scopeType {
	case ScopeGlobal:
		return pgtype.UUID{}, pgtype.UUID{}, nil
	case ScopeBot:
		parsed, err := db.ParseUUID(strings.TrimSpace(botID))
		if err != nil {
			return pgtype.UUID{}, pgtype.UUID{}, errors.New("scope_bot_id is required for bot scope")
		}
		return parsed, pgtype.UUID{}, nil
	case ScopeBotGroup:
		parsed, err := db.ParseUUID(strings.TrimSpace(botGroupID))
		if err != nil {
			return pgtype.UUID{}, pgtype.UUID{}, errors.New("scope_bot_group_id is required for bot_group scope")
		}
		return pgtype.UUID{}, parsed, nil
	default:
		return pgtype.UUID{}, pgtype.UUID{}, errors.New("scope_type must be global, bot, or bot_group")
	}
}

func optionalUUID(raw string) (pgtype.UUID, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return pgtype.UUID{}, nil
	}
	return db.ParseUUID(raw)
}

func encodeStringList(items []string) ([]byte, error) {
	if items == nil {
		items = []string{}
	}
	normalized := make([]string, 0, len(items))
	for _, item := range items {
		if value := strings.TrimSpace(item); value != "" {
			normalized = append(normalized, value)
		}
	}
	return json.Marshal(normalized)
}

func decodeStringList(payload []byte) ([]string, error) {
	if len(payload) == 0 {
		return []string{}, nil
	}
	var items []string
	if err := json.Unmarshal(payload, &items); err != nil {
		return nil, err
	}
	if items == nil {
		items = []string{}
	}
	return items, nil
}

func toAPIToken(row sqlc.IntegrationApiToken) (APIToken, error) {
	events, err := decodeStringList(row.AllowedEventTypes)
	if err != nil {
		return APIToken{}, err
	}
	actions, err := decodeStringList(row.AllowedActionTypes)
	if err != nil {
		return APIToken{}, err
	}
	return APIToken{
		ID:                 row.ID.String(),
		Name:               row.Name,
		ScopeType:          row.ScopeType,
		ScopeBotID:         uuidString(row.ScopeBotID),
		ScopeBotGroupID:    uuidString(row.ScopeBotGroupID),
		AllowedEventTypes:  events,
		AllowedActionTypes: actions,
		ExpiresAt:          timePtr(row.ExpiresAt),
		DisabledAt:         timePtr(row.DisabledAt),
		LastUsedAt:         timePtr(row.LastUsedAt),
		CreatedByUserID:    uuidString(row.CreatedByUserID),
		CreatedAt:          timeValue(row.CreatedAt),
		UpdatedAt:          timeValue(row.UpdatedAt),
	}, nil
}

func uuidString(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	return uuid.UUID(value.Bytes).String()
}

func timeValue(value pgtype.Timestamptz) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time
}

func timePtr(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time
	return &result
}
