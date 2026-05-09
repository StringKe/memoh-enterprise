package runnerdispatch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/memohai/memoh/internal/auth"
	eventv1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/event/v1"
	runnerv1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/runner/v1"
	"github.com/memohai/memoh/internal/connectapi/gen/memoh/runner/v1/runnerv1connect"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	"github.com/memohai/memoh/internal/heartbeat"
	"github.com/memohai/memoh/internal/schedule"
	"github.com/memohai/memoh/internal/serviceauth"
	sessionpkg "github.com/memohai/memoh/internal/session"
)

const (
	defaultRunnerInstanceID = "memoh-agent-runner"
	defaultRunLeaseTTL      = 15 * time.Minute
	emailTriggerTokenTTL    = 10 * time.Minute
)

type WorkspaceTargetResolver interface {
	WorkspaceExecutorTarget(botID string) string
}

type LeaseQueries interface {
	CreateAgentRunLease(ctx context.Context, arg sqlc.CreateAgentRunLeaseParams) (sqlc.AgentRunLease, error)
	GetBotByID(ctx context.Context, id pgtype.UUID) (sqlc.GetBotByIDRow, error)
}

type Service struct {
	log              *slog.Logger
	queries          LeaseQueries
	sessions         *sessionpkg.Service
	runner           runnerv1connect.RunnerServiceClient
	workspaceTargets WorkspaceTargetResolver
	runnerInstanceID string
	jwtSecret        string
	leaseTTL         time.Duration
	now              func() time.Time
}

type Deps struct {
	Logger           *slog.Logger
	Queries          LeaseQueries
	Sessions         *sessionpkg.Service
	Runner           runnerv1connect.RunnerServiceClient
	WorkspaceTargets WorkspaceTargetResolver
	RunnerInstanceID string
	JWTSecret        string
	LeaseTTL         time.Duration
	Now              func() time.Time
}

func New(deps Deps) *Service {
	log := deps.Logger
	if log == nil {
		log = slog.Default()
	}
	runnerInstanceID := strings.TrimSpace(deps.RunnerInstanceID)
	if runnerInstanceID == "" {
		runnerInstanceID = defaultRunnerInstanceID
	}
	leaseTTL := deps.LeaseTTL
	if leaseTTL <= 0 {
		leaseTTL = defaultRunLeaseTTL
	}
	now := deps.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Service{
		log:              log.With(slog.String("service", "runner_dispatch")),
		queries:          deps.Queries,
		sessions:         deps.Sessions,
		runner:           deps.Runner,
		workspaceTargets: deps.WorkspaceTargets,
		runnerInstanceID: runnerInstanceID,
		jwtSecret:        strings.TrimSpace(deps.JWTSecret),
		leaseTTL:         leaseTTL,
		now:              now,
	}
}

func NewRunnerClient(addr string) runnerv1connect.RunnerServiceClient {
	baseURL := strings.TrimSpace(addr)
	if baseURL == "" {
		baseURL = "http://127.0.0.1:26813"
	}
	return runnerv1connect.NewRunnerServiceClient(http.DefaultClient, baseURL)
}

func (s *Service) Chat(ctx context.Context, req conversation.ChatRequest) (conversation.ChatResponse, error) {
	chunks, errs := s.StreamChat(ctx, req)
	var text strings.Builder
	var messages []conversation.ModelMessage
	for chunks != nil || errs != nil {
		select {
		case <-ctx.Done():
			return conversation.ChatResponse{}, ctx.Err()
		case chunk, ok := <-chunks:
			if !ok {
				chunks = nil
				continue
			}
			delta, terminalMessages := parseConversationChunk(chunk)
			text.WriteString(delta)
			if len(terminalMessages) > 0 {
				messages = terminalMessages
			}
		case err, ok := <-errs:
			if !ok {
				errs = nil
				continue
			}
			if err != nil {
				return conversation.ChatResponse{}, err
			}
		}
	}
	if len(messages) == 0 && strings.TrimSpace(text.String()) != "" {
		messages = []conversation.ModelMessage{{
			Role:    "assistant",
			Content: conversation.NewTextContent(text.String()),
		}}
	}
	return conversation.ChatResponse{Messages: messages}, nil
}

