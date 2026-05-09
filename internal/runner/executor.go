package runner

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	sdk "github.com/memohai/twilight-ai/sdk"
	"google.golang.org/protobuf/types/known/structpb"

	agentpkg "github.com/memohai/memoh/internal/agent"
	agenttools "github.com/memohai/memoh/internal/agent/tools"
	eventv1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/event/v1"
	runnerv1 "github.com/memohai/memoh/internal/connectapi/gen/memoh/runner/v1"
	"github.com/memohai/memoh/internal/models"
	"github.com/memohai/memoh/internal/workspace/executorclient"
)

const (
	RunEventTextDelta = "text_delta"
	RunEventTool      = "tool"
)

type Executor interface {
	Execute(ctx context.Context, input ExecutionInput) (ExecutionResult, error)
}

type ExecutionInput struct {
	Request RunRequest
	Context *runnerv1.ResolveRunContextResponse
	History []*runnerv1.SessionMessage
	Emit    func(event *eventv1.AgentRunEvent) error
}

type ExecutionResult struct {
	Status        string
	AssistantText string
}

type AgentExecutorDeps struct {
	Logger         *slog.Logger
	Workspace      *WorkspaceClient
	Provider       *ProviderClient
	Memory         *MemoryClient
	ToolApproval   *ToolApprovalClient
	StructuredData *StructuredDataClient
	HTTPClient     *http.Client
}

type AgentExecutor struct {
	logger         *slog.Logger
	workspace      *WorkspaceClient
	provider       *ProviderClient
	memory         *MemoryClient
	toolApproval   *ToolApprovalClient
	structuredData *StructuredDataClient
	httpClient     *http.Client
}

func NewAgentExecutor(deps AgentExecutorDeps) *AgentExecutor {
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}
	httpClient := deps.HTTPClient
	if httpClient == nil {
		httpClient = models.NewProviderHTTPClient(0)
	}
	return &AgentExecutor{
		logger:         logger,
		workspace:      deps.Workspace,
		provider:       deps.Provider,
		memory:         deps.Memory,
		toolApproval:   deps.ToolApproval,
		structuredData: deps.StructuredData,
		httpClient:     httpClient,
	}
}

func (e *AgentExecutor) Execute(ctx context.Context, input ExecutionInput) (ExecutionResult, error) {
	if e == nil {
		return ExecutionResult{}, errors.New("agent executor is not configured")
	}
	cfg, err := e.buildRunConfig(ctx, input)
	if err != nil {
		return ExecutionResult{}, err
	}
	workspaceProvider := &supportWorkspaceProvider{workspace: e.workspace, lease: input.Request.Lease}
	agent := agentpkg.New(agentpkg.Deps{
		Logger:                    e.logger,
		WorkspaceExecutorProvider: workspaceProvider,
	})
	agent.SetToolProviders(e.toolProviders(workspaceProvider, input.Request.Lease))
	var assistant strings.Builder
	for event := range agent.Stream(ctx, cfg) {
		if event.Type == agentpkg.EventTextDelta {
			assistant.WriteString(event.Delta)
		}
		if input.Emit != nil {
			if err := input.Emit(agentStreamEventToRunEvent(input.Request.Lease, event)); err != nil {
				return ExecutionResult{}, err
			}
		}
		if event.Type == agentpkg.EventError {
			return ExecutionResult{}, errors.New(event.Error)
		}
		if event.Type == agentpkg.EventAgentAbort {
			return ExecutionResult{Status: RunStatusCancelled, AssistantText: assistant.String()}, nil
		}
	}
	assistantText := assistant.String()
	if e.memory != nil && strings.TrimSpace(assistantText) != "" {
		_, err := e.memory.WriteMemory(ctx, input.Request.Lease, &runnerv1.MemoryRecord{
			Scope:   "bot",
			Content: assistantText,
		})
		if err != nil {
			return ExecutionResult{}, err
		}
	}
	return ExecutionResult{Status: RunStatusCompleted, AssistantText: assistantText}, nil
}

