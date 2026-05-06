package connectapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/memohai/memoh/internal/bots"
	privatev1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1"
	"github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1/privatev1connect"
	"github.com/memohai/memoh/internal/mcp"
)

type mcpConnectionService interface {
	ListByBot(context.Context, string) ([]mcp.Connection, error)
	Get(context.Context, string, string) (mcp.Connection, error)
	Create(context.Context, string, mcp.UpsertRequest) (mcp.Connection, error)
	Update(context.Context, string, string, mcp.UpsertRequest) (mcp.Connection, error)
	Delete(context.Context, string, string) error
	BatchDelete(context.Context, string, []string) error
	Import(context.Context, string, mcp.ImportRequest) ([]mcp.Connection, error)
	ExportByBot(context.Context, string) (mcp.ExportResponse, error)
	UpdateProbeResult(context.Context, string, string, string, []mcp.ToolDescriptor, string) error
}

type mcpOAuthService interface {
	Discover(context.Context, string) (*mcp.DiscoveryResult, error)
	SaveDiscovery(context.Context, string, *mcp.DiscoveryResult) error
	StartAuthorization(context.Context, string, string, string, string) (*mcp.AuthorizeResult, error)
	HandleCallback(context.Context, string, string) (string, error)
	GetStatus(context.Context, string) (*mcp.OAuthStatus, error)
	RevokeToken(context.Context, string) error
}

type McpFederationGateway interface {
	ListHTTPConnectionTools(context.Context, mcp.Connection) ([]mcp.ToolDescriptor, error)
	ListSSEConnectionTools(context.Context, mcp.Connection) ([]mcp.ToolDescriptor, error)
	ListStdioConnectionTools(context.Context, string, mcp.Connection) ([]mcp.ToolDescriptor, error)
}

type McpService struct {
	connections mcpConnectionService
	oauth       mcpOAuthService
	bots        *bots.Service
	gateway     McpFederationGateway
}

func NewMcpService(connections *mcp.ConnectionService, oauth *mcp.OAuthService, bots *bots.Service, gateway McpFederationGateway) *McpService {
	return &McpService{
		connections: connections,
		oauth:       oauth,
		bots:        bots,
		gateway:     gateway,
	}
}

func NewMcpHandler(service *McpService) Handler {
	path, handler := privatev1connect.NewMcpServiceHandler(service)
	return NewHandler(path, handler)
}

func (s *McpService) CreateMcpConnection(ctx context.Context, req *connect.Request[privatev1.CreateMcpConnectionRequest]) (*connect.Response[privatev1.CreateMcpConnectionResponse], error) {
	botID := strings.TrimSpace(req.Msg.GetBotId())
	if err := s.requireBotAccess(ctx, botID); err != nil {
		return nil, err
	}
	connection, err := s.connections.Create(ctx, botID, mcpUpsertFromCreateRequest(req.Msg))
	if err != nil {
		return nil, mcpConnectError(err)
	}
	return connect.NewResponse(&privatev1.CreateMcpConnectionResponse{Connection: mcpConnectionToProto(connection)}), nil
}

func (s *McpService) ListMcpConnections(ctx context.Context, req *connect.Request[privatev1.ListMcpConnectionsRequest]) (*connect.Response[privatev1.ListMcpConnectionsResponse], error) {
	botID := strings.TrimSpace(req.Msg.GetBotId())
	if err := s.requireBotAccess(ctx, botID); err != nil {
		return nil, err
	}
	items, err := s.connections.ListByBot(ctx, botID)
	if err != nil {
		return nil, mcpConnectError(err)
	}
	return connect.NewResponse(&privatev1.ListMcpConnectionsResponse{
		Connections: mcpConnectionsToProto(items),
		Page:        &privatev1.PageResponse{},
	}), nil
}

func (s *McpService) GetMcpConnection(ctx context.Context, req *connect.Request[privatev1.GetMcpConnectionRequest]) (*connect.Response[privatev1.GetMcpConnectionResponse], error) {
	connection, err := s.findConnection(ctx, req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&privatev1.GetMcpConnectionResponse{Connection: mcpConnectionToProto(connection)}), nil
}

