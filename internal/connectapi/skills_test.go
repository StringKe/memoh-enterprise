package connectapi

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"

	privatev1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1"
	pb "github.com/memohai/memoh/internal/connectapi/gen/memoh/workspace/v1"
	skillset "github.com/memohai/memoh/internal/skills"
)

func TestSkillServiceUpsertListActionAndDelete(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	client := &testSkillFileClient{root: root}
	service := &SkillService{
		files: testSkillProvider{client: client},
		roots: testSkillRoots{
			roots: []string{"/data/.agents/skills"},
		},
	}
	ctx := WithUserID(context.Background(), "user-1")
	compatPath := "/data/.agents/skills/beta/SKILL.md"
	writeTestSkillFile(t, root, compatPath, testSkillRaw("beta", "Beta"))

	upsertResp, err := service.UpsertSkills(ctx, connect.NewRequest(&privatev1.UpsertSkillsRequest{
		BotId: "bot-1",
		Skills: []*privatev1.Skill{{
			Name:        "alpha",
			Description: "Alpha",
			Metadata:    mapToStruct(map[string]any{"content": "Alpha body"}),
		}},
	}))
	if err != nil {
		t.Fatalf("UpsertSkills returned error: %v", err)
	}
	if len(upsertResp.Msg.GetSkills()) != 2 {
		t.Fatalf("upsert skills len = %d, want 2", len(upsertResp.Msg.GetSkills()))
	}

	listResp, err := service.ListSkills(ctx, connect.NewRequest(&privatev1.ListSkillsRequest{BotId: "bot-1"}))
	if err != nil {
		t.Fatalf("ListSkills returned error: %v", err)
	}
	alpha := mustFindProtoSkill(t, listResp.Msg.GetSkills(), "alpha")
	if alpha.GetSource() != path.Join(skillset.ManagedDir(), "alpha", "SKILL.md") {
		t.Fatalf("alpha source = %q", alpha.GetSource())
	}
	if alpha.GetMetadata().AsMap()["content"] != "Alpha body" {
		t.Fatalf("alpha content metadata = %#v", alpha.GetMetadata().AsMap()["content"])
	}

	_, err = service.ApplySkillAction(ctx, connect.NewRequest(&privatev1.ApplySkillActionRequest{
		BotId:   "bot-1",
		SkillId: compatPath,
		Action:  skillset.ActionDisable,
	}))
	if err != nil {
		t.Fatalf("ApplySkillAction returned error: %v", err)
	}
	disabledResp, err := service.ListSkills(ctx, connect.NewRequest(&privatev1.ListSkillsRequest{BotId: "bot-1"}))
	if err != nil {
		t.Fatalf("ListSkills after disable returned error: %v", err)
	}
	beta := mustFindProtoSkill(t, disabledResp.Msg.GetSkills(), "beta")
	if beta.GetEnabled() {
		t.Fatal("beta enabled = true, want false after disable")
	}

	_, err = service.DeleteSkills(ctx, connect.NewRequest(&privatev1.DeleteSkillsRequest{
		BotId:    "bot-1",
		SkillIds: []string{"alpha"},
	}))
	if err != nil {
		t.Fatalf("DeleteSkills returned error: %v", err)
	}
	if _, err := os.Stat(localTestSkillPath(root, path.Join(skillset.ManagedDir(), "alpha"))); !os.IsNotExist(err) {
		t.Fatalf("managed alpha dir still exists, err = %v", err)
	}
}