func (s *Service) StreamChat(ctx context.Context, req conversation.ChatRequest) (<-chan conversation.StreamChunk, <-chan error) {
	chunkCh := make(chan conversation.StreamChunk)
	errCh := make(chan error, 1)
	go func() {
		defer close(chunkCh)
		defer close(errCh)

		lease, err := s.createLease(ctx, req, sessionpkg.TypeChat)
		if err != nil {
			errCh <- err
			return
		}
		if s.runner == nil {
			errCh <- errors.New("runner service client not configured")
			return
		}
		if _, err := s.runner.StartRun(ctx, connect.NewRequest(&runnerv1.StartRunRequest{
			Lease:         lease,
			Prompt:        req.Query,
			AttachmentIds: attachmentIDs(req.Attachments),
			Options:       runOptions(req, "chat"),
		})); err != nil {
			errCh <- err
			return
		}
		s.streamEvents(ctx, lease, chunkCh, errCh)
	}()
	return chunkCh, errCh
}

func (s *Service) TriggerSchedule(ctx context.Context, botID string, payload schedule.TriggerPayload, token string) (schedule.TriggerResult, error) {
	req := conversation.ChatRequest{
		BotID:     botID,
		ChatID:    botID,
		SessionID: payload.SessionID,
		Query:     payload.Command,
		UserID:    payload.OwnerUserID,
		Token:     token,
	}
	text, err := s.runTrigger(ctx, req, "schedule")
	if err != nil {
		return schedule.TriggerResult{}, err
	}
	return schedule.TriggerResult{Status: "ok", Text: text}, nil
}

func (s *Service) TriggerHeartbeat(ctx context.Context, botID string, payload heartbeat.TriggerPayload, token string) (heartbeat.TriggerResult, error) {
	req := conversation.ChatRequest{
		BotID:     botID,
		ChatID:    botID,
		SessionID: payload.SessionID,
		Query:     "heartbeat",
		UserID:    payload.OwnerUserID,
		Token:     token,
	}
	text, err := s.runTrigger(ctx, req, "heartbeat")
	if err != nil {
		return heartbeat.TriggerResult{}, err
	}
	return heartbeat.TriggerResult{Status: "ok", Text: text, SessionID: req.SessionID}, nil
}

func (s *Service) TriggerBotChat(ctx context.Context, botID, content string) error {
	ownerID, err := s.resolveBotOwner(ctx, botID)
	if err != nil {
		return err
	}
	token, err := s.generateToken(ownerID)
	if err != nil {
		return err
	}
	_, err = s.Chat(ctx, conversation.ChatRequest{
		BotID:          botID,
		ChatID:         botID,
		Query:          content,
		UserID:         ownerID,
		Token:          token,
		CurrentChannel: "email",
	})
	return err
}

func (s *Service) runTrigger(ctx context.Context, req conversation.ChatRequest, source string) (string, error) {
	lease, err := s.createLease(ctx, req, source)
	if err != nil {
		return "", err
	}
	if s.runner == nil {
		return "", errors.New("runner service client not configured")
	}
	if _, err := s.runner.StartRun(ctx, connect.NewRequest(&runnerv1.StartRunRequest{
		Lease:   lease,
		Prompt:  req.Query,
		Options: runOptions(req, source),
	})); err != nil {
		return "", err
	}
	stream, err := s.runner.StreamRunEvents(ctx, connect.NewRequest(&runnerv1.StreamRunEventsRequest{
		RunId:            lease.GetRunId(),
		RunnerInstanceId: lease.GetRunnerInstanceId(),
		LeaseVersion:     lease.GetLeaseVersion(),
	}))
	if err != nil {
		return "", err
	}
	var text strings.Builder
	for stream.Receive() {
		event := stream.Msg().GetEvent()
		if event == nil {
			continue
		}
		if event.GetText() != "" {
			text.WriteString(event.GetText())
		}
		if event.GetStatus() == "failed" {
			return text.String(), errors.New(firstNonEmpty(event.GetText(), "runner failed"))
		}
	}
	if err := stream.Err(); err != nil {
		return "", err
	}
	return strings.TrimSpace(text.String()), nil
}

