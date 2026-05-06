package connectapi

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/memohai/memoh/internal/bots"
	privatev1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1"
	"github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1/privatev1connect"
	"github.com/memohai/memoh/internal/handlers"
)

type containerCreator interface {
	CreateContainerStream(ctx context.Context, botID string, req handlers.CreateContainerRequest, send func(any)) error
}

type containerBotAuthorizer interface {
	AuthorizeAccess(ctx context.Context, userID, botID string, isAdmin bool) (bots.Bot, error)
}

type ContainerService struct {
	creator containerCreator
	bots    containerBotAuthorizer
}

func NewContainerService(creator *handlers.ContainerdHandler, bots *bots.Service) *ContainerService {
	return &ContainerService{creator: creator, bots: bots}
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
