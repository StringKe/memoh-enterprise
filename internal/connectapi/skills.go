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
	"github.com/memohai/memoh/internal/iam/rbac"
	skillset "github.com/memohai/memoh/internal/skills"
	"github.com/memohai/memoh/internal/workspace/bridge"
	pb "github.com/memohai/memoh/internal/workspace/bridgepb"
)

type skillFileClient interface {
	ListDirAll(ctx context.Context, filePath string, recursive bool) ([]*pb.FileEntry, error)
	ReadRaw(ctx context.Context, filePath string) (io.ReadCloser, error)
	WriteRaw(ctx context.Context, filePath string, r io.Reader) (int64, error)
	WriteFile(ctx context.Context, filePath string, content []byte) error
	Mkdir(ctx context.Context, filePath string) error
	Stat(ctx context.Context, filePath string) (*pb.FileEntry, error)
	DeleteFile(ctx context.Context, filePath string, recursive bool) error
}

type skillFileClientProvider interface {
	SkillClient(ctx context.Context, botID string) (skillFileClient, error)
}

type SkillRootResolver interface {
	ResolveWorkspaceSkillDiscoveryRoots(ctx context.Context, botID string) ([]string, error)
}

type bridgeSkillClientProvider struct {
	provider bridge.Provider
}

func (p bridgeSkillClientProvider) SkillClient(ctx context.Context, botID string) (skillFileClient, error) {
	if p.provider == nil {
		return nil, errors.New("workspace bridge provider not configured")
	}
	return p.provider.MCPClient(ctx, botID)
}

type SkillService struct {
	bots       *bots.Service
	files      skillFileClientProvider
	roots      SkillRootResolver
	baseURL    string
	httpClient *http.Client
	logger     *slog.Logger
}

func NewSkillService(log *slog.Logger, cfg config.Config, containers bridge.Provider, roots SkillRootResolver, botService *bots.Service) *SkillService {
	if log == nil {
		log = slog.Default()
	}
	return &SkillService{
		bots:       botService,
		files:      bridgeSkillClientProvider{provider: containers},
		roots:      roots,
		baseURL:    cfg.Supermarket.GetBaseURL(),
		httpClient: &http.Client{Timeout: 30 * time.Second},
		logger:     log.With(slog.String("service", "connect_skills")),
	}
}

func NewSkillHandler(service *SkillService) Handler {
	path, handler := privatev1connect.NewSkillServiceHandler(service)
	return NewHandler(path, handler)
}

func (s *SkillService) ListSkills(ctx context.Context, req *connect.Request[privatev1.ListSkillsRequest]) (*connect.Response[privatev1.ListSkillsResponse], error) {
	botID, err := s.requireBotPermission(ctx, req.Msg.GetBotId(), rbac.PermissionBotRead)
	if err != nil {
		return nil, err
	}
	client, roots, err := s.containerClientAndRoots(ctx, botID)
	if err != nil {
		return nil, err
	}
	items, err := skillset.List(ctx, client, roots)
	if err != nil {
		return nil, skillConnectError(err)
	}
	return connect.NewResponse(&privatev1.ListSkillsResponse{Skills: skillsToProto(items)}), nil
}

func (s *SkillService) UpsertSkills(ctx context.Context, req *connect.Request[privatev1.UpsertSkillsRequest]) (*connect.Response[privatev1.UpsertSkillsResponse], error) {
	botID, err := s.requireBotPermission(ctx, req.Msg.GetBotId(), rbac.PermissionBotUpdate)
	if err != nil {
		return nil, err
	}
	if len(req.Msg.GetSkills()) == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("skills is required"))
	}
	client, roots, err := s.containerClientAndRoots(ctx, botID)
	if err != nil {
		return nil, err
	}
	for _, item := range req.Msg.GetSkills() {
		raw, err := skillRawFromProto(item)
		if err != nil {
			return nil, err
		}
		parsed := skillset.ParseFile(raw, item.GetName())
		dirPath, err := skillset.ManagedSkillDirForName(parsed.Name)
		if err != nil {
			return nil, skillConnectError(err)
		}
		if err := client.Mkdir(ctx, dirPath); err != nil {
			return nil, skillConnectError(err)
		}
		if err := client.WriteFile(ctx, path.Join(dirPath, "SKILL.md"), []byte(raw)); err != nil {
			return nil, skillConnectError(err)
		}
	}
	items, err := skillset.List(ctx, client, roots)
	if err != nil {
		return nil, skillConnectError(err)
	}
	return connect.NewResponse(&privatev1.UpsertSkillsResponse{Skills: skillsToProto(items)}), nil
}