func (s *Service) createLease(ctx context.Context, req conversation.ChatRequest, sessionType string) (*runnerv1.RunLease, error) {
	if s == nil {
		return nil, errors.New("runner dispatch service not configured")
	}
	if s.queries == nil {
		return nil, errors.New("runner dispatch queries not configured")
	}
	botID := strings.TrimSpace(req.BotID)
	if botID == "" {
		return nil, errors.New("bot id is required")
	}
	userID := strings.TrimSpace(req.UserID)
	if userID == "" {
		return nil, errors.New("user id is required")
	}
	sessionID, err := s.ensureSession(ctx, req, sessionType)
	if err != nil {
		return nil, err
	}
	runID := uuid.NewString()
	target := s.workspaceExecutorTarget(botID)
	if target == "" {
		return nil, errors.New("workspace executor target is required")
	}
	workspaceID := "workspace-" + botID
	expiresAt := s.now().Add(s.leaseTTL)
	pgRunID, err := db.ParseUUID(runID)
	if err != nil {
		return nil, err
	}
	pgBotID, err := db.ParseUUID(botID)
	if err != nil {
		return nil, err
	}
	pgSessionID, err := db.ParseUUID(sessionID)
	if err != nil {
		return nil, err
	}
	pgUserID, err := db.ParseUUID(userID)
	if err != nil {
		return nil, err
	}
	row, err := s.queries.CreateAgentRunLease(ctx, sqlc.CreateAgentRunLeaseParams{
		RunID:                     pgRunID,
		RunnerInstanceID:          s.runnerInstanceID,
		BotID:                     pgBotID,
		BotGroupID:                pgtype.UUID{},
		SessionID:                 pgSessionID,
		UserID:                    pgUserID,
		PermissionSnapshotVersion: 1,
		AllowedToolScopes: []string{
			serviceauth.ScopeRunnerRun,
			serviceauth.ScopeRunnerEvents,
			serviceauth.ScopeWorkspaceExec,
			serviceauth.ScopeWorkspaceFiles,
		},
		WorkspaceExecutorTarget: target,
		WorkspaceID:             workspaceID,
		ExpiresAt:               pgtype.Timestamptz{Time: expiresAt, Valid: true},
		LeaseVersion:            1,
	})
	if err != nil {
		return nil, err
	}
	return &runnerv1.RunLease{
		RunId:                     row.RunID.String(),
		RunnerInstanceId:          row.RunnerInstanceID,
		BotId:                     row.BotID.String(),
		BotGroupId:                row.BotGroupID.String(),
		SessionId:                 row.SessionID.String(),
		UserId:                    row.UserID.String(),
		PermissionSnapshotVersion: row.PermissionSnapshotVersion,
		AllowedToolScopes:         append([]string(nil), row.AllowedToolScopes...),
		WorkspaceExecutorTarget:   row.WorkspaceExecutorTarget,
		WorkspaceId:               row.WorkspaceID,
		ExpiresAt:                 timestamppb.New(row.ExpiresAt.Time),
		LeaseVersion:              row.LeaseVersion,
	}, nil
}

func (s *Service) ensureSession(ctx context.Context, req conversation.ChatRequest, sessionType string) (string, error) {
	if id := strings.TrimSpace(req.SessionID); id != "" {
		return id, nil
	}
	if s.sessions == nil {
		return "", errors.New("session service not configured")
	}
	sess, err := s.sessions.Create(ctx, sessionpkg.CreateInput{
		BotID:       strings.TrimSpace(req.BotID),
		RouteID:     strings.TrimSpace(req.RouteID),
		ChannelType: strings.TrimSpace(req.CurrentChannel),
		Type:        firstNonEmpty(sessionType, sessionpkg.TypeChat),
		Title:       strings.TrimSpace(req.Query),
	})
	if err != nil {
		return "", err
	}
	return sess.ID, nil
}

func (s *Service) workspaceExecutorTarget(botID string) string {
	if s.workspaceTargets == nil {
		return ""
	}
	return strings.TrimSpace(s.workspaceTargets.WorkspaceExecutorTarget(botID))
}

func (s *Service) streamEvents(ctx context.Context, lease *runnerv1.RunLease, chunkCh chan<- conversation.StreamChunk, errCh chan<- error) {
	stream, err := s.runner.StreamRunEvents(ctx, connect.NewRequest(&runnerv1.StreamRunEventsRequest{
		RunId:            lease.GetRunId(),
		RunnerInstanceId: lease.GetRunnerInstanceId(),
		LeaseVersion:     lease.GetLeaseVersion(),
	}))
	if err != nil {
		errCh <- err
		return
	}
	for stream.Receive() {
		for _, chunk := range chunksFromRunEvent(stream.Msg().GetEvent()) {
			select {
			case chunkCh <- chunk:
			case <-ctx.Done():
				errCh <- ctx.Err()
				return
			}
		}
	}
	if err := stream.Err(); err != nil {
		errCh <- err
	}
}

func (s *Service) resolveBotOwner(ctx context.Context, botID string) (string, error) {
	if s.queries == nil {
		return "", errors.New("runner dispatch queries not configured")
	}
	pgBotID, err := db.ParseUUID(botID)
	if err != nil {
		return "", err
	}
	bot, err := s.queries.GetBotByID(ctx, pgBotID)
	if err != nil {
		return "", fmt.Errorf("get bot: %w", err)
	}
	ownerID := bot.OwnerUserID.String()
	if ownerID == "" {
		return "", errors.New("bot owner not found")
	}
	return ownerID, nil
}

