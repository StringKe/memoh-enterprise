package structureddata

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/memohai/memoh/internal/db"
	dbsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
	postgresstore "github.com/memohai/memoh/internal/db/postgres/store"
)

func TestServiceIntegrationBotDDLAndCrossBotReadGrant(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("skip integration test: TEST_POSTGRES_DSN is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("skip integration test: cannot connect to database: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		t.Skipf("skip integration test: database ping failed: %v", err)
	}
	store, err := postgresstore.New(pool)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	service, err := NewService(slog.Default(), store)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	queries := store.SQLC()
	ownerID := createStructuredDataTestUser(ctx, t, queries)
	sourceBotID := createStructuredDataTestBot(ctx, t, queries, ownerID, "structured-data-source")
	targetBotID := createStructuredDataTestBot(ctx, t, queries, ownerID, "structured-data-target")
	t.Cleanup(func() {
		deleteStructuredDataTestBot(context.Background(), t, queries, sourceBotID)
		deleteStructuredDataTestBot(context.Background(), t, queries, targetBotID)
	})

	sourceSpace, err := service.EnsureSpace(ctx, OwnerRef{Type: OwnerTypeBot, BotID: sourceBotID})
	if err != nil {
		t.Fatalf("ensure source space: %v", err)
	}
	targetSpace, err := service.EnsureSpace(ctx, OwnerRef{Type: OwnerTypeBot, BotID: targetBotID})
	if err != nil {
		t.Fatalf("ensure target space: %v", err)
	}
	t.Cleanup(func() {
		dropStructuredDataTestSpace(context.Background(), t, pool, sourceSpace)
		dropStructuredDataTestSpace(context.Background(), t, pool, targetSpace)
	})

	_, err = service.ExecuteAsOwner(ctx, ExecuteInput{
		SpaceID: sourceSpace.ID.String(),
		SQL:     "create table items (id bigint primary key, name text); insert into items values (1, 'alpha');",
	})
	if err != nil {
		t.Fatalf("owner ddl: %v", err)
	}

	_, err = service.ExecuteForBot(ctx, ExecuteInput{
		SpaceID:    sourceSpace.ID.String(),
		ActorBotID: targetBotID,
		SQL:        "select count(*) as count from items;",
	})
	if !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("shared read before grant error = %v, want %v", err, ErrAccessDenied)
	}

	_, err = service.UpsertGrant(ctx, GrantInput{
		SpaceID:     sourceSpace.ID.String(),
		TargetType:  TargetTypeBot,
		TargetBotID: targetBotID,
		Privileges:  []string{"read"},
		ActorUserID: ownerID,
	})
	if err != nil {
		t.Fatalf("grant read: %v", err)
	}

	result, err := service.ExecuteForBot(ctx, ExecuteInput{
		SpaceID:    sourceSpace.ID.String(),
		ActorBotID: targetBotID,
		SQL:        "select count(*) as count from items;",
	})
	if err != nil {
		t.Fatalf("shared read after grant: %v", err)
	}
	if result.RowCount != 1 || len(result.Rows) != 1 {
		t.Fatalf("read result = %#v", result)
	}

	_, err = service.ExecuteForBot(ctx, ExecuteInput{
		SpaceID:    sourceSpace.ID.String(),
		ActorBotID: targetBotID,
		SQL:        "insert into items values (2, 'beta');",
	})
	if err == nil || strings.Contains(err.Error(), "structured data access denied") {
		t.Fatalf("shared write error = %v, want PostgreSQL permission error", err)
	}
}

