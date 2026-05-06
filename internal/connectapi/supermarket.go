package connectapi

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"connectrpc.com/connect"

	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/config"
	privatev1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1"
	"github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1/privatev1connect"
	"github.com/memohai/memoh/internal/handlers"
	"github.com/memohai/memoh/internal/iam/rbac"
	"github.com/memohai/memoh/internal/mcp"
	skillset "github.com/memohai/memoh/internal/skills"
	"github.com/memohai/memoh/internal/workspace/executorclient"
)

type SupermarketService struct {
	baseURL    string
	httpClient *http.Client
	mcp        mcpConnectionService
	files      skillFileClientProvider
	bots       *bots.Service
	logger     *slog.Logger
}

func NewSupermarketService(log *slog.Logger, cfg config.Config, connections *mcp.ConnectionService, containers executorclient.Provider, botService *bots.Service) *SupermarketService {
	if log == nil {
		log = slog.Default()
	}
	return &SupermarketService{
		baseURL:    cfg.Supermarket.GetBaseURL(),
		httpClient: &http.Client{Timeout: 30 * time.Second},
		mcp:        connections,
		files:      workspaceExecutorSkillClientProvider{provider: containers},
		bots:       botService,
		logger:     log.With(slog.String("service", "connect_supermarket")),
	}
}

func NewSupermarketHandler(service *SupermarketService) Handler {
	path, handler := privatev1connect.NewSupermarketServiceHandler(service)
	return NewHandler(path, handler)
}

func (s *SupermarketService) ListSupermarketMcps(ctx context.Context, req *connect.Request[privatev1.ListSupermarketMcpsRequest]) (*connect.Response[privatev1.ListSupermarketMcpsResponse], error) {
	if _, err := UserIDFromContext(ctx); err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	u, err := s.catalogURL("/api/mcps", req.Msg.GetQuery(), req.Msg.GetTags(), req.Msg.GetPage())
	if err != nil {
		return nil, err
	}
	var upstream handlers.SupermarketMcpListResponse
	if err := s.getJSON(ctx, u, &upstream); err != nil {
		return nil, err
	}
	items := make([]*privatev1.SupermarketItem, 0, len(upstream.Data))
	for _, item := range upstream.Data {
		items = append(items, supermarketMcpToProto(item))
	}
	return connect.NewResponse(&privatev1.ListSupermarketMcpsResponse{
		Items: items,
		Page:  supermarketPageResponse(upstream.Page, upstream.Limit, upstream.Total),
	}), nil
}

func (s *SupermarketService) GetSupermarketMcp(ctx context.Context, req *connect.Request[privatev1.GetSupermarketMcpRequest]) (*connect.Response[privatev1.GetSupermarketMcpResponse], error) {
	if _, err := UserIDFromContext(ctx); err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	item, err := s.fetchMcp(ctx, req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&privatev1.GetSupermarketMcpResponse{Item: supermarketMcpToProto(item)}), nil
}

func (s *SupermarketService) ListSupermarketSkills(ctx context.Context, req *connect.Request[privatev1.ListSupermarketSkillsRequest]) (*connect.Response[privatev1.ListSupermarketSkillsResponse], error) {
	if _, err := UserIDFromContext(ctx); err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	u, err := s.catalogURL("/api/skills", req.Msg.GetQuery(), req.Msg.GetTags(), req.Msg.GetPage())
	if err != nil {
		return nil, err
	}
	var upstream handlers.SupermarketSkillListResponse
	if err := s.getJSON(ctx, u, &upstream); err != nil {
		return nil, err
	}
	items := make([]*privatev1.SupermarketItem, 0, len(upstream.Data))
	for _, item := range upstream.Data {
		items = append(items, supermarketSkillToItemProto(item))
	}
	return connect.NewResponse(&privatev1.ListSupermarketSkillsResponse{
		Items: items,
		Page:  supermarketPageResponse(upstream.Page, upstream.Limit, upstream.Total),
	}), nil
}