func (s *McpService) UpdateMcpConnection(ctx context.Context, req *connect.Request[privatev1.UpdateMcpConnectionRequest]) (*connect.Response[privatev1.UpdateMcpConnectionResponse], error) {
	current, err := s.findConnection(ctx, req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	update := mcpUpsertFromConnection(current)
	if req.Msg.Name != nil {
		update.Name = strings.TrimSpace(req.Msg.GetName())
	}
	if req.Msg.Transport != nil {
		update.Transport = strings.TrimSpace(req.Msg.GetTransport())
	}
	if req.Msg.Enabled != nil {
		enabled := req.Msg.GetEnabled()
		update.Active = &enabled
	}
	if req.Msg.GetConfig() != nil {
		mergeMcpConfigIntoUpsert(&update, structToMap(req.Msg.GetConfig()))
	}
	connection, err := s.connections.Update(ctx, current.BotID, current.ID, update)
	if err != nil {
		return nil, mcpConnectError(err)
	}
	return connect.NewResponse(&privatev1.UpdateMcpConnectionResponse{Connection: mcpConnectionToProto(connection)}), nil
}

func (s *McpService) DeleteMcpConnection(ctx context.Context, req *connect.Request[privatev1.DeleteMcpConnectionRequest]) (*connect.Response[privatev1.DeleteMcpConnectionResponse], error) {
	connection, err := s.findConnection(ctx, req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	if err := s.connections.Delete(ctx, connection.BotID, connection.ID); err != nil {
		return nil, mcpConnectError(err)
	}
	return connect.NewResponse(&privatev1.DeleteMcpConnectionResponse{}), nil
}

func (s *McpService) BatchDeleteMcpConnections(ctx context.Context, req *connect.Request[privatev1.BatchDeleteMcpConnectionsRequest]) (*connect.Response[privatev1.BatchDeleteMcpConnectionsResponse], error) {
	groups := map[string][]string{}
	for _, id := range req.Msg.GetIds() {
		connection, err := s.findConnection(ctx, id)
		if err != nil {
			return nil, err
		}
		groups[connection.BotID] = append(groups[connection.BotID], connection.ID)
	}
	if len(groups) == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("ids are required"))
	}
	var deleted int32
	for botID, ids := range groups {
		if err := s.connections.BatchDelete(ctx, botID, ids); err != nil {
			return nil, mcpConnectError(err)
		}
		deleted += int32FromInt(len(ids))
	}
	return connect.NewResponse(&privatev1.BatchDeleteMcpConnectionsResponse{DeletedCount: deleted}), nil
}

func (s *McpService) ProbeMcpConnection(ctx context.Context, req *connect.Request[privatev1.ProbeMcpConnectionRequest]) (*connect.Response[privatev1.ProbeMcpConnectionResponse], error) {
	connection, err := s.findConnection(ctx, req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	if req.Msg.GetConfig() != nil {
		mergeMcpConfigIntoConnection(&connection, structToMap(req.Msg.GetConfig()))
	}
	if s.gateway == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("mcp federation gateway not configured"))
	}

	tools, probeErr := s.listConnectionTools(ctx, connection)
	if probeErr != nil {
		_ = s.connections.UpdateProbeResult(ctx, connection.BotID, connection.ID, "error", []mcp.ToolDescriptor{}, probeErr.Error())
		return connect.NewResponse(&privatev1.ProbeMcpConnectionResponse{
			Result: &privatev1.McpProbeResult{
				Ok:      false,
				Message: probeErr.Error(),
				Metadata: mapToStruct(map[string]any{
					"auth_required": mcpProbeAuthRequired(probeErr),
				}),
			},
		}), nil
	}
	if tools == nil {
		tools = []mcp.ToolDescriptor{}
	}
	_ = s.connections.UpdateProbeResult(ctx, connection.BotID, connection.ID, "connected", tools, "")
	return connect.NewResponse(&privatev1.ProbeMcpConnectionResponse{
		Result: &privatev1.McpProbeResult{
			Ok:        true,
			Message:   "connected",
			ToolNames: mcpToolNames(tools),
			Tools:     mcpToolDescriptorsToProto(tools),
			Metadata: mapToStruct(map[string]any{
				"tools": mcpToolsMetadata(tools),
			}),
		},
	}), nil
}

