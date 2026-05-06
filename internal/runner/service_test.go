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

func TestServiceStartRunUsesSupportAPIBeforeTerminalEvent(t *testing.T) {
	ctx := context.Background()
	lease := testRunLease("run-support-execute")
	supportBackend := &fakeRunnerSupportService{
		allowedRunID:     lease.RunID,
		responseRunLease: lease,
	}
	supportClient, closeSupport := newRunnerSupportTestClient(t, supportBackend)
	defer closeSupport()
	svc := NewService(ServiceDeps{
		SupportClient: supportClient,
		Clock: func() time.Time {
			return time.Date(2026, 5, 6, 10, 0, 0, 0, time.UTC)
		},
	})
	client, closeServer := newRunnerServiceTestClient(t, svc)
	defer closeServer()

	if _, err := client.StartRun(ctx, connect.NewRequest(&runnerv1.StartRunRequest{
		Lease:   lease.Proto(),
		Prompt:  "hello",
		Options: mustStruct(map[string]any{"source": "chat"}),
	})); err != nil {
		t.Fatal(err)
	}
	stream, err := client.StreamRunEvents(ctx, connect.NewRequest(&runnerv1.StreamRunEventsRequest{
		RunId:            lease.RunID,
		RunnerInstanceId: lease.RunnerInstanceID,
		LeaseVersion:     lease.LeaseVersion,
	}))
	if err != nil {
		t.Fatal(err)
	}
	var terminal *eventv1.AgentRunEvent
	for stream.Receive() {
		event := stream.Msg().GetEvent()
		if event.GetStatus() == RunStatusFailed {
			terminal = event
			break
		}
	}
	if terminal == nil {
		t.Fatalf("terminal event missing: %v", stream.Err())
	}
	if supportBackend.validateCalls == 0 || supportBackend.contextCalls == 0 || supportBackend.historyCalls == 0 {
		t.Fatalf("support calls validate=%d context=%d history=%d", supportBackend.validateCalls, supportBackend.contextCalls, supportBackend.historyCalls)
	}
	if supportBackend.appendEventCalls == 0 {
		t.Fatal("runner did not append run events through support API")
	}
}

func TestServiceStartRunStreamsExecutorDeltaAndPersistsAssistantMessage(t *testing.T) {
	ctx := context.Background()
	lease := testRunLease("run-executor-complete")
	supportBackend := &fakeRunnerSupportService{
		allowedRunID:     lease.RunID,
		responseRunLease: lease,
	}
	supportClient, closeSupport := newRunnerSupportTestClient(t, supportBackend)
	defer closeSupport()
	svc := NewService(ServiceDeps{
		SupportClient: supportClient,
		Executor: fakeExecutorFunc(func(_ context.Context, input ExecutionInput) (ExecutionResult, error) {
			if input.Context.GetLease().GetRunId() != lease.RunID {
				t.Fatalf("executor context lease = %q", input.Context.GetLease().GetRunId())
			}
			if len(input.History) != 1 {
				t.Fatalf("executor history len = %d", len(input.History))
			}
			if err := input.Emit(RunEvent{
				EventType: RunEventTextDelta,
				Status:    RunStatusRunning,
				Text:      "hello",
			}.ProtoForLease(lease)); err != nil {
				return ExecutionResult{}, err
			}
			return ExecutionResult{Status: RunStatusCompleted, AssistantText: "hello"}, nil
		}),
	})
	client, closeServer := newRunnerServiceTestClient(t, svc)
	defer closeServer()

	if _, err := client.StartRun(ctx, connect.NewRequest(&runnerv1.StartRunRequest{
		Lease:   lease.Proto(),
		Prompt:  "hello",
		Options: mustStruct(map[string]any{"source": "chat"}),
	})); err != nil {
		t.Fatal(err)
	}
	stream, err := client.StreamRunEvents(ctx, connect.NewRequest(&runnerv1.StreamRunEventsRequest{
		RunId:            lease.RunID,
		RunnerInstanceId: lease.RunnerInstanceID,
		LeaseVersion:     lease.LeaseVersion,
	}))
	if err != nil {
		t.Fatal(err)
	}
	var sawDelta, sawCompleted bool
	for stream.Receive() {
		event := stream.Msg().GetEvent()
		if event.GetEventType() == RunEventTextDelta && event.GetText() == "hello" {
			sawDelta = true
		}
		if event.GetEventType() == RunEventCompleted && event.GetStatus() == RunStatusCompleted {
			sawCompleted = true
			break
		}
	}
	if !sawDelta || !sawCompleted {
		t.Fatalf("sawDelta=%v sawCompleted=%v streamErr=%v", sawDelta, sawCompleted, stream.Err())
	}
	if supportBackend.appendEventCalls < 3 {
		t.Fatalf("append event calls = %d", supportBackend.appendEventCalls)
	}
	if supportBackend.appendSessionCalls != 1 || supportBackend.lastAssistantText != "hello" {
		t.Fatalf("append session calls=%d text=%q", supportBackend.appendSessionCalls, supportBackend.lastAssistantText)
	}
}

