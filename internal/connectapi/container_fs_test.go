package connectapi

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"

	privatev1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1"
	"github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1/privatev1connect"
	"github.com/memohai/memoh/internal/connectapi/gen/memoh/workspace/v1/workspacev1connect"
	ctr "github.com/memohai/memoh/internal/container"
	"github.com/memohai/memoh/internal/handlers"
	"github.com/memohai/memoh/internal/workspace"
	"github.com/memohai/memoh/internal/workspace/executorclient"
	"github.com/memohai/memoh/internal/workspace/executorsvc"
)

func TestContainerFileRPCsUseWorkspaceExecutor(t *testing.T) {
	client, provider := newContainerRuntimeTestClient(t)

	if _, err := client.MkdirContainerFile(context.Background(), connect.NewRequest(&privatev1.MkdirContainerFileRequest{
		BotId: "bot-1",
		Path:  "notes",
	})); err != nil {
		t.Fatal(err)
	}
	write, err := client.WriteContainerFile(context.Background(), connect.NewRequest(&privatev1.WriteContainerFileRequest{
		BotId:   "bot-1",
		Path:    "notes/a.txt",
		Content: []byte("hello"),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if write.Msg.GetBytesWritten() != 5 {
		t.Fatalf("bytes_written = %d", write.Msg.GetBytesWritten())
	}
	upload, err := client.UploadContainerFile(context.Background(), connect.NewRequest(&privatev1.UploadContainerFileRequest{
		BotId:   "bot-1",
		Path:    "notes/b.bin",
		Content: []byte{0, 1, 2},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if upload.Msg.GetBytesWritten() != 3 {
		t.Fatalf("upload bytes_written = %d", upload.Msg.GetBytesWritten())
	}

	list, err := client.ListContainerFiles(context.Background(), connect.NewRequest(&privatev1.ListContainerFilesRequest{
		BotId:    "bot-1",
		Path:     "notes",
		PageSize: 1,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if got := list.Msg.GetEntries(); len(got) != 1 || got[0].GetPath() == "" {
		t.Fatalf("list entries = %#v", got)
	}
	if list.Msg.GetNextPageToken() != "1" {
		t.Fatalf("next_page_token = %q", list.Msg.GetNextPageToken())
	}

	read, err := client.ReadContainerFile(context.Background(), connect.NewRequest(&privatev1.ReadContainerFileRequest{
		BotId:    "bot-1",
		Path:     "notes/a.txt",
		MaxBytes: 16,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if string(read.Msg.GetContent()) != "hello" || !read.Msg.GetEof() {
		t.Fatalf("read response = %#v", read.Msg)
	}
	download, err := client.DownloadContainerFile(context.Background(), connect.NewRequest(&privatev1.DownloadContainerFileRequest{
		BotId: "bot-1",
		Path:  "notes/b.bin",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if string(download.Msg.GetContent()) != string([]byte{0, 1, 2}) || download.Msg.GetFilename() != "b.bin" {
		t.Fatalf("download response = %#v", download.Msg)
	}

	if _, err := client.RenameContainerFile(context.Background(), connect.NewRequest(&privatev1.RenameContainerFileRequest{
		BotId:   "bot-1",
		OldPath: "notes/a.txt",
		NewPath: "notes/c.txt",
	})); err != nil {
		t.Fatal(err)
	}
	if _, err := client.DeleteContainerFile(context.Background(), connect.NewRequest(&privatev1.DeleteContainerFileRequest{
		BotId: "bot-1",
		Path:  "notes/c.txt",
	})); err != nil {
		t.Fatal(err)
	}
	if provider.executorCalls == 0 {
		t.Fatal("workspace executor provider was not used")
	}
}

func newContainerRuntimeTestClient(t *testing.T) (privatev1connect.ContainerServiceClient, *containerRuntimeTestProvider) {
	t.Helper()
	root := t.TempDir()
	_, executorHandler := workspacev1connect.NewWorkspaceExecutorServiceHandler(executorsvc.New(executorsvc.Options{
		DefaultWorkDir:    root,
		WorkspaceRoot:     root,
		AllowHostAbsolute: true,
	}))
	executorServer := httptest.NewServer(executorHandler)
	t.Cleanup(executorServer.Close)
	executor := executorclient.NewClient(workspacev1connect.NewWorkspaceExecutorServiceClient(executorServer.Client(), executorServer.URL), nil)
	provider := &containerRuntimeTestProvider{
		client: executor,
		info: executorclient.WorkspaceInfo{
			Backend:        executorclient.WorkspaceBackendLocal,
			DefaultWorkDir: root,
		},
		metrics: &workspace.ContainerMetricsResult{
			Supported: true,
			Status:    workspace.ContainerMetricsStatus{Exists: true, TaskRunning: true},
			SampledAt: time.Date(2026, 5, 6, 10, 0, 0, 0, time.UTC),
			CPU:       &ctr.CPUMetrics{UsagePercent: 12.5},
			Memory:    &ctr.MemoryMetrics{UsageBytes: 1024, LimitBytes: 4096},
		},
		snapshot: &workspace.SnapshotCreateInfo{
			SnapshotName:        "manual",
			RuntimeSnapshotName: "runtime-snapshot-1",
			DisplayName:         "Manual",
			Snapshotter:         "overlayfs",
			Version:             7,
			CreatedAt:           time.Date(2026, 5, 6, 10, 1, 0, 0, time.UTC),
		},
		snapshotData: &workspace.BotSnapshotData{
			RuntimeSnapshots: []ctr.SnapshotInfo{{
				Name:    "runtime-snapshot-1",
				Kind:    "committed",
				Created: time.Date(2026, 5, 6, 10, 1, 0, 0, time.UTC),
			}},
			ManagedMeta: map[string]workspace.ManagedSnapshotMeta{
				"runtime-snapshot-1": {Source: workspace.SnapshotSourceManual, Version: intPtr(7), DisplayName: "Manual"},
			},
		},
	}
	service := &ContainerService{
		creator:   &instantContainerCreator{},
		bots:      fakeContainerAuthorizer{},
		executors: provider,
		runtime:   provider,
		terminals: make(map[string]*terminalSession),
	}
	client, cleanup := newContainerServiceTestClient(t, service)
	t.Cleanup(cleanup)
	return client, provider
}

type containerRuntimeTestProvider struct {
	client        *executorclient.Client
	info          executorclient.WorkspaceInfo
	metrics       *workspace.ContainerMetricsResult
	snapshot      *workspace.SnapshotCreateInfo
	snapshotData  *workspace.BotSnapshotData
	executorCalls int
	stopped       bool
	rollback      int
}

func (p *containerRuntimeTestProvider) ExecutorClient(context.Context, string) (*executorclient.Client, error) {
	p.executorCalls++
	return p.client, nil
}

func (p *containerRuntimeTestProvider) WorkspaceInfo(context.Context, string) (executorclient.WorkspaceInfo, error) {
	return p.info, nil
}

func (p *containerRuntimeTestProvider) StopBot(context.Context, string) error {
	p.stopped = true
	return nil
}

func (p *containerRuntimeTestProvider) GetContainerMetrics(context.Context, string) (*workspace.ContainerMetricsResult, error) {
	return p.metrics, nil
}

func (p *containerRuntimeTestProvider) CreateSnapshot(context.Context, string, string, string) (*workspace.SnapshotCreateInfo, error) {
	return p.snapshot, nil
}

func (p *containerRuntimeTestProvider) ListBotSnapshotData(context.Context, string) (*workspace.BotSnapshotData, error) {
	return p.snapshotData, nil
}

func (p *containerRuntimeTestProvider) RollbackVersion(_ context.Context, _ string, version int) error {
	p.rollback = version
	return nil
}

func intPtr(value int) *int {
	return &value
}

type instantContainerCreator struct {
	req handlers.CreateContainerRequest
}

func (c *instantContainerCreator) CreateContainerStream(_ context.Context, _ string, req handlers.CreateContainerRequest, send func(any)) error {
	c.req = req
	send(map[string]any{"type": "done"})
	return nil
}
