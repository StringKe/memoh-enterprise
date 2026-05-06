package connectapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"

	privatev1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1"
	"github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1/privatev1connect"
	"github.com/memohai/memoh/internal/conversation"
)

type fakeChatStreamer struct {
	ctxSeen  chan context.Context
	chunkOut chan conversation.StreamChunk
	errOut   chan error
	reqSeen  conversation.ChatRequest
}

func (f *fakeChatStreamer) StreamChat(ctx context.Context, req conversation.ChatRequest) (<-chan conversation.StreamChunk, <-chan error) {
	f.reqSeen = req
	f.ctxSeen <- ctx
	return f.chunkOut, f.errOut
}

func TestChatServiceStreamsChunks(t *testing.T) {
	t.Parallel()

	fake := &fakeChatStreamer{
		ctxSeen:  make(chan context.Context, 1),
		chunkOut: make(chan conversation.StreamChunk, 1),
		errOut:   make(chan error),
	}
	fake.chunkOut <- mustJSONChunk(t, map[string]any{"id": "evt-1", "type": "text_delta", "delta": "hello"})
	close(fake.chunkOut)
	close(fake.errOut)

	client, cleanup := newChatServiceTestClient(t, &ChatService{runner: fake})
	defer cleanup()

	stream, err := client.StreamChat(context.Background(), connect.NewRequest(&privatev1.StreamChatRequest{
		BotId:         "bot-1",
		SessionId:     "session-1",
		Message:       "hello",
		AttachmentIds: []string{"asset-1"},
	}))
	if err != nil {
		t.Fatalf("StreamChat() error = %v", err)
	}
	if !stream.Receive() {
		t.Fatal("expected first stream message")
	}
	msg := stream.Msg()
	if msg.GetType() != "text_delta" || msg.GetText() != "hello" {
		t.Fatalf("unexpected stream msg: %#v", msg)
	}
	if stream.Receive() {
		t.Fatal("expected stream to end")
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream error = %v", err)
	}
	if fake.reqSeen.BotID != "bot-1" || fake.reqSeen.SessionID != "session-1" {
		t.Fatalf("unexpected request: %#v", fake.reqSeen)
	}
	if len(fake.reqSeen.Attachments) != 1 || fake.reqSeen.Attachments[0].ContentHash != "asset-1" {
		t.Fatalf("attachments = %#v, want asset content hash", fake.reqSeen.Attachments)
	}
}

func TestChatServiceStreamCancellationReachesResolver(t *testing.T) {
	t.Parallel()

	fake := &fakeChatStreamer{
		ctxSeen:  make(chan context.Context, 1),
		chunkOut: make(chan conversation.StreamChunk, 1),
		errOut:   make(chan error),
	}
	fake.chunkOut <- mustJSONChunk(t, map[string]any{"id": "evt-1", "type": "text_delta", "delta": "hello"})
	client, cleanup := newChatServiceTestClient(t, &ChatService{runner: fake})
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	stream, err := client.StreamChat(ctx, connect.NewRequest(&privatev1.StreamChatRequest{
		BotId:   "bot-1",
		Message: "hello",
	}))
	if err != nil {
		t.Fatalf("StreamChat() error = %v", err)
	}
	if !stream.Receive() {
		t.Fatal("expected first stream message")
	}
	resolverCtx := <-fake.ctxSeen
	cancel()
	select {
	case <-resolverCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("resolver context was not cancelled")
	}
	close(fake.chunkOut)
	close(fake.errOut)
}

func newChatServiceTestClient(t *testing.T, service *ChatService) (privatev1connect.ChatServiceClient, func()) {
	t.Helper()
	_, handler := privatev1connect.NewChatServiceHandler(service)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(WithUserID(r.Context(), "user-1"))
		handler.ServeHTTP(w, r)
	}))
	return privatev1connect.NewChatServiceClient(server.Client(), server.URL), server.Close
}

func mustJSONChunk(t *testing.T, payload map[string]any) conversation.StreamChunk {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
