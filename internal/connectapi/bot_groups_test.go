package connectapi

import (
	"context"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/botgroups"
	privatev1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	postgresstore "github.com/memohai/memoh/internal/db/postgres/store"
	"github.com/memohai/memoh/internal/iam/rbac"
)

type fakeBotGroupDBTX struct {
	ownerID      pgtype.UUID
	groupID      pgtype.UUID
	roleID       pgtype.UUID
	assignmentID pgtype.UUID
	principalID  pgtype.UUID
	deletedRole  pgtype.UUID
}

func (db *fakeBotGroupDBTX) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if strings.Contains(sql, "DELETE FROM iam_principal_roles") {
		db.deletedRole = args[0].(pgtype.UUID)
		if got := args[1].(string); got != string(rbac.ResourceBotGroup) {
			return pgconn.CommandTag{}, pgx.ErrNoRows
		}
		if got := args[2].(pgtype.UUID); got != db.groupID {
			return pgconn.CommandTag{}, pgx.ErrNoRows
		}
	}
	return pgconn.CommandTag{}, nil
}

func (db *fakeBotGroupDBTX) Query(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
	if strings.Contains(sql, "FROM iam_principal_roles") {
		return &fakeBotGroupRows{
			scans: []func(dest ...any) error{
				func(dest ...any) error {
					scanPrincipalRole(dest, db.assignmentID, db.groupID, db.principalID)
					return nil
				},
			},
		}, nil
	}
	return &fakeBotGroupRows{}, nil
}

func (db *fakeBotGroupDBTX) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	switch {
	case strings.Contains(sql, "FROM bot_groups") && strings.Contains(sql, "WHERE id ="):
		return fakeBotGroupRow(func(dest ...any) error {
			scanConnectBotGroup(dest, db.groupID, db.ownerID)
			return nil
		})
	case strings.Contains(sql, "FROM iam_roles") && strings.Contains(sql, "WHERE key"):
		return fakeBotGroupRow(func(dest ...any) error {
			scope := string(rbac.ResourceBotGroup)
			if args[0].(string) == string(rbac.RoleBotViewer) {
				scope = string(rbac.ResourceBot)
			}
			*dest[0].(*pgtype.UUID) = db.roleID
			*dest[1].(*string) = args[0].(string)
			*dest[2].(*string) = scope
			*dest[3].(*string) = ""
			*dest[4].(*bool) = true
			*dest[5].(*pgtype.Timestamptz) = pgtype.Timestamptz{}
			*dest[6].(*pgtype.Timestamptz) = pgtype.Timestamptz{}
			return nil
		})
	case strings.Contains(sql, "INSERT INTO iam_principal_roles"):
		return fakeBotGroupRow(func(dest ...any) error {
			*dest[0].(*pgtype.UUID) = db.assignmentID
			*dest[1].(*string) = args[0].(string)
			*dest[2].(*pgtype.UUID) = args[1].(pgtype.UUID)
			*dest[3].(*pgtype.UUID) = args[2].(pgtype.UUID)
			*dest[4].(*string) = args[3].(string)
			*dest[5].(*pgtype.UUID) = args[4].(pgtype.UUID)
			*dest[6].(*string) = args[5].(string)
			*dest[7].(*pgtype.UUID) = pgtype.UUID{}
			*dest[8].(*pgtype.Timestamptz) = pgtype.Timestamptz{}
			*dest[9].(*pgtype.Timestamptz) = pgtype.Timestamptz{}
			return nil
		})
	}
	return fakeBotGroupRow(func(_ ...any) error { return pgx.ErrNoRows })
}

type fakeBotGroupRow func(dest ...any) error

func (r fakeBotGroupRow) Scan(dest ...any) error { return r(dest...) }

type fakeBotGroupRows struct {
	index int
	scans []func(dest ...any) error
}

