package connectapi

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/memohai/memoh/internal/serviceauth"
)

type InternalAuthService struct {
	signer       *serviceauth.Signer
	registration *serviceauth.RegistrationValidator
	kubernetes   *serviceauth.KubernetesServiceAccountValidator
	leases       RunLeaseResolver
	now          func() time.Time
}

type IssueServiceTokenRequest struct {
	ServiceName              string
	InstanceID               string
	Audience                 string
	Scopes                   []string
	TTL                      time.Duration
	ExpiresAt                time.Time
	BootstrapToken           string
	BootstrapTokenExpiresAt  time.Time
	KubernetesServiceAccount serviceauth.KubernetesServiceAccountIdentity
	Workspace                *WorkspaceTokenRequest
}

type WorkspaceTokenRequest struct {
	RunID                   string
	RunnerInstanceID        string
	BotID                   string
	SessionID               string
	WorkspaceID             string
	WorkspaceExecutorTarget string
	LeaseVersion            int64
}

type IssueServiceTokenResponse struct {
	Token     string
	ExpiresAt time.Time
}

func NewInternalAuthService(
	signer *serviceauth.Signer,
	registration *serviceauth.RegistrationValidator,
	kubernetes *serviceauth.KubernetesServiceAccountValidator,
	leases RunLeaseResolver,
) *InternalAuthService {
	return &InternalAuthService{
		signer:       signer,
		registration: registration,
		kubernetes:   kubernetes,
		leases:       leases,
		now:          time.Now,
	}
}

func (s *InternalAuthService) SetNow(now func() time.Time) {
	if now != nil {
		s.now = now
	}
}

func (s *InternalAuthService) IssueServiceToken(ctx context.Context, req IssueServiceTokenRequest) (IssueServiceTokenResponse, error) {
	if s == nil || s.signer == nil {
		return IssueServiceTokenResponse{}, errors.New("internal auth signer is not configured")
	}
	if err := s.authenticateRegistration(req); err != nil {
		return IssueServiceTokenResponse{}, err
	}
	now := s.now().UTC()
	maxTTL := serviceauth.MaxServiceTokenTTL
	if req.Workspace != nil {
		maxTTL = serviceauth.MaxWorkspaceTokenTTL
	}
	expiresAt, err := requestedExpiresAt(now, req.TTL, req.ExpiresAt, maxTTL)
	if err != nil {
		return IssueServiceTokenResponse{}, err
	}
	claims := serviceauth.Claims{
		Issuer:    serviceauth.Issuer,
		Audience:  strings.TrimSpace(req.Audience),
		Subject:   strings.TrimSpace(req.InstanceID),
		Scopes:    normalizedScopes(req.Scopes),
		IssuedAt:  now,
		ExpiresAt: expiresAt,
	}
	if claims.Audience == "" {
		return IssueServiceTokenResponse{}, errors.New("audience is required")
	}
	if claims.Subject == "" {
		return IssueServiceTokenResponse{}, errors.New("instance_id is required")
	}
	if len(claims.Scopes) == 0 {
		return IssueServiceTokenResponse{}, errors.New("scopes are required")
	}
	if req.Workspace != nil {
		lease, err := s.validateWorkspaceRequest(ctx, req, expiresAt)
		if err != nil {
			return IssueServiceTokenResponse{}, err
		}
		claims.Audience = serviceauth.AudienceWorkspaceExecutor
		claims.Subject = lease.RunnerInstanceID
		claims.RunID = lease.RunID
		claims.LeaseVersion = lease.LeaseVersion
		claims.WorkspaceID = lease.WorkspaceID
		claims.WorkspaceExecutorTarget = lease.WorkspaceExecutorTarget
	}
	token, err := s.signer.Sign(claims)
	if err != nil {
		return IssueServiceTokenResponse{}, err
	}
	return IssueServiceTokenResponse{Token: token, ExpiresAt: expiresAt}, nil
}

func (s *InternalAuthService) authenticateRegistration(req IssueServiceTokenRequest) error {
	kube := req.KubernetesServiceAccount
	hasKubernetesIdentity := strings.TrimSpace(kube.Audience) != "" ||
		strings.TrimSpace(kube.Namespace) != "" ||
		strings.TrimSpace(kube.ServiceAccountName) != ""
	if hasKubernetesIdentity {
		return s.kubernetes.Validate(kube)
	}
	if s.registration == nil {
		return errors.New("internal auth registration validator is not configured")
	}
	return s.registration.Validate(serviceauth.RegistrationToken{
		Token:     req.BootstrapToken,
		ExpiresAt: req.BootstrapTokenExpiresAt,
	})
}

func (s *InternalAuthService) validateWorkspaceRequest(ctx context.Context, req IssueServiceTokenRequest, expiresAt time.Time) (serviceauth.RunLease, error) {
	if s.leases == nil {
		return serviceauth.RunLease{}, ErrRunnerSupportDependencyMissing
	}
	if req.Audience != "" && req.Audience != serviceauth.AudienceWorkspaceExecutor {
		return serviceauth.RunLease{}, serviceauth.ErrPermissionDenied
	}
	for _, scope := range normalizedScopes(req.Scopes) {
		if scope != serviceauth.ScopeWorkspaceExec && scope != serviceauth.ScopeWorkspaceFiles {
			return serviceauth.RunLease{}, serviceauth.ErrPermissionDenied
		}
	}
	workspace := req.Workspace
	lease, err := s.leases.ResolveRunLease(ctx, workspace.RunID)
	if err != nil {
		return serviceauth.RunLease{}, err
	}
	if expiresAt.After(lease.ExpiresAt) {
		return serviceauth.RunLease{}, serviceauth.ErrPermissionDenied
	}
	if workspace.RunnerInstanceID != lease.RunnerInstanceID ||
		workspace.BotID != lease.BotID ||
		workspace.SessionID != lease.SessionID ||
		workspace.WorkspaceID != lease.WorkspaceID ||
		workspace.WorkspaceExecutorTarget != lease.WorkspaceExecutorTarget ||
		workspace.LeaseVersion != lease.LeaseVersion ||
		strings.TrimSpace(req.InstanceID) != lease.RunnerInstanceID {
		return serviceauth.RunLease{}, serviceauth.ErrPermissionDenied
	}
	return lease, nil
}

func requestedExpiresAt(now time.Time, ttl time.Duration, expiresAt time.Time, maxTTL time.Duration) (time.Time, error) {
	if !expiresAt.IsZero() {
		if !expiresAt.After(now) {
			return time.Time{}, errors.New("expires_at must be in the future")
		}
		if expiresAt.Sub(now) > maxTTL {
			return time.Time{}, errors.New("requested ttl exceeds max")
		}
		return expiresAt.UTC(), nil
	}
	if ttl == 0 {
		ttl = maxTTL
	}
	if ttl <= 0 || ttl > maxTTL {
		return time.Time{}, errors.New("requested ttl exceeds max")
	}
	return now.Add(ttl).UTC(), nil
}

func normalizedScopes(scopes []string) []string {
	out := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		scope = strings.TrimSpace(scope)
		if scope == "" || slices.Contains(out, scope) {
			continue
		}
		out = append(out, scope)
	}
	return out
}