func (s *SkillService) DeleteSkills(ctx context.Context, req *connect.Request[privatev1.DeleteSkillsRequest]) (*connect.Response[privatev1.DeleteSkillsResponse], error) {
	botID, err := s.requireBotPermission(ctx, req.Msg.GetBotId(), rbac.PermissionBotUpdate)
	if err != nil {
		return nil, err
	}
	if len(req.Msg.GetSkillIds()) == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("skill_ids is required"))
	}
	client, _, err := s.containerClientAndRoots(ctx, botID)
	if err != nil {
		return nil, err
	}
	for _, id := range req.Msg.GetSkillIds() {
		dirPath, err := skillset.ManagedSkillDirForName(id)
		if err != nil {
			return nil, skillConnectError(err)
		}
		if _, err := client.Stat(ctx, dirPath); err != nil {
			return nil, skillConnectError(err)
		}
		if err := client.DeleteFile(ctx, dirPath, true); err != nil {
			return nil, skillConnectError(err)
		}
	}
	return connect.NewResponse(&privatev1.DeleteSkillsResponse{}), nil
}

func (s *SkillService) ApplySkillAction(ctx context.Context, req *connect.Request[privatev1.ApplySkillActionRequest]) (*connect.Response[privatev1.ApplySkillActionResponse], error) {
	botID, err := s.requireBotPermission(ctx, req.Msg.GetBotId(), rbac.PermissionBotUpdate)
	if err != nil {
		return nil, err
	}
	targetPath := skillActionTargetPath(req.Msg)
	if strings.TrimSpace(targetPath) == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("skill_id or payload.target_path is required"))
	}
	client, roots, err := s.containerClientAndRoots(ctx, botID)
	if err != nil {
		return nil, err
	}
	if err := skillset.ApplyAction(ctx, client, roots, skillset.ActionRequest{
		Action:     req.Msg.GetAction(),
		TargetPath: targetPath,
	}); err != nil {
		return nil, skillConnectError(err)
	}
	items, err := skillset.List(ctx, client, roots)
	if err != nil {
		return nil, skillConnectError(err)
	}
	return connect.NewResponse(&privatev1.ApplySkillActionResponse{
		Skill:  findSkillProto(items, targetPath),
		Result: mapToStruct(map[string]any{"ok": true}),
	}), nil
}

func (s *SkillService) ListSkillCatalog(ctx context.Context, req *connect.Request[privatev1.ListSkillCatalogRequest]) (*connect.Response[privatev1.ListSkillCatalogResponse], error) {
	if _, err := UserIDFromContext(ctx); err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	source, err := normalizeSkillCatalogSource(req.Msg.GetSource())
	if err != nil {
		return nil, err
	}
	u, err := url.Parse(strings.TrimRight(s.catalogBaseURL(source), "/") + "/api/skills")
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	query := u.Query()
	if pageSize := req.Msg.GetPage().GetPageSize(); pageSize > 0 {
		query.Set("limit", strconv.Itoa(int(pageSize)))
	}
	if pageToken := strings.TrimSpace(req.Msg.GetPage().GetPageToken()); pageToken != "" {
		query.Set("page", pageToken)
	}
	u.RawQuery = query.Encode()

	var upstream supermarketSkillListResponse
	if err := s.getCatalogJSON(ctx, u.String(), &upstream); err != nil {
		return nil, err
	}
	out := make([]*privatev1.Skill, 0, len(upstream.Data))
	for _, item := range upstream.Data {
		out = append(out, supermarketSkillToProto(source, item))
	}
	return connect.NewResponse(&privatev1.ListSkillCatalogResponse{
		Skills: out,
		Page:   &privatev1.PageResponse{NextPageToken: nextCatalogPageToken(upstream.Page, upstream.Limit, upstream.Total)},
	}), nil
}

