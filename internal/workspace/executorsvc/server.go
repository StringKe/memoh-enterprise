package executorsvc

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	"connectrpc.com/connect"
	"github.com/creack/pty"

	workspacev1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/workspace/v1"
	"github.com/memohai/memoh/internal/connectapi/gen/memoh/workspace/v1/workspacev1connect"
	"github.com/memohai/memoh/internal/workspace/security"
)

const (
	readMaxLines     = 2000
	readMaxLineLen   = 0
	binaryProbeBytes = 8 * 1024
	DefaultWorkDir   = "/data"
)

type Options struct {
	DefaultWorkDir    string
	WorkspaceRoot     string
	DataMount         string
	AllowHostAbsolute bool
}

type Server struct {
	workspacev1connect.UnimplementedWorkspaceExecutorServiceHandler

	defaultWorkDir string
	pathResolver   *security.PathResolver

	processMu sync.RWMutex
	processes map[string]*ptyProcess
}

type ptyProcess struct {
	id        string
	command   string
	startedAt time.Time
	status    string
	cmd       *exec.Cmd
	ptmx      *os.File
	cancel    context.CancelFunc
	output    chan ptyOutput
}

type ptyOutput struct {
	data     []byte
	exited   bool
	exitCode int32
}

func New(opts Options) *Server {
	defaultWorkDir := strings.TrimSpace(opts.DefaultWorkDir)
	if defaultWorkDir == "" {
		defaultWorkDir = DefaultWorkDir
	}
	workspaceRoot := strings.TrimSpace(opts.WorkspaceRoot)
	if workspaceRoot != "" {
		if abs, err := filepath.Abs(workspaceRoot); err == nil {
			workspaceRoot = abs
		}
	}
	resolver, err := security.NewPathResolver(security.PathResolverOptions{
		DefaultWorkDir:    defaultWorkDir,
		WorkspaceRoot:     workspaceRoot,
		DataMount:         opts.DataMount,
		AllowHostAbsolute: opts.AllowHostAbsolute,
	})
	if err != nil {
		panic(err)
	}
	return &Server{
		defaultWorkDir: filepath.Clean(defaultWorkDir),
		pathResolver:   resolver,
		processes:      make(map[string]*ptyProcess),
	}
}

func (s *Server) GetWorkspaceInfo(context.Context, *connect.Request[workspacev1.GetWorkspaceInfoRequest]) (*connect.Response[workspacev1.GetWorkspaceInfoResponse], error) {
	return connect.NewResponse(&workspacev1.GetWorkspaceInfoResponse{
		Backend:        "container",
		DefaultWorkDir: s.defaultWorkDir,
		AllowedRoots:   []string{s.pathResolver.DefaultWorkDir()},
		Runtime: map[string]string{
			"protocol": "connectrpc",
			"service":  workspacev1connect.WorkspaceExecutorServiceName,
		},
	}), nil
}

