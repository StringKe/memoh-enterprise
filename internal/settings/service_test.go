package settings

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	postgresstore "github.com/memohai/memoh/internal/db/postgres/store"
)

type fakeSettingsRow struct {
	scanFunc func(dest ...any) error
}

func (r *fakeSettingsRow) Scan(dest ...any) error {
	return r.scanFunc(dest...)
}

type fakeSettingsDBTX struct {
	queryRowFunc func(ctx context.Context, sql string, args ...any) pgx.Row
	execFunc     func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func (d *fakeSettingsDBTX) Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	if d.execFunc != nil {
		return d.execFunc(ctx, sql, args...)
	}
	return pgconn.CommandTag{}, nil
}

func (*fakeSettingsDBTX) Query(context.Context, string, ...interface{}) (pgx.Rows, error) {
	return nil, nil
}

func (d *fakeSettingsDBTX) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if d.queryRowFunc != nil {
		return d.queryRowFunc(ctx, sql, args...)
	}
	return &fakeSettingsRow{scanFunc: func(_ ...any) error { return pgx.ErrNoRows }}
}

func TestNormalizeBotSettingsReadRow_ShowToolCallsInIMDefault(t *testing.T) {
	t.Parallel()

	row := sqlc.GetSettingsByBotIDRow{
		Language:            "en",
		ReasoningEnabled:    false,
		ReasoningEffort:     "medium",
		HeartbeatEnabled:    false,
		HeartbeatInterval:   60,
		CompactionEnabled:   false,
		CompactionThreshold: 0,
		CompactionRatio:     80,
		ShowToolCallsInIm:   false,
	}
	got := normalizeBotSettingsReadRow(row)
	if got.ShowToolCallsInIM {
		t.Fatalf("expected default ShowToolCallsInIM=false, got true")
	}
}

func TestOverrideEnabledDefaultsToLocalOverride(t *testing.T) {
	t.Parallel()

	if !overrideEnabled(nil, FieldLanguage) {
		t.Fatal("nil mask must preserve legacy local override")
	}
	if !overrideEnabled(OverrideMask{}, FieldLanguage) {
		t.Fatal("missing field must preserve legacy local override")
	}
	if overrideEnabled(OverrideMask{FieldLanguage: false}, FieldLanguage) {
		t.Fatal("explicit false must restore inheritance")
	}
}

func TestApplyGroupDefaultsUsesGroupWhenOverrideDisabled(t *testing.T) {
	t.Parallel()

	groupID := parseSettingsTestUUID(t, "00000000-0000-0000-0000-000000000010")
	local := Settings{
		Language:           "en",
		ReasoningEffort:    "medium",
		HeartbeatInterval:  30,
		ToolApprovalConfig: DefaultToolApprovalConfig(),
		OverlayConfig:      map[string]any{},
	}
	group := sqlc.BotGroupSetting{
		GroupID:           groupID,
		Language:          pgtype.Text{String: "ja", Valid: true},
		ReasoningEffort:   pgtype.Text{String: "high", Valid: true},
		HeartbeatInterval: pgtype.Int4{Int32: 90, Valid: true},
	}

	got := applyGroupDefaults(local, group, OverrideMask{
		FieldLanguage:          false,
		FieldReasoningEffort:   true,
		FieldHeartbeatInterval: false,
	})

	if got.Language != "ja" {
		t.Fatalf("language = %q, want group default ja", got.Language)
	}
	if got.ReasoningEffort != "medium" {
		t.Fatalf("reasoning effort = %q, want local medium", got.ReasoningEffort)
	}
	if got.HeartbeatInterval != 90 {
		t.Fatalf("heartbeat interval = %d, want group default 90", got.HeartbeatInterval)
	}
	assertSource(t, got.Sources, FieldLanguage, SourceBotGroup)
	assertSource(t, got.Sources, FieldReasoningEffort, SourceBot)
}

