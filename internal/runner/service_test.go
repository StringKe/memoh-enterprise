package runner

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"

	eventv1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/event/v1"
	runnerv1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/runner/v1"
	"github.com/memohai/memoh/internal/connectapi/gen/memoh/runner/v1/runnerv1connect"
)

func TestServiceStartCancelAndEventStream(t *testing.T) {
	ctx := context.Background()
	lease := testRunLease("run-service")
	svc := NewService(ServiceDeps{Clock: func() time.Time {
		return time.Date(2026, 5, 6, 10, 0, 0, 0, time.UTC)
	}})
	client, closeServer := newRunnerServiceTestClient(t, svc)
	defer closeServer()

	startResp, err := client.StartRun(ctx, connect.NewRequest(&runnerv1.StartRunRequest{
		Lease:  lease.Proto(),
		Prompt: "hello",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if startResp.Msg.GetStatus() != RunStatusRunning {
		t.Fatalf("start status = %q", startResp.Msg.GetStatus())
	}

	stream, err := client.StreamRunEvents(ctx, connect.NewRequest(&runnerv1.StreamRunEventsRequest{
		RunId:            lease.RunID,
		RunnerInstanceId: lease.RunnerInstanceID,
		LeaseVersion:     lease.LeaseVersion,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if got := receiveRunEvent(t, stream).GetEventType(); got != RunEventStarted {
		t.Fatalf("first event = %q", got)
	}

	if err := svc.PublishRunEvent(ctx, lease.Ref(), RunEvent{
		EventType: "run.delta",
		Status:    RunStatusRunning,
		Text:      "chunk",
	}.ProtoForLease(lease)); err != nil {
		t.Fatal(err)
	}
	if got := receiveRunEvent(t, stream).GetText(); got != "chunk" {
		t.Fatalf("delta text = %q", got)
	}

	cancelResp, err := client.CancelRun(ctx, connect.NewRequest(&runnerv1.CancelRunRequest{
		RunId:            lease.RunID,
		RunnerInstanceId: lease.RunnerInstanceID,
		LeaseVersion:     lease.LeaseVersion,
		Reason:           "user requested",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cancelResp.Msg.GetStatus() != RunStatusCancelled {
		t.Fatalf("cancel status = %q", cancelResp.Msg.GetStatus())
	}
	terminal := receiveRunEvent(t, stream)
	if terminal.GetEventType() != RunEventCancelled {
		t.Fatalf("terminal event = %q", terminal.GetEventType())
	}
	if terminal.GetStatus() != RunStatusCancelled {
		t.Fatalf("terminal status = %q", terminal.GetStatus())
	}
	if stream.Receive() {
		t.Fatalf("stream produced event after terminal event")
	}
	if err := stream.Err(); err != nil {
		t.Fatal(err)
	}
}

func newRunnerServiceTestClient(t *testing.T, svc *Service) (runnerv1connect.RunnerServiceClient, func()) {
	t.Helper()
	path, handler := runnerv1connect.NewRunnerServiceHandler(svc)
	mux := http.NewServeMux()
	mux.Handle(path, handler)
	server := httptest.NewServer(mux)
	return runnerv1connect.NewRunnerServiceClient(server.Client(), server.URL), server.Close
}

func receiveRunEvent(t *testing.T, stream *connect.ServerStreamForClient[runnerv1.StreamRunEventsResponse]) *eventv1.AgentRunEvent {
	t.Helper()
	if !stream.Receive() {
		t.Fatalf("stream ended: %v", stream.Err())
	}
	return stream.Msg().GetEvent()
}