func (s *Server) ReadFile(_ context.Context, req *connect.Request[workspacev1.ReadFileRequest]) (*connect.Response[workspacev1.ReadFileResponse], error) {
	path, err := s.resolvePath(req.Msg.GetPath())
	if err != nil {
		return nil, pathError(err)
	}

	f, err := os.Open(path) //nolint:gosec // G304: workspace executor intentionally serves agent-selected paths.
	if err != nil {
		return nil, connectErrorf(connect.CodeNotFound, "open: %v", err)
	}
	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil {
		return nil, connectErrorf(connect.CodeInternal, "stat: %v", err)
	}
	if info.Size() > security.UnaryReadMaxBytes {
		return nil, connect.NewError(connect.CodeResourceExhausted, errors.New("file exceeds unary read limit"))
	}

	probe := make([]byte, binaryProbeBytes)
	n, _ := f.Read(probe)
	if bytes.IndexByte(probe[:n], 0) >= 0 {
		return connect.NewResponse(&workspacev1.ReadFileResponse{Binary: true}), nil
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, connectErrorf(connect.CodeInternal, "seek: %v", err)
	}

	lineOffset := req.Msg.GetLineOffset()
	if lineOffset < 1 {
		lineOffset = 1
	}
	lineCount := req.Msg.GetLineCount()
	if lineCount < 1 || lineCount > readMaxLines {
		lineCount = readMaxLines
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var currentLine int32
	var totalLines int32
	var out strings.Builder
	var linesRead int32
	bytesWritten := 0

	for scanner.Scan() {
		currentLine++
		totalLines = currentLine
		if currentLine < lineOffset || linesRead >= lineCount {
			continue
		}

		line := scanner.Text()
		if readMaxLineLen > 0 && utf8.RuneCountInString(line) > readMaxLineLen {
			line = truncateRunes(line, readMaxLineLen) + "..."
		}

		entry := line + "\n"
		if bytesWritten+len(entry) > security.UnaryReadMaxBytes {
			return nil, connect.NewError(connect.CodeResourceExhausted, errors.New("read response exceeds unary read limit"))
		}
		out.WriteString(entry)
		bytesWritten += len(entry)
		linesRead++
	}
	if err := scanner.Err(); err != nil {
		return nil, connectErrorf(connect.CodeInternal, "scan: %v", err)
	}

	return connect.NewResponse(&workspacev1.ReadFileResponse{
		Content:    out.String(),
		TotalLines: totalLines,
	}), nil
}

func (s *Server) WriteFile(_ context.Context, req *connect.Request[workspacev1.WriteFileRequest]) (*connect.Response[workspacev1.WriteFileResponse], error) {
	if len(req.Msg.GetContent()) > security.UnaryReadMaxBytes {
		return nil, connect.NewError(connect.CodeResourceExhausted, errors.New("write content exceeds unary limit"))
	}
	path, err := s.resolvePath(req.Msg.GetPath())
	if err != nil {
		return nil, pathError(err)
	}

	if req.Msg.GetCreateParentDirs() {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			return nil, connectErrorf(connect.CodeInternal, "mkdir: %v", err)
		}
	}
	if err := os.WriteFile(path, req.Msg.GetContent(), 0o600); err != nil {
		return nil, connectErrorf(connect.CodeInternal, "write: %v", err)
	}
	return connect.NewResponse(&workspacev1.WriteFileResponse{BytesWritten: int64(len(req.Msg.GetContent()))}), nil
}

func (s *Server) ListDir(_ context.Context, req *connect.Request[workspacev1.ListDirRequest]) (*connect.Response[workspacev1.ListDirResponse], error) {
	resp, err := s.listDir(req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

func (s *Server) listDir(req *workspacev1.ListDirRequest) (*workspacev1.ListDirResponse, error) {
	dir := req.GetPath()
	if dir == "" {
		dir = "."
	}
	dir, err := s.resolvePath(dir)
	if err != nil {
		return nil, pathError(err)
	}

	var all []*workspacev1.FileEntry

	if req.GetRecursive() {
		err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			rel, _ := filepath.Rel(dir, p)
			if rel == "." {
				return nil
			}
			entry, _ := buildFileEntry(rel, d)
			if entry != nil {
				all = append(all, entry)
			}
			return nil
		})
		if err != nil {
			return nil, connectErrorf(connect.CodeNotFound, "walk: %v", err)
		}

		if threshold := req.GetCollapseThreshold(); threshold > 0 {
			all = collapseHeavySubdirs(all, int(threshold))
		}
	} else {
		dirEntries, err := os.ReadDir(dir)
		if err != nil {
			return nil, connectErrorf(connect.CodeNotFound, "readdir: %v", err)
		}
		for _, d := range dirEntries {
			entry, _ := buildFileEntry(d.Name(), d)
			if entry != nil {
				all = append(all, entry)
			}
		}
	}

	totalCount := int32(min(len(all), math.MaxInt32)) //nolint:gosec // clamped
	offset := int32(0)
	if token := strings.TrimSpace(req.GetPageToken()); token != "" {
		parsed, err := strconv.ParseInt(token, 10, 32)
		if err != nil || parsed < 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid page_token"))
		}
		offset = int32(parsed) //nolint:gosec // bounded by ParseInt bit size
	}
	limit := req.GetPageSize()
	if limit < 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("page_size must be non-negative"))
	}
	if limit > security.ListPageSize {
		limit = security.ListPageSize
	}

	var entries []*workspacev1.FileEntry
	if int(offset) < len(all) {
		entries = all[offset:]
	}
	if limit > 0 && int(limit) < len(entries) {
		entries = entries[:limit]
	}

	truncated := int(offset)+len(entries) < int(totalCount)
	nextToken := ""
	if truncated {
		nextToken = strconv.FormatInt(int64(offset)+int64(len(entries)), 10)
	}
	return &workspacev1.ListDirResponse{
		Entries:       entries,
		NextPageToken: nextToken,
		TotalCount:    totalCount,
		Truncated:     truncated,
	}, nil
}