func (s *Service) generateToken(userID string) (string, error) {
	if s.jwtSecret == "" {
		return "", errors.New("jwt secret not configured")
	}
	signed, _, err := auth.GenerateToken(userID, s.jwtSecret, emailTriggerTokenTTL)
	if err != nil {
		return "", err
	}
	return "Bearer " + signed, nil
}

func attachmentIDs(items []conversation.ChatAttachment) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		if id := strings.TrimSpace(item.ContentHash); id != "" {
			out = append(out, id)
		}
	}
	return out
}

func runOptions(req conversation.ChatRequest, source string) *structpb.Struct {
	fields := map[string]any{
		"source":                 strings.TrimSpace(source),
		"current_channel":        strings.TrimSpace(req.CurrentChannel),
		"conversation_type":      strings.TrimSpace(req.ConversationType),
		"source_identity_id":     strings.TrimSpace(req.SourceChannelIdentityID),
		"reply_target":           strings.TrimSpace(req.ReplyTarget),
		"external_message_id":    strings.TrimSpace(req.ExternalMessageID),
		"model":                  strings.TrimSpace(req.Model),
		"provider":               strings.TrimSpace(req.Provider),
		"reasoning_effort":       strings.TrimSpace(req.ReasoningEffort),
		"user_message_persisted": req.UserMessagePersisted,
	}
	out, err := structpb.NewStruct(fields)
	if err != nil {
		return nil
	}
	return out
}

func chunksFromRunEvent(event *eventv1.AgentRunEvent) []conversation.StreamChunk {
	if event == nil {
		return nil
	}
	if payload := payloadChunk(event.GetPayload()); len(payload) > 0 {
		return []conversation.StreamChunk{payload}
	}
	switch strings.TrimSpace(event.GetEventType()) {
	case "run.started":
		return marshalChunks(map[string]any{"type": "agent_start"})
	case "run.completed":
		return marshalChunks(map[string]any{"type": "agent_end"})
	case "run.cancelled":
		return marshalChunks(map[string]any{"type": "agent_abort", "error": firstNonEmpty(event.GetText(), "run cancelled")})
	case "run.failed":
		return marshalChunks(
			map[string]any{"type": "error", "error": firstNonEmpty(event.GetText(), "runner failed")},
			map[string]any{"type": "agent_abort"},
		)
	default:
		if strings.TrimSpace(event.GetText()) != "" {
			return marshalChunks(map[string]any{"type": "text_delta", "delta": event.GetText()})
		}
	}
	return nil
}

func payloadChunk(payload *structpb.Struct) conversation.StreamChunk {
	if payload == nil {
		return nil
	}
	fields := payload.AsMap()
	if _, ok := fields["type"]; !ok {
		return nil
	}
	data, err := json.Marshal(fields)
	if err != nil {
		return nil
	}
	return conversation.StreamChunk(data)
}

func marshalChunks(items ...map[string]any) []conversation.StreamChunk {
	out := make([]conversation.StreamChunk, 0, len(items))
	for _, item := range items {
		data, err := json.Marshal(item)
		if err == nil {
			out = append(out, conversation.StreamChunk(data))
		}
	}
	return out
}

func parseConversationChunk(chunk conversation.StreamChunk) (string, []conversation.ModelMessage) {
	var envelope struct {
		Type     string                      `json:"type"`
		Delta    string                      `json:"delta"`
		Text     string                      `json:"text"`
		Messages []conversation.ModelMessage `json:"messages"`
	}
	if err := json.Unmarshal(chunk, &envelope); err != nil {
		return "", nil
	}
	switch envelope.Type {
	case "text_delta":
		return envelope.Delta, envelope.Messages
	default:
		return envelope.Text, envelope.Messages
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

var (
	_ schedule.Triggerer  = (*Service)(nil)
	_ heartbeat.Triggerer = (*Service)(nil)
	_ interface {
		TriggerBotChat(context.Context, string, string) error
	} = (*Service)(nil)
	_ interface {
		Chat(context.Context, conversation.ChatRequest) (conversation.ChatResponse, error)
		StreamChat(context.Context, conversation.ChatRequest) (<-chan conversation.StreamChunk, <-chan error)
		TriggerSchedule(context.Context, string, schedule.TriggerPayload, string) (schedule.TriggerResult, error)
	} = (*Service)(nil)
)
