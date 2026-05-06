package bots

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/acl"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	postgresstore "github.com/memohai/memoh/internal/db/postgres/store"
	"github.com/memohai/memoh/internal/iam/rbac"
)

// fakeRow implements pgx.Row with a custom scan function.
type fakeRow struct {
	scanFunc func(dest ...any) error
}

func (r *fakeRow) Scan(dest ...any) error {
	return r.scanFunc(dest...)
}

// fakeDBTX implements sqlc.DBTX for unit testing.
type fakeDBTX struct {
	queryRowFunc func(ctx context.Context, sql string, args ...any) pgx.Row
}

func (*fakeDBTX) Exec(context.Context, string, ...interface{}) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (*fakeDBTX) Query(context.Context, string, ...interface{}) (pgx.Rows, error) {
	return nil, nil
}

func (d *fakeDBTX) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if d.queryRowFunc != nil {
		return d.queryRowFunc(ctx, sql, args...)
	}
	return &fakeRow{scanFunc: func(_ ...any) error { return pgx.ErrNoRows }}
}

// makeBotRow creates a fakeRow that populates a sqlc.GetBotByIDRow via Scan.
// Column order: id, owner_user_id, group_id, display_name, avatar_url, timezone, is_active, status,
// language, reasoning_enabled, reasoning_effort,
// chat_model_id, search_provider_id, memory_provider_id,
// heartbeat_enabled, heartbeat_interval, heartbeat_prompt,
// compaction_enabled, compaction_threshold, compaction_model_id,
// settings_override_mask, metadata, created_at, updated_at.
func makeBotRow(botID, ownerUserID pgtype.UUID) *fakeRow {
	return makeBotRowWithGroup(botID, ownerUserID, pgtype.UUID{})
}

func makeBotRowWithGroup(botID, ownerUserID, groupID pgtype.UUID) *fakeRow {
	return &fakeRow{
		scanFunc: func(dest ...any) error {
			if len(dest) < 25 {
				return pgx.ErrNoRows
			}
			*dest[0].(*pgtype.UUID) = botID
			*dest[1].(*pgtype.UUID) = ownerUserID
			*dest[2].(*pgtype.UUID) = groupID
			*dest[3].(*pgtype.Text) = pgtype.Text{String: "test-bot", Valid: true}
			*dest[4].(*pgtype.Text) = pgtype.Text{}
			*dest[5].(*pgtype.Text) = pgtype.Text{}
			*dest[6].(*bool) = true
			*dest[7].(*string) = BotStatusReady
			*dest[8].(*string) = "en"                // Language
			*dest[9].(*bool) = false                 // ReasoningEnabled
			*dest[10].(*string) = "medium"           // ReasoningEffort
			*dest[11].(*pgtype.UUID) = pgtype.UUID{} // ChatModelID
			*dest[12].(*pgtype.UUID) = pgtype.UUID{} // SearchProviderID
			*dest[13].(*pgtype.UUID) = pgtype.UUID{} // MemoryProviderID
			*dest[14].(*bool) = false                // HeartbeatEnabled
			*dest[15].(*int32) = 30                  // HeartbeatInterval
			*dest[16].(*string) = ""                 // HeartbeatPrompt
			*dest[17].(*bool) = false                // CompactionEnabled
			*dest[18].(*int32) = 100000              // CompactionThreshold
			*dest[19].(*int32) = 80                  // CompactionRatio
			*dest[20].(*pgtype.UUID) = pgtype.UUID{} // CompactionModelID
			*dest[21].(*[]byte) = []byte(`{}`)
			*dest[22].(*[]byte) = []byte(`{}`)
			*dest[23].(*pgtype.Timestamptz) = pgtype.Timestamptz{}
			*dest[24].(*pgtype.Timestamptz) = pgtype.Timestamptz{}
			return nil
		},
	}
}

