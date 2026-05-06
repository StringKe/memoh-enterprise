package connector

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/memohai/memoh/internal/channel"
)

type LeaseStore interface {
	RegisterInstance(ctx context.Context, instance Instance) error
	AcquireLease(ctx context.Context, req AcquireLeaseRequest, now time.Time, ttl time.Duration) (LeaseToken, error)
	RenewLease(ctx context.Context, token LeaseToken, now time.Time, ttl time.Duration) (LeaseToken, error)
	ReleaseLease(ctx context.Context, token LeaseToken, now time.Time) error
	GetLease(ctx context.Context, channelConfigID string) (LeaseToken, bool, error)
	SaveStatus(ctx context.Context, status Status) error
}

type InboundSink interface {
	AcceptInbound(ctx context.Context, event InboundEvent) error
}

type Service struct {
	store    LeaseStore
	inbound  InboundSink
	leaseTTL time.Duration
	now      func() time.Time
}

type ServiceOption func(*Service)

func WithInboundSink(sink InboundSink) ServiceOption {
	return func(s *Service) {
		s.inbound = sink
	}
}

func WithClock(now func() time.Time) ServiceOption {
	return func(s *Service) {
		if now != nil {
			s.now = now
		}
	}
}

func WithLeaseTTL(ttl time.Duration) ServiceOption {
	return func(s *Service) {
		if ttl > 0 {
			s.leaseTTL = ttl
		}
	}
}

func NewService(store LeaseStore, opts ...ServiceOption) *Service {
	if store == nil {
		store = NewMemoryLeaseStore()
	}
	s := &Service{
		store:    store,
		leaseTTL: DefaultLeaseTTL,
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *Service) Register(ctx context.Context, req RegisterRequest) (Instance, error) {
	instance := Instance{
		OwnerID:         strings.TrimSpace(req.OwnerID),
		OwnerInstanceID: strings.TrimSpace(req.OwnerInstanceID),
		ChannelType:     channel.ChannelType(strings.TrimSpace(req.ChannelType.String())),
		RegisteredAt:    s.now().UTC(),
		Metadata:        cloneStringMap(req.Metadata),
	}
	if instance.OwnerID == "" || instance.OwnerInstanceID == "" || instance.ChannelType == "" {
		return Instance{}, ErrInvalidLeaseToken
	}
	if err := s.store.RegisterInstance(ctx, instance); err != nil {
		return Instance{}, err
	}
	return instance, nil
}

func (s *Service) AcquireLease(ctx context.Context, req AcquireLeaseRequest) (LeaseToken, error) {
	req.ChannelConfigID = strings.TrimSpace(req.ChannelConfigID)
	req.OwnerID = strings.TrimSpace(req.OwnerID)
	req.OwnerInstanceID = strings.TrimSpace(req.OwnerInstanceID)
	req.ChannelType = channel.ChannelType(strings.TrimSpace(req.ChannelType.String()))
	if req.ChannelConfigID == "" || req.OwnerID == "" || req.OwnerInstanceID == "" || req.ChannelType == "" {
		return LeaseToken{}, ErrInvalidLeaseToken
	}
	return s.store.AcquireLease(ctx, req, s.now().UTC(), s.leaseTTL)
}

func (s *Service) RenewLease(ctx context.Context, token LeaseToken) (LeaseToken, error) {
	if err := validateTokenShape(token); err != nil {
		return LeaseToken{}, err
	}
	return s.store.RenewLease(ctx, token, s.now().UTC(), s.leaseTTL)
}

func (s *Service) ReleaseLease(ctx context.Context, token LeaseToken) error {
	if err := validateTokenShape(token); err != nil {
		return err
	}
	return s.store.ReleaseLease(ctx, token, s.now().UTC())
}

func (s *Service) GetLease(ctx context.Context, channelConfigID string) (LeaseToken, bool, error) {
	return s.store.GetLease(ctx, strings.TrimSpace(channelConfigID))
}

func (s *Service) ValidateLease(ctx context.Context, token LeaseToken) error {
	if err := validateTokenShape(token); err != nil {
		return err
	}
	current, ok, err := s.store.GetLease(ctx, token.ChannelConfigID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrLeaseNotFound
	}
	if token.ChannelType != "" && current.ChannelType != token.ChannelType {
		return ErrLeaseStale
	}
	if current.OwnerInstanceID != token.OwnerInstanceID || current.LeaseVersion != token.LeaseVersion {
		return ErrLeaseStale
	}
	if !current.ExpiresAt.After(s.now().UTC()) {
		return ErrLeaseExpired
	}
	return nil
}

func (s *Service) ReportStatus(ctx context.Context, status Status) error {
	if err := s.ValidateLease(ctx, status.Token); err != nil {
		return err
	}
	status.UpdatedAt = s.now().UTC()
	if status.State == "" {
		status.State = StatusStateUnknown
	}
	return s.store.SaveStatus(ctx, status)
}

func (s *Service) AcceptInbound(ctx context.Context, event InboundEvent) error {
	if err := s.ValidateLease(ctx, event.Token); err != nil {
		return err
	}
	if strings.TrimSpace(event.Config.ID) != event.Token.ChannelConfigID {
		return ErrInvalidLeaseToken
	}
	if event.Config.ChannelType != "" && event.Config.ChannelType != event.Token.ChannelType {
		return ErrInvalidLeaseToken
	}
	if event.ReceivedAt.IsZero() {
		event.ReceivedAt = s.now().UTC()
	}
	if s.inbound == nil {
		return nil
	}
	return s.inbound.AcceptInbound(ctx, event)
}

func validateTokenShape(token LeaseToken) error {
	if strings.TrimSpace(token.ChannelConfigID) == "" ||
		strings.TrimSpace(token.OwnerInstanceID) == "" ||
		token.LeaseVersion <= 0 {
		return ErrInvalidLeaseToken
	}
	return nil
}

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]string, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

