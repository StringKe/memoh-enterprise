package structureddata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/memohai/memoh/internal/db"
	dbsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
	postgresstore "github.com/memohai/memoh/internal/db/postgres/store"
)

const (
	defaultMaxRows  = int32(500)
	absoluteMaxRows = int32(5000)
)

var generatedIdentifierPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

type Service struct {
	logger  *slog.Logger
	pool    *pgxpool.Pool
	queries *dbsqlc.Queries
}

func NewService(log *slog.Logger, store *postgresstore.Store) (*Service, error) {
	if log == nil {
		log = slog.Default()
	}
	if store == nil || store.Pool() == nil || store.SQLC() == nil {
		return nil, ErrDependencyMissing
	}
	return &Service{
		logger:  log.With(slog.String("service", "structured_data")),
		pool:    store.Pool(),
		queries: store.SQLC(),
	}, nil
}

func (s *Service) EnsureSpace(ctx context.Context, owner OwnerRef) (dbsqlc.StructuredDataSpace, error) {
	if s == nil || s.queries == nil || s.pool == nil {
		return dbsqlc.StructuredDataSpace{}, ErrDependencyMissing
	}
	normalized, ownerID, err := normalizeOwner(owner)
	if err != nil {
		return dbsqlc.StructuredDataSpace{}, err
	}
	existing, err := s.getSpaceByOwner(ctx, normalized)
	if err == nil {
		return existing, s.ensureDatabaseSpace(ctx, existing)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return dbsqlc.StructuredDataSpace{}, err
	}

	schemaName, roleName := generatedNames(normalized.Type, ownerID)
	space, err := s.queries.CreateStructuredDataSpace(ctx, dbsqlc.CreateStructuredDataSpaceParams{
		OwnerType:       string(normalized.Type),
		OwnerBotID:      nullableUUID(normalized.BotID),
		OwnerBotGroupID: nullableUUID(normalized.BotGroupID),
		SchemaName:      schemaName,
		RoleName:        roleName,
		DisplayName:     defaultDisplayName(normalized.Type),
		Metadata:        []byte("{}"),
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			space, readErr := s.getSpaceByOwner(ctx, normalized)
			if readErr != nil {
				return dbsqlc.StructuredDataSpace{}, readErr
			}
			return space, s.ensureDatabaseSpace(ctx, space)
		}
		return dbsqlc.StructuredDataSpace{}, err
	}
	if err := s.ensureDatabaseSpace(ctx, space); err != nil {
		return dbsqlc.StructuredDataSpace{}, err
	}
	_ = s.audit(ctx, auditInput{SpaceID: space.ID, ActorType: "system", Operation: "ensure_space", Success: true})
	return space, nil
}

func (s *Service) ListSpaces(ctx context.Context) ([]dbsqlc.StructuredDataSpace, error) {
	if s == nil || s.queries == nil {
		return nil, ErrDependencyMissing
	}
	return s.queries.ListStructuredDataSpaces(ctx)
}

func (s *Service) ListSpacesForBot(ctx context.Context, botID string) ([]dbsqlc.StructuredDataSpace, error) {
	botUUID, err := db.ParseUUID(strings.TrimSpace(botID))
	if err != nil {
		return nil, err
	}
	bot, err := s.queries.GetBotByID(ctx, botUUID)
	if err != nil {
		return nil, err
	}
	if _, err := s.EnsureSpace(ctx, OwnerRef{Type: OwnerTypeBot, BotID: bot.ID.String()}); err != nil {
		return nil, err
	}
	if bot.GroupID.Valid {
		if _, err := s.EnsureSpace(ctx, OwnerRef{Type: OwnerTypeBotGroup, BotGroupID: bot.GroupID.String()}); err != nil {
			return nil, err
		}
	}
	return s.queries.ListStructuredDataSpacesForBotActor(ctx, dbsqlc.ListStructuredDataSpacesForBotActorParams{
		BotID:      bot.ID,
		BotGroupID: bot.GroupID,
	})
}