func makeUpdateBotProfileRow(botID, ownerUserID, groupID pgtype.UUID) *fakeRow {
	return &fakeRow{
		scanFunc: func(dest ...any) error {
			if len(dest) < 21 {
				return pgx.ErrNoRows
			}
			*dest[0].(*pgtype.UUID) = botID
			*dest[1].(*pgtype.UUID) = ownerUserID
			*dest[2].(*pgtype.UUID) = groupID
			*dest[3].(*pgtype.Text) = pgtype.Text{String: "test-bot", Valid: true}
			*dest[4].(*pgtype.Text) = pgtype.Text{}
			*dest[5].(*pgtype.Text) = pgtype.Text{}
			*dest[6].(*bool) = true
			*dest[7].(*string) = BotStatusReady
			*dest[8].(*string) = "en"
			*dest[9].(*bool) = false
			*dest[10].(*string) = "medium"
			*dest[11].(*pgtype.UUID) = pgtype.UUID{}
			*dest[12].(*pgtype.UUID) = pgtype.UUID{}
			*dest[13].(*pgtype.UUID) = pgtype.UUID{}
			*dest[14].(*bool) = false
			*dest[15].(*int32) = 30
			*dest[16].(*string) = ""
			*dest[17].(*[]byte) = []byte(`{}`)
			*dest[18].(*[]byte) = []byte(`{}`)
			*dest[19].(*pgtype.Timestamptz) = pgtype.Timestamptz{}
			*dest[20].(*pgtype.Timestamptz) = pgtype.Timestamptz{}
			return nil
		},
	}
}

func makeBotGroupRow(groupID, ownerUserID pgtype.UUID) *fakeRow {
	return makeBotGroupRowWithVisibility(groupID, ownerUserID, "private")
}

func makeBotGroupRowWithVisibility(groupID, ownerUserID pgtype.UUID, visibility string) *fakeRow {
	return &fakeRow{
		scanFunc: func(dest ...any) error {
			if len(dest) < 8 {
				return pgx.ErrNoRows
			}
			*dest[0].(*pgtype.UUID) = groupID
			*dest[1].(*pgtype.UUID) = ownerUserID
			*dest[2].(*string) = "test-group"
			*dest[3].(*string) = ""
			*dest[4].(*string) = visibility
			*dest[5].(*[]byte) = []byte(`{}`)
			*dest[6].(*pgtype.Timestamptz) = pgtype.Timestamptz{}
			*dest[7].(*pgtype.Timestamptz) = pgtype.Timestamptz{}
			return nil
		},
	}
}

func mustParseUUID(s string) pgtype.UUID {
	var u pgtype.UUID
	_ = u.Scan(s)
	return u
}

type fakePermissionService struct {
	allowed             bool
	allowedByPermission map[rbac.PermissionKey]bool
	err                 error
	check               rbac.Check
	checks              []rbac.Check
}

func (f *fakePermissionService) HasPermission(_ context.Context, check rbac.Check) (bool, error) {
	f.check = check
	f.checks = append(f.checks, check)
	if f.allowedByPermission != nil {
		return f.allowedByPermission[check.PermissionKey], f.err
	}
	return f.allowed, f.err
}

func TestAuthorizeAccess(t *testing.T) {
	ownerUUID := mustParseUUID("00000000-0000-0000-0000-000000000001")
	botUUID := mustParseUUID("00000000-0000-0000-0000-000000000002")
	strangerUUID := mustParseUUID("00000000-0000-0000-0000-000000000003")
	ownerID := ownerUUID.String()
	botID := botUUID.String()
	strangerID := strangerUUID.String()

	tests := []struct {
		name      string
		userID    string
		allowed   bool
		wantErr   bool
		wantErrIs error
	}{
		{
			name:    "owner always allowed",
			userID:  ownerID,
			allowed: true,
			wantErr: false,
		},
		{
			name:    "rbac allowed",
			userID:  strangerID,
			allowed: true,
			wantErr: false,
		},
		{
			name:      "stranger denied",
			userID:    strangerID,
			wantErr:   true,
			wantErrIs: ErrBotAccessDenied,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &fakeDBTX{
				queryRowFunc: func(_ context.Context, _ string, args ...any) pgx.Row {
					_ = args
					return makeBotRow(botUUID, ownerUUID)
				},
			}
			svc := NewService(nil, postgresstore.NewQueries(sqlc.New(db)))
			permission := &fakePermissionService{allowed: tt.allowed}
			svc.SetRBACService(permission)

			_, err := svc.AuthorizeAccess(context.Background(), tt.userID, botID, false)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.wantErrIs != nil && err.Error() != tt.wantErrIs.Error() {
					t.Fatalf("expected error %q, got %q", tt.wantErrIs, err)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if permission.check.PermissionKey != rbac.PermissionBotRead {
				t.Fatalf("permission = %q, want %q", permission.check.PermissionKey, rbac.PermissionBotRead)
			}
			if permission.check.ResourceType != rbac.ResourceBot || permission.check.ResourceID != botID {
				t.Fatalf("unexpected permission check: %+v", permission.check)
			}
		})
	}
}

