package connectapi

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/memohai/memoh/internal/serviceauth"
)

func TestRunnerSupportValidateRunLease(t *testing.T) {
	now := time.Date(2026, 5, 6, 10, 0, 0, 0, time.UTC)
	lease := workspaceTestLease(now)
	service := NewRunnerSupportService(fakeRunLeaseResolver{lease: lease}, nil)
	got, err := service.ValidateRunLease(context.Background(), ValidateRunLeaseRequest{Lease: refFromLease(lease)})
	if err != nil {
		t.Fatal(err)
	}
	if got.RunID != lease.RunID {
		t.Fatalf("run id = %q", got.RunID)
	}
	wrong := refFromLease(lease)
	wrong.SessionID = "other-session"
	_, err = service.ValidateRunLease(context.Background(), ValidateRunLeaseRequest{Lease: wrong})
	if !errors.Is(err, serviceauth.ErrPermissionDenied) {
		t.Fatalf("wrong session error = %v", err)
	}
}

func TestRunnerSupportDelegatesBusinessOperations(t *testing.T) {
	now := time.Date(2026, 5, 6, 10, 0, 0, 0, time.UTC)
	lease := workspaceTestLease(now)
	backend := &fakeRunnerSupportBackend{}
	service := NewRunnerSupportService(fakeRunLeaseResolver{lease: lease}, nil)
	service.SetRunContextResolver(backend)
	service.SetSessionHistoryReader(backend)
	service.SetRunEventAppender(backend)
	service.SetSessionMessageAppender(backend)
	service.SetOutboundSupport(backend)
	service.SetMemorySupport(backend)
	service.SetSecretSupport(backend)
	service.SetProviderCredentialSupport(backend)
	service.SetToolApprovalSupport(backend)
	ref := refFromLease(lease)

	if _, err := service.ResolveRunContext(context.Background(), ResolveRunContextRequest{Lease: ref}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ReadSessionHistory(context.Background(), ReadSessionHistoryRequest{Lease: ref, Limit: 20}); err != nil {
		t.Fatal(err)
	}
	if err := service.AppendRunEvent(context.Background(), AppendRunEventRequest{Lease: ref, EventType: "started"}); err != nil {
		t.Fatal(err)
	}
	if err := service.AppendSessionMessage(context.Background(), AppendSessionMessageRequest{Lease: ref, Message: SessionMessage{Role: "assistant"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ResolveOutboundTarget(context.Background(), ResolveOutboundTargetRequest{Lease: ref, ChannelType: "local"}); err != nil {
		t.Fatal(err)
	}
	if err := service.RequestOutboundDispatch(context.Background(), RequestOutboundDispatchRequest{Lease: ref}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ReadMemory(context.Background(), ReadMemoryRequest{Lease: ref, Query: "q"}); err != nil {
		t.Fatal(err)
	}
	if err := service.WriteMemory(context.Background(), WriteMemoryRequest{Lease: ref, Entries: []map[string]any{{"k": "v"}}}); err != nil {
		t.Fatal(err)
	}
	if secret, err := service.ResolveScopedSecret(context.Background(), ResolveScopedSecretRequest{Lease: ref, Name: "provider"}); err != nil || secret.Value != "secret" {
		t.Fatalf("secret = %#v err=%v", secret, err)
	}
	if creds, err := service.ResolveProviderCredentials(context.Background(), ResolveProviderCredentialsRequest{Lease: ref, ProviderID: "provider-1"}); err != nil || creds.Credentials["api_key"] != "secret" {
		t.Fatalf("credentials = %#v err=%v", creds, err)
	}
	if decision, err := service.EvaluateToolApprovalPolicy(context.Background(), EvaluateToolApprovalPolicyRequest{Lease: ref, ToolName: "exec"}); err != nil || !decision.RequiresApproval {
		t.Fatalf("decision = %#v err=%v", decision, err)
	}
	if approval, err := service.RequestToolApproval(context.Background(), RequestToolApprovalRequest{Lease: ref, ToolName: "exec"}); err != nil || approval.RequestID != "approval-1" {
		t.Fatalf("approval = %#v err=%v", approval, err)
	}
	if backend.calls != 12 {
		t.Fatalf("backend calls = %d", backend.calls)
	}
}

type fakeRunLeaseResolver struct {
	lease serviceauth.RunLease
	err   error
}

func (f fakeRunLeaseResolver) ResolveRunLease(context.Context, string) (serviceauth.RunLease, error) {
	return f.lease, f.err
}

func refFromLease(lease serviceauth.RunLease) RunLeaseRef {
	return RunLeaseRef{
		RunID:                   lease.RunID,
		RunnerInstanceID:        lease.RunnerInstanceID,
		BotID:                   lease.BotID,
		SessionID:               lease.SessionID,
		WorkspaceID:             lease.WorkspaceID,
		WorkspaceExecutorTarget: lease.WorkspaceExecutorTarget,
		LeaseVersion:            lease.LeaseVersion,
	}
}

type fakeRunnerSupportBackend struct {
	calls int
}

func (f *fakeRunnerSupportBackend) ResolveRunContext(context.Context, ResolveRunContextRequest) (ResolveRunContextResponse, error) {
	f.calls++
	return ResolveRunContextResponse{Context: map[string]any{"bot_id": "bot-1"}}, nil
}

func (f *fakeRunnerSupportBackend) ReadSessionHistory(context.Context, ReadSessionHistoryRequest) (ReadSessionHistoryResponse, error) {
	f.calls++
	return ReadSessionHistoryResponse{Messages: []SessionMessage{{Role: "user", Content: "hi"}}}, nil
}

func (f *fakeRunnerSupportBackend) AppendRunEvent(context.Context, AppendRunEventRequest) error {
	f.calls++
	return nil
}

func (f *fakeRunnerSupportBackend) AppendSessionMessage(context.Context, AppendSessionMessageRequest) error {
	f.calls++
	return nil
}

func (f *fakeRunnerSupportBackend) ResolveOutboundTarget(context.Context, ResolveOutboundTargetRequest) (ResolveOutboundTargetResponse, error) {
	f.calls++
	return ResolveOutboundTargetResponse{Target: map[string]any{"channel": "local"}}, nil
}

func (f *fakeRunnerSupportBackend) RequestOutboundDispatch(context.Context, RequestOutboundDispatchRequest) error {
	f.calls++
	return nil
}

func (f *fakeRunnerSupportBackend) ReadMemory(context.Context, ReadMemoryRequest) (ReadMemoryResponse, error) {
	f.calls++
	return ReadMemoryResponse{Items: []map[string]any{{"text": "memory"}}}, nil
}

func (f *fakeRunnerSupportBackend) WriteMemory(context.Context, WriteMemoryRequest) error {
	f.calls++
	return nil
}

func (f *fakeRunnerSupportBackend) ResolveScopedSecret(context.Context, ResolveScopedSecretRequest) (ResolveScopedSecretResponse, error) {
	f.calls++
	return ResolveScopedSecretResponse{Value: "secret"}, nil
}

func (f *fakeRunnerSupportBackend) ResolveProviderCredentials(context.Context, ResolveProviderCredentialsRequest) (ResolveProviderCredentialsResponse, error) {
	f.calls++
	return ResolveProviderCredentialsResponse{Credentials: map[string]any{"api_key": "secret"}}, nil
}

func (f *fakeRunnerSupportBackend) EvaluateToolApprovalPolicy(context.Context, EvaluateToolApprovalPolicyRequest) (EvaluateToolApprovalPolicyResponse, error) {
	f.calls++
	return EvaluateToolApprovalPolicyResponse{RequiresApproval: true, Reason: "exec"}, nil
}

func (f *fakeRunnerSupportBackend) RequestToolApproval(context.Context, RequestToolApprovalRequest) (RequestToolApprovalResponse, error) {
	f.calls++
	return RequestToolApprovalResponse{RequestID: "approval-1"}, nil
}