func (s *Service) DescribeSpace(ctx context.Context, spaceID string) (dbsqlc.StructuredDataSpace, []Table, error) {
	space, err := s.GetSpace(ctx, spaceID)
	if err != nil {
		return dbsqlc.StructuredDataSpace{}, nil, err
	}
	if err := s.ensureDatabaseSpace(ctx, space); err != nil {
		return dbsqlc.StructuredDataSpace{}, nil, err
	}
	rows, err := s.pool.Query(ctx, `
SELECT
  c.table_schema,
  c.table_name,
  c.column_name,
  c.data_type,
  c.is_nullable,
  COALESCE(c.column_default, '')
FROM information_schema.columns c
JOIN information_schema.tables t
  ON t.table_schema = c.table_schema
 AND t.table_name = c.table_name
WHERE c.table_schema = $1
  AND t.table_type = 'BASE TABLE'
ORDER BY c.table_name ASC, c.ordinal_position ASC
`, space.SchemaName)
	if err != nil {
		return dbsqlc.StructuredDataSpace{}, nil, err
	}
	defer rows.Close()

	byName := map[string]*Table{}
	order := []string{}
	for rows.Next() {
		var schemaName, tableName, columnName, dataType, nullable, defaultValue string
		if err := rows.Scan(&schemaName, &tableName, &columnName, &dataType, &nullable, &defaultValue); err != nil {
			return dbsqlc.StructuredDataSpace{}, nil, err
		}
		table := byName[tableName]
		if table == nil {
			table = &Table{SchemaName: schemaName, Name: tableName}
			byName[tableName] = table
			order = append(order, tableName)
		}
		table.Columns = append(table.Columns, Column{
			Name:         columnName,
			Type:         dataType,
			Nullable:     nullable == "YES",
			DefaultValue: defaultValue,
		})
	}
	if err := rows.Err(); err != nil {
		return dbsqlc.StructuredDataSpace{}, nil, err
	}
	tables := make([]Table, 0, len(order))
	for _, name := range order {
		tables = append(tables, *byName[name])
	}
	return space, tables, nil
}

func (s *Service) GetSpace(ctx context.Context, spaceID string) (dbsqlc.StructuredDataSpace, error) {
	pgID, err := db.ParseUUID(strings.TrimSpace(spaceID))
	if err != nil {
		return dbsqlc.StructuredDataSpace{}, err
	}
	return s.queries.GetStructuredDataSpaceByID(ctx, pgID)
}

func (s *Service) UpsertGrant(ctx context.Context, input GrantInput) (dbsqlc.StructuredDataGrant, error) {
	space, err := s.GetSpace(ctx, input.SpaceID)
	if err != nil {
		return dbsqlc.StructuredDataGrant{}, err
	}
	if err := s.ensureDatabaseSpace(ctx, space); err != nil {
		return dbsqlc.StructuredDataGrant{}, err
	}
	privileges, err := normalizePrivileges(input.Privileges)
	if err != nil {
		return dbsqlc.StructuredDataGrant{}, err
	}
	targetRole, err := s.ensureTargetRole(ctx, input.TargetType, input.TargetBotID, input.TargetBotGroupID)
	if err != nil {
		return dbsqlc.StructuredDataGrant{}, err
	}
	if err := s.applyGrant(ctx, space, targetRole, privileges); err != nil {
		return dbsqlc.StructuredDataGrant{}, err
	}
	actorID := nullableUUID(input.ActorUserID)
	var grant dbsqlc.StructuredDataGrant
	switch input.TargetType {
	case TargetTypeBot:
		targetID, parseErr := db.ParseUUID(strings.TrimSpace(input.TargetBotID))
		if parseErr != nil {
			return dbsqlc.StructuredDataGrant{}, parseErr
		}
		created, upsertErr := s.queries.UpsertStructuredDataGrantForBot(ctx, dbsqlc.UpsertStructuredDataGrantForBotParams{
			SpaceID:         space.ID,
			TargetBotID:     targetID,
			Privileges:      privileges,
			CreatedByUserID: actorID,
		})
		if upsertErr != nil {
			return dbsqlc.StructuredDataGrant{}, upsertErr
		}
		grant = created
	case TargetTypeBotGroup:
		targetID, parseErr := db.ParseUUID(strings.TrimSpace(input.TargetBotGroupID))
		if parseErr != nil {
			return dbsqlc.StructuredDataGrant{}, parseErr
		}
		created, upsertErr := s.queries.UpsertStructuredDataGrantForBotGroup(ctx, dbsqlc.UpsertStructuredDataGrantForBotGroupParams{
			SpaceID:          space.ID,
			TargetBotGroupID: targetID,
			Privileges:       privileges,
			CreatedByUserID:  actorID,
		})
		if upsertErr != nil {
			return dbsqlc.StructuredDataGrant{}, upsertErr
		}
		grant = created
	default:
		return dbsqlc.StructuredDataGrant{}, ErrInvalidTarget
	}
	_ = s.audit(ctx, auditInput{SpaceID: space.ID, ActorType: "user", ActorUserID: actorID, Operation: "grant", Success: true})
	return grant, nil
}

