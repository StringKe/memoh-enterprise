package connectapi

import (
	"context"
	"crypto/ed25519"
	"errors"
	"testing"
	"time"

	"github.com/memohai/memoh/internal/serviceauth"
)

func TestInternalAuthIssueWorkspaceTokenValidatesRunLease(t *testing.T) {
	now := time.Date(2026, 5, 6, 10, 0, 0, 0, time.UTC)
	service, verifier := newInternalAuthWorkspaceTestService(t, now, fakeRunLeaseResolver{
		lease: workspaceTestLease(now),
	})

	resp, err := service.IssueServiceToken(context.Background(), workspaceIssueRequest(now, workspaceTestLease(now), 60*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	verifier.SetNow(func() time.Time { return now.Add(time.Second) })
	claims, err := verifier.Verify(resp.Token)
	if err != nil {
		t.Fatal(err)
	}
	if err := serviceauth.RequireWorkspaceLease(claims, workspaceTestLease(now), serviceauth.ScopeWorkspaceExec, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}

	wrongWorkspace := workspaceIssueRequest(now, workspaceTestLease(now), 60*time.Second)
	wrongWorkspace.Workspace.WorkspaceID = "wrong"
	_, err = service.IssueServiceToken(context.Background(), wrongWorkspace)
	if !errors.Is(err, serviceauth.ErrPermissionDenied) {
		t.Fatalf("wrong workspace id error = %v", err)
	}

	wrongTarget := workspaceIssueRequest(now, workspaceTestLease(now), 60*time.Second)
	wrongTarget.Workspace.WorkspaceExecutorTarget = "unix:///wrong.sock"
	_, err = service.IssueServiceToken(context.Background(), wrongTarget)
	if !errors.Is(err, serviceauth.ErrPermissionDenied) {
		t.Fatalf("wrong workspace target error = %v", err)
	}

	tooLong := workspaceIssueRequest(now, workspaceTestLease(now), 61*time.Second)
	_, err = service.IssueServiceToken(context.Background(), tooLong)
	if err == nil {
		t.Fatal("workspace ttl over 60s was accepted")
	}

	shortLease := workspaceTestLease(now)
	shortLease.ExpiresAt = now.Add(30 * time.Second)
	shortService, _ := newInternalAuthWorkspaceTestService(t, now, fakeRunLeaseResolver{lease: shortLease})
	afterLease := workspaceIssueRequest(now, shortLease, 0)
	afterLease.ExpiresAt = shortLease.ExpiresAt.Add(time.Second)
	_, err = shortService.IssueServiceToken(context.Background(), afterLease)
	if !errors.Is(err, serviceauth.ErrPermissionDenied) {
		t.Fatalf("exp after run lease error = %v", err)
	}
}

func newInternalAuthWorkspaceTestService(t *testing.T, now time.Time, leases RunLeaseResolver) (*InternalAuthService, *serviceauth.Verifier) {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	publicKey, err := serviceauth.PublicKeyFromPrivate(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := serviceauth.NewSigner("active", map[string]ed25519.PrivateKey{"active": privateKey})
	if err != nil {
		t.Fatal(err)
	}
	signer.SetNow(func() time.Time { return now })
	registration := serviceauth.NewRegistrationValidator("bootstrap")
	registration.SetNow(func() time.Time { return now })
	service := NewInternalAuthService(signer, registration, nil, leases)
	service.SetNow(func() time.Time { return now })
	verifier, err := serviceauth.NewVerifier(map[string]ed25519.PublicKey{"active": publicKey})
	if err != nil {
		t.Fatal(err)
	}
	return service, verifier
}

func workspaceIssueRequest(now time.Time, lease serviceauth.RunLease, ttl time.Duration) IssueServiceTokenRequest {
	return IssueServiceTokenRequest{
		ServiceName:             serviceauth.AudienceAgentRunner,
		InstanceID:              lease.RunnerInstanceID,
		Audience:                serviceauth.AudienceWorkspaceExecutor,
		Scopes:                  []string{serviceauth.ScopeWorkspaceExec},
		TTL:                     ttl,
		BootstrapToken:          "bootstrap",
		BootstrapTokenExpiresAt: now.Add(time.Minute),
		Workspace: &WorkspaceTokenRequest{
			RunID:                   lease.RunID,
			RunnerInstanceID:        lease.RunnerInstanceID,
			BotID:                   lease.BotID,
			SessionID:               lease.SessionID,
			WorkspaceID:             lease.WorkspaceID,
			WorkspaceExecutorTarget: lease.WorkspaceExecutorTarget,
			LeaseVersion:            lease.LeaseVersion,
		},
	}
}

func workspaceTestLease(now time.Time) serviceauth.RunLease {
	return serviceauth.RunLease{
		RunID:                   "run-1",
		RunnerInstanceID:        "runner-1",
		BotID:                   "bot-1",
		SessionID:               "session-1",
		UserID:                  "user-1",
		WorkspaceExecutorTarget: "unix:///run/memoh/workspace-executor.sock",
		WorkspaceID:             "workspace-1",
		ExpiresAt:               now.Add(60 * time.Second),
		LeaseVersion:            7,
	}
}
