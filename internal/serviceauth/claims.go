package serviceauth

import (
	"errors"
	"slices"
	"time"
)

const (
	Issuer = "memoh-internal"

	AudienceServer             = "memoh-server"
	AudienceAgentRunner        = "memoh-agent-runner"
	AudienceConnector          = "memoh-connector"
	AudienceIntegrationGateway = "memoh-integration-gateway"
	AudienceWorkspaceExecutor  = "workspace-executor"

	ScopeRunnerRun          = "runner.run"
	ScopeRunnerCancel       = "runner.cancel"
	ScopeRunnerEvents       = "runner.events"
	ScopeWorkspaceExec      = "workspace.exec"
	ScopeWorkspaceFiles     = "workspace.files"
	ScopeConnectorInbound   = "connector.inbound"
	ScopeConnectorOutbound  = "connector.outbound"
	ScopeIntegrationGateway = "integration.gateway"
	ScopeServerContext      = "server.context"
	ScopeServerEvents       = "server.events"
)

const (
	MaxServiceTokenTTL   = 15 * time.Minute
	MaxWorkspaceTokenTTL = 60 * time.Second
)

var (
	ErrUnauthenticated  = errors.New("unauthenticated")
	ErrPermissionDenied = errors.New("permission denied")
	ErrInvalidToken     = errors.New("invalid service token")
	ErrInvalidKey       = errors.New("invalid service auth key")
)

type Claims struct {
	KeyID                   string
	Issuer                  string
	Audience                string
	Subject                 string
	Scopes                  []string
	RunID                   string
	LeaseVersion            int64
	WorkspaceID             string
	WorkspaceExecutorTarget string
	IssuedAt                time.Time
	ExpiresAt               time.Time
}

type RunLease struct {
	RunID                     string
	RunnerInstanceID          string
	BotID                     string
	BotGroupID                string
	SessionID                 string
	UserID                    string
	PermissionSnapshotVersion int64
	AllowedToolScopes         []string
	WorkspaceExecutorTarget   string
	WorkspaceID               string
	ExpiresAt                 time.Time
	LeaseVersion              int64
}

func RequireScope(claims Claims, audience string, scope string, now time.Time) error {
	if claims.Issuer != Issuer {
		return ErrUnauthenticated
	}
	if claims.Audience != audience {
		return ErrPermissionDenied
	}
	if !slices.Contains(claims.Scopes, scope) {
		return ErrPermissionDenied
	}
	if claims.ExpiresAt.IsZero() || !now.Before(claims.ExpiresAt) {
		return ErrUnauthenticated
	}
	if claims.IssuedAt.IsZero() || claims.ExpiresAt.Sub(claims.IssuedAt) > MaxServiceTokenTTL {
		return ErrUnauthenticated
	}
	return nil
}

func RequireWorkspaceLease(claims Claims, lease RunLease, scope string, now time.Time) error {
	if err := RequireScope(claims, AudienceWorkspaceExecutor, scope, now); err != nil {
		return err
	}
	if claims.ExpiresAt.Sub(claims.IssuedAt) > MaxWorkspaceTokenTTL {
		return ErrUnauthenticated
	}
	if claims.ExpiresAt.After(lease.ExpiresAt) {
		return ErrPermissionDenied
	}
	if claims.RunID != lease.RunID {
		return ErrPermissionDenied
	}
	if claims.LeaseVersion != lease.LeaseVersion {
		return ErrPermissionDenied
	}
	if claims.WorkspaceID != lease.WorkspaceID {
		return ErrPermissionDenied
	}
	if claims.WorkspaceExecutorTarget != lease.WorkspaceExecutorTarget {
		return ErrPermissionDenied
	}
	return nil
}
