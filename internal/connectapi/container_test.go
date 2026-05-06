package connectapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"

	"github.com/memohai/memoh/internal/bots"
	privatev1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1"
	"github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1/privatev1connect"
	"github.com/memohai/memoh/internal/handlers"
)

type fakeContainerCreator struct {
	ctxSeen chan context.Context
	reqSeen handlers.CreateContainerRequest
}

func (f *fakeContainerCreator) CreateContainerStream(ctx context.Context, _ string, req handlers.CreateContainerRequest, send func(any)) error {
	f.reqSeen = req
	f.ctxSeen <- ctx
	send(map[string]any{"type": "creating"})
	<-ctx.Done()
	return ctx.Err()
}

type fakeContainerAuthorizer struct{}

func (fakeContainerAuthorizer) AuthorizeAccess(context.Context, string, string, bool) (bots.Bot, error) {
	return bots.Bot{ID: "bot-1"}, nil
}

func TestContainerServiceStreamDeadlineReachesCreator(t *testing.T) {
	t.Parallel()

	creator := &fakeContainerCreator{ctxSeen: make(chan context.Context, 1)}
	client, cleanup := newContainerServiceTestClient(t, &ContainerService{
		creator: creator,
		bots:    fakeContainerAuthorizer{},
	})
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	stream, err := client.StreamContainerProgress(ctx, connect.NewRequest(&privatev1.StreamContainerProgressRequest{
		BotId: "bot-1",
	}))
	if err != nil {
		t.Fatalf("StreamContainerProgress() error = %v", err)
	}
	if !stream.Receive() {
		t.Fatal("expected initial container progress event")
	}
	if stream.Msg().GetType() != "creating" {
		t.Fatalf("event type = %q, want creating", stream.Msg().GetType())
	}
	creatorCtx := <-creator.ctxSeen
	if _, ok := creatorCtx.Deadline(); !ok {
		t.Fatal("creator context has no deadline")
	}
	cancel()
	select {
	case <-creatorCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("creator context did not observe cancellation")
	}
}

func TestContainerStreamRequestMapsOptions(t *testing.T) {
	t.Parallel()

	req := containerStreamRequest(&privatev1.StreamContainerProgressRequest{
		Options: mustStruct(t, map[string]any{
			"image":                "ubuntu:24.04",
			"snapshotter":          "overlayfs",
			"workspace_backend":    "local",
			"local_workspace_path": "/tmp/memoh",
			"restore_data":         true,
			"gpu": map[string]any{
				"devices": []any{"nvidia.com/gpu=all"},
			},
		}),
	})
	if req.Image != "ubuntu:24.04" || req.Snapshotter != "overlayfs" {
		t.Fatalf("unexpected image/snapshotter: %#v", req)
	}
	if req.WorkspaceBackend != "local" || req.LocalWorkspacePath != "/tmp/memoh" || !req.RestoreData {
		t.Fatalf("unexpected workspace options: %#v", req)
	}
	if req.GPU == nil || len(req.GPU.Devices) != 1 || req.GPU.Devices[0] != "nvidia.com/gpu=all" {
		t.Fatalf("unexpected GPU options: %#v", req.GPU)
	}
}

func newContainerServiceTestClient(t *testing.T, service *ContainerService) (privatev1connect.ContainerServiceClient, func()) {
	t.Helper()
	_, handler := privatev1connect.NewContainerServiceHandler(service)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(WithUserID(r.Context(), "user-1"))
		handler.ServeHTTP(w, r)
	}))
	return privatev1connect.NewContainerServiceClient(server.Client(), server.URL), server.Close
}
