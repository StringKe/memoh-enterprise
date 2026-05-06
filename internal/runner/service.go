package runner

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	eventv1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/event/v1"
	runnerv1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/runner/v1"
	"github.com/memohai/memoh/internal/connectapi/gen/memoh/runner/v1/runnerv1connect"
)

type ServiceDeps struct {
	SupportClient runnerv1connect.RunnerSupportServiceClient
	ContextClient *ContextClient
	Workspace     *WorkspaceClient
	Provider      *ProviderClient
	Memory        *MemoryClient
	ToolApproval  *ToolApprovalClient
	Browser       *BrowserClient
	Executor      Executor
	Logger        *slog.Logger
	Clock         func() time.Time
}

type Service struct {
	support      runnerv1connect.RunnerSupportServiceClient
	context      *ContextClient
	workspace    *WorkspaceClient
	provider     *ProviderClient
	memory       *MemoryClient
	toolApproval *ToolApprovalClient
	browser      *BrowserClient
	executor     Executor
	logger       *slog.Logger
	clock        func() time.Time

	mu   sync.Mutex
	runs map[string]*runState
}

type runState struct {
	lease        RunLease
	status       string
	nextEventSeq int64
	events       []*eventv1.AgentRunEvent
	subscribers  map[chan *eventv1.AgentRunEvent]struct{}
}

func NewService(deps ServiceDeps) *Service {
	clock := deps.Clock
	if clock == nil {
		clock = func() time.Time { return time.Now().UTC() }
	}
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}
	executor := deps.Executor
	if executor == nil {
		executor = NewAgentExecutor(AgentExecutorDeps{
			Logger:       logger,
			Workspace:    deps.Workspace,
			Provider:     deps.Provider,
			Memory:       deps.Memory,
			ToolApproval: deps.ToolApproval,
			Browser:      deps.Browser,
		})
	}
	return &Service{
		support:      deps.SupportClient,
		context:      firstContextClient(deps.ContextClient, deps.SupportClient),
		workspace:    deps.Workspace,
		provider:     deps.Provider,
		memory:       deps.Memory,
		toolApproval: deps.ToolApproval,
		browser:      deps.Browser,
		executor:     executor,
		logger:       logger,
		clock:        clock,
		runs:         make(map[string]*runState),
	}
}

func firstContextClient(client *ContextClient, support runnerv1connect.RunnerSupportServiceClient) *ContextClient {
	if client != nil {
		return client
	}
	if support == nil {
		return nil
	}
	return NewContextClient(support)
}

func (s *Service) StartRun(ctx context.Context, req *connect.Request[runnerv1.StartRunRequest]) (*connect.Response[runnerv1.StartRunResponse], error) {
	runReq, err := RunRequestFromProto(req.Msg)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	lease := runReq.Lease
	ref := lease.Ref()

	s.mu.Lock()
	if existing, ok := s.runs[lease.RunID]; ok {
		if !existing.lease.MatchesRef(ref) {
			s.mu.Unlock()
			return nil, connect.NewError(connect.CodePermissionDenied, ErrRunRefMismatch)
		}
		status := existing.status
		s.mu.Unlock()
		return connect.NewResponse(&runnerv1.StartRunResponse{RunId: lease.RunID, Status: status}), nil
	}
	s.runs[lease.RunID] = &runState{
		lease:       lease,
		status:      RunStatusRunning,
		subscribers: make(map[chan *eventv1.AgentRunEvent]struct{}),
	}
	s.mu.Unlock()

	if err := s.PublishRunEvent(ctx, ref, RunEvent{EventType: RunEventStarted, Status: RunStatusRunning}.ProtoForLease(lease)); err != nil {
		return nil, err
	}
	if req.Msg.GetOptions() != nil {
		go s.executeRun(context.WithoutCancel(ctx), runReq)
	}
	return connect.NewResponse(&runnerv1.StartRunResponse{RunId: lease.RunID, Status: RunStatusRunning}), nil
}