func (s *SkillService) InstallSkill(ctx context.Context, req *connect.Request[privatev1.InstallSkillRequest]) (*connect.Response[privatev1.InstallSkillResponse], error) {
	botID, err := s.requireBotPermission(ctx, req.Msg.GetBotId(), rbac.PermissionBotUpdate)
	if err != nil {
		return nil, err
	}
	source, err := normalizeSkillCatalogSource(req.Msg.GetSource())
	if err != nil {
		return nil, err
	}
	skillID := strings.TrimSpace(req.Msg.GetSkillId())
	if _, err := skillset.ManagedSkillDirForName(skillID); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid skill_id"))
	}
	client, roots, err := s.containerClientAndRoots(ctx, botID)
	if err != nil {
		return nil, err
	}
	if err := s.downloadCatalogSkill(ctx, client, source, skillID); err != nil {
		return nil, err
	}
	items, err := skillset.List(ctx, client, roots)
	if err != nil {
		return nil, skillConnectError(err)
	}
	return connect.NewResponse(&privatev1.InstallSkillResponse{Skill: findSkillByNameProto(items, skillID)}), nil
}

func (s *SkillService) containerClientAndRoots(ctx context.Context, botID string) (skillFileClient, []string, error) {
	if s.files == nil {
		return nil, nil, connect.NewError(connect.CodeInternal, errors.New("skill file client provider not configured"))
	}
	client, err := s.files.SkillClient(ctx, botID)
	if err != nil {
		return nil, nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("container not reachable: %w", err))
	}
	var roots []string
	if s.roots != nil {
		roots, err = s.roots.ResolveWorkspaceSkillDiscoveryRoots(ctx, botID)
		if err != nil {
			return nil, nil, connect.NewError(connect.CodeInternal, err)
		}
	}
	return client, roots, nil
}

func (s *SkillService) requireBotPermission(ctx context.Context, botID string, permission rbac.PermissionKey) (string, error) {
	botID = strings.TrimSpace(botID)
	if botID == "" {
		return "", connect.NewError(connect.CodeInvalidArgument, errors.New("bot_id is required"))
	}
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return "", connect.NewError(connect.CodeUnauthenticated, err)
	}
	if s.bots == nil {
		return botID, nil
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

func (s *SkillService) catalogBaseURL(source string) string {
	if source == "" || source == "supermarket" {
		return s.baseURL
	}
	return source
}

func (s *SkillService) getCatalogJSON(ctx context.Context, rawURL string, target any) error {
	client := s.httpClient
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return connect.NewError(connect.CodeInternal, err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req) //nolint:gosec // catalog base URL is server configuration or explicit admin input.
	if err != nil {
		if s.logger != nil {
			s.logger.Error("skill catalog request failed", slog.String("url", rawURL), slog.Any("error", err))
		}
		return connect.NewError(connect.CodeUnavailable, errors.New("skill catalog unreachable"))
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return connect.NewError(connect.CodeNotFound, errors.New("skill catalog item not found"))
	}
	if resp.StatusCode != http.StatusOK {
		return connect.NewError(connect.CodeUnavailable, fmt.Errorf("skill catalog returned status %d", resp.StatusCode))
	}
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return connect.NewError(connect.CodeUnavailable, errors.New("invalid skill catalog JSON"))
	}
	return nil
}

