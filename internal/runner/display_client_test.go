package runner

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"

	runnerv1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/runner/v1"
	"github.com/memohai/memoh/internal/connectapi/gen/memoh/runner/v1/runnerv1connect"
	displaypkg "github.com/memohai/memoh/internal/display"
)

type fakeDisplayBackend struct {
	runnerv1connect.UnimplementedRunnerSupportServiceHandler
	enabled        bool
	enabledCalls   int
	gotEnabledID   string
	screenshot     []byte
	mime           string
	screenshotErr  error
	gotShotID      string
	gotInputBotID  string
	gotInputEvents []*runnerv1.DisplayInputEvent
	inputErr       error
}

func (s *fakeDisplayBackend) IsBotDisplayEnabled(_ context.Context, req *connect.Request[runnerv1.IsBotDisplayEnabledRequest]) (*connect.Response[runnerv1.IsBotDisplayEnabledResponse], error) {
	s.enabledCalls++
	s.gotEnabledID = req.Msg.GetBotId()
	return connect.NewResponse(&runnerv1.IsBotDisplayEnabledResponse{Enabled: s.enabled}), nil
}

func (s *fakeDisplayBackend) CaptureBotDisplayScreenshot(_ context.Context, req *connect.Request[runnerv1.CaptureBotDisplayScreenshotRequest]) (*connect.Response[runnerv1.CaptureBotDisplayScreenshotResponse], error) {
	s.gotShotID = req.Msg.GetBotId()
	if s.screenshotErr != nil {
		return nil, connect.NewError(connect.CodeInternal, s.screenshotErr)
	}
	return connect.NewResponse(&runnerv1.CaptureBotDisplayScreenshotResponse{Image: s.screenshot, MimeType: s.mime}), nil
}

func (s *fakeDisplayBackend) SendBotDisplayInputs(_ context.Context, req *connect.Request[runnerv1.SendBotDisplayInputsRequest]) (*connect.Response[runnerv1.SendBotDisplayInputsResponse], error) {
	s.gotInputBotID = req.Msg.GetBotId()
	s.gotInputEvents = req.Msg.GetEvents()
	if s.inputErr != nil {
		return nil, connect.NewError(connect.CodeInternal, s.inputErr)
	}
	return connect.NewResponse(&runnerv1.SendBotDisplayInputsResponse{}), nil
}

func newDisplayBackendClient(t *testing.T, backend *fakeDisplayBackend) runnerv1connect.RunnerSupportServiceClient {
	t.Helper()
	mux := http.NewServeMux()
	path, handler := runnerv1connect.NewRunnerSupportServiceHandler(backend)
	mux.Handle(path, handler)
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return runnerv1connect.NewRunnerSupportServiceClient(server.Client(), server.URL)
}

func displayTestLease() RunLease {
	return RunLease{
		RunID:            "run-display-1",
		RunnerInstanceID: "runner-1",
		BotID:            "bot-display-1",
		SessionID:        "session-1",
		WorkspaceID:      "workspace-1",
		ExpiresAt:        time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC),
		LeaseVersion:     1,
	}
}

func TestDisplayClientIsEnabledForwardsBotID(t *testing.T) {
	t.Parallel()
	backend := &fakeDisplayBackend{enabled: true}
	client := NewDisplayClient(newDisplayBackendClient(t, backend), displayTestLease())
	if !client.IsEnabled(context.Background(), "bot-display-1") {
		t.Fatal("expected enabled=true")
	}
	if backend.enabledCalls != 1 {
		t.Fatalf("backend calls = %d", backend.enabledCalls)
	}
	if backend.gotEnabledID != "bot-display-1" {
		t.Fatalf("forwarded bot id = %q", backend.gotEnabledID)
	}
}

func TestDisplayClientIsEnabledFalseOnError(t *testing.T) {
	t.Parallel()
	// nil client -> can't call; expect false rather than panic.
	var c *DisplayClient
	if c.IsEnabled(context.Background(), "bot-1") {
		t.Fatal("nil client should report disabled")
	}
}

