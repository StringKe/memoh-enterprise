package connectapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/memohai/memoh/internal/bots"
	privatev1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1"
	"github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1/privatev1connect"
	workspacev1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/workspace/v1"
	"github.com/memohai/memoh/internal/handlers"
	"github.com/memohai/memoh/internal/workspace"
	"github.com/memohai/memoh/internal/workspace/executorclient"
)

type containerCreator interface {
	CreateContainerStream(ctx context.Context, botID string, req handlers.CreateContainerRequest, send func(any)) error
}

type containerBotAuthorizer interface {
	AuthorizeAccess(ctx context.Context, userID, botID string, isAdmin bool) (bots.Bot, error)
}

type ContainerService struct {
	privatev1connect.UnimplementedContainerServiceHandler

	creator   containerCreator
	bots      containerBotAuthorizer
	executors workspace.ExecutorProvider

	terminalMu sync.RWMutex
	terminals  map[string]*terminalSession
}

type terminalSession struct {
	id     string
	botID  string
	stream *executorclient.ExecStream
	output chan *privatev1.StreamTerminalResponse
	done   chan struct{}
	once   sync.Once
}

func NewContainerService(creator *handlers.ContainerdHandler, bots *bots.Service, executors *workspace.Manager) *ContainerService {
	return &ContainerService{
		creator:   creator,
		bots:      bots,
		executors: executors,
		terminals: make(map[string]*terminalSession),
	}
}

func NewContainerHandler(service *ContainerService) Handler {
	path, handler := privatev1connect.NewContainerServiceHandler(service)
	return NewHandler(path, handler)
}

func (s *ContainerService) StreamContainerProgress(ctx context.Context, req *connect.Request[privatev1.StreamContainerProgressRequest], stream *connect.ServerStream[privatev1.StreamContainerProgressResponse]) error {
	if s.creator == nil {
		return connect.NewError(connect.CodeInternal, errors.New("container creator is not configured"))
	}
	if s.bots == nil {
		return connect.NewError(connect.CodeInternal, errors.New("bot authorizer is not configured"))
	}
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return connect.NewError(connect.CodeUnauthenticated, err)
	}
	botID := strings.TrimSpace(req.Msg.GetBotId())
	if botID == "" {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("bot_id is required"))
	}
	if _, err := s.bots.AuthorizeAccess(ctx, userID, botID, false); err != nil {
		return connectError(err)
	}

	sendErr := make(chan error, 1)
	send := func(payload any) {
		select {
		case <-ctx.Done():
			return
		default:
		}
		msg, err := containerProgressResponseFromPayload(payload)
		if err != nil {
			select {
			case sendErr <- err:
			default:
			}
			return
		}
		if err := stream.Send(msg); err != nil {
			select {
			case sendErr <- err:
			default:
			}
		}
	}

	if err := s.creator.CreateContainerStream(ctx, botID, containerStreamRequest(req.Msg), send); err != nil {
		return connectError(err)
	}
	select {
	case err := <-sendErr:
		if err != nil {
			return err
		}
	default:
	}
	return ctx.Err()
}

func (s *ContainerService) OpenTerminal(ctx context.Context, req *connect.Request[privatev1.OpenTerminalRequest]) (*connect.Response[privatev1.OpenTerminalResponse], error) {
	botID, client, err := s.executorForBot(ctx, req.Msg.GetBotId())
	if err != nil {
		return nil, err
	}
	command := strings.TrimSpace(req.Msg.GetCommand())
	if command == "" {
		command = "/bin/sh"
	}
	cols, rows := terminalSize(req.Msg.GetSize())
	streamCtx := context.WithoutCancel(ctx)
	execStream, err := client.ExecStreamPTY(streamCtx, command, strings.TrimSpace(req.Msg.GetWorkDir()), cols, rows)
	if err != nil {
		return nil, workspaceExecutorError(err)
	}

	terminalID := uuid.NewString()
	session := &terminalSession{
		id:     terminalID,
		botID:  botID,
		stream: execStream,
		output: make(chan *privatev1.StreamTerminalResponse, 128),
		done:   make(chan struct{}),
	}
	s.terminalMu.Lock()
	s.terminals[terminalID] = session
	s.terminalMu.Unlock()
	go s.drainTerminal(session)

	return connect.NewResponse(&privatev1.OpenTerminalResponse{
		TerminalId: terminalID,
		ProcessId:  terminalID,
	}), nil
}