func (s *SupermarketService) GetSupermarketSkill(ctx context.Context, req *connect.Request[privatev1.GetSupermarketSkillRequest]) (*connect.Response[privatev1.GetSupermarketSkillResponse], error) {
	if _, err := UserIDFromContext(ctx); err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	id := strings.TrimSpace(req.Msg.GetId())
	if id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}
	var item handlers.SupermarketSkillEntry
	if err := s.getJSON(ctx, strings.TrimRight(s.baseURL, "/")+"/api/skills/"+url.PathEscape(id), &item); err != nil {
		return nil, err
	}
	return connect.NewResponse(&privatev1.GetSupermarketSkillResponse{Item: supermarketSkillToItemProto(item)}), nil
}

func (s *SupermarketService) ListSupermarketTags(ctx context.Context, _ *connect.Request[privatev1.ListSupermarketTagsRequest]) (*connect.Response[privatev1.ListSupermarketTagsResponse], error) {
	if _, err := UserIDFromContext(ctx); err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	var upstream handlers.SupermarketTagsResponse
	if err := s.getJSON(ctx, strings.TrimRight(s.baseURL, "/")+"/api/tags", &upstream); err != nil {
		return nil, err
	}
	tags := make([]*privatev1.SupermarketTag, 0, len(upstream.Tags))
	for _, tag := range upstream.Tags {
		tags = append(tags, &privatev1.SupermarketTag{Id: tag, Name: tag})
	}
	return connect.NewResponse(&privatev1.ListSupermarketTagsResponse{Tags: tags}), nil
}

func (s *SupermarketService) InstallSupermarketMcp(ctx context.Context, req *connect.Request[privatev1.InstallSupermarketMcpRequest]) (*connect.Response[privatev1.InstallSupermarketMcpResponse], error) {
	botID, err := s.requireBotPermission(ctx, req.Msg.GetBotId(), rbac.PermissionBotUpdate)
	if err != nil {
		return nil, err
	}
	if s.mcp == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("mcp connection service not configured"))
	}
	item, err := s.fetchMcp(ctx, req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	if _, err := s.mcp.Create(ctx, botID, supermarketMcpToUpsert(item, supermarketEnvOptions(req.Msg.GetOptions()))); err != nil {
		return nil, mcpConnectError(err)
	}
	return connect.NewResponse(&privatev1.InstallSupermarketMcpResponse{Item: supermarketMcpToProto(item)}), nil
}

func (s *SupermarketService) InstallSupermarketSkill(ctx context.Context, req *connect.Request[privatev1.InstallSupermarketSkillRequest]) (*connect.Response[privatev1.InstallSupermarketSkillResponse], error) {
	botID, err := s.requireBotPermission(ctx, req.Msg.GetBotId(), rbac.PermissionBotUpdate)
	if err != nil {
		return nil, err
	}
	skillID := strings.TrimSpace(req.Msg.GetId())
	if skillID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}
	if strings.Contains(skillID, "..") || strings.Contains(skillID, "/") {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid id"))
	}
	if s.files == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("skill file client provider not configured"))
	}
	client, err := s.files.SkillClient(ctx, botID)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("container not reachable: %w", err))
	}

	downloadURL := strings.TrimRight(s.baseURL, "/") + "/api/skills/" + url.PathEscape(skillID) + "/download"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	resp, err := s.httpClient.Do(httpReq) //nolint:gosec // catalog base URL is server configuration.
	if err != nil {
		if s.logger != nil {
			s.logger.Error("supermarket skill download failed", slog.String("url", downloadURL), slog.Any("error", err))
		}
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("supermarket unreachable"))
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("skill %q not found in supermarket", skillID))
	}
	if resp.StatusCode != http.StatusOK {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("supermarket returned status %d", resp.StatusCode))
	}

	filesWritten, err := writeSupermarketSkillArchive(ctx, client, skillID, resp.Body)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&privatev1.InstallSupermarketSkillResponse{
		Item: &privatev1.SupermarketItem{
			Id:          skillID,
			Name:        skillID,
			DisplayName: skillID,
			Metadata:    mapToStruct(map[string]any{"files_written": filesWritten}),
		},
	}), nil
}