func (s *Server) Exec(ctx context.Context, stream *connect.BidiStream[workspacev1.ExecRequest, workspacev1.ExecResponse]) error {
	return s.exec(ctx, &connectExecStream{ctx: ctx, stream: stream})
}

type execStream interface {
	Context() context.Context
	Receive() (*workspacev1.ExecRequest, error)
	Send(*workspacev1.ExecResponse) error
}

type connectExecStream struct {
	ctx    context.Context
	stream *connect.BidiStream[workspacev1.ExecRequest, workspacev1.ExecResponse]
}

func (s *connectExecStream) Context() context.Context {
	return s.ctx
}

func (s *connectExecStream) Receive() (*workspacev1.ExecRequest, error) {
	return s.stream.Receive()
}

func (s *connectExecStream) Send(resp *workspacev1.ExecResponse) error {
	return s.stream.Send(resp)
}

func (s *Server) exec(ctx context.Context, stream execStream) error {
	firstMsg, err := stream.Receive()
	if err != nil {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("failed to receive exec start"))
	}
	start := firstMsg.GetStart()
	if start == nil {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("first exec frame must be start"))
	}
	if strings.TrimSpace(start.GetCommand()) == "" {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("command is required"))
	}

	if start.GetPty() {
		return s.execPTY(ctx, stream, start)
	}
	return s.execPipe(ctx, stream, start)
}

func (s *Server) execPTY(ctx context.Context, stream execStream, start *workspacev1.ExecStart) error {
	workDir, err := s.resolveExecWorkDir(start.GetWorkDir())
	if err != nil {
		return pathError(err)
	}

	timeout, err := security.NormalizeExecTimeout(start.GetTimeoutSeconds())
	if err != nil {
		return connect.NewError(connect.CodeInvalidArgument, err)
	}
	procCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(procCtx, "/bin/sh", "-c", start.GetCommand()) //nolint:gosec // G204: intentional agent command execution.
	cmd.Dir = workDir
	env, err := security.SanitizeEnv(append(os.Environ(), "TERM=xterm-256color"), start.GetEnv())
	if err != nil {
		return connect.NewError(connect.CodePermissionDenied, err)
	}
	cmd.Env = env

	initialSize := &pty.Winsize{Rows: 24, Cols: 80}
	ptmx, err := pty.StartWithSize(cmd, initialSize)
	if err != nil {
		return connectErrorf(connect.CodeInternal, "pty start: %v", err)
	}
	defer func() { _ = ptmx.Close() }()

	go func() {
		for {
			msg, recvErr := stream.Receive()
			if recvErr != nil {
				return
			}
			if resize := msg.GetResize(); resize != nil && resize.GetCols() > 0 && resize.GetRows() > 0 {
				_ = pty.Setsize(ptmx, ptySize(resize))
			}
			if data := msg.GetStdinData(); len(data) > 0 {
				_, _ = ptmx.Write(data)
			}
		}
	}()

	streamPipe(stream, ptmx, workspacev1.ExecResponse_KIND_STDOUT)

	exitCode := waitExitCode(cmd.Wait())
	if errors.Is(procCtx.Err(), context.DeadlineExceeded) {
		return connect.NewError(connect.CodeDeadlineExceeded, errors.New("command timeout"))
	}
	_ = stream.Send(&workspacev1.ExecResponse{Kind: workspacev1.ExecResponse_KIND_EXIT, ExitCode: exitCode})
	return nil
}