func TestServiceStartRunFailsWhenSupportClientMissing(t *testing.T) {
	ctx := context.Background()
	lease := testRunLease("run-support-missing")
	svc := NewService(ServiceDeps{})
	client, closeServer := newRunnerServiceTestClient(t, svc)
	defer closeServer()

	if _, err := client.StartRun(ctx, connect.NewRequest(&runnerv1.StartRunRequest{
		Lease:   lease.Proto(),
		Options: mustStruct(map[string]any{"source": "chat"}),
	})); err != nil {
		t.Fatal(err)
	}
	stream, err := client.StreamRunEvents(ctx, connect.NewRequest(&runnerv1.StreamRunEventsRequest{
		RunId:            lease.RunID,
		RunnerInstanceId: lease.RunnerInstanceID,
		LeaseVersion:     lease.LeaseVersion,
	}))
	if err != nil {
		t.Fatal(err)
	}
	var terminal *eventv1.AgentRunEvent
	for stream.Receive() {
		event := stream.Msg().GetEvent()
		if event.GetStatus() == RunStatusFailed {
			terminal = event
			break
		}
	}
	if terminal == nil || terminal.GetEventType() != RunEventFailed {
		t.Fatalf("terminal = %#v err=%v", terminal, stream.Err())
	}
}

func TestServiceStartRunRejectsLeaseMismatchAndTerminalEvents(t *testing.T) {
	ctx := context.Background()
	lease := testRunLease("run-lease-mismatch")
	svc := NewService(ServiceDeps{})
	client, closeServer := newRunnerServiceTestClient(t, svc)
	defer closeServer()

	if _, err := client.StartRun(ctx, connect.NewRequest(&runnerv1.StartRunRequest{Lease: lease.Proto()})); err != nil {
		t.Fatal(err)
	}
	wrong := lease
	wrong.LeaseVersion++
	if _, err := client.StartRun(ctx, connect.NewRequest(&runnerv1.StartRunRequest{Lease: wrong.Proto()})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("mismatch error = %v", err)
	}
	if err := svc.AcknowledgeCompletion(ctx, lease.Ref(), RunStatusCancelled, "stop"); err != nil {
		t.Fatal(err)
	}
	err := svc.PublishRunEvent(ctx, lease.Ref(), RunEvent{EventType: RunEventTextDelta, Text: "late"}.ProtoForLease(lease))
	if connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Fatalf("late event error = %v", err)
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

type fakeExecutorFunc func(context.Context, ExecutionInput) (ExecutionResult, error)

func (f fakeExecutorFunc) Execute(ctx context.Context, input ExecutionInput) (ExecutionResult, error) {
	return f(ctx, input)
}