func (*fakeBotGroupRows) Close()                                       {}
func (*fakeBotGroupRows) Err() error                                   { return nil }
func (*fakeBotGroupRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (*fakeBotGroupRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *fakeBotGroupRows) Next() bool                                 { return r.index < len(r.scans) }
func (r *fakeBotGroupRows) Scan(dest ...any) error {
	err := r.scans[r.index](dest...)
	r.index++
	return err
}
func (*fakeBotGroupRows) Values() ([]any, error) { return nil, nil }
func (*fakeBotGroupRows) RawValues() [][]byte    { return nil }
func (*fakeBotGroupRows) Conn() *pgx.Conn        { return nil }

func TestBotGroupPrincipalRolesRPC(t *testing.T) {
	db := newFakeBotGroupRPCDB(t)
	service := newTestBotGroupRPCService(db)
	ctx := WithUserID(context.Background(), db.ownerID.String())

	listResp, err := service.ListBotGroupPrincipalRoles(ctx, connect.NewRequest(&privatev1.ListBotGroupPrincipalRolesRequest{
		GroupId: db.groupID.String(),
	}))
	if err != nil {
		t.Fatalf("ListBotGroupPrincipalRoles returned error: %v", err)
	}
	if got := listResp.Msg.GetRoles()[0].GetRole(); got != string(rbac.RoleBotGroupViewer) {
		t.Fatalf("role = %q, want %q", got, rbac.RoleBotGroupViewer)
	}

	assignResp, err := service.AssignBotGroupPrincipalRole(ctx, connect.NewRequest(&privatev1.AssignBotGroupPrincipalRoleRequest{
		GroupId:       db.groupID.String(),
		PrincipalType: string(rbac.PrincipalGroup),
		PrincipalId:   db.principalID.String(),
		Role:          string(rbac.RoleBotGroupEditor),
	}))
	if err != nil {
		t.Fatalf("AssignBotGroupPrincipalRole returned error: %v", err)
	}
	if assignResp.Msg.GetRole().GetGroupId() != db.groupID.String() {
		t.Fatalf("group id = %q, want %q", assignResp.Msg.GetRole().GetGroupId(), db.groupID.String())
	}

	_, err = service.DeleteBotGroupPrincipalRole(ctx, connect.NewRequest(&privatev1.DeleteBotGroupPrincipalRoleRequest{
		GroupId: db.groupID.String(),
		Id:      db.assignmentID.String(),
	}))
	if err != nil {
		t.Fatalf("DeleteBotGroupPrincipalRole returned error: %v", err)
	}
	if db.deletedRole != db.assignmentID {
		t.Fatalf("deleted role = %s, want %s", db.deletedRole.String(), db.assignmentID.String())
	}
}

func TestAssignBotGroupPrincipalRoleRejectsBotScopedRole(t *testing.T) {
	db := newFakeBotGroupRPCDB(t)
	service := newTestBotGroupRPCService(db)
	ctx := WithUserID(context.Background(), db.ownerID.String())

	_, err := service.AssignBotGroupPrincipalRole(ctx, connect.NewRequest(&privatev1.AssignBotGroupPrincipalRoleRequest{
		GroupId:       db.groupID.String(),
		PrincipalType: string(rbac.PrincipalUser),
		PrincipalId:   db.principalID.String(),
		Role:          string(rbac.RoleBotViewer),
	}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("code = %s, want %s, err = %v", connect.CodeOf(err), connect.CodeInvalidArgument, err)
	}
}

func newFakeBotGroupRPCDB(t *testing.T) *fakeBotGroupDBTX {
	t.Helper()
	return &fakeBotGroupDBTX{
		ownerID:      parseConnectUUID(t, "00000000-0000-0000-0000-000000000001"),
		groupID:      parseConnectUUID(t, "00000000-0000-0000-0000-000000000002"),
		roleID:       parseConnectUUID(t, "00000000-0000-0000-0000-000000000003"),
		assignmentID: parseConnectUUID(t, "00000000-0000-0000-0000-000000000004"),
		principalID:  parseConnectUUID(t, "00000000-0000-0000-0000-000000000005"),
	}
}

func newTestBotGroupRPCService(db *fakeBotGroupDBTX) *BotGroupService {
	queries := postgresstore.NewQueries(sqlc.New(db))
	return &BotGroupService{
		groups:  botgroups.NewService(nil, queries),
		queries: queries,
	}
}

func parseConnectUUID(t *testing.T, value string) pgtype.UUID {
	t.Helper()
	var id pgtype.UUID
	if err := id.Scan(value); err != nil {
		t.Fatalf("parse uuid: %v", err)
	}
	return id
}

func scanConnectBotGroup(dest []any, groupID, ownerID pgtype.UUID) {
	*dest[0].(*pgtype.UUID) = groupID
	*dest[1].(*pgtype.UUID) = ownerID
	*dest[2].(*string) = "group"
	*dest[3].(*string) = ""
	*dest[4].(*string) = botgroups.VisibilityPrivate
	*dest[5].(*[]byte) = []byte(`{}`)
	*dest[6].(*pgtype.Timestamptz) = pgtype.Timestamptz{}
	*dest[7].(*pgtype.Timestamptz) = pgtype.Timestamptz{}
}

func scanPrincipalRole(dest []any, roleID, groupID, principalID pgtype.UUID) {
	*dest[0].(*pgtype.UUID) = roleID
	*dest[1].(*string) = string(rbac.PrincipalGroup)
	*dest[2].(*pgtype.UUID) = principalID
	*dest[3].(*pgtype.UUID) = roleID
	*dest[4].(*string) = string(rbac.ResourceBotGroup)
	*dest[5].(*pgtype.UUID) = groupID
	*dest[6].(*string) = string(rbac.SourceManual)
	*dest[7].(*pgtype.UUID) = pgtype.UUID{}
	*dest[8].(*pgtype.Timestamptz) = pgtype.Timestamptz{}
	*dest[9].(*pgtype.Timestamptz) = pgtype.Timestamptz{}
	*dest[10].(*string) = string(rbac.RoleBotGroupViewer)
	*dest[11].(*string) = string(rbac.ResourceBotGroup)
	*dest[12].(*pgtype.Text) = pgtype.Text{}
	*dest[13].(*pgtype.Text) = pgtype.Text{}
	*dest[14].(*pgtype.Text) = pgtype.Text{}
	*dest[15].(*pgtype.Text) = pgtype.Text{String: "engineering", Valid: true}
	*dest[16].(*pgtype.Text) = pgtype.Text{String: "Engineering", Valid: true}
}
