package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"connectrpc.com/connect"

	privatev1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1"
	"github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1/privatev1connect"
	"github.com/memohai/memoh/internal/serviceauth"
)

type serviceTokenSource struct {
	client         privatev1connect.InternalAuthServiceClient
	bootstrapToken string
	instanceID     string
	now            func() time.Time

	mu        sync.Mutex
	token     string
	expiresAt time.Time
}

func newServiceTokenSource(serverURL string) (func(context.Context) (string, error), error) {
	if token := strings.TrimSpace(os.Getenv("MEMOH_SERVICE_TOKEN")); token != "" {
		return func(context.Context) (string, error) {
			return token, nil
		}, nil
	}
	bootstrapToken := strings.TrimSpace(os.Getenv("MEMOH_INTERNAL_AUTH_BOOTSTRAP_TOKEN"))
	if bootstrapToken == "" {
		return nil, errors.New("MEMOH_SERVICE_TOKEN or MEMOH_INTERNAL_AUTH_BOOTSTRAP_TOKEN is required")
	}
	instanceID := strings.TrimSpace(os.Getenv("MEMOH_INTEGRATION_GATEWAY_INSTANCE_ID"))
	if instanceID == "" {
		if hostname, err := os.Hostname(); err == nil {
			instanceID = hostname
		}
	}
	if instanceID == "" {
		instanceID = "integration-gateway"
	}
	source := &serviceTokenSource{
		client:         privatev1connect.NewInternalAuthServiceClient(http.DefaultClient, strings.TrimRight(serverURL, "/")),
		bootstrapToken: bootstrapToken,
		instanceID:     instanceID,
		now:            time.Now,
	}
	return source.Token, nil
}

func (s *serviceTokenSource) Token(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now().UTC()
	if s.token != "" && now.Before(s.expiresAt.Add(-time.Minute)) {
		return s.token, nil
	}
	resp, err := s.client.IssueServiceToken(ctx, connect.NewRequest(&privatev1.IssueServiceTokenRequest{
		CallerService:    serviceauth.AudienceIntegrationGateway,
		CallerInstanceId: s.instanceID,
		TargetAudience:   serviceauth.AudienceServer,
		Scopes:           []string{serviceauth.ScopeIntegrationGateway},
		BootstrapToken:   s.bootstrapToken,
		TtlSeconds:       int32(serviceauth.MaxServiceTokenTTL / time.Second),
	}))
	if err != nil {
		return "", err
	}
	token := strings.TrimSpace(resp.Msg.GetToken())
	if token == "" {
		return "", errors.New("internal auth returned empty service token")
	}
	s.token = token
	if resp.Msg.GetExpiresAt() != nil {
		s.expiresAt = resp.Msg.GetExpiresAt().AsTime()
	} else {
		s.expiresAt = now.Add(serviceauth.MaxServiceTokenTTL)
	}
	return s.token, nil
}