func (s *Service) ListGrants(ctx context.Context, spaceID string) ([]dbsqlc.StructuredDataGrant, error) {
	pgID, err := db.ParseUUID(strings.TrimSpace(spaceID))
	if err != nil {
		return nil, err
	}
	return s.queries.ListStructuredDataGrantsBySpace(ctx, pgID)
}

func (s *Service) GetGrant(ctx context.Context, grantID string) (dbsqlc.StructuredDataGrant, error) {
	pgID, err := db.ParseUUID(strings.TrimSpace(grantID))
	if err != nil {
		return dbsqlc.StructuredDataGrant{}, err
	}
	return s.queries.GetStructuredDataGrantByID(ctx, pgID)
}

func (s *Service) DeleteGrant(ctx context.Context, grantID string, actorUserID string) error {
	pgID, err := db.ParseUUID(strings.TrimSpace(grantID))
	if err != nil {
		return err
	}
	grant, err := s.queries.GetStructuredDataGrantByID(ctx, pgID)
	if err != nil {
		return err
	}
	space, err := s.queries.GetStructuredDataSpaceByID(ctx, grant.SpaceID)
	if err != nil {
		return err
	}
	targetRole, err := s.targetRoleForGrant(ctx, grant)
	if err != nil {
		return err
	}
	if err := s.revokeGrant(ctx, space, targetRole); err != nil {
		return err
	}
	if err := s.queries.DeleteStructuredDataGrant(ctx, pgID); err != nil {
		return err
	}
	_ = s.audit(ctx, auditInput{SpaceID: space.ID, ActorType: "user", ActorUserID: nullableUUID(actorUserID), Operation: "revoke", Success: true})
	return nil
}

func (s *Service) ExecuteAsOwner(ctx context.Context, input ExecuteInput) (SQLResult, error) {
	space, err := s.GetSpace(ctx, input.SpaceID)
	if err != nil {
		return SQLResult{}, err
	}
	if err := s.ensureDatabaseSpace(ctx, space); err != nil {
		return SQLResult{}, err
	}
	input.ActorType = "user"
	input.ExecutionRole = space.RoleName
	input.SearchPathSpace = space.SchemaName
	return s.execute(ctx, space, input)
}

func (s *Service) ExecuteForBot(ctx context.Context, input ExecuteInput) (SQLResult, error) {
	space, role, err := s.resolveBotExecution(ctx, input)
	if err != nil {
		return SQLResult{}, err
	}
	input.ActorType = "bot"
	input.ExecutionRole = role
	input.SearchPathSpace = space.SchemaName
	return s.execute(ctx, space, input)
}