func (e *AgentExecutor) buildRunConfig(ctx context.Context, input ExecutionInput) (agentpkg.RunConfig, error) {
	modelContext := structToMap(input.Context.GetModel())
	modelID := firstString(modelContext, "model_id", "id")
	providerID := firstString(modelContext, "provider_id")
	if modelID == "" || providerID == "" {
		return agentpkg.RunConfig{}, errors.New("runner model context requires model_id and provider_id")
	}
	if e.provider == nil {
		return agentpkg.RunConfig{}, ErrSupportClientMissing
	}
	creds, err := e.provider.ResolveProviderCredentials(ctx, input.Request.Lease, providerID, "", []string{"model_call"})
	if err != nil {
		return agentpkg.RunConfig{}, err
	}
	credentials := structToMap(creds.GetCredentials())
	clientType := firstString(credentials, "client_type")
	if clientType == "" {
		clientType = firstString(modelContext, "client_type")
	}
	sdkModel := models.NewSDKChatModel(models.SDKModelConfig{
		ModelID:         modelID,
		ClientType:      clientType,
		APIKey:          firstString(credentials, "api_key"),
		CodexAccountID:  firstString(credentials, "codex_account_id"),
		BaseURL:         firstString(credentials, "base_url"),
		HTTPClient:      e.httpClient,
		ReasoningConfig: reasoningConfig(modelContext),
	})
	messages := sdkMessagesFromHistory(input.History)
	if e.memory != nil && strings.TrimSpace(input.Request.Prompt) != "" {
		memories, err := e.memory.ReadMemory(ctx, input.Request.Lease, input.Request.Prompt, []string{"bot"}, 6)
		if err != nil {
			return agentpkg.RunConfig{}, err
		}
		if len(memories) > 0 {
			messages = append(messages, sdk.UserMessage(formatMemoryContext(memories)))
		}
	}
	if strings.TrimSpace(input.Request.Prompt) != "" {
		messages = append(messages, sdk.UserMessage(input.Request.Prompt))
	}
	botContext := structToMap(input.Context.GetBot())
	now := time.Now().UTC()
	return agentpkg.RunConfig{
		Model:              sdkModel,
		ReasoningEffort:    firstString(modelContext, "reasoning_effort"),
		PromptCacheTTL:     firstString(modelContext, "prompt_cache_ttl"),
		Messages:           messages,
		Query:              input.Request.Prompt,
		System:             agentpkg.GenerateSystemPrompt(agentpkg.SystemPromptParams{Now: now, Timezone: firstString(botContext, "timezone"), SupportsImageInput: stringSliceContains(stringSlice(modelContext, "compatibilities"), models.CompatVision)}),
		SessionType:        firstString(botContext, "session_type"),
		SupportsImageInput: stringSliceContains(stringSlice(modelContext, "compatibilities"), models.CompatVision),
		SupportsToolCall:   stringSliceContains(stringSlice(modelContext, "compatibilities"), models.CompatToolCall),
		Identity: agentpkg.SessionContext{
			BotID:     input.Request.Lease.BotID,
			ChatID:    input.Request.Lease.BotID,
			SessionID: input.Request.Lease.SessionID,
			Timezone:  firstString(botContext, "timezone"),
		},
		LoopDetection: agentpkg.LoopDetectionConfig{Enabled: boolValue(botContext, "loop_detection_enabled")},
		ToolApprovalHandler: func(ctx context.Context, call sdk.ToolCall) (sdk.ToolApprovalResult, error) {
			return e.handleToolApproval(ctx, input.Request.Lease, call)
		},
	}, nil
}

func (e *AgentExecutor) toolProviders(workspaceProvider executorclient.Provider, lease RunLease) []agenttools.ToolProvider {
	providers := []agenttools.ToolProvider{
		agenttools.NewContainerProvider(e.logger, workspaceProvider, nil, ""),
	}
	if e.structuredData != nil {
		providers = append(providers, agenttools.NewStructuredDataProvider(e.structuredData.Runtime(lease)))
	}
	return providers
}

