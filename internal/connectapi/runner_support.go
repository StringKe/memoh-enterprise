package connectapi

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	dbsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
	"github.com/memohai/memoh/internal/serviceauth"
)

var ErrRunnerSupportDependencyMissing = errors.New("runner support dependency is not configured")

type RunLeaseRef struct {
	RunID                   string
	RunnerInstanceID        string
	BotID                   string
	SessionID               string
	WorkspaceID             string
	WorkspaceExecutorTarget string
	LeaseVersion            int64
}

type RunLeaseResolver interface {
	ResolveRunLease(ctx context.Context, runID string) (serviceauth.RunLease, error)
}

type AgentRunLeaseQueries interface {
	GetActiveAgentRunLease(ctx context.Context, runID pgtype.UUID) (dbsqlc.AgentRunLease, error)
}

type SQLRunLeaseResolver struct {
	Queries AgentRunLeaseQueries
}

func (r SQLRunLeaseResolver) ResolveRunLease(ctx context.Context, runID string) (serviceauth.RunLease, error) {
	if r.Queries == nil {
		return serviceauth.RunLease{}, ErrRunnerSupportDependencyMissing
	}
	pgRunID, err := parseUUIDText(runID)
	if err != nil {
		return serviceauth.RunLease{}, err
	}
	row, err := r.Queries.GetActiveAgentRunLease(ctx, pgRunID)
	if err != nil {
		return serviceauth.RunLease{}, err
	}
	return runLeaseFromSQL(row), nil
}

type RunnerSupportService struct {
	leases          RunLeaseResolver
	internalAuth    *InternalAuthService
	runContext      RunContextResolver
	sessionHistory  SessionHistoryReader
	runEvents       RunEventAppender
	sessionMessages SessionMessageAppender
	outbound        OutboundSupport
	memory          MemorySupport
	secrets         SecretSupport
	providers       ProviderCredentialSupport
	toolApprovals   ToolApprovalSupport
}

func NewRunnerSupportService(leases RunLeaseResolver, internalAuth *InternalAuthService) *RunnerSupportService {
	return &RunnerSupportService{leases: leases, internalAuth: internalAuth}
}

func (s *RunnerSupportService) SetRunContextResolver(resolver RunContextResolver) {
	s.runContext = resolver
}

func (s *RunnerSupportService) SetSessionHistoryReader(reader SessionHistoryReader) {
	s.sessionHistory = reader
}

func (s *RunnerSupportService) SetRunEventAppender(appender RunEventAppender) {
	s.runEvents = appender
}

func (s *RunnerSupportService) SetSessionMessageAppender(appender SessionMessageAppender) {
	s.sessionMessages = appender
}

func (s *RunnerSupportService) SetOutboundSupport(outbound OutboundSupport) {
	s.outbound = outbound
}

func (s *RunnerSupportService) SetMemorySupport(memory MemorySupport) {
	s.memory = memory
}

func (s *RunnerSupportService) SetSecretSupport(secrets SecretSupport) {
	s.secrets = secrets
}

func (s *RunnerSupportService) SetProviderCredentialSupport(providers ProviderCredentialSupport) {
	s.providers = providers
}

func (s *RunnerSupportService) SetToolApprovalSupport(approvals ToolApprovalSupport) {
	s.toolApprovals = approvals
}

func (s *RunnerSupportService) ValidateRunLease(ctx context.Context, req ValidateRunLeaseRequest) (serviceauth.RunLease, error) {
	return s.requireLease(ctx, req.Lease)
}

