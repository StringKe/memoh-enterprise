package botgroups

import "time"

const (
	VisibilityPrivate      = "private"
	VisibilityOrganization = "organization"
	VisibilityPublic       = "public"
)

type Group struct {
	ID          string         `json:"id"`
	OwnerUserID string         `json:"owner_user_id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Visibility  string         `json:"visibility"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	BotCount    int64          `json:"bot_count"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

type CreateGroupRequest struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Visibility  string         `json:"visibility,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type UpdateGroupRequest struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Visibility  string         `json:"visibility,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type GroupSettings struct {
	GroupID                string         `json:"group_id"`
	Timezone               *string        `json:"timezone,omitempty"`
	Language               *string        `json:"language,omitempty"`
	ReasoningEnabled       *bool          `json:"reasoning_enabled,omitempty"`
	ReasoningEffort        *string        `json:"reasoning_effort,omitempty"`
	ChatModelID            *string        `json:"chat_model_id,omitempty"`
	SearchProviderID       *string        `json:"search_provider_id,omitempty"`
	MemoryProviderID       *string        `json:"memory_provider_id,omitempty"`
	HeartbeatEnabled       *bool          `json:"heartbeat_enabled,omitempty"`
	HeartbeatInterval      *int32         `json:"heartbeat_interval,omitempty"`
	HeartbeatPrompt        *string        `json:"heartbeat_prompt,omitempty"`
	HeartbeatModelID       *string        `json:"heartbeat_model_id,omitempty"`
	CompactionEnabled      *bool          `json:"compaction_enabled,omitempty"`
	CompactionThreshold    *int32         `json:"compaction_threshold,omitempty"`
	CompactionRatio        *int32         `json:"compaction_ratio,omitempty"`
	CompactionModelID      *string        `json:"compaction_model_id,omitempty"`
	TitleModelID           *string        `json:"title_model_id,omitempty"`
	ImageModelID           *string        `json:"image_model_id,omitempty"`
	DiscussProbeModelID    *string        `json:"discuss_probe_model_id,omitempty"`
	TTSModelID             *string        `json:"tts_model_id,omitempty"`
	TranscriptionModelID   *string        `json:"transcription_model_id,omitempty"`
	BrowserContextID       *string        `json:"browser_context_id,omitempty"`
	PersistFullToolResults *bool          `json:"persist_full_tool_results,omitempty"`
	ShowToolCallsInIM      *bool          `json:"show_tool_calls_in_im,omitempty"`
	ToolApprovalConfig     map[string]any `json:"tool_approval_config,omitempty"`
	OverlayProvider        *string        `json:"overlay_provider,omitempty"`
	OverlayEnabled         *bool          `json:"overlay_enabled,omitempty"`
	OverlayConfig          map[string]any `json:"overlay_config,omitempty"`
	Metadata               map[string]any `json:"metadata,omitempty"`
	UpdatedAt              time.Time      `json:"updated_at"`
}