type MemoryLeaseStore struct {
	mu        sync.Mutex
	leases    map[string]LeaseToken
	instances map[string]Instance
	statuses  map[string]Status
}

func NewMemoryLeaseStore() *MemoryLeaseStore {
	return &MemoryLeaseStore{
		leases:    map[string]LeaseToken{},
		instances: map[string]Instance{},
		statuses:  map[string]Status{},
	}
}

func (s *MemoryLeaseStore) RegisterInstance(_ context.Context, instance Instance) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.instances[instance.OwnerInstanceID] = instance
	return nil
}

func (s *MemoryLeaseStore) AcquireLease(_ context.Context, req AcquireLeaseRequest, now time.Time, ttl time.Duration) (LeaseToken, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, exists := s.leases[req.ChannelConfigID]
	if exists && current.ExpiresAt.After(now) {
		return LeaseToken{}, ErrLeaseHeld
	}
	version := int64(1)
	if exists {
		version = current.LeaseVersion + 1
	}
	token := LeaseToken{
		ChannelConfigID: req.ChannelConfigID,
		ChannelType:     req.ChannelType,
		OwnerID:         req.OwnerID,
		OwnerInstanceID: req.OwnerInstanceID,
		LeaseVersion:    version,
		AcquiredAt:      now,
		ExpiresAt:       now.Add(ttl),
	}
	s.leases[req.ChannelConfigID] = token
	return token, nil
}

func (s *MemoryLeaseStore) RenewLease(_ context.Context, token LeaseToken, now time.Time, ttl time.Duration) (LeaseToken, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, exists := s.leases[token.ChannelConfigID]
	if !exists {
		return LeaseToken{}, ErrLeaseNotFound
	}
	if current.OwnerInstanceID != token.OwnerInstanceID || current.LeaseVersion != token.LeaseVersion {
		return LeaseToken{}, ErrLeaseStale
	}
	if !current.ExpiresAt.After(now) {
		return LeaseToken{}, ErrLeaseExpired
	}
	current.ExpiresAt = now.Add(ttl)
	s.leases[token.ChannelConfigID] = current
	return current, nil
}

func (s *MemoryLeaseStore) ReleaseLease(_ context.Context, token LeaseToken, _ time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, exists := s.leases[token.ChannelConfigID]
	if !exists {
		return ErrLeaseNotFound
	}
	if current.OwnerInstanceID != token.OwnerInstanceID || current.LeaseVersion != token.LeaseVersion {
		return ErrLeaseStale
	}
	delete(s.leases, token.ChannelConfigID)
	return nil
}

func (s *MemoryLeaseStore) GetLease(_ context.Context, channelConfigID string) (LeaseToken, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	token, ok := s.leases[strings.TrimSpace(channelConfigID)]
	return token, ok, nil
}

func (s *MemoryLeaseStore) SaveStatus(_ context.Context, status Status) error {
	if err := validateTokenShape(status.Token); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.statuses[status.Token.ChannelConfigID] = status
	return nil
}

func (s *MemoryLeaseStore) Status(channelConfigID string) (Status, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	status, ok := s.statuses[strings.TrimSpace(channelConfigID)]
	return status, ok
}

func IsLeaseFenceError(err error) bool {
	return errors.Is(err, ErrLeaseNotFound) ||
		errors.Is(err, ErrLeaseExpired) ||
		errors.Is(err, ErrLeaseStale)
}
