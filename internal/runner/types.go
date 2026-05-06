package runner

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	eventv1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/event/v1"
	runnerv1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/runner/v1"
)

const (
	RunStatusRunning   = "running"
	RunStatusCancelled = "cancelled"
	RunStatusCompleted = "completed"
	RunStatusFailed    = "failed"

	RunEventStarted   = "run.started"
	RunEventCancelled = "run.cancelled"
	RunEventCompleted = "run.completed"
	RunEventFailed    = "run.failed"
)

var (
	ErrNilRunLease                  = errors.New("run lease is nil")
	ErrInvalidRunLease              = errors.New("run lease is invalid")
	ErrRunNotFound                  = errors.New("run not found")
	ErrRunRefMismatch               = errors.New("run reference does not match active lease")
	ErrRunAlreadyTerminal           = errors.New("run is already terminal")
	ErrSupportClientMissing         = errors.New("runner support client is not configured")
	ErrBrowserClientMissing         = errors.New("browser service client is not configured")
	ErrWorkspaceExecutorTargetEmpty = errors.New("workspace executor target is empty")
	ErrWorkspaceTokenOutlivesLease  = errors.New("workspace token expires after run lease")
)

type RunRequest struct {
	Lease         RunLease
	Prompt        string
	AttachmentIDs []string
	Options       *structpb.Struct
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

type RunRef struct {
	RunID            string
	RunnerInstanceID string
	LeaseVersion     int64
}

type RunEvent struct {
	EventID    string
	EventType  string
	Status     string
	Text       string
	Payload    *structpb.Struct
	OccurredAt time.Time
}

type WorkspaceToken struct {
	Token     string
	ExpiresAt time.Time
}

func RunRequestFromProto(req *runnerv1.StartRunRequest) (RunRequest, error) {
	if req == nil {
		return RunRequest{}, ErrInvalidRunLease
	}
	lease, err := RunLeaseFromProto(req.GetLease())
	if err != nil {
		return RunRequest{}, err
	}
	return RunRequest{
		Lease:         lease,
		Prompt:        req.GetPrompt(),
		AttachmentIDs: append([]string(nil), req.GetAttachmentIds()...),
		Options:       req.GetOptions(),
	}, nil
}

func RunLeaseFromProto(lease *runnerv1.RunLease) (RunLease, error) {
	if lease == nil {
		return RunLease{}, ErrNilRunLease
	}
	out := RunLease{
		RunID:                     strings.TrimSpace(lease.GetRunId()),
		RunnerInstanceID:          strings.TrimSpace(lease.GetRunnerInstanceId()),
		BotID:                     strings.TrimSpace(lease.GetBotId()),
		BotGroupID:                strings.TrimSpace(lease.GetBotGroupId()),
		SessionID:                 strings.TrimSpace(lease.GetSessionId()),
		UserID:                    strings.TrimSpace(lease.GetUserId()),
		PermissionSnapshotVersion: lease.GetPermissionSnapshotVersion(),
		AllowedToolScopes:         append([]string(nil), lease.GetAllowedToolScopes()...),
		WorkspaceExecutorTarget:   strings.TrimSpace(lease.GetWorkspaceExecutorTarget()),
		WorkspaceID:               strings.TrimSpace(lease.GetWorkspaceId()),
		LeaseVersion:              lease.GetLeaseVersion(),
	}
	if lease.GetExpiresAt() != nil {
		out.ExpiresAt = lease.GetExpiresAt().AsTime()
	}
	return out, out.Validate()
}

func (l RunLease) Validate() error {
	switch {
	case strings.TrimSpace(l.RunID) == "":
		return fmt.Errorf("%w: run id is required", ErrInvalidRunLease)
	case strings.TrimSpace(l.RunnerInstanceID) == "":
		return fmt.Errorf("%w: runner instance id is required", ErrInvalidRunLease)
	case strings.TrimSpace(l.BotID) == "":
		return fmt.Errorf("%w: bot id is required", ErrInvalidRunLease)
	case strings.TrimSpace(l.SessionID) == "":
		return fmt.Errorf("%w: session id is required", ErrInvalidRunLease)
	case l.LeaseVersion <= 0:
		return fmt.Errorf("%w: lease version is required", ErrInvalidRunLease)
	case l.ExpiresAt.IsZero():
		return fmt.Errorf("%w: expires at is required", ErrInvalidRunLease)
	default:
		return nil
	}
}

func (l RunLease) Proto() *runnerv1.RunLease {
	return &runnerv1.RunLease{
		RunId:                     l.RunID,
		RunnerInstanceId:          l.RunnerInstanceID,
		BotId:                     l.BotID,
		BotGroupId:                l.BotGroupID,
		SessionId:                 l.SessionID,
		UserId:                    l.UserID,
		PermissionSnapshotVersion: l.PermissionSnapshotVersion,
		AllowedToolScopes:         append([]string(nil), l.AllowedToolScopes...),
		WorkspaceExecutorTarget:   l.WorkspaceExecutorTarget,
		WorkspaceId:               l.WorkspaceID,
		ExpiresAt:                 timestamppb.New(l.ExpiresAt),
		LeaseVersion:              l.LeaseVersion,
	}
}

func (l RunLease) Ref() RunRef {
	return RunRef{
		RunID:            l.RunID,
		RunnerInstanceID: l.RunnerInstanceID,
		LeaseVersion:     l.LeaseVersion,
	}
}

func (l RunLease) MatchesRef(ref RunRef) bool {
	return l.RunID == ref.RunID &&
		l.RunnerInstanceID == ref.RunnerInstanceID &&
		l.LeaseVersion == ref.LeaseVersion
}

func RunRefFromProto(ref *runnerv1.RunSupportRef) RunRef {
	if ref == nil {
		return RunRef{}
	}
	return RunRef{
		RunID:            strings.TrimSpace(ref.GetRunId()),
		RunnerInstanceID: strings.TrimSpace(ref.GetRunnerInstanceId()),
		LeaseVersion:     ref.GetLeaseVersion(),
	}
}

func (r RunRef) Proto() *runnerv1.RunSupportRef {
	return &runnerv1.RunSupportRef{
		RunId:            r.RunID,
		RunnerInstanceId: r.RunnerInstanceID,
		LeaseVersion:     r.LeaseVersion,
	}
}

func (r RunRef) Validate() error {
	switch {
	case strings.TrimSpace(r.RunID) == "":
		return fmt.Errorf("%w: run id is required", ErrInvalidRunLease)
	case strings.TrimSpace(r.RunnerInstanceID) == "":
		return fmt.Errorf("%w: runner instance id is required", ErrInvalidRunLease)
	case r.LeaseVersion <= 0:
		return fmt.Errorf("%w: lease version is required", ErrInvalidRunLease)
	default:
		return nil
	}
}

func (e RunEvent) ProtoForLease(lease RunLease) *eventv1.AgentRunEvent {
	occurredAt := e.OccurredAt
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}
	return &eventv1.AgentRunEvent{
		EventId:    e.EventID,
		RunId:      lease.RunID,
		BotId:      lease.BotID,
		BotGroupId: lease.BotGroupID,
		SessionId:  lease.SessionID,
		UserId:     lease.UserID,
		EventType:  e.EventType,
		Status:     e.Status,
		Text:       e.Text,
		Payload:    e.Payload,
		OccurredAt: timestamppb.New(occurredAt),
	}
}

func terminalStatus(status string) bool {
	switch status {
	case RunStatusCancelled, RunStatusCompleted, RunStatusFailed:
		return true
	default:
		return false
	}
}
