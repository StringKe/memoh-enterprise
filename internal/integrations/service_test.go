package integrations

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	integrationv1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/integration/v1"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	postgresstore "github.com/memohai/memoh/internal/db/postgres/store"
)

type fakeIntegrationRow struct {
	scanFunc func(dest ...any) error
}

func (r *fakeIntegrationRow) Scan(dest ...any) error {
	return r.scanFunc(dest...)
}

type fakeIntegrationDBTX struct {
	queryRowFunc func(ctx context.Context, sql string, args ...any) pgx.Row
	execFunc     func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func (db *fakeIntegrationDBTX) Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	if db.execFunc != nil {
		return db.execFunc(ctx, sql, args...)
	}
	return pgconn.CommandTag{}, nil
}

func (*fakeIntegrationDBTX) Query(context.Context, string, ...interface{}) (pgx.Rows, error) {
	return nil, nil
}

func (db *fakeIntegrationDBTX) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if db.queryRowFunc != nil {
		return db.queryRowFunc(ctx, sql, args...)
	}
	return &fakeIntegrationRow{scanFunc: func(_ ...any) error { return pgx.ErrNoRows }}
}

func TestHashTokenIsStableAndDoesNotExposeRawToken(t *testing.T) {
	t.Parallel()

	raw := "memoh_it_test"
	got := HashToken(raw)
	if got != HashToken(raw) {
		t.Fatal("hash must be stable")
	}
	if got == raw {
		t.Fatal("hash must not equal raw token")
	}
	if len(got) != 64 {
		t.Fatalf("hash length = %d, want 64 hex chars", len(got))
	}
}

func TestParseScopeValidation(t *testing.T) {
	t.Parallel()

	if botID, groupID, err := parseScope(ScopeGlobal, "", ""); err != nil || botID.Valid || groupID.Valid {
		t.Fatalf("global scope = (%#v, %#v, %v), want empty ids and nil error", botID, groupID, err)
	}
	if _, _, err := parseScope(ScopeBot, "", ""); err == nil {
		t.Fatal("bot scope without scope_bot_id must fail")
	}
	if _, _, err := parseScope(ScopeBotGroup, "", ""); err == nil {
		t.Fatal("bot_group scope without scope_bot_group_id must fail")
	}
	if _, _, err := parseScope("invalid", "", ""); err == nil {
		t.Fatal("invalid scope must fail")
	}
}

func TestGenerateRawTokenPrefix(t *testing.T) {
	t.Parallel()

	token, err := generateRawToken()
	if err != nil {
		t.Fatalf("generateRawToken returned error: %v", err)
	}
	if len(token) <= len("memoh_it_") || token[:len("memoh_it_")] != "memoh_it_" {
		t.Fatalf("token prefix = %q, want memoh_it_", token)
	}
}

func TestAuthorizeBotRejectsDisallowedAction(t *testing.T) {
	t.Parallel()

	service := NewService(nil, nil)
	identity := TokenIdentity{Token: APIToken{
		ScopeType:          ScopeGlobal,
		AllowedActionTypes: []string{"send_message"},
	}}
	if err := service.AuthorizeBot(context.Background(), identity, "bot-1", "request_action"); err == nil {
		t.Fatal("disallowed action must fail")
	}
}

func TestAuthorizeBotRejectsBotScopeForOtherBot(t *testing.T) {
	t.Parallel()

	service := NewService(nil, nil)
	identity := TokenIdentity{Token: APIToken{
		ScopeType:          ScopeBot,
		ScopeBotID:         "bot-1",
		AllowedActionTypes: []string{"send_message"},
	}}
	if err := service.AuthorizeBot(context.Background(), identity, "bot-2", "send_message"); err == nil {
		t.Fatal("bot scoped token must reject other bots")
	}
}

func TestAuthorizeBotGroupScope(t *testing.T) {
	t.Parallel()

	service := NewService(nil, nil)
	identity := TokenIdentity{Token: APIToken{
		ScopeType:       ScopeBotGroup,
		ScopeBotGroupID: "group-1",
	}}
	if err := service.AuthorizeBotGroup(identity, "group-1"); err != nil {
		t.Fatalf("matching bot group scope returned error: %v", err)
	}
	if err := service.AuthorizeBotGroup(identity, "group-2"); err == nil {
		t.Fatal("bot group scope must reject other groups")
	}
}