func (s *RunnerSupportService) IssueWorkspaceToken(ctx context.Context, req RunnerIssueWorkspaceTokenRequest) (IssueServiceTokenResponse, error) {
	if s.internalAuth == nil {
		return IssueServiceTokenResponse{}, ErrRunnerSupportDependencyMissing
	}
	lease, err := s.requireLease(ctx, req.Lease)
	if err != nil {
		return IssueServiceTokenResponse{}, err
	}
	return s.internalAuth.IssueServiceToken(ctx, IssueServiceTokenRequest{
		ServiceName:              serviceauth.AudienceAgentRunner,
		InstanceID:               lease.RunnerInstanceID,
		Audience:                 serviceauth.AudienceWorkspaceExecutor,
		Scopes:                   req.Scopes,
		TTL:                      req.TTL,
		BootstrapToken:           req.BootstrapToken,
		BootstrapTokenExpiresAt:  req.BootstrapTokenExpiresAt,
		KubernetesServiceAccount: req.KubernetesServiceAccount,
		Workspace: &WorkspaceTokenRequest{
			RunID:                   lease.RunID,
			RunnerInstanceID:        lease.RunnerInstanceID,
			BotID:                   lease.BotID,
			SessionID:               lease.SessionID,
			WorkspaceID:             lease.WorkspaceID,
			WorkspaceExecutorTarget: lease.WorkspaceExecutorTarget,
			LeaseVersion:            lease.LeaseVersion,
		},
	})
}

func (s *RunnerSupportService) ResolveRunContext(ctx context.Context, req ResolveRunContextRequest) (ResolveRunContextResponse, error) {
	if _, err := s.requireLease(ctx, req.Lease); err != nil {
		return ResolveRunContextResponse{}, err
	}
	if s.runContext == nil {
		return ResolveRunContextResponse{}, ErrRunnerSupportDependencyMissing
	}
	return s.runContext.ResolveRunContext(ctx, req)
}

func (s *RunnerSupportService) ReadSessionHistory(ctx context.Context, req ReadSessionHistoryRequest) (ReadSessionHistoryResponse, error) {
	if _, err := s.requireLease(ctx, req.Lease); err != nil {
		return ReadSessionHistoryResponse{}, err
	}
	if s.sessionHistory == nil {
		return ReadSessionHistoryResponse{}, ErrRunnerSupportDependencyMissing
	}
	return s.sessionHistory.ReadSessionHistory(ctx, req)
}

func (s *RunnerSupportService) AppendRunEvent(ctx context.Context, req AppendRunEventRequest) error {
	if _, err := s.requireLease(ctx, req.Lease); err != nil {
		return err
	}
	if s.runEvents == nil {
		return ErrRunnerSupportDependencyMissing
	}
	return s.runEvents.AppendRunEvent(ctx, req)
}

func (s *RunnerSupportService) AppendSessionMessage(ctx context.Context, req AppendSessionMessageRequest) error {
	if _, err := s.requireLease(ctx, req.Lease); err != nil {
		return err
	}
	if s.sessionMessages == nil {
		return ErrRunnerSupportDependencyMissing
	}
	return s.sessionMessages.AppendSessionMessage(ctx, req)
}

func (s *RunnerSupportService) ResolveOutboundTarget(ctx context.Context, req ResolveOutboundTargetRequest) (ResolveOutboundTargetResponse, error) {
	if _, err := s.requireLease(ctx, req.Lease); err != nil {
		return ResolveOutboundTargetResponse{}, err
	}
	if s.outbound == nil {
		return ResolveOutboundTargetResponse{}, ErrRunnerSupportDependencyMissing
	}
	return s.outbound.ResolveOutboundTarget(ctx, req)
}

func (s *RunnerSupportService) RequestOutboundDispatch(ctx context.Context, req RequestOutboundDispatchRequest) error {
	if _, err := s.requireLease(ctx, req.Lease); err != nil {
		return err
	}
	if s.outbound == nil {
		return ErrRunnerSupportDependencyMissing
	}
	return s.outbound.RequestOutboundDispatch(ctx, req)
}

func (s *RunnerSupportService) ReadMemory(ctx context.Context, req ReadMemoryRequest) (ReadMemoryResponse, error) {
	if _, err := s.requireLease(ctx, req.Lease); err != nil {
		return ReadMemoryResponse{}, err
	}
	if s.memory == nil {
		return ReadMemoryResponse{}, ErrRunnerSupportDependencyMissing
	}
	return s.memory.ReadMemory(ctx, req)
}

func (s *RunnerSupportService) WriteMemory(ctx context.Context, req WriteMemoryRequest) error {
	if _, err := s.requireLease(ctx, req.Lease); err != nil {
		return err
	}
	if s.memory == nil {
		return ErrRunnerSupportDependencyMissing
	}
	return s.memory.WriteMemory(ctx, req)
}

