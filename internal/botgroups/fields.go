package botgroups

const (
	FieldTimezone               = "timezone"
	FieldLanguage               = "language"
	FieldReasoningEnabled       = "reasoning_enabled"
	FieldReasoningEffort        = "reasoning_effort"
	FieldChatModelID            = "chat_model_id"
	FieldSearchProviderID       = "search_provider_id"
	FieldMemoryProviderID       = "memory_provider_id"
	FieldHeartbeatEnabled       = "heartbeat_enabled"
	FieldHeartbeatInterval      = "heartbeat_interval"
	FieldHeartbeatPrompt        = "heartbeat_prompt"
	FieldHeartbeatModelID       = "heartbeat_model_id"
	FieldCompactionEnabled      = "compaction_enabled"
	FieldCompactionThreshold    = "compaction_threshold"
	FieldCompactionRatio        = "compaction_ratio"
	FieldCompactionModelID      = "compaction_model_id"
	FieldTitleModelID           = "title_model_id"
	FieldImageModelID           = "image_model_id"
	FieldDiscussProbeModelID    = "discuss_probe_model_id"
	FieldTTSModelID             = "tts_model_id"
	FieldTranscriptionModelID   = "transcription_model_id"
	FieldPersistFullToolResults = "persist_full_tool_results"
	FieldShowToolCallsInIM      = "show_tool_calls_in_im"
	FieldToolApprovalConfig     = "tool_approval_config"
	FieldOverlayProvider        = "overlay_provider"
	FieldOverlayEnabled         = "overlay_enabled"
	FieldOverlayConfig          = "overlay_config"
)

var InheritableFields = []string{
	FieldTimezone,
	FieldLanguage,
	FieldReasoningEnabled,
	FieldReasoningEffort,
	FieldChatModelID,
	FieldSearchProviderID,
	FieldMemoryProviderID,
	FieldHeartbeatEnabled,
	FieldHeartbeatInterval,
	FieldHeartbeatPrompt,
	FieldHeartbeatModelID,
	FieldCompactionEnabled,
	FieldCompactionThreshold,
	FieldCompactionRatio,
	FieldCompactionModelID,
	FieldTitleModelID,
	FieldImageModelID,
	FieldDiscussProbeModelID,
	FieldTTSModelID,
	FieldTranscriptionModelID,
	FieldPersistFullToolResults,
	FieldShowToolCallsInIM,
	FieldToolApprovalConfig,
	FieldOverlayProvider,
	FieldOverlayEnabled,
	FieldOverlayConfig,
}