func (s *SkillService) downloadCatalogSkill(ctx context.Context, client skillFileClient, source, skillID string) error {
	downloadURL := strings.TrimRight(s.catalogBaseURL(source), "/") + "/api/skills/" + skillID + "/download"
	httpClient := s.httpClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return connect.NewError(connect.CodeInternal, err)
	}
	resp, err := httpClient.Do(req) //nolint:gosec // catalog base URL is server configuration or explicit admin input.
	if err != nil {
		if s.logger != nil {
			s.logger.Error("skill download failed", slog.String("url", downloadURL), slog.Any("error", err))
		}
		return connect.NewError(connect.CodeUnavailable, errors.New("skill catalog unreachable"))
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return connect.NewError(connect.CodeNotFound, fmt.Errorf("skill %q not found in catalog", skillID))
	}
	if resp.StatusCode != http.StatusOK {
		return connect.NewError(connect.CodeUnavailable, fmt.Errorf("skill catalog returned status %d", resp.StatusCode))
	}
	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return connect.NewError(connect.CodeUnavailable, errors.New("invalid skill archive gzip"))
	}
	defer func() { _ = gz.Close() }()

	skillDir, err := skillset.ManagedSkillDirForName(skillID)
	if err != nil {
		return skillConnectError(err)
	}
	if err := client.Mkdir(ctx, skillDir); err != nil {
		return skillConnectError(err)
	}
	tr := tar.NewReader(gz)
	filesWritten := 0
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return connect.NewError(connect.CodeUnavailable, fmt.Errorf("invalid skill archive tar: %w", err))
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		relativePath := cleanCatalogSkillArchivePath(skillID, hdr.Name)
		if relativePath == "" {
			continue
		}
		content, err := io.ReadAll(tr)
		if err != nil {
			return connect.NewError(connect.CodeUnavailable, fmt.Errorf("read skill archive entry failed: %w", err))
		}
		filePath := path.Join(skillDir, relativePath)
		if dir := path.Dir(filePath); dir != skillDir {
			if err := client.Mkdir(ctx, dir); err != nil {
				return skillConnectError(err)
			}
		}
		if err := client.WriteFile(ctx, filePath, content); err != nil {
			return skillConnectError(err)
		}
		filesWritten++
	}
	if filesWritten == 0 {
		return connect.NewError(connect.CodeUnavailable, errors.New("skill archive was empty"))
	}
	return nil
}

func skillsToProto(items []skillset.Entry) []*privatev1.Skill {
	out := make([]*privatev1.Skill, 0, len(items))
	for _, item := range items {
		out = append(out, skillEntryToProto(item))
	}
	return out
}

func skillEntryToProto(item skillset.Entry) *privatev1.Skill {
	metadata := map[string]any{
		"content":     item.Content,
		"raw":         item.Raw,
		"source_path": item.SourcePath,
		"source_root": item.SourceRoot,
		"source_kind": item.SourceKind,
		"managed":     item.Managed,
		"state":       item.State,
	}
	for key, value := range item.Metadata {
		metadata[key] = value
	}
	if strings.TrimSpace(item.ShadowedBy) != "" {
		metadata["shadowed_by"] = item.ShadowedBy
	}
	return &privatev1.Skill{
		Id:          item.Name,
		Name:        item.Name,
		Source:      item.SourcePath,
		Description: item.Description,
		Enabled:     item.State == skillset.StateEffective,
		Metadata:    mapToStruct(metadata),
	}
}

func skillRawFromProto(item *privatev1.Skill) (string, error) {
	if item == nil {
		return "", connect.NewError(connect.CodeInvalidArgument, errors.New("skill is required"))
	}
	metadata := structToMap(item.GetMetadata())
	if raw, _ := metadata["raw"].(string); strings.TrimSpace(raw) != "" {
		return raw, nil
	}
	content, _ := metadata["content"].(string)
	if strings.TrimSpace(content) == "" {
		content = item.GetDescription()
	}
	name := strings.TrimSpace(item.GetName())
	if name == "" {
		name = strings.TrimSpace(item.GetId())
	}
	if !skillset.IsValidName(name) {
		return "", connect.NewError(connect.CodeInvalidArgument, errors.New("skill must have a valid name"))
	}
	description := strings.TrimSpace(item.GetDescription())
	if description == "" {
		description = name
	}
	return fmt.Sprintf("---\nname: %s\ndescription: %s\n---\n%s\n", name, description, strings.TrimSpace(content)), nil
}

func skillActionTargetPath(req *privatev1.ApplySkillActionRequest) string {
	payload := structToMap(req.GetPayload())
	if value, _ := payload["target_path"].(string); strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return strings.TrimSpace(req.GetSkillId())
}

