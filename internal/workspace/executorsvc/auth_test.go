package executorsvc

import (
	"errors"
	"testing"
	"time"

	"github.com/memohai/memoh/internal/serviceauth"
)

func TestAuthorizeWorkspaceLeaseRejectsWrongClaims(t *testing.T) {
	t.Parallel()

	now := time.Unix(100, 0)
	baseClaims := serviceauth.Claims{
		Issuer:                  serviceauth.Issuer,
		Audience:                serviceauth.AudienceWorkspaceExecutor,
		Scopes:                  []string{serviceauth.ScopeWorkspaceExec},
		RunID:                   "run-1",
		LeaseVersion:            7,
		WorkspaceID:             "workspace-1",
		WorkspaceExecutorTarget: "unix:///run/memoh/workspace-executor.sock",
		IssuedAt:                now.Add(-time.Second),
		ExpiresAt:               now.Add(30 * time.Second),
	}
	baseLease := serviceauth.RunLease{
		RunID:                   "run-1",
		LeaseVersion:            7,
		WorkspaceID:             "workspace-1",
		WorkspaceExecutorTarget: "unix:///run/memoh/workspace-executor.sock",
		ExpiresAt:               now.Add(time.Minute),
	}

	tests := []struct {
		name   string
		claims serviceauth.Claims
		lease  serviceauth.RunLease
	}{
		{
			name: "wrong audience",
			claims: func() serviceauth.Claims {
				claims := baseClaims
				claims.Audience = serviceauth.AudienceServer
				return claims
			}(),
			lease: baseLease,
		},
		{
			name:   "wrong workspace id",
			claims: baseClaims,
			lease: func() serviceauth.RunLease {
				lease := baseLease
				lease.WorkspaceID = "workspace-2"
				return lease
			}(),
		},
		{
			name:   "wrong target",
			claims: baseClaims,
			lease: func() serviceauth.RunLease {
				lease := baseLease
				lease.WorkspaceExecutorTarget = "unix:///run/memoh/other.sock"
				return lease
			}(),
		},
		{
			name:   "wrong lease version",
			claims: baseClaims,
			lease: func() serviceauth.RunLease {
				lease := baseLease
				lease.LeaseVersion = 8
				return lease
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := authorizeWorkspaceLease(tt.claims, tt.lease, serviceauth.ScopeWorkspaceExec, now)
			if !errors.Is(err, serviceauth.ErrPermissionDenied) {
				t.Fatalf("authorizeWorkspaceLease error = %v, want ErrPermissionDenied", err)
			}
		})
	}
}

func TestAuthorizeWorkspaceLeaseAcceptsMatchingClaims(t *testing.T) {
	t.Parallel()

	now := time.Unix(100, 0)
	claims := serviceauth.Claims{
		Issuer:                  serviceauth.Issuer,
		Audience:                serviceauth.AudienceWorkspaceExecutor,
		Scopes:                  []string{serviceauth.ScopeWorkspaceExec},
		RunID:                   "run-1",
		LeaseVersion:            7,
		WorkspaceID:             "workspace-1",
		WorkspaceExecutorTarget: "unix:///run/memoh/workspace-executor.sock",
		IssuedAt:                now.Add(-time.Second),
		ExpiresAt:               now.Add(30 * time.Second),
	}
	lease := serviceauth.RunLease{
		RunID:                   "run-1",
		LeaseVersion:            7,
		WorkspaceID:             "workspace-1",
		WorkspaceExecutorTarget: "unix:///run/memoh/workspace-executor.sock",
		ExpiresAt:               now.Add(time.Minute),
	}

	if err := authorizeWorkspaceLease(claims, lease, serviceauth.ScopeWorkspaceExec, now); err != nil {
		t.Fatalf("authorizeWorkspaceLease: %v", err)
	}
}
