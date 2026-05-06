package botgroups

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	postgresstore "github.com/memohai/memoh/internal/db/postgres/store"
	"github.com/memohai/memoh/internal/iam/rbac"
)

type fakeDBTX struct {
	ownerID         pgtype.UUID
	groupID         pgtype.UUID
	otherID         pgtype.UUID
	deletedGroupID  pgtype.UUID
	lastUpsertArgs  []any
	settingsNoRows  bool
	groupLookupHits int
	visibility      string
}

func (db *fakeDBTX) Exec(_ context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	if strings.Contains(sql, "DELETE FROM bot_groups") {
		db.deletedGroupID = args[0].(pgtype.UUID)
	}
	return pgconn.CommandTag{}, nil
}

func (db *fakeDBTX) Query(_ context.Context, sql string, _ ...interface{}) (pgx.Rows, error) {
	if strings.Contains(sql, "FROM bot_groups") {
		return &fakeRows{
			scans: []func(dest ...any) error{
				func(dest ...any) error {
					scanBotGroupWithVisibility(dest, db.groupID, db.ownerID, "group-a", db.visibility)
					return nil
				},
			},
		}, nil
	}
	return &fakeRows{}, nil
}

func (db *fakeDBTX) QueryRow(_ context.Context, sql string, args ...interface{}) pgx.Row {
	switch {
	case strings.Contains(sql, "INSERT INTO bot_groups"):
		return fakeRow(func(dest ...any) error {
			scanBotGroupWithVisibility(dest, db.groupID, db.ownerID, args[1].(string), db.visibility)
			return nil
		})
	case strings.Contains(sql, "SELECT count(*)"):
		return fakeRow(func(dest ...any) error {
			*dest[0].(*int64) = 1
			return nil
		})
	case strings.Contains(sql, "WHERE owner_user_id"):
		ownerID := args[0].(pgtype.UUID)
		if ownerID != db.ownerID {
			return fakeRow(func(_ ...any) error { return pgx.ErrNoRows })
		}
		return fakeRow(func(dest ...any) error {
			scanBotGroupWithVisibility(dest, db.groupID, db.ownerID, "group-a", db.visibility)
			return nil
		})
	case strings.Contains(sql, "WHERE id ="):
		db.groupLookupHits++
		return fakeRow(func(dest ...any) error {
			scanBotGroupWithVisibility(dest, db.groupID, db.ownerID, "group-a", db.visibility)
			return nil
		})
	case strings.Contains(sql, "FROM bot_group_settings"):
		if db.settingsNoRows {
			return fakeRow(func(_ ...any) error { return pgx.ErrNoRows })
		}
		return fakeRow(func(dest ...any) error {
			scanGroupSettings(dest, db.groupID)
			return nil
		})
	case strings.Contains(sql, "INSERT INTO bot_group_settings"):
		db.lastUpsertArgs = args
		return fakeRow(func(dest ...any) error {
			scanGroupSettings(dest, db.groupID)
			return nil
		})
	}
	return fakeRow(func(_ ...any) error { return pgx.ErrNoRows })
}

type fakeRow func(dest ...any) error

func (r fakeRow) Scan(dest ...any) error {
	return r(dest...)
}

type fakeRows struct {
	index int
	scans []func(dest ...any) error
}

func (*fakeRows) Close() {}

func (*fakeRows) Err() error { return nil }

func (*fakeRows) CommandTag() pgconn.CommandTag { return pgconn.CommandTag{} }

func (*fakeRows) FieldDescriptions() []pgconn.FieldDescription { return nil }

func (r *fakeRows) Next() bool {
	return r.index < len(r.scans)
}

func (r *fakeRows) Scan(dest ...any) error {
	err := r.scans[r.index](dest...)
	r.index++
	return err
}

func (*fakeRows) Values() ([]any, error) { return nil, nil }

func (*fakeRows) RawValues() [][]byte { return nil }

func (*fakeRows) Conn() *pgx.Conn { return nil }

