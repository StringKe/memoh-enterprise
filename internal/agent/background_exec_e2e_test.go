package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	sdk "github.com/memohai/twilight-ai/sdk"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"github.com/memohai/memoh/internal/agent/background"
	agenttools "github.com/memohai/memoh/internal/agent/tools"
	workspacev1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/workspace/v1"
	"github.com/memohai/memoh/internal/connectapi/gen/memoh/workspace/v1/workspacev1connect"
	"github.com/memohai/memoh/internal/workspace/executorclient"
)

// ---------------------------------------------------------------------------
// Mock container service with controllable Exec behavior
// ---------------------------------------------------------------------------

type execBehavior struct {
	stdout   string
	stderr   string
	exitCode int32
	delay    time.Duration // how long before sending output
}

type mockExecContainerService struct {
	workspacev1connect.UnimplementedWorkspaceExecutorServiceHandler

	mu        sync.Mutex
	behaviors map[string]execBehavior // command prefix -> behavior
	written   map[string][]byte       // path -> content (WriteFile)
}

func newMockExecContainerService() *mockExecContainerService {
	return &mockExecContainerService{
		behaviors: make(map[string]execBehavior),
		written:   make(map[string][]byte),
	}
}

func (s *mockExecContainerService) setBehavior(cmdPrefix string, b execBehavior) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.behaviors[cmdPrefix] = b
}

func (s *mockExecContainerService) findBehavior(cmd string) (execBehavior, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for prefix, b := range s.behaviors {
		if strings.Contains(cmd, prefix) {
			return b, true
		}
	}
	return execBehavior{}, false
}

