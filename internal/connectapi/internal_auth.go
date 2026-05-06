package connectapi

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	privatev1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1"
	"github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1/privatev1connect"
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
	KeyID     string
	Issuer    string
	Audience  string
	IssuedAt  time.Time
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

type internalAuthHandler struct {
	service *InternalAuthService
}

func NewInternalAuthHandler(service *InternalAuthService) Handler {
	path, handler := privatev1connect.NewInternalAuthServiceHandler(&internalAuthHandler{service: service})
	return NewHandler(path, handler)
}

func (h *internalAuthHandler) IssueServiceToken(ctx context.Context, req *connect.Request[privatev1.IssueServiceTokenRequest]) (*connect.Response[privatev1.IssueServiceTokenResponse], error) {
	if h == nil || h.service == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal auth service is not configured"))
	}
	msg := req.Msg
	if msg.GetTtlSeconds() < 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("ttl_seconds must be non-negative"))
	}
	coreReq := IssueServiceTokenRequest{
		ServiceName:             msg.GetCallerService(),
		InstanceID:              msg.GetCallerInstanceId(),
		Audience:                msg.GetTargetAudience(),
		Scopes:                  append([]string(nil), msg.GetScopes()...),
		TTL:                     time.Duration(msg.GetTtlSeconds()) * time.Second,
		BootstrapToken:          msg.GetBootstrapToken(),
		BootstrapTokenExpiresAt: h.service.now().UTC().Add(serviceauth.MaxServiceTokenTTL),
	}
	if requestHasWorkspaceTokenFields(msg) {
		if coreReq.Audience == "" {
			coreReq.Audience = serviceauth.AudienceWorkspaceExecutor
		}
		coreReq.Workspace = &WorkspaceTokenRequest{
			RunID:                   msg.GetRunId(),
			RunnerInstanceID:        msg.GetCallerInstanceId(),
			WorkspaceID:             msg.GetWorkspaceId(),
			WorkspaceExecutorTarget: msg.GetWorkspaceExecutorTarget(),
			LeaseVersion:            msg.GetLeaseVersion(),
		}
	}
	resp, err := h.service.IssueServiceToken(ctx, coreReq)
	if err != nil {
		return nil, internalAuthConnectError(err)
	}
	return connect.NewResponse(&privatev1.IssueServiceTokenResponse{
		Token:     resp.Token,
		KeyId:     resp.KeyID,
		Issuer:    resp.Issuer,
		Audience:  resp.Audience,
		IssuedAt:  timestamppb.New(resp.IssuedAt),
		ExpiresAt: timestamppb.New(resp.ExpiresAt),
	}), nil
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
	return IssueServiceTokenResponse{
		Token:     token,
		KeyID:     s.signer.ActiveKeyID(),
		Issuer:    claims.Issuer,
		Audience:  claims.Audience,
		IssuedAt:  claims.IssuedAt,
		ExpiresAt: expiresAt,
	}, nil
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
	if strings.TrimSpace(req.InstanceID) != lease.RunnerInstanceID ||
		workspace.WorkspaceID != lease.WorkspaceID ||
		workspace.WorkspaceExecutorTarget != lease.WorkspaceExecutorTarget ||
		workspace.LeaseVersion != lease.LeaseVersion {
		return serviceauth.RunLease{}, serviceauth.ErrPermissionDenied
	}
	if strings.TrimSpace(workspace.RunnerInstanceID) != "" && workspace.RunnerInstanceID != lease.RunnerInstanceID {
		return serviceauth.RunLease{}, serviceauth.ErrPermissionDenied
	}
	if strings.TrimSpace(workspace.BotID) != "" && workspace.BotID != lease.BotID {
		return serviceauth.RunLease{}, serviceauth.ErrPermissionDenied
	}
	if strings.TrimSpace(workspace.SessionID) != "" && workspace.SessionID != lease.SessionID {
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

func requestHasWorkspaceTokenFields(req *privatev1.IssueServiceTokenRequest) bool {
	return strings.TrimSpace(req.GetRunId()) != "" ||
		strings.TrimSpace(req.GetWorkspaceId()) != "" ||
		strings.TrimSpace(req.GetWorkspaceExecutorTarget()) != "" ||
		req.GetLeaseVersion() != 0
}

func internalAuthConnectError(err error) error {
	switch {
	case errors.Is(err, serviceauth.ErrUnauthenticated):
		return connect.NewError(connect.CodeUnauthenticated, err)
	case errors.Is(err, serviceauth.ErrPermissionDenied):
		return connect.NewError(connect.CodePermissionDenied, err)
	case errors.Is(err, ErrRunnerSupportDependencyMissing):
		return connect.NewError(connect.CodeInternal, err)
	default:
		return connect.NewError(connect.CodeInvalidArgument, err)
	}
}
