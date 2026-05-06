package workspace

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"github.com/memohai/memoh/internal/config"
	"github.com/memohai/memoh/internal/connectapi/gen/memoh/workspace/v1/workspacev1connect"
	ctr "github.com/memohai/memoh/internal/container"
	"github.com/memohai/memoh/internal/workspace/executorclient"
	"github.com/memohai/memoh/internal/workspace/executorsvc"
)

const (
	LocalContainerPrefix = "local-"
	localRuntimeName     = "local"
)

var unsafeWorkspaceName = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

type LocalService struct {
	cfg          config.LocalConfig
	dataRoot     string
	logger       *slog.Logger
	mu           sync.Mutex
	localClients map[string]*localExecutorClient
}

type localContainerMetadata struct {
	ID            string            `json:"id"`
	BotID         string            `json:"bot_id"`
	Image         string            `json:"image"`
	Labels        map[string]string `json:"labels"`
	WorkspacePath string            `json:"workspace_path"`
	Status        string            `json:"status"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
}

type localExecutorClient struct {
	client *executorclient.Client
	server *httptest.Server
}

func NewLocalService(log *slog.Logger, cfg config.LocalConfig, dataRoot string) *LocalService {
	if log == nil {
		log = slog.Default()
	}
	return &LocalService{
		cfg:          cfg,
		dataRoot:     dataRoot,
		logger:       log.With(slog.String("service", "local-workspace")),
		localClients: make(map[string]*localExecutorClient),
	}
}

func (s *LocalService) Enabled() bool {
	return s != nil && s.cfg.Enabled
}

func (s *LocalService) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, client := range s.localClients {
		client.close()
		delete(s.localClients, id)
	}
}

func (s *LocalService) CreateContainer(ctx context.Context, req ctr.CreateContainerRequest) (ctr.ContainerInfo, error) {
	if !s.Enabled() {
		return ctr.ContainerInfo{}, ctr.ErrNotSupported
	}
	if strings.TrimSpace(req.ID) == "" {
		return ctr.ContainerInfo{}, ctr.ErrInvalidArgument
	}
	botID, ok := BotIDFromContainerID(req.ID)
	if !ok {
		botID = strings.TrimSpace(req.Labels[BotLabelKey])
	}
	if strings.TrimSpace(botID) == "" {
		return ctr.ContainerInfo{}, ctr.ErrInvalidArgument
	}
	path := strings.TrimSpace(req.StorageRef.Key)
	if path == "" {
		path = s.DefaultWorkspacePath(botID, botID)
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return ctr.ContainerInfo{}, err
	}
	if err := os.MkdirAll(absPath, 0o750); err != nil {
		return ctr.ContainerInfo{}, err
	}
	if err := seedExecutorTemplates(absPath); err != nil {
		return ctr.ContainerInfo{}, err
	}
	if err := os.MkdirAll(s.metadataRoot(), 0o750); err != nil {
		return ctr.ContainerInfo{}, err
	}

	now := time.Now()
	existing, err := s.readMetadata(req.ID)
	switch {
	case err == nil:
		existing.Image = req.ImageRef
		existing.Labels = cloneStringMap(req.Labels)
		existing.WorkspacePath = absPath
		existing.UpdatedAt = now
		if err := s.writeMetadata(existing); err != nil {
			return ctr.ContainerInfo{}, err
		}
		return s.GetContainer(ctx, req.ID)
	case !ctr.IsNotFound(err):
		return ctr.ContainerInfo{}, err
	}

	meta := localContainerMetadata{
		ID:            req.ID,
		BotID:         botID,
		Image:         req.ImageRef,
		Labels:        cloneStringMap(req.Labels),
		WorkspacePath: absPath,
		Status:        ctr.TaskStatusCreated.String(),
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if meta.Labels == nil {
		meta.Labels = map[string]string{}
	}
	meta.Labels[BotLabelKey] = botID
	meta.Labels[WorkspaceLabelKey] = WorkspaceLabelValue
	if err := s.writeMetadata(meta); err != nil {
		return ctr.ContainerInfo{}, err
	}
	return s.GetContainer(ctx, req.ID)
}

func (s *LocalService) GetContainer(_ context.Context, id string) (ctr.ContainerInfo, error) {
	meta, err := s.readMetadata(id)
	if err != nil {
		return ctr.ContainerInfo{}, err
	}
	return meta.containerInfo(), nil
}

func (s *LocalService) ListContainers(_ context.Context) ([]ctr.ContainerInfo, error) {
	metas, err := s.listMetadata()
	if err != nil {
		return nil, err
	}
	out := make([]ctr.ContainerInfo, 0, len(metas))
	for _, meta := range metas {
		out = append(out, meta.containerInfo())
	}
	return out, nil
}

func (s *LocalService) DeleteContainer(_ context.Context, id string, opts *ctr.DeleteContainerOptions) error {
	meta, err := s.readMetadata(id)
	if err != nil {
		return err
	}
	s.closeClient(meta.BotID)
	if opts != nil && opts.CleanupSnapshot {
		if err := os.RemoveAll(meta.WorkspacePath); err != nil {
			return err
		}
	}
	if err := os.Remove(s.metadataPath(id)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s *LocalService) ListContainersByLabel(_ context.Context, key, value string) ([]ctr.ContainerInfo, error) {
	if strings.TrimSpace(key) == "" {
		return nil, ctr.ErrInvalidArgument
	}
	metas, err := s.listMetadata()
	if err != nil {
		return nil, err
	}
	out := []ctr.ContainerInfo{}
	for _, meta := range metas {
		got := strings.TrimSpace(meta.Labels[key])
		if got == "" {
			continue
		}
		if strings.TrimSpace(value) != "" && got != strings.TrimSpace(value) {
			continue
		}
		out = append(out, meta.containerInfo())
	}
	return out, nil
}

func (s *LocalService) StartContainer(_ context.Context, id string, _ *ctr.StartTaskOptions) error {
	return s.updateStatus(id, ctr.TaskStatusRunning.String())
}

func (s *LocalService) StopContainer(_ context.Context, id string, _ *ctr.StopTaskOptions) error {
	meta, err := s.readMetadata(id)
	if err != nil {
		return err
	}
	s.closeClient(meta.BotID)
	return s.updateStatus(id, ctr.TaskStatusStopped.String())
}

func (s *LocalService) DeleteTask(_ context.Context, id string, _ *ctr.DeleteTaskOptions) error {
	return s.updateStatus(id, ctr.TaskStatusStopped.String())
}

func (s *LocalService) GetTaskInfo(_ context.Context, id string) (ctr.TaskInfo, error) {
	meta, err := s.readMetadata(id)
	if err != nil {
		return ctr.TaskInfo{}, err
	}
	return ctr.TaskInfo{
		ContainerID: meta.ID,
		ID:          meta.ID,
		PID:         uint32(os.Getpid()), //nolint:gosec // PID is informational only.
		Status:      localTaskStatus(meta.Status),
	}, nil
}

func (*LocalService) GetContainerMetrics(context.Context, string) (ctr.ContainerMetrics, error) {
	return ctr.ContainerMetrics{}, ctr.ErrNotSupported
}

func (s *LocalService) ListTasks(_ context.Context, opts *ctr.ListTasksOptions) ([]ctr.TaskInfo, error) {
	metas, err := s.listMetadata()
	if err != nil {
		return nil, err
	}
	out := []ctr.TaskInfo{}
	for _, meta := range metas {
		if opts != nil && strings.TrimSpace(opts.ContainerID) != "" && opts.ContainerID != meta.ID {
			continue
		}
		out = append(out, ctr.TaskInfo{
			ContainerID: meta.ID,
			ID:          meta.ID,
			PID:         uint32(os.Getpid()), //nolint:gosec // PID is informational only.
			Status:      localTaskStatus(meta.Status),
		})
	}
	return out, nil
}

func (*LocalService) SetupNetwork(context.Context, ctr.NetworkRequest) (ctr.NetworkResult, error) {
	return ctr.NetworkResult{IP: "127.0.0.1"}, nil
}

func (*LocalService) RemoveNetwork(context.Context, ctr.NetworkRequest) error { return nil }
func (*LocalService) CheckNetwork(context.Context, ctr.NetworkRequest) error  { return nil }

func (*LocalService) CommitSnapshot(context.Context, ctr.CommitSnapshotRequest) error {
	return ctr.ErrNotSupported
}

func (*LocalService) ListSnapshots(context.Context, ctr.ListSnapshotsRequest) ([]ctr.SnapshotInfo, error) {
	return nil, ctr.ErrNotSupported
}

func (*LocalService) PrepareSnapshot(context.Context, ctr.PrepareSnapshotRequest) error {
	return ctr.ErrNotSupported
}

func (*LocalService) RestoreContainer(context.Context, ctr.CreateContainerRequest) (ctr.ContainerInfo, error) {
	return ctr.ContainerInfo{}, ctr.ErrNotSupported
}

func (*LocalService) SnapshotSupported(context.Context) bool {
	return false
}

func (s *LocalService) ExecutorClient(ctx context.Context, botID string) (*executorclient.Client, error) {
	meta, err := s.readMetadata(LocalContainerPrefix + strings.TrimSpace(botID))
	if err != nil {
		return nil, err
	}
	if localTaskStatus(meta.Status) != ctr.TaskStatusRunning {
		if err := s.updateStatus(meta.ID, ctr.TaskStatusRunning.String()); err != nil {
			return nil, err
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if cached, ok := s.localClients[botID]; ok {
		return cached.client, nil
	}
	client, err := s.newExecutorClient(ctx, meta)
	if err != nil {
		return nil, err
	}
	s.localClients[botID] = client
	return client.client, nil
}

func (s *LocalService) WorkspaceInfo(_ context.Context, botID string) (executorclient.WorkspaceInfo, error) {
	meta, err := s.readMetadata(LocalContainerPrefix + strings.TrimSpace(botID))
	if err != nil {
		return executorclient.WorkspaceInfo{}, err
	}
	return executorclient.WorkspaceInfo{
		Backend:        executorclient.WorkspaceBackendLocal,
		DefaultWorkDir: meta.WorkspacePath,
	}, nil
}

func (s *LocalService) DefaultWorkspacePath(botID, displayName string) string {
	name := strings.TrimSpace(displayName)
	if name == "" {
		name = botID
	}
	name = strings.Trim(unsafeWorkspaceName.ReplaceAllString(name, "-"), ".-")
	if name == "" {
		name = botID
	}
	return filepath.Join(s.cfg.WorkspaceParent(), name)
}

func (s *LocalService) newExecutorClient(ctx context.Context, meta localContainerMetadata) (*localExecutorClient, error) {
	mux := http.NewServeMux()
	path, handler := workspacev1connect.NewWorkspaceExecutorServiceHandler(executorsvc.New(executorsvc.Options{
		DefaultWorkDir:    meta.WorkspacePath,
		WorkspaceRoot:     meta.WorkspacePath,
		DataMount:         config.DefaultDataMount,
		AllowHostAbsolute: s.cfg.AllowAbsolutePaths,
	}))
	mux.Handle(path, handler)
	server := httptest.NewServer(h2c.NewHandler(mux, &http2.Server{}))
	client, err := executorclient.Dial(ctx, server.URL)
	if err != nil {
		server.Close()
		return nil, err
	}
	return &localExecutorClient{
		client: client,
		server: server,
	}, nil
}

func (s *LocalService) closeClient(botID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if client, ok := s.localClients[botID]; ok {
		client.close()
		delete(s.localClients, botID)
	}
}

func (c *localExecutorClient) close() {
	if c == nil {
		return
	}
	if c.client != nil {
		_ = c.client.Close()
	}
	if c.server != nil {
		c.server.Close()
	}
}

func (s *LocalService) metadataRoot() string {
	return s.cfg.MetadataPath(s.dataRoot)
}

func (s *LocalService) metadataPath(id string) string {
	name := strings.NewReplacer("/", "_", "\\", "_", ":", "_").Replace(id)
	return filepath.Join(s.metadataRoot(), name+".json")
}

func (s *LocalService) readMetadata(id string) (localContainerMetadata, error) {
	if strings.TrimSpace(id) == "" {
		return localContainerMetadata{}, ctr.ErrInvalidArgument
	}
	data, err := os.ReadFile(s.metadataPath(id))
	if err != nil {
		if os.IsNotExist(err) {
			return localContainerMetadata{}, ctr.ErrNotFound
		}
		return localContainerMetadata{}, err
	}
	var meta localContainerMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return localContainerMetadata{}, err
	}
	return meta, nil
}

func (s *LocalService) writeMetadata(meta localContainerMetadata) error {
	meta.UpdatedAt = time.Now()
	if meta.CreatedAt.IsZero() {
		meta.CreatedAt = meta.UpdatedAt
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.metadataRoot(), 0o750); err != nil {
		return err
	}
	return os.WriteFile(s.metadataPath(meta.ID), data, 0o600)
}

func (s *LocalService) listMetadata() ([]localContainerMetadata, error) {
	entries, err := os.ReadDir(s.metadataRoot())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	out := []localContainerMetadata{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.metadataRoot(), entry.Name()))
		if err != nil {
			continue
		}
		var meta localContainerMetadata
		if err := json.Unmarshal(data, &meta); err != nil {
			continue
		}
		out = append(out, meta)
	}
	return out, nil
}

func (s *LocalService) updateStatus(id, status string) error {
	meta, err := s.readMetadata(id)
	if err != nil {
		return err
	}
	meta.Status = status
	return s.writeMetadata(meta)
}

func (m localContainerMetadata) containerInfo() ctr.ContainerInfo {
	return ctr.ContainerInfo{
		ID:     m.ID,
		Image:  m.Image,
		Labels: cloneStringMap(m.Labels),
		StorageRef: ctr.StorageRef{
			Driver: localRuntimeName,
			Key:    m.WorkspacePath,
			Kind:   "directory",
		},
		Runtime:   ctr.RuntimeInfo{Name: localRuntimeName},
		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
	}
}

func localTaskStatus(value string) ctr.TaskStatus {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case ctr.TaskStatusRunning.String():
		return ctr.TaskStatusRunning
	case ctr.TaskStatusStopped.String():
		return ctr.TaskStatusStopped
	case ctr.TaskStatusPaused.String():
		return ctr.TaskStatusPaused
	case ctr.TaskStatusCreated.String():
		return ctr.TaskStatusCreated
	default:
		return ctr.TaskStatusUnknown
	}
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