func (s *Server) execPipe(ctx context.Context, stream execStream, start *workspacev1.ExecStart) error {
	workDir, err := s.resolveExecWorkDir(start.GetWorkDir())
	if err != nil {
		return pathError(err)
	}

	timeout, err := security.NormalizeExecTimeout(start.GetTimeoutSeconds())
	if err != nil {
		return connect.NewError(connect.CodeInvalidArgument, err)
	}

	procCtx, procCancel := context.WithTimeout(ctx, timeout)
	defer procCancel()

	cmd := exec.CommandContext(procCtx, "/bin/sh", "-c", start.GetCommand()) //nolint:gosec // G204: intentional agent command execution.
	cmd.Dir = workDir
	env, err := security.SanitizeEnv(os.Environ(), start.GetEnv())
	if err != nil {
		return connect.NewError(connect.CodePermissionDenied, err)
	}
	cmd.Env = env

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return connectErrorf(connect.CodeInternal, "stdin pipe: %v", err)
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return connectErrorf(connect.CodeInternal, "stdout pipe: %v", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return connectErrorf(connect.CodeInternal, "stderr pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		return connectErrorf(connect.CodeInternal, "start: %v", err)
	}

	go func() {
		select {
		case <-procCtx.Done():
		case <-stream.Context().Done():
		}
		_ = stdoutPipe.Close()
		_ = stderrPipe.Close()
	}()

	go func() {
		for {
			msg, recvErr := stream.Receive()
			if recvErr != nil {
				_ = stdinPipe.Close()
				return
			}
			if data := msg.GetStdinData(); len(data) > 0 {
				_, _ = stdinPipe.Write(data)
			}
		}
	}()

	done := make(chan struct{})
	go func() {
		defer close(done)
		streamPipe(stream, stdoutPipe, workspacev1.ExecResponse_KIND_STDOUT)
	}()
	streamPipe(stream, stderrPipe, workspacev1.ExecResponse_KIND_STDERR)
	<-done

	exitCode := waitExitCode(cmd.Wait())
	if errors.Is(procCtx.Err(), context.DeadlineExceeded) {
		return connect.NewError(connect.CodeDeadlineExceeded, errors.New("command timeout"))
	}
	_ = stream.Send(&workspacev1.ExecResponse{Kind: workspacev1.ExecResponse_KIND_EXIT, ExitCode: exitCode})
	return nil
}

func (s *Server) ReadRaw(_ context.Context, req *connect.Request[workspacev1.ReadRawRequest], stream *connect.ServerStream[workspacev1.ReadRawResponse]) error {
	return s.readRaw(req.Msg, stream.Send)
}

func (s *Server) readRaw(req *workspacev1.ReadRawRequest, send func(*workspacev1.ReadRawResponse) error) error {
	path, err := s.resolvePath(req.GetPath())
	if err != nil {
		return pathError(err)
	}

	f, err := os.Open(path) //nolint:gosec // G304: workspace executor intentionally serves agent-selected paths.
	if err != nil {
		return connectErrorf(connect.CodeNotFound, "open: %v", err)
	}
	defer func() { _ = f.Close() }()

	if req.GetOffset() > 0 {
		if _, err := f.Seek(req.GetOffset(), io.SeekStart); err != nil {
			return connectErrorf(connect.CodeInvalidArgument, "seek: %v", err)
		}
	}

	buf := make([]byte, security.RawChunkSize)
	offset := req.GetOffset()
	remaining := req.GetMaxBytes()
	for {
		readBuf := buf
		if remaining > 0 && remaining < int64(len(buf)) {
			readBuf = buf[:remaining]
		}
		n, err := f.Read(readBuf)
		if n > 0 {
			if sendErr := send(&workspacev1.ReadRawResponse{Chunk: &workspacev1.DataChunk{
				Data:   append([]byte(nil), readBuf[:n]...),
				Offset: offset,
			}}); sendErr != nil {
				return sendErr
			}
			offset += int64(n)
			if remaining > 0 {
				remaining -= int64(n)
				if remaining <= 0 {
					break
				}
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return connectErrorf(connect.CodeInternal, "read: %v", err)
		}
	}
	return nil
}

func (s *Server) WriteRaw(_ context.Context, stream *connect.ClientStream[workspacev1.WriteRawRequest]) (*connect.Response[workspacev1.WriteRawResponse], error) {
	resp, err := s.writeRaw(func() (*workspacev1.WriteRawChunk, error) {
		if !stream.Receive() {
			if err := stream.Err(); err != nil {
				return nil, err
			}
			return nil, io.EOF
		}
		return stream.Msg().GetChunk(), nil
	})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

func (s *Server) writeRaw(recv func() (*workspacev1.WriteRawChunk, error)) (*workspacev1.WriteRawResponse, error) {
	var f *os.File
	var written int64

	for {
		chunk, err := recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if chunk == nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("chunk is required"))
		}
		if len(chunk.GetData()) > security.RawChunkSize {
			return nil, connect.NewError(connect.CodeResourceExhausted, errors.New("raw chunk exceeds limit"))
		}

		if f == nil {
			if strings.TrimSpace(chunk.GetPath()) == "" {
				return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("path is required on first raw chunk"))
			}
			path, err := s.resolvePath(chunk.GetPath())
			if err != nil {
				return nil, pathError(err)
			}
			if mkErr := os.MkdirAll(filepath.Dir(path), 0o750); mkErr != nil {
				return nil, connectErrorf(connect.CodeInternal, "mkdir: %v", mkErr)
			}
			flag := os.O_CREATE | os.O_WRONLY
			if chunk.GetTruncate() {
				flag |= os.O_TRUNC
			}
			f, err = os.OpenFile(path, flag, 0o600) //nolint:gosec // G304: workspace executor intentionally serves agent-selected paths.
			if err != nil {
				return nil, connectErrorf(connect.CodeInternal, "create: %v", err)
			}
			defer func() { _ = f.Close() }()
		}

		if _, err := f.Seek(chunk.GetOffset(), io.SeekStart); err != nil {
			return nil, connectErrorf(connect.CodeInternal, "seek: %v", err)
		}
		if len(chunk.GetData()) > 0 {
			n, err := f.Write(chunk.GetData())
			written += int64(n)
			if err != nil {
				return nil, connectErrorf(connect.CodeInternal, "write: %v", err)
			}
		}
	}
	return &workspacev1.WriteRawResponse{BytesWritten: written}, nil
}