func (s *Service) executeRun(ctx context.Context, req RunRequest) {
	ref := req.Lease.Ref()
	if s.support == nil {
		_ = s.AcknowledgeCompletion(ctx, ref, RunStatusFailed, "agent runner support client is not configured")
		return
	}
	contextClient := s.context
	if contextClient == nil {
		_ = s.AcknowledgeCompletion(ctx, ref, RunStatusFailed, "agent runner context client is not configured")
		return
	}
	if _, err := contextClient.ValidateRunLease(ctx, req.Lease); err != nil {
		_ = s.AcknowledgeCompletion(ctx, ref, RunStatusFailed, err.Error())
		return
	}
	resolved, err := contextClient.ResolveRunContext(ctx, req.Lease)
	if err != nil {
		_ = s.AcknowledgeCompletion(ctx, ref, RunStatusFailed, err.Error())
		return
	}
	history, err := contextClient.ReadSessionHistory(ctx, req.Lease, 50, "")
	if err != nil {
		_ = s.AcknowledgeCompletion(ctx, ref, RunStatusFailed, err.Error())
		return
	}
	if s.executor == nil {
		_ = s.AcknowledgeCompletion(ctx, ref, RunStatusFailed, "agent runner executor is not configured")
		return
	}
	result, err := s.executor.Execute(ctx, ExecutionInput{
		Request: req,
		Context: resolved,
		History: history,
		Emit: func(event *eventv1.AgentRunEvent) error {
			return s.PublishRunEvent(ctx, ref, event)
		},
	})
	if err != nil {
		_ = s.AcknowledgeCompletion(ctx, ref, RunStatusFailed, err.Error())
		return
	}
	if text := result.AssistantText; text != "" {
		if _, err := s.support.AppendSessionMessage(ctx, connect.NewRequest(&runnerv1.AppendSessionMessageRequest{
			Ref: req.Lease.Ref().Proto(),
			Message: &runnerv1.SessionMessage{
				SessionId: req.Lease.SessionID,
				BotId:     req.Lease.BotID,
				Role:      "assistant",
				Text:      text,
			},
		})); err != nil {
			_ = s.AcknowledgeCompletion(ctx, ref, RunStatusFailed, err.Error())
			return
		}
	}
	status := result.Status
	if status == "" {
		status = RunStatusCompleted
	}
	_ = s.AcknowledgeCompletion(ctx, ref, status, result.AssistantText)
}

func (s *Service) CancelRun(ctx context.Context, req *connect.Request[runnerv1.CancelRunRequest]) (*connect.Response[runnerv1.CancelRunResponse], error) {
	if req.Msg == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, ErrInvalidRunLease)
	}
	ref := RunRef{
		RunID:            req.Msg.GetRunId(),
		RunnerInstanceID: req.Msg.GetRunnerInstanceId(),
		LeaseVersion:     req.Msg.GetLeaseVersion(),
	}
	if err := ref.Validate(); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	status, err := s.statusForRef(ref)
	if err != nil {
		return nil, connectCodeError(err)
	}
	if terminalStatus(status) {
		return connect.NewResponse(&runnerv1.CancelRunResponse{RunId: ref.RunID, Status: status}), nil
	}
	if err := s.AcknowledgeCompletion(ctx, ref, RunStatusCancelled, req.Msg.GetReason()); err != nil {
		return nil, err
	}
	return connect.NewResponse(&runnerv1.CancelRunResponse{RunId: ref.RunID, Status: RunStatusCancelled}), nil
}