func (s *McpService) ImportMcpConnections(ctx context.Context, req *connect.Request[privatev1.ImportMcpConnectionsRequest]) (*connect.Response[privatev1.ImportMcpConnectionsResponse], error) {
	botID := strings.TrimSpace(req.Msg.GetBotId())
	if err := s.requireBotAccess(ctx, botID); err != nil {
		return nil, err
	}
	items, err := s.connections.Import(ctx, botID, mcpImportRequestFromMap(structToMap(req.Msg.GetPayload())))
	if err != nil {
		return nil, mcpConnectError(err)
	}
	return connect.NewResponse(&privatev1.ImportMcpConnectionsResponse{Connections: mcpConnectionsToProto(items)}), nil
}

func (s *McpService) ExportMcpConnections(ctx context.Context, req *connect.Request[privatev1.ExportMcpConnectionsRequest]) (*connect.Response[privatev1.ExportMcpConnectionsResponse], error) {
	botID := strings.TrimSpace(req.Msg.GetBotId())
	if err := s.requireBotAccess(ctx, botID); err != nil {
		return nil, err
	}
	export, err := s.connections.ExportByBot(ctx, botID)
	if err != nil {
		return nil, mcpConnectError(err)
	}
	if ids := stringSet(req.Msg.GetIds()); len(ids) > 0 {
		items, err := s.connections.ListByBot(ctx, botID)
		if err != nil {
			return nil, mcpConnectError(err)
		}
		export.MCPServers = filterMcpExportServers(export.MCPServers, items, ids)
	}
	payload := mcpExportResponseToMap(export)
	return connect.NewResponse(&privatev1.ExportMcpConnectionsResponse{Payload: mapToStruct(payload)}), nil
}

func (s *McpService) DiscoverMcp(ctx context.Context, req *connect.Request[privatev1.DiscoverMcpRequest]) (*connect.Response[privatev1.DiscoverMcpResponse], error) {
	botID := strings.TrimSpace(req.Msg.GetBotId())
	if err := s.requireBotAccess(ctx, botID); err != nil {
		return nil, err
	}
	source := strings.TrimSpace(req.Msg.GetSource())
	if source == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("source is required"))
	}

	if strings.HasPrefix(source, "{") {
		items, err := mcpConnectionsFromSourceJSON(botID, source)
		if err != nil {
			return nil, mcpConnectError(err)
		}
		return connect.NewResponse(&privatev1.DiscoverMcpResponse{Connections: mcpConnectionsToProto(items)}), nil
	}

	if s.oauth != nil {
		if connection, err := s.connections.Get(ctx, botID, source); err == nil {
			serverURL, _ := connection.Config["url"].(string)
			if strings.TrimSpace(serverURL) == "" {
				return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("MCP server URL is required for OAuth discovery"))
			}
			discovery, err := s.oauth.Discover(ctx, serverURL)
			if err != nil {
				return nil, mcpConnectError(err)
			}
			if err := s.oauth.SaveDiscovery(ctx, connection.ID, discovery); err != nil {
				return nil, mcpConnectError(err)
			}
			return connect.NewResponse(&privatev1.DiscoverMcpResponse{Connections: []*privatev1.McpConnection{mcpConnectionToProto(connection)}}), nil
		}
	}

	return connect.NewResponse(&privatev1.DiscoverMcpResponse{
		Connections: []*privatev1.McpConnection{{
			BotId:     botID,
			Name:      mcpNameFromSourceURL(source),
			Transport: "http",
			Enabled:   true,
			Config: mapToStruct(map[string]any{
				"url": source,
			}),
		}},
	}), nil
}