func (s *Server) DeleteFile(_ context.Context, req *connect.Request[workspacev1.DeleteFileRequest]) (*connect.Response[workspacev1.DeleteFileResponse], error) {
	path, err := s.resolvePath(req.Msg.GetPath())
	if err != nil {
		return nil, pathError(err)
	}

	if req.Msg.GetRecursive() {
		err = os.RemoveAll(path)
	} else {
		err = os.Remove(path)
	}
	if err != nil && !os.IsNotExist(err) {
		return nil, connectErrorf(connect.CodeInternal, "delete: %v", err)
	}
	return connect.NewResponse(&workspacev1.DeleteFileResponse{}), nil
}

func (s *Server) Stat(_ context.Context, req *connect.Request[workspacev1.StatRequest]) (*connect.Response[workspacev1.StatResponse], error) {
	path, err := s.resolvePath(req.Msg.GetPath())
	if err != nil {
		return nil, pathError(err)
	}

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("not found"))
		}
		return nil, connectErrorf(connect.CodeInternal, "stat: %v", err)
	}
	return connect.NewResponse(&workspacev1.StatResponse{
		Entry: &workspacev1.FileEntry{
			Path:    filepath.Base(path),
			IsDir:   info.IsDir(),
			Size:    info.Size(),
			Mode:    info.Mode().String(),
			ModTime: info.ModTime().Format(time.RFC3339),
		},
	}), nil
}

func (s *Server) Mkdir(_ context.Context, req *connect.Request[workspacev1.MkdirRequest]) (*connect.Response[workspacev1.MkdirResponse], error) {
	path, err := s.resolvePath(req.Msg.GetPath())
	if err != nil {
		return nil, pathError(err)
	}

	if req.Msg.GetParents() {
		err = os.MkdirAll(path, 0o750)
	} else {
		err = os.Mkdir(path, 0o750)
	}
	if err != nil {
		return nil, connectErrorf(connect.CodeInternal, "mkdir: %v", err)
	}
	return connect.NewResponse(&workspacev1.MkdirResponse{}), nil
}

