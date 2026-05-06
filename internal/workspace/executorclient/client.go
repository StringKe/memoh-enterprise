// Package executorclient provides a ConnectRPC client for the workspace executor service.
package executorclient

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"connectrpc.com/connect"
	"golang.org/x/net/http2"

	workspacev1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/workspace/v1"
	"github.com/memohai/memoh/internal/connectapi/gen/memoh/workspace/v1/workspacev1connect"
)

const clientMaxAge = 10 * time.Minute

// Client wraps a ConnectRPC client to a single workspace executor.
type Client struct {
	svc       workspacev1connect.WorkspaceExecutorServiceClient
	close     func() error
	target    string
	createdAt time.Time
}

// NewClient wraps an existing generated ConnectRPC client.
func NewClient(svc workspacev1connect.WorkspaceExecutorServiceClient, closeFn func() error) *Client {
	return &Client{svc: svc, close: closeFn, createdAt: time.Now()}
}

// Dial creates a new Client connected to the given target.
// For UDS use "unix:///path/to/sock"; for TCP use "host:port" or an http URL.
func Dial(_ context.Context, target string) (*Client, error) {
	baseURL, httpClient, closeFn, err := transportForTarget(target)
	if err != nil {
		return nil, err
	}
	return &Client{
		svc:       workspacev1connect.NewWorkspaceExecutorServiceClient(httpClient, baseURL),
		close:     closeFn,
		target:    target,
		createdAt: time.Now(),
	}, nil
}

func transportForTarget(target string) (string, *http.Client, func() error, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", nil, nil, errors.New("workspace executor target is required")
	}

	if strings.HasPrefix(target, "unix://") {
		socketPath := strings.TrimPrefix(target, "unix://")
		if socketPath == "" {
			return "", nil, nil, errors.New("workspace executor unix socket path is required")
		}
		transport := &http2.Transport{
			AllowHTTP: true,
			DialTLSContext: func(ctx context.Context, _, _ string, _ *tls.Config) (net.Conn, error) {
				var dialer net.Dialer
				return dialer.DialContext(ctx, "unix", socketPath)
			},
		}
		return "http://workspace-executor", &http.Client{Transport: transport}, func() error {
			transport.CloseIdleConnections()
			return nil
		}, nil
	}

	baseURL := target
	if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		baseURL = "http://" + baseURL
	}
	transport := &http2.Transport{AllowHTTP: strings.HasPrefix(baseURL, "http://")}
	if strings.HasPrefix(baseURL, "http://") {
		transport.DialTLSContext = func(ctx context.Context, network, address string, _ *tls.Config) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, network, address)
		}
	}
	return strings.TrimRight(baseURL, "/"), &http.Client{Transport: transport}, func() error {
		transport.CloseIdleConnections()
		return nil
	}, nil
}

func (c *Client) Close() error {
	if c.close == nil {
		return nil
	}
	return c.close()
}

func (c *Client) ReadFile(ctx context.Context, path string, lineOffset, lineCount int32) (*workspacev1.ReadFileResponse, error) {
	resp, err := c.svc.ReadFile(ctx, connect.NewRequest(&workspacev1.ReadFileRequest{
		Path:       path,
		LineOffset: lineOffset,
		LineCount:  lineCount,
	}))
	if err != nil {
		return nil, mapError(err)
	}
	return resp.Msg, nil
}

func (c *Client) WriteFile(ctx context.Context, path string, content []byte) error {
	_, err := c.svc.WriteFile(ctx, connect.NewRequest(&workspacev1.WriteFileRequest{
		Path:             path,
		Content:          content,
		CreateParentDirs: true,
	}))
	return mapError(err)
}

// ListDirResult holds the paginated result of a directory listing.
type ListDirResult struct {
	Entries    []*workspacev1.FileEntry
	TotalCount int32
	Truncated  bool
}

