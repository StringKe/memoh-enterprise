package executorsvc

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"connectrpc.com/connect"

	workspacev1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/workspace/v1"
	"github.com/memohai/memoh/internal/workspace/security"
)

func TestReadRawUsesOneMiBChunks(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	filePath := filepath.Join(root, "raw.bin")
	data := make([]byte, security.RawChunkSize+1)
	if err := os.WriteFile(filePath, data, 0o600); err != nil {
		t.Fatalf("write raw fixture: %v", err)
	}

	var chunks [][]byte
	err := New(Options{DefaultWorkDir: root, WorkspaceRoot: root, AllowHostAbsolute: true}).readRaw(
		&workspacev1.ReadRawRequest{Path: filePath},
		func(resp *workspacev1.ReadRawResponse) error {
			chunks = append(chunks, append([]byte(nil), resp.GetChunk().GetData()...))
			return nil
		},
	)
	if err != nil {
		t.Fatalf("ReadRaw: %v", err)
	}
	if len(chunks) != 2 {
		t.Fatalf("chunk count = %d, want 2", len(chunks))
	}
	if len(chunks[0]) != security.RawChunkSize || len(chunks[1]) != 1 {
		t.Fatalf("chunk sizes = %d/%d, want %d/1", len(chunks[0]), len(chunks[1]), security.RawChunkSize)
	}
}

func TestWriteRawRejectsOversizedChunk(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	chunks := []*workspacev1.WriteRawChunk{{
		Path: filepath.Join(root, "raw.bin"),
		Data: make([]byte, security.RawChunkSize+1),
	}}
	index := 0
	_, err := New(Options{DefaultWorkDir: root, WorkspaceRoot: root, AllowHostAbsolute: true}).writeRaw(func() (*workspacev1.WriteRawChunk, error) {
		if index >= len(chunks) {
			return nil, io.EOF
		}
		chunk := chunks[index]
		index++
		return chunk, nil
	})
	if connect.CodeOf(err) != connect.CodeResourceExhausted {
		t.Fatalf("WriteRaw status = %s, want %s", connect.CodeOf(err), connect.CodeResourceExhausted)
	}
}

func TestReadFileRejectsOversizedUnaryRead(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	f, err := os.CreateTemp(root, "large-*.txt")
	if err != nil {
		t.Fatalf("create large file: %v", err)
	}
	filePath := f.Name()
	if err := f.Truncate(int64(security.UnaryReadMaxBytes + 1)); err != nil {
		_ = f.Close()
		t.Fatalf("truncate large file: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close large file: %v", err)
	}

	_, err = New(Options{DefaultWorkDir: root, WorkspaceRoot: root, AllowHostAbsolute: true}).ReadFile(
		context.Background(),
		connect.NewRequest(&workspacev1.ReadFileRequest{Path: filePath}),
	)
	if connect.CodeOf(err) != connect.CodeResourceExhausted {
		t.Fatalf("ReadFile status = %s, want %s", connect.CodeOf(err), connect.CodeResourceExhausted)
	}
}

func TestListDirClampsPageSizeTo200(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	for i := 0; i < security.ListPageSize+1; i++ {
		name := filepath.Join(root, "file-"+string(rune('a'+(i%26)))+"-"+string(rune('a'+((i/26)%26))))
		if err := os.WriteFile(name, nil, 0o600); err != nil {
			t.Fatalf("write list fixture: %v", err)
		}
	}

	resp, err := New(Options{DefaultWorkDir: root, WorkspaceRoot: root, AllowHostAbsolute: true}).listDir(
		&workspacev1.ListDirRequest{Path: root, PageSize: int32(security.ListPageSize + 1)},
	)
	if err != nil {
		t.Fatalf("ListDir: %v", err)
	}
	if len(resp.GetEntries()) != security.ListPageSize {
		t.Fatalf("entry count = %d, want %d", len(resp.GetEntries()), security.ListPageSize)
	}
	if !resp.GetTruncated() {
		t.Fatal("ListDir truncated = false, want true")
	}
}

func TestExecRejectsForbiddenEnv(t *testing.T) {
	stream := newCancelOnStdoutExecStream()
	srv := New(Options{DefaultWorkDir: t.TempDir()})

	err := srv.execPipe(context.Background(), stream, &workspacev1.ExecStart{
		Command: "printf ok",
		Env:     []string{"AWS_SECRET_ACCESS_KEY=secret"},
	})
	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("execPipe status = %s, want %s", connect.CodeOf(err), connect.CodePermissionDenied)
	}
}

func TestExecRejectsTimeoutAboveMax(t *testing.T) {
	stream := newCancelOnStdoutExecStream()
	srv := New(Options{DefaultWorkDir: t.TempDir()})

	err := srv.execPipe(context.Background(), stream, &workspacev1.ExecStart{
		Command:        "printf ok",
		TimeoutSeconds: 601,
	})
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("execPipe status = %s, want %s", connect.CodeOf(err), connect.CodeInvalidArgument)
	}
}