func TestAuthorizeBotRejectsBotGroupScopeForBotOutsideGroup(t *testing.T) {
	t.Parallel()

	botID := "00000000-0000-0000-0000-000000000101"
	groupID := parseIntegrationTestUUID(t, "00000000-0000-0000-0000-000000000201")
	otherGroupID := "00000000-0000-0000-0000-000000000202"
	db := &fakeIntegrationDBTX{
		queryRowFunc: func(context.Context, string, ...any) pgx.Row {
			return makeBotByIDRow(groupID)
		},
	}
	service := NewService(nil, postgresstore.NewQueries(sqlc.New(db)))
	identity := TokenIdentity{Token: APIToken{
		ScopeType:          ScopeBotGroup,
		ScopeBotGroupID:    otherGroupID,
		AllowedActionTypes: []string{"send_message"},
	}}
	if err := service.AuthorizeBot(context.Background(), identity, botID, "send_message"); err == nil {
		t.Fatal("bot_group scoped token must reject bots outside the group")
	}
}

func TestAuthorizeEventRejectsDisallowedEventType(t *testing.T) {
	t.Parallel()

	service := NewService(nil, nil)
	identity := TokenIdentity{Token: APIToken{
		AllowedEventTypes: []string{"message.created"},
	}}
	if err := service.AuthorizeEvent(identity, "message.deleted"); err == nil {
		t.Fatal("event type outside token allowlist must fail")
	}
}

func TestValidateTokenTouchesActiveToken(t *testing.T) {
	raw := "memoh_it_test"
	tokenID := parseIntegrationTestUUID(t, "00000000-0000-0000-0000-000000000001")
	touched := false
	db := &fakeIntegrationDBTX{
		queryRowFunc: func(_ context.Context, _ string, args ...any) pgx.Row {
			if args[0] != HashToken(raw) {
				t.Fatalf("token hash arg = %q, want %q", args[0], HashToken(raw))
			}
			return makeIntegrationTokenRow(tokenID, ScopeGlobal, pgtype.UUID{}, pgtype.UUID{}, pgtype.Timestamptz{}, pgtype.Timestamptz{})
		},
		execFunc: func(_ context.Context, _ string, args ...any) (pgconn.CommandTag, error) {
			if args[0] != tokenID {
				t.Fatalf("touch id arg = %#v, want %#v", args[0], tokenID)
			}
			touched = true
			return pgconn.CommandTag{}, nil
		},
	}
	service := NewService(nil, postgresstore.NewQueries(sqlc.New(db)))

	identity, err := service.ValidateToken(context.Background(), raw)
	if err != nil {
		t.Fatalf("ValidateToken returned error: %v", err)
	}
	if identity.Token.ID != tokenID.String() {
		t.Fatalf("identity token id = %q, want %q", identity.Token.ID, tokenID.String())
	}
	if !touched {
		t.Fatal("expected last_used_at touch")
	}
}

func TestHubAckPreventsReplayAfterReconnect(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	identity := TokenIdentity{Token: APIToken{ID: "token-1"}}
	first := hub.Register(identity, func(*integrationv1.Envelope) error { return nil })
	if _, err := hub.Subscribe(first, []string{"bot-1"}, nil, nil); err != nil {
		t.Fatalf("subscribe first connection: %v", err)
	}
	hub.Publish(&integrationv1.IntegrationEvent{
		EventId:   "event-1",
		EventType: "message.created",
		BotId:     "bot-1",
	})
	if !hub.Ack("token-1", "event-1") {
		t.Fatal("first ack should be recorded")
	}
	if hub.Ack("token-1", "event-1") {
		t.Fatal("duplicate ack should be deduplicated")
	}
	hub.Unregister(first)

	second := hub.Register(identity, func(*integrationv1.Envelope) error { return nil })
	replay, err := hub.Subscribe(second, []string{"bot-1"}, nil, nil)
	if err != nil {
		t.Fatalf("subscribe second connection: %v", err)
	}
	if len(replay) != 0 {
		t.Fatalf("replay count = %d, want 0 after ack", len(replay))
	}
	if got := hub.LastAckedEventID("token-1"); got != "event-1" {
		t.Fatalf("last acked = %q, want event-1", got)
	}
}

func TestHubCreateSessionBindsExternalSessionID(t *testing.T) {
	t.Parallel()

	hub := NewHub()
	first := hub.CreateOrBindSession("token-1", "bot-1", "external-1", map[string]string{"source": "test"})
	second := hub.CreateOrBindSession("token-1", "bot-1", "external-1", nil)
	if first.ID != second.ID {
		t.Fatalf("bound session id = %q, want %q", second.ID, first.ID)
	}
	session, ok := hub.Session(first.ID)
	if !ok {
		t.Fatal("created session must be readable by session id")
	}
	if session.BotID != "bot-1" || session.ExternalSessionID != "external-1" {
		t.Fatalf("session = %#v, want bot/external ids preserved", session)
	}
}