func (s *ContainerService) StreamTerminal(ctx context.Context, req *connect.Request[privatev1.StreamTerminalRequest], stream *connect.ServerStream[privatev1.StreamTerminalResponse]) error {
	session, err := s.terminalForRequest(ctx, req.Msg.GetBotId(), req.Msg.GetTerminalId())
	if err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-session.output:
			if !ok {
				return nil
			}
			if err := stream.Send(msg); err != nil {
				return err
			}
		}
	}
}

func (s *ContainerService) WriteTerminalInput(ctx context.Context, req *connect.Request[privatev1.WriteTerminalInputRequest]) (*connect.Response[privatev1.WriteTerminalInputResponse], error) {
	session, err := s.terminalForRequest(ctx, req.Msg.GetBotId(), req.Msg.GetTerminalId())
	if err != nil {
		return nil, err
	}
	if err := session.stream.SendStdin(req.Msg.GetData()); err != nil {
		return nil, workspaceExecutorError(err)
	}
	return connect.NewResponse(&privatev1.WriteTerminalInputResponse{}), nil
}

func (s *ContainerService) ResizeTerminal(ctx context.Context, req *connect.Request[privatev1.ResizeTerminalRequest]) (*connect.Response[privatev1.ResizeTerminalResponse], error) {
	session, err := s.terminalForRequest(ctx, req.Msg.GetBotId(), req.Msg.GetTerminalId())
	if err != nil {
		return nil, err
	}
	cols, rows := terminalSize(req.Msg.GetSize())
	if err := session.stream.Resize(cols, rows); err != nil {
		return nil, workspaceExecutorError(err)
	}
	return connect.NewResponse(&privatev1.ResizeTerminalResponse{}), nil
}

func (s *ContainerService) CloseTerminal(ctx context.Context, req *connect.Request[privatev1.CloseTerminalRequest]) (*connect.Response[privatev1.CloseTerminalResponse], error) {
	session, err := s.terminalForRequest(ctx, req.Msg.GetBotId(), req.Msg.GetTerminalId())
	if err != nil {
		return nil, err
	}
	session.close()
	s.removeTerminal(session.id)
	return connect.NewResponse(&privatev1.CloseTerminalResponse{}), nil
}

func (s *ContainerService) ListContainerFiles(ctx context.Context, req *connect.Request[privatev1.ListContainerFilesRequest]) (*connect.Response[privatev1.ListContainerFilesResponse], error) {
	_, client, err := s.executorForBot(ctx, req.Msg.GetBotId())
	if err != nil {
		return nil, err
	}
	offset, err := parseFilePageToken(req.Msg.GetPageToken())
	if err != nil {
		return nil, err
	}
	limit := req.Msg.GetPageSize()
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	result, err := client.ListDir(ctx, strings.TrimSpace(req.Msg.GetPath()), req.Msg.GetRecursive(), offset, limit, 0)
	if err != nil {
		return nil, workspaceExecutorError(err)
	}
	entries := make([]*privatev1.ContainerFileEntry, 0, len(result.Entries))
	for _, entry := range result.Entries {
		entries = append(entries, containerFileEntryToProto(entry))
	}
	nextPageToken := ""
	if result.Truncated {
		nextPageToken = strconv.Itoa(int(offset) + len(entries))
	}
	return connect.NewResponse(&privatev1.ListContainerFilesResponse{
		Entries:       entries,
		NextPageToken: nextPageToken,
	}), nil
}