func TestGetBotUsesSystemSourceWhenGroupDefaultsMissing(t *testing.T) {
	botUUID := parseSettingsTestUUID(t, "00000000-0000-0000-0000-000000000001")
	ownerUUID := parseSettingsTestUUID(t, "00000000-0000-0000-0000-000000000002")
	groupUUID := parseSettingsTestUUID(t, "00000000-0000-0000-0000-000000000003")
	mask := []byte(`{"language":false,"reasoning_effort":true}`)

	db := &fakeSettingsDBTX{
		queryRowFunc: func(_ context.Context, sql string, _ ...any) pgx.Row {
			switch {
			case strings.Contains(sql, "SELECT id, owner_user_id") && strings.Contains(sql, "FROM bots"):
				return makeSettingsBotRow(botUUID, ownerUUID, mask, groupUUID)
			case strings.Contains(sql, "FROM bots") && strings.Contains(sql, "LEFT JOIN bot_group_settings"):
				return makeSettingsReadRow()
			case strings.Contains(sql, "FROM bot_group_settings"):
				return &fakeSettingsRow{scanFunc: func(_ ...any) error { return pgx.ErrNoRows }}
			default:
				return &fakeSettingsRow{scanFunc: func(_ ...any) error { return pgx.ErrNoRows }}
			}
		},
	}
	svc := NewService(nil, postgresstore.NewQueries(sqlc.New(db)), nil, nil)

	got, err := svc.GetBot(context.Background(), botUUID.String())
	if err != nil {
		t.Fatalf("GetBot returned error: %v", err)
	}
	if got.Language != "en" {
		t.Fatalf("language = %q, want system default en", got.Language)
	}
	assertSource(t, got.Sources, FieldLanguage, SourceSystem)
	assertSource(t, got.Sources, FieldReasoningEffort, SourceBot)
}

func TestDecodeOverrideMaskInvalidPayloadPreservesLocalOverride(t *testing.T) {
	t.Parallel()

	mask := decodeOverrideMask([]byte(`{`))
	if !overrideEnabled(mask, FieldLanguage) {
		t.Fatal("invalid mask must preserve local override")
	}
	valid, err := json.Marshal(OverrideMask{FieldLanguage: false})
	if err != nil {
		t.Fatalf("marshal mask: %v", err)
	}
	mask = decodeOverrideMask(valid)
	if overrideEnabled(mask, FieldLanguage) {
		t.Fatal("decoded explicit false must restore inheritance")
	}
}

func parseSettingsTestUUID(t *testing.T, raw string) pgtype.UUID {
	t.Helper()
	var id pgtype.UUID
	if err := id.Scan(raw); err != nil {
		t.Fatalf("parse uuid: %v", err)
	}
	return id
}

func assertSource(t *testing.T, sources []FieldSource, field, source string) {
	t.Helper()
	for _, item := range sources {
		if item.Field == field {
			if item.Source != source {
				t.Fatalf("source for %s = %s, want %s", field, item.Source, source)
			}
			return
		}
	}
	t.Fatalf("source for %s not found in %#v", field, sources)
}

func TestNormalizeBotSettingsReadRow_ShowToolCallsInIMPropagates(t *testing.T) {
	t.Parallel()

	row := sqlc.GetSettingsByBotIDRow{
		Language:          "en",
		ReasoningEffort:   "medium",
		HeartbeatInterval: 60,
		CompactionRatio:   80,
		ShowToolCallsInIm: true,
	}
	got := normalizeBotSettingsReadRow(row)
	if !got.ShowToolCallsInIM {
		t.Fatalf("expected ShowToolCallsInIM=true to propagate from row")
	}
}

func TestUpsertRequestShowToolCallsInIM_PointerSemantics(t *testing.T) {
	t.Parallel()

	// When the field is nil, the UpsertRequest should not touch the current
	// setting. When non-nil, the dereferenced value should win. We exercise
	// the small gate block without hitting the database.
	current := Settings{ShowToolCallsInIM: true}

	var req UpsertRequest
	if req.ShowToolCallsInIM != nil {
		current.ShowToolCallsInIM = *req.ShowToolCallsInIM
	}
	if !current.ShowToolCallsInIM {
		t.Fatalf("nil pointer must leave current value unchanged")
	}

	off := false
	req.ShowToolCallsInIM = &off
	if req.ShowToolCallsInIM != nil {
		current.ShowToolCallsInIM = *req.ShowToolCallsInIM
	}
	if current.ShowToolCallsInIM {
		t.Fatalf("explicit false pointer must clear the flag")
	}
}