func (s *McpService) StartMcpOauth(ctx context.Context, req *connect.Request[privatev1.StartMcpOauthRequest]) (*connect.Response[privatev1.StartMcpOauthResponse], error) {
	connection, err := s.findConnection(ctx, req.Msg.GetConnectionId())
	if err != nil {
		return nil, err
	}
	if s.oauth == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("mcp oauth service not configured"))
	}
	result, err := s.oauth.StartAuthorization(ctx, connection.ID, req.Msg.GetClientId(), req.Msg.GetClientSecret(), req.Msg.GetRedirectUri())
	if err != nil {
		return nil, mcpConnectError(err)
	}
	return connect.NewResponse(&privatev1.StartMcpOauthResponse{
		AuthorizeUrl: result.AuthorizationURL,
		State:        stateFromAuthorizationURL(result.AuthorizationURL),
	}), nil
}

func (s *McpService) GetMcpOauthStatus(ctx context.Context, req *connect.Request[privatev1.GetMcpOauthStatusRequest]) (*connect.Response[privatev1.GetMcpOauthStatusResponse], error) {
	connection, err := s.findConnection(ctx, req.Msg.GetConnectionId())
	if err != nil {
		return nil, err
	}
	if s.oauth == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("mcp oauth service not configured"))
	}
	status, err := s.oauth.GetStatus(ctx, connection.ID)
	if err != nil {
		return nil, mcpConnectError(err)
	}
	return connect.NewResponse(&privatev1.GetMcpOauthStatusResponse{
		Authorized: status.HasToken && !status.Expired,
		Metadata: mapToStruct(map[string]any{
			"configured":   status.Configured,
			"has_token":    status.HasToken,
			"expired":      status.Expired,
			"scopes":       status.Scopes,
			"auth_server":  status.AuthServer,
			"callback_url": status.CallbackURL,
			"expires_at":   mcpOptionalTimeString(status.ExpiresAt),
		}),
	}), nil
}

func (s *McpService) RevokeMcpOauth(ctx context.Context, req *connect.Request[privatev1.RevokeMcpOauthRequest]) (*connect.Response[privatev1.RevokeMcpOauthResponse], error) {
	connection, err := s.findConnection(ctx, req.Msg.GetConnectionId())
	if err != nil {
		return nil, err
	}
	if s.oauth == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("mcp oauth service not configured"))
	}
	if err := s.oauth.RevokeToken(ctx, connection.ID); err != nil {
		return nil, mcpConnectError(err)
	}
	return connect.NewResponse(&privatev1.RevokeMcpOauthResponse{}), nil
}

func (s *McpService) ExchangeMcpOauth(ctx context.Context, req *connect.Request[privatev1.ExchangeMcpOauthRequest]) (*connect.Response[privatev1.ExchangeMcpOauthResponse], error) {
	code := strings.TrimSpace(req.Msg.GetCode())
	state := strings.TrimSpace(req.Msg.GetState())
	if code == "" || state == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("code and state are required"))
	}
	if s.oauth == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("mcp oauth service not configured"))
	}
	if _, err := s.oauth.HandleCallback(ctx, state, code); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	return connect.NewResponse(&privatev1.ExchangeMcpOauthResponse{Success: true}), nil
}

func (s *McpService) requireBotAccess(ctx context.Context, botID string) error {
	if s.connections == nil {
		return connect.NewError(connect.CodeInternal, errors.New("mcp connection service not configured"))
	}
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return connect.NewError(connect.CodeUnauthenticated, err)
	}
	botID = strings.TrimSpace(botID)
	if botID == "" {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("bot_id is required"))
	}
	if s.bots == nil {
		return connect.NewError(connect.CodeInternal, errors.New("bot service not configured"))
	}
	if _, err := s.bots.AuthorizeAccess(ctx, userID, botID, false); err != nil {
		return botConnectError(err)
	}
	return nil
}

