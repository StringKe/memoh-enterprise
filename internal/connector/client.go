package connector

import "context"

type ControlClient interface {
	Register(ctx context.Context, req RegisterRequest) (RegisterResponse, error)
	AcquireLease(ctx context.Context, req AcquireLeaseRequest) (AcquireLeaseResponse, error)
	RenewLease(ctx context.Context, req RenewLeaseRequest) (RenewLeaseResponse, error)
	ReleaseLease(ctx context.Context, req ReleaseLeaseRequest) (ReleaseLeaseResponse, error)
	ReportStatus(ctx context.Context, req ReportStatusRequest) (ReportStatusResponse, error)
	SendInbound(ctx context.Context, event InboundEvent) error
	StreamOutbound(ctx context.Context, req StreamOutboundRequest, handle func(context.Context, OutboundCommand) error) error
}

type Client struct {
	control ControlClient
}

func NewClient(control ControlClient) *Client {
	return &Client{control: control}
}

func (c *Client) Register(ctx context.Context, req RegisterRequest) (Instance, error) {
	resp, err := c.control.Register(ctx, req)
	if err != nil {
		return Instance{}, err
	}
	return resp.Instance, nil
}

func (c *Client) AcquireLease(ctx context.Context, req AcquireLeaseRequest) (LeaseToken, error) {
	resp, err := c.control.AcquireLease(ctx, req)
	if err != nil {
		return LeaseToken{}, err
	}
	return resp.Lease, nil
}

func (c *Client) RenewLease(ctx context.Context, token LeaseToken) (LeaseToken, error) {
	resp, err := c.control.RenewLease(ctx, RenewLeaseRequest{Token: token})
	if err != nil {
		return LeaseToken{}, err
	}
	return resp.Lease, nil
}

func (c *Client) ReleaseLease(ctx context.Context, token LeaseToken) error {
	_, err := c.control.ReleaseLease(ctx, ReleaseLeaseRequest{Token: token})
	return err
}

func (c *Client) ReportStatus(ctx context.Context, status Status) error {
	_, err := c.control.ReportStatus(ctx, ReportStatusRequest{Status: status})
	return err
}

func (c *Client) SendInbound(ctx context.Context, event InboundEvent) error {
	return c.control.SendInbound(ctx, event)
}

func (c *Client) StreamOutbound(ctx context.Context, token LeaseToken, handle func(context.Context, OutboundCommand) error) error {
	return c.control.StreamOutbound(ctx, StreamOutboundRequest{Token: token}, handle)
}
