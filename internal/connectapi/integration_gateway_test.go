package connectapi

import (
	"context"
	"crypto/ed25519"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/integrations"
	"github.com/memohai/memoh/internal/serviceauth"
)

func TestIntegrationGatewayServiceRequiresServiceToken(t *testing.T) {
	t.Parallel()

	_, verifier := newIntegrationGatewayServiceAuth(t)
	service := NewIntegrationGatewayServiceWithBackend(&fakeIntegrationGatewayBackend{}, integrations.NewHub(), nil, verifier)
	client, cleanup := newIntegrationGatewayTestClient(t, service, "")
	defer cleanup()

	_, err := client.ValidateToken(context.Background(), "raw-token")
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("ValidateToken error code = %v, want unauthenticated", connect.CodeOf(err))
	}
}

func TestIntegrationGatewayServiceUsesServiceTokenAndBackend(t *testing.T) {
	t.Parallel()

	serviceToken, verifier := newIntegrationGatewayServiceAuth(t)
	apiToken := integrations.APIToken{
		ID:                 "token-1",
		Name:               "integration",
		ScopeType:          integrations.ScopeGlobal,
		AllowedEventTypes:  []string{"bot.message"},
		AllowedActionTypes: []string{"create_session"},
		CreatedAt:          time.Now().UTC(),
		UpdatedAt:          time.Now().UTC(),
	}
	backend := &fakeIntegrationGatewayBackend{
		identity: integrations.TokenIdentity{Token: apiToken},
		session: integrations.IntegrationSession{
			ID:        "session-1",
			BotID:     "bot-1",
			CreatedAt: time.Now().UTC(),
		},
	}
	service := NewIntegrationGatewayServiceWithBackend(backend, integrations.NewHub(), func(context.Context, string) (integrations.APIToken, error) {
		return apiToken, nil
	}, verifier)
	client, cleanup := newIntegrationGatewayTestClient(t, service, serviceToken)
	defer cleanup()

	identity, err := client.ValidateToken(context.Background(), "raw-token")
	if err != nil {
		t.Fatal(err)
	}
	if identity.Token.ID != "token-1" {
		t.Fatalf("identity token id = %q, want token-1", identity.Token.ID)
	}
	session, err := client.CreateSession(context.Background(), identity, "bot-1", "external-1", map[string]string{"source": "test"})
	if err != nil {
		t.Fatal(err)
	}
	if session.ID != "session-1" || backend.createdBotID != "bot-1" {
		t.Fatalf("session=%#v createdBotID=%q", session, backend.createdBotID)
	}
}

func newIntegrationGatewayTestClient(t *testing.T, service *IntegrationGatewayService, serviceToken string) (*integrations.GatewayClient, func()) {
	t.Helper()
	e := echo.New()
	NewIntegrationGatewayHandler(service).Register(e)
	server := httptest.NewServer(e)
	baseURL := server.URL + "/connect"
	return integrations.NewGatewayClient(integrations.GatewayClientOptions{
		BaseURL:      baseURL,
		ServiceToken: serviceToken,
	}), server.Close
}

func newIntegrationGatewayServiceAuth(t *testing.T) (string, *serviceauth.Verifier) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := serviceauth.NewSigner("active", map[string]ed25519.PrivateKey{"active": privateKey})
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := serviceauth.NewVerifier(map[string]ed25519.PublicKey{"active": publicKey})
	if err != nil {
		t.Fatal(err)
	}
	token, err := signer.Sign(serviceauth.Claims{
		Audience:  serviceauth.AudienceServer,
		Subject:   serviceauth.AudienceIntegrationGateway,
		Scopes:    []string{serviceauth.ScopeIntegrationGateway},
		ExpiresAt: time.Now().UTC().Add(serviceauth.MaxServiceTokenTTL),
	})
	if err != nil {
		t.Fatal(err)
	}
	return token, verifier
}

type fakeIntegrationGatewayBackend struct {
	identity     integrations.TokenIdentity
	session      integrations.IntegrationSession
	createdBotID string
}

func (f *fakeIntegrationGatewayBackend) ValidateToken(context.Context, string) (integrations.TokenIdentity, error) {
	return f.identity, nil
}

func (*fakeIntegrationGatewayBackend) AuthorizeBot(context.Context, integrations.TokenIdentity, string, string) error {
	return nil
}

func (*fakeIntegrationGatewayBackend) AuthorizeBotGroup(context.Context, integrations.TokenIdentity, string) error {
	return nil
}

func (*fakeIntegrationGatewayBackend) AuthorizeEvent(context.Context, integrations.TokenIdentity, string) error {
	return nil
}

func (*fakeIntegrationGatewayBackend) AckEvent(context.Context, integrations.TokenIdentity, string) error {
	return nil
}

func (f *fakeIntegrationGatewayBackend) CreateSession(_ context.Context, _ integrations.TokenIdentity, botID string, _ string, _ map[string]string) (integrations.IntegrationSession, error) {
	f.createdBotID = botID
	return f.session, nil
}

func (*fakeIntegrationGatewayBackend) GetSessionStatus(context.Context, integrations.TokenIdentity, string) (integrations.SessionStatus, error) {
	return integrations.SessionStatus{}, nil
}

func (*fakeIntegrationGatewayBackend) SendBotMessage(context.Context, integrations.TokenIdentity, integrations.SendBotMessageGatewayRequest) (integrations.SendBotMessageResult, error) {
	return integrations.SendBotMessageResult{}, nil
}

func (*fakeIntegrationGatewayBackend) RequestAction(context.Context, integrations.TokenIdentity, integrations.RequestActionGatewayRequest) (integrations.RequestActionResult, error) {
	return integrations.RequestActionResult{}, nil
}
