package runner

import (
	"context"
	"errors"
	"math"

	"connectrpc.com/connect"

	runnerv1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/runner/v1"
	"github.com/memohai/memoh/internal/connectapi/gen/memoh/runner/v1/runnerv1connect"
	displaypkg "github.com/memohai/memoh/internal/display"
)

// DisplayClient is the agent-runner side of the workspace display proxy. It
// implements internal/agent/tools.BrowserDisplay by forwarding every call to
// the server through RunnerSupportService, so the agent runner does not need
// host-side access to the Xvnc Unix socket.
type DisplayClient struct {
	client runnerv1connect.RunnerSupportServiceClient
	lease  RunLease
}

// NewDisplayClient creates a DisplayClient bound to a RunLease so the server
// can authorise each call by re-validating the lease.
func NewDisplayClient(client runnerv1connect.RunnerSupportServiceClient, lease RunLease) *DisplayClient {
	return &DisplayClient{client: client, lease: lease}
}

// IsEnabled reports whether the workspace display is enabled for the bot.
func (c *DisplayClient) IsEnabled(ctx context.Context, botID string) bool {
	if c == nil || c.client == nil {
		return false
	}
	resp, err := c.client.IsBotDisplayEnabled(ctx, connect.NewRequest(&runnerv1.IsBotDisplayEnabledRequest{
		Ref:   c.lease.Ref().Proto(),
		BotId: botID,
	}))
	if err != nil {
		return false
	}
	return resp.Msg.GetEnabled()
}

// Screenshot captures a single JPEG snapshot of the workspace display through
// the server.
func (c *DisplayClient) Screenshot(ctx context.Context, botID string) ([]byte, string, error) {
	if c == nil || c.client == nil {
		return nil, "", ErrSupportClientMissing
	}
	resp, err := c.client.CaptureBotDisplayScreenshot(ctx, connect.NewRequest(&runnerv1.CaptureBotDisplayScreenshotRequest{
		Ref:   c.lease.Ref().Proto(),
		BotId: botID,
	}))
	if err != nil {
		return nil, "", err
	}
	return resp.Msg.GetImage(), resp.Msg.GetMimeType(), nil
}

// ControlInputs forwards pointer/key events to the workspace display through
// the server.
func (c *DisplayClient) ControlInputs(ctx context.Context, botID string, events []displaypkg.ControlInput) error {
	if c == nil || c.client == nil {
		return ErrSupportClientMissing
	}
	if len(events) == 0 {
		return nil
	}
	wire := make([]*runnerv1.DisplayInputEvent, 0, len(events))
	for _, e := range events {
		wire = append(wire, &runnerv1.DisplayInputEvent{
			Type:       e.Type,
			X:          clampInt32(e.X),
			Y:          clampInt32(e.Y),
			ButtonMask: uint32(e.ButtonMask),
			Keysym:     e.Keysym,
			Down:       e.Down,
		})
	}
	if _, err := c.client.SendBotDisplayInputs(ctx, connect.NewRequest(&runnerv1.SendBotDisplayInputsRequest{
		Ref:    c.lease.Ref().Proto(),
		BotId:  botID,
		Events: wire,
	})); err != nil {
		return err
	}
	return nil
}

// ensure DisplayClient satisfies the contract used by browser tools without
// importing internal/agent/tools (which would cycle).
var _ interface {
	IsEnabled(ctx context.Context, botID string) bool
	Screenshot(ctx context.Context, botID string) ([]byte, string, error)
	ControlInputs(ctx context.Context, botID string, events []displaypkg.ControlInput) error
} = (*DisplayClient)(nil)

// clampInt32 truncates a host-side coordinate (int) into the proto wire's
// int32 range to satisfy the gosec G115 lint without losing display
// resolutions ever observed in practice.
func clampInt32(v int) int32 {
	switch {
	case v > math.MaxInt32:
		return math.MaxInt32
	case v < math.MinInt32:
		return math.MinInt32
	default:
		return int32(v)
	}
}

var _ = errors.New