func (s *RunnerSupportService) ResolveScopedSecret(ctx context.Context, req ResolveScopedSecretRequest) (ResolveScopedSecretResponse, error) {
	if _, err := s.requireLease(ctx, req.Lease); err != nil {
		return ResolveScopedSecretResponse{}, err
	}
	if s.secrets == nil {
		return ResolveScopedSecretResponse{}, ErrRunnerSupportDependencyMissing
	}
	return s.secrets.ResolveScopedSecret(ctx, req)
}

func (s *RunnerSupportService) ResolveProviderCredentials(ctx context.Context, req ResolveProviderCredentialsRequest) (ResolveProviderCredentialsResponse, error) {
	if _, err := s.requireLease(ctx, req.Lease); err != nil {
		return ResolveProviderCredentialsResponse{}, err
	}
	if s.providers == nil {
		return ResolveProviderCredentialsResponse{}, ErrRunnerSupportDependencyMissing
	}
	return s.providers.ResolveProviderCredentials(ctx, req)
}

func (s *RunnerSupportService) EvaluateToolApprovalPolicy(ctx context.Context, req EvaluateToolApprovalPolicyRequest) (EvaluateToolApprovalPolicyResponse, error) {
	if _, err := s.requireLease(ctx, req.Lease); err != nil {
		return EvaluateToolApprovalPolicyResponse{}, err
	}
	if s.toolApprovals == nil {
		return EvaluateToolApprovalPolicyResponse{}, ErrRunnerSupportDependencyMissing
	}
	return s.toolApprovals.EvaluateToolApprovalPolicy(ctx, req)
}

func (s *RunnerSupportService) RequestToolApproval(ctx context.Context, req RequestToolApprovalRequest) (RequestToolApprovalResponse, error) {
	if _, err := s.requireLease(ctx, req.Lease); err != nil {
		return RequestToolApprovalResponse{}, err
	}
	if s.toolApprovals == nil {
		return RequestToolApprovalResponse{}, ErrRunnerSupportDependencyMissing
	}
	return s.toolApprovals.RequestToolApproval(ctx, req)
}

func (s *RunnerSupportService) requireLease(ctx context.Context, ref RunLeaseRef) (serviceauth.RunLease, error) {
	if s == nil || s.leases == nil {
		return serviceauth.RunLease{}, ErrRunnerSupportDependencyMissing
	}
	if strings.TrimSpace(ref.RunID) == "" {
		return serviceauth.RunLease{}, errors.New("run_id is required")
	}
	lease, err := s.leases.ResolveRunLease(ctx, ref.RunID)
	if err != nil {
		return serviceauth.RunLease{}, err
	}
	if ref.RunnerInstanceID != lease.RunnerInstanceID ||
		ref.LeaseVersion != lease.LeaseVersion ||
		ref.BotID != lease.BotID ||
		ref.SessionID != lease.SessionID ||
		ref.WorkspaceID != lease.WorkspaceID ||
		ref.WorkspaceExecutorTarget != lease.WorkspaceExecutorTarget {
		return serviceauth.RunLease{}, serviceauth.ErrPermissionDenied
	}
	return lease, nil
}

type ValidateRunLeaseRequest struct {
	Lease RunLeaseRef
}

type RunnerIssueWorkspaceTokenRequest struct {
	Lease                    RunLeaseRef
	Scopes                   []string
	TTL                      time.Duration
	BootstrapToken           string
	BootstrapTokenExpiresAt  time.Time
	KubernetesServiceAccount serviceauth.KubernetesServiceAccountIdentity
}

type ResolveRunContextRequest struct {
	Lease RunLeaseRef
}

type ResolveRunContextResponse struct {
	Context map[string]any
}

type ReadSessionHistoryRequest struct {
	Lease RunLeaseRef
	Limit int32
}

type ReadSessionHistoryResponse struct {
	Messages []SessionMessage
}

type SessionMessage struct {
	Role      string
	Content   string
	Metadata  []byte
	CreatedAt time.Time
}

type AppendRunEventRequest struct {
	Lease       RunLeaseRef
	EventType   string
	Payload     []byte
	Idempotency string
}

type AppendSessionMessageRequest struct {
	Lease   RunLeaseRef
	Message SessionMessage
}

