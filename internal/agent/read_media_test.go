package agent

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"connectrpc.com/connect"
	sdk "github.com/memohai/twilight-ai/sdk"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	agenttools "github.com/memohai/memoh/internal/agent/tools"
	workspacev1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/workspace/v1"
	"github.com/memohai/memoh/internal/connectapi/gen/memoh/workspace/v1/workspacev1connect"
	"github.com/memohai/memoh/internal/workspace/executorclient"
)

// agentReadMediaContainerService implements both ReadFile and ReadRaw so
// that the merged read tool (ContainerProvider) can detect binary files
// and then delegate to ReadImageFromContainer.
type agentReadMediaContainerService struct {
	workspacev1connect.UnimplementedWorkspaceExecutorServiceHandler
	files map[string][]byte
}

func (s *agentReadMediaContainerService) ReadFile(_ context.Context, req *connect.Request[workspacev1.ReadFileRequest]) (*connect.Response[workspacev1.ReadFileResponse], error) {
	data, ok := s.files[req.Msg.GetPath()]
	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("not found"))
	}
	_ = data
	return connect.NewResponse(&workspacev1.ReadFileResponse{Binary: true}), nil
}

func (s *agentReadMediaContainerService) ReadRaw(_ context.Context, req *connect.Request[workspacev1.ReadRawRequest], stream *connect.ServerStream[workspacev1.ReadRawResponse]) error {
	data, ok := s.files[req.Msg.GetPath()]
	if !ok {
		return connect.NewError(connect.CodeNotFound, errors.New("not found"))
	}
	if len(data) == 0 {
		return nil
	}
	return stream.Send(&workspacev1.ReadRawResponse{Chunk: &workspacev1.DataChunk{Data: data}})
}

type agentReadMediaWorkspaceExecutorProvider struct {
	client *executorclient.Client
}

func (p *agentReadMediaWorkspaceExecutorProvider) ExecutorClient(_ context.Context, _ string) (*executorclient.Client, error) {
	return p.client, nil
}