func TestDisplayClientScreenshotReturnsBytes(t *testing.T) {
	t.Parallel()
	backend := &fakeDisplayBackend{screenshot: []byte("png-bytes"), mime: "image/jpeg"}
	client := NewDisplayClient(newDisplayBackendClient(t, backend), displayTestLease())
	img, mime, err := client.Screenshot(context.Background(), "bot-99")
	if err != nil {
		t.Fatalf("Screenshot: %v", err)
	}
	if string(img) != "png-bytes" {
		t.Fatalf("image = %q", string(img))
	}
	if mime != "image/jpeg" {
		t.Fatalf("mime = %q", mime)
	}
	if backend.gotShotID != "bot-99" {
		t.Fatalf("forwarded bot id = %q", backend.gotShotID)
	}
}

func TestDisplayClientScreenshotPropagatesError(t *testing.T) {
	t.Parallel()
	backend := &fakeDisplayBackend{screenshotErr: errors.New("display offline")}
	client := NewDisplayClient(newDisplayBackendClient(t, backend), displayTestLease())
	if _, _, err := client.Screenshot(context.Background(), "bot-1"); err == nil {
		t.Fatal("expected error")
	}
}

func TestDisplayClientScreenshotMissingClient(t *testing.T) {
	t.Parallel()
	c := NewDisplayClient(nil, displayTestLease())
	if _, _, err := c.Screenshot(context.Background(), "bot-1"); !errors.Is(err, ErrSupportClientMissing) {
		t.Fatalf("expected ErrSupportClientMissing, got %v", err)
	}
}

func TestDisplayClientControlInputsSendsEvents(t *testing.T) {
	t.Parallel()
	backend := &fakeDisplayBackend{}
	client := NewDisplayClient(newDisplayBackendClient(t, backend), displayTestLease())
	events := []displaypkg.ControlInput{
		{Type: "pointer", X: 100, Y: 200, ButtonMask: 1},
		{Type: "key", Keysym: 0xff0d, Down: true},
	}
	if err := client.ControlInputs(context.Background(), "bot-7", events); err != nil {
		t.Fatalf("ControlInputs: %v", err)
	}
	if backend.gotInputBotID != "bot-7" {
		t.Fatalf("bot id = %q", backend.gotInputBotID)
	}
	if len(backend.gotInputEvents) != 2 {
		t.Fatalf("events = %d", len(backend.gotInputEvents))
	}
	if backend.gotInputEvents[0].GetType() != "pointer" || backend.gotInputEvents[0].GetX() != 100 || backend.gotInputEvents[0].GetButtonMask() != 1 {
		t.Fatalf("pointer event = %+v", backend.gotInputEvents[0])
	}
	if backend.gotInputEvents[1].GetKeysym() != 0xff0d || !backend.gotInputEvents[1].GetDown() {
		t.Fatalf("key event = %+v", backend.gotInputEvents[1])
	}
}

func TestDisplayClientControlInputsEmptyIsNoop(t *testing.T) {
	t.Parallel()
	backend := &fakeDisplayBackend{}
	client := NewDisplayClient(newDisplayBackendClient(t, backend), displayTestLease())
	if err := client.ControlInputs(context.Background(), "bot-1", nil); err != nil {
		t.Fatalf("ControlInputs nil: %v", err)
	}
	if backend.gotInputEvents != nil {
		t.Fatalf("expected backend not called, got %d events", len(backend.gotInputEvents))
	}
}

func TestDisplayClientControlInputsMissingClient(t *testing.T) {
	t.Parallel()
	c := NewDisplayClient(nil, displayTestLease())
	if err := c.ControlInputs(context.Background(), "bot-1", []displaypkg.ControlInput{{Type: "pointer"}}); !errors.Is(err, ErrSupportClientMissing) {
		t.Fatalf("expected ErrSupportClientMissing, got %v", err)
	}
}

func TestClampInt32Bounds(t *testing.T) {
	t.Parallel()
	if got := clampInt32(0); got != 0 {
		t.Fatalf("0 -> %d", got)
	}
	if got := clampInt32(1<<31 - 1); got != 1<<31-1 {
		t.Fatalf("MaxInt32 -> %d", got)
	}
	if got := clampInt32(1 << 40); got != 1<<31-1 {
		t.Fatalf("overflow positive -> %d", got)
	}
	if got := clampInt32(-(1 << 40)); got != -(1 << 31) {
		t.Fatalf("overflow negative -> %d", got)
	}
}