func (s *SupermarketService) requireBotPermission(ctx context.Context, botID string, permission rbac.PermissionKey) (string, error) {
	botID = strings.TrimSpace(botID)
	if botID == "" {
		return "", connect.NewError(connect.CodeInvalidArgument, errors.New("bot_id is required"))
	}
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return "", connect.NewError(connect.CodeUnauthenticated, err)
	}
	if s.bots == nil {
		return "", connect.NewError(connect.CodeInternal, errors.New("bot service not configured"))
	}
	allowed, err := s.bots.HasBotPermission(ctx, userID, botID, permission)
	if err != nil {
		return "", botConnectError(err)
	}
	if !allowed {
		return "", botConnectError(bots.ErrBotAccessDenied)
	}
	return botID, nil
}

func (s *SupermarketService) fetchMcp(ctx context.Context, id string) (handlers.SupermarketMcpEntry, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return handlers.SupermarketMcpEntry{}, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}
	var item handlers.SupermarketMcpEntry
	err := s.getJSON(ctx, strings.TrimRight(s.baseURL, "/")+"/api/mcps/"+url.PathEscape(id), &item)
	return item, err
}

func (s *SupermarketService) catalogURL(upstreamPath string, query string, tags []string, page *privatev1.PageRequest) (string, error) {
	u, err := url.Parse(strings.TrimRight(s.baseURL, "/") + upstreamPath)
	if err != nil {
		return "", connect.NewError(connect.CodeInternal, err)
	}
	values := u.Query()
	if q := strings.TrimSpace(query); q != "" {
		values.Set("q", q)
	}
	for _, tag := range tags {
		if tag = strings.TrimSpace(tag); tag != "" {
			values.Add("tag", tag)
		}
	}
	if pageSize := page.GetPageSize(); pageSize > 0 {
		values.Set("limit", strconv.Itoa(int(pageSize)))
	}
	if pageToken := strings.TrimSpace(page.GetPageToken()); pageToken != "" {
		values.Set("page", pageToken)
	}
	u.RawQuery = values.Encode()
	return u.String(), nil
}

func (s *SupermarketService) getJSON(ctx context.Context, rawURL string, target any) error {
	client := s.httpClient
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return connect.NewError(connect.CodeInternal, err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req) //nolint:gosec // catalog base URL is server configuration.
	if err != nil {
		if s.logger != nil {
			s.logger.Error("supermarket request failed", slog.String("url", rawURL), slog.Any("error", err))
		}
		return connect.NewError(connect.CodeUnavailable, errors.New("supermarket unreachable"))
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return connect.NewError(connect.CodeNotFound, errors.New("supermarket item not found"))
	}
	if resp.StatusCode != http.StatusOK {
		return connect.NewError(connect.CodeUnavailable, fmt.Errorf("supermarket returned status %d", resp.StatusCode))
	}
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return connect.NewError(connect.CodeUnavailable, errors.New("invalid JSON from supermarket"))
	}
	return nil
}

func supermarketPageResponse(page, limit, total int) *privatev1.PageResponse {
	if limit <= 0 || page <= 0 || page*limit >= total {
		return &privatev1.PageResponse{}
	}
	return &privatev1.PageResponse{NextPageToken: strconv.Itoa(page + 1)}
}