func (s *mockExecContainerService) Exec(ctx context.Context, stream *connect.BidiStream[workspacev1.ExecRequest, workspacev1.ExecResponse]) error {
	input, err := stream.Receive()
	if err != nil {
		return err
	}
	start := input.GetStart()
	if start == nil {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("missing exec start"))
	}
	cmd := start.GetCommand()

	b, ok := s.findBehavior(cmd)
	if !ok {
		b = execBehavior{stdout: fmt.Sprintf("[executed] %s\n", cmd), exitCode: 0}
	}

	if b.delay > 0 {
		select {
		case <-time.After(b.delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	if b.stdout != "" {
		if err := stream.Send(&workspacev1.ExecResponse{
			Kind: workspacev1.ExecResponse_KIND_STDOUT,
			Data: []byte(b.stdout),
		}); err != nil {
			return err
		}
	}
	if b.stderr != "" {
		if err := stream.Send(&workspacev1.ExecResponse{
			Kind: workspacev1.ExecResponse_KIND_STDERR,
			Data: []byte(b.stderr),
		}); err != nil {
			return err
		}
	}
	return stream.Send(&workspacev1.ExecResponse{
		Kind:     workspacev1.ExecResponse_KIND_EXIT,
		ExitCode: b.exitCode,
	})
}

func (s *mockExecContainerService) WriteFile(_ context.Context, req *connect.Request[workspacev1.WriteFileRequest]) (*connect.Response[workspacev1.WriteFileResponse], error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.written[req.Msg.GetPath()] = req.Msg.GetContent()
	return connect.NewResponse(&workspacev1.WriteFileResponse{BytesWritten: int64(len(req.Msg.GetContent()))}), nil
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func setupExecTestInfra(t *testing.T, svc *mockExecContainerService) (executorclient.Provider, func()) {
	t.Helper()

	mux := http.NewServeMux()
	path, handler := workspacev1connect.NewWorkspaceExecutorServiceHandler(svc)
	mux.Handle(path, handler)
	server := httptest.NewServer(h2c.NewHandler(mux, &http2.Server{}))

	client, err := executorclient.Dial(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("executorclient.Dial: %v", err)
	}
	cleanup := func() {
		_ = client.Close()
		server.Close()
	}

	bp := &agentReadMediaWorkspaceExecutorProvider{client: client}
	return bp, cleanup
}

// ---------------------------------------------------------------------------
// E2E Test: Explicit background exec
// ---------------------------------------------------------------------------

func TestE2E_ExplicitBackgroundExec(t *testing.T) {
	t.Parallel()

	svc := newMockExecContainerService()
	svc.setBehavior("npm install", execBehavior{
		stdout:   "added 42 packages\n",
		exitCode: 0,
		delay:    100 * time.Millisecond, // simulate some work
	})

	bp, cleanup := setupExecTestInfra(t, svc)
	defer cleanup()

	bgMgr := background.New(nil)

	// Model calls exec with run_in_background, then on step 2 sees notification.
	var step2Params sdk.GenerateParams
	modelProvider := &agentReadMediaMockProvider{
		handler: func(call int, params sdk.GenerateParams) (*sdk.GenerateResult, error) {
			switch call {
			case 1:
				// Model decides to run npm install in background.
				return &sdk.GenerateResult{
					FinishReason: sdk.FinishReasonToolCalls,
					ToolCalls: []sdk.ToolCall{{
						ToolCallID: "call-1",
						ToolName:   "exec",
						Input: map[string]any{
							"command":           "npm install",
							"run_in_background": true,
							"description":       "Install dependencies",
						},
					}},
				}, nil
			case 2:
				// Model sees tool result with background_started.
				// It should do something else or reply.
				// Simulate waiting a bit so the background task has time to complete.
				time.Sleep(300 * time.Millisecond)
				return &sdk.GenerateResult{
					FinishReason: sdk.FinishReasonToolCalls,
					ToolCalls: []sdk.ToolCall{{
						ToolCallID: "call-2",
						ToolName:   "exec",
						Input: map[string]any{
							"command": "echo hello",
						},
					}},
				}, nil
			case 3:
				// Step 3: model should see the background notification injected.
				step2Params = params
				return &sdk.GenerateResult{
					Text:         "All done!",
					FinishReason: sdk.FinishReasonStop,
				}, nil
			default:
				return &sdk.GenerateResult{
					Text:         "unexpected",
					FinishReason: sdk.FinishReasonStop,
				}, nil
			}
		},
	}

	a := New(Deps{})
	a.SetToolProviders([]agenttools.ToolProvider{
		agenttools.NewContainerProvider(nil, bp, bgMgr, "/data"),
	})

	result, err := a.Generate(context.Background(), RunConfig{
		Model:             &sdk.Model{ID: "mock", Provider: modelProvider},
		Messages:          []sdk.Message{sdk.UserMessage("install deps and say hi")},
		System:            "You are a helpful bot.",
		SupportsToolCall:  true,
		Identity:          SessionContext{BotID: "bot-test-1", SessionID: "sess-1"},
		BackgroundManager: bgMgr,
	})
	if err != nil {
		t.Fatalf("Generate error: %v", err)
	}
	if result.Text != "All done!" {
		t.Errorf("unexpected text: %q", result.Text)
	}

	// Verify step 2 params contain the background notification.
	found := false
	for _, msg := range step2Params.Messages {
		if msg.Role == sdk.MessageRoleUser {
			for _, part := range msg.Content {
				if tp, ok := part.(sdk.TextPart); ok {
					if strings.Contains(tp.Text, "task-notification") &&
						strings.Contains(tp.Text, "completed") {
						found = true
					}
				}
			}
		}
	}
	if !found {
		t.Error("expected background task notification to be injected into step 3 messages")
	}
}

// ---------------------------------------------------------------------------
// E2E Test: Foreground timeout flips to background
// ---------------------------------------------------------------------------

func TestE2E_ForegroundTimeoutFlip(t *testing.T) {
	t.Parallel()

	svc := newMockExecContainerService()
	// Command takes 3 seconds — longer than our 1-second soft timeout.
	svc.setBehavior("slow-build", execBehavior{
		stdout:   "build completed successfully\n",
		exitCode: 0,
		delay:    3 * time.Second,
	})

	bp, cleanup := setupExecTestInfra(t, svc)
	defer cleanup()

	bgMgr := background.New(nil)

	var toolResult map[string]any
	modelProvider := &agentReadMediaMockProvider{
		handler: func(call int, params sdk.GenerateParams) (*sdk.GenerateResult, error) {
			switch call {
			case 1:
				// Model runs a command with short timeout (will flip).
				return &sdk.GenerateResult{
					FinishReason: sdk.FinishReasonToolCalls,
					ToolCalls: []sdk.ToolCall{{
						ToolCallID: "call-1",
						ToolName:   "exec",
						Input: map[string]any{
							"command":     "slow-build",
							"timeout":     1, // 1 second — will flip
							"description": "Run slow build",
						},
					}},
				}, nil
			case 2:
				// Extract the tool result from step 1.
				toolResult = extractToolResult(t, params, "call-1")
				return &sdk.GenerateResult{
					Text:         "Build moved to background.",
					FinishReason: sdk.FinishReasonStop,
				}, nil
			default:
				return &sdk.GenerateResult{
					Text:         "unexpected",
					FinishReason: sdk.FinishReasonStop,
				}, nil
			}
		},
	}

	a := New(Deps{})
	a.SetToolProviders([]agenttools.ToolProvider{
		agenttools.NewContainerProvider(nil, bp, bgMgr, "/data"),
	})

	result, err := a.Generate(context.Background(), RunConfig{
		Model:             &sdk.Model{ID: "mock", Provider: modelProvider},
		Messages:          []sdk.Message{sdk.UserMessage("run the build")},
		System:            "You are a helpful bot.",
		SupportsToolCall:  true,
		Identity:          SessionContext{BotID: "bot-test-2", SessionID: "sess-2"},
		BackgroundManager: bgMgr,
	})
	if err != nil {
		t.Fatalf("Generate error: %v", err)
	}
	if result.Text != "Build moved to background." {
		t.Errorf("unexpected text: %q", result.Text)
	}

	// The tool result should indicate auto_backgrounded.
	if toolResult == nil {
		t.Fatal("tool result not captured")
	}
	status, _ := toolResult["status"].(string)
	if status != "auto_backgrounded" {
		t.Errorf("expected status auto_backgrounded, got %q", status)
	}
	taskID, _ := toolResult["task_id"].(string)
	if taskID == "" {
		t.Error("expected non-empty task_id")
	}
	msg, _ := toolResult["message"].(string)
	if !strings.Contains(msg, "no work was lost") {
		t.Errorf("expected flip message mentioning no work lost, got %q", msg)
	}

	// Wait for the background task to complete and verify notification.
	deadline := time.After(10 * time.Second)
	for {
		notifications := bgMgr.DrainNotifications("bot-test-2", "sess-2")
		if len(notifications) > 0 {
			n := notifications[0]
			if n.Status != background.TaskCompleted {
				t.Errorf("expected completed, got %s", n.Status)
			}
			if !strings.Contains(n.OutputTail, "build completed") {
				t.Errorf("expected build output in tail, got %q", n.OutputTail)
			}
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for background task notification")
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// ---------------------------------------------------------------------------
// E2E Test: Sleep command rejection
// ---------------------------------------------------------------------------

func TestE2E_SleepRejection(t *testing.T) {
	t.Parallel()

	svc := newMockExecContainerService()
	bp, cleanup := setupExecTestInfra(t, svc)
	defer cleanup()

	bgMgr := background.New(nil)

	var sleepToolResult map[string]any
	var sleepWasError bool
	modelProvider := &agentReadMediaMockProvider{
		handler: func(call int, params sdk.GenerateParams) (*sdk.GenerateResult, error) {
			switch call {
			case 1:
				// Model tries to sleep 10.
				return &sdk.GenerateResult{
					FinishReason: sdk.FinishReasonToolCalls,
					ToolCalls: []sdk.ToolCall{{
						ToolCallID: "call-1",
						ToolName:   "exec",
						Input: map[string]any{
							"command": "sleep 10",
						},
					}},
				}, nil
			case 2:
				// Check the tool result — should be an error.
				sleepToolResult, sleepWasError = extractToolResultWithError(params, "call-1")
				return &sdk.GenerateResult{
					Text:         "Got it, won't sleep.",
					FinishReason: sdk.FinishReasonStop,
				}, nil
			default:
				return &sdk.GenerateResult{
					Text:         "unexpected",
					FinishReason: sdk.FinishReasonStop,
				}, nil
			}
		},
	}

	a := New(Deps{})
	a.SetToolProviders([]agenttools.ToolProvider{
		agenttools.NewContainerProvider(nil, bp, bgMgr, "/data"),
	})

	result, err := a.Generate(context.Background(), RunConfig{
		Model:             &sdk.Model{ID: "mock", Provider: modelProvider},
		Messages:          []sdk.Message{sdk.UserMessage("wait 10 seconds")},
		System:            "You are a bot.",
		SupportsToolCall:  true,
		Identity:          SessionContext{BotID: "bot-test-3", SessionID: "sess-3"},
		BackgroundManager: bgMgr,
	})
	if err != nil {
		t.Fatalf("Generate error: %v", err)
	}
	if result.Text != "Got it, won't sleep." {
		t.Errorf("unexpected text: %q", result.Text)
	}

	if !sleepWasError {
		t.Error("expected sleep command to return is_error=true")
	}
	_ = sleepToolResult // the error message is in the tool result
}

// ---------------------------------------------------------------------------
// E2E Test: Running tasks summary injection
// ---------------------------------------------------------------------------

func TestE2E_RunningTasksSummaryInjected(t *testing.T) {
	t.Parallel()

	svc := newMockExecContainerService()
	// Long-running task that won't complete during the test.
	svc.setBehavior("long-task", execBehavior{
		delay: 30 * time.Second,
	})

	bp, cleanup := setupExecTestInfra(t, svc)
	defer cleanup()

	bgMgr := background.New(nil)

	var step3System string
	modelProvider := &agentReadMediaMockProvider{
		handler: func(call int, params sdk.GenerateParams) (*sdk.GenerateResult, error) {
			switch call {
			case 1:
				return &sdk.GenerateResult{
					FinishReason: sdk.FinishReasonToolCalls,
					ToolCalls: []sdk.ToolCall{{
						ToolCallID: "call-1",
						ToolName:   "exec",
						Input: map[string]any{
							"command":           "long-task",
							"run_in_background": true,
							"description":       "Long running task",
						},
					}},
				}, nil
			case 2:
				// Do another tool call so prepareStep fires again.
				return &sdk.GenerateResult{
					FinishReason: sdk.FinishReasonToolCalls,
					ToolCalls: []sdk.ToolCall{{
						ToolCallID: "call-2",
						ToolName:   "exec",
						Input: map[string]any{
							"command": "echo check",
						},
					}},
				}, nil
			case 3:
				// Capture the system prompt which should include running tasks.
				step3System = params.System
				return &sdk.GenerateResult{
					Text:         "Done checking.",
					FinishReason: sdk.FinishReasonStop,
				}, nil
			default:
				return &sdk.GenerateResult{
					Text:         "unexpected",
					FinishReason: sdk.FinishReasonStop,
				}, nil
			}
		},
	}

	a := New(Deps{})
	a.SetToolProviders([]agenttools.ToolProvider{
		agenttools.NewContainerProvider(nil, bp, bgMgr, "/data"),
	})

	_, err := a.Generate(context.Background(), RunConfig{
		Model:             &sdk.Model{ID: "mock", Provider: modelProvider},
		Messages:          []sdk.Message{sdk.UserMessage("start background and check")},
		System:            "You are a bot.",
		SupportsToolCall:  true,
		Identity:          SessionContext{BotID: "bot-test-4", SessionID: "sess-4"},
		BackgroundManager: bgMgr,
	})
	if err != nil {
		t.Fatalf("Generate error: %v", err)
	}

	if !strings.Contains(step3System, "Currently running background tasks:") {
		t.Error("expected running tasks summary in system prompt")
	}
	if !strings.Contains(step3System, "Long running task") {
		t.Errorf("expected task description in system prompt, got: %s", step3System)
	}
}

// ---------------------------------------------------------------------------
// Helpers for extracting tool results from params
// ---------------------------------------------------------------------------

func extractToolResult(t *testing.T, params sdk.GenerateParams, toolCallID string) map[string]any {
	t.Helper()
	for _, msg := range params.Messages {
		if msg.Role != sdk.MessageRoleTool {
			continue
		}
		for _, part := range msg.Content {
			tr, ok := part.(sdk.ToolResultPart)
			if !ok || tr.ToolCallID != toolCallID {
				continue
			}
			raw, _ := json.Marshal(tr.Result)
			var m map[string]any
			_ = json.Unmarshal(raw, &m)
			return m
		}
	}
	t.Fatalf("tool result for %s not found in params", toolCallID)
	return nil
}

func extractToolResultWithError(params sdk.GenerateParams, toolCallID string) (map[string]any, bool) {
	for _, msg := range params.Messages {
		if msg.Role != sdk.MessageRoleTool {
			continue
		}
		for _, part := range msg.Content {
			tr, ok := part.(sdk.ToolResultPart)
			if !ok || tr.ToolCallID != toolCallID {
				continue
			}
			raw, _ := json.Marshal(tr.Result)
			var m map[string]any
			_ = json.Unmarshal(raw, &m)
			return m, tr.IsError
		}
	}
	return nil, false
}
