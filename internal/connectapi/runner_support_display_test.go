package connectapi

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/memohai/memoh/internal/serviceauth"
)

type fakeDisplaySupport struct {
	enabled       bool
	enabledCalls  int
	enabledBotID  string
	screenshotErr error
	screenshot    []byte
	screenshotMIM string
	controlErr    error
	controlEvents []DisplayInputEvent
	controlBotID  string
}

func (f *fakeDisplaySupport) IsEnabled(_ context.Context, botID string) bool {
	f.enabledCalls++
	f.enabledBotID = botID
	return f.enabled
}

func (f *fakeDisplaySupport) Screenshot(_ context.Context, botID string) ([]byte, string, error) {
	f.enabledBotID = botID
	if f.screenshotErr != nil {
		return nil, "", f.screenshotErr
	}
	return f.screenshot, f.screenshotMIM, nil
}

func (f *fakeDisplaySupport) ControlInputs(_ context.Context, botID string, events []DisplayInputEvent) error {
	f.controlBotID = botID
	f.controlEvents = events
	return f.controlErr
}

func TestRunnerSupportIsBotDisplayEnabledRejectsWrongLease(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 6, 10, 0, 0, 0, time.UTC)
	lease := workspaceTestLease(now)
	service := NewRunnerSupportService(fakeRunLeaseResolver{lease: lease}, nil)
	service.SetDisplaySupport(&fakeDisplaySupport{enabled: true})
	wrong := refFromLease(lease)
	wrong.SessionID = "other"
	if _, err := service.IsBotDisplayEnabled(context.Background(), IsBotDisplayEnabledRequest{Lease: wrong, BotID: "bot-1"}); !errors.Is(err, serviceauth.ErrPermissionDenied) {
		t.Fatalf("expected permission denied, got %v", err)
	}
}

func TestRunnerSupportIsBotDisplayEnabledForwardsToBackend(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 6, 10, 0, 0, 0, time.UTC)
	lease := workspaceTestLease(now)
	display := &fakeDisplaySupport{enabled: true}
	service := NewRunnerSupportService(fakeRunLeaseResolver{lease: lease}, nil)
	service.SetDisplaySupport(display)
	enabled, err := service.IsBotDisplayEnabled(context.Background(), IsBotDisplayEnabledRequest{Lease: refFromLease(lease), BotID: "bot-1"})
	if err != nil {
		t.Fatalf("IsBotDisplayEnabled: %v", err)
	}
	if !enabled {
		t.Fatal("expected enabled=true")
	}
	if display.enabledCalls != 1 {
		t.Fatalf("backend calls = %d, want 1", display.enabledCalls)
	}
	if display.enabledBotID != "bot-1" {
		t.Fatalf("bot id = %q", display.enabledBotID)
	}
}

func TestRunnerSupportIsBotDisplayEnabledReturnsFalseWithoutSupport(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 6, 10, 0, 0, 0, time.UTC)
	lease := workspaceTestLease(now)
	service := NewRunnerSupportService(fakeRunLeaseResolver{lease: lease}, nil)
	enabled, err := service.IsBotDisplayEnabled(context.Background(), IsBotDisplayEnabledRequest{Lease: refFromLease(lease), BotID: "bot-1"})
	if err != nil {
		t.Fatalf("IsBotDisplayEnabled without support: %v", err)
	}
	if enabled {
		t.Fatal("expected enabled=false when display support is missing")
	}
}

func TestRunnerSupportCaptureBotDisplayScreenshotRequiresSupport(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 6, 10, 0, 0, 0, time.UTC)
	lease := workspaceTestLease(now)
	service := NewRunnerSupportService(fakeRunLeaseResolver{lease: lease}, nil)
	if _, err := service.CaptureBotDisplayScreenshot(context.Background(), CaptureBotDisplayScreenshotRequest{Lease: refFromLease(lease), BotID: "bot-1"}); !errors.Is(err, ErrRunnerSupportDependencyMissing) {
		t.Fatalf("expected dependency missing, got %v", err)
	}
}

