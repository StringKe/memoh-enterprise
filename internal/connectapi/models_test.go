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
	"github.com/memohai/memoh/internal/models"
)

type fakeModelRow struct {
	scanFunc func(dest ...any) error
}

func (r *fakeModelRow) Scan(dest ...any) error {
	return r.scanFunc(dest...)
}

type fakeModelRows struct {
	rows []*fakeModelRow
	idx  int
}

func (*fakeModelRows) Close() {}

func (*fakeModelRows) Err() error { return nil }

func (*fakeModelRows) CommandTag() pgconn.CommandTag { return pgconn.CommandTag{} }

func (*fakeModelRows) FieldDescriptions() []pgconn.FieldDescription { return nil }

func (r *fakeModelRows) Next() bool {
	if r.idx >= len(r.rows) {
		return false
	}
	r.idx++
	return true
}

func (r *fakeModelRows) Scan(dest ...any) error {
	return r.rows[r.idx-1].Scan(dest...)
}

func (*fakeModelRows) Values() ([]any, error) { return nil, nil }

func (*fakeModelRows) RawValues() [][]byte { return nil }

func (*fakeModelRows) Conn() *pgx.Conn { return nil }

type fakeModelDBTX struct {
	execFunc     func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	queryFunc    func(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	queryRowFunc func(ctx context.Context, sql string, args ...any) pgx.Row
}

func (db *fakeModelDBTX) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if db.execFunc != nil {
		return db.execFunc(ctx, sql, args...)
	}
	return pgconn.CommandTag{}, nil
}

func (db *fakeModelDBTX) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if db.queryFunc != nil {
		return db.queryFunc(ctx, sql, args...)
	}
	return &fakeModelRows{}, nil
}

func (db *fakeModelDBTX) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if db.queryRowFunc != nil {
		return db.queryRowFunc(ctx, sql, args...)
	}
	return &fakeModelRow{scanFunc: func(_ ...any) error { return pgx.ErrNoRows }}
}

func TestModelServiceCreateMapsRequestConfig(t *testing.T) {
	t.Parallel()

	providerID := "550e8400-e29b-41d4-a716-446655440000"
	modelID := "650e8400-e29b-41d4-a716-446655440000"
	db := &fakeModelDBTX{
		queryRowFunc: func(_ context.Context, sql string, args ...any) pgx.Row {
			switch {
			case strings.Contains(sql, "INSERT INTO models"):
				if got := args[0].(string); got != "gpt-4.1" {
					t.Fatalf("model_id arg = %q, want gpt-4.1", got)
				}
				if got := args[1].(pgtype.Text); got.String != "GPT 4.1" || !got.Valid {
					t.Fatalf("name arg = %#v, want valid GPT 4.1", got)
				}
				if got := args[3].(string); got != "chat" {
					t.Fatalf("type arg = %q, want chat", got)
				}
				var config map[string]any
				if err := json.Unmarshal(args[4].([]byte), &config); err != nil {
					t.Fatalf("config arg is invalid JSON: %v", err)
				}
				if got := config["context_window"]; got != float64(128000) {
					t.Fatalf("context_window = %#v, want 128000", got)
				}
				compat := config["compatibilities"].([]any)
				if compat[0] != "vision" || compat[1] != "tool-call" {
					t.Fatalf("compatibilities = %#v", compat)
				}
				efforts := config["reasoning_efforts"].([]any)
				if efforts[0] != "low" || efforts[1] != "high" {
					t.Fatalf("reasoning_efforts = %#v", efforts)
				}
				return makeModelRow(t, modelID, "gpt-4.1", "GPT 4.1", providerID, "chat", args[4].([]byte))
			case strings.Contains(sql, "SELECT id, model_id"):
				return makeModelRow(t, modelID, "gpt-4.1", "GPT 4.1", providerID, "chat", []byte(`{"context_window":128000,"compatibilities":["vision","tool-call"],"reasoning_efforts":["low","high"]}`))
			default:
				t.Fatalf("unexpected sql: %s", sql)
				return nil
			}
		},
	}
	service := newTestModelService(db)
	metadata := mustStruct(t, map[string]any{"context_window": 128000})
	reasoning := mustStruct(t, map[string]any{"reasoning_efforts": []any{"low", "high"}})

	resp, err := service.CreateModel(context.Background(), connect.NewRequest(&privatev1.CreateModelRequest{
		ProviderId:  providerID,
		ModelId:     " gpt-4.1 ",
		DisplayName: " GPT 4.1 ",
		Type:        "chat",
		Modalities:  []string{"vision", "tool-call"},
		Reasoning:   reasoning,
		Metadata:    metadata,
	}))
	if err != nil {
		t.Fatalf("CreateModel returned error: %v", err)
	}
	if resp.Msg.GetModel().GetDisplayName() != "GPT 4.1" {
		t.Fatalf("display name = %q, want GPT 4.1", resp.Msg.GetModel().GetDisplayName())
	}
	if got := resp.Msg.GetModel().GetMetadata().AsMap()["context_window"]; got != float64(128000) {
		t.Fatalf("response context_window = %#v, want 128000", got)
	}
}