func findSkillProto(items []skillset.Entry, targetPath string) *privatev1.Skill {
	for _, item := range items {
		if item.SourcePath == targetPath || item.ShadowedBy == targetPath {
			return skillEntryToProto(item)
		}
	}
	return nil
}

func findSkillByNameProto(items []skillset.Entry, name string) *privatev1.Skill {
	for _, item := range items {
		if item.Name == name && item.Managed {
			return skillEntryToProto(item)
		}
	}
	for _, item := range items {
		if item.Name == name {
			return skillEntryToProto(item)
		}
	}
	return &privatev1.Skill{
		Id:      name,
		Name:    name,
		Source:  path.Join(skillset.ManagedDir(), name, "SKILL.md"),
		Enabled: true,
		Metadata: mapToStruct(map[string]any{
			"source_kind": skillset.SourceKindManaged,
			"managed":     true,
			"state":       skillset.StateEffective,
		}),
	}
}

type supermarketSkillAuthor struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type supermarketSkillMetadata struct {
	Author   supermarketSkillAuthor `json:"author"`
	Tags     []string               `json:"tags,omitempty"`
	Homepage string                 `json:"homepage,omitempty"`
}

type supermarketSkillEntry struct {
	ID          string                   `json:"id"`
	Name        string                   `json:"name"`
	Description string                   `json:"description"`
	Metadata    supermarketSkillMetadata `json:"metadata"`
	Content     string                   `json:"content"`
	Files       []string                 `json:"files"`
}

type supermarketSkillListResponse struct {
	Total int                     `json:"total"`
	Page  int                     `json:"page"`
	Limit int                     `json:"limit"`
	Data  []supermarketSkillEntry `json:"data"`
}

func supermarketSkillToProto(source string, item supermarketSkillEntry) *privatev1.Skill {
	metadata := map[string]any{
		"author":   map[string]any{"name": item.Metadata.Author.Name, "email": item.Metadata.Author.Email},
		"tags":     stringSliceToAny(item.Metadata.Tags),
		"homepage": item.Metadata.Homepage,
		"content":  item.Content,
		"files":    stringSliceToAny(item.Files),
	}
	return &privatev1.Skill{
		Id:          item.ID,
		Name:        item.Name,
		Source:      source,
		Description: item.Description,
		Enabled:     true,
		Metadata:    mapToStruct(metadata),
	}
}

func nextCatalogPageToken(page, limit, total int) string {
	if page <= 0 || limit <= 0 || page*limit >= total {
		return ""
	}
	return strconv.Itoa(page + 1)
}

func normalizeSkillCatalogSource(source string) (string, error) {
	source = strings.TrimSpace(source)
	if source == "" || source == "supermarket" {
		return "supermarket", nil
	}
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		return source, nil
	}
	return "", connect.NewError(connect.CodeInvalidArgument, errors.New("unsupported skill catalog source"))
}

func cleanCatalogSkillArchivePath(skillID, archiveName string) string {
	name := strings.TrimSpace(archiveName)
	if name == "" {
		return ""
	}
	name = strings.TrimPrefix(name, skillID+"/")
	name = path.Clean("/" + name)
	if name == "/" || strings.HasPrefix(name, "/../") {
		return ""
	}
	name = strings.TrimPrefix(name, "/")
	if name == "" || strings.Contains(name, "..") {
		return ""
	}
	return name
}

func skillConnectError(err error) error {
	switch {
	case errors.Is(err, bridge.ErrNotFound):
		return connect.NewError(connect.CodeNotFound, err)
	case errors.Is(err, bridge.ErrBadRequest):
		return connect.NewError(connect.CodeInvalidArgument, err)
	case errors.Is(err, bridge.ErrForbidden):
		return connect.NewError(connect.CodePermissionDenied, err)
	case errors.Is(err, bridge.ErrUnavailable):
		return connect.NewError(connect.CodeUnavailable, err)
	default:
		return connect.NewError(connect.CodeInternal, err)
	}
}
