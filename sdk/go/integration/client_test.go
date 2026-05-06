package integration

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/encoding/protojson"

	integrationv1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/integration/v1"
)

func TestClientConnectSubscribeAckAndPing(t *testing.T) {
	server := newTestServer(t, func(t *testing.T, conn *websocket.Conn) {
		envelope := readTestEnvelope(t, conn)
		if envelope.GetAuthRequest() == nil {
			t.Fatalf("expected auth request, got %T", envelope.GetPayload())
		}
		writeTestEnvelope(t, conn, &integrationv1.Envelope{
			Version:   defaultProtocolVersion,
			MessageId: "server-auth",
			Payload: &integrationv1.Envelope_AuthResponse{AuthResponse: &integrationv1.AuthResponse{
				IntegrationId: "token-1",
				ScopeType:     "global",
			}},
		})

		subscribe := readTestEnvelope(t, conn)
		if subscribe.GetSubscribeRequest() == nil {
			t.Fatalf("expected subscribe request, got %T", subscribe.GetPayload())
		}
		writeTestEnvelope(t, conn, &integrationv1.Envelope{
			CorrelationId: subscribe.GetCorrelationId(),
			Payload: &integrationv1.Envelope_SubscribeResponse{SubscribeResponse: &integrationv1.SubscribeResponse{
				EventTypes: []string{"message"},
			}},
		})

		ack := readTestEnvelope(t, conn)
		writeTestEnvelope(t, conn, &integrationv1.Envelope{
			CorrelationId: ack.GetCorrelationId(),
			Payload: &integrationv1.Envelope_AckResponse{AckResponse: &integrationv1.AckResponse{
				EventId: ack.GetAckRequest().GetEventId(),
			}},
		})

		ping := readTestEnvelope(t, conn)
		writeTestEnvelope(t, conn, &integrationv1.Envelope{
			CorrelationId: ping.GetCorrelationId(),
			Payload:       &integrationv1.Envelope_Pong{Pong: &integrationv1.Pong{}},
		})
	})
	defer server.Close()

	client := New(Options{
		URL:            wsURL(server.URL),
		Token:          "memoh_it_test",
		RequestTimeout: time.Second,
		IDFactory:      stableIDFactory(),
	})
	ctx := context.Background()
	info, err := client.Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if info.IntegrationID != "token-1" || info.ScopeType != "global" {
		t.Fatalf("unexpected connection info: %+v", info)
	}
	subscribe, err := client.Subscribe(ctx, SubscribeOptions{EventTypes: []string{"message"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(subscribe.GetEventTypes(), ","); got != "message" {
		t.Fatalf("unexpected event types: %s", got)
	}
	if err := client.AckEvent(ctx, "event-1"); err != nil {
		t.Fatal(err)
	}
	if err := client.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestClientSendsAuthHeadersAndClientCredentials(t *testing.T) {
	server := newRequestTestServer(t, func(t *testing.T, r *http.Request, conn *websocket.Conn) {
		if got := r.Header.Get("Authorization"); got != "Bearer memoh_it_test" {
			t.Fatalf("authorization header = %q, want bearer token", got)
		}
		if got := r.Header.Get("X-Memoh-Client-ID"); got != "client-1" {
			t.Fatalf("client id header = %q, want client-1", got)
		}
		if got := r.Header.Get("X-Memoh-Client-Secret"); got != "secret-1" {
			t.Fatalf("client secret header = %q, want secret-1", got)
		}
		if got := r.Header.Get("X-Extra"); got != "value" {
			t.Fatalf("extra header = %q, want value", got)
		}
		envelope := readTestEnvelope(t, conn)
		if envelope.GetAuthRequest().GetToken() != "memoh_it_test" {
			t.Fatalf("auth token = %q, want memoh_it_test", envelope.GetAuthRequest().GetToken())
		}
		writeTestEnvelope(t, conn, &integrationv1.Envelope{
			Payload: &integrationv1.Envelope_AuthResponse{AuthResponse: &integrationv1.AuthResponse{}},
		})
	})
	defer server.Close()

	client := New(Options{
		URL:              wsURL(server.URL),
		Token:            "memoh_it_test",
		ClientID:         "client-1",
		ClientCredential: "secret-1",
		Header:           http.Header{"X-Extra": []string{"value"}},
	})
	if _, err := client.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestClientHeartbeatSendsPing(t *testing.T) {
	heartbeat := make(chan string, 1)
	server := newTestServer(t, func(t *testing.T, conn *websocket.Conn) {
		_ = readTestEnvelope(t, conn)
		writeTestEnvelope(t, conn, &integrationv1.Envelope{
			Payload: &integrationv1.Envelope_AuthResponse{AuthResponse: &integrationv1.AuthResponse{}},
		})
		ping := readTestEnvelope(t, conn)
		if ping.GetPing() == nil {
			t.Fatalf("expected heartbeat ping, got %T", ping.GetPayload())
		}
		heartbeat <- ping.GetCorrelationId()
		writeTestEnvelope(t, conn, &integrationv1.Envelope{
			CorrelationId: ping.GetCorrelationId(),
			Payload:       &integrationv1.Envelope_Pong{Pong: &integrationv1.Pong{}},
		})
	})
	defer server.Close()

	client := New(Options{
		URL:               wsURL(server.URL),
		Token:             "memoh_it_test",
		RequestTimeout:    time.Second,
		HeartbeatInterval: time.Millisecond,
		IDFactory:         stableIDFactory(),
	})
	if _, err := client.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case correlationID := <-heartbeat:
		if correlationID == "" {
			t.Fatal("heartbeat ping correlation id is empty")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for heartbeat ping")
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestClientRejectsProtocolError(t *testing.T) {
	server := newTestServer(t, func(t *testing.T, conn *websocket.Conn) {
		_ = readTestEnvelope(t, conn)
		writeTestEnvelope(t, conn, &integrationv1.Envelope{
			Payload: &integrationv1.Envelope_AuthResponse{AuthResponse: &integrationv1.AuthResponse{}},
		})
		action := readTestEnvelope(t, conn)
		writeTestEnvelope(t, conn, &integrationv1.Envelope{
			CorrelationId: action.GetCorrelationId(),
			Payload: &integrationv1.Envelope_Error{Error: &integrationv1.Error{
				Code:    "permission_denied",
				Message: "denied",
			}},
		})
	})
	defer server.Close()

	client := New(Options{
		URL:            wsURL(server.URL),
		Token:          "memoh_it_test",
		RequestTimeout: time.Second,
		IDFactory:      stableIDFactory(),
	})
	if _, err := client.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, err := client.RequestAction(context.Background(), RequestActionOptions{BotID: "bot-1", ActionType: "run_task"})
	if err == nil {
		t.Fatal("expected request action error")
	}
	var protocolErr *ProtocolError
	if !errors.As(err, &protocolErr) {
		t.Fatalf("expected ProtocolError, got %T", err)
	}
	if protocolErr.Code != "permission_denied" {
		t.Fatalf("unexpected error code: %s", protocolErr.Code)
	}
}

func TestClientSendsBotMessagesAndManagesSessions(t *testing.T) {
	server := newTestServer(t, func(t *testing.T, conn *websocket.Conn) {
		_ = readTestEnvelope(t, conn)
		writeTestEnvelope(t, conn, &integrationv1.Envelope{
			Payload: &integrationv1.Envelope_AuthResponse{AuthResponse: &integrationv1.AuthResponse{}},
		})

		message := readTestEnvelope(t, conn)
		if message.GetSendBotMessageRequest() == nil {
			t.Fatalf("expected send bot message request, got %T", message.GetPayload())
		}
		writeTestEnvelope(t, conn, &integrationv1.Envelope{
			CorrelationId: message.GetCorrelationId(),
			Payload: &integrationv1.Envelope_SendBotMessageResponse{SendBotMessageResponse: &integrationv1.SendBotMessageResponse{
				MessageId: "message-1",
				SessionId: "session-1",
			}},
		})

		createSession := readTestEnvelope(t, conn)
		if createSession.GetCreateSessionRequest() == nil {
			t.Fatalf("expected create session request, got %T", createSession.GetPayload())
		}
		writeTestEnvelope(t, conn, &integrationv1.Envelope{
			CorrelationId: createSession.GetCorrelationId(),
			Payload: &integrationv1.Envelope_CreateSessionResponse{CreateSessionResponse: &integrationv1.CreateSessionResponse{
				BotId:     "bot-1",
				SessionId: "session-2",
			}},
		})

		status := readTestEnvelope(t, conn)
		if status.GetGetSessionStatusRequest() == nil {
			t.Fatalf("expected get session status request, got %T", status.GetPayload())
		}
		writeTestEnvelope(t, conn, &integrationv1.Envelope{
			CorrelationId: status.GetCorrelationId(),
			Payload: &integrationv1.Envelope_GetSessionStatusResponse{GetSessionStatusResponse: &integrationv1.GetSessionStatusResponse{
				BotId:     "bot-1",
				SessionId: "session-2",
				Status:    "active",
			}},
		})

		botStatus := readTestEnvelope(t, conn)
		if botStatus.GetGetBotStatusRequest() == nil {
			t.Fatalf("expected get bot status request, got %T", botStatus.GetPayload())
		}
		writeTestEnvelope(t, conn, &integrationv1.Envelope{
			CorrelationId: botStatus.GetCorrelationId(),
			Payload: &integrationv1.Envelope_GetBotStatusResponse{GetBotStatusResponse: &integrationv1.GetBotStatusResponse{
				BotId:  "bot-1",
				Status: "available",
			}},
		})

		action := readTestEnvelope(t, conn)
		if action.GetRequestActionRequest() == nil {
			t.Fatalf("expected request action request, got %T", action.GetPayload())
		}
		writeTestEnvelope(t, conn, &integrationv1.Envelope{
			CorrelationId: action.GetCorrelationId(),
			Payload: &integrationv1.Envelope_RequestActionResponse{RequestActionResponse: &integrationv1.RequestActionResponse{
				ActionId:   "action-1",
				BotId:      "bot-1",
				ActionType: "run_task",
				Status:     "accepted",
			}},
		})
	})
	defer server.Close()

	client := New(Options{
		URL:            wsURL(server.URL),
		Token:          "memoh_it_test",
		RequestTimeout: time.Second,
		IDFactory:      stableIDFactory(),
	})
	if _, err := client.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	message, err := client.SendBotMessage(context.Background(), SendBotMessageOptions{
		BotID:     "bot-1",
		SessionID: "session-1",
		Text:      "hello",
		Metadata:  map[string]string{"source": "test"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if message.GetMessageId() != "message-1" {
		t.Fatalf("unexpected message id: %s", message.GetMessageId())
	}
	session, err := client.CreateSession(context.Background(), CreateSessionOptions{
		BotID:             "bot-1",
		ExternalSessionID: "external-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if session.GetSessionId() != "session-2" {
		t.Fatalf("unexpected session id: %s", session.GetSessionId())
	}
	status, err := client.GetSessionStatus(context.Background(), "session-2")
	if err != nil {
		t.Fatal(err)
	}
	if status.GetStatus() != "active" {
		t.Fatalf("unexpected status: %s", status.GetStatus())
	}
	botStatus, err := client.GetBotStatus(context.Background(), "bot-1")
	if err != nil {
		t.Fatal(err)
	}
	if botStatus.GetStatus() != "available" {
		t.Fatalf("unexpected bot status: %s", botStatus.GetStatus())
	}
	action, err := client.RequestAction(context.Background(), RequestActionOptions{
		BotID:       "bot-1",
		ActionType:  "run_task",
		PayloadJSON: `{"task":"sync"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if action.GetActionId() != "action-1" {
		t.Fatalf("unexpected action id: %s", action.GetActionId())
	}
}

func TestClientEventsReceivesUnsolicitedEnvelope(t *testing.T) {
	server := newTestServer(t, func(t *testing.T, conn *websocket.Conn) {
		_ = readTestEnvelope(t, conn)
		writeTestEnvelope(t, conn, &integrationv1.Envelope{
			Payload: &integrationv1.Envelope_AuthResponse{AuthResponse: &integrationv1.AuthResponse{}},
		})
		writeTestEnvelope(t, conn, &integrationv1.Envelope{
			MessageId: "server-event-1",
			Payload: &integrationv1.Envelope_Error{Error: &integrationv1.Error{
				Code:    "event",
				Message: "queued event",
			}},
		})
	})
	defer server.Close()

	client := New(Options{
		URL:            wsURL(server.URL),
		Token:          "memoh_it_test",
		RequestTimeout: time.Second,
	})
	if _, err := client.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case event := <-client.Events():
		if event.GetMessageId() != "server-event-1" {
			t.Fatalf("unexpected event id: %s", event.GetMessageId())
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestClientDeduplicatesEventsByMessageID(t *testing.T) {
	server := newTestServer(t, func(t *testing.T, conn *websocket.Conn) {
		_ = readTestEnvelope(t, conn)
		writeTestEnvelope(t, conn, &integrationv1.Envelope{
			Payload: &integrationv1.Envelope_AuthResponse{AuthResponse: &integrationv1.AuthResponse{}},
		})
		event := &integrationv1.Envelope{
			MessageId: "server-event-1",
			Payload: &integrationv1.Envelope_Error{Error: &integrationv1.Error{
				Code:    "event",
				Message: "queued event",
			}},
		}
		writeTestEnvelope(t, conn, event)
		writeTestEnvelope(t, conn, event)
	})
	defer server.Close()

	client := New(Options{
		URL:            wsURL(server.URL),
		Token:          "memoh_it_test",
		RequestTimeout: time.Second,
	})
	if _, err := client.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case event := <-client.Events():
		if event.GetMessageId() != "server-event-1" {
			t.Fatalf("unexpected event id: %s", event.GetMessageId())
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
	select {
	case event := <-client.Events():
		t.Fatalf("unexpected duplicate event: %s", event.GetMessageId())
	case <-time.After(50 * time.Millisecond):
	}
}

func TestClientReconnectRetriesAfterFailedHandshake(t *testing.T) {
	var attempts int32
	server := newTestServer(t, func(t *testing.T, conn *websocket.Conn) {
		attempt := atomic.AddInt32(&attempts, 1)
		_ = readTestEnvelope(t, conn)
		if attempt == 1 {
			_ = conn.Close()
			return
		}
		writeTestEnvelope(t, conn, &integrationv1.Envelope{
			Payload: &integrationv1.Envelope_AuthResponse{AuthResponse: &integrationv1.AuthResponse{
				IntegrationId: "token-2",
				ScopeType:     "global",
			}},
		})
	})
	defer server.Close()

	client := New(Options{
		URL:                  wsURL(server.URL),
		Token:                "memoh_it_test",
		RequestTimeout:       time.Second,
		ReconnectBackoff:     time.Millisecond,
		MaxReconnectAttempts: 1,
	})
	info, err := client.Reconnect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.IntegrationID != "token-2" {
		t.Fatalf("unexpected integration id: %s", info.IntegrationID)
	}
	if atomic.LoadInt32(&attempts) != 2 {
		t.Fatalf("unexpected attempt count: %d", attempts)
	}
}

func newTestServer(t *testing.T, handler func(*testing.T, *websocket.Conn)) *httptest.Server {
	t.Helper()
	return newRequestTestServer(t, func(t *testing.T, _ *http.Request, conn *websocket.Conn) {
		handler(t, conn)
	})
}

func newRequestTestServer(t *testing.T, handler func(*testing.T, *http.Request, *websocket.Conn)) *httptest.Server {
	t.Helper()
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer func() { _ = conn.Close() }()
		handler(t, r, conn)
	}))
}

func readTestEnvelope(t *testing.T, conn *websocket.Conn) *integrationv1.Envelope {
	t.Helper()
	_, payload, err := conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	var envelope integrationv1.Envelope
	if err := protojson.Unmarshal(payload, &envelope); err != nil {
		t.Fatal(err)
	}
	return &envelope
}

func writeTestEnvelope(t *testing.T, conn *websocket.Conn, envelope *integrationv1.Envelope) {
	t.Helper()
	payload, err := protojson.MarshalOptions{EmitUnpopulated: false}.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
		t.Fatal(err)
	}
}

func wsURL(url string) string {
	return "ws" + strings.TrimPrefix(url, "http")
}

func stableIDFactory() func() string {
	next := 0
	return func() string {
		next++
		return fmt.Sprintf("id-%d", next)
	}
}
