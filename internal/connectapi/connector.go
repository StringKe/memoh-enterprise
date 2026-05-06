package connectapi

import (
	"context"
	"errors"
	"io"
	"time"

	"connectrpc.com/connect"

	"github.com/memohai/memoh/internal/connector"
)

type ConnectorOutboundSource interface {
	StreamOutbound(ctx context.Context, token connector.LeaseToken, send func(context.Context, connector.OutboundCommand) error) error
}

type ConnectorService struct {
	connectors         *connector.Service
	outbound           ConnectorOutboundSource
	staleCheckInterval time.Duration
}

func NewConnectorService(connectors *connector.Service, outbound ConnectorOutboundSource) *ConnectorService {
	return &ConnectorService{
		connectors:         connectors,
		outbound:           outbound,
		staleCheckInterval: connector.DefaultRenewInterval,
	}
}

func (s *ConnectorService) Register(ctx context.Context, req *connect.Request[connector.RegisterRequest]) (*connect.Response[connector.RegisterResponse], error) {
	if s.connectors == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("connector service not configured"))
	}
	instance, err := s.connectors.Register(ctx, *req.Msg)
	if err != nil {
		return nil, connectorConnectError(err)
	}
	return connect.NewResponse(&connector.RegisterResponse{Instance: instance}), nil
}

func (s *ConnectorService) AcquireLease(ctx context.Context, req *connect.Request[connector.AcquireLeaseRequest]) (*connect.Response[connector.AcquireLeaseResponse], error) {
	if s.connectors == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("connector service not configured"))
	}
	lease, err := s.connectors.AcquireLease(ctx, *req.Msg)
	if err != nil {
		return nil, connectorConnectError(err)
	}
	return connect.NewResponse(&connector.AcquireLeaseResponse{Lease: lease}), nil
}

func (s *ConnectorService) RenewLease(ctx context.Context, req *connect.Request[connector.RenewLeaseRequest]) (*connect.Response[connector.RenewLeaseResponse], error) {
	if s.connectors == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("connector service not configured"))
	}
	lease, err := s.connectors.RenewLease(ctx, req.Msg.Token)
	if err != nil {
		return nil, connectorConnectError(err)
	}
	return connect.NewResponse(&connector.RenewLeaseResponse{Lease: lease}), nil
}

func (s *ConnectorService) ReleaseLease(ctx context.Context, req *connect.Request[connector.ReleaseLeaseRequest]) (*connect.Response[connector.ReleaseLeaseResponse], error) {
	if s.connectors == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("connector service not configured"))
	}
	if err := s.connectors.ReleaseLease(ctx, req.Msg.Token); err != nil {
		return nil, connectorConnectError(err)
	}
	return connect.NewResponse(&connector.ReleaseLeaseResponse{}), nil
}

func (s *ConnectorService) ReportStatus(ctx context.Context, req *connect.Request[connector.ReportStatusRequest]) (*connect.Response[connector.ReportStatusResponse], error) {
	if s.connectors == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("connector service not configured"))
	}
	if err := s.connectors.ReportStatus(ctx, req.Msg.Status); err != nil {
		return nil, connectorConnectError(err)
	}
	return connect.NewResponse(&connector.ReportStatusResponse{}), nil
}

func (s *ConnectorService) StreamInbound(ctx context.Context, stream *connect.ClientStream[connector.InboundEvent]) (*connect.Response[connector.StreamInboundResponse], error) {
	if s.connectors == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("connector service not configured"))
	}
	for stream.Receive() {
		event := stream.Msg()
		if event == nil {
			continue
		}
		if err := s.connectors.AcceptInbound(ctx, *event); err != nil {
			return nil, connectorConnectError(err)
		}
	}
	if err := stream.Err(); err != nil {
		return nil, connect.NewError(connect.CodeUnavailable, err)
	}
	return connect.NewResponse(&connector.StreamInboundResponse{}), nil
}

func (s *ConnectorService) StreamOutbound(ctx context.Context, req *connect.Request[connector.StreamOutboundRequest], stream *connect.ServerStream[connector.OutboundCommand]) error {
	if s.connectors == nil {
		return connect.NewError(connect.CodeInternal, errors.New("connector service not configured"))
	}
	token := req.Msg.Token
	if err := s.connectors.ValidateLease(ctx, token); err != nil {
		return connectorConnectError(err)
	}
	if s.outbound == nil {
		return s.waitUntilOutboundLeaseStale(ctx, token)
	}
	err := s.outbound.StreamOutbound(ctx, token, func(sendCtx context.Context, command connector.OutboundCommand) error {
		if err := s.connectors.ValidateLease(sendCtx, token); err != nil {
			return err
		}
		command.Token = token
		return stream.Send(&command)
	})
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, io.EOF) {
			return nil
		}
		return connectorConnectError(err)
	}
	return nil
}

func (s *ConnectorService) waitUntilOutboundLeaseStale(ctx context.Context, token connector.LeaseToken) error {
	interval := s.staleCheckInterval
	if interval <= 0 {
		interval = connector.DefaultRenewInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := s.connectors.ValidateLease(ctx, token); err != nil {
				return connectorConnectError(err)
			}
		}
	}
}

func connectorConnectError(err error) error {
	switch {
	case errors.Is(err, connector.ErrInvalidLeaseToken):
		return connect.NewError(connect.CodeInvalidArgument, err)
	case errors.Is(err, connector.ErrLeaseHeld):
		return connect.NewError(connect.CodeAlreadyExists, err)
	case connector.IsLeaseFenceError(err):
		return connect.NewError(connect.CodeFailedPrecondition, err)
	default:
		return connect.NewError(connect.CodeInternal, err)
	}
}