func TestUpsertBotSettingsUsesPresenceFlagsForExplicitClear(t *testing.T) {
	botUUID := parseSettingsTestUUID(t, "00000000-0000-0000-0000-000000000001")
	ownerUUID := parseSettingsTestUUID(t, "00000000-0000-0000-0000-000000000002")
	clearChatModel := ""
	var upsertArgs []any

	db := &fakeSettingsDBTX{
		queryRowFunc: func(_ context.Context, sql string, args ...any) pgx.Row {
			switch {
			case strings.Contains(sql, "WITH updated AS"):
				upsertArgs = append([]any(nil), args...)
				return makeSettingsReadRow()
			case strings.Contains(sql, "SELECT id, owner_user_id") && strings.Contains(sql, "FROM bots"):
				return makeSettingsBotRow(botUUID, ownerUUID, []byte(`{}`))
			case strings.Contains(sql, "SELECT\n  overlay_enabled") && strings.Contains(sql, "overlay_provider"):
				return makeSettingsOverlayRow()
			case strings.Contains(sql, "FROM bots") && strings.Contains(sql, "LEFT JOIN models"):
				return makeSettingsReadRow()
			default:
				return &fakeSettingsRow{scanFunc: func(_ ...any) error { return pgx.ErrNoRows }}
			}
		},
	}
	svc := NewService(nil, postgresstore.NewQueries(sqlc.New(db)), nil, nil)

	if _, err := svc.UpsertBot(context.Background(), botUUID.String(), UpsertRequest{ChatModelID: &clearChatModel}); err != nil {
		t.Fatalf("UpsertBot returned error: %v", err)
	}
	if len(upsertArgs) < 13 {
		t.Fatalf("upsert args length = %d, want at least 13", len(upsertArgs))
	}
	if upsertArgs[9].(bool) {
		t.Fatalf("timezone present = true, want false for absent field")
	}
	if !upsertArgs[11].(bool) {
		t.Fatalf("chat_model_id present = false, want true for explicit clear")
	}
	if got := upsertArgs[12].(pgtype.UUID); got.Valid {
		t.Fatalf("chat_model_id uuid valid = true, want SQL NULL for explicit clear")
	}
}

func TestRestoreInheritanceWritesFalseMask(t *testing.T) {
	botUUID := parseSettingsTestUUID(t, "00000000-0000-0000-0000-000000000001")
	ownerUUID := parseSettingsTestUUID(t, "00000000-0000-0000-0000-000000000002")
	var writtenMask OverrideMask
	currentMask := []byte(`{"language":true,"timezone":true}`)

	db := &fakeSettingsDBTX{
		queryRowFunc: func(_ context.Context, sql string, _ ...any) pgx.Row {
			switch {
			case strings.Contains(sql, "FROM bots") && strings.Contains(sql, "WHERE id = $1"):
				return makeSettingsBotRow(botUUID, ownerUUID, currentMask)
			case strings.Contains(sql, "FROM bots") && strings.Contains(sql, "LEFT JOIN models"):
				return makeSettingsReadRow()
			default:
				return &fakeSettingsRow{scanFunc: func(_ ...any) error { return pgx.ErrNoRows }}
			}
		},
		execFunc: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			if !strings.Contains(sql, "settings_override_mask = $1::jsonb") {
				return pgconn.CommandTag{}, nil
			}
			if err := json.Unmarshal(args[0].([]byte), &writtenMask); err != nil {
				t.Fatalf("unmarshal written mask: %v", err)
			}
			currentMask = args[0].([]byte)
			if args[1] != botUUID {
				t.Fatalf("bot id arg = %#v, want %#v", args[1], botUUID)
			}
			return pgconn.CommandTag{}, nil
		},
	}
	svc := NewService(nil, postgresstore.NewQueries(sqlc.New(db)), nil, nil)

	got, err := svc.RestoreInheritance(context.Background(), botUUID.String(), []string{FieldLanguage})
	if err != nil {
		t.Fatalf("RestoreInheritance returned error: %v", err)
	}
	if writtenMask[FieldLanguage] {
		t.Fatalf("language mask = true, want false")
	}
	if !writtenMask[FieldTimezone] {
		t.Fatalf("timezone mask = false, want preserved true")
	}
	if got.OverrideMask[FieldLanguage] {
		t.Fatalf("effective language mask = true, want false")
	}
}

