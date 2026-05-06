package connectapi

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/bind"
	privatev1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	postgresstore "github.com/memohai/memoh/internal/db/postgres/store"
)

type fakeUserServiceRow struct {
	scanFunc func(dest ...any) error
}

func (r *fakeUserServiceRow) Scan(dest ...any) error {
	return r.scanFunc(dest...)
}

type fakeUserServiceDBTX struct {
	queryRowFunc func(ctx context.Context, sql string, args ...any) pgx.Row
}

func (*fakeUserServiceDBTX) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (*fakeUserServiceDBTX) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, nil
}

func (db *fakeUserServiceDBTX) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if db.queryRowFunc != nil {
		return db.queryRowFunc(ctx, sql, args...)
	}
	return &fakeUserServiceRow{scanFunc: func(_ ...any) error { return pgx.ErrNoRows }}
}

func TestUserServiceIssueBindCodeHonorsTtlMinimum(t *testing.T) {
	t.Parallel()

	userID := "550e8400-e29b-41d4-a716-446655440000"
	before := time.Now().UTC()
	db := &fakeUserServiceDBTX{
		queryRowFunc: func(_ context.Context, _ string, args ...any) pgx.Row {
			if got := args[2].(pgtype.Text); got.String != "feishu" || !got.Valid {
				t.Fatalf("channel arg = %#v, want valid feishu", got)
			}
			expiresAt := args[3].(pgtype.Timestamptz)
			if !expiresAt.Valid {
				t.Fatal("expires_at must be valid")
			}
			if expiresAt.Time.Before(before.Add(59 * time.Second)) {
				t.Fatalf("expires_at = %s, want at least 60s from request start", expiresAt.Time)
			}
			if expiresAt.Time.After(before.Add(70 * time.Second)) {
				t.Fatalf("expires_at = %s, want close to 60s from request start", expiresAt.Time)
			}
			return makeBindCodeRow(t, args[0].(string), userID, expiresAt)
		},
	}
	service := &UserService{
		bind: bind.NewService(nil, nil, postgresstore.NewQueries(sqlc.New(db))),
	}
	ctx := WithUserID(context.Background(), userID)

	resp, err := service.IssueBindCode(ctx, connect.NewRequest(&privatev1.IssueBindCodeRequest{
		Channel:    " Feishu ",
		TtlSeconds: 1,
	}))
	if err != nil {
		t.Fatalf("IssueBindCode returned error: %v", err)
	}
	if resp.Msg.GetBindCode().GetChannel() != "feishu" {
		t.Fatalf("channel = %q, want feishu", resp.Msg.GetBindCode().GetChannel())
	}
	if resp.Msg.GetBindCode().GetCode() == "" {
		t.Fatal("bind code must be returned")
	}
}

func makeBindCodeRow(t *testing.T, token, userID string, expiresAt pgtype.Timestamptz) pgx.Row {
	t.Helper()
	var id pgtype.UUID
	if err := id.Scan("650e8400-e29b-41d4-a716-446655440000"); err != nil {
		t.Fatalf("parse id: %v", err)
	}
	var issuer pgtype.UUID
	if err := issuer.Scan(userID); err != nil {
		t.Fatalf("parse issuer: %v", err)
	}
	return &fakeUserServiceRow{
		scanFunc: func(dest ...any) error {
			*dest[0].(*pgtype.UUID) = id
			*dest[1].(*string) = token
			*dest[2].(*pgtype.UUID) = issuer
			*dest[3].(*pgtype.Text) = pgtype.Text{String: "feishu", Valid: true}
			*dest[4].(*pgtype.Timestamptz) = expiresAt
			*dest[5].(*pgtype.Timestamptz) = pgtype.Timestamptz{}
			*dest[6].(*pgtype.UUID) = pgtype.UUID{}
			*dest[7].(*pgtype.Timestamptz) = pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
			return nil
		},
	}
}