func TestServiceIntegrationBotGroupDDLAndCrossGroupReadGrant(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("skip integration test: TEST_POSTGRES_DSN is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("skip integration test: cannot connect to database: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		t.Skipf("skip integration test: database ping failed: %v", err)
	}
	store, err := postgresstore.New(pool)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	service, err := NewService(slog.Default(), store)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	queries := store.SQLC()
	ownerID := createStructuredDataTestUser(ctx, t, queries)
	sourceGroupID := createStructuredDataTestBotGroup(ctx, t, queries, ownerID, "structured-data-source-group")
	targetGroupID := createStructuredDataTestBotGroup(ctx, t, queries, ownerID, "structured-data-target-group")
	t.Cleanup(func() {
		deleteStructuredDataTestBotGroup(context.Background(), t, queries, sourceGroupID)
		deleteStructuredDataTestBotGroup(context.Background(), t, queries, targetGroupID)
	})
	sourceBotID := createStructuredDataTestBotInGroup(ctx, t, queries, ownerID, sourceGroupID, "structured-data-source-group-bot")
	targetBotID := createStructuredDataTestBotInGroup(ctx, t, queries, ownerID, targetGroupID, "structured-data-target-group-bot")
	t.Cleanup(func() {
		deleteStructuredDataTestBot(context.Background(), t, queries, sourceBotID)
		deleteStructuredDataTestBot(context.Background(), t, queries, targetBotID)
	})

	sourceSpace, err := service.EnsureSpace(ctx, OwnerRef{Type: OwnerTypeBotGroup, BotGroupID: sourceGroupID})
	if err != nil {
		t.Fatalf("ensure source group space: %v", err)
	}
	targetSpace, err := service.EnsureSpace(ctx, OwnerRef{Type: OwnerTypeBotGroup, BotGroupID: targetGroupID})
	if err != nil {
		t.Fatalf("ensure target group space: %v", err)
	}
	t.Cleanup(func() {
		dropStructuredDataTestSpace(context.Background(), t, pool, sourceSpace)
		dropStructuredDataTestSpace(context.Background(), t, pool, targetSpace)
	})

	_, err = service.ExecuteForBot(ctx, ExecuteInput{
		SpaceID:    sourceSpace.ID.String(),
		ActorBotID: sourceBotID,
		SQL:        "create table group_items (id bigint primary key, name text); insert into group_items values (1, 'alpha');",
	})
	if err != nil {
		t.Fatalf("source group bot ddl: %v", err)
	}

	_, err = service.ExecuteForBot(ctx, ExecuteInput{
		SpaceID:    sourceSpace.ID.String(),
		ActorBotID: targetBotID,
		SQL:        "select count(*) as count from group_items;",
	})
	if !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("cross group read before grant error = %v, want %v", err, ErrAccessDenied)
	}

	_, err = service.UpsertGrant(ctx, GrantInput{
		SpaceID:          sourceSpace.ID.String(),
		TargetType:       TargetTypeBotGroup,
		TargetBotGroupID: targetGroupID,
		Privileges:       []string{"read"},
		ActorUserID:      ownerID,
	})
	if err != nil {
		t.Fatalf("grant group read: %v", err)
	}

	spaces, err := service.ListSpacesForBot(ctx, targetBotID)
	if err != nil {
		t.Fatalf("list target bot spaces: %v", err)
	}
	if !structuredDataTestSpacesContain(spaces, sourceSpace.ID.String()) {
		t.Fatalf("target bot spaces do not include granted source group space: %#v", spaces)
	}

	result, err := service.ExecuteForBot(ctx, ExecuteInput{
		SpaceID:    sourceSpace.ID.String(),
		ActorBotID: targetBotID,
		SQL:        "select count(*) as count from group_items;",
	})
	if err != nil {
		t.Fatalf("cross group read after grant: %v", err)
	}
	if result.RowCount != 1 || len(result.Rows) != 1 {
		t.Fatalf("read result = %#v", result)
	}

	_, err = service.ExecuteForBot(ctx, ExecuteInput{
		SpaceID:    sourceSpace.ID.String(),
		ActorBotID: targetBotID,
		SQL:        "insert into group_items values (2, 'beta');",
	})
	if err == nil || strings.Contains(err.Error(), "structured data access denied") {
		t.Fatalf("cross group write error = %v, want PostgreSQL permission error", err)
	}
}

