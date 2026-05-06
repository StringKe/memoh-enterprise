package integrations

import (
	"context"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/structpb"

	integrationv1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/integration/v1"
)

func TestWebSocketHandlerServesHTTPWithoutEcho(t *testing.T) {
	t.Parallel()

	handler := NewGatewayWebSocketHandler(nil, NewLocalGatewayBackend(NewService(nil, nil), NewHub()))
	req := httptest.NewRequest(http.MethodPost, WebSocketPath, strings.NewReader(""))
	req.Header.Set("Content-Type", "text/plain")
	recorder := httptest.NewRecorder()
	handler.HTTPHandler().ServeHTTP(recorder, req)
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", recorder.Code)
	}
}

func TestGatewayClientUsesConnectRPCForTokenSessionAndEvents(t *testing.T) {
	t.Parallel()

	var validatedHeader string
	mux := http.NewServeMux()
	mux.Handle(IntegrationGatewayValidateTokenProcedure, connect.NewUnaryHandlerSimple(IntegrationGatewayValidateTokenProcedure, func(ctx context.Context, req *structpb.Struct) (*structpb.Struct, error) {
		if callInfo, ok := connect.CallInfoForHandlerContext(ctx); ok {
			validatedHeader = callInfo.RequestHeader().Get("Authorization")
		}
		if got := stringStructField(req, "token"); got != "memoh_it_test" {
			t.Fatalf("token = %q, want memoh_it_test", got)
		}
		return structpb.NewStruct(map[string]any{
			"token": map[string]any{
				"id":                   "token-1",
				"name":                 "integration",
				"scope_type":           ScopeGlobal,
				"allowed_event_types":  []any{"message.created"},
				"allowed_action_types": []any{"send_message"},
				"created_at":           time.Now().UTC().Format(time.RFC3339Nano),
				"updated_at":           time.Now().UTC().Format(time.RFC3339Nano),
			},
		})
	}))
	mux.Handle(IntegrationGatewayCreateSessionProcedure, connect.NewUnaryHandlerSimple(IntegrationGatewayCreateSessionProcedure, func(_ context.Context, req *structpb.Struct) (*structpb.Struct, error) {
		if got := stringStructField(req, "token_id"); got != "token-1" {
			t.Fatalf("token_id = %q, want token-1", got)
		}
		return structpb.NewStruct(map[string]any{
			"session": map[string]any{
				"session_id":          "session-1",
				"bot_id":              "bot-1",
				"external_session_id": "external-1",
				"metadata":            map[string]any{"source": "test"},
				"created_at":          time.Now().UTC().Format(time.RFC3339Nano),
			},
		})
	}))
	mux.Handle(IntegrationGatewayPublishEventProcedure, connect.NewUnaryHandlerSimple(IntegrationGatewayPublishEventProcedure, func(_ context.Context, req *structpb.Struct) (*structpb.Struct, error) {
		if !strings.Contains(stringStructField(req, "event_json"), "event-1") {
			t.Fatal("publish event payload missing event id")
		}
		return structpb.NewStruct(map[string]any{})
	}))
	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewGatewayClient(GatewayClientOptions{
		BaseURL:      server.URL,
		ServiceToken: "service-token",
	})
	identity, err := client.ValidateToken(context.Background(), "memoh_it_test")
	if err != nil {
		t.Fatal(err)
	}
	if identity.Token.ID != "token-1" || identity.Token.ScopeType != ScopeGlobal {
		t.Fatalf("identity = %#v", identity)
	}
	if validatedHeader != "Bearer service-token" {
		t.Fatalf("authorization header = %q, want bearer service token", validatedHeader)
	}
	session, err := client.CreateSession(context.Background(), identity, "bot-1", "external-1", map[string]string{"source": "test"})
	if err != nil {
		t.Fatal(err)
	}
	if session.ID != "session-1" || session.BotID != "bot-1" || session.Metadata["source"] != "test" {
		t.Fatalf("session = %#v", session)
	}
	if err := client.PublishEvent(context.Background(), &integrationv1.IntegrationEvent{EventId: "event-1", EventType: "message.created"}); err != nil {
		t.Fatal(err)
	}
}

func TestExternalIntegrationProtoDoesNotImportPrivateProto(t *testing.T) {
	t.Parallel()

	root := findRepoRoot(t)
	files, err := filepath.Glob(filepath.Join(root, "proto/memoh/integration/v1/*.proto"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("integration proto files not found")
	}
	for _, file := range files {
		rel, err := filepath.Rel(root, file)
		if err != nil {
			t.Fatal(err)
		}
		content, err := fs.ReadFile(os.DirFS(root), rel)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(content), "\"memoh/private/") {
			t.Fatalf("%s imports private proto", file)
		}
	}
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("repository root not found")
		}
		dir = parent
	}
}