func (c *Client) ListDir(ctx context.Context, dir string, recursive bool, offset, limit, collapseThreshold int32) (*ListDirResult, error) {
	pageToken := ""
	if offset > 0 {
		pageToken = strconv.FormatInt(int64(offset), 10)
	}
	resp, err := c.svc.ListDir(ctx, connect.NewRequest(&workspacev1.ListDirRequest{
		Path:              dir,
		Recursive:         recursive,
		PageSize:          limit,
		PageToken:         pageToken,
		CollapseThreshold: collapseThreshold,
	}))
	if err != nil {
		return nil, mapError(err)
	}
	return &ListDirResult{
		Entries:    resp.Msg.GetEntries(),
		TotalCount: resp.Msg.GetTotalCount(),
		Truncated:  resp.Msg.GetTruncated(),
	}, nil
}

// ListDirAll lists all entries without pagination.
func (c *Client) ListDirAll(ctx context.Context, path string, recursive bool) ([]*workspacev1.FileEntry, error) {
	result, err := c.ListDir(ctx, path, recursive, 0, 0, 0)
	if err != nil {
		return nil, err
	}
	return result.Entries, nil
}

func (c *Client) Stat(ctx context.Context, path string) (*workspacev1.FileEntry, error) {
	resp, err := c.svc.Stat(ctx, connect.NewRequest(&workspacev1.StatRequest{Path: path}))
	if err != nil {
		return nil, mapError(err)
	}
	return resp.Msg.GetEntry(), nil
}

func (c *Client) Mkdir(ctx context.Context, path string) error {
	_, err := c.svc.Mkdir(ctx, connect.NewRequest(&workspacev1.MkdirRequest{Path: path, Parents: true}))
	return mapError(err)
}

func (c *Client) Rename(ctx context.Context, oldPath, newPath string) error {
	_, err := c.svc.Rename(ctx, connect.NewRequest(&workspacev1.RenameRequest{
		OldPath:   oldPath,
		NewPath:   newPath,
		Overwrite: true,
	}))
	return mapError(err)
}

// ExecResult holds the output of a non-streaming exec call.
type ExecResult struct {
	Stdout   string
	Stderr   string
	ExitCode int32
}

// Exec runs a command and collects all output. For streaming, use ExecStream.
func (c *Client) Exec(ctx context.Context, command, workDir string, timeout int32) (*ExecResult, error) {
	return c.ExecWithStdin(ctx, command, workDir, timeout, nil)
}

// ExecWithStdin runs a command with optional stdin data.
func (c *Client) ExecWithStdin(ctx context.Context, command, workDir string, timeout int32, stdinData []byte) (*ExecResult, error) {
	stream, err := c.ExecStream(ctx, command, workDir, timeout)
	if err != nil {
		return nil, err
	}
	if len(stdinData) > 0 {
		if err := stream.SendStdin(stdinData); err != nil {
			_ = stream.Close()
			return nil, mapError(err)
		}
	}
	if err := stream.CloseRequest(); err != nil {
		_ = stream.Close()
		return nil, mapError(err)
	}

	var stdout, stderr bytes.Buffer
	var exitCode int32
	for {
		msg, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			_ = stream.Close()
			return nil, mapError(err)
		}
		switch msg.GetKind() {
		case workspacev1.ExecResponse_KIND_STDOUT:
			stdout.Write(msg.GetData())
		case workspacev1.ExecResponse_KIND_STDERR:
			stderr.Write(msg.GetData())
		case workspacev1.ExecResponse_KIND_EXIT:
			exitCode = msg.GetExitCode()
		case workspacev1.ExecResponse_KIND_ERROR:
			_ = stream.Close()
			return nil, fmt.Errorf("%s: %s", msg.GetErrorCode(), msg.GetErrorMessage())
		}
	}
	_ = stream.Close()

	return &ExecResult{Stdout: stdout.String(), Stderr: stderr.String(), ExitCode: exitCode}, nil
}

// ExecStream returns a bidirectional stream for interactive exec.
func (c *Client) ExecStream(ctx context.Context, command, workDir string, timeout int32) (*ExecStream, error) {
	streamCtx, cancel := context.WithCancel(ctx)
	stream := c.svc.Exec(streamCtx)
	err := stream.Send(&workspacev1.ExecRequest{Frame: &workspacev1.ExecRequest_Start{Start: &workspacev1.ExecStart{
		Command:        command,
		WorkDir:        workDir,
		TimeoutSeconds: timeout,
	}}})
	if err != nil {
		cancel()
		return nil, mapError(err)
	}
	return &ExecStream{stream: stream, cancel: cancel}, nil
}