func TestModelServiceGetNoRowsMapsNotFound(t *testing.T) {
	t.Parallel()

	service := newTestModelService(&fakeModelDBTX{})

	_, err := service.GetModel(context.Background(), connect.NewRequest(&privatev1.GetModelRequest{
		Id: "650e8400-e29b-41d4-a716-446655440000",
	}))
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("code = %s, want %s, err = %v", connect.CodeOf(err), connect.CodeNotFound, err)
	}
}

func TestModelServiceCapabilitiesMapsConfig(t *testing.T) {
	t.Parallel()

	providerID := "550e8400-e29b-41d4-a716-446655440000"
	modelID := "650e8400-e29b-41d4-a716-446655440000"
	db := &fakeModelDBTX{
		queryRowFunc: func(_ context.Context, sql string, _ ...any) pgx.Row {
			if !strings.Contains(sql, "SELECT id, model_id") {
				t.Fatalf("unexpected sql: %s", sql)
			}
			return makeModelRow(t, modelID, "claude-3.7", "Claude 3.7", providerID, "chat", []byte(`{"compatibilities":["vision","tool-call","reasoning"],"context_window":200000}`))
		},
	}
	service := newTestModelService(db)

	resp, err := service.GetModelCapabilities(context.Background(), connect.NewRequest(&privatev1.GetModelCapabilitiesRequest{
		Id: modelID,
	}))
	if err != nil {
		t.Fatalf("GetModelCapabilities returned error: %v", err)
	}
	capabilities := resp.Msg.GetCapabilities()
	if !capabilities.GetSupportsVision() || !capabilities.GetSupportsTools() || !capabilities.GetSupportsReasoning() {
		t.Fatalf("capabilities = %#v, want vision/tools/reasoning true", capabilities)
	}
	if !capabilities.GetSupportsStreaming() {
		t.Fatal("supports_streaming = false, want true for chat")
	}
	if got := capabilities.GetMetadata().AsMap()["context_window"]; got != float64(200000) {
		t.Fatalf("metadata context_window = %#v, want 200000", got)
	}
}

func newTestModelService(db sqlc.DBTX) *ModelService {
	logger := slog.New(slog.DiscardHandler)
	return NewModelService(models.NewService(logger, postgresstore.NewQueries(sqlc.New(db))))
}

func mustStruct(t *testing.T, value map[string]any) *structpb.Struct {
	t.Helper()
	result, err := structpb.NewStruct(value)
	if err != nil {
		t.Fatalf("new struct: %v", err)
	}
	return result
}

func makeModelRow(t *testing.T, id, modelID, name, providerID, modelType string, config []byte) pgx.Row {
	t.Helper()
	var pgID pgtype.UUID
	if err := pgID.Scan(id); err != nil {
		t.Fatalf("parse id: %v", err)
	}
	var pgProviderID pgtype.UUID
	if err := pgProviderID.Scan(providerID); err != nil {
		t.Fatalf("parse provider id: %v", err)
	}
	now := pgtype.Timestamptz{Time: time.Date(2026, 5, 5, 1, 2, 3, 0, time.UTC), Valid: true}
	return &fakeModelRow{
		scanFunc: func(dest ...any) error {
			*dest[0].(*pgtype.UUID) = pgID
			*dest[1].(*string) = modelID
			*dest[2].(*pgtype.Text) = pgtype.Text{String: name, Valid: true}
			*dest[3].(*pgtype.UUID) = pgProviderID
			*dest[4].(*string) = modelType
			*dest[5].(*[]byte) = config
			*dest[6].(*pgtype.Timestamptz) = now
			*dest[7].(*pgtype.Timestamptz) = now
			return nil
		},
	}
}