func (s *Service) resolveBotExecution(ctx context.Context, input ExecuteInput) (dbsqlc.StructuredDataSpace, string, error) {
	actorBotID, err := db.ParseUUID(strings.TrimSpace(input.ActorBotID))
	if err != nil {
		return dbsqlc.StructuredDataSpace{}, "", err
	}
	bot, err := s.queries.GetBotByID(ctx, actorBotID)
	if err != nil {
		return dbsqlc.StructuredDataSpace{}, "", err
	}

	var space dbsqlc.StructuredDataSpace
	switch {
	case strings.TrimSpace(input.SpaceID) != "":
		space, err = s.GetSpace(ctx, input.SpaceID)
	case input.Owner.Type != "":
		space, err = s.EnsureSpace(ctx, input.Owner)
	default:
		space, err = s.EnsureSpace(ctx, OwnerRef{Type: OwnerTypeBot, BotID: bot.ID.String()})
	}
	if err != nil {
		return dbsqlc.StructuredDataSpace{}, "", err
	}
	if err := s.ensureDatabaseSpace(ctx, space); err != nil {
		return dbsqlc.StructuredDataSpace{}, "", err
	}

	if space.OwnerType == string(OwnerTypeBot) && space.OwnerBotID.Valid && space.OwnerBotID == bot.ID {
		return space, space.RoleName, nil
	}
	if space.OwnerType == string(OwnerTypeBotGroup) && bot.GroupID.Valid && space.OwnerBotGroupID == bot.GroupID {
		return space, space.RoleName, nil
	}

	grants, err := s.queries.ListStructuredDataGrantsBySpace(ctx, space.ID)
	if err != nil {
		return dbsqlc.StructuredDataSpace{}, "", err
	}
	for _, grant := range grants {
		if grant.TargetType == string(TargetTypeBot) && grant.TargetBotID.Valid && grant.TargetBotID == bot.ID {
			roleSpace, err := s.EnsureSpace(ctx, OwnerRef{Type: OwnerTypeBot, BotID: bot.ID.String()})
			if err != nil {
				return dbsqlc.StructuredDataSpace{}, "", err
			}
			return space, roleSpace.RoleName, nil
		}
		if grant.TargetType == string(TargetTypeBotGroup) && bot.GroupID.Valid && grant.TargetBotGroupID.Valid && grant.TargetBotGroupID == bot.GroupID {
			roleSpace, err := s.EnsureSpace(ctx, OwnerRef{Type: OwnerTypeBotGroup, BotGroupID: bot.GroupID.String()})
			if err != nil {
				return dbsqlc.StructuredDataSpace{}, "", err
			}
			return space, roleSpace.RoleName, nil
		}
	}
	return dbsqlc.StructuredDataSpace{}, "", ErrAccessDenied
}

func (s *Service) execute(ctx context.Context, space dbsqlc.StructuredDataSpace, input ExecuteInput) (SQLResult, error) {
	sqlText := strings.TrimSpace(input.SQL)
	if sqlText == "" {
		return SQLResult{}, ErrSQLRequired
	}
	maxRows := normalizeMaxRows(input.MaxRows)
	started := time.Now()
	result, err := s.executeSQL(ctx, space, input.ExecutionRole, input.SearchPathSpace, sqlText, maxRows)
	duration := time.Since(started).Milliseconds()
	auditErr := ""
	if err != nil {
		auditErr = err.Error()
	}
	_ = s.audit(ctx, auditInput{
		SpaceID:     space.ID,
		ActorType:   input.ActorType,
		ActorUserID: nullableUUID(input.ActorUserID),
		ActorBotID:  nullableUUID(input.ActorBotID),
		Operation:   "execute_sql",
		Statement:   sqlText,
		Success:     err == nil,
		Error:       auditErr,
		RowCount:    result.RowCount,
		DurationMs:  duration,
	})
	return result, err
}