func (e *AgentExecutor) handleToolApproval(ctx context.Context, lease RunLease, call sdk.ToolCall) (sdk.ToolApprovalResult, error) {
	if e.toolApproval == nil {
		return sdk.ToolApprovalResult{Decision: sdk.ToolApprovalDecisionApproved}, nil
	}
	payload := map[string]any{
		"tool_call_id": call.ToolCallID,
		"tool_name":    call.ToolName,
		"input":        call.Input,
	}
	st, _ := structpb.NewStruct(payload)
	decision, err := e.toolApproval.EvaluateToolApprovalPolicy(ctx, lease, call.ToolName, "", st)
	if err != nil {
		return sdk.ToolApprovalResult{}, err
	}
	if decision.GetDecision() != "requires_approval" {
		return sdk.ToolApprovalResult{Decision: sdk.ToolApprovalDecisionApproved}, nil
	}
	approval, err := e.toolApproval.RequestToolApproval(ctx, lease, call.ToolName, "", st)
	if err != nil {
		return sdk.ToolApprovalResult{}, err
	}
	return sdk.ToolApprovalResult{
		Decision:   sdk.ToolApprovalDecisionDeferred,
		ApprovalID: approval.GetApprovalRequestId(),
		Metadata: map[string]any{
			"tool_name":     call.ToolName,
			"tool_call_id":  call.ToolCallID,
			"approval_id":   approval.GetApprovalRequestId(),
			"approval_stat": approval.GetStatus(),
		},
	}, nil
}

type supportWorkspaceProvider struct {
	workspace *WorkspaceClient
	lease     RunLease
}

func (p *supportWorkspaceProvider) ExecutorClient(ctx context.Context, _ string) (*executorclient.Client, error) {
	if p == nil || p.workspace == nil {
		return nil, ErrSupportClientMissing
	}
	client, _, err := p.workspace.ExecutorClient(ctx, p.lease, []string{"workspace.exec", "workspace.read", "workspace.write"})
	if err != nil {
		return nil, err
	}
	return executorclient.NewClient(client, nil), nil
}

func sdkMessagesFromHistory(history []*runnerv1.SessionMessage) []sdk.Message {
	messages := make([]sdk.Message, 0, len(history))
	for _, item := range history {
		text := strings.TrimSpace(item.GetText())
		if text == "" {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(item.GetRole())) {
		case "assistant":
			messages = append(messages, sdk.AssistantMessage(text))
		case "system":
			messages = append(messages, sdk.SystemMessage(text))
		default:
			messages = append(messages, sdk.UserMessage(text))
		}
	}
	return messages
}

func formatMemoryContext(memories []*runnerv1.MemoryRecord) string {
	var b strings.Builder
	b.WriteString("<memory_context>\n")
	for _, memory := range memories {
		content := strings.TrimSpace(memory.GetContent())
		if content == "" {
			continue
		}
		b.WriteString("- ")
		b.WriteString(content)
		b.WriteString("\n")
	}
	b.WriteString("</memory_context>")
	return b.String()
}

func agentStreamEventToRunEvent(lease RunLease, event agentpkg.StreamEvent) *eventv1.AgentRunEvent {
	payload := map[string]any{}
	data, err := json.Marshal(event)
	if err == nil {
		_ = json.Unmarshal(data, &payload)
	}
	eventType := string(event.Type)
	text := event.Delta
	status := RunStatusRunning
	if event.Type == agentpkg.EventError {
		text = event.Error
		status = RunStatusFailed
	}
	if event.Type == agentpkg.EventAgentEnd {
		status = RunStatusCompleted
	}
	if event.Type == agentpkg.EventAgentAbort {
		status = RunStatusCancelled
	}
	return RunEvent{
		EventType: eventType,
		Status:    status,
		Text:      text,
		Payload:   mapToStruct(payload),
	}.ProtoForLease(lease)
}

func structToMap(value *structpb.Struct) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	return value.AsMap()
}

func mapToStruct(value map[string]any) *structpb.Struct {
	if len(value) == 0 {
		return nil
	}
	out, err := structpb.NewStruct(value)
	if err != nil {
		return nil
	}
	return out
}

func firstString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		switch v := values[key].(type) {
		case string:
			if strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
		case json.Number:
			if strings.TrimSpace(v.String()) != "" {
				return strings.TrimSpace(v.String())
			}
		}
	}
	return ""
}

func boolValue(values map[string]any, key string) bool {
	v, _ := values[key].(bool)
	return v
}

func stringSlice(values map[string]any, key string) []string {
	raw, ok := values[key].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
			out = append(out, strings.TrimSpace(text))
		}
	}
	return out
}

func stringSliceContains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func reasoningConfig(values map[string]any) *models.ReasoningConfig {
	if !boolValue(values, "reasoning_enabled") {
		return nil
	}
	effort := firstString(values, "reasoning_effort")
	if effort == "" {
		effort = models.ReasoningEffortMedium
	}
	return &models.ReasoningConfig{Enabled: true, Effort: effort}
}