func (s *ContainerService) ReadContainerFile(ctx context.Context, req *connect.Request[privatev1.ReadContainerFileRequest]) (*connect.Response[privatev1.ReadContainerFileResponse], error) {
	_, client, err := s.executorForBot(ctx, req.Msg.GetBotId())
	if err != nil {
		return nil, err
	}
	content, eof, err := readContainerFileBytes(ctx, client, req.Msg.GetPath(), req.Msg.GetOffset(), req.Msg.GetMaxBytes())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&privatev1.ReadContainerFileResponse{
		Content: content,
		Binary:  bytes.IndexByte(content, 0) >= 0,
		Eof:     eof,
	}), nil
}

func (s *ContainerService) WriteContainerFile(ctx context.Context, req *connect.Request[privatev1.WriteContainerFileRequest]) (*connect.Response[privatev1.WriteContainerFileResponse], error) {
	_, client, err := s.executorForBot(ctx, req.Msg.GetBotId())
	if err != nil {
		return nil, err
	}
	written, err := client.WriteRaw(ctx, strings.TrimSpace(req.Msg.GetPath()), bytes.NewReader(req.Msg.GetContent()))
	if err != nil {
		return nil, workspaceExecutorError(err)
	}
	return connect.NewResponse(&privatev1.WriteContainerFileResponse{BytesWritten: written}), nil
}

func (s *ContainerService) UploadContainerFile(ctx context.Context, req *connect.Request[privatev1.UploadContainerFileRequest]) (*connect.Response[privatev1.UploadContainerFileResponse], error) {
	_, client, err := s.executorForBot(ctx, req.Msg.GetBotId())
	if err != nil {
		return nil, err
	}
	written, err := client.WriteRaw(ctx, strings.TrimSpace(req.Msg.GetPath()), bytes.NewReader(req.Msg.GetContent()))
	if err != nil {
		return nil, workspaceExecutorError(err)
	}
	return connect.NewResponse(&privatev1.UploadContainerFileResponse{BytesWritten: written}), nil
}

func (s *ContainerService) DownloadContainerFile(ctx context.Context, req *connect.Request[privatev1.DownloadContainerFileRequest]) (*connect.Response[privatev1.DownloadContainerFileResponse], error) {
	_, client, err := s.executorForBot(ctx, req.Msg.GetBotId())
	if err != nil {
		return nil, err
	}
	content, _, err := readContainerFileBytes(ctx, client, req.Msg.GetPath(), 0, containerDownloadMaxBytes)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&privatev1.DownloadContainerFileResponse{
		Content:  content,
		MimeType: http.DetectContentType(content),
		Filename: path.Base(strings.TrimSpace(req.Msg.GetPath())),
	}), nil
}

func (s *ContainerService) MkdirContainerFile(ctx context.Context, req *connect.Request[privatev1.MkdirContainerFileRequest]) (*connect.Response[privatev1.MkdirContainerFileResponse], error) {
	_, client, err := s.executorForBot(ctx, req.Msg.GetBotId())
	if err != nil {
		return nil, err
	}
	if err := client.Mkdir(ctx, strings.TrimSpace(req.Msg.GetPath())); err != nil {
		return nil, workspaceExecutorError(err)
	}
	return connect.NewResponse(&privatev1.MkdirContainerFileResponse{}), nil
}

func (s *ContainerService) RenameContainerFile(ctx context.Context, req *connect.Request[privatev1.RenameContainerFileRequest]) (*connect.Response[privatev1.RenameContainerFileResponse], error) {
	_, client, err := s.executorForBot(ctx, req.Msg.GetBotId())
	if err != nil {
		return nil, err
	}
	if !req.Msg.GetOverwrite() {
		if _, err := client.Stat(ctx, strings.TrimSpace(req.Msg.GetNewPath())); err == nil {
			return nil, connect.NewError(connect.CodeAlreadyExists, errors.New("target path already exists"))
		} else if !errors.Is(err, executorclient.ErrNotFound) {
			return nil, workspaceExecutorError(err)
		}
	}
	if err := client.Rename(ctx, strings.TrimSpace(req.Msg.GetOldPath()), strings.TrimSpace(req.Msg.GetNewPath())); err != nil {
		return nil, workspaceExecutorError(err)
	}
	return connect.NewResponse(&privatev1.RenameContainerFileResponse{}), nil
}