func (s *Service) StreamRunEvents(ctx context.Context, req *connect.Request[runnerv1.StreamRunEventsRequest], stream *connect.ServerStream[runnerv1.StreamRunEventsResponse]) error {
	if req.Msg == nil {
		return connect.NewError(connect.CodeInvalidArgument, ErrInvalidRunLease)
	}
	ref := RunRef{
		RunID:            req.Msg.GetRunId(),
		RunnerInstanceID: req.Msg.GetRunnerInstanceId(),
		LeaseVersion:     req.Msg.GetLeaseVersion(),
	}
	snapshot, ch, unsubscribe, terminal, err := s.subscribe(ref)
	if err != nil {
		return connectCodeError(err)
	}
	defer unsubscribe()

	for _, event := range snapshot {
		if err := stream.Send(&runnerv1.StreamRunEventsResponse{Event: event}); err != nil {
			return err
		}
	}
	if terminal {
		return nil
	}
	for {
		select {
		case event, ok := <-ch:
			if !ok {
				return nil
			}
			if err := stream.Send(&runnerv1.StreamRunEventsResponse{Event: event}); err != nil {
				return err
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (s *Service) PublishRunEvent(ctx context.Context, ref RunRef, event *eventv1.AgentRunEvent) error {
	if event == nil {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("run event is nil"))
	}
	return s.publish(ctx, ref, event, "")
}

func (s *Service) AcknowledgeCompletion(ctx context.Context, ref RunRef, status string, text string) error {
	if !terminalStatus(status) {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("status %q is not terminal", status))
	}
	eventType := RunEventCompleted
	if status == RunStatusCancelled {
		eventType = RunEventCancelled
	}
	if status == RunStatusFailed {
		eventType = RunEventFailed
	}
	lease, err := s.leaseForRef(ref)
	if err != nil {
		return connectCodeError(err)
	}
	event := RunEvent{EventType: eventType, Status: status, Text: text}.ProtoForLease(lease)
	return s.publish(ctx, ref, event, status)
}

func (s *Service) publish(ctx context.Context, ref RunRef, event *eventv1.AgentRunEvent, terminal string) error {
	prepared, lease, err := s.prepareEvent(ref, event, terminal)
	if err != nil {
		return connectCodeError(err)
	}
	if s.support != nil {
		_, err := s.support.AppendRunEvent(ctx, connect.NewRequest(&runnerv1.AppendRunEventRequest{
			Ref:   lease.Ref().Proto(),
			Event: prepared,
		}))
		if err != nil {
			return err
		}
	}
	return s.commitEvent(ctx, ref, prepared, terminal)
}

func (s *Service) prepareEvent(ref RunRef, event *eventv1.AgentRunEvent, terminal string) (*eventv1.AgentRunEvent, RunLease, error) {
	if err := ref.Validate(); err != nil {
		return nil, RunLease{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	state, ok := s.runs[ref.RunID]
	if !ok {
		return nil, RunLease{}, ErrRunNotFound
	}
	if !state.lease.MatchesRef(ref) {
		return nil, RunLease{}, ErrRunRefMismatch
	}
	if terminalStatus(state.status) {
		return nil, RunLease{}, ErrRunAlreadyTerminal
	}
	state.nextEventSeq++
	if event == nil {
		event = &eventv1.AgentRunEvent{}
	}
	prepared := proto.Clone(event).(*eventv1.AgentRunEvent)
	if prepared.EventId == "" {
		prepared.EventId = fmt.Sprintf("%s-%d", ref.RunID, state.nextEventSeq)
	}
	prepared.RunId = state.lease.RunID
	prepared.BotId = state.lease.BotID
	prepared.BotGroupId = state.lease.BotGroupID
	prepared.SessionId = state.lease.SessionID
	prepared.UserId = state.lease.UserID
	if prepared.OccurredAt == nil {
		prepared.OccurredAt = timestamppb.New(s.clock())
	}
	if terminal != "" {
		prepared.Status = terminal
	}
	return prepared, state.lease, nil
}

func (s *Service) commitEvent(ctx context.Context, ref RunRef, event *eventv1.AgentRunEvent, terminal string) error {
	s.mu.Lock()
	state, ok := s.runs[ref.RunID]
	if !ok {
		s.mu.Unlock()
		return ErrRunNotFound
	}
	if !state.lease.MatchesRef(ref) {
		s.mu.Unlock()
		return ErrRunRefMismatch
	}
	state.events = append(state.events, event)
	if terminal != "" {
		state.status = terminal
	}
	subscribers := make([]chan *eventv1.AgentRunEvent, 0, len(state.subscribers))
	for ch := range state.subscribers {
		subscribers = append(subscribers, ch)
	}
	s.mu.Unlock()

	for _, ch := range subscribers {
		select {
		case ch <- event:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if terminal != "" {
		s.closeSubscribers(ref)
	}
	return nil
}

func (s *Service) subscribe(ref RunRef) ([]*eventv1.AgentRunEvent, <-chan *eventv1.AgentRunEvent, func(), bool, error) {
	if err := ref.Validate(); err != nil {
		return nil, nil, nil, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	state, ok := s.runs[ref.RunID]
	if !ok {
		return nil, nil, nil, false, ErrRunNotFound
	}
	if !state.lease.MatchesRef(ref) {
		return nil, nil, nil, false, ErrRunRefMismatch
	}
	snapshot := append([]*eventv1.AgentRunEvent(nil), state.events...)
	if terminalStatus(state.status) {
		return snapshot, nil, func() {}, true, nil
	}
	ch := make(chan *eventv1.AgentRunEvent, 64)
	state.subscribers[ch] = struct{}{}
	unsubscribe := func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if current, ok := s.runs[ref.RunID]; ok {
			if _, ok := current.subscribers[ch]; ok {
				delete(current.subscribers, ch)
				close(ch)
			}
		}
	}
	return snapshot, ch, unsubscribe, false, nil
}

func (s *Service) closeSubscribers(ref RunRef) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.runs[ref.RunID]
	if !ok {
		return
	}
	for ch := range state.subscribers {
		close(ch)
		delete(state.subscribers, ch)
	}
}

func (s *Service) statusForRef(ref RunRef) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.runs[ref.RunID]
	if !ok {
		return "", ErrRunNotFound
	}
	if !state.lease.MatchesRef(ref) {
		return "", ErrRunRefMismatch
	}
	return state.status, nil
}

func (s *Service) leaseForRef(ref RunRef) (RunLease, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.runs[ref.RunID]
	if !ok {
		return RunLease{}, ErrRunNotFound
	}
	if !state.lease.MatchesRef(ref) {
		return RunLease{}, ErrRunRefMismatch
	}
	return state.lease, nil
}

func connectCodeError(err error) error {
	switch {
	case errors.Is(err, ErrRunNotFound):
		return connect.NewError(connect.CodeNotFound, err)
	case errors.Is(err, ErrRunRefMismatch):
		return connect.NewError(connect.CodePermissionDenied, err)
	case errors.Is(err, ErrRunAlreadyTerminal):
		return connect.NewError(connect.CodeFailedPrecondition, err)
	case errors.Is(err, ErrInvalidRunLease):
		return connect.NewError(connect.CodeInvalidArgument, err)
	default:
		return err
	}
}