func TestSkillServiceCatalogListAndInstall(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	client := &testSkillFileClient{root: root}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/skills":
			if got := r.URL.Query().Get("limit"); got != "1" {
				t.Fatalf("limit query = %q, want 1", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"total":2,"page":1,"limit":1,"data":[{"id":"alpha","name":"Alpha","description":"Alpha skill","metadata":{"author":{"name":"Ada","email":"ada@example.com"},"tags":["ops"],"homepage":"https://example.com"},"content":"Alpha body","files":["SKILL.md"]}]}`))
		case "/api/skills/alpha/download":
			w.Header().Set("Content-Type", "application/gzip")
			writeSkillArchive(t, w, "alpha", testSkillRaw("alpha", "Alpha skill"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	service := &SkillService{
		files:      testSkillProvider{client: client},
		baseURL:    server.URL,
		httpClient: server.Client(),
	}
	ctx := WithUserID(context.Background(), "user-1")

	catalogResp, err := service.ListSkillCatalog(ctx, connect.NewRequest(&privatev1.ListSkillCatalogRequest{
		Source: "supermarket",
		Page:   &privatev1.PageRequest{PageSize: 1},
	}))
	if err != nil {
		t.Fatalf("ListSkillCatalog returned error: %v", err)
	}
	if catalogResp.Msg.GetPage().GetNextPageToken() != "2" {
		t.Fatalf("next page token = %q, want 2", catalogResp.Msg.GetPage().GetNextPageToken())
	}
	if catalogResp.Msg.GetSkills()[0].GetMetadata().AsMap()["content"] != "Alpha body" {
		t.Fatalf("catalog content metadata = %#v", catalogResp.Msg.GetSkills()[0].GetMetadata().AsMap()["content"])
	}

	installResp, err := service.InstallSkill(ctx, connect.NewRequest(&privatev1.InstallSkillRequest{
		BotId:   "bot-1",
		Source:  "supermarket",
		SkillId: "alpha",
	}))
	if err != nil {
		t.Fatalf("InstallSkill returned error: %v", err)
	}
	if installResp.Msg.GetSkill().GetName() != "alpha" {
		t.Fatalf("installed skill name = %q, want alpha", installResp.Msg.GetSkill().GetName())
	}
	if _, err := os.Stat(localTestSkillPath(root, path.Join(skillset.ManagedDir(), "alpha", "SKILL.md"))); err != nil {
		t.Fatalf("installed SKILL.md missing: %v", err)
	}
}

func TestSkillServiceValidation(t *testing.T) {
	t.Parallel()

	service := &SkillService{}
	_, err := service.ListSkills(context.Background(), connect.NewRequest(&privatev1.ListSkillsRequest{BotId: "bot-1"}))
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("code = %v, want %v", connect.CodeOf(err), connect.CodeUnauthenticated)
	}

	_, err = service.InstallSkill(WithUserID(context.Background(), "user-1"), connect.NewRequest(&privatev1.InstallSkillRequest{
		BotId:   "bot-1",
		SkillId: "../bad",
	}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("code = %v, want %v", connect.CodeOf(err), connect.CodeInvalidArgument)
	}
}

type testSkillProvider struct {
	client skillFileClient
}

func (p testSkillProvider) SkillClient(context.Context, string) (skillFileClient, error) {
	return p.client, nil
}

type testSkillRoots struct {
	roots []string
}

func (r testSkillRoots) ResolveWorkspaceSkillDiscoveryRoots(context.Context, string) ([]string, error) {
	return r.roots, nil
}

type testSkillFileClient struct {
	root string
}

func (c *testSkillFileClient) ListDirAll(_ context.Context, filePath string, _ bool) ([]*pb.FileEntry, error) {
	localPath := localTestSkillPath(c.root, filePath)
	entries, err := os.ReadDir(localPath)
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	out := make([]*pb.FileEntry, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		out = append(out, &pb.FileEntry{
			Path:    path.Join(filePath, entry.Name()),
			IsDir:   entry.IsDir(),
			Size:    info.Size(),
			Mode:    info.Mode().String(),
			ModTime: info.ModTime().UTC().Format(time.RFC3339),
		})
	}
	return out, nil
}

func (c *testSkillFileClient) ReadRaw(_ context.Context, filePath string) (io.ReadCloser, error) {
	return os.Open(localTestSkillPath(c.root, filePath))
}

func (c *testSkillFileClient) WriteRaw(_ context.Context, filePath string, r io.Reader) (int64, error) {
	if err := os.MkdirAll(filepath.Dir(localTestSkillPath(c.root, filePath)), 0o750); err != nil {
		return 0, err
	}
	file, err := os.Create(localTestSkillPath(c.root, filePath))
	if err != nil {
		return 0, err
	}
	defer func() { _ = file.Close() }()
	return io.Copy(file, r)
}

func (c *testSkillFileClient) WriteFile(_ context.Context, filePath string, content []byte) error {
	localPath := localTestSkillPath(c.root, filePath)
	if err := os.MkdirAll(filepath.Dir(localPath), 0o750); err != nil {
		return err
	}
	return os.WriteFile(localPath, content, 0o600)
}

func (c *testSkillFileClient) Mkdir(_ context.Context, filePath string) error {
	return os.MkdirAll(localTestSkillPath(c.root, filePath), 0o750)
}

func (c *testSkillFileClient) Stat(_ context.Context, filePath string) (*pb.FileEntry, error) {
	info, err := os.Stat(localTestSkillPath(c.root, filePath))
	if err != nil {
		return nil, err
	}
	return &pb.FileEntry{Path: filePath, IsDir: info.IsDir(), Size: info.Size()}, nil
}

func (c *testSkillFileClient) DeleteFile(_ context.Context, filePath string, recursive bool) error {
	if recursive {
		return os.RemoveAll(localTestSkillPath(c.root, filePath))
	}
	return os.Remove(localTestSkillPath(c.root, filePath))
}

func writeTestSkillFile(t *testing.T, root, containerPath, content string) {
	t.Helper()
	localPath := localTestSkillPath(root, containerPath)
	if err := os.MkdirAll(filepath.Dir(localPath), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(localPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write skill: %v", err)
	}
}

func localTestSkillPath(root, containerPath string) string {
	clean := path.Clean("/" + strings.TrimSpace(containerPath))
	return filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(clean, "/")))
}

func testSkillRaw(name, description string) string {
	return "---\nname: " + name + "\ndescription: " + description + "\n---\n" + description + " body\n"
}

func mustFindProtoSkill(t *testing.T, items []*privatev1.Skill, name string) *privatev1.Skill {
	t.Helper()
	for _, item := range items {
		if item.GetName() == name {
			return item
		}
	}
	t.Fatalf("skill %q not found in %+v", name, items)
	return nil
}

func writeSkillArchive(t *testing.T, w io.Writer, skillID, raw string) {
	t.Helper()
	gz := gzip.NewWriter(w)
	tarWriter := tar.NewWriter(gz)
	data := []byte(raw)
	if err := tarWriter.WriteHeader(&tar.Header{
		Name: skillID + "/SKILL.md",
		Mode: 0o600,
		Size: int64(len(data)),
	}); err != nil {
		t.Fatalf("write tar header: %v", err)
	}
	if _, err := tarWriter.Write(data); err != nil {
		t.Fatalf("write tar data: %v", err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
}
