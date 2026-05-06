package worker

import (
	"context"
	"errors"
	"log/slog"
	"sync"
)

type Runtime struct {
	deps   Dependencies
	cancel context.CancelFunc
	done   chan error
	mu     sync.Mutex
}

func NewRuntime(deps Dependencies) *Runtime {
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}
	return &Runtime{deps: deps}
}

func (r *Runtime) Start(ctx context.Context) error {
	if r == nil || r.deps.Consumer == nil {
		return errors.New("worker event consumer is not configured")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cancel != nil {
		return errors.New("worker runtime already started")
	}
	r.registerHandlers()
	runCtx, cancel := context.WithCancel(ctx)
	r.cancel = cancel
	done := make(chan error, 1)
	r.done = done
	go func() {
		err := r.deps.Consumer.Run(runCtx)
		if errors.Is(err, context.Canceled) {
			err = nil
		}
		done <- err
	}()
	return nil
}

func (r *Runtime) Stop(ctx context.Context) error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	cancel := r.cancel
	done := r.done
	r.cancel = nil
	r.done = nil
	r.mu.Unlock()
	if cancel == nil || done == nil {
		return nil
	}
	cancel()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *Runtime) Run(ctx context.Context) error {
	if err := r.Start(ctx); err != nil {
		return err
	}
	<-ctx.Done()
	return r.Stop(context.WithoutCancel(ctx))
}

func (r *Runtime) registerHandlers() {
	r.deps.Consumer.RegisterHandler(TopicScheduleStartup, r.handleScheduleStartup)
	r.deps.Consumer.RegisterHandler(TopicHeartbeatStartup, r.handleHeartbeatStartup)
	r.deps.Consumer.RegisterHandler(TopicCompactionRun, r.handleCompactionRun)
	r.deps.Consumer.RegisterHandler(TopicCleanupRun, r.handleCleanupRun)
}