func (s *ContainerService) DeleteContainerFile(ctx context.Context, req *connect.Request[privatev1.DeleteContainerFileRequest]) (*connect.Response[privatev1.DeleteContainerFileResponse], error) {
	_, client, err := s.executorForBot(ctx, req.Msg.GetBotId())
	if err != nil {
		return nil, err
	}
	if err := client.DeleteFile(ctx, strings.TrimSpace(req.Msg.GetPath()), req.Msg.GetRecursive()); err != nil {
		return nil, workspaceExecutorError(err)
	}
	return connect.NewResponse(&privatev1.DeleteContainerFileResponse{}), nil
}

func (s *ContainerService) ImportContainerData(ctx context.Context, req *connect.Request[privatev1.ImportContainerDataRequest]) (*connect.Response[privatev1.ImportContainerDataResponse], error) {
	botID, client, err := s.executorForBot(ctx, req.Msg.GetBotId())
	if err != nil {
		return nil, err
	}
	target := strings.TrimSpace(req.Msg.GetSource())
	if target == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("source is required"))
	}
	if _, err := client.WriteRaw(ctx, target, bytes.NewReader(req.Msg.GetData())); err != nil {
		return nil, workspaceExecutorError(err)
	}
	return connect.NewResponse(&privatev1.ImportContainerDataResponse{Operation: completedWorkspaceOperation(botID, "import_container_data")}), nil
}

func (s *ContainerService) ExportContainerData(ctx context.Context, req *connect.Request[privatev1.ExportContainerDataRequest]) (*connect.Response[privatev1.ExportContainerDataResponse], error) {
	_, client, err := s.executorForBot(ctx, req.Msg.GetBotId())
	if err != nil {
		return nil, err
	}
	content, _, err := readContainerFileBytes(ctx, client, req.Msg.GetPath(), 0, containerDownloadMaxBytes)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&privatev1.ExportContainerDataResponse{
		Data:     content,
		MimeType: http.DetectContentType(content),
		Filename: path.Base(strings.TrimSpace(req.Msg.GetPath())),
	}), nil
}

const (
	containerReadDefaultMaxBytes = 1 << 20
	containerDownloadMaxBytes    = 16 << 20
)

func (s *ContainerService) executorForBot(ctx context.Context, rawBotID string) (string, *executorclient.Client, error) {
	botID, err := s.requireContainerAccess(ctx, rawBotID)
	if err != nil {
		return "", nil, err
	}
	if s.executors == nil {
		return "", nil, connect.NewError(connect.CodeInternal, errors.New("workspace executor provider is not configured"))
	}
	client, err := s.executors.ExecutorClient(ctx, botID)
	if err != nil {
		return "", nil, workspaceExecutorError(err)
	}
	return botID, client, nil
}

func (s *ContainerService) requireContainerAccess(ctx context.Context, rawBotID string) (string, error) {
	if s.bots == nil {
		return "", connect.NewError(connect.CodeInternal, errors.New("bot authorizer is not configured"))
	}
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return "", connect.NewError(connect.CodeUnauthenticated, err)
	}
	botID := strings.TrimSpace(rawBotID)
	if botID == "" {
		return "", connect.NewError(connect.CodeInvalidArgument, errors.New("bot_id is required"))
	}
	if _, err := s.bots.AuthorizeAccess(ctx, userID, botID, false); err != nil {
		return "", connectError(err)
	}
	return botID, nil
}

