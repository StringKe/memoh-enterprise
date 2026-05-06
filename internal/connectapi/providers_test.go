package connectapi

import (
	"context"
	"encoding/json"
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
	"github.com/memohai/memoh/internal/models"
	"github.com/memohai/memoh/internal/providers"
)

type fakeProviderRow struct {
	scanFunc func(dest ...any) error
}

func (r *fakeProviderRow) Scan(dest ...any) error {
	return r.scanFunc(dest...)
}

type fakeProviderDBTX struct {
	execFunc     func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	queryRowFunc func(ctx context.Context, sql string, args ...any) pgx.Row
}

func (db *fakeProviderDBTX) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if db.execFunc != nil {
		return db.execFunc(ctx, sql, args...)
	}
	return pgconn.CommandTag{}, nil
}

func (*fakeProviderDBTX) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, nil
}

func (db *fakeProviderDBTX) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if db.queryRowFunc != nil {
		return db.queryRowFunc(ctx, sql, args...)
	}
	return &fakeProviderRow{scanFunc: func(_ ...any) error { return pgx.ErrNoRows }}
}

func TestProviderServiceCreateMapsRequest(t *testing.T) {
	t.Parallel()

	db := &fakeProviderDBTX{
		queryRowFunc: func(_ context.Context, sql string, args ...any) pgx.Row {
			if !strings.Contains(sql, "INSERT INTO providers") {
				t.Fatalf("sql = %q, want create provider query", sql)
			}
			if got := args[0].(string); got != "OpenAI Main" {
				t.Fatalf("name arg = %q, want OpenAI Main", got)
			}
			if got := args[1].(string); got != "openai-completions" {
				t.Fatalf("client_type arg = %q, want openai-completions", got)
			}
			if got := args[3].(bool); !got {
				t.Fatalf("enable arg = %v, want true to match REST create behavior", got)
			}
			var config map[string]any
			if err := json.Unmarshal(args[4].([]byte), &config); err != nil {
				t.Fatalf("config json: %v", err)
			}
			if config["base_url"] != "https://api.example.test/v1" {
				t.Fatalf("config base_url = %#v, want https://api.example.test/v1", config["base_url"])
			}
			if config["api_key"] != "secret" {
				t.Fatalf("config api_key = %#v, want secret", config["api_key"])
			}
			if config["organization"] != "org-1" {
				t.Fatalf("config organization = %#v, want org-1", config["organization"])
			}
			return makeProviderRow(t, "550e8400-e29b-41d4-a716-446655440000", args[0].(string), args[1].(string), true, args[4].([]byte), args[5].([]byte))
		},
	}
	service := newTestProviderService(db)
	config, err := structpb.NewStruct(map[string]any{"organization": "org-1"})
	if err != nil {
		t.Fatalf("new struct: %v", err)
	}

	resp, err := service.CreateProvider(context.Background(), connect.NewRequest(&privatev1.CreateProviderRequest{
		Name:       " OpenAI Main ",
		BaseUrl:    "https://api.example.test/v1",
		ApiKey:     "secret",
		ClientType: "openai-completions",
		Enabled:    false,
		Config:     config,
	}))
	if err != nil {
		t.Fatalf("CreateProvider returned error: %v", err)
	}
	if resp.Msg.GetProvider().GetName() != "OpenAI Main" {
		t.Fatalf("response name = %q, want OpenAI Main", resp.Msg.GetProvider().GetName())
	}
	if !resp.Msg.GetProvider().GetEnabled() {
		t.Fatal("response enabled = false, want true")
	}
	if resp.Msg.GetProvider().GetBaseUrl() != "https://api.example.test/v1" {
		t.Fatalf("response base_url = %q, want https://api.example.test/v1", resp.Msg.GetProvider().GetBaseUrl())
	}
}

func TestProviderServiceGetMapsNotFound(t *testing.T) {
	t.Parallel()

	service := newTestProviderService(&fakeProviderDBTX{})

	_, err := service.GetProvider(context.Background(), connect.NewRequest(&privatev1.GetProviderRequest{
		Id: "550e8400-e29b-41d4-a716-446655440000",
	}))
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("code = %v, want %v, err = %v", connect.CodeOf(err), connect.CodeNotFound, err)
	}
}

func newTestProviderService(db sqlc.DBTX) *ProviderService {
	queries := postgresstore.NewQueries(sqlc.New(db))
	return NewProviderService(
		providers.NewService(nilLogger(), queries, "http://localhost:26817/auth/callback"),
		models.NewService(nilLogger(), queries),
	)
}

func makeProviderRow(t *testing.T, id, name, clientType string, enabled bool, config, metadata []byte) pgx.Row {
	t.Helper()
	var pgID pgtype.UUID
	if err := pgID.Scan(id); err != nil {
		t.Fatalf("parse id: %v", err)
	}
	now := pgtype.Timestamptz{Time: time.Date(2026, 5, 5, 1, 2, 3, 0, time.UTC), Valid: true}
	return &fakeProviderRow{
		scanFunc: func(dest ...any) error {
			*dest[0].(*pgtype.UUID) = pgID
			*dest[1].(*string) = name
			*dest[2].(*string) = clientType
			*dest[3].(*pgtype.Text) = pgtype.Text{}
			*dest[4].(*bool) = enabled
			*dest[5].(*[]byte) = config
			*dest[6].(*[]byte) = metadata
			*dest[7].(*pgtype.Timestamptz) = now
			*dest[8].(*pgtype.Timestamptz) = now
			return nil
		},
	}
}
