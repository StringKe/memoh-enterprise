package tools

import (
	"context"
	"testing"

	displaypkg "github.com/memohai/memoh/internal/display"
)

func TestBrowserKeyChordHelpers(t *testing.T) {
	parts := splitKeyChord("Control+Shift+a")
	if len(parts) != 3 || parts[0] != "Control" || parts[1] != "Shift" || parts[2] != "a" {
		t.Fatalf("unexpected chord parts: %#v", parts)
	}
	if got := namedKeysym("Enter"); got != 0xff0d {
		t.Fatalf("unexpected Enter keysym: %#x", got)
	}
	if got := namedKeysym("Control"); got != 0xffe3 {
		t.Fatalf("unexpected Control keysym: %#x", got)
	}
	if got := keysymForRune('你'); got != 0x01000000|uint32('你') {
		t.Fatalf("unexpected unicode keysym: %#x", got)
	}
}

func TestBrowserCDPKeyInfo(t *testing.T) {
	enter := keyInfoForCDP("Enter")
	if enter.Key != "Enter" || enter.KeyCode != 13 {
		t.Fatalf("unexpected Enter key info: %#v", enter)
	}
	letter := keyInfoForCDP("a")
	if letter.Key != "a" || letter.Code != "KeyA" || letter.KeyCode != int('A') || letter.Text != "a" {
		t.Fatalf("unexpected letter key info: %#v", letter)
	}
	if got := cdpModifier("Control") | cdpModifier("Shift"); got != 10 {
		t.Fatalf("unexpected modifier mask: %d", got)
	}
}

func TestBrowserScrollDeltas(t *testing.T) {
	if got := scrollDeltaY("down", 500); got != 500 {
		t.Fatalf("unexpected down delta: %d", got)
	}
	if got := scrollDeltaY("up", 500); got != -500 {
		t.Fatalf("unexpected up delta: %d", got)
	}
	if got := scrollDeltaX("left", 300); got != -300 {
		t.Fatalf("unexpected left delta: %d", got)
	}
	if got := scrollDeltaX("right", 300); got != 300 {
		t.Fatalf("unexpected right delta: %d", got)
	}
}

func TestBrowserActionAliases(t *testing.T) {
	if got := normalizeBrowserAction("dblclick"); got != "double_click" {
		t.Fatalf("unexpected dblclick alias: %q", got)
	}
	if got := normalizeBrowserAction("scrollintoview"); got != "scroll_into_view" {
		t.Fatalf("unexpected scrollintoview alias: %q", got)
	}
	if got := normalizeBrowserAction("fill"); got != "fill" {
		t.Fatalf("unexpected canonical action: %q", got)
	}
}

func TestBrowserRefHelpers(t *testing.T) {
	for _, input := range []string{"12", "e12", "E12", "ref=e12"} {
		if got := normalizeBrowserRef(input); got != "e12" {
			t.Fatalf("normalizeBrowserRef(%q) = %q", input, got)
		}
	}
	if _, err := browserRefIndex("e0"); err == nil {
		t.Fatal("expected invalid zero ref")
	}
	target := browserTargetArg(map[string]any{"ref": "12", "selector": "#fallback"}, "selector", "ref")
	if target.Ref != "e12" || target.Selector != "#fallback" {
		t.Fatalf("unexpected target: %#v", target)
	}
	result := target.withResult(map[string]any{"ok": true})
	if result["ref"] != "e12" || result["selector"] != "#fallback" {
		t.Fatalf("target metadata missing from result: %#v", result)
	}
}

func TestBrowserSchemasAreStrict(t *testing.T) {
	schema := browserObjectSchema(map[string]any{"action": map[string]any{"type": "string"}}, []string{"action"})
	if schema["additionalProperties"] != false {
		t.Fatalf("expected strict browser schema, got %#v", schema["additionalProperties"])
	}
	if required, ok := schema["required"].([]string); !ok || len(required) != 1 || required[0] != "action" {
		t.Fatalf("unexpected required fields: %#v", schema["required"])
	}
}

type fakeBrowserDisplay struct {
	enabled       bool
	enabledCalls  int
	enabledBotID  string
	screenshotErr error
}

func (f *fakeBrowserDisplay) IsEnabled(_ context.Context, botID string) bool {
	f.enabledCalls++
	f.enabledBotID = botID
	return f.enabled
}