func TestRunnerSupportCaptureBotDisplayScreenshotForwards(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 6, 10, 0, 0, 0, time.UTC)
	lease := workspaceTestLease(now)
	display := &fakeDisplaySupport{screenshot: []byte{1, 2, 3}, screenshotMIM: "image/jpeg"}
	service := NewRunnerSupportService(fakeRunLeaseResolver{lease: lease}, nil)
	service.SetDisplaySupport(display)
	resp, err := service.CaptureBotDisplayScreenshot(context.Background(), CaptureBotDisplayScreenshotRequest{Lease: refFromLease(lease), BotID: "bot-7"})
	if err != nil {
		t.Fatalf("Screenshot: %v", err)
	}
	if string(resp.Image) != "\x01\x02\x03" {
		t.Fatalf("image = %x", resp.Image)
	}
	if resp.MimeType != "image/jpeg" {
		t.Fatalf("mime = %q", resp.MimeType)
	}
	if display.enabledBotID != "bot-7" {
		t.Fatalf("bot id passed to backend = %q", display.enabledBotID)
	}
}

func TestRunnerSupportCaptureBotDisplayScreenshotPropagatesError(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 6, 10, 0, 0, 0, time.UTC)
	lease := workspaceTestLease(now)
	want := errors.New("display unavailable")
	service := NewRunnerSupportService(fakeRunLeaseResolver{lease: lease}, nil)
	service.SetDisplaySupport(&fakeDisplaySupport{screenshotErr: want})
	if _, err := service.CaptureBotDisplayScreenshot(context.Background(), CaptureBotDisplayScreenshotRequest{Lease: refFromLease(lease), BotID: "bot-1"}); !errors.Is(err, want) {
		t.Fatalf("expected wrapped error, got %v", err)
	}
}

func TestRunnerSupportSendBotDisplayInputsForwards(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 6, 10, 0, 0, 0, time.UTC)
	lease := workspaceTestLease(now)
	display := &fakeDisplaySupport{}
	service := NewRunnerSupportService(fakeRunLeaseResolver{lease: lease}, nil)
	service.SetDisplaySupport(display)
	events := []DisplayInputEvent{
		{Type: "pointer", X: 10, Y: 20, ButtonMask: 1},
		{Type: "key", Keysym: 0xff0d, Down: true},
	}
	if err := service.SendBotDisplayInputs(context.Background(), SendBotDisplayInputsRequest{Lease: refFromLease(lease), BotID: "bot-9", Events: events}); err != nil {
		t.Fatalf("SendBotDisplayInputs: %v", err)
	}
	if display.controlBotID != "bot-9" {
		t.Fatalf("bot id = %q", display.controlBotID)
	}
	if len(display.controlEvents) != 2 {
		t.Fatalf("events = %d", len(display.controlEvents))
	}
	if display.controlEvents[0].X != 10 || display.controlEvents[0].Y != 20 || display.controlEvents[0].ButtonMask != 1 {
		t.Fatalf("pointer event = %+v", display.controlEvents[0])
	}
	if display.controlEvents[1].Keysym != 0xff0d || !display.controlEvents[1].Down {
		t.Fatalf("key event = %+v", display.controlEvents[1])
	}
}

func TestRunnerSupportSendBotDisplayInputsRejectsWrongLease(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 6, 10, 0, 0, 0, time.UTC)
	lease := workspaceTestLease(now)
	service := NewRunnerSupportService(fakeRunLeaseResolver{lease: lease}, nil)
	service.SetDisplaySupport(&fakeDisplaySupport{})
	wrong := refFromLease(lease)
	wrong.SessionID = "other-session"
	if err := service.SendBotDisplayInputs(context.Background(), SendBotDisplayInputsRequest{Lease: wrong, BotID: "bot-1"}); !errors.Is(err, serviceauth.ErrPermissionDenied) {
		t.Fatalf("expected permission denied, got %v", err)
	}
}

func TestRunnerSupportSendBotDisplayInputsRequiresSupport(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 6, 10, 0, 0, 0, time.UTC)
	lease := workspaceTestLease(now)
	service := NewRunnerSupportService(fakeRunLeaseResolver{lease: lease}, nil)
	if err := service.SendBotDisplayInputs(context.Background(), SendBotDisplayInputsRequest{Lease: refFromLease(lease), BotID: "bot-1"}); !errors.Is(err, ErrRunnerSupportDependencyMissing) {
		t.Fatalf("expected dependency missing, got %v", err)
	}
}
