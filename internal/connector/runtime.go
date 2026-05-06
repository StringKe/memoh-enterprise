package connector

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/memohai/memoh/internal/channel"
)

type LeaseRenewer interface {
	RenewLease(ctx context.Context, token LeaseToken) (LeaseToken, error)
	ReleaseLease(ctx context.Context, token LeaseToken) error
}

type InboundClient interface {
	SendInbound(ctx context.Context, event InboundEvent) error
}

type Runtime struct {
	manager       *channel.Manager
	renewer       LeaseRenewer
	inbound       InboundClient
	renewInterval time.Duration
	logger        *slog.Logger

	mu      sync.RWMutex
	token   LeaseToken
	config  channel.ChannelConfig
	cancel  context.CancelFunc
	stopped chan struct{}
}

type RuntimeOption func(*Runtime)

func WithRenewInterval(interval time.Duration) RuntimeOption {
	return func(r *Runtime) {
		if interval > 0 {
			r.renewInterval = interval
		}
	}
}

func NewRuntime(log *slog.Logger, registry channel.AdapterRegistry, renewer LeaseRenewer, inbound InboundClient, opts ...RuntimeOption) *Runtime {
	if log == nil {
		log = slog.Default()
	}
	if registry == nil {
		registry = channel.NewRegistry()
	}
	r := &Runtime{
		renewer:       renewer,
		inbound:       inbound,
		renewInterval: DefaultRenewInterval,
		logger:        log.With(slog.String("component", "connector_runtime")),
	}
	for _, opt := range opts {
		opt(r)
	}
	r.manager = channel.NewManager(log, registry, nil, nil, channel.WithInboundSink(r))
	return r
}

func (r *Runtime) Start(ctx context.Context, token LeaseToken, cfg channel.ChannelConfig) error {
	if r.renewer == nil {
		return errors.New("connector lease renewer not configured")
	}
	if err := validateRuntimeStart(token, cfg); err != nil {
		return err
	}
	r.mu.Lock()
	if r.cancel != nil {
		r.mu.Unlock()
		return errors.New("connector runtime already started")
	}
	runCtx, cancel := context.WithCancel(ctx)
	r.token = token
	r.config = cfg
	r.cancel = cancel
	r.stopped = make(chan struct{})
	stopped := r.stopped
	r.mu.Unlock()

	if err := r.manager.EnsureConnection(runCtx, cfg); err != nil {
		cancel()
		r.clearRunning()
		return err
	}
	go r.renewLoop(runCtx, cfg.ID, stopped)
	return nil
}

func (r *Runtime) Stop(ctx context.Context) error {
	r.mu.Lock()
	cancel := r.cancel
	cfg := r.config
	r.cancel = nil
	r.config = channel.ChannelConfig{}
	r.token = LeaseToken{}
	r.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if strings.TrimSpace(cfg.ID) == "" {
		return nil
	}
	return r.manager.Stop(ctx, cfg.ID)
}

func (r *Runtime) HandleInbound(ctx context.Context, cfg channel.ChannelConfig, msg channel.InboundMessage) error {
	if r.inbound == nil {
		return errors.New("connector inbound client not configured")
	}
	token := r.currentToken()
	if err := validateRuntimeStart(token, cfg); err != nil {
		return err
	}
	return r.inbound.SendInbound(ctx, InboundEvent{
		Token:      token,
		Config:     cfg,
		Message:    msg,
		ReceivedAt: time.Now().UTC(),
	})
}

func (r *Runtime) renewLoop(ctx context.Context, configID string, stopped chan struct{}) {
	defer close(stopped)
	ticker := time.NewTicker(r.renewInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			token := r.currentToken()
			renewed, err := r.renewer.RenewLease(ctx, token)
			if err != nil {
				if r.logger != nil {
					r.logger.Warn("lease renew failed", slog.String("config_id", configID), slog.Any("error", err))
				}
				_ = r.manager.Stop(context.WithoutCancel(ctx), configID)
				r.clearRunning()
				return
			}
			r.setToken(renewed)
		}
	}
}

func (r *Runtime) currentToken() LeaseToken {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.token
}

func (r *Runtime) setToken(token LeaseToken) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.token = token
}

func (r *Runtime) clearRunning() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cancel = nil
	r.config = channel.ChannelConfig{}
	r.token = LeaseToken{}
}

func validateRuntimeStart(token LeaseToken, cfg channel.ChannelConfig) error {
	if err := validateTokenShape(token); err != nil {
		return err
	}
	if strings.TrimSpace(cfg.ID) == "" {
		return errors.New("channel config id is required")
	}
	if strings.TrimSpace(cfg.ID) != strings.TrimSpace(token.ChannelConfigID) {
		return ErrInvalidLeaseToken
	}
	if cfg.ChannelType != "" && token.ChannelType != "" && cfg.ChannelType != token.ChannelType {
		return ErrInvalidLeaseToken
	}
	return nil
}
