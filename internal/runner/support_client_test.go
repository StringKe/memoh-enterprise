package runner

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	browserv1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/browser/v1"
	eventv1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/event/v1"
	runnerv1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/runner/v1"
	"github.com/memohai/memoh/internal/connectapi/gen/memoh/runner/v1/runnerv1connect"
)

func TestSupportClientsDelegateRunnerSupportRPCs(t *testing.T) {
	ctx := context.Background()
	lease := testRunLease("run-support")
	backend := &fakeRunnerSupportService{
		allowedRunID:     lease.RunID,
		tokenExpiresAt:   lease.ExpiresAt.Add(-time.Minute),
		responseRunLease: lease,
	}
	client, closeServer := newRunnerSupportTestClient(t, backend)
	defer closeServer()

	contextClient := NewContextClient(client)
	runContext, err := contextClient.ResolveRunContext(ctx, lease)
	if err != nil {
		t.Fatal(err)
	}
	if runContext.GetLease().GetRunId() != lease.RunID {
		t.Fatalf("run context lease = %q", runContext.GetLease().GetRunId())
	}
	validation, err := contextClient.ValidateRunLease(ctx, lease)
	if err != nil {
		t.Fatal(err)
	}
	if !validation.GetValid() {
		t.Fatalf("lease validation returned false")
	}
	history, err := contextClient.ReadSessionHistory(ctx, lease, 20, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || history[0].GetText() != "hello" {
		t.Fatalf("history = %#v", history)
	}

	workspaceClient := NewWorkspaceClient(client, nil)
	token, err := workspaceClient.IssueWorkspaceToken(ctx, lease, []string{"workspace.exec"})
	if err != nil {
		t.Fatal(err)
	}
	if token.Token != "workspace-token" {
		t.Fatalf("workspace token = %q", token.Token)
	}

	outboundClient := NewOutboundClient(client)
	target, err := outboundClient.ResolveOutboundTarget(ctx, lease, "local", "conversation-1")
	if err != nil {
		t.Fatal(err)
	}
	if target.GetChannelConfigId() != "channel-config-1" {
		t.Fatalf("target channel config = %q", target.GetChannelConfigId())
	}
	dispatch, err := outboundClient.RequestOutboundDispatch(ctx, lease, OutboundDispatch{
		ChannelConfigID: target.GetChannelConfigId(),
		ChannelType:     target.GetChannelType(),
		ConversationID:  target.GetConversationId(),
		Text:            "reply",
	})
	if err != nil {
		t.Fatal(err)
	}
	if dispatch.GetDispatchId() != "dispatch-1" {
		t.Fatalf("dispatch id = %q", dispatch.GetDispatchId())
	}

	memoryClient := NewMemoryClient(client)
	memories, err := memoryClient.ReadMemory(ctx, lease, "query", []string{"bot"}, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(memories) != 1 || memories[0].GetContent() != "memory" {
		t.Fatalf("memories = %#v", memories)
	}
	memoryID, err := memoryClient.WriteMemory(ctx, lease, &runnerv1.MemoryRecord{Scope: "bot", Content: "new"})
	if err != nil {
		t.Fatal(err)
	}
	if memoryID != "memory-2" {
		t.Fatalf("memory id = %q", memoryID)
	}

	secretClient := NewSecretClient(client)
	secret, err := secretClient.ResolveScopedSecret(ctx, lease, "provider.openai", "model_call")
	if err != nil {
		t.Fatal(err)
	}
	if secret.GetSecretValue() != "secret-value" {
		t.Fatalf("secret value = %q", secret.GetSecretValue())
	}

	providerClient := NewProviderClient(client)
	credentials, err := providerClient.ResolveProviderCredentials(ctx, lease, "provider-1", "openai", []string{"model_call"})
	if err != nil {
		t.Fatal(err)
	}
	if credentials.GetCredentials().GetFields()["api_key"].GetStringValue() != "secret-value" {
		t.Fatalf("credentials = %#v", credentials.GetCredentials())
	}

	approvalClient := NewToolApprovalClient(client)
	decision, err := approvalClient.EvaluateToolApprovalPolicy(ctx, lease, "exec", "workspace.exec", nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.GetDecision() != "requires_approval" {
		t.Fatalf("decision = %q", decision.GetDecision())
	}
	approval, err := approvalClient.RequestToolApproval(ctx, lease, "exec", "workspace.exec", nil)
	if err != nil {
		t.Fatal(err)
	}
	if approval.GetApprovalRequestId() != "approval-1" {
		t.Fatalf("approval id = %q", approval.GetApprovalRequestId())
	}
}

func TestBrowserClientDelegatesContextActionAndScreenshot(t *testing.T) {
	browser := NewBrowserClient(fakeBrowserServiceClient{})
	ctx := context.Background()

	browserContext, err := browser.CreateContext(ctx, "core", "device", nil)
	if err != nil {
		t.Fatal(err)
	}
	if browserContext.GetContextId() != "context-1" {
		t.Fatalf("context id = %q", browserContext.GetContextId())
	}
	session, err := browser.CreateSession(ctx, browserContext.GetContextId(), "https://example.test")
	if err != nil {
		t.Fatal(err)
	}
	if session.GetSessionId() != "session-1" {
		t.Fatalf("session id = %q", session.GetSessionId())
	}
	action, err := browser.RunAction(ctx, BrowserAction{Kind: BrowserActionNavigate, SessionID: session.GetSessionId(), URL: "https://example.test"})
	if err != nil {
		t.Fatal(err)
	}
	if action.Status != "ok" {
		t.Fatalf("action status = %q", action.Status)
	}
	screenshot, err := browser.Screenshot(ctx, session.GetSessionId(), "", true, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if string(screenshot.GetImage()) != "png" {
		t.Fatalf("screenshot = %q", string(screenshot.GetImage()))
	}
}

func testRunLease(runID string) RunLease {
	return RunLease{
		RunID:                   runID,
		RunnerInstanceID:        "runner-1",
		BotID:                   "bot-1",
		BotGroupID:              "group-1",
		SessionID:               "session-1",
		UserID:                  "user-1",
		AllowedToolScopes:       []string{"workspace.exec", "memory.write"},
		WorkspaceExecutorTarget: "http://workspace-executor.test",
		WorkspaceID:             "workspace-1",
		ExpiresAt:               time.Date(2026, 5, 6, 10, 15, 0, 0, time.UTC),
		LeaseVersion:            7,
	}
}

func newRunnerSupportTestClient(t *testing.T, svc *fakeRunnerSupportService) (runnerv1connect.RunnerSupportServiceClient, func()) {
	t.Helper()
	path, handler := runnerv1connect.NewRunnerSupportServiceHandler(svc)
	mux := http.NewServeMux()
	mux.Handle(path, handler)
	server := httptest.NewServer(mux)
	return runnerv1connect.NewRunnerSupportServiceClient(server.Client(), server.URL), server.Close
}

type fakeRunnerSupportService struct {
	runnerv1connect.UnimplementedRunnerSupportServiceHandler
	allowedRunID     string
	tokenExpiresAt   time.Time
	responseRunLease RunLease
}

func (s *fakeRunnerSupportService) requireRef(ref *runnerv1.RunSupportRef) error {
	if ref.GetRunId() != s.allowedRunID {
		return connect.NewError(connect.CodePermissionDenied, errors.New("run lease denied"))
	}
	return nil
}

func (s *fakeRunnerSupportService) lease() RunLease {
	if s.responseRunLease.RunID != "" {
		return s.responseRunLease
	}
	return testRunLease(s.allowedRunID)
}

func (s *fakeRunnerSupportService) ResolveRunContext(_ context.Context, req *connect.Request[runnerv1.ResolveRunContextRequest]) (*connect.Response[runnerv1.ResolveRunContextResponse], error) {
	if err := s.requireRef(req.Msg.GetRef()); err != nil {
		return nil, err
	}
	return connect.NewResponse(&runnerv1.ResolveRunContextResponse{
		Lease: s.lease().Proto(),
		Bot:   mustStruct(map[string]any{"id": "bot-1"}),
		Model: mustStruct(map[string]any{"id": "model-1"}),
	}), nil
}

func (s *fakeRunnerSupportService) ValidateRunLease(_ context.Context, req *connect.Request[runnerv1.ValidateRunLeaseRequest]) (*connect.Response[runnerv1.ValidateRunLeaseResponse], error) {
	if err := s.requireRef(req.Msg.GetRef()); err != nil {
		return nil, err
	}
	return connect.NewResponse(&runnerv1.ValidateRunLeaseResponse{Lease: s.lease().Proto(), Valid: true}), nil
}

func (s *fakeRunnerSupportService) IssueWorkspaceToken(_ context.Context, req *connect.Request[runnerv1.IssueWorkspaceTokenRequest]) (*connect.Response[runnerv1.IssueWorkspaceTokenResponse], error) {
	if err := s.requireRef(req.Msg.GetRef()); err != nil {
		return nil, err
	}
	expiresAt := s.tokenExpiresAt
	if expiresAt.IsZero() {
		expiresAt = s.lease().ExpiresAt.Add(-time.Minute)
	}
	return connect.NewResponse(&runnerv1.IssueWorkspaceTokenResponse{
		Token:     "workspace-token",
		ExpiresAt: timestamppb.New(expiresAt),
	}), nil
}

func (s *fakeRunnerSupportService) ReadSessionHistory(_ context.Context, req *connect.Request[runnerv1.ReadSessionHistoryRequest]) (*connect.Response[runnerv1.ReadSessionHistoryResponse], error) {
	if err := s.requireRef(req.Msg.GetRef()); err != nil {
		return nil, err
	}
	return connect.NewResponse(&runnerv1.ReadSessionHistoryResponse{
		Messages: []*runnerv1.SessionMessage{{MessageId: "message-1", SessionId: req.Msg.GetSessionId(), BotId: "bot-1", Role: "user", Text: "hello"}},
	}), nil
}

func (s *fakeRunnerSupportService) AppendRunEvent(_ context.Context, req *connect.Request[runnerv1.AppendRunEventRequest]) (*connect.Response[runnerv1.AppendRunEventResponse], error) {
	if err := s.requireRef(req.Msg.GetRef()); err != nil {
		return nil, err
	}
	eventID := req.Msg.GetEvent().GetEventId()
	if eventID == "" {
		eventID = "event-1"
	}
	return connect.NewResponse(&runnerv1.AppendRunEventResponse{EventId: eventID}), nil
}

func (s *fakeRunnerSupportService) AppendSessionMessage(_ context.Context, req *connect.Request[runnerv1.AppendSessionMessageRequest]) (*connect.Response[runnerv1.AppendSessionMessageResponse], error) {
	if err := s.requireRef(req.Msg.GetRef()); err != nil {
		return nil, err
	}
	return connect.NewResponse(&runnerv1.AppendSessionMessageResponse{MessageId: "message-2"}), nil
}

func (s *fakeRunnerSupportService) ResolveOutboundTarget(_ context.Context, req *connect.Request[runnerv1.ResolveOutboundTargetRequest]) (*connect.Response[runnerv1.ResolveOutboundTargetResponse], error) {
	if err := s.requireRef(req.Msg.GetRef()); err != nil {
		return nil, err
	}
	return connect.NewResponse(&runnerv1.ResolveOutboundTargetResponse{
		ChannelConfigId: "channel-config-1",
		ChannelType:     req.Msg.GetChannelType(),
		ConversationId:  req.Msg.GetConversationId(),
		Target:          mustStruct(map[string]any{"id": "target-1"}),
	}), nil
}

func (s *fakeRunnerSupportService) RequestOutboundDispatch(_ context.Context, req *connect.Request[runnerv1.RequestOutboundDispatchRequest]) (*connect.Response[runnerv1.RequestOutboundDispatchResponse], error) {
	if err := s.requireRef(req.Msg.GetRef()); err != nil {
		return nil, err
	}
	return connect.NewResponse(&runnerv1.RequestOutboundDispatchResponse{DispatchId: "dispatch-1", Status: "queued"}), nil
}

func (s *fakeRunnerSupportService) ReadMemory(_ context.Context, req *connect.Request[runnerv1.ReadMemoryRequest]) (*connect.Response[runnerv1.ReadMemoryResponse], error) {
	if err := s.requireRef(req.Msg.GetRef()); err != nil {
		return nil, err
	}
	return connect.NewResponse(&runnerv1.ReadMemoryResponse{Memories: []*runnerv1.MemoryRecord{{MemoryId: "memory-1", Scope: "bot", Content: "memory", Score: 0.9}}}), nil
}

func (s *fakeRunnerSupportService) WriteMemory(_ context.Context, req *connect.Request[runnerv1.WriteMemoryRequest]) (*connect.Response[runnerv1.WriteMemoryResponse], error) {
	if err := s.requireRef(req.Msg.GetRef()); err != nil {
		return nil, err
	}
	return connect.NewResponse(&runnerv1.WriteMemoryResponse{MemoryId: "memory-2"}), nil
}

func (s *fakeRunnerSupportService) ResolveScopedSecret(_ context.Context, req *connect.Request[runnerv1.ResolveScopedSecretRequest]) (*connect.Response[runnerv1.ResolveScopedSecretResponse], error) {
	if err := s.requireRef(req.Msg.GetRef()); err != nil {
		return nil, err
	}
	return connect.NewResponse(&runnerv1.ResolveScopedSecretResponse{SecretValue: "secret-value", ExpiresAt: timestamppb.New(s.lease().ExpiresAt)}), nil
}

func (s *fakeRunnerSupportService) ResolveProviderCredentials(_ context.Context, req *connect.Request[runnerv1.ResolveProviderCredentialsRequest]) (*connect.Response[runnerv1.ResolveProviderCredentialsResponse], error) {
	if err := s.requireRef(req.Msg.GetRef()); err != nil {
		return nil, err
	}
	return connect.NewResponse(&runnerv1.ResolveProviderCredentialsResponse{
		ProviderId:   req.Msg.GetProviderId(),
		ProviderName: req.Msg.GetProviderName(),
		Credentials:  mustStruct(map[string]any{"api_key": "secret-value"}),
		ExpiresAt:    timestamppb.New(s.lease().ExpiresAt),
	}), nil
}

func (s *fakeRunnerSupportService) EvaluateToolApprovalPolicy(_ context.Context, req *connect.Request[runnerv1.EvaluateToolApprovalPolicyRequest]) (*connect.Response[runnerv1.EvaluateToolApprovalPolicyResponse], error) {
	if err := s.requireRef(req.Msg.GetRef()); err != nil {
		return nil, err
	}
	return connect.NewResponse(&runnerv1.EvaluateToolApprovalPolicyResponse{Decision: "requires_approval", Reason: "workspace exec"}), nil
}

func (s *fakeRunnerSupportService) RequestToolApproval(_ context.Context, req *connect.Request[runnerv1.RequestToolApprovalRequest]) (*connect.Response[runnerv1.RequestToolApprovalResponse], error) {
	if err := s.requireRef(req.Msg.GetRef()); err != nil {
		return nil, err
	}
	return connect.NewResponse(&runnerv1.RequestToolApprovalResponse{ApprovalRequestId: "approval-1", Status: "pending"}), nil
}

func mustStruct(values map[string]any) *structpb.Struct {
	msg, err := structpb.NewStruct(values)
	if err != nil {
		panic(err)
	}
	return msg
}

type fakeBrowserServiceClient struct{}

func (fakeBrowserServiceClient) ListCores(context.Context, *connect.Request[browserv1.ListCoresRequest]) (*connect.Response[browserv1.ListCoresResponse], error) {
	return connect.NewResponse(&browserv1.ListCoresResponse{}), nil
}

func (fakeBrowserServiceClient) ListDevices(context.Context, *connect.Request[browserv1.ListDevicesRequest]) (*connect.Response[browserv1.ListDevicesResponse], error) {
	return connect.NewResponse(&browserv1.ListDevicesResponse{}), nil
}

func (fakeBrowserServiceClient) CreateContext(context.Context, *connect.Request[browserv1.CreateContextRequest]) (*connect.Response[browserv1.CreateContextResponse], error) {
	return connect.NewResponse(&browserv1.CreateContextResponse{Context: &browserv1.BrowserContext{ContextId: "context-1"}}), nil
}

func (fakeBrowserServiceClient) CloseContext(context.Context, *connect.Request[browserv1.CloseContextRequest]) (*connect.Response[browserv1.CloseContextResponse], error) {
	return connect.NewResponse(&browserv1.CloseContextResponse{}), nil
}

func (fakeBrowserServiceClient) CreateSession(context.Context, *connect.Request[browserv1.CreateSessionRequest]) (*connect.Response[browserv1.CreateSessionResponse], error) {
	return connect.NewResponse(&browserv1.CreateSessionResponse{Session: &browserv1.BrowserSession{SessionId: "session-1", ContextId: "context-1"}}), nil
}

func (fakeBrowserServiceClient) CloseSession(context.Context, *connect.Request[browserv1.CloseSessionRequest]) (*connect.Response[browserv1.CloseSessionResponse], error) {
	return connect.NewResponse(&browserv1.CloseSessionResponse{}), nil
}

func (fakeBrowserServiceClient) Navigate(context.Context, *connect.Request[browserv1.NavigateRequest]) (*connect.Response[browserv1.NavigateResponse], error) {
	return connect.NewResponse(&browserv1.NavigateResponse{Status: "ok", Session: &browserv1.BrowserSession{SessionId: "session-1", ContextId: "context-1"}}), nil
}

func (fakeBrowserServiceClient) Click(context.Context, *connect.Request[browserv1.ClickRequest]) (*connect.Response[browserv1.ClickResponse], error) {
	return connect.NewResponse(&browserv1.ClickResponse{Status: "ok"}), nil
}

func (fakeBrowserServiceClient) TypeText(context.Context, *connect.Request[browserv1.TypeTextRequest]) (*connect.Response[browserv1.TypeTextResponse], error) {
	return connect.NewResponse(&browserv1.TypeTextResponse{Status: "ok"}), nil
}

func (fakeBrowserServiceClient) Screenshot(context.Context, *connect.Request[browserv1.ScreenshotRequest]) (*connect.Response[browserv1.ScreenshotResponse], error) {
	return connect.NewResponse(&browserv1.ScreenshotResponse{Image: []byte("png"), MimeType: "image/png"}), nil
}

func (fakeBrowserServiceClient) Evaluate(context.Context, *connect.Request[browserv1.EvaluateRequest]) (*connect.Response[browserv1.EvaluateResponse], error) {
	value, _ := structpb.NewValue("ok")
	return connect.NewResponse(&browserv1.EvaluateResponse{Result: value}), nil
}

var _ = eventv1.AgentRunEvent{}