func createStructuredDataTestUser(ctx context.Context, t *testing.T, queries *dbsqlc.Queries) string {
	t.Helper()
	user, err := queries.CreateUser(ctx, dbsqlc.CreateUserParams{
		IsActive: true,
		Metadata: []byte("{}"),
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	return user.ID.String()
}

func createStructuredDataTestBot(ctx context.Context, t *testing.T, queries *dbsqlc.Queries, ownerID string, name string) string {
	t.Helper()
	return createStructuredDataTestBotInGroup(ctx, t, queries, ownerID, "", name)
}

func createStructuredDataTestBotInGroup(ctx context.Context, t *testing.T, queries *dbsqlc.Queries, ownerID string, groupID string, name string) string {
	t.Helper()
	pgOwnerID, err := db.ParseUUID(ownerID)
	if err != nil {
		t.Fatalf("parse owner id: %v", err)
	}
	pgGroupID := pgtype.UUID{}
	if groupID != "" {
		pgGroupID, err = db.ParseUUID(groupID)
		if err != nil {
			t.Fatalf("parse group id: %v", err)
		}
	}
	metadata, _ := json.Marshal(map[string]any{"source": "structured-data-integration-test"})
	bot, err := queries.CreateBot(ctx, dbsqlc.CreateBotParams{
		OwnerUserID: pgOwnerID,
		GroupID:     pgGroupID,
		DisplayName: pgtype.Text{String: name, Valid: true},
		AvatarUrl:   pgtype.Text{},
		IsActive:    true,
		Metadata:    metadata,
		Status:      "ready",
	})
	if err != nil {
		t.Fatalf("create bot: %v", err)
	}
	return bot.ID.String()
}

func createStructuredDataTestBotGroup(ctx context.Context, t *testing.T, queries *dbsqlc.Queries, ownerID string, name string) string {
	t.Helper()
	pgOwnerID, err := db.ParseUUID(ownerID)
	if err != nil {
		t.Fatalf("parse owner id: %v", err)
	}
	metadata, _ := json.Marshal(map[string]any{"source": "structured-data-integration-test"})
	group, err := queries.CreateBotGroup(ctx, dbsqlc.CreateBotGroupParams{
		OwnerUserID: pgOwnerID,
		Name:        name,
		Description: "",
		Visibility:  "private",
		Metadata:    metadata,
	})
	if err != nil {
		t.Fatalf("create bot group: %v", err)
	}
	return group.ID.String()
}

func dropStructuredDataTestSpace(ctx context.Context, t *testing.T, pool *pgxpool.Pool, space dbsqlc.StructuredDataSpace) {
	t.Helper()
	_, _ = pool.Exec(ctx, "DROP SCHEMA IF EXISTS "+quoteIdent(space.SchemaName)+" CASCADE")
	_, _ = pool.Exec(ctx, "REVOKE "+quoteIdent(space.RoleName)+" FROM CURRENT_USER")
	_, _ = pool.Exec(ctx, "DROP ROLE IF EXISTS "+quoteIdent(space.RoleName))
}

func deleteStructuredDataTestBot(ctx context.Context, t *testing.T, queries *dbsqlc.Queries, botID string) {
	t.Helper()
	pgBotID, err := db.ParseUUID(botID)
	if err != nil {
		return
	}
	_ = queries.DeleteBotByID(ctx, pgBotID)
}

func deleteStructuredDataTestBotGroup(ctx context.Context, t *testing.T, queries *dbsqlc.Queries, groupID string) {
	t.Helper()
	pgGroupID, err := db.ParseUUID(groupID)
	if err != nil {
		return
	}
	_ = queries.DeleteBotGroup(ctx, pgGroupID)
}

func structuredDataTestSpacesContain(spaces []dbsqlc.StructuredDataSpace, spaceID string) bool {
	for _, space := range spaces {
		if space.ID.String() == spaceID {
			return true
		}
	}
	return false
}