func (s *Service) executeSQL(ctx context.Context, space dbsqlc.StructuredDataSpace, roleName string, schemaName string, sqlText string, maxRows int32) (SQLResult, error) {
	if err := validateGeneratedIdentifier(roleName); err != nil {
		return SQLResult{}, err
	}
	if err := validateGeneratedIdentifier(schemaName); err != nil {
		return SQLResult{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return SQLResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, "SET LOCAL ROLE "+quoteIdent(roleName)); err != nil {
		return SQLResult{}, err
	}
	if _, err := tx.Exec(ctx, "SELECT set_config('search_path', $1, true)", quoteIdent(schemaName)); err != nil {
		return SQLResult{}, err
	}
	rows, err := tx.Query(ctx, sqlText, pgx.QueryExecModeSimpleProtocol)
	if err != nil {
		return SQLResult{}, err
	}
	defer rows.Close()

	fields := rows.FieldDescriptions()
	columns := make([]string, 0, len(fields))
	for _, field := range fields {
		columns = append(columns, field.Name)
	}
	outRows := make([]map[string]any, 0)
	truncated := false
	for rows.Next() {
		if len(outRows) >= int(maxRows) {
			truncated = true
			continue
		}
		values, err := rows.Values()
		if err != nil {
			return SQLResult{}, err
		}
		row := make(map[string]any, len(columns))
		for idx, column := range columns {
			row[column] = normalizeValue(values[idx])
		}
		outRows = append(outRows, row)
	}
	if err := rows.Err(); err != nil {
		return SQLResult{}, err
	}
	tag := rows.CommandTag()
	if err := tx.Commit(ctx); err != nil {
		return SQLResult{}, err
	}
	rowCount := tag.RowsAffected()
	if len(columns) > 0 {
		rowCount = int64(len(outRows))
	}
	return SQLResult{
		Columns:    columns,
		Rows:       outRows,
		RowCount:   rowCount,
		CommandTag: tag.String(),
		Truncated:  truncated,
		SchemaName: space.SchemaName,
	}, nil
}

func (s *Service) getSpaceByOwner(ctx context.Context, owner OwnerRef) (dbsqlc.StructuredDataSpace, error) {
	switch owner.Type {
	case OwnerTypeBot:
		id, err := db.ParseUUID(owner.BotID)
		if err != nil {
			return dbsqlc.StructuredDataSpace{}, err
		}
		return s.queries.GetStructuredDataSpaceByBot(ctx, id)
	case OwnerTypeBotGroup:
		id, err := db.ParseUUID(owner.BotGroupID)
		if err != nil {
			return dbsqlc.StructuredDataSpace{}, err
		}
		return s.queries.GetStructuredDataSpaceByBotGroup(ctx, id)
	default:
		return dbsqlc.StructuredDataSpace{}, ErrInvalidOwner
	}
}

func (s *Service) ensureDatabaseSpace(ctx context.Context, space dbsqlc.StructuredDataSpace) error {
	if err := validateGeneratedIdentifier(space.SchemaName); err != nil {
		return err
	}
	if err := validateGeneratedIdentifier(space.RoleName); err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	exists, err := roleExists(ctx, tx, space.RoleName)
	if err != nil {
		return err
	}
	if !exists {
		if _, err := tx.Exec(ctx, "CREATE ROLE "+quoteIdent(space.RoleName)+" NOLOGIN"); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, "GRANT "+quoteIdent(space.RoleName)+" TO CURRENT_USER"); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS "+quoteIdent(space.SchemaName)+" AUTHORIZATION "+quoteIdent(space.RoleName)); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "ALTER SCHEMA "+quoteIdent(space.SchemaName)+" OWNER TO "+quoteIdent(space.RoleName)); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "GRANT USAGE, CREATE ON SCHEMA "+quoteIdent(space.SchemaName)+" TO "+quoteIdent(space.RoleName)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) ensureTargetRole(ctx context.Context, targetType TargetType, targetBotID string, targetBotGroupID string) (string, error) {
	switch targetType {
	case TargetTypeBot:
		space, err := s.EnsureSpace(ctx, OwnerRef{Type: OwnerTypeBot, BotID: targetBotID})
		if err != nil {
			return "", err
		}
		return space.RoleName, nil
	case TargetTypeBotGroup:
		space, err := s.EnsureSpace(ctx, OwnerRef{Type: OwnerTypeBotGroup, BotGroupID: targetBotGroupID})
		if err != nil {
			return "", err
		}
		return space.RoleName, nil
	default:
		return "", ErrInvalidTarget
	}
}

func (s *Service) targetRoleForGrant(ctx context.Context, grant dbsqlc.StructuredDataGrant) (string, error) {
	switch grant.TargetType {
	case string(TargetTypeBot):
		if !grant.TargetBotID.Valid {
			return "", ErrInvalidTarget
		}
		return s.ensureTargetRole(ctx, TargetTypeBot, grant.TargetBotID.String(), "")
	case string(TargetTypeBotGroup):
		if !grant.TargetBotGroupID.Valid {
			return "", ErrInvalidTarget
		}
		return s.ensureTargetRole(ctx, TargetTypeBotGroup, "", grant.TargetBotGroupID.String())
	default:
		return "", ErrInvalidTarget
	}
}

func (s *Service) applyGrant(ctx context.Context, space dbsqlc.StructuredDataSpace, targetRole string, privileges []string) error {
	if err := validateGeneratedIdentifier(targetRole); err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := revokeGrantTx(ctx, tx, space, targetRole); err != nil {
		return err
	}
	schema := quoteIdent(space.SchemaName)
	role := quoteIdent(targetRole)
	ownerRole := quoteIdent(space.RoleName)
	if _, err := tx.Exec(ctx, "GRANT USAGE ON SCHEMA "+schema+" TO "+role); err != nil {
		return err
	}
	privSet := privilegeSet(privileges)
	if privSet[PrivilegeRead] || privSet[PrivilegeWrite] {
		if _, err := tx.Exec(ctx, "GRANT SELECT ON ALL TABLES IN SCHEMA "+schema+" TO "+role); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA "+schema+" TO "+role); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "ALTER DEFAULT PRIVILEGES FOR ROLE "+ownerRole+" IN SCHEMA "+schema+" GRANT SELECT ON TABLES TO "+role); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "ALTER DEFAULT PRIVILEGES FOR ROLE "+ownerRole+" IN SCHEMA "+schema+" GRANT USAGE, SELECT ON SEQUENCES TO "+role); err != nil {
			return err
		}
	}
	if privSet[PrivilegeWrite] {
		if _, err := tx.Exec(ctx, "GRANT INSERT, UPDATE, DELETE, TRUNCATE ON ALL TABLES IN SCHEMA "+schema+" TO "+role); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "GRANT UPDATE ON ALL SEQUENCES IN SCHEMA "+schema+" TO "+role); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "ALTER DEFAULT PRIVILEGES FOR ROLE "+ownerRole+" IN SCHEMA "+schema+" GRANT INSERT, UPDATE, DELETE, TRUNCATE ON TABLES TO "+role); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "ALTER DEFAULT PRIVILEGES FOR ROLE "+ownerRole+" IN SCHEMA "+schema+" GRANT UPDATE ON SEQUENCES TO "+role); err != nil {
			return err
		}
	}
	if privSet[PrivilegeDDL] {
		if _, err := tx.Exec(ctx, "GRANT CREATE ON SCHEMA "+schema+" TO "+role); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Service) revokeGrant(ctx context.Context, space dbsqlc.StructuredDataSpace, targetRole string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := revokeGrantTx(ctx, tx, space, targetRole); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func revokeGrantTx(ctx context.Context, tx pgx.Tx, space dbsqlc.StructuredDataSpace, targetRole string) error {
	if err := validateGeneratedIdentifier(space.SchemaName); err != nil {
		return err
	}
	if err := validateGeneratedIdentifier(space.RoleName); err != nil {
		return err
	}
	if err := validateGeneratedIdentifier(targetRole); err != nil {
		return err
	}
	schema := quoteIdent(space.SchemaName)
	role := quoteIdent(targetRole)
	ownerRole := quoteIdent(space.RoleName)
	statements := []string{
		"REVOKE ALL PRIVILEGES ON ALL TABLES IN SCHEMA " + schema + " FROM " + role,
		"REVOKE ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA " + schema + " FROM " + role,
		"REVOKE CREATE ON SCHEMA " + schema + " FROM " + role,
		"REVOKE USAGE ON SCHEMA " + schema + " FROM " + role,
		"ALTER DEFAULT PRIVILEGES FOR ROLE " + ownerRole + " IN SCHEMA " + schema + " REVOKE ALL ON TABLES FROM " + role,
		"ALTER DEFAULT PRIVILEGES FOR ROLE " + ownerRole + " IN SCHEMA " + schema + " REVOKE ALL ON SEQUENCES FROM " + role,
	}
	for _, statement := range statements {
		if _, err := tx.Exec(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

type auditInput struct {
	SpaceID     pgtype.UUID
	ActorType   string
	ActorUserID pgtype.UUID
	ActorBotID  pgtype.UUID
	Operation   string
	Statement   string
	Success     bool
	Error       string
	RowCount    int64
	DurationMs  int64
}

func (s *Service) audit(ctx context.Context, input auditInput) error {
	if s == nil || s.queries == nil {
		return nil
	}
	actorType := strings.TrimSpace(input.ActorType)
	if actorType == "" {
		actorType = "system"
	}
	_, err := s.queries.CreateStructuredDataAudit(ctx, dbsqlc.CreateStructuredDataAuditParams{
		SpaceID:     input.SpaceID,
		ActorType:   actorType,
		ActorUserID: input.ActorUserID,
		ActorBotID:  input.ActorBotID,
		Operation:   strings.TrimSpace(input.Operation),
		Statement:   input.Statement,
		Success:     input.Success,
		Error:       strings.TrimSpace(input.Error),
		RowCount:    input.RowCount,
		DurationMs:  input.DurationMs,
	})
	return err
}

func normalizeOwner(owner OwnerRef) (OwnerRef, string, error) {
	owner.Type = OwnerType(strings.TrimSpace(string(owner.Type)))
	owner.BotID = strings.TrimSpace(owner.BotID)
	owner.BotGroupID = strings.TrimSpace(owner.BotGroupID)
	switch owner.Type {
	case OwnerTypeBot:
		id, err := parseUUIDHex(owner.BotID)
		if err != nil || owner.BotGroupID != "" {
			return OwnerRef{}, "", ErrInvalidOwner
		}
		return owner, id, nil
	case OwnerTypeBotGroup:
		id, err := parseUUIDHex(owner.BotGroupID)
		if err != nil || owner.BotID != "" {
			return OwnerRef{}, "", ErrInvalidOwner
		}
		return owner, id, nil
	default:
		return OwnerRef{}, "", ErrInvalidOwner
	}
}

func parseUUIDHex(value string) (string, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil {
		return "", err
	}
	return strings.ReplaceAll(parsed.String(), "-", ""), nil
}

func generatedNames(ownerType OwnerType, ownerIDHex string) (string, string) {
	switch ownerType {
	case OwnerTypeBot:
		return "bot_data_" + ownerIDHex, "memoh_bot_" + ownerIDHex
	case OwnerTypeBotGroup:
		return "bot_group_data_" + ownerIDHex, "memoh_group_" + ownerIDHex
	default:
		return "", ""
	}
}

func defaultDisplayName(ownerType OwnerType) string {
	switch ownerType {
	case OwnerTypeBot:
		return "Bot structured data"
	case OwnerTypeBotGroup:
		return "Bot group structured data"
	default:
		return "Structured data"
	}
}

func normalizePrivileges(values []string) ([]string, error) {
	seen := map[string]struct{}{}
	for _, value := range values {
		privilege := strings.TrimSpace(value)
		switch Privilege(privilege) {
		case PrivilegeRead, PrivilegeWrite, PrivilegeDDL:
			seen[privilege] = struct{}{}
		default:
			return nil, ErrInvalidPrivilege
		}
	}
	if len(seen) == 0 {
		return nil, ErrInvalidPrivilege
	}
	out := make([]string, 0, len(seen))
	for value := range seen {
		out = append(out, value)
	}
	sort.Strings(out)
	return out, nil
}

func privilegeSet(values []string) map[Privilege]bool {
	out := map[Privilege]bool{}
	for _, value := range values {
		out[Privilege(value)] = true
	}
	return out
}

func normalizeMaxRows(value int32) int32 {
	if value <= 0 {
		return defaultMaxRows
	}
	if value > absoluteMaxRows {
		return absoluteMaxRows
	}
	return value
}

func nullableUUID(value string) pgtype.UUID {
	value = strings.TrimSpace(value)
	if value == "" {
		return pgtype.UUID{}
	}
	parsed, err := db.ParseUUID(value)
	if err != nil {
		return pgtype.UUID{}
	}
	return parsed
}

func roleExists(ctx context.Context, tx pgx.Tx, roleName string) (bool, error) {
	var exists bool
	err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = $1)", roleName).Scan(&exists)
	return exists, err
}

func validateGeneratedIdentifier(value string) error {
	if !generatedIdentifierPattern.MatchString(value) {
		return fmt.Errorf("invalid generated PostgreSQL identifier %q", value)
	}
	return nil
}

func quoteIdent(value string) string {
	return pgx.Identifier{value}.Sanitize()
}

func normalizeValue(value any) any {
	switch v := value.(type) {
	case nil:
		return nil
	case []byte:
		if json.Valid(v) {
			var decoded any
			if json.Unmarshal(v, &decoded) == nil {
				return decoded
			}
		}
		return string(v)
	case time.Time:
		return v.UTC().Format(time.RFC3339Nano)
	case pgtype.UUID:
		if !v.Valid {
			return nil
		}
		return v.String()
	default:
		return v
	}
}