func (s *Server) Rename(_ context.Context, req *connect.Request[workspacev1.RenameRequest]) (*connect.Response[workspacev1.RenameResponse], error) {
	oldPath := req.Msg.GetOldPath()
	newPath := req.Msg.GetNewPath()
	if oldPath == "" || newPath == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("old_path and new_path are required"))
	}
	oldPath, err := s.resolvePath(oldPath)
	if err != nil {
		return nil, pathError(err)
	}
	newPath, err = s.resolvePath(newPath)
	if err != nil {
		return nil, pathError(err)
	}

	if err := os.MkdirAll(filepath.Dir(newPath), 0o750); err != nil {
		return nil, connectErrorf(connect.CodeInternal, "mkdir parent: %v", err)
	}
	if req.Msg.GetOverwrite() {
		_ = os.Remove(newPath)
	}
	if err := os.Rename(oldPath, newPath); err != nil {
		return nil, connectErrorf(connect.CodeInternal, "rename: %v", err)
	}
	return connect.NewResponse(&workspacev1.RenameResponse{}), nil
}

func (s *Server) OpenPTY(ctx context.Context, req *connect.Request[workspacev1.OpenPTYRequest]) (*connect.Response[workspacev1.OpenPTYResponse], error) {
	proc, err := s.startPTYProcess(ctx, req.Msg.GetCommand(), req.Msg.GetWorkDir(), req.Msg.GetEnv(), req.Msg.GetSize())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&workspacev1.OpenPTYResponse{ProcessId: proc.id}), nil
}

func (s *Server) StreamPTY(ctx context.Context, req *connect.Request[workspacev1.StreamPTYRequest], stream *connect.ServerStream[workspacev1.StreamPTYResponse]) error {
	proc, err := s.lookupProcess(req.Msg.GetProcessId())
	if err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case output, ok := <-proc.output:
			if !ok {
				return nil
			}
			if err := stream.Send(&workspacev1.StreamPTYResponse{Output: &workspacev1.PTYOutput{
				Data:     output.data,
				Exited:   output.exited,
				ExitCode: output.exitCode,
			}}); err != nil {
				return err
			}
			if output.exited {
				return nil
			}
		}
	}
}

func (s *Server) WritePTY(_ context.Context, req *connect.Request[workspacev1.WritePTYRequest]) (*connect.Response[workspacev1.WritePTYResponse], error) {
	proc, err := s.lookupProcess(req.Msg.GetProcessId())
	if err != nil {
		return nil, err
	}
	if len(req.Msg.GetData()) > 0 {
		if _, err := proc.ptmx.Write(req.Msg.GetData()); err != nil {
			return nil, connectErrorf(connect.CodeInternal, "pty write: %v", err)
		}
	}
	return connect.NewResponse(&workspacev1.WritePTYResponse{}), nil
}

func (s *Server) ResizePTY(_ context.Context, req *connect.Request[workspacev1.ResizePTYRequest]) (*connect.Response[workspacev1.ResizePTYResponse], error) {
	proc, err := s.lookupProcess(req.Msg.GetProcessId())
	if err != nil {
		return nil, err
	}
	if size := req.Msg.GetSize(); size != nil && size.GetCols() > 0 && size.GetRows() > 0 {
		if err := pty.Setsize(proc.ptmx, ptySize(size)); err != nil {
			return nil, connectErrorf(connect.CodeInternal, "pty resize: %v", err)
		}
	}
	return connect.NewResponse(&workspacev1.ResizePTYResponse{}), nil
}

func (s *Server) KillProcess(_ context.Context, req *connect.Request[workspacev1.KillProcessRequest]) (*connect.Response[workspacev1.KillProcessResponse], error) {
	proc, err := s.lookupProcess(req.Msg.GetProcessId())
	if err != nil {
		return nil, err
	}
	signalName := strings.ToUpper(strings.TrimSpace(req.Msg.GetSignal()))
	var sig os.Signal
	switch signalName {
	case "", "TERM", "SIGTERM":
		sig = syscall.SIGTERM
	case "KILL", "SIGKILL":
		sig = os.Kill
	case "INT", "SIGINT", "INTERRUPT":
		sig = os.Interrupt
	default:
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("unsupported signal"))
	}
	proc.cancel()
	if proc.cmd.Process != nil {
		_ = proc.cmd.Process.Signal(sig)
	}
	return connect.NewResponse(&workspacev1.KillProcessResponse{}), nil
}