func supermarketMcpToProto(item handlers.SupermarketMcpEntry) *privatev1.SupermarketItem {
	return &privatev1.SupermarketItem{
		Id:          item.ID,
		Name:        item.Name,
		DisplayName: item.Name,
		Description: item.Description,
		Tags:        item.Tags,
		Metadata: mapToStruct(map[string]any{
			"author":    item.Author,
			"transport": item.Transport,
			"icon":      item.Icon,
			"homepage":  item.Homepage,
			"url":       item.URL,
			"command":   item.Command,
			"args":      item.Args,
			"headers":   item.Headers,
			"env":       item.Env,
		}),
	}
}

func supermarketSkillToItemProto(item handlers.SupermarketSkillEntry) *privatev1.SupermarketItem {
	return &privatev1.SupermarketItem{
		Id:          item.ID,
		Name:        item.Name,
		DisplayName: item.Name,
		Description: item.Description,
		Tags:        item.Metadata.Tags,
		Metadata: mapToStruct(map[string]any{
			"author":   item.Metadata.Author,
			"homepage": item.Metadata.Homepage,
			"content":  item.Content,
			"files":    item.Files,
		}),
	}
}

func supermarketMcpToUpsert(entry handlers.SupermarketMcpEntry, envOverrides map[string]string) mcp.UpsertRequest {
	headers := make(map[string]string, len(entry.Headers))
	for _, hdr := range entry.Headers {
		headers[hdr.Key] = hdr.DefaultValue
	}

	env := make(map[string]string, len(entry.Env))
	for _, item := range entry.Env {
		if override, ok := envOverrides[item.Key]; ok {
			env[item.Key] = override
		} else {
			env[item.Key] = item.DefaultValue
		}
	}

	return mcp.UpsertRequest{
		Name:      entry.Name,
		Command:   entry.Command,
		Args:      entry.Args,
		URL:       entry.URL,
		Headers:   headers,
		Env:       env,
		Transport: entry.Transport,
	}
}

func supermarketEnvOptions(options any) map[string]string {
	raw := map[string]any{}
	if value, ok := options.(interface{ AsMap() map[string]any }); ok && value != nil {
		raw = value.AsMap()
	}
	envRaw, _ := raw["env"].(map[string]any)
	env := map[string]string{}
	for key, value := range envRaw {
		if text, ok := value.(string); ok {
			env[key] = text
		}
	}
	return env
}

func writeSupermarketSkillArchive(ctx context.Context, client skillFileClient, skillID string, body io.Reader) (int, error) {
	gz, err := gzip.NewReader(body)
	if err != nil {
		return 0, connect.NewError(connect.CodeUnavailable, errors.New("invalid gzip response from supermarket"))
	}
	defer func() { _ = gz.Close() }()

	skillDir := path.Join(skillset.ManagedDir(), skillID)
	if err := client.Mkdir(ctx, skillDir); err != nil {
		return 0, connect.NewError(connect.CodeInternal, fmt.Errorf("mkdir failed: %w", err))
	}

	tr := tar.NewReader(gz)
	filesWritten := 0
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return 0, connect.NewError(connect.CodeUnavailable, fmt.Errorf("invalid tar: %w", err))
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}

		relativePath := strings.TrimPrefix(hdr.Name, skillID+"/")
		if relativePath == "" || strings.Contains(relativePath, "..") {
			continue
		}

		content, err := io.ReadAll(tr)
		if err != nil {
			return 0, connect.NewError(connect.CodeUnavailable, fmt.Errorf("read tar entry failed: %w", err))
		}

		filePath := path.Join(skillDir, relativePath)
		dir := path.Dir(filePath)
		if dir != skillDir {
			_ = client.Mkdir(ctx, dir)
		}
		if err := client.WriteFile(ctx, filePath, content); err != nil {
			return 0, connect.NewError(connect.CodeInternal, fmt.Errorf("write file %s failed: %w", relativePath, err))
		}
		filesWritten++
	}
	if filesWritten == 0 {
		return 0, connect.NewError(connect.CodeUnavailable, errors.New("skill archive was empty"))
	}
	return filesWritten, nil
}