func (s *McpService) findConnection(ctx context.Context, id string) (mcp.Connection, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return mcp.Connection{}, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}
	if s.connections == nil {
		return mcp.Connection{}, connect.NewError(connect.CodeInternal, errors.New("mcp connection service not configured"))
	}
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return mcp.Connection{}, connect.NewError(connect.CodeUnauthenticated, err)
	}
	if s.bots == nil {
		return mcp.Connection{}, connect.NewError(connect.CodeInternal, errors.New("bot service not configured"))
	}
	accessible, err := s.bots.ListAccessible(ctx, userID)
	if err != nil {
		return mcp.Connection{}, botConnectError(err)
	}
	for _, bot := range accessible {
		items, err := s.connections.ListByBot(ctx, bot.ID)
		if err != nil {
			return mcp.Connection{}, mcpConnectError(err)
		}
		for _, item := range items {
			if item.ID == id {
				return item, nil
			}
		}
	}
	return mcp.Connection{}, connect.NewError(connect.CodeNotFound, pgx.ErrNoRows)
}

func (s *McpService) listConnectionTools(ctx context.Context, connection mcp.Connection) ([]mcp.ToolDescriptor, error) {
	switch strings.ToLower(strings.TrimSpace(connection.Type)) {
	case "http":
		return s.gateway.ListHTTPConnectionTools(ctx, connection)
	case "sse":
		return s.gateway.ListSSEConnectionTools(ctx, connection)
	case "stdio":
		return s.gateway.ListStdioConnectionTools(ctx, connection.BotID, connection)
	default:
		return nil, fmt.Errorf("unsupported connection type: %s", connection.Type)
	}
}

func mcpConnectionToProto(connection mcp.Connection) *privatev1.McpConnection {
	return &privatev1.McpConnection{
		Id:        connection.ID,
		BotId:     connection.BotID,
		Name:      connection.Name,
		Transport: connection.Type,
		Enabled:   connection.Active,
		Config:    mapToStruct(connection.Config),
		Audit: &privatev1.AuditFields{
			CreatedAt: timeToProto(connection.CreatedAt),
			UpdatedAt: timeToProto(connection.UpdatedAt),
		},
		Status:        connection.Status,
		ToolsCache:    mcpToolDescriptorsToProto(connection.ToolsCache),
		LastProbedAt:  timePtrToProto(connection.LastProbedAt),
		StatusMessage: connection.StatusMessage,
		AuthType:      connection.AuthType,
	}
}

func mcpConnectionsToProto(items []mcp.Connection) []*privatev1.McpConnection {
	out := make([]*privatev1.McpConnection, 0, len(items))
	for _, item := range items {
		out = append(out, mcpConnectionToProto(item))
	}
	return out
}

func mcpUpsertFromCreateRequest(req *privatev1.CreateMcpConnectionRequest) mcp.UpsertRequest {
	active := req.GetEnabled()
	upsert := mcp.UpsertRequest{
		Name:      strings.TrimSpace(req.GetName()),
		Transport: strings.TrimSpace(req.GetTransport()),
		Active:    &active,
	}
	mergeMcpConfigIntoUpsert(&upsert, structToMap(req.GetConfig()))
	return upsert
}

func mcpUpsertFromConnection(connection mcp.Connection) mcp.UpsertRequest {
	active := connection.Active
	upsert := mcp.UpsertRequest{
		Name:      connection.Name,
		Transport: connection.Type,
		Active:    &active,
		AuthType:  connection.AuthType,
	}
	mergeMcpConfigIntoUpsert(&upsert, connection.Config)
	return upsert
}