func TestServiceOwnerChecks(t *testing.T) {
	db := newFakeDB(t)
	service := newTestService(db)

	if _, err := service.GetGroup(context.Background(), db.ownerID.String(), db.groupID.String()); err != nil {
		t.Fatalf("owner GetGroup returned error: %v", err)
	}

	if _, err := service.GetGroup(context.Background(), db.otherID.String(), db.groupID.String()); !errors.Is(err, ErrGroupAccessDenied) {
		t.Fatalf("other GetGroup error = %v, want ErrGroupAccessDenied", err)
	}
}

func TestHasGroupPermissionAllowsOrganizationReadAndUse(t *testing.T) {
	db := newFakeDB(t)
	db.visibility = VisibilityOrganization
	service := newTestService(db)
	service.SetRBACService(&fakePermissionService{})

	allowed, err := service.HasGroupPermission(context.Background(), db.otherID.String(), db.groupID.String(), rbac.PermissionBotGroupRead)
	if err != nil {
		t.Fatalf("HasGroupPermission read returned error: %v", err)
	}
	if !allowed {
		t.Fatal("organization group read = false, want true")
	}
	allowed, err = service.HasGroupPermission(context.Background(), db.otherID.String(), db.groupID.String(), rbac.PermissionBotGroupUse)
	if err != nil {
		t.Fatalf("HasGroupPermission use returned error: %v", err)
	}
	if !allowed {
		t.Fatal("organization group use = false, want true")
	}
	allowed, err = service.HasGroupPermission(context.Background(), db.otherID.String(), db.groupID.String(), rbac.PermissionBotGroupUpdate)
	if err != nil {
		t.Fatalf("HasGroupPermission update returned error: %v", err)
	}
	if allowed {
		t.Fatal("organization group update = true, want false without RBAC")
	}
}

func TestHasGroupPermissionUsesRBACForPrivateGroup(t *testing.T) {
	db := newFakeDB(t)
	permission := &fakePermissionService{allowed: true}
	service := newTestService(db)
	service.SetRBACService(permission)

	allowed, err := service.HasGroupPermission(context.Background(), db.otherID.String(), db.groupID.String(), rbac.PermissionBotGroupUpdate)
	if err != nil {
		t.Fatalf("HasGroupPermission returned error: %v", err)
	}
	if !allowed {
		t.Fatal("private group update = false, want true from RBAC")
	}
	if permission.check.PermissionKey != rbac.PermissionBotGroupUpdate {
		t.Fatalf("permission = %q, want %q", permission.check.PermissionKey, rbac.PermissionBotGroupUpdate)
	}
	if permission.check.ResourceType != rbac.ResourceBotGroup || permission.check.ResourceID != db.groupID.String() {
		t.Fatalf("unexpected RBAC check: %+v", permission.check)
	}
}

func TestServiceGroupSettingsNullableDefaults(t *testing.T) {
	db := newFakeDB(t)
	service := newTestService(db)
	reasoningEnabled := false

	got, err := service.UpsertGroupSettings(context.Background(), db.ownerID.String(), db.groupID.String(), GroupSettings{
		ReasoningEnabled: &reasoningEnabled,
	})
	if err != nil {
		t.Fatalf("UpsertGroupSettings returned error: %v", err)
	}
	if got.Timezone != nil {
		t.Fatalf("timezone = %v, want nil", *got.Timezone)
	}
	if len(db.lastUpsertArgs) == 0 {
		t.Fatal("upsert was not called")
	}
	timezoneArg := db.lastUpsertArgs[1].(pgtype.Text)
	reasoningArg := db.lastUpsertArgs[3].(pgtype.Bool)
	if timezoneArg.Valid {
		t.Fatalf("timezone arg valid = true, want false")
	}
	if !reasoningArg.Valid || reasoningArg.Bool {
		t.Fatalf("reasoning arg = %#v, want explicit false", reasoningArg)
	}
}

func TestDeleteGroupUsesDatabaseSetNullCascade(t *testing.T) {
	db := newFakeDB(t)
	service := newTestService(db)

	if err := service.DeleteGroup(context.Background(), db.ownerID.String(), db.groupID.String()); err != nil {
		t.Fatalf("DeleteGroup returned error: %v", err)
	}
	if db.deletedGroupID != db.groupID {
		t.Fatalf("deleted group id = %s, want %s", db.deletedGroupID.String(), db.groupID.String())
	}
}