func (s *ContainerService) terminalForRequest(ctx context.Context, rawBotID, rawTerminalID string) (*terminalSession, error) {
	botID, err := s.requireContainerAccess(ctx, rawBotID)
	if err != nil {
		return nil, err
	}
	terminalID := strings.TrimSpace(rawTerminalID)
	if terminalID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("terminal_id is required"))
	}
	s.terminalMu.RLock()
	session := s.terminals[terminalID]
	s.terminalMu.RUnlock()
	if session == nil || session.botID != botID {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("terminal not found"))
	}
	return session, nil
}

func (s *ContainerService) drainTerminal(session *terminalSession) {
	defer func() {
		session.close()
		close(session.output)
		s.removeTerminal(session.id)
	}()
	for {
		msg, err := session.stream.Recv()
		if errors.Is(err, io.EOF) {
			return
		}
		if err != nil {
			session.send(&privatev1.StreamTerminalResponse{
				Exited:    true,
				ExitCode:  -1,
				CreatedAt: timestamppb.Now(),
			})
			return
		}
		switch msg.GetKind() {
		case workspacev1.ExecResponse_KIND_EXIT:
			session.send(&privatev1.StreamTerminalResponse{
				Exited:    true,
				ExitCode:  msg.GetExitCode(),
				CreatedAt: timestamppb.Now(),
			})
			return
		default:
			if data := msg.GetData(); len(data) > 0 {
				session.send(&privatev1.StreamTerminalResponse{
					Data:      data,
					CreatedAt: timestamppb.Now(),
				})
			}
		}
	}
}

func (s *ContainerService) removeTerminal(terminalID string) {
	s.terminalMu.Lock()
	delete(s.terminals, terminalID)
	s.terminalMu.Unlock()
}

func (s *terminalSession) send(msg *privatev1.StreamTerminalResponse) {
	select {
	case s.output <- msg:
	case <-s.done:
	}
}

func (s *terminalSession) close() {
	s.once.Do(func() {
		close(s.done)
		_ = s.stream.Close()
	})
}

func terminalSize(size *privatev1.TerminalSize) (uint32, uint32) {
	cols := size.GetCols()
	rows := size.GetRows()
	if cols == 0 {
		cols = 80
	}
	if rows == 0 {
		rows = 24
	}
	return cols, rows
}

func parseFilePageToken(token string) (int32, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return 0, nil
	}
	offset, err := strconv.Atoi(token)
	if err != nil || offset < 0 {
		return 0, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid page_token"))
	}
	return int32(offset), nil //nolint:gosec // offset is validated by strconv and page sizes are bounded.
}

func readContainerFileBytes(ctx context.Context, client *executorclient.Client, filePath string, offset, maxBytes int64) ([]byte, bool, error) {
	filePath = strings.TrimSpace(filePath)
	if filePath == "" {
		return nil, false, connect.NewError(connect.CodeInvalidArgument, errors.New("path is required"))
	}
	if offset < 0 {
		return nil, false, connect.NewError(connect.CodeInvalidArgument, errors.New("offset must be non-negative"))
	}
	if maxBytes <= 0 {
		maxBytes = containerReadDefaultMaxBytes
	}
	if maxBytes > containerDownloadMaxBytes {
		return nil, false, connect.NewError(connect.CodeResourceExhausted, errors.New("max_bytes exceeds limit"))
	}
	rc, err := client.ReadRaw(ctx, filePath)
	if err != nil {
		return nil, false, workspaceExecutorError(err)
	}
	defer func() { _ = rc.Close() }()
	if offset > 0 {
		if _, err := io.CopyN(io.Discard, rc, offset); err != nil {
			if errors.Is(err, io.EOF) {
				return nil, true, nil
			}
			return nil, false, workspaceExecutorError(err)
		}
	}
	limited := io.LimitReader(rc, maxBytes+1)
	content, err := io.ReadAll(limited)
	if err != nil {
		return nil, false, workspaceExecutorError(err)
	}
	eof := int64(len(content)) <= maxBytes
	if !eof {
		content = content[:maxBytes]
	}
	return content, eof, nil
}