func TestCreateRejectsUnknownACLPreset(t *testing.T) {
	ownerUUID := mustParseUUID("00000000-0000-0000-0000-000000000001")
	createCalled := false

	db := &fakeDBTX{
		queryRowFunc: func(_ context.Context, sql string, _ ...any) pgx.Row {
			switch {
			case strings.Contains(sql, "FROM iam_users") && strings.Contains(sql, "WHERE id = $1"):
				return &fakeRow{scanFunc: func(_ ...any) error { return nil }}
			case strings.Contains(sql, "INSERT INTO bots"):
				createCalled = true
				return &fakeRow{scanFunc: func(_ ...any) error { return nil }}
			default:
				return &fakeRow{scanFunc: func(_ ...any) error { return pgx.ErrNoRows }}
			}
		},
	}

	svc := NewService(nil, postgresstore.NewQueries(sqlc.New(db)))
	_, err := svc.Create(context.Background(), ownerUUID.String(), CreateBotRequest{
		DisplayName: "test-bot",
		AclPreset:   "not_a_real_preset",
	})
	if !errors.Is(err, acl.ErrUnknownPreset) {
		t.Fatalf("expected ErrUnknownPreset, got %v", err)
	}
	if createCalled {
		t.Fatal("bot row should not be created when acl preset is invalid")
	}
}

func TestCreateSettingsOverrideMaskInheritsGroupDefaults(t *testing.T) {
	payload, err := createSettingsOverrideMask(true, false)
	if err != nil {
		t.Fatalf("createSettingsOverrideMask returned error: %v", err)
	}
	mask, err := decodeOverrideMask(payload)
	if err != nil {
		t.Fatalf("decodeOverrideMask returned error: %v", err)
	}
	for field, enabled := range mask {
		if enabled {
			t.Fatalf("field %s override = true, want false", field)
		}
	}
	if len(mask) == 0 {
		t.Fatal("inherited mask must include settings fields")
	}
}

func TestCreateSettingsOverrideMaskPreservesExplicitTimezone(t *testing.T) {
	payload, err := createSettingsOverrideMask(true, true)
	if err != nil {
		t.Fatalf("createSettingsOverrideMask returned error: %v", err)
	}
	mask, err := decodeOverrideMask(payload)
	if err != nil {
		t.Fatalf("decodeOverrideMask returned error: %v", err)
	}
	if !mask["timezone"] {
		t.Fatal("timezone override = false, want true")
	}
	if mask["language"] {
		t.Fatal("language override = true, want false")
	}
}

func TestAssignGroupAssignsOwnBotToOwnGroup(t *testing.T) {
	ownerUUID := mustParseUUID("00000000-0000-0000-0000-000000000001")
	botUUID := mustParseUUID("00000000-0000-0000-0000-000000000002")
	groupUUID := mustParseUUID("00000000-0000-0000-0000-000000000003")
	updateCalled := false

	db := &fakeDBTX{
		queryRowFunc: func(_ context.Context, sql string, args ...any) pgx.Row {
			switch {
			case strings.Contains(sql, "FROM bots") && strings.Contains(sql, "WHERE id = $1"):
				return makeBotRow(botUUID, ownerUUID)
			case strings.Contains(sql, "FROM bot_groups") && strings.Contains(sql, "WHERE id ="):
				if args[0] != groupUUID {
					t.Fatalf("unexpected group owner lookup args: %#v", args)
				}
				return makeBotGroupRow(groupUUID, ownerUUID)
			case strings.Contains(sql, "UPDATE bots") && strings.Contains(sql, "group_id = $5"):
				updateCalled = true
				gotGroupID, ok := args[4].(pgtype.UUID)
				if !ok || gotGroupID != groupUUID {
					t.Fatalf("update group id = %#v, want %#v", args[4], groupUUID)
				}
				return makeUpdateBotProfileRow(botUUID, ownerUUID, groupUUID)
			default:
				return &fakeRow{scanFunc: func(_ ...any) error { return pgx.ErrNoRows }}
			}
		},
	}

	svc := NewService(nil, postgresstore.NewQueries(sqlc.New(db)))
	permission := &fakePermissionService{allowed: true}
	svc.SetRBACService(permission)

	bot, err := svc.AssignGroup(context.Background(), ownerUUID.String(), botUUID.String(), groupUUID.String())
	if err != nil {
		t.Fatalf("AssignGroup returned error: %v", err)
	}
	if bot.GroupID != groupUUID.String() {
		t.Fatalf("group id = %q, want %q", bot.GroupID, groupUUID.String())
	}
	if !updateCalled {
		t.Fatal("expected bot profile update")
	}
	if permission.check.PermissionKey != rbac.PermissionBotUpdate {
		t.Fatalf("permission = %q, want %q", permission.check.PermissionKey, rbac.PermissionBotUpdate)
	}
}

