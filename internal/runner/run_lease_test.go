package runner

import (
	"context"
	"errors"
	"testing"
	"time"

	"connectrpc.com/connect"
)

func TestRunLeaseDeniesCrossRunSecret(t *testing.T) {
	backend := &fakeRunnerSupportService{allowedRunID: "run-a"}
	client, closeServer := newRunnerSupportTestClient(t, backend)
	defer closeServer()

	secretClient := NewSecretClient(client)
	_, err := secretClient.ResolveScopedSecret(context.Background(), testRunLease("run-b"), "provider.openai", "model_call")
	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("secret error code = %v err=%v", connect.CodeOf(err), err)
	}
}

func TestRunLeaseRejectsWorkspaceTokenOutlivingLease(t *testing.T) {
	lease := testRunLease("run-token")
	backend := &fakeRunnerSupportService{
		allowedRunID:     lease.RunID,
		tokenExpiresAt:   lease.ExpiresAt.Add(time.Second),
		responseRunLease: lease,
	}
	client, closeServer := newRunnerSupportTestClient(t, backend)
	defer closeServer()

	workspaceClient := NewWorkspaceClient(client, nil)
	_, err := workspaceClient.IssueWorkspaceToken(context.Background(), lease, []string{"workspace.exec"})
	if !errors.Is(err, ErrWorkspaceTokenOutlivesLease) {
		t.Fatalf("workspace token error = %v", err)
	}
}
