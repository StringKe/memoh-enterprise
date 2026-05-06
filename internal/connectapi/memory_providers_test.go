package connectapi

import (
	"context"
	"errors"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	privatev1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1"
	memprovider "github.com/memohai/memoh/internal/memory/adapters"
)

type fakeMemoryProviderAdmin struct {
	createReq memprovider.ProviderCreateRequest
	updateID  string
	updateReq memprovider.ProviderUpdateRequest
	getErr    error
	status    memprovider.ProviderStatusResponse
}

func (*fakeMemoryProviderAdmin) ListMeta(context.Context) []memprovider.ProviderMeta {
	return []memprovider.ProviderMeta{{
		Provider:    "builtin",
		DisplayName: "Built-in",
		ConfigSchema: memprovider.ProviderConfigSchema{Fields: map[string]memprovider.ProviderFieldSchema{
			"memory_mode": {Type: "select", Title: "Memory Mode"},
		}},
	}}
}

func (f *fakeMemoryProviderAdmin) Create(_ context.Context, req memprovider.ProviderCreateRequest) (memprovider.ProviderGetResponse, error) {
	f.createReq = req
	return memprovider.ProviderGetResponse{
		ID:        "550e8400-e29b-41d4-a716-446655440000",
		Name:      req.Name,
		Provider:  string(req.Provider),
		Config:    req.Config,
		CreatedAt: time.Date(2026, 5, 5, 1, 2, 3, 0, time.UTC),
		UpdatedAt: time.Date(2026, 5, 5, 1, 2, 4, 0, time.UTC),
	}, nil
}

func (*fakeMemoryProviderAdmin) List(context.Context) ([]memprovider.ProviderGetResponse, error) {
	return nil, nil
}

func (f *fakeMemoryProviderAdmin) Get(context.Context, string) (memprovider.ProviderGetResponse, error) {
	if f.getErr != nil {
		return memprovider.ProviderGetResponse{}, f.getErr
	}
	return memprovider.ProviderGetResponse{}, nil
}

func (f *fakeMemoryProviderAdmin) Update(_ context.Context, id string, req memprovider.ProviderUpdateRequest) (memprovider.ProviderGetResponse, error) {
	f.updateID = id
	f.updateReq = req
	return memprovider.ProviderGetResponse{ID: id, Name: *req.Name, Provider: "builtin", Config: req.Config}, nil
}

func (*fakeMemoryProviderAdmin) Delete(context.Context, string) error {
	return nil
}

func (f *fakeMemoryProviderAdmin) Status(context.Context, string) (memprovider.ProviderStatusResponse, error) {
	return f.status, nil
}

func TestMemoryProviderServiceCreateMapsRequest(t *testing.T) {
	t.Parallel()

	admin := &fakeMemoryProviderAdmin{}
	service := &MemoryProviderService{providers: admin}
	config := mapToStruct(map[string]any{"memory_mode": "sparse"})

	resp, err := service.CreateMemoryProvider(context.Background(), connect.NewRequest(&privatev1.CreateMemoryProviderRequest{
		Name:   " Built-in Main ",
		Type:   "builtin",
		Config: config,
	}))
	if err != nil {
		t.Fatalf("CreateMemoryProvider returned error: %v", err)
	}
	if admin.createReq.Name != " Built-in Main " {
		t.Fatalf("create name = %q, want raw request name to match REST service trimming", admin.createReq.Name)
	}
	if admin.createReq.Provider != memprovider.ProviderBuiltin {
		t.Fatalf("create provider = %q, want builtin", admin.createReq.Provider)
	}
	if admin.createReq.Config["memory_mode"] != "sparse" {
		t.Fatalf("create config memory_mode = %#v, want sparse", admin.createReq.Config["memory_mode"])
	}
	if resp.Msg.GetProvider().GetType() != "builtin" {
		t.Fatalf("response provider type = %q, want builtin", resp.Msg.GetProvider().GetType())
	}
}