type ResolveOutboundTargetRequest struct {
	Lease       RunLeaseRef
	ChannelType string
}

type ResolveOutboundTargetResponse struct {
	Target map[string]any
}

type RequestOutboundDispatchRequest struct {
	Lease   RunLeaseRef
	Target  map[string]any
	Payload []byte
}

type ReadMemoryRequest struct {
	Lease RunLeaseRef
	Query string
	Limit int32
}

type ReadMemoryResponse struct {
	Items []map[string]any
}

type WriteMemoryRequest struct {
	Lease   RunLeaseRef
	Entries []map[string]any
}

type ResolveScopedSecretRequest struct {
	Lease RunLeaseRef
	Name  string
}

type ResolveScopedSecretResponse struct {
	Value string
}

type ResolveProviderCredentialsRequest struct {
	Lease      RunLeaseRef
	ProviderID string
}

type ResolveProviderCredentialsResponse struct {
	Credentials map[string]any
}

type EvaluateToolApprovalPolicyRequest struct {
	Lease    RunLeaseRef
	ToolName string
	Input    []byte
}

type EvaluateToolApprovalPolicyResponse struct {
	RequiresApproval bool
	Reason           string
}

type RequestToolApprovalRequest struct {
	Lease      RunLeaseRef
	ToolName   string
	ToolInput  []byte
	PromptText string
}

type RequestToolApprovalResponse struct {
	RequestID string
}

type RunContextResolver interface {
	ResolveRunContext(ctx context.Context, req ResolveRunContextRequest) (ResolveRunContextResponse, error)
}

type SessionHistoryReader interface {
	ReadSessionHistory(ctx context.Context, req ReadSessionHistoryRequest) (ReadSessionHistoryResponse, error)
}

type RunEventAppender interface {
	AppendRunEvent(ctx context.Context, req AppendRunEventRequest) error
}

type SessionMessageAppender interface {
	AppendSessionMessage(ctx context.Context, req AppendSessionMessageRequest) error
}

type OutboundSupport interface {
	ResolveOutboundTarget(ctx context.Context, req ResolveOutboundTargetRequest) (ResolveOutboundTargetResponse, error)
	RequestOutboundDispatch(ctx context.Context, req RequestOutboundDispatchRequest) error
}

type MemorySupport interface {
	ReadMemory(ctx context.Context, req ReadMemoryRequest) (ReadMemoryResponse, error)
	WriteMemory(ctx context.Context, req WriteMemoryRequest) error
}

type SecretSupport interface {
	ResolveScopedSecret(ctx context.Context, req ResolveScopedSecretRequest) (ResolveScopedSecretResponse, error)
}

type ProviderCredentialSupport interface {
	ResolveProviderCredentials(ctx context.Context, req ResolveProviderCredentialsRequest) (ResolveProviderCredentialsResponse, error)
}

type ToolApprovalSupport interface {
	EvaluateToolApprovalPolicy(ctx context.Context, req EvaluateToolApprovalPolicyRequest) (EvaluateToolApprovalPolicyResponse, error)
	RequestToolApproval(ctx context.Context, req RequestToolApprovalRequest) (RequestToolApprovalResponse, error)
}

func runLeaseFromSQL(row dbsqlc.AgentRunLease) serviceauth.RunLease {
	return serviceauth.RunLease{
		RunID:                     row.RunID.String(),
		RunnerInstanceID:          row.RunnerInstanceID,
		BotID:                     row.BotID.String(),
		BotGroupID:                row.BotGroupID.String(),
		SessionID:                 row.SessionID.String(),
		UserID:                    row.UserID.String(),
		PermissionSnapshotVersion: row.PermissionSnapshotVersion,
		AllowedToolScopes:         append([]string(nil), row.AllowedToolScopes...),
		WorkspaceExecutorTarget:   row.WorkspaceExecutorTarget,
		WorkspaceID:               row.WorkspaceID,
		ExpiresAt:                 row.ExpiresAt.Time,
		LeaseVersion:              row.LeaseVersion,
	}
}

func parseUUIDText(value string) (pgtype.UUID, error) {
	var out pgtype.UUID
	if err := out.Scan(strings.TrimSpace(value)); err != nil {
		return pgtype.UUID{}, err
	}
	return out, nil
}