// ExecStream wraps a bidirectional exec stream.
type ExecStream struct {
	stream *connect.BidiStreamForClient[workspacev1.ExecRequest, workspacev1.ExecResponse]
	cancel context.CancelFunc
}

// SendStdin sends data to the process stdin.
func (s *ExecStream) SendStdin(data []byte) error {
	return s.stream.Send(&workspacev1.ExecRequest{Frame: &workspacev1.ExecRequest_StdinData{StdinData: data}})
}

// Recv receives output from the process.
func (s *ExecStream) Recv() (*workspacev1.ExecResponse, error) {
	return s.stream.Receive()
}

// Resize sends a terminal resize event to the running process.
func (s *ExecStream) Resize(cols, rows uint32) error {
	return s.stream.Send(&workspacev1.ExecRequest{Frame: &workspacev1.ExecRequest_Resize{Resize: &workspacev1.TerminalResize{
		Cols: cols,
		Rows: rows,
	}}})
}

// CloseRequest closes the client-to-server side of the stream.
func (s *ExecStream) CloseRequest() error {
	return s.stream.CloseRequest()
}

// Close closes the stream.
func (s *ExecStream) Close() error {
	if s.cancel != nil {
		s.cancel()
	}
	return s.stream.CloseResponse()
}

// ExecStreamPTY opens a bidirectional PTY exec stream.
func (c *Client) ExecStreamPTY(ctx context.Context, command, workDir string, cols, rows uint32) (*ExecStream, error) {
	streamCtx, cancel := context.WithCancel(ctx)
	stream := c.svc.Exec(streamCtx)
	err := stream.Send(&workspacev1.ExecRequest{Frame: &workspacev1.ExecRequest_Start{Start: &workspacev1.ExecStart{
		Command: command,
		WorkDir: workDir,
		Pty:     true,
	}}})
	if err != nil {
		cancel()
		return nil, mapError(err)
	}
	if cols > 0 && rows > 0 {
		if err := stream.Send(&workspacev1.ExecRequest{Frame: &workspacev1.ExecRequest_Resize{Resize: &workspacev1.TerminalResize{
			Cols: cols,
			Rows: rows,
		}}}); err != nil {
			cancel()
			return nil, mapError(err)
		}
	}
	return &ExecStream{stream: stream, cancel: cancel}, nil
}

// ReadRaw streams raw file bytes. Caller must consume the returned reader.
func (c *Client) ReadRaw(ctx context.Context, path string) (io.ReadCloser, error) {
	stream, err := c.svc.ReadRaw(ctx, connect.NewRequest(&workspacev1.ReadRawRequest{Path: path}))
	if err != nil {
		return nil, mapError(err)
	}
	return newStreamReader(stream)
}

// WriteRaw writes raw bytes to a file in the workspace.
func (c *Client) WriteRaw(ctx context.Context, path string, r io.Reader) (int64, error) {
	stream := c.svc.WriteRaw(ctx)
	buf := make([]byte, 64*1024)
	first := true
	var offset int64
	for {
		n, readErr := r.Read(buf)
		if n > 0 {
			chunk := &workspacev1.WriteRawChunk{
				Data:   append([]byte(nil), buf[:n]...),
				Offset: offset,
			}
			if first {
				chunk.Path = path
				chunk.Truncate = true
				first = false
			}
			if sendErr := stream.Send(&workspacev1.WriteRawRequest{Chunk: chunk}); sendErr != nil {
				return 0, mapError(sendErr)
			}
			offset += int64(n)
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return 0, readErr
		}
	}
	if first {
		if err := stream.Send(&workspacev1.WriteRawRequest{Chunk: &workspacev1.WriteRawChunk{Path: path, Truncate: true}}); err != nil {
			return 0, mapError(err)
		}
	}

	resp, err := stream.CloseAndReceive()
	if err != nil {
		return 0, mapError(err)
	}
	return resp.Msg.GetBytesWritten(), nil
}

