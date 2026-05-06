package executorclient

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	workspacev1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/workspace/v1"
	"github.com/memohai/memoh/internal/connectapi/gen/memoh/workspace/v1/workspacev1connect"
)

type rawReadTestServer struct {
	workspacev1connect.UnimplementedWorkspaceExecutorServiceHandler
	files map[string][]byte
}

func (s *rawReadTestServer) ReadRaw(_ context.Context, req *connect.Request[workspacev1.ReadRawRequest], stream *connect.ServerStream[workspacev1.ReadRawResponse]) error {
	data, ok := s.files[req.Msg.GetPath()]
	if !ok {
		return connect.NewError(connect.CodeNotFound, errors.New("missing file"))
	}
	if len(data) == 0 {
		return nil
	}
	if err := stream.Send(&workspacev1.ReadRawResponse{Chunk: &workspacev1.DataChunk{Data: data[:1]}}); err != nil {
		return err
	}
	if len(data) > 1 {
		if err := stream.Send(&workspacev1.ReadRawResponse{Chunk: &workspacev1.DataChunk{Data: data[1:]}}); err != nil {
			return err
		}
	}
	return nil
}

func newTestReadRawClient(t *testing.T, files map[string][]byte) *Client {
	t.Helper()

	mux := http.NewServeMux()
	path, handler := workspacev1connect.NewWorkspaceExecutorServiceHandler(&rawReadTestServer{files: files})
	mux.Handle(path, handler)
	server := httptest.NewServer(h2c.NewHandler(mux, &http2.Server{}))
	t.Cleanup(server.Close)

	return NewClient(workspacev1connect.NewWorkspaceExecutorServiceClient(server.Client(), server.URL), nil)
}

func TestClientReadRawMissingFileReturnsNotFoundImmediately(t *testing.T) {
	t.Parallel()

	client := newTestReadRawClient(t, map[string][]byte{})
	_, err := client.ReadRaw(context.Background(), "/data/media/missing.jpg")
	if err == nil {
		t.Fatal("expected read raw to fail for missing file")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestClientReadRawPreservesFirstChunk(t *testing.T) {
	t.Parallel()

	client := newTestReadRawClient(t, map[string][]byte{
		"/data/media/existing.jpg": []byte("hello"),
	})
	reader, err := client.ReadRaw(context.Background(), "/data/media/existing.jpg")
	if err != nil {
		t.Fatalf("ReadRaw returned error: %v", err)
	}
	defer func() { _ = reader.Close() }()

	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read raw reader failed: %v", err)
	}
	if got := string(data); got != "hello" {
		t.Fatalf("expected full payload, got %q", got)
	}
}

func TestClientReadRawSupportsEmptyFile(t *testing.T) {
	t.Parallel()

	client := newTestReadRawClient(t, map[string][]byte{
		"/data/media/empty.txt": {},
	})
	reader, err := client.ReadRaw(context.Background(), "/data/media/empty.txt")
	if err != nil {
		t.Fatalf("ReadRaw returned error: %v", err)
	}
	defer func() { _ = reader.Close() }()

	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read raw empty reader failed: %v", err)
	}
	if len(data) != 0 {
		t.Fatalf("expected empty payload, got %q", string(data))
	}
}