func (s *Server) ListProcesses(context.Context, *connect.Request[workspacev1.ListProcessesRequest]) (*connect.Response[workspacev1.ListProcessesResponse], error) {
	s.processMu.RLock()
	defer s.processMu.RUnlock()

	processes := make([]*workspacev1.ProcessInfo, 0, len(s.processes))
	for _, proc := range s.processes {
		processes = append(processes, &workspacev1.ProcessInfo{
			ProcessId:   proc.id,
			Command:     proc.command,
			Status:      proc.status,
			StartedUnix: proc.startedAt.Unix(),
		})
	}
	return connect.NewResponse(&workspacev1.ListProcessesResponse{Processes: processes}), nil
}

func (s *Server) StartMCPServer(ctx context.Context, req *connect.Request[workspacev1.StartMCPServerRequest]) (*connect.Response[workspacev1.StartMCPServerResponse], error) {
	proc, err := s.startPTYProcess(ctx, req.Msg.GetCommand(), req.Msg.GetWorkDir(), req.Msg.GetEnv(), nil)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&workspacev1.StartMCPServerResponse{ProcessId: proc.id}), nil
}

func (s *Server) StopMCPServer(ctx context.Context, req *connect.Request[workspacev1.StopMCPServerRequest]) (*connect.Response[workspacev1.StopMCPServerResponse], error) {
	_, err := s.KillProcess(ctx, connect.NewRequest(&workspacev1.KillProcessRequest{ProcessId: req.Msg.GetProcessId(), Signal: "TERM"}))
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&workspacev1.StopMCPServerResponse{}), nil
}

func (s *Server) startPTYProcess(ctx context.Context, command, workDir string, env []string, size *workspacev1.TerminalResize) (*ptyProcess, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("command is required"))
	}
	resolvedWorkDir, err := s.resolveExecWorkDir(workDir)
	if err != nil {
		return nil, pathError(err)
	}
	procCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	cmd := exec.CommandContext(procCtx, "/bin/sh", "-c", command) //nolint:gosec // G204: intentional workspace command execution.
	cmd.Dir = resolvedWorkDir
	cleanEnv, err := security.SanitizeEnv(append(os.Environ(), "TERM=xterm-256color"), env)
	if err != nil {
		cancel()
		return nil, connect.NewError(connect.CodePermissionDenied, err)
	}
	cmd.Env = cleanEnv

	initialSize := &pty.Winsize{Rows: 24, Cols: 80}
	if size != nil && size.GetCols() > 0 && size.GetRows() > 0 {
		initialSize = ptySize(size)
	}
	ptmx, err := pty.StartWithSize(cmd, initialSize)
	if err != nil {
		cancel()
		return nil, connectErrorf(connect.CodeInternal, "pty start: %v", err)
	}

	proc := &ptyProcess{
		id:        strconv.FormatInt(time.Now().UnixNano(), 36),
		command:   command,
		startedAt: time.Now(),
		status:    "running",
		cmd:       cmd,
		ptmx:      ptmx,
		cancel:    cancel,
		output:    make(chan ptyOutput, 256),
	}

	s.processMu.Lock()
	s.processes[proc.id] = proc
	s.processMu.Unlock()

	go s.collectPTYOutput(proc)
	return proc, nil
}

func (s *Server) collectPTYOutput(proc *ptyProcess) {
	defer func() {
		_ = proc.ptmx.Close()
		s.processMu.Lock()
		delete(s.processes, proc.id)
		s.processMu.Unlock()
		close(proc.output)
	}()

	buf := make([]byte, 4096)
	for {
		n, err := proc.ptmx.Read(buf)
		if n > 0 {
			proc.output <- ptyOutput{data: append([]byte(nil), buf[:n]...)}
		}
		if err != nil {
			break
		}
	}
	exitCode := waitExitCode(proc.cmd.Wait())
	proc.status = "exited"
	proc.output <- ptyOutput{exited: true, exitCode: exitCode}
}