func (c *Client) DeleteFile(ctx context.Context, path string, recursive bool) error {
	_, err := c.svc.DeleteFile(ctx, connect.NewRequest(&workspacev1.DeleteFileRequest{Path: path, Recursive: recursive}))
	return mapError(err)
}

// streamReader adapts a Connect server stream into an io.ReadCloser.
type streamReader struct {
	stream *connect.ServerStreamForClient[workspacev1.ReadRawResponse]
	buf    []byte
	off    int
	closed bool
}

func newStreamReader(stream *connect.ServerStreamForClient[workspacev1.ReadRawResponse]) (io.ReadCloser, error) {
	reader := &streamReader{stream: stream}
	if err := reader.fill(); err != nil {
		if errors.Is(err, io.EOF) {
			return io.NopCloser(bytes.NewReader(nil)), nil
		}
		_ = stream.Close()
		return nil, mapError(err)
	}
	return reader, nil
}

func (r *streamReader) fill() error {
	for r.off >= len(r.buf) {
		if !r.stream.Receive() {
			if err := r.stream.Err(); err != nil {
				return mapError(err)
			}
			return io.EOF
		}
		chunk := r.stream.Msg().GetChunk()
		if chunk == nil {
			r.buf = nil
		} else {
			r.buf = chunk.GetData()
		}
		r.off = 0
		if len(r.buf) == 0 && chunk != nil && chunk.GetEof() {
			return io.EOF
		}
	}
	return nil
}

func (r *streamReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if err := r.fill(); err != nil {
		return 0, err
	}
	n := copy(p, r.buf[r.off:])
	r.off += n
	return n, nil
}

func (r *streamReader) Close() error {
	if r.closed {
		return nil
	}
	r.closed = true
	return r.stream.Close()
}

// Provider resolves a workspace executor client for a given bot.
type Provider interface {
	ExecutorClient(ctx context.Context, botID string) (*Client, error)
}

// Pool manages cached ConnectRPC clients keyed by bot ID.
type Pool struct {
	mu             sync.RWMutex
	clients        map[string]*Client
	dialTargetFunc func(botID string) string
}

// NewPool creates a client pool. dialTargetFunc maps bot ID to a Connect target.
func NewPool(dialTargetFunc func(string) string) *Pool {
	return &Pool{
		clients:        make(map[string]*Client),
		dialTargetFunc: dialTargetFunc,
	}
}

// ExecutorClient implements Provider. Alias for Get.
func (p *Pool) ExecutorClient(ctx context.Context, botID string) (*Client, error) {
	return p.Get(ctx, botID)
}

// Get returns a cached client or dials a new one.
func (p *Pool) Get(ctx context.Context, botID string) (*Client, error) {
	p.mu.RLock()
	if c, ok := p.clients[botID]; ok && time.Since(c.createdAt) < clientMaxAge {
		p.mu.RUnlock()
		return c, nil
	}
	p.mu.RUnlock()
	p.Remove(botID)

	target := p.dialTargetFunc(botID)
	if target == "" {
		return nil, fmt.Errorf("no workspace executor target for bot %s", botID)
	}

	c, err := Dial(ctx, target)
	if err != nil {
		return nil, err
	}

	p.mu.Lock()
	if existing, ok := p.clients[botID]; ok {
		p.mu.Unlock()
		_ = c.Close()
		return existing, nil
	}
	p.clients[botID] = c
	p.mu.Unlock()
	return c, nil
}

// Remove closes and removes the client for a bot.
func (p *Pool) Remove(botID string) {
	p.mu.Lock()
	if c, ok := p.clients[botID]; ok {
		_ = c.Close()
		delete(p.clients, botID)
	}
	p.mu.Unlock()
}

// CloseAll closes all cached clients.
func (p *Pool) CloseAll() {
	p.mu.Lock()
	for id, c := range p.clients {
		_ = c.Close()
		delete(p.clients, id)
	}
	p.mu.Unlock()
}
