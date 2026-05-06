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
	pgOwnerID, err := db.ParseUUID(ownerID)
	if err != nil {
		t.Fatalf("parse owner id: %v", err)
	}
	metadata, _ := json.Marshal(map[string]any{"source": "structured-data-integration-test"})
	bot, err := queries.CreateBot(ctx, dbsqlc.CreateBotParams{
		OwnerUserID: pgOwnerID,
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
