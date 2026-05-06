package connectapi

import (
	"context"
	"crypto/ed25519"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"

	privatev1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1"
	"github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1/privatev1connect"
	"github.com/memohai/memoh/internal/serviceauth"
)

func TestInternalAuthIssueServiceToken(t *testing.T) {
	now := time.Date(2026, 5, 6, 10, 0, 0, 0, time.UTC)
	_, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	publicKey, err := serviceauth.PublicKeyFromPrivate(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := serviceauth.NewSigner("active", map[string]ed25519.PrivateKey{"active": privateKey})
	if err != nil {
		t.Fatal(err)
	}
	signer.SetNow(func() time.Time { return now })
	registration := serviceauth.NewRegistrationValidator("bootstrap")
	registration.SetNow(func() time.Time { return now })
	service := NewInternalAuthService(signer, registration, nil, nil)
	service.SetNow(func() time.Time { return now })

	resp, err := service.IssueServiceToken(context.Background(), IssueServiceTokenRequest{
		ServiceName:             serviceauth.AudienceConnector,
		InstanceID:              "connector-1",
		Audience:                serviceauth.AudienceServer,
		Scopes:                  []string{serviceauth.ScopeServerEvents},
		TTL:                     15 * time.Minute,
		BootstrapToken:          "bootstrap",
		BootstrapTokenExpiresAt: now.Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.ExpiresAt.Equal(now.Add(15 * time.Minute)) {
		t.Fatalf("expires_at = %s", resp.ExpiresAt)
	}
	verifier, err := serviceauth.NewVerifier(map[string]ed25519.PublicKey{"active": publicKey})
	if err != nil {
		t.Fatal(err)
	}
	verifier.SetNow(func() time.Time { return now.Add(time.Second) })
	claims, err := verifier.Verify(resp.Token)
	if err != nil {
		t.Fatal(err)
	}
	if err := serviceauth.RequireScope(claims, serviceauth.AudienceServer, serviceauth.ScopeServerEvents, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}

	_, err = service.IssueServiceToken(context.Background(), IssueServiceTokenRequest{
		InstanceID:              "connector-1",
		Audience:                serviceauth.AudienceServer,
		Scopes:                  []string{serviceauth.ScopeServerEvents},
		TTL:                     15*time.Minute + time.Second,
		BootstrapToken:          "bootstrap",
		BootstrapTokenExpiresAt: now.Add(time.Minute),
	})
	if err == nil {
		t.Fatal("ttl over 15m was accepted")
	}
}

func TestInternalAuthRegistrationFailures(t *testing.T) {
	now := time.Date(2026, 5, 6, 10, 0, 0, 0, time.UTC)
	_, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := serviceauth.NewSigner("active", map[string]ed25519.PrivateKey{"active": privateKey})
	if err != nil {
		t.Fatal(err)
	}
	registration := serviceauth.NewRegistrationValidator("bootstrap")
	registration.SetNow(func() time.Time { return now })
	service := NewInternalAuthService(signer, registration, nil, nil)
	service.SetNow(func() time.Time { return now })

	_, err = service.IssueServiceToken(context.Background(), IssueServiceTokenRequest{
		InstanceID:              "connector-1",
		Audience:                serviceauth.AudienceServer,
		Scopes:                  []string{serviceauth.ScopeServerEvents},
		BootstrapToken:          "wrong",
		BootstrapTokenExpiresAt: now.Add(time.Minute),
	})
	if !errors.Is(err, serviceauth.ErrPermissionDenied) {
		t.Fatalf("wrong bootstrap token error = %v", err)
	}
}

func TestInternalAuthConnectHandlerIssuesServiceToken(t *testing.T) {
	now := time.Date(2026, 5, 6, 10, 0, 0, 0, time.UTC)
	_, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	publicKey, err := serviceauth.PublicKeyFromPrivate(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := serviceauth.NewSigner("active", map[string]ed25519.PrivateKey{"active": privateKey})
	if err != nil {
		t.Fatal(err)
	}
	signer.SetNow(func() time.Time { return now })
	registration := serviceauth.NewRegistrationValidator("bootstrap")
	registration.SetNow(func() time.Time { return now })
	service := NewInternalAuthService(signer, registration, nil, nil)
	service.SetNow(func() time.Time { return now })
	client, cleanup := newInternalAuthTestClient(t, service)
	defer cleanup()

	resp, err := client.IssueServiceToken(context.Background(), connect.NewRequest(&privatev1.IssueServiceTokenRequest{
		CallerService:    serviceauth.AudienceConnector,
		CallerInstanceId: "connector-1",
		TargetAudience:   serviceauth.AudienceServer,
		Scopes:           []string{serviceauth.ScopeServerEvents},
		BootstrapToken:   "bootstrap",
		TtlSeconds:       60,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Msg.GetKeyId() != "active" {
		t.Fatalf("key_id = %q", resp.Msg.GetKeyId())
	}
	if resp.Msg.GetIssuer() != serviceauth.Issuer || resp.Msg.GetAudience() != serviceauth.AudienceServer {
		t.Fatalf("issuer/audience = %q/%q", resp.Msg.GetIssuer(), resp.Msg.GetAudience())
	}
	if !resp.Msg.GetIssuedAt().AsTime().Equal(now) || !resp.Msg.GetExpiresAt().AsTime().Equal(now.Add(time.Minute)) {
		t.Fatalf("issued/expires = %s/%s", resp.Msg.GetIssuedAt().AsTime(), resp.Msg.GetExpiresAt().AsTime())
	}
	verifier, err := serviceauth.NewVerifier(map[string]ed25519.PublicKey{"active": publicKey})
	if err != nil {
		t.Fatal(err)
	}
	verifier.SetNow(func() time.Time { return now.Add(time.Second) })
	claims, err := verifier.Verify(resp.Msg.GetToken())
	if err != nil {
		t.Fatal(err)
	}
	if err := serviceauth.RequireScope(claims, serviceauth.AudienceServer, serviceauth.ScopeServerEvents, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
}

func newInternalAuthTestClient(t *testing.T, service *InternalAuthService) (privatev1connect.InternalAuthServiceClient, func()) {
	t.Helper()
	_, handler := privatev1connect.NewInternalAuthServiceHandler(&internalAuthHandler{service: service})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.ServeHTTP(w, r)
	}))
	return privatev1connect.NewInternalAuthServiceClient(server.Client(), server.URL), server.Close
}