func newAgentReadMediaWorkspaceExecutorProvider(t *testing.T, files map[string][]byte) executorclient.Provider {
	t.Helper()

	mux := http.NewServeMux()
	path, handler := workspacev1connect.NewWorkspaceExecutorServiceHandler(&agentReadMediaContainerService{files: files})
	mux.Handle(path, handler)
	server := httptest.NewServer(h2c.NewHandler(mux, &http2.Server{}))
	t.Cleanup(server.Close)
	client, err := executorclient.Dial(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("executorclient.Dial: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	return &agentReadMediaWorkspaceExecutorProvider{client: client}
}

type agentReadMediaMockProvider struct {
	name    string
	calls   int
	handler func(call int, params sdk.GenerateParams) (*sdk.GenerateResult, error)
}

func (m *agentReadMediaMockProvider) Name() string {
	if m.name != "" {
		return m.name
	}
	return "mock"
}

func (*agentReadMediaMockProvider) ListModels(context.Context) ([]sdk.Model, error) {
	return nil, nil
}

func (*agentReadMediaMockProvider) Test(context.Context) *sdk.ProviderTestResult {
	return &sdk.ProviderTestResult{Status: sdk.ProviderStatusOK, Message: "ok"}
}

func (*agentReadMediaMockProvider) TestModel(context.Context, string) (*sdk.ModelTestResult, error) {
	return &sdk.ModelTestResult{Supported: true, Message: "supported"}, nil
}

func (m *agentReadMediaMockProvider) DoGenerate(_ context.Context, params sdk.GenerateParams) (*sdk.GenerateResult, error) {
	m.calls++
	return m.handler(m.calls, params)
}

func (m *agentReadMediaMockProvider) DoStream(ctx context.Context, params sdk.GenerateParams) (*sdk.StreamResult, error) {
	result, err := m.DoGenerate(ctx, params)
	if err != nil {
		return nil, err
	}
	ch := make(chan sdk.StreamPart, 8)
	go func() {
		defer close(ch)
		ch <- &sdk.StartPart{}
		ch <- &sdk.StartStepPart{}
		if result.Text != "" {
			ch <- &sdk.TextStartPart{ID: "mock"}
			ch <- &sdk.TextDeltaPart{ID: "mock", Text: result.Text}
			ch <- &sdk.TextEndPart{ID: "mock"}
		}
		for _, tc := range result.ToolCalls {
			ch <- &sdk.StreamToolCallPart{
				ToolCallID: tc.ToolCallID,
				ToolName:   tc.ToolName,
				Input:      tc.Input,
			}
		}
		ch <- &sdk.FinishStepPart{FinishReason: result.FinishReason, Usage: result.Usage, Response: result.Response}
		ch <- &sdk.FinishPart{FinishReason: result.FinishReason, TotalUsage: result.Usage}
	}()
	return &sdk.StreamResult{Stream: ch}, nil
}

func assertInjectedReadMediaMessage(t *testing.T, msg sdk.Message, expectedImage, expectedMediaType string) {
	t.Helper()

	if msg.Role != sdk.MessageRoleUser {
		t.Fatalf("expected injected read_media message role %q, got %q", sdk.MessageRoleUser, msg.Role)
	}
	if len(msg.Content) != 1 {
		t.Fatalf("expected one injected content part, got %d", len(msg.Content))
	}
	image, ok := msg.Content[0].(sdk.ImagePart)
	if !ok {
		t.Fatalf("expected sdk.ImagePart, got %T", msg.Content[0])
	}
	if image.Image != expectedImage {
		t.Fatalf("unexpected injected image payload: %q", image.Image)
	}
	if image.MediaType != expectedMediaType {
		t.Fatalf("unexpected injected media type: %q", image.MediaType)
	}
}

func TestAgentGenerateReadMediaInjectsImageIntoNextStep(t *testing.T) {
	t.Parallel()

	// The PNG data must contain a null byte (\x00) so that the execRead
	// binary probe (bytes.IndexByte(probe, 0)) detects it as binary and
	// delegates to ReadImageFromContainer. Real PNG files always contain
	// null bytes in their IHDR and other chunks.
	pngBytes := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00payload")
	expectedDataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngBytes)

	modelProvider := &agentReadMediaMockProvider{
		handler: func(call int, params sdk.GenerateParams) (*sdk.GenerateResult, error) {
			if call == 1 {
				return &sdk.GenerateResult{
					FinishReason: sdk.FinishReasonToolCalls,
					ToolCalls: []sdk.ToolCall{{
						ToolCallID: "call-1",
						ToolName:   "read",
						Input:      map[string]any{"path": "/data/images/demo.png"},
					}},
				}, nil
			}

			if len(params.Messages) < 4 {
				t.Fatalf("expected prior tool and injected messages, got %d", len(params.Messages))
			}

			last := params.Messages[len(params.Messages)-1]
			if last.Role != sdk.MessageRoleUser {
				t.Fatalf("expected last message to be injected user image, got %s", last.Role)
			}
			if len(last.Content) != 1 {
				t.Fatalf("expected one injected content part, got %d", len(last.Content))
			}
			image, ok := last.Content[0].(sdk.ImagePart)
			if !ok {
				t.Fatalf("expected sdk.ImagePart, got %T", last.Content[0])
			}
			if image.Image != expectedDataURL {
				t.Fatalf("unexpected injected image payload: %q", image.Image)
			}
			if image.MediaType != "image/png" {
				t.Fatalf("unexpected injected media type: %q", image.MediaType)
			}

			var toolResult sdk.ToolResultPart
			foundToolMessage := false
			for _, msg := range params.Messages {
				if msg.Role != sdk.MessageRoleTool || len(msg.Content) == 0 {
					continue
				}
				part, ok := msg.Content[0].(sdk.ToolResultPart)
				if !ok {
					continue
				}
				toolResult = part
				foundToolMessage = true
				break
			}
			if !foundToolMessage {
				t.Fatal("expected tool result message before second step")
			}
			raw, err := json.Marshal(toolResult.Result)
			if err != nil {
				t.Fatalf("marshal tool result: %v", err)
			}
			if !bytes.Contains(raw, []byte(`"ok":true`)) {
				t.Fatalf("expected compact success metadata, got %s", raw)
			}
			if bytes.Contains(raw, []byte(expectedDataURL)) || bytes.Contains(raw, []byte("payload")) {
				t.Fatalf("tool result leaked image bytes: %s", raw)
			}

			return &sdk.GenerateResult{
				Text:         "done",
				FinishReason: sdk.FinishReasonStop,
			}, nil
		},
	}

	// ContainerProvider normalizes paths by stripping the workdir prefix,
	// so the mock files map must use the normalized (relative) path.
	bp := newAgentReadMediaWorkspaceExecutorProvider(t, map[string][]byte{
		"images/demo.png": pngBytes,
	})

	a := New(Deps{})
	a.SetToolProviders([]agenttools.ToolProvider{
		agenttools.NewContainerProvider(nil, bp, nil, "/data"),
	})

	result, err := a.Generate(context.Background(), RunConfig{
		Model:              &sdk.Model{ID: "mock-model", Provider: modelProvider},
		Messages:           []sdk.Message{sdk.UserMessage("look at the image")},
		SupportsImageInput: true,
		SupportsToolCall:   true,
		Identity: SessionContext{
			BotID: "bot-1",
		},
	})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	if result.Text != "done" {
		t.Fatalf("unexpected result text: %q", result.Text)
	}
	if len(result.Messages) != 4 {
		t.Fatalf("expected persisted step + injected history, got %d messages", len(result.Messages))
	}
	assertInjectedReadMediaMessage(t, result.Messages[2], expectedDataURL, "image/png")
	if result.Messages[3].Role != sdk.MessageRoleAssistant {
		t.Fatalf("expected final persisted message to be assistant, got %s", result.Messages[3].Role)
	}
	if modelProvider.calls != 2 {
		t.Fatalf("expected 2 model calls, got %d", modelProvider.calls)
	}
}

func TestAgentGenerateReadMediaInjectsAnthropicSafeImageIntoNextStep(t *testing.T) {
	t.Parallel()

	pngBytes := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00payload")
	expectedBase64 := base64.StdEncoding.EncodeToString(pngBytes)

	modelProvider := &agentReadMediaMockProvider{
		name: "anthropic-messages",
		handler: func(call int, params sdk.GenerateParams) (*sdk.GenerateResult, error) {
			if call == 1 {
				return &sdk.GenerateResult{
					FinishReason: sdk.FinishReasonToolCalls,
					ToolCalls: []sdk.ToolCall{{
						ToolCallID: "call-1",
						ToolName:   "read",
						Input:      map[string]any{"path": "/data/images/demo.png"},
					}},
				}, nil
			}

			last := params.Messages[len(params.Messages)-1]
			image, ok := last.Content[0].(sdk.ImagePart)
			if !ok {
				t.Fatalf("expected sdk.ImagePart, got %T", last.Content[0])
			}
			if image.Image != expectedBase64 {
				t.Fatalf("expected raw base64 for anthropic, got %q", image.Image)
			}
			if image.MediaType != "image/png" {
				t.Fatalf("unexpected injected media type: %q", image.MediaType)
			}
			if strings.HasPrefix(image.Image, "data:") {
				t.Fatalf("anthropic image payload must not be a data URL: %q", image.Image)
			}

			return &sdk.GenerateResult{
				Text:         "done",
				FinishReason: sdk.FinishReasonStop,
			}, nil
		},
	}

	// ContainerProvider normalizes paths by stripping the workdir prefix,
	// so the mock files map must use the normalized (relative) path.
	bp := newAgentReadMediaWorkspaceExecutorProvider(t, map[string][]byte{
		"images/demo.png": pngBytes,
	})

	a := New(Deps{})
	a.SetToolProviders([]agenttools.ToolProvider{
		agenttools.NewContainerProvider(nil, bp, nil, "/data"),
	})

	_, err := a.Generate(context.Background(), RunConfig{
		Model:              &sdk.Model{ID: "mock-model", Provider: modelProvider},
		Messages:           []sdk.Message{sdk.UserMessage("look at the image")},
		SupportsImageInput: true,
		SupportsToolCall:   true,
		Identity: SessionContext{
			BotID: "bot-1",
		},
	})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
}

func TestAgentStreamReadMediaPersistsInjectedImageInTerminalMessages(t *testing.T) {
	t.Parallel()

	pngBytes := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00payload")
	expectedDataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngBytes)

	modelProvider := &agentReadMediaMockProvider{
		handler: func(call int, _ sdk.GenerateParams) (*sdk.GenerateResult, error) {
			if call == 1 {
				return &sdk.GenerateResult{
					FinishReason: sdk.FinishReasonToolCalls,
					ToolCalls: []sdk.ToolCall{{
						ToolCallID: "call-1",
						ToolName:   "read",
						Input:      map[string]any{"path": "/data/images/demo.png"},
					}},
				}, nil
			}
			return &sdk.GenerateResult{
				Text:         "done",
				FinishReason: sdk.FinishReasonStop,
			}, nil
		},
	}

	// ContainerProvider normalizes paths by stripping the workdir prefix,
	// so the mock files map must use the normalized (relative) path.
	bp := newAgentReadMediaWorkspaceExecutorProvider(t, map[string][]byte{
		"images/demo.png": pngBytes,
	})

	a := New(Deps{})
	a.SetToolProviders([]agenttools.ToolProvider{
		agenttools.NewContainerProvider(nil, bp, nil, "/data"),
	})

	var terminal StreamEvent
	for event := range a.Stream(context.Background(), RunConfig{
		Model:              &sdk.Model{ID: "mock-model", Provider: modelProvider},
		Messages:           []sdk.Message{sdk.UserMessage("look at the image")},
		SupportsImageInput: true,
		SupportsToolCall:   true,
		Identity: SessionContext{
			BotID: "bot-1",
		},
	}) {
		if event.IsTerminal() {
			terminal = event
		}
	}

	if terminal.Type != EventAgentEnd {
		t.Fatalf("expected terminal event %q, got %q", EventAgentEnd, terminal.Type)
	}

	var messages []sdk.Message
	if err := json.Unmarshal(terminal.Messages, &messages); err != nil {
		t.Fatalf("unmarshal terminal messages: %v", err)
	}
	if len(messages) != 4 {
		t.Fatalf("expected persisted step + injected history, got %d messages", len(messages))
	}
	assertInjectedReadMediaMessage(t, messages[2], expectedDataURL, "image/png")
	if messages[3].Role != sdk.MessageRoleAssistant {
		t.Fatalf("expected final persisted message to be assistant, got %s", messages[3].Role)
	}
}