func mergeMcpConfigIntoUpsert(upsert *mcp.UpsertRequest, config map[string]any) {
	if config == nil {
		return
	}
	if value := strings.TrimSpace(stringFromMap(config, "command")); value != "" {
		upsert.Command = value
	}
	if value := strings.TrimSpace(stringFromMap(config, "url")); value != "" {
		upsert.URL = value
	}
	if value := strings.TrimSpace(stringFromMap(config, "cwd")); value != "" {
		upsert.Cwd = value
	}
	if value := strings.TrimSpace(stringFromMap(config, "transport")); value != "" {
		upsert.Transport = value
	}
	if value := strings.TrimSpace(stringFromMap(config, "auth_type")); value != "" {
		upsert.AuthType = value
	}
	if args := stringSliceFromAny(config["args"]); len(args) > 0 {
		upsert.Args = args
	}
	if env := stringMapFromAny(config["env"]); len(env) > 0 {
		upsert.Env = env
	}
	if headers := stringMapFromAny(config["headers"]); len(headers) > 0 {
		upsert.Headers = headers
	}
}

func mergeMcpConfigIntoConnection(connection *mcp.Connection, config map[string]any) {
	if connection.Config == nil {
		connection.Config = map[string]any{}
	}
	for key, value := range config {
		connection.Config[key] = value
	}
	if transport := strings.TrimSpace(stringFromMap(config, "transport")); transport != "" {
		connection.Type = transport
	}
}

func mcpImportRequestFromMap(payload map[string]any) mcp.ImportRequest {
	rawServers, _ := payload["mcpServers"].(map[string]any)
	out := mcp.ImportRequest{MCPServers: map[string]mcp.MCPServerEntry{}}
	for name, raw := range rawServers {
		entryMap, _ := raw.(map[string]any)
		out.MCPServers[name] = mcp.MCPServerEntry{
			Command:   stringFromMap(entryMap, "command"),
			Args:      stringSliceFromAny(entryMap["args"]),
			Env:       stringMapFromAny(entryMap["env"]),
			Cwd:       stringFromMap(entryMap, "cwd"),
			URL:       stringFromMap(entryMap, "url"),
			Headers:   stringMapFromAny(entryMap["headers"]),
			Transport: stringFromMap(entryMap, "transport"),
		}
	}
	return out
}

func mcpExportResponseToMap(export mcp.ExportResponse) map[string]any {
	servers := make(map[string]any, len(export.MCPServers))
	for name, entry := range export.MCPServers {
		value := map[string]any{}
		if entry.Command != "" {
			value["command"] = entry.Command
		}
		if len(entry.Args) > 0 {
			value["args"] = entry.Args
		}
		if len(entry.Env) > 0 {
			value["env"] = entry.Env
		}
		if entry.Cwd != "" {
			value["cwd"] = entry.Cwd
		}
		if entry.URL != "" {
			value["url"] = entry.URL
		}
		if len(entry.Headers) > 0 {
			value["headers"] = entry.Headers
		}
		if entry.Transport != "" {
			value["transport"] = entry.Transport
		}
		servers[name] = value
	}
	return map[string]any{"mcpServers": servers}
}

func filterMcpExportServers(servers map[string]mcp.MCPServerEntry, items []mcp.Connection, ids map[string]struct{}) map[string]mcp.MCPServerEntry {
	names := map[string]struct{}{}
	for _, item := range items {
		if _, ok := ids[item.ID]; ok {
			names[item.Name] = struct{}{}
		}
	}
	filtered := make(map[string]mcp.MCPServerEntry, len(names))
	for name, entry := range servers {
		if _, ok := names[name]; ok {
			filtered[name] = entry
		}
	}
	return filtered
}

func mcpConnectionsFromSourceJSON(botID string, source string) ([]mcp.Connection, error) {
	var payload map[string]any
	if err := json.Unmarshal([]byte(source), &payload); err != nil {
		return nil, err
	}
	importReq := mcpImportRequestFromMap(payload)
	out := make([]mcp.Connection, 0, len(importReq.MCPServers))
	for name, entry := range importReq.MCPServers {
		upsert := mcp.UpsertRequest{
			Name:      name,
			Command:   entry.Command,
			Args:      entry.Args,
			Env:       entry.Env,
			Cwd:       entry.Cwd,
			URL:       entry.URL,
			Headers:   entry.Headers,
			Transport: entry.Transport,
		}
		transport, config, err := mcpPreviewTransportAndConfig(upsert)
		if err != nil {
			return nil, err
		}
		out = append(out, mcp.Connection{
			BotID:  botID,
			Name:   name,
			Type:   transport,
			Config: config,
			Active: true,
		})
	}
	return out, nil
}