func (f *fakeBrowserDisplay) Screenshot(context.Context, string) ([]byte, string, error) {
	if f.screenshotErr != nil {
		return nil, "", f.screenshotErr
	}
	return nil, "", nil
}

func (*fakeBrowserDisplay) ControlInputs(context.Context, string, []displaypkg.ControlInput) error {
	return nil
}

func TestBrowserProviderToolsRequiresDisplay(t *testing.T) {
	p := NewBrowserProvider(nil, nil, nil, "")
	tools, err := p.Tools(context.Background(), SessionContext{BotID: "bot-1"})
	if err != nil {
		t.Fatalf("Tools err = %v", err)
	}
	if tools != nil {
		t.Fatalf("expected nil tools without display, got %d", len(tools))
	}
}

func TestBrowserProviderToolsSkipsSubagent(t *testing.T) {
	display := &fakeBrowserDisplay{enabled: true}
	p := NewBrowserProvider(nil, nil, display, "")
	tools, err := p.Tools(context.Background(), SessionContext{BotID: "bot-1", IsSubagent: true})
	if err != nil {
		t.Fatalf("Tools err = %v", err)
	}
	if tools != nil {
		t.Fatalf("expected subagent skip, got %d tools", len(tools))
	}
	if display.enabledCalls != 0 {
		t.Fatalf("display.IsEnabled should not be called for subagent, got %d", display.enabledCalls)
	}
}

func TestBrowserProviderToolsRequiresBotID(t *testing.T) {
	display := &fakeBrowserDisplay{enabled: true}
	p := NewBrowserProvider(nil, nil, display, "")
	tools, err := p.Tools(context.Background(), SessionContext{})
	if err != nil || tools != nil {
		t.Fatalf("expected nil tools/err, got tools=%d err=%v", len(tools), err)
	}
	if display.enabledCalls != 0 {
		t.Fatalf("display.IsEnabled should not be called without bot id")
	}
}

func TestBrowserProviderToolsGatedByIsEnabled(t *testing.T) {
	display := &fakeBrowserDisplay{enabled: false}
	p := NewBrowserProvider(nil, nil, display, "")
	tools, err := p.Tools(context.Background(), SessionContext{BotID: "bot-1"})
	if err != nil {
		t.Fatalf("Tools err = %v", err)
	}
	if tools != nil {
		t.Fatalf("expected gate to drop tools when disabled")
	}
	if display.enabledCalls != 1 {
		t.Fatalf("display.IsEnabled calls = %d", display.enabledCalls)
	}
	if display.enabledBotID != "bot-1" {
		t.Fatalf("display.IsEnabled bot id = %q", display.enabledBotID)
	}
}

func TestBrowserProviderToolsExposesAllToolsWhenEnabled(t *testing.T) {
	display := &fakeBrowserDisplay{enabled: true}
	p := NewBrowserProvider(nil, nil, display, "")
	tools, err := p.Tools(context.Background(), SessionContext{BotID: "bot-1"})
	if err != nil {
		t.Fatalf("Tools err = %v", err)
	}
	wantNames := map[string]bool{
		"browser_action":         false,
		"browser_observe":        false,
		"browser_remote_session": false,
		"computer_use":           false,
	}
	for _, tool := range tools {
		if _, ok := wantNames[tool.Name]; ok {
			wantNames[tool.Name] = true
		}
	}
	for name, found := range wantNames {
		if !found {
			t.Errorf("expected tool %q to be exposed when display enabled", name)
		}
	}
}

func TestBrowserProviderEnsureDisplayEnabledRejectsNilDisplay(t *testing.T) {
	p := NewBrowserProvider(nil, nil, nil, "")
	if err := p.ensureDisplayEnabled(context.Background(), "bot-1"); err == nil {
		t.Fatal("expected error when display is nil")
	}
}

func TestBrowserProviderEnsureDisplayEnabledRejectsDisabled(t *testing.T) {
	display := &fakeBrowserDisplay{enabled: false}
	p := NewBrowserProvider(nil, nil, display, "")
	if err := p.ensureDisplayEnabled(context.Background(), "bot-1"); err == nil {
		t.Fatal("expected error when display disabled")
	}
}

func TestBrowserProviderEnsureDisplayEnabledOK(t *testing.T) {
	display := &fakeBrowserDisplay{enabled: true}
	p := NewBrowserProvider(nil, nil, display, "")
	if err := p.ensureDisplayEnabled(context.Background(), "bot-1"); err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
}
