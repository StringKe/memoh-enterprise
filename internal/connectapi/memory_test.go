package connectapi

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"connectrpc.com/connect"
	"github.com/labstack/echo/v4"

	memprovider "github.com/memohai/memoh/internal/memory/adapters"
)

func TestMemoryItemToProtoMapsFields(t *testing.T) {
	t.Parallel()

	got := memoryItemToProto("bot-1", memprovider.MemoryItem{
		ID:        "memory-1",
		Memory:    "remember this",
		Score:     0.75,
		CreatedAt: "2026-05-05T01:02:03Z",
		UpdatedAt: "2026-05-05T01:03:03Z",
		Metadata:  map[string]any{"source": "manual"},
		BotID:     "bot-2",
	})

	if got.GetId() != "memory-1" {
		t.Fatalf("id = %q, want memory-1", got.GetId())
	}
	if got.GetBotId() != "bot-2" {
		t.Fatalf("bot id = %q, want item bot id", got.GetBotId())
	}
	if got.GetContent() != "remember this" {
		t.Fatalf("content = %q, want remember this", got.GetContent())
	}
	if got.GetScore() != 0.75 {
		t.Fatalf("score = %v, want 0.75", got.GetScore())
	}
	if got.GetMetadata().AsMap()["source"] != "manual" {
		t.Fatalf("metadata = %#v, want source=manual", got.GetMetadata().AsMap())
	}
	if got.GetCreatedAt().AsTime().UTC().Format("2006-01-02T15:04:05Z") != "2026-05-05T01:02:03Z" {
		t.Fatalf("created_at = %v", got.GetCreatedAt().AsTime())
	}
}

func TestMemoryStatusToProtoMapsErrorsAndMetadata(t *testing.T) {
	t.Parallel()

	got := memoryStatusToProto("bot-1", memprovider.MemoryStatusResponse{
		ProviderType:      "builtin",
		MemoryMode:        "sparse",
		CanManualSync:     true,
		MarkdownFileCount: 3,
		Encoder:           memprovider.HealthStatus{OK: true},
		Qdrant:            memprovider.HealthStatus{Error: "qdrant down"},
	})

	if got.GetReady() {
		t.Fatal("ready = true, want false when qdrant has error")
	}
	if got.GetMessage() != "qdrant down" {
		t.Fatalf("message = %q, want qdrant down", got.GetMessage())
	}
	metadata := got.GetMetadata().AsMap()
	if metadata["memory_mode"] != "sparse" {
		t.Fatalf("metadata memory_mode = %#v, want sparse", metadata["memory_mode"])
	}
	if metadata["markdown_file_count"] != float64(3) {
		t.Fatalf("metadata markdown_file_count = %#v, want 3", metadata["markdown_file_count"])
	}
}

func TestBuildConnectMemoryFiltersMatchesRESTSharedNamespace(t *testing.T) {
	t.Parallel()

	got := buildConnectMemoryFilters("bot-1")
	if got["namespace"] != "bot" {
		t.Fatalf("namespace = %#v, want bot", got["namespace"])
	}
	if got["scopeId"] != "bot-1" {
		t.Fatalf("scopeId = %#v, want bot-1", got["scopeId"])
	}
}

func TestEchoToConnectErrorMapping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		code connect.Code
	}{
		{name: "bad request", err: echo.NewHTTPError(http.StatusBadRequest, "bad"), code: connect.CodeInvalidArgument},
		{name: "forbidden", err: echo.NewHTTPError(http.StatusForbidden, "denied"), code: connect.CodePermissionDenied},
		{name: "not found", err: echo.NewHTTPError(http.StatusNotFound, "missing"), code: connect.CodeNotFound},
		{name: "unavailable", err: echo.NewHTTPError(http.StatusServiceUnavailable, "down"), code: connect.CodeUnavailable},
		{name: "conflict", err: echo.NewHTTPError(http.StatusConflict, "conflict"), code: connect.CodeFailedPrecondition},
		{name: "fallback", err: errors.New("boom"), code: connect.CodeInvalidArgument},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := echoToConnectError(tt.err)
			if connect.CodeOf(err) != tt.code {
				t.Fatalf("code = %v, want %v", connect.CodeOf(err), tt.code)
			}
		})
	}
}

func TestMemoryServiceRequiresBotIDBeforeAuth(t *testing.T) {
	t.Parallel()

	service := &MemoryService{}
	_, err := service.requireBotAccess(context.Background(), " ")
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("code = %v, want %v", connect.CodeOf(err), connect.CodeInvalidArgument)
	}
}