func (s *Server) lookupProcess(processID string) (*ptyProcess, error) {
	processID = strings.TrimSpace(processID)
	if processID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("process_id is required"))
	}
	s.processMu.RLock()
	proc := s.processes[processID]
	s.processMu.RUnlock()
	if proc == nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("process not found"))
	}
	return proc, nil
}

func (s *Server) resolveExecWorkDir(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return s.pathResolver.DefaultWorkDir(), nil
	}
	return s.resolvePath(path)
}

func (s *Server) resolvePath(path string) (string, error) {
	return s.pathResolver.Resolve(path)
}

func connectErrorf(code connect.Code, format string, args ...any) error {
	return connect.NewError(code, fmt.Errorf(format, args...))
}

func pathError(err error) error {
	switch {
	case errors.Is(err, security.ErrPathRequired):
		return connect.NewError(connect.CodeInvalidArgument, errors.New("path is required"))
	case errors.Is(err, security.ErrPathEscapesWorkspace):
		return connect.NewError(connect.CodePermissionDenied, errors.New("path escapes workspace"))
	default:
		return connectErrorf(connect.CodeInternal, "resolve path: %v", err)
	}
}

func waitExitCode(err error) int32 {
	if err == nil {
		return 0
	}
	exitErr := &exec.ExitError{}
	if errors.As(err, &exitErr) {
		ec := exitErr.ExitCode()
		return int32(max(math.MinInt32, min(math.MaxInt32, ec))) //nolint:gosec // G115
	}
	return -1
}

func ptySize(size *workspacev1.TerminalResize) *pty.Winsize {
	return &pty.Winsize{
		Rows: uint16(size.GetRows()), //nolint:gosec // G115
		Cols: uint16(size.GetCols()), //nolint:gosec // G115
	}
}

func collapseHeavySubdirs(entries []*workspacev1.FileEntry, threshold int) []*workspacev1.FileEntry {
	counts := make(map[string]int)
	for _, e := range entries {
		top := listTopDir(e.GetPath())
		if top != "" {
			counts[top]++
		}
	}

	heavy := make(map[string]bool)
	for dir, n := range counts {
		if n > threshold {
			heavy[dir] = true
		}
	}
	if len(heavy) == 0 {
		return entries
	}

	seen := make(map[string]bool)
	out := make([]*workspacev1.FileEntry, 0, len(entries))
	for _, e := range entries {
		path := e.GetPath()
		top := listTopDir(path)

		if !heavy[top] {
			out = append(out, e)
			continue
		}
		if path == top && e.GetIsDir() {
			out = append(out, e)
			continue
		}
		if seen[top] {
			continue
		}
		seen[top] = true
		out = append(out, &workspacev1.FileEntry{
			Path:    top + "/",
			IsDir:   true,
			Summary: fmt.Sprintf("%d items (not expanded)", counts[top]),
		})
	}
	return out
}

func listTopDir(path string) string {
	if i := strings.IndexByte(path, '/'); i >= 0 {
		return path[:i]
	}
	return ""
}

func streamPipe(stream execStream, r io.Reader, kind workspacev1.ExecResponse_Kind) {
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			_ = stream.Send(&workspacev1.ExecResponse{
				Kind: kind,
				Data: append([]byte(nil), buf[:n]...),
			})
		}
		if err != nil {
			break
		}
	}
}

func buildFileEntry(name string, d fs.DirEntry) (*workspacev1.FileEntry, error) {
	info, err := d.Info()
	if err != nil {
		return nil, err
	}
	return &workspacev1.FileEntry{
		Path:    name,
		IsDir:   d.IsDir(),
		Size:    info.Size(),
		Mode:    info.Mode().String(),
		ModTime: info.ModTime().Format(time.RFC3339),
	}, nil
}

func truncateRunes(s string, maxRunes int) string {
	pos := 0
	count := 0
	for pos < len(s) && count < maxRunes {
		_, size := utf8.DecodeRuneInString(s[pos:])
		pos += size
		count++
	}
	return s[:pos]
}