func makeSettingsBotRow(botID, ownerUserID pgtype.UUID, mask []byte, groupID ...pgtype.UUID) *fakeSettingsRow {
	return &fakeSettingsRow{
		scanFunc: func(dest ...any) error {
			if len(dest) < 26 {
				return pgx.ErrNoRows
			}
			var scannedGroupID pgtype.UUID
			if len(groupID) > 0 {
				scannedGroupID = groupID[0]
			}
			*dest[0].(*pgtype.UUID) = botID
			*dest[1].(*pgtype.UUID) = ownerUserID
			*dest[2].(*pgtype.UUID) = scannedGroupID
			*dest[3].(*pgtype.Text) = pgtype.Text{String: "test-bot", Valid: true}
			*dest[4].(*pgtype.Text) = pgtype.Text{}
			*dest[5].(*pgtype.Text) = pgtype.Text{}
			*dest[6].(*bool) = true
			*dest[7].(*string) = "ready"
			*dest[8].(*string) = "en"
			*dest[9].(*bool) = false
			*dest[10].(*string) = "medium"
			*dest[11].(*pgtype.UUID) = pgtype.UUID{}
			*dest[12].(*pgtype.UUID) = pgtype.UUID{}
			*dest[13].(*pgtype.UUID) = pgtype.UUID{}
			*dest[14].(*bool) = false
			*dest[15].(*int32) = 30
			*dest[16].(*string) = ""
			*dest[17].(*bool) = false
			*dest[18].(*bool) = false
			*dest[19].(*int32) = 100000
			*dest[20].(*int32) = 80
			*dest[21].(*pgtype.UUID) = pgtype.UUID{}
			*dest[22].(*[]byte) = mask
			*dest[23].(*[]byte) = []byte(`{}`)
			*dest[24].(*pgtype.Timestamptz) = pgtype.Timestamptz{}
			*dest[25].(*pgtype.Timestamptz) = pgtype.Timestamptz{}
			return nil
		},
	}
}

func makeSettingsReadRow() *fakeSettingsRow {
	return &fakeSettingsRow{
		scanFunc: func(dest ...any) error {
			if len(dest) < 27 {
				return pgx.ErrNoRows
			}
			*dest[0].(*pgtype.UUID) = pgtype.UUID{}
			*dest[1].(*string) = "en"
			*dest[2].(*bool) = false
			*dest[3].(*string) = "medium"
			*dest[4].(*bool) = false
			*dest[5].(*int32) = 30
			*dest[6].(*string) = ""
			*dest[7].(*bool) = false
			*dest[8].(*int32) = 100000
			*dest[9].(*int32) = 80
			*dest[10].(*pgtype.Text) = pgtype.Text{}
			for i := 11; i <= 20; i++ {
				*dest[i].(*pgtype.UUID) = pgtype.UUID{}
			}
			*dest[21].(*bool) = false
			*dest[22].(*bool) = false
			*dest[23].(*[]byte) = []byte(`{}`)
			*dest[24].(*string) = ""
			*dest[25].(*bool) = false
			*dest[26].(*[]byte) = []byte(`{}`)
			return nil
		},
	}
}

func makeSettingsOverlayRow() *fakeSettingsRow {
	return &fakeSettingsRow{
		scanFunc: func(dest ...any) error {
			if len(dest) < 3 {
				return pgx.ErrNoRows
			}
			*dest[0].(*bool) = false
			*dest[1].(*string) = ""
			*dest[2].(*[]byte) = []byte(`{}`)
			return nil
		},
	}
}
