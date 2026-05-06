package connectapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"

	"github.com/memohai/memoh/internal/compaction"
	privatev1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1"
	"github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1/privatev1connect"
	dbsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
	"github.com/memohai/memoh/internal/eventbus"
	"github.com/memohai/memoh/internal/iam/rbac"
	messagepkg "github.com/memohai/memoh/internal/message"
)

func TestBotSessionRuntimeHistoryCompactAndListMessages(t *testing.T) {
	now := time.Date(2026, 5, 6, 10, 0, 0, 0, time.UTC)
	messages := fakeSessionMessageService{items: []messagepkg.Message{
		{ID: "message-1", BotID: "bot-1", SessionID: "session-1", Role: "user", Content: json.RawMessage(`{"text":"hello"}`), CreatedAt: now.Add(-time.Minute)},
		{ID: "message-2", BotID: "bot-1", SessionID: "session-1", Role: "assistant", DisplayContent: "world", CreatedAt: now},
		{ID: "message-3", BotID: "bot-2", SessionID: "session-1", Role: "user", DisplayContent: "other", CreatedAt: now},
	}}
	publisher := &fakeBotEventPublisher{}
	service := &BotService{
		permissions:        allowBotPermission{},
		messages:           messages,
		events:             publisher,
		workerConsumerName: "worker",
		now:                func() time.Time { return now },
	}
	client, cleanup := newBotSessionRuntimeTestClient(t, service)
	defer cleanup()

	history, err := client.ReadBotSessionHistory(context.Background(), connect.NewRequest(&privatev1.ReadBotSessionHistoryRequest{
		BotId:     "bot-1",
		SessionId: "session-1",
		Limit:     1,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if got := history.Msg.GetMessages(); len(got) != 1 || got[0].GetId() != "message-2" {
		t.Fatalf("history messages = %#v", got)
	}

	list, err := client.ListBotSessionMessages(context.Background(), connect.NewRequest(&privatev1.ListBotSessionMessagesRequest{
		BotId:     "bot-1",
		SessionId: "session-1",
		Page:      &privatev1.PageRequest{PageSize: 1},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if got := list.Msg.GetMessages(); len(got) != 1 || got[0].GetId() != "message-1" {
		t.Fatalf("list messages = %#v", got)
	}
	if list.Msg.GetPage().GetNextPageToken() != "1" {
		t.Fatalf("next page token = %q", list.Msg.GetPage().GetNextPageToken())
	}

	compact, err := client.CompactBotSession(context.Background(), connect.NewRequest(&privatev1.CompactBotSessionRequest{
		BotId:     "bot-1",
		SessionId: "session-1",
		Reason:    "manual",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if compact.Msg.GetSessionId() != "session-1" || compact.Msg.GetSummary() != "queued" {
		t.Fatalf("compact response = %#v", compact.Msg)
	}
	if publisher.event.Topic != botSessionCompactionTopic {
		t.Fatalf("topic = %q", publisher.event.Topic)
	}
	if got := publisher.consumers; len(got) != 1 || got[0] != "worker" {
		t.Fatalf("consumers = %#v", got)
	}
	var cfg compaction.TriggerConfig
	if err := json.Unmarshal(publisher.event.PayloadJSON, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.BotID != "bot-1" || cfg.SessionID != "session-1" {
		t.Fatalf("compaction payload = %#v", cfg)
	}
}

func newBotSessionRuntimeTestClient(t *testing.T, service *BotService) (privatev1connect.BotServiceClient, func()) {
	t.Helper()
	_, handler := privatev1connect.NewBotServiceHandler(service)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(WithUserID(r.Context(), "user-1"))
		handler.ServeHTTP(w, r)
	}))
	return privatev1connect.NewBotServiceClient(server.Client(), server.URL), server.Close
}

type allowBotPermission struct{}

func (allowBotPermission) HasBotPermission(context.Context, string, string, rbac.PermissionKey) (bool, error) {
	return true, nil
}

type fakeBotEventPublisher struct {
	event     eventbus.Event
	consumers []string
}

func (f *fakeBotEventPublisher) Publish(_ context.Context, event eventbus.Event, consumers []string) (dbsqlc.EventOutbox, error) {
	f.event = event
	f.consumers = append([]string(nil), consumers...)
	return dbsqlc.EventOutbox{}, nil
}

type fakeSessionMessageService struct {
	items []messagepkg.Message
}

func (fakeSessionMessageService) Persist(context.Context, messagepkg.PersistInput) (messagepkg.Message, error) {
	return messagepkg.Message{}, nil
}

func (f fakeSessionMessageService) List(context.Context, string) ([]messagepkg.Message, error) {
	return f.items, nil
}

func (f fakeSessionMessageService) ListSince(context.Context, string, time.Time) ([]messagepkg.Message, error) {
	return f.items, nil
}

func (f fakeSessionMessageService) ListActiveSince(context.Context, string, time.Time) ([]messagepkg.Message, error) {
	return f.items, nil
}

func (f fakeSessionMessageService) ListLatest(context.Context, string, int32) ([]messagepkg.Message, error) {
	return f.items, nil
}

func (f fakeSessionMessageService) ListBefore(context.Context, string, time.Time, int32) ([]messagepkg.Message, error) {
	return f.items, nil
}

func (f fakeSessionMessageService) ListBySession(context.Context, string) ([]messagepkg.Message, error) {
	return f.items, nil
}

func (f fakeSessionMessageService) ListSinceBySession(context.Context, string, time.Time) ([]messagepkg.Message, error) {
	return f.items, nil
}

func (f fakeSessionMessageService) ListActiveSinceBySession(context.Context, string, time.Time) ([]messagepkg.Message, error) {
	return f.items, nil
}

func (f fakeSessionMessageService) ListLatestBySession(context.Context, string, int32) ([]messagepkg.Message, error) {
	return f.items, nil
}

func (f fakeSessionMessageService) ListBeforeBySession(context.Context, string, time.Time, int32) ([]messagepkg.Message, error) {
	return f.items, nil
}

func (fakeSessionMessageService) DeleteByBot(context.Context, string) error {
	return nil
}

func (fakeSessionMessageService) DeleteBySession(context.Context, string) error {
	return nil
}

func (fakeSessionMessageService) LinkAssets(context.Context, string, []messagepkg.AssetRef) error {
	return nil
}
