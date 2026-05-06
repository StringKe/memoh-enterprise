package tools

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	workspacev1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/workspace/v1"
	"github.com/memohai/memoh/internal/connectapi/gen/memoh/workspace/v1/workspacev1connect"
	"github.com/memohai/memoh/internal/workspace/executorclient"
)

type readMediaTestContainerService struct {
	workspacev1connect.UnimplementedWorkspaceExecutorServiceHandler
	files map[string][]byte
}

func (s *readMediaTestContainerService) ReadRaw(_ context.Context, req *connect.Request[workspacev1.ReadRawRequest], stream *connect.ServerStream[workspacev1.ReadRawResponse]) error {
	data, ok := s.files[req.Msg.GetPath()]
	if !ok {
		return connect.NewError(connect.CodeNotFound, errors.New("not found"))
	}
	if len(data) == 0 {
		return nil
	}
	return stream.Send(&workspacev1.ReadRawResponse{Chunk: &workspacev1.DataChunk{Data: data}})
}

func newReadMediaTestClient(t *testing.T, files map[string][]byte) *executorclient.Client {
	t.Helper()

	mux := http.NewServeMux()
	path, handler := workspacev1connect.NewWorkspaceExecutorServiceHandler(&readMediaTestContainerService{files: files})
	mux.Handle(path, handler)
	server := httptest.NewServer(h2c.NewHandler(mux, &http2.Server{}))
	t.Cleanup(server.Close)
	client, err := executorclient.Dial(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("executorclient.Dial: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	return client
}

func TestReadImageFromContainerSuccess(t *testing.T) {
	t.Parallel()

	pngBytes := []byte("\x89PNG\r\n\x1a\npayload")
	client := newReadMediaTestClient(t, map[string][]byte{
		"/data/images/demo.png": pngBytes,
	})

	result := ReadImageFromContainer(context.Background(), client, "/data/images/demo.png", 0)

	if !result.Public.OK {
		t.Fatalf("expected success result, got %+v", result.Public)
	}
	if result.Public.Path != "/data/images/demo.png" {
		t.Fatalf("unexpected path: %q", result.Public.Path)
	}
	if result.Public.Mime != "image/png" {
		t.Fatalf("unexpected mime: %q", result.Public.Mime)
	}
	if result.Public.Size != len(pngBytes) {
		t.Fatalf("unexpected size: %d", result.Public.Size)
	}

	expectedBase64 := base64.StdEncoding.EncodeToString(pngBytes)
	if result.ImageBase64 != expectedBase64 {
		t.Fatalf("unexpected image payload: %q", result.ImageBase64)
	}
	if result.ImageMediaType != "image/png" {
		t.Fatalf("unexpected image media type: %q", result.ImageMediaType)
	}
}

func TestReadImageFromContainerRejectsUnsupportedMime(t *testing.T) {
	t.Parallel()

	svgBytes := []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`)
	client := newReadMediaTestClient(t, map[string][]byte{
		"/data/images/demo.svg": svgBytes,
	})

	result := ReadImageFromContainer(context.Background(), client, "/data/images/demo.svg", 0)

	if result.Public.OK {
		t.Fatalf("expected error result, got %+v", result.Public)
	}
	if !strings.Contains(result.Public.Error, "PNG, JPEG, GIF, or WebP") {
		t.Fatalf("unexpected error: %q", result.Public.Error)
	}
	if result.ImageBase64 != "" {
		t.Fatalf("expected no injected image for error result, got %q", result.ImageBase64)
	}
}

func TestReadImageFromContainerRejectsCorruptedBytes(t *testing.T) {
	t.Parallel()

	client := newReadMediaTestClient(t, map[string][]byte{
		"/data/images/demo.png": []byte("definitely not a png"),
	})

	result := ReadImageFromContainer(context.Background(), client, "/data/images/demo.png", 0)

	if result.Public.OK {
		t.Fatalf("expected error result, got %+v", result.Public)
	}
	if !strings.Contains(result.Public.Error, "PNG, JPEG, GIF, or WebP") {
		t.Fatalf("unexpected error: %q", result.Public.Error)
	}
	if result.ImageBase64 != "" {
		t.Fatalf("expected no injected image for error result, got %q", result.ImageBase64)
	}
}

func TestReadImageFromContainerNotFound(t *testing.T) {
	t.Parallel()

	client := newReadMediaTestClient(t, map[string][]byte{})

	result := ReadImageFromContainer(context.Background(), client, "/data/images/missing.png", 0)

	if result.Public.OK {
		t.Fatalf("expected error result, got %+v", result.Public)
	}
	if result.ImageBase64 != "" {
		t.Fatalf("expected no injected image for error result, got %q", result.ImageBase64)
	}
}