func TestMemoryProviderServiceUpdateIgnoresUnsupportedTypeAndEnabled(t *testing.T) {
	t.Parallel()

	admin := &fakeMemoryProviderAdmin{}
	service := &MemoryProviderService{providers: admin}
	name := "Memory Main"

	_, err := service.UpdateMemoryProvider(context.Background(), connect.NewRequest(&privatev1.UpdateMemoryProviderRequest{
		Id:      "550e8400-e29b-41d4-a716-446655440000",
		Name:    &name,
		Type:    stringPtr("mem0"),
		Enabled: boolPtr(false),
		Config:  mapToStruct(map[string]any{"context_target_items": 6}),
	}))
	if err != nil {
		t.Fatalf("UpdateMemoryProvider returned error: %v", err)
	}
	if admin.updateID != "550e8400-e29b-41d4-a716-446655440000" {
		t.Fatalf("update id = %q", admin.updateID)
	}
	if admin.updateReq.Name == nil || *admin.updateReq.Name != name {
		t.Fatalf("update name = %#v, want %q", admin.updateReq.Name, name)
	}
	if admin.updateReq.Config["context_target_items"] != float64(6) {
		t.Fatalf("update config = %#v, want context_target_items=6", admin.updateReq.Config)
	}
}

func TestMemoryProviderServiceGetMapsNotFound(t *testing.T) {
	t.Parallel()

	service := &MemoryProviderService{providers: &fakeMemoryProviderAdmin{getErr: pgx.ErrNoRows}}

	_, err := service.GetMemoryProvider(context.Background(), connect.NewRequest(&privatev1.GetMemoryProviderRequest{
		Id: "550e8400-e29b-41d4-a716-446655440000",
	}))
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("code = %v, want %v, err = %v", connect.CodeOf(err), connect.CodeNotFound, err)
	}
}

func TestMemoryProviderServiceMetaAndStatusMapping(t *testing.T) {
	t.Parallel()

	service := &MemoryProviderService{providers: &fakeMemoryProviderAdmin{
		status: memprovider.ProviderStatusResponse{
			ProviderType:     "builtin",
			MemoryMode:       "sparse",
			EmbeddingModelID: "model-1",
			Collections: []memprovider.ProviderCollectionStatus{{
				Name:   "memory_sparse",
				Exists: true,
				Points: 7,
				Qdrant: memprovider.HealthStatus{OK: true},
			}},
		},
	}}

	metaResp, err := service.ListMemoryProviderMeta(context.Background(), connect.NewRequest(&privatev1.ListMemoryProviderMetaRequest{}))
	if err != nil {
		t.Fatalf("ListMemoryProviderMeta returned error: %v", err)
	}
	fields := metaResp.Msg.GetProviders()[0].GetSchema().AsMap()["fields"].(map[string]any)
	if fields["memory_mode"].(map[string]any)["type"] != "select" {
		t.Fatalf("memory_mode type = %#v, want select", fields["memory_mode"])
	}

	statusResp, err := service.GetMemoryProviderStatus(context.Background(), connect.NewRequest(&privatev1.GetMemoryProviderStatusRequest{
		Id: "550e8400-e29b-41d4-a716-446655440000",
	}))
	if err != nil {
		t.Fatalf("GetMemoryProviderStatus returned error: %v", err)
	}
	if !statusResp.Msg.GetStatus().GetOk() {
		t.Fatal("status ok = false, want true")
	}
	metadata := statusResp.Msg.GetStatus().GetMetadata().AsMap()
	if metadata["memory_mode"] != "sparse" {
		t.Fatalf("metadata memory_mode = %#v, want sparse", metadata["memory_mode"])
	}
}

func TestMemoryProviderConnectErrorUsesFallback(t *testing.T) {
	t.Parallel()

	err := memoryProviderConnectError(errors.New("boom"))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("code = %v, want %v", connect.CodeOf(err), connect.CodeInvalidArgument)
	}
}

func boolPtr(value bool) *bool {
	return &value
}

func TestMemoryProviderSchemaToStructHandlesEmpty(t *testing.T) {
	t.Parallel()

	got := memoryProviderSchemaToStruct(memprovider.ProviderConfigSchema{})
	if got == nil || got.Fields == nil {
		t.Fatal("schema struct must be non-nil")
	}
}