func TestResponseEnvelopeCarriesProtocolVersionAndPayload(t *testing.T) {
	t.Parallel()

	envelope := responseEnvelope("corr-1", &integrationv1.Envelope_Pong{Pong: &integrationv1.Pong{}})
	if envelope.GetVersion() != wsProtocolVersion {
		t.Fatalf("version = %q, want %q", envelope.GetVersion(), wsProtocolVersion)
	}
	if envelope.GetCorrelationId() != "corr-1" {
		t.Fatalf("correlation id = %q, want corr-1", envelope.GetCorrelationId())
	}
	if envelope.GetPong() == nil {
		t.Fatal("pong payload is missing")
	}
}

func TestWebSocketHeartbeatPingAndReadTimeoutClose(t *testing.T) {
	tokenID := parseIntegrationTestUUID(t, "00000000-0000-0000-0000-000000000001")
	db := &fakeIntegrationDBTX{
		queryRowFunc: func(context.Context, string, ...any) pgx.Row {
			return makeIntegrationTokenRow(tokenID, ScopeGlobal, pgtype.UUID{}, pgtype.UUID{}, pgtype.Timestamptz{}, pgtype.Timestamptz{})
		},
	}
	service := NewService(nil, postgresstore.NewQueries(sqlc.New(db)))
	handler := NewWebSocketHandler(nil, service)
	handler.heartbeatTimeout = 50 * time.Millisecond
	server := httptest.NewServer(handler.HTTPHandler())
	defer server.Close()

	wsURL := "ws" + server.URL[len("http"):] + "/integration/v1/ws"
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if resp != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer func() { _ = conn.Close() }()

	writeTestEnvelope(t, conn, &integrationv1.Envelope{
		Version:       wsProtocolVersion,
		MessageId:     "auth-1",
		CorrelationId: "auth-1",
		Payload:       &integrationv1.Envelope_AuthRequest{AuthRequest: &integrationv1.AuthRequest{Token: "memoh_it_test"}},
	})
	auth := readTestEnvelope(t, conn)
	if auth.GetAuthResponse() == nil {
		t.Fatalf("auth response payload missing: %#v", auth.GetPayload())
	}

	writeTestEnvelope(t, conn, &integrationv1.Envelope{
		Version:       wsProtocolVersion,
		MessageId:     "ping-1",
		CorrelationId: "ping-1",
		Payload:       &integrationv1.Envelope_Ping{Ping: &integrationv1.Ping{}},
	})
	pong := readTestEnvelope(t, conn)
	if pong.GetPong() == nil {
		t.Fatalf("pong payload missing: %#v", pong.GetPayload())
	}

	_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	if _, _, err := conn.ReadMessage(); err == nil {
		t.Fatal("idle websocket should close after heartbeat timeout")
	}
}

func TestWebSocketCreateSessionBindsExternalSessionID(t *testing.T) {
	tokenID := parseIntegrationTestUUID(t, "00000000-0000-0000-0000-000000000001")
	db := &fakeIntegrationDBTX{
		queryRowFunc: func(context.Context, string, ...any) pgx.Row {
			return makeIntegrationTokenRow(tokenID, ScopeGlobal, pgtype.UUID{}, pgtype.UUID{}, pgtype.Timestamptz{}, pgtype.Timestamptz{})
		},
	}
	service := NewService(nil, postgresstore.NewQueries(sqlc.New(db)))
	handler := NewWebSocketHandler(nil, service)
	conn, server := startIntegrationWSTest(t, handler)
	defer server.Close()
	defer func() { _ = conn.Close() }()

	create := func(correlationID string) string {
		writeTestEnvelope(t, conn, &integrationv1.Envelope{
			Version:       wsProtocolVersion,
			MessageId:     correlationID,
			CorrelationId: correlationID,
			Payload: &integrationv1.Envelope_CreateSessionRequest{CreateSessionRequest: &integrationv1.CreateSessionRequest{
				BotId:             "bot-1",
				ExternalSessionId: "external-1",
			}},
		})
		resp := readTestEnvelope(t, conn).GetCreateSessionResponse()
		if resp == nil {
			t.Fatal("create session response missing")
		}
		return resp.GetSessionId()
	}
	first := create("create-1")
	second := create("create-2")
	if first != second {
		t.Fatalf("bound session = %q, want %q", second, first)
	}
}

func TestValidateTokenRejectsExpiredToken(t *testing.T) {
	raw := "memoh_it_test"
	tokenID := parseIntegrationTestUUID(t, "00000000-0000-0000-0000-000000000001")
	db := &fakeIntegrationDBTX{
		queryRowFunc: func(context.Context, string, ...any) pgx.Row {
			return makeIntegrationTokenRow(tokenID, ScopeGlobal, pgtype.UUID{}, pgtype.UUID{}, pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true}, pgtype.Timestamptz{})
		},
	}
	service := NewService(nil, postgresstore.NewQueries(sqlc.New(db)))

	if _, err := service.ValidateToken(context.Background(), raw); err == nil {
		t.Fatal("expired token must fail")
	}
}

