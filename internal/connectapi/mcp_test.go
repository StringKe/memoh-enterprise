package connectapi

import (
	"context"
	"errors"
	"testing"

	"connectrpc.com/connect"

	privatev1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1"
	"github.com/memohai/memoh/internal/mcp"
)

func TestMcpUpsertFromCreateRequestMapsStdioConfig(t *testing.T) {
	req := &privatev1.CreateMcpConnectionRequest{
		Name:      "filesystem",
		Transport: "stdio",
		Enabled:   true,
		Config: mapToStruct(map[string]any{
			"command": "npx",
			"args":    []any{"-y", "@modelcontextprotocol/server-filesystem"},
			"env": map[string]any{
				"DEBUG": "1",
			},
			"cwd":       "/tmp",
			"auth_type": "none",
		}),
	}

	got := mcpUpsertFromCreateRequest(req)

	if got.Name != "filesystem" {
		t.Fatalf("Name = %q, want filesystem", got.Name)
	}
	if got.Transport != "stdio" {
		t.Fatalf("Transport = %q, want stdio", got.Transport)
	}
	if got.Active == nil || !*got.Active {
		t.Fatalf("Active = %v, want true", got.Active)
	}
	if got.Command != "npx" {
		t.Fatalf("Command = %q, want npx", got.Command)
	}
	if len(got.Args) != 2 || got.Args[0] != "-y" {
		t.Fatalf("Args = %#v, want filesystem args", got.Args)
	}
	if got.Env["DEBUG"] != "1" {
		t.Fatalf("Env = %#v, want DEBUG=1", got.Env)
	}
	if got.Cwd != "/tmp" {
		t.Fatalf("Cwd = %q, want /tmp", got.Cwd)
	}
	if got.AuthType != "none" {
		t.Fatalf("AuthType = %q, want none", got.AuthType)
	}
}

func TestMcpConnectionsFromSourceJSONPreviewsConnections(t *testing.T) {
	source := `{"mcpServers":{"linear":{"url":"https://mcp.linear.app","transport":"sse"},"shell":{"command":"bun","args":["run","server"]}}}`

	got, err := mcpConnectionsFromSourceJSON("bot-1", source)
	if err != nil {
		t.Fatalf("mcpConnectionsFromSourceJSON returned error: %v", err)
	}

	byName := map[string]mcp.Connection{}
	for _, item := range got {
		byName[item.Name] = item
	}
	if byName["linear"].Type != "sse" {
		t.Fatalf("linear.Type = %q, want sse", byName["linear"].Type)
	}
	if byName["linear"].Config["url"] != "https://mcp.linear.app" {
		t.Fatalf("linear.Config = %#v, want url", byName["linear"].Config)
	}
	if byName["shell"].Type != "stdio" {
		t.Fatalf("shell.Type = %q, want stdio", byName["shell"].Type)
	}
	if byName["shell"].Config["command"] != "bun" {
		t.Fatalf("shell.Config = %#v, want command", byName["shell"].Config)
	}
}

func TestFilterMcpExportServersUsesConnectionIDs(t *testing.T) {
	servers := map[string]mcp.MCPServerEntry{
		"linear": {URL: "https://mcp.linear.app"},
		"shell":  {Command: "bun"},
	}
	items := []mcp.Connection{
		{ID: "conn-1", Name: "linear"},
		{ID: "conn-2", Name: "shell"},
	}

	got := filterMcpExportServers(servers, items, map[string]struct{}{"conn-2": {}})

	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got["shell"].Command != "bun" {
		t.Fatalf("got = %#v, want shell server", got)
	}
	if _, ok := got["linear"]; ok {
		t.Fatalf("linear should be filtered out: %#v", got)
	}
}

func TestStateFromAuthorizationURL(t *testing.T) {
	got := stateFromAuthorizationURL("https://auth.example/authorize?client_id=client&state=state-123")
	if got != "state-123" {
		t.Fatalf("stateFromAuthorizationURL = %q, want state-123", got)
	}
}

func TestMcpServiceExchangeOauthRequiresCodeAndState(t *testing.T) {
	t.Parallel()

	service := &McpService{}

	_, err := service.ExchangeMcpOauth(context.Background(), connect.NewRequest(&privatev1.ExchangeMcpOauthRequest{
		Code: "code-1",
	}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("code = %s, want %s, err = %v", connect.CodeOf(err), connect.CodeInvalidArgument, err)
	}
}

func TestMcpServiceExchangeOauthCallsCallback(t *testing.T) {
	t.Parallel()

	oauth := &fakeMcpOAuthService{}
	service := &McpService{oauth: oauth}

	resp, err := service.ExchangeMcpOauth(context.Background(), connect.NewRequest(&privatev1.ExchangeMcpOauthRequest{
		Code:  " code-1 ",
		State: " state-1 ",
	}))
	if err != nil {
		t.Fatalf("ExchangeMcpOauth returned error: %v", err)
	}
	if !resp.Msg.GetSuccess() {
		t.Fatal("success = false, want true")
	}
	if oauth.callbackState != "state-1" || oauth.callbackCode != "code-1" {
		t.Fatalf("callback state/code = %q/%q, want state-1/code-1", oauth.callbackState, oauth.callbackCode)
	}
}

func TestMcpServiceExchangeOauthMapsCallbackError(t *testing.T) {
	t.Parallel()

	service := &McpService{oauth: &fakeMcpOAuthService{callbackErr: errors.New("invalid oauth state")}}

	_, err := service.ExchangeMcpOauth(context.Background(), connect.NewRequest(&privatev1.ExchangeMcpOauthRequest{
		Code:  "code-1",
		State: "state-1",
	}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("code = %s, want %s, err = %v", connect.CodeOf(err), connect.CodeInvalidArgument, err)
	}
}

type fakeMcpOAuthService struct {
	callbackState string
	callbackCode  string
	callbackErr   error
}

func (*fakeMcpOAuthService) Discover(context.Context, string) (*mcp.DiscoveryResult, error) {
	return nil, nil
}

func (*fakeMcpOAuthService) SaveDiscovery(context.Context, string, *mcp.DiscoveryResult) error {
	return nil
}

func (*fakeMcpOAuthService) StartAuthorization(context.Context, string, string, string, string) (*mcp.AuthorizeResult, error) {
	return &mcp.AuthorizeResult{}, nil
}

func (s *fakeMcpOAuthService) HandleCallback(_ context.Context, state string, code string) (string, error) {
	s.callbackState = state
	s.callbackCode = code
	return "connection-1", s.callbackErr
}

func (*fakeMcpOAuthService) GetStatus(context.Context, string) (*mcp.OAuthStatus, error) {
	return &mcp.OAuthStatus{}, nil
}

func (*fakeMcpOAuthService) RevokeToken(context.Context, string) error {
	return nil
}
