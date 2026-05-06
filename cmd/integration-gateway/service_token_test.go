package main

import (
	"context"
	"crypto/ed25519"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	privatev1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1"
	"github.com/memohai/memoh/internal/serviceauth"
)

func TestServiceTokenSourceUsesStaticToken(t *testing.T) {
	t.Setenv("MEMOH_SERVICE_TOKEN", "static-token")
	t.Setenv("MEMOH_INTERNAL_AUTH_BOOTSTRAP_TOKEN", "")

	source, err := newServiceTokenSource("http://server/connect")
	if err != nil {
		t.Fatal(err)
	}
	token, err := source(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if token != "static-token" {
		t.Fatalf("token = %q, want static-token", token)
	}
}

func TestServiceTokenSourceBootstrapsAndCachesToken(t *testing.T) {
	t.Setenv("MEMOH_SERVICE_TOKEN", "")
	t.Setenv("MEMOH_INTERNAL_AUTH_BOOTSTRAP_TOKEN", "bootstrap")
	t.Setenv("MEMOH_INTEGRATION_GATEWAY_INSTANCE_ID", "gateway-1")

	_, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := serviceauth.NewSigner("active", map[string]ed25519.PrivateKey{"active": privateKey})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	issued := 0
	client := fakeInternalAuthClient{issue: func(_ context.Context, req *connect.Request[privatev1.IssueServiceTokenRequest]) (*connect.Response[privatev1.IssueServiceTokenResponse], error) {
		issued++
		if req.Msg.GetCallerService() != serviceauth.AudienceIntegrationGateway || req.Msg.GetCallerInstanceId() != "gateway-1" {
			t.Fatalf("request = %#v", req.Msg)
		}
		token, err := signer.Sign(serviceauth.Claims{
			Audience:  serviceauth.AudienceServer,
			Subject:   serviceauth.AudienceIntegrationGateway,
			Scopes:    []string{serviceauth.ScopeIntegrationGateway},
			ExpiresAt: now.Add(serviceauth.MaxServiceTokenTTL),
		})
		if err != nil {
			t.Fatal(err)
		}
		return connect.NewResponse(&privatev1.IssueServiceTokenResponse{
			Token:     token,
			ExpiresAt: timestamppb.New(now.Add(serviceauth.MaxServiceTokenTTL)),
		}), nil
	}}
	source := &serviceTokenSource{
		client:         client,
		bootstrapToken: "bootstrap",
		instanceID:     "gateway-1",
		now:            func() time.Time { return now },
	}

	first, err := source.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	second, err := source.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if first == "" || second != first {
		t.Fatalf("first=%q second=%q", first, second)
	}
	if issued != 1 {
		t.Fatalf("issued = %d, want 1", issued)
	}
}

type fakeInternalAuthClient struct {
	issue func(context.Context, *connect.Request[privatev1.IssueServiceTokenRequest]) (*connect.Response[privatev1.IssueServiceTokenResponse], error)
}

func (f fakeInternalAuthClient) IssueServiceToken(ctx context.Context, req *connect.Request[privatev1.IssueServiceTokenRequest]) (*connect.Response[privatev1.IssueServiceTokenResponse], error) {
	return f.issue(ctx, req)
}