func newTestService(fake *fakeDBTX) *Service {
	return NewService(nil, postgresstore.NewQueries(sqlc.New(fake)))
}

func newFakeDB(t *testing.T) *fakeDBTX {
	t.Helper()
	return &fakeDBTX{
		ownerID:    parseTestUUID(t, "00000000-0000-0000-0000-000000000001"),
		groupID:    parseTestUUID(t, "00000000-0000-0000-0000-000000000002"),
		otherID:    parseTestUUID(t, "00000000-0000-0000-0000-000000000003"),
		visibility: VisibilityPrivate,
	}
}

func parseTestUUID(t *testing.T, value string) pgtype.UUID {
	t.Helper()
	var id pgtype.UUID
	if err := id.Scan(value); err != nil {
		t.Fatalf("parse uuid: %v", err)
	}
	return id
}

func scanBotGroupWithVisibility(dest []any, groupID, ownerID pgtype.UUID, name string, visibility string) {
	*dest[0].(*pgtype.UUID) = groupID
	*dest[1].(*pgtype.UUID) = ownerID
	*dest[2].(*string) = name
	*dest[3].(*string) = "description"
	*dest[4].(*string) = visibility
	*dest[5].(*[]byte) = []byte(`{"source":"test"}`)
	*dest[6].(*pgtype.Timestamptz) = pgtype.Timestamptz{}
	*dest[7].(*pgtype.Timestamptz) = pgtype.Timestamptz{}
}

type fakePermissionService struct {
	allowed bool
	check   rbac.Check
}

func (f *fakePermissionService) HasPermission(_ context.Context, check rbac.Check) (bool, error) {
	f.check = check
	return f.allowed, nil
}

func scanGroupSettings(dest []any, groupID pgtype.UUID) {
	*dest[0].(*pgtype.UUID) = groupID
	*dest[1].(*pgtype.Text) = pgtype.Text{}
	*dest[2].(*pgtype.Text) = pgtype.Text{}
	*dest[3].(*pgtype.Bool) = pgtype.Bool{Bool: false, Valid: true}
	*dest[4].(*pgtype.Text) = pgtype.Text{}
	*dest[5].(*pgtype.UUID) = pgtype.UUID{}
	*dest[6].(*pgtype.UUID) = pgtype.UUID{}
	*dest[7].(*pgtype.UUID) = pgtype.UUID{}
	*dest[8].(*pgtype.Bool) = pgtype.Bool{}
	*dest[9].(*pgtype.Int4) = pgtype.Int4{}
	*dest[10].(*pgtype.Text) = pgtype.Text{}
	*dest[11].(*pgtype.UUID) = pgtype.UUID{}
	*dest[12].(*pgtype.Bool) = pgtype.Bool{}
	*dest[13].(*pgtype.Int4) = pgtype.Int4{}
	*dest[14].(*pgtype.Int4) = pgtype.Int4{}
	*dest[15].(*pgtype.UUID) = pgtype.UUID{}
	*dest[16].(*pgtype.UUID) = pgtype.UUID{}
	*dest[17].(*pgtype.UUID) = pgtype.UUID{}
	*dest[18].(*pgtype.UUID) = pgtype.UUID{}
	*dest[19].(*pgtype.UUID) = pgtype.UUID{}
	*dest[20].(*pgtype.UUID) = pgtype.UUID{}
	*dest[21].(*pgtype.UUID) = pgtype.UUID{}
	*dest[22].(*pgtype.Bool) = pgtype.Bool{}
	*dest[23].(*pgtype.Bool) = pgtype.Bool{}
	*dest[24].(*[]byte) = nil
	*dest[25].(*pgtype.Text) = pgtype.Text{}
	*dest[26].(*pgtype.Bool) = pgtype.Bool{}
	*dest[27].(*[]byte) = nil
	*dest[28].(*[]byte) = []byte(`{}`)
	*dest[29].(*pgtype.Timestamptz) = pgtype.Timestamptz{}
}