func TestAssignGroupRejectsAnotherUsersGroup(t *testing.T) {
	ownerUUID := mustParseUUID("00000000-0000-0000-0000-000000000001")
	botUUID := mustParseUUID("00000000-0000-0000-0000-000000000002")
	groupUUID := mustParseUUID("00000000-0000-0000-0000-000000000003")
	updateCalled := false

	db := &fakeDBTX{
		queryRowFunc: func(_ context.Context, sql string, _ ...any) pgx.Row {
			switch {
			case strings.Contains(sql, "FROM bots") && strings.Contains(sql, "WHERE id = $1"):
				return makeBotRow(botUUID, ownerUUID)
			case strings.Contains(sql, "FROM bot_groups") && strings.Contains(sql, "WHERE id ="):
				return makeBotGroupRow(groupUUID, mustParseUUID("00000000-0000-0000-0000-000000000004"))
			case strings.Contains(sql, "UPDATE bots"):
				updateCalled = true
				return makeUpdateBotProfileRow(botUUID, ownerUUID, groupUUID)
			default:
				return &fakeRow{scanFunc: func(_ ...any) error { return pgx.ErrNoRows }}
			}
		},
	}

	svc := NewService(nil, postgresstore.NewQueries(sqlc.New(db)))
	svc.SetRBACService(&fakePermissionService{
		allowedByPermission: map[rbac.PermissionKey]bool{
			rbac.PermissionBotUpdate: true,
		},
	})

	_, err := svc.AssignGroup(context.Background(), ownerUUID.String(), botUUID.String(), groupUUID.String())
	if !errors.Is(err, ErrBotGroupNotAllowed) {
		t.Fatalf("expected ErrBotGroupNotAllowed, got %v", err)
	}
	if updateCalled {
		t.Fatal("bot profile should not update when group owner check fails")
	}
}

func TestClearGroupPreservesNoGroupBehavior(t *testing.T) {
	ownerUUID := mustParseUUID("00000000-0000-0000-0000-000000000001")
	botUUID := mustParseUUID("00000000-0000-0000-0000-000000000002")
	updateCalled := false

	db := &fakeDBTX{
		queryRowFunc: func(_ context.Context, sql string, args ...any) pgx.Row {
			switch {
			case strings.Contains(sql, "FROM bots") && strings.Contains(sql, "WHERE id = $1"):
				return makeBotRow(botUUID, ownerUUID)
			case strings.Contains(sql, "UPDATE bots") && strings.Contains(sql, "group_id = $5"):
				updateCalled = true
				gotGroupID, ok := args[4].(pgtype.UUID)
				if !ok || gotGroupID.Valid {
					t.Fatalf("update group id = %#v, want invalid pgtype.UUID", args[4])
				}
				return makeUpdateBotProfileRow(botUUID, ownerUUID, pgtype.UUID{})
			default:
				return &fakeRow{scanFunc: func(_ ...any) error { return pgx.ErrNoRows }}
			}
		},
	}

	svc := NewService(nil, postgresstore.NewQueries(sqlc.New(db)))
	svc.SetRBACService(&fakePermissionService{allowed: true})

	bot, err := svc.ClearGroup(context.Background(), ownerUUID.String(), botUUID.String())
	if err != nil {
		t.Fatalf("ClearGroup returned error: %v", err)
	}
	if bot.GroupID != "" {
		t.Fatalf("group id = %q, want empty", bot.GroupID)
	}
	if !updateCalled {
		t.Fatal("expected bot profile update")
	}
}

