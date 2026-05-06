package connectapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/protobuf/types/known/structpb"

	privatev1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	postgresstore "github.com/memohai/memoh/internal/db/postgres/store"
	"github.com/memohai/memoh/internal/searchproviders"
)

type fakeSearchProviderRow struct {
	scanFunc func(dest ...any) error
}

func (r *fakeSearchProviderRow) Scan(dest ...any) error {
	return r.scanFunc(dest...)
}

type fakeSearchProviderDBTX struct {
	execFunc     func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	queryRowFunc func(ctx context.Context, sql string, args ...any) pgx.Row
}

func (db *fakeSearchProviderDBTX) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if db.execFunc != nil {
		return db.execFunc(ctx, sql, args...)
	}
	return pgconn.CommandTag{}, nil
}

func (*fakeSearchProviderDBTX) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, nil
}

func (db *fakeSearchProviderDBTX) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if db.queryRowFunc != nil {
		return db.queryRowFunc(ctx, sql, args...)
	}
	return &fakeSearchProviderRow{scanFunc: func(_ ...any) error { return pgx.ErrNoRows }}
}

func TestSearchProviderServiceCreateMapsRequest(t *testing.T) {
	t.Parallel()

	db := &fakeSearchProviderDBTX{
		queryRowFunc: func(_ context.Context, sql string, args ...any) pgx.Row {
			if !strings.Contains(sql, "INSERT INTO search_providers") {
				t.Fatalf("sql = %q, want create search provider query", sql)
			}
			if got := args[0].(string); got != "Brave Main" {
				t.Fatalf("name arg = %q, want Brave Main", got)
			}
			if got := args[1].(string); got != "brave" {
				t.Fatalf("provider arg = %q, want brave", got)
			}
			var config map[string]any
			if err := json.Unmarshal(args[2].([]byte), &config); err != nil {
				t.Fatalf("config json: %v", err)
			}
			if config["api_key"] != "secret" {
				t.Fatalf("config api_key = %#v, want secret", config["api_key"])
			}
			if got := args[3].(bool); got {
				t.Fatalf("enable arg = %v, want false to match REST create behavior", got)
			}
			return makeSearchProviderRow(t, "550e8400-e29b-41d4-a716-446655440000", "Brave Main", "brave", args[2].([]byte), false)
		},
	}
	service := newTestSearchProviderService(db)
	config, err := structpb.NewStruct(map[string]any{"api_key": "secret"})
	if err != nil {
		t.Fatalf("new struct: %v", err)
	}

	resp, err := service.CreateSearchProvider(context.Background(), connect.NewRequest(&privatev1.CreateSearchProviderRequest{
		Name:    " Brave Main ",
		Type:    "brave",
		Enabled: true,
		Config:  config,
	}))
	if err != nil {
		t.Fatalf("CreateSearchProvider returned error: %v", err)
	}
	if resp.Msg.GetProvider().GetName() != "Brave Main" {
		t.Fatalf("response name = %q, want Brave Main", resp.Msg.GetProvider().GetName())
	}
	if resp.Msg.GetProvider().GetEnabled() {
		t.Fatal("response enabled = true, want false")
	}
}

func TestSearchProviderServiceGetMapsNotFound(t *testing.T) {
	t.Parallel()

	service := newTestSearchProviderService(&fakeSearchProviderDBTX{})

	_, err := service.GetSearchProvider(context.Background(), connect.NewRequest(&privatev1.GetSearchProviderRequest{
		Id: "550e8400-e29b-41d4-a716-446655440000",
	}))
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("code = %v, want %v, err = %v", connect.CodeOf(err), connect.CodeNotFound, err)
	}
}

func TestSearchProviderServiceListMetaReturnsSchema(t *testing.T) {
	t.Parallel()

	service := newTestSearchProviderService(&fakeSearchProviderDBTX{})

	resp, err := service.ListSearchProviderMeta(context.Background(), connect.NewRequest(&privatev1.ListSearchProviderMetaRequest{}))
	if err != nil {
		t.Fatalf("ListSearchProviderMeta returned error: %v", err)
	}
	if len(resp.Msg.GetProviders()) == 0 {
		t.Fatal("providers must not be empty")
	}
	brave := resp.Msg.GetProviders()[0]
	if brave.GetType() != "brave" {
		t.Fatalf("first provider type = %q, want brave", brave.GetType())
	}
	fields := brave.GetSchema().AsMap()["fields"].(map[string]any)
	apiKey := fields["api_key"].(map[string]any)
	if apiKey["type"] != "secret" {
		t.Fatalf("brave api_key type = %#v, want secret", apiKey["type"])
	}
}

func newTestSearchProviderService(db sqlc.DBTX) *SearchProviderService {
	return NewSearchProviderService(searchproviders.NewService(nilLogger(), postgresstore.NewQueries(sqlc.New(db))))
}

func nilLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

func makeSearchProviderRow(t *testing.T, id, name, provider string, config []byte, enable bool) pgx.Row {
	t.Helper()
	var pgID pgtype.UUID
	if err := pgID.Scan(id); err != nil {
		t.Fatalf("parse id: %v", err)
	}
	now := pgtype.Timestamptz{Time: time.Date(2026, 5, 5, 1, 2, 3, 0, time.UTC), Valid: true}
	return &fakeSearchProviderRow{
		scanFunc: func(dest ...any) error {
			*dest[0].(*pgtype.UUID) = pgID
			*dest[1].(*string) = name
			*dest[2].(*string) = provider
			*dest[3].(*[]byte) = config
			*dest[4].(*bool) = enable
			*dest[5].(*pgtype.Timestamptz) = now
			*dest[6].(*pgtype.Timestamptz) = now
			return nil
		},
	}
}