func containerFileEntryToProto(entry *workspacev1.FileEntry) *privatev1.ContainerFileEntry {
	if entry == nil {
		return nil
	}
	var modifiedAt *timestamppb.Timestamp
	if parsed, err := time.Parse(time.RFC3339, entry.GetModTime()); err == nil {
		modifiedAt = timestamppb.New(parsed)
	}
	return &privatev1.ContainerFileEntry{
		Path:       entry.GetPath(),
		IsDir:      entry.GetIsDir(),
		Size:       entry.GetSize(),
		Mode:       entry.GetMode(),
		ModifiedAt: modifiedAt,
	}
}

func completedWorkspaceOperation(botID, operationType string) *privatev1.WorkspaceOperation {
	return &privatev1.WorkspaceOperation{
		OperationId:   uuid.NewString(),
		BotId:         botID,
		OperationType: operationType,
		Status:        "completed",
		CreatedAt:     timestamppb.Now(),
	}
}

func workspaceExecutorError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, executorclient.ErrNotFound):
		return connect.NewError(connect.CodeNotFound, err)
	case errors.Is(err, executorclient.ErrBadRequest):
		return connect.NewError(connect.CodeInvalidArgument, err)
	case errors.Is(err, executorclient.ErrForbidden):
		return connect.NewError(connect.CodePermissionDenied, err)
	case errors.Is(err, executorclient.ErrUnavailable):
		return connect.NewError(connect.CodeUnavailable, err)
	default:
		return connect.NewError(connect.CodeInternal, err)
	}
}

func containerStreamRequest(req *privatev1.StreamContainerProgressRequest) handlers.CreateContainerRequest {
	options := req.GetOptions().AsMap()
	out := handlers.CreateContainerRequest{
		Snapshotter:        optionString(options, "snapshotter"),
		Image:              optionString(options, "image"),
		WorkspaceBackend:   firstOptionString(options, "workspace_backend", "workspaceBackend"),
		LocalWorkspacePath: firstOptionString(options, "local_workspace_path", "localWorkspacePath"),
		RestoreData:        optionBool(options, "restore_data") || optionBool(options, "restoreData"),
	}
	if devices := optionStringSlice(options, "gpu_devices"); len(devices) > 0 {
		out.GPU = &handlers.ContainerGPURequest{Devices: devices}
	} else if gpu, ok := options["gpu"].(map[string]any); ok {
		if devices := optionStringSlice(gpu, "devices"); len(devices) > 0 {
			out.GPU = &handlers.ContainerGPURequest{Devices: devices}
		}
	}
	return out
}

func containerProgressResponseFromPayload(payload any) (*privatev1.StreamContainerProgressResponse, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	body, err := structFromJSON(data)
	if err != nil {
		return nil, err
	}
	fields := body.AsMap()
	return &privatev1.StreamContainerProgressResponse{
		Id:        stringValue(fields, "id"),
		Type:      stringValue(fields, "type"),
		Message:   firstStringValue(fields, "message", "error"),
		Payload:   body,
		CreatedAt: timestamppb.New(time.Now()),
	}, nil
}

func optionString(options map[string]any, key string) string {
	value, _ := options[key].(string)
	return strings.TrimSpace(value)
}

func firstOptionString(options map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := optionString(options, key); value != "" {
			return value
		}
	}
	return ""
}

func optionBool(options map[string]any, key string) bool {
	value, _ := options[key].(bool)
	return value
}

func optionStringSlice(options map[string]any, key string) []string {
	raw, ok := options[key].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		value, ok := item.(string)
		if !ok || strings.TrimSpace(value) == "" {
			continue
		}
		out = append(out, strings.TrimSpace(value))
	}
	return out
}