func TestHasBotPermissionInheritsBotGroupRead(t *testing.T) {
	ownerUUID := mustParseUUID("00000000-0000-0000-0000-000000000001")
	userUUID := mustParseUUID("00000000-0000-0000-0000-000000000004")
	botUUID := mustParseUUID("00000000-0000-0000-0000-000000000002")
	groupUUID := mustParseUUID("00000000-0000-0000-0000-000000000003")

	db := &fakeDBTX{
		queryRowFunc: func(_ context.Context, sql string, _ ...any) pgx.Row {
			switch {
			case strings.Contains(sql, "FROM bots") && strings.Contains(sql, "WHERE id = $1"):
				return makeBotRowWithGroup(botUUID, ownerUUID, groupUUID)
			case strings.Contains(sql, "FROM bot_groups") && strings.Contains(sql, "WHERE id ="):
				return makeBotGroupRow(groupUUID, ownerUUID)
			default:
				return &fakeRow{scanFunc: func(_ ...any) error { return pgx.ErrNoRows }}
			}
		},
	}
	permission := &fakePermissionService{
		allowedByPermission: map[rbac.PermissionKey]bool{
			rbac.PermissionBotGroupRead: true,
		},
	}
	svc := NewService(nil, postgresstore.NewQueries(sqlc.New(db)))
	svc.SetRBACService(permission)

	allowed, err := svc.HasBotPermission(context.Background(), userUUID.String(), botUUID.String(), rbac.PermissionBotRead)
	if err != nil {
		t.Fatalf("HasBotPermission returned error: %v", err)
	}
	if !allowed {
		t.Fatal("bot read = false, want true inherited from bot group")
	}
	if len(permission.checks) != 2 {
		t.Fatalf("permission checks = %d, want 2", len(permission.checks))
	}
	if permission.checks[0].ResourceType != rbac.ResourceBot || permission.checks[0].PermissionKey != rbac.PermissionBotRead {
		t.Fatalf("first check = %+v, want direct bot read", permission.checks[0])
	}
	if permission.checks[1].ResourceType != rbac.ResourceBotGroup || permission.checks[1].PermissionKey != rbac.PermissionBotGroupRead {
		t.Fatalf("second check = %+v, want inherited bot group read", permission.checks[1])
	}
}

func TestHasBotPermissionAllowsOrganizationGroupReadWithoutRole(t *testing.T) {
	ownerUUID := mustParseUUID("00000000-0000-0000-0000-000000000001")
	userUUID := mustParseUUID("00000000-0000-0000-0000-000000000004")
	botUUID := mustParseUUID("00000000-0000-0000-0000-000000000002")
	groupUUID := mustParseUUID("00000000-0000-0000-0000-000000000003")

	db := &fakeDBTX{
		queryRowFunc: func(_ context.Context, sql string, _ ...any) pgx.Row {
			switch {
			case strings.Contains(sql, "FROM bots") && strings.Contains(sql, "WHERE id = $1"):
				return makeBotRowWithGroup(botUUID, ownerUUID, groupUUID)
			case strings.Contains(sql, "FROM bot_groups") && strings.Contains(sql, "WHERE id ="):
				return makeBotGroupRowWithVisibility(groupUUID, ownerUUID, "organization")
			default:
				return &fakeRow{scanFunc: func(_ ...any) error { return pgx.ErrNoRows }}
			}
		},
	}
	svc := NewService(nil, postgresstore.NewQueries(sqlc.New(db)))
	svc.SetRBACService(&fakePermissionService{})

	allowed, err := svc.HasBotPermission(context.Background(), userUUID.String(), botUUID.String(), rbac.PermissionBotChat)
	if err != nil {
		t.Fatalf("HasBotPermission returned error: %v", err)
	}
	if !allowed {
		t.Fatal("bot chat = false, want true from organization visibility")
	}
}

func TestHasBotPermissionDeniesPrivateGroupWithoutRole(t *testing.T) {
	ownerUUID := mustParseUUID("00000000-0000-0000-0000-000000000001")
	userUUID := mustParseUUID("00000000-0000-0000-0000-000000000004")
	botUUID := mustParseUUID("00000000-0000-0000-0000-000000000002")
	groupUUID := mustParseUUID("00000000-0000-0000-0000-000000000003")

	db := &fakeDBTX{
		queryRowFunc: func(_ context.Context, sql string, _ ...any) pgx.Row {
			switch {
			case strings.Contains(sql, "FROM bots") && strings.Contains(sql, "WHERE id = $1"):
				return makeBotRowWithGroup(botUUID, ownerUUID, groupUUID)
			case strings.Contains(sql, "FROM bot_groups") && strings.Contains(sql, "WHERE id ="):
				return makeBotGroupRow(groupUUID, ownerUUID)
			default:
				return &fakeRow{scanFunc: func(_ ...any) error { return pgx.ErrNoRows }}
			}
		},
	}
	svc := NewService(nil, postgresstore.NewQueries(sqlc.New(db)))
	svc.SetRBACService(&fakePermissionService{})

	allowed, err := svc.HasBotPermission(context.Background(), userUUID.String(), botUUID.String(), rbac.PermissionBotRead)
	if err != nil {
		t.Fatalf("HasBotPermission returned error: %v", err)
	}
	if allowed {
		t.Fatal("private group bot read = true, want false without role")
	}
}
