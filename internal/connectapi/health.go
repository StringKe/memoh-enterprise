package connectapi

import (
	"context"

	"connectrpc.com/connect"

	privatev1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1"
	"github.com/memohai/memoh/internal/connectapi/gen/memoh/private/v1/privatev1connect"
)

type HealthService struct{}

func NewHealthService() *HealthService {
	return &HealthService{}
}

func NewHealthHandler(service *HealthService) Handler {
	path, handler := privatev1connect.NewHealthServiceHandler(service)
	return NewHandler(path, handler)
}

func (*HealthService) Ping(context.Context, *connect.Request[privatev1.PingRequest]) (*connect.Response[privatev1.PingResponse], error) {
	return connect.NewResponse(&privatev1.PingResponse{Status: "ok"}), nil
}

func (*HealthService) Health(context.Context, *connect.Request[privatev1.HealthRequest]) (*connect.Response[privatev1.HealthResponse], error) {
	return connect.NewResponse(&privatev1.HealthResponse{Status: "ok"}), nil
}