func startIntegrationWSTest(t *testing.T, handler *WebSocketHandler) (*websocket.Conn, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler.HTTPHandler())
	wsURL := "ws" + server.URL[len("http"):] + "/integration/v1/ws"
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if resp != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		server.Close()
		t.Fatalf("dial websocket: %v", err)
	}
	writeTestEnvelope(t, conn, &integrationv1.Envelope{
		Version:       wsProtocolVersion,
		MessageId:     "auth-1",
		CorrelationId: "auth-1",
		Payload:       &integrationv1.Envelope_AuthRequest{AuthRequest: &integrationv1.AuthRequest{Token: "memoh_it_test"}},
	})
	if auth := readTestEnvelope(t, conn); auth.GetAuthResponse() == nil {
		t.Fatalf("auth response payload missing: %#v", auth.GetPayload())
	}
	return conn, server
}

func writeTestEnvelope(t *testing.T, conn *websocket.Conn, envelope *integrationv1.Envelope) {
	t.Helper()
	payload, err := marshalEnvelope(envelope)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
		t.Fatalf("write websocket envelope: %v", err)
	}
}

func readTestEnvelope(t *testing.T, conn *websocket.Conn) *integrationv1.Envelope {
	t.Helper()
	var envelope integrationv1.Envelope
	if err := readEnvelope(conn, &envelope); err != nil {
		t.Fatalf("read websocket envelope: %v", err)
	}
	return &envelope
}

func parseIntegrationTestUUID(t *testing.T, raw string) pgtype.UUID {
	t.Helper()
	var id pgtype.UUID
	if err := id.Scan(raw); err != nil {
		t.Fatalf("parse uuid: %v", err)
	}
	return id
}

func makeIntegrationTokenRow(id pgtype.UUID, scope string, botID, groupID pgtype.UUID, expiresAt, disabledAt pgtype.Timestamptz) *fakeIntegrationRow {
	return &fakeIntegrationRow{
		scanFunc: func(dest ...any) error {
			if len(dest) < 14 {
				return pgx.ErrNoRows
			}
			*dest[0].(*pgtype.UUID) = id
			*dest[1].(*string) = "test-token"
			*dest[2].(*string) = "hash"
			*dest[3].(*string) = scope
			*dest[4].(*pgtype.UUID) = botID
			*dest[5].(*pgtype.UUID) = groupID
			*dest[6].(*[]byte) = []byte(`[]`)
			*dest[7].(*[]byte) = []byte(`[]`)
			*dest[8].(*pgtype.Timestamptz) = expiresAt
			*dest[9].(*pgtype.Timestamptz) = disabledAt
			*dest[10].(*pgtype.Timestamptz) = pgtype.Timestamptz{}
			*dest[11].(*pgtype.UUID) = pgtype.UUID{}
			*dest[12].(*pgtype.Timestamptz) = pgtype.Timestamptz{}
			*dest[13].(*pgtype.Timestamptz) = pgtype.Timestamptz{}
			return nil
		},
	}
}

func makeBotByIDRow(groupID pgtype.UUID) *fakeIntegrationRow {
	return &fakeIntegrationRow{
		scanFunc: func(dest ...any) error {
			if len(dest) < 25 {
				return pgx.ErrNoRows
			}
			*dest[0].(*pgtype.UUID) = pgtype.UUID{Valid: true}
			*dest[1].(*pgtype.UUID) = pgtype.UUID{Valid: true}
			*dest[2].(*pgtype.UUID) = groupID
			*dest[3].(*pgtype.Text) = pgtype.Text{String: "test-bot", Valid: true}
			*dest[4].(*pgtype.Text) = pgtype.Text{}
			*dest[5].(*pgtype.Text) = pgtype.Text{String: "UTC", Valid: true}
			*dest[6].(*bool) = true
			*dest[7].(*string) = "active"
			*dest[8].(*string) = "en"
			*dest[9].(*bool) = false
			*dest[10].(*string) = ""
			*dest[11].(*pgtype.UUID) = pgtype.UUID{}
			*dest[12].(*pgtype.UUID) = pgtype.UUID{}
			*dest[13].(*pgtype.UUID) = pgtype.UUID{}
			*dest[14].(*bool) = false
			*dest[15].(*int32) = 0
			*dest[16].(*string) = ""
			*dest[17].(*bool) = false
			*dest[18].(*int32) = 0
			*dest[19].(*int32) = 0
			*dest[20].(*pgtype.UUID) = pgtype.UUID{}
			*dest[21].(*[]byte) = []byte(`[]`)
			*dest[22].(*[]byte) = []byte(`{}`)
			*dest[23].(*pgtype.Timestamptz) = pgtype.Timestamptz{}
			*dest[24].(*pgtype.Timestamptz) = pgtype.Timestamptz{}
			return nil
		},
	}
}
