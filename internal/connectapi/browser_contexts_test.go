package connectapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/memohai/memoh/internal/browsercontexts"
	privatev1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	postgresstore "github.com/memohai/memoh/internal/db/postgres/store"
)

type fakeBrowserContextRow struct {
	scanFunc func(dest ...any) error
}

func (r *fakeBrowserContextRow) Scan(dest ...any) error {
	return r.scanFunc(dest...)
}

type fakeBrowserContextDBTX struct {
	queryRowFunc func(ctx context.Context, sql string, args ...any) pgx.Row
}

func (*fakeBrowserContextDBTX) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (*fakeBrowserContextDBTX) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, nil
}

func (db *fakeBrowserContextDBTX) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if db.queryRowFunc != nil {
		return db.queryRowFunc(ctx, sql, args...)
	}
	return &fakeBrowserContextRow{scanFunc: func(_ ...any) error { return pgx.ErrNoRows }}
}

func TestBrowserContextServiceCreateMapsRequestConfig(t *testing.T) {
	t.Parallel()

	db := &fakeBrowserContextDBTX{
		queryRowFunc: func(_ context.Context, _ string, args ...any) pgx.Row {
			if got := args[0].(string); got != "work" {
				t.Fatalf("name arg = %q, want work", got)
			}
			var config map[string]any
			if err := json.Unmarshal(args[1].([]byte), &config); err != nil {
				t.Fatalf("config arg is invalid JSON: %v", err)
			}
			if got := config["core"]; got != "firefox" {
				t.Fatalf("config.core = %#v, want firefox", got)
			}
			viewport := config["viewport"].(map[string]any)
			if got := viewport["width"]; got != float64(1440) {
				t.Fatalf("config.viewport.width = %#v, want 1440", got)
			}
			return makeBrowserContextRow(t, "750e8400-e29b-41d4-a716-446655440000", "work", args[1].([]byte))
		},
	}
	service := newTestBrowserContextService(db)
	config, err := structpb.NewStruct(map[string]any{
		"viewport": map[string]any{"width": 1440},
	})
	if err != nil {
		t.Fatalf("build struct: %v", err)
	}

	resp, err := service.CreateBrowserContext(context.Background(), connect.NewRequest(&privatev1.CreateBrowserContextRequest{
		Name:   "work",
		Core:   "firefox",
		Config: config,
	}))
	if err != nil {
		t.Fatalf("CreateBrowserContext returned error: %v", err)
	}
	if resp.Msg.GetContext().GetId() != "750e8400-e29b-41d4-a716-446655440000" {
		t.Fatalf("id = %q", resp.Msg.GetContext().GetId())
	}
	if resp.Msg.GetContext().GetCore() != "firefox" {
		t.Fatalf("core = %q, want firefox", resp.Msg.GetContext().GetCore())
	}
}

func TestBrowserContextServiceGetNoRowsMapsNotFound(t *testing.T) {
	t.Parallel()

	service := newTestBrowserContextService(&fakeBrowserContextDBTX{})

	_, err := service.GetBrowserContext(context.Background(), connect.NewRequest(&privatev1.GetBrowserContextRequest{
		Id: "750e8400-e29b-41d4-a716-446655440000",
	}))
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("code = %s, want %s, err = %v", connect.CodeOf(err), connect.CodeNotFound, err)
	}
}

func TestBrowserContextServiceListBrowserCores(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cores/" {
			t.Fatalf("path = %q, want /cores/", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"cores":["chromium","firefox"]}`))
	}))
	t.Cleanup(server.Close)
	service := &BrowserContextService{
		browserGatewayURL: server.URL,
		httpClient:        server.Client(),
	}

	resp, err := service.ListBrowserCores(context.Background(), connect.NewRequest(&privatev1.ListBrowserCoresRequest{}))
	if err != nil {
		t.Fatalf("ListBrowserCores returned error: %v", err)
	}
	if len(resp.Msg.GetCores()) != 2 {
		t.Fatalf("cores len = %d, want 2", len(resp.Msg.GetCores()))
	}
	if resp.Msg.GetCores()[1].GetId() != "firefox" {
		t.Fatalf("second core = %q, want firefox", resp.Msg.GetCores()[1].GetId())
	}
}

func newTestBrowserContextService(db *fakeBrowserContextDBTX) *BrowserContextService {
	return &BrowserContextService{
		contexts: browsercontexts.NewService(slog.Default(), postgresstore.NewQueries(sqlc.New(db))),
	}
}

func makeBrowserContextRow(t *testing.T, id string, name string, config []byte) pgx.Row {
	t.Helper()
	var pgID pgtype.UUID
	if err := pgID.Scan(id); err != nil {
		t.Fatalf("parse id: %v", err)
	}
	now := pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
	return &fakeBrowserContextRow{
		scanFunc: func(dest ...any) error {
			*dest[0].(*pgtype.UUID) = pgID
			*dest[1].(*string) = name
			*dest[2].(*[]byte) = config
			*dest[3].(*pgtype.Timestamptz) = now
			*dest[4].(*pgtype.Timestamptz) = now
			return nil
		},
	}
}