func mcpPreviewTransportAndConfig(req mcp.UpsertRequest) (string, map[string]any, error) {
	hasCommand := strings.TrimSpace(req.Command) != ""
	hasURL := strings.TrimSpace(req.URL) != ""
	if !hasCommand && !hasURL {
		return "", nil, errors.New("command or url is required")
	}
	if hasCommand && hasURL {
		return "", nil, errors.New("command and url are mutually exclusive")
	}
	config := map[string]any{}
	if hasCommand {
		config["command"] = strings.TrimSpace(req.Command)
		if len(req.Args) > 0 {
			config["args"] = req.Args
		}
		if len(req.Env) > 0 {
			config["env"] = req.Env
		}
		if strings.TrimSpace(req.Cwd) != "" {
			config["cwd"] = strings.TrimSpace(req.Cwd)
		}
		return "stdio", config, nil
	}
	config["url"] = strings.TrimSpace(req.URL)
	if len(req.Headers) > 0 {
		config["headers"] = req.Headers
	}
	if strings.EqualFold(strings.TrimSpace(req.Transport), "sse") {
		return "sse", config, nil
	}
	return "http", config, nil
}

func mcpToolNames(tools []mcp.ToolDescriptor) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		if strings.TrimSpace(tool.Name) != "" {
			names = append(names, strings.TrimSpace(tool.Name))
		}
	}
	return names
}

func mcpToolDescriptorsToProto(tools []mcp.ToolDescriptor) []*privatev1.McpToolDescriptor {
	out := make([]*privatev1.McpToolDescriptor, 0, len(tools))
	for _, tool := range tools {
		out = append(out, &privatev1.McpToolDescriptor{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: mapToStruct(tool.InputSchema),
		})
	}
	return out
}

func mcpToolsMetadata(tools []mcp.ToolDescriptor) []map[string]any {
	out := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		out = append(out, map[string]any{
			"name":         tool.Name,
			"description":  tool.Description,
			"input_schema": tool.InputSchema,
		})
	}
	return out
}

func mcpProbeAuthRequired(err error) bool {
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "401") || strings.Contains(text, "unauthorized")
}

func mcpOptionalTimeString(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.Format(time.RFC3339)
}

func timePtrToProto(value *time.Time) *timestamppb.Timestamp {
	if value == nil {
		return nil
	}
	return timeToProto(*value)
}

func stringFromMap(value map[string]any, key string) string {
	raw, _ := value[key].(string)
	return raw
}

func stringMapFromAny(value any) map[string]string {
	switch typed := value.(type) {
	case map[string]string:
		return typed
	case map[string]any:
		out := make(map[string]string, len(typed))
		for key, value := range typed {
			if text, ok := value.(string); ok {
				out[key] = text
			}
		}
		return out
	default:
		return nil
	}
}

func stateFromAuthorizationURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return parsed.Query().Get("state")
}

func mcpNameFromSourceURL(source string) string {
	parsed, err := url.Parse(source)
	if err == nil && parsed.Host != "" {
		return parsed.Host
	}
	return "mcp-server"
}

func mcpConnectError(err error) error {
	var connectErr *connect.Error
	if errors.As(err, &connectErr) {
		return connectErr
	}
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return connect.NewError(connect.CodeNotFound, err)
	case strings.Contains(err.Error(), "required"),
		strings.Contains(err.Error(), "invalid"),
		strings.Contains(err.Error(), "mutually exclusive"),
		strings.Contains(err.Error(), "unsupported"):
		return connect.NewError(connect.CodeInvalidArgument, err)
	default:
		return connect.NewError(connect.CodeInternal, err)
	}
}
