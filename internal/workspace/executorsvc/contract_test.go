package executorsvc

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"

	workspacev1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/workspace/v1"
	"github.com/memohai/memoh/internal/workspace/security"
)

func TestWorkspaceExecutorContractFixtures(t *testing.T) {
	ctx := context.Background()

	t.Run("read file", func(t *testing.T) {
		root := workspaceFixtureRoot(t)
		fixture := readJSONFixture[struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		}](t, "read_file_success.json")

		resp, err := New(Options{DefaultWorkDir: root, WorkspaceRoot: root, AllowHostAbsolute: true}).ReadFile(ctx, connect.NewRequest(&workspacev1.ReadFileRequest{Path: fixture.Path}))
		if err != nil {
			t.Fatal(err)
		}
		if got := resp.Msg.GetContent(); got != fixture.Content {
			t.Fatalf("content = %q, want %q", got, fixture.Content)
		}
	})

	t.Run("write file", func(t *testing.T) {
		root := workspaceFixtureRoot(t)
		fixture := readJSONFixture[struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		}](t, "write_file_success.json")

		_, err := New(Options{DefaultWorkDir: root, WorkspaceRoot: root, AllowHostAbsolute: true}).WriteFile(ctx, connect.NewRequest(&workspacev1.WriteFileRequest{
			Path:             fixture.Path,
			Content:          []byte(fixture.Content),
			CreateParentDirs: true,
		}))
		if err != nil {
			t.Fatal(err)
		}
		got, err := fs.ReadFile(os.DirFS(root), filepath.ToSlash(fixture.Path))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != fixture.Content {
			t.Fatalf("written content = %q, want %q", string(got), fixture.Content)
		}
	})

	t.Run("list dir", func(t *testing.T) {
		root := workspaceFixtureRoot(t)
		fixture := readJSONFixture[struct {
			Path     string `json:"path"`
			PageSize int32  `json:"page_size"`
			Entries  []struct {
				Name string `json:"name"`
			} `json:"entries"`
		}](t, "list_dir_success.json")

		resp, err := New(Options{DefaultWorkDir: root, WorkspaceRoot: root, AllowHostAbsolute: true}).ListDir(ctx, connect.NewRequest(&workspacev1.ListDirRequest{
			Path:     fixture.Path,
			PageSize: fixture.PageSize,
		}))
		if err != nil {
			t.Fatal(err)
		}
		got := map[string]bool{}
		for _, entry := range resp.Msg.GetEntries() {
			got[entry.GetPath()] = true
		}
		for _, entry := range fixture.Entries {
			if !got[entry.Name] {
				t.Fatalf("entry %q not found in %#v", entry.Name, got)
			}
		}
	})

	t.Run("stat mkdir rename delete", func(t *testing.T) {
		root := workspaceFixtureRoot(t)
		srv := New(Options{DefaultWorkDir: root, WorkspaceRoot: root, AllowHostAbsolute: true})
		statFixture := readJSONFixture[struct {
			Path string `json:"path"`
			Size int64  `json:"size"`
		}](t, "stat_success.json")
		statResp, err := srv.Stat(ctx, connect.NewRequest(&workspacev1.StatRequest{Path: statFixture.Path}))
		if err != nil {
			t.Fatal(err)
		}
		if statResp.Msg.GetEntry().GetSize() != statFixture.Size {
			t.Fatalf("stat size = %d, want %d", statResp.Msg.GetEntry().GetSize(), statFixture.Size)
		}

		mkdirFixture := readJSONFixture[struct {
			Path    string `json:"path"`
			Parents bool   `json:"parents"`
		}](t, "mkdir_success.json")
		if _, err := srv.Mkdir(ctx, connect.NewRequest(&workspacev1.MkdirRequest{Path: mkdirFixture.Path, Parents: mkdirFixture.Parents})); err != nil {
			t.Fatal(err)
		}
		if info, err := os.Stat(filepath.Join(root, mkdirFixture.Path)); err != nil || !info.IsDir() {
			t.Fatalf("mkdir did not create directory: %v", err)
		}

		renameFixture := readJSONFixture[struct {
			OldPath string `json:"old_path"`
			NewPath string `json:"new_path"`
		}](t, "rename_success.json")
		if err := os.WriteFile(filepath.Join(root, renameFixture.OldPath), []byte("done"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := srv.Rename(ctx, connect.NewRequest(&workspacev1.RenameRequest{OldPath: renameFixture.OldPath, NewPath: renameFixture.NewPath, Overwrite: true})); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(root, renameFixture.NewPath)); err != nil {
			t.Fatal(err)
		}

		deleteFixture := readJSONFixture[struct {
			Path      string `json:"path"`
			Recursive bool   `json:"recursive"`
		}](t, "delete_file_success.json")
		if _, err := srv.DeleteFile(ctx, connect.NewRequest(&workspacev1.DeleteFileRequest{Path: deleteFixture.Path, Recursive: deleteFixture.Recursive})); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(root, deleteFixture.Path)); !os.IsNotExist(err) {
			t.Fatalf("deleted path still exists: %v", err)
		}
	})

	t.Run("raw read write", func(t *testing.T) {
		root := workspaceFixtureRoot(t)
		srv := New(Options{DefaultWorkDir: root, WorkspaceRoot: root, AllowHostAbsolute: true})

		rawRead := readJSONLFixture(t, "read_raw_success.jsonl")
		var readReq struct {
			Path   string `json:"path"`
			Offset int64  `json:"offset"`
			Limit  int64  `json:"limit"`
		}
		decodeFixtureLine(t, rawRead[0], &readReq)
		var readChunks [][]byte
		if err := srv.readRaw(&workspacev1.ReadRawRequest{Path: readReq.Path, Offset: readReq.Offset, MaxBytes: readReq.Limit}, func(resp *workspacev1.ReadRawResponse) error {
			readChunks = append(readChunks, resp.GetChunk().GetData())
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if got := string(readChunks[0]); got != "hello" {
			t.Fatalf("raw read = %q, want hello", got)
		}

		rawWrite := readJSONLFixture(t, "write_raw_success.jsonl")
		var writeHead struct {
			Path   string `json:"path"`
			Offset int64  `json:"offset"`
		}
		var writeData struct {
			DataBase64 string `json:"data_base64"`
		}
		decodeFixtureLine(t, rawWrite[0], &writeHead)
		decodeFixtureLine(t, rawWrite[1], &writeData)
		data, err := base64.StdEncoding.DecodeString(writeData.DataBase64)
		if err != nil {
			t.Fatal(err)
		}
		chunks := []*workspacev1.WriteRawChunk{{
			Path:     writeHead.Path,
			Offset:   writeHead.Offset,
			Data:     data,
			Truncate: true,
		}}
		index := 0
		resp, err := srv.writeRaw(func() (*workspacev1.WriteRawChunk, error) {
			if index >= len(chunks) {
				return nil, io.EOF
			}
			chunk := chunks[index]
			index++
			return chunk, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if resp.GetBytesWritten() != int64(len(data)) {
			t.Fatalf("bytes_written = %d, want %d", resp.GetBytesWritten(), len(data))
		}
	})
}

func TestWorkspaceExecutorErrorContractFixtures(t *testing.T) {
	ctx := context.Background()

	t.Run("path escape", func(t *testing.T) {
		root := workspaceFixtureRoot(t)
		fixture := readJSONFixture[struct {
			Path      string `json:"path"`
			ErrorCode string `json:"error_code"`
		}](t, "error_path_escape.json")
		_, err := New(Options{DefaultWorkDir: root, WorkspaceRoot: root, AllowHostAbsolute: true}).ReadFile(ctx, connect.NewRequest(&workspacev1.ReadFileRequest{Path: fixture.Path}))
		assertConnectCode(t, err, fixture.ErrorCode)
	})

	t.Run("symlink escape", func(t *testing.T) {
		root := workspaceFixtureRoot(t)
		outside := t.TempDir()
		if err := os.WriteFile(filepath.Join(outside, "passwd"), []byte("secret"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(root, "link-outside")); err != nil {
			t.Fatal(err)
		}
		fixture := readJSONFixture[struct {
			Path      string `json:"path"`
			ErrorCode string `json:"error_code"`
		}](t, "error_symlink_escape.json")
		_, err := New(Options{DefaultWorkDir: root, WorkspaceRoot: root, AllowHostAbsolute: true}).ReadFile(ctx, connect.NewRequest(&workspacev1.ReadFileRequest{Path: fixture.Path}))
		assertConnectCode(t, err, fixture.ErrorCode)
	})

	t.Run("invalid exec first frame", func(t *testing.T) {
		root := workspaceFixtureRoot(t)
		fixture := readJSONFixture[struct {
			ErrorCode string `json:"error_code"`
		}](t, "error_exec_first_frame_invalid.json")
		err := New(Options{DefaultWorkDir: root, WorkspaceRoot: root, AllowHostAbsolute: true}).exec(ctx, &fixtureExecStream{
			ctx: ctx,
			requests: []*workspacev1.ExecRequest{{
				Frame: &workspacev1.ExecRequest_StdinData{StdinData: []byte("missing start")},
			}},
		})
		assertConnectCode(t, err, fixture.ErrorCode)
	})

	t.Run("exec timeout", func(t *testing.T) {
		root := workspaceFixtureRoot(t)
		fixture := readJSONFixture[struct {
			Command        []string `json:"command"`
			TimeoutSeconds int32    `json:"timeout_seconds"`
			ErrorCode      string   `json:"error_code"`
		}](t, "error_exec_timeout.json")
		err := New(Options{DefaultWorkDir: root, WorkspaceRoot: root, AllowHostAbsolute: true}).execPipe(ctx, &fixtureExecStream{ctx: ctx}, &workspacev1.ExecStart{
			Command:        shellFixtureCommand(fixture.Command),
			TimeoutSeconds: fixture.TimeoutSeconds,
		})
		assertConnectCode(t, err, fixture.ErrorCode)
	})

	t.Run("resource exhausted", func(t *testing.T) {
		root := workspaceFixtureRoot(t)
		fixture := readJSONFixture[struct {
			Path      string `json:"path"`
			Limit     int64  `json:"limit"`
			ErrorCode string `json:"error_code"`
		}](t, "error_resource_exhausted.json")
		if err := os.WriteFile(filepath.Join(root, fixture.Path), make([]byte, fixture.Limit), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := New(Options{DefaultWorkDir: root, WorkspaceRoot: root, AllowHostAbsolute: true}).ReadFile(ctx, connect.NewRequest(&workspacev1.ReadFileRequest{Path: fixture.Path}))
		assertConnectCode(t, err, fixture.ErrorCode)
	})

	t.Run("process not found", func(t *testing.T) {
		root := workspaceFixtureRoot(t)
		fixture := readJSONFixture[struct {
			ProcessID string `json:"process_id"`
			ErrorCode string `json:"error_code"`
		}](t, "error_process_not_found.json")
		_, err := New(Options{DefaultWorkDir: root, WorkspaceRoot: root, AllowHostAbsolute: true}).KillProcess(ctx, connect.NewRequest(&workspacev1.KillProcessRequest{ProcessId: fixture.ProcessID}))
		assertConnectCode(t, err, fixture.ErrorCode)
	})

	t.Run("max timeout", func(t *testing.T) {
		if got := security.MaxExecTimeout; got != 600*time.Second {
			t.Fatalf("MaxExecTimeout = %s, want 10m0s", got)
		}
	})
}

func workspaceFixtureRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("hello workspace\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "notes"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes", "todo.md"), []byte("- item\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes", "done.md"), []byte("- done\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "large.bin"), []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

func readJSONFixture[T any](t *testing.T, name string) T {
	t.Helper()
	data, err := os.ReadFile(workspaceFixturePath(t, name))
	if err != nil {
		t.Fatal(err)
	}
	var out T
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func readJSONLFixture(t *testing.T, name string) []string {
	t.Helper()
	f, err := os.Open(workspaceFixturePath(t, name))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			lines = append(lines, line)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return lines
}

func decodeFixtureLine(t *testing.T, line string, out any) {
	t.Helper()
	if err := json.Unmarshal([]byte(line), out); err != nil {
		t.Fatal(err)
	}
}

func workspaceFixturePath(t *testing.T, name string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime caller unavailable")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "fixtures", "workspace", name)
}

func assertConnectCode(t *testing.T, err error, code string) {
	t.Helper()
	if got, want := connect.CodeOf(err).String(), code; got != want {
		t.Fatalf("connect code = %s, want %s; err=%v", got, want, err)
	}
}

func shellFixtureCommand(command []string) string {
	if len(command) >= 3 && command[0] == "/bin/sh" && command[1] == "-lc" {
		return command[2]
	}
	return strings.Join(command, " ")
}

type fixtureExecStream struct {
	ctx      context.Context
	requests []*workspacev1.ExecRequest
	outputs  []*workspacev1.ExecResponse
	index    int
}

func (s *fixtureExecStream) Context() context.Context {
	if s.ctx != nil {
		return s.ctx
	}
	return context.Background()
}

func (s *fixtureExecStream) Receive() (*workspacev1.ExecRequest, error) {
	if s.index >= len(s.requests) {
		return nil, io.EOF
	}
	req := s.requests[s.index]
	s.index++
	return req, nil
}

func (s *fixtureExecStream) Send(resp *workspacev1.ExecResponse) error {
	s.outputs = append(s.outputs, resp)
	return nil
}
