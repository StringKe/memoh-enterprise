package settings

import (
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
)

const (
	SourceBot      = "bot"
	SourceBotGroup = "bot_group"
	SourceSystem   = "system"
)

func decodeOverrideMask(payload []byte) OverrideMask {
	if len(payload) == 0 {
		return OverrideMask{}
	}
	var mask OverrideMask
	if err := json.Unmarshal(payload, &mask); err != nil || mask == nil {
		return OverrideMask{}
	}
	return mask
}

func overrideEnabled(mask OverrideMask, field string) bool {
	if mask == nil {
		return true
	}
	enabled, ok := mask[field]
	if !ok {
		return true
	}
	return enabled
}

func applyGroupDefaults(local Settings, group sqlc.BotGroupSetting, mask OverrideMask) Settings {
	local.OverrideMask = mask
	groupID := group.GroupID.String()
	if groupID != "" && group.GroupID.Valid {
		local.GroupID = groupID
	}
	sources := make([]FieldSource, 0, len(InheritableFields))
	applyString := func(field string, value pgtype.Text, target *string) {
		if !overrideEnabled(mask, field) {
			if value.Valid {
				*target = value.String
				sources = append(sources, FieldSource{Field: field, Source: SourceBotGroup, SourceID: groupID})
				return
			}
			sources = append(sources, FieldSource{Field: field, Source: SourceSystem})
			return
		}
		sources = append(sources, FieldSource{Field: field, Source: SourceBot})
	}
	applyBool := func(field string, value pgtype.Bool, target *bool) {
		if !overrideEnabled(mask, field) {
			if value.Valid {
				*target = value.Bool
				sources = append(sources, FieldSource{Field: field, Source: SourceBotGroup, SourceID: groupID})
				return
			}
			sources = append(sources, FieldSource{Field: field, Source: SourceSystem})
			return
		}
		sources = append(sources, FieldSource{Field: field, Source: SourceBot})
	}
	applyInt := func(field string, value pgtype.Int4, target *int) {
		if !overrideEnabled(mask, field) {
			if value.Valid {
				*target = int(value.Int32)
				sources = append(sources, FieldSource{Field: field, Source: SourceBotGroup, SourceID: groupID})
				return
			}
			sources = append(sources, FieldSource{Field: field, Source: SourceSystem})
			return
		}
		sources = append(sources, FieldSource{Field: field, Source: SourceBot})
	}
	applyUUID := func(field string, value pgtype.UUID, target *string) {
		if !overrideEnabled(mask, field) {
			if value.Valid {
				*target = uuid.UUID(value.Bytes).String()
				sources = append(sources, FieldSource{Field: field, Source: SourceBotGroup, SourceID: groupID})
				return
			}
			sources = append(sources, FieldSource{Field: field, Source: SourceSystem})
			return
		}
		sources = append(sources, FieldSource{Field: field, Source: SourceBot})
	}
	applyJSON := func(field string, value []byte, apply func([]byte)) {
		if !overrideEnabled(mask, field) {
			if len(value) > 0 {
				apply(value)
				sources = append(sources, FieldSource{Field: field, Source: SourceBotGroup, SourceID: groupID})
				return
			}
			sources = append(sources, FieldSource{Field: field, Source: SourceSystem})
			return
		}
		sources = append(sources, FieldSource{Field: field, Source: SourceBot})
	}

	applyString(FieldTimezone, group.Timezone, &local.Timezone)
	applyString(FieldLanguage, group.Language, &local.Language)
	applyBool(FieldReasoningEnabled, group.ReasoningEnabled, &local.ReasoningEnabled)
	applyString(FieldReasoningEffort, group.ReasoningEffort, &local.ReasoningEffort)
	applyUUID(FieldChatModelID, group.ChatModelID, &local.ChatModelID)
	applyUUID(FieldSearchProviderID, group.SearchProviderID, &local.SearchProviderID)
	applyUUID(FieldMemoryProviderID, group.MemoryProviderID, &local.MemoryProviderID)
	applyBool(FieldHeartbeatEnabled, group.HeartbeatEnabled, &local.HeartbeatEnabled)
	applyInt(FieldHeartbeatInterval, group.HeartbeatInterval, &local.HeartbeatInterval)
	applyString(FieldHeartbeatPrompt, group.HeartbeatPrompt, &local.HeartbeatPrompt)
	applyUUID(FieldHeartbeatModelID, group.HeartbeatModelID, &local.HeartbeatModelID)
	applyBool(FieldCompactionEnabled, group.CompactionEnabled, &local.CompactionEnabled)
	applyInt(FieldCompactionThreshold, group.CompactionThreshold, &local.CompactionThreshold)
	applyInt(FieldCompactionRatio, group.CompactionRatio, &local.CompactionRatio)
	applyUUID(FieldCompactionModelID, group.CompactionModelID, &local.CompactionModelID)
	applyUUID(FieldTitleModelID, group.TitleModelID, &local.TitleModelID)
	applyUUID(FieldImageModelID, group.ImageModelID, &local.ImageModelID)
	applyUUID(FieldDiscussProbeModelID, group.DiscussProbeModelID, &local.DiscussProbeModelID)
	applyUUID(FieldTTSModelID, group.TtsModelID, &local.TtsModelID)
	applyUUID(FieldTranscriptionModelID, group.TranscriptionModelID, &local.TranscriptionModelID)
	applyUUID(FieldBrowserContextID, group.BrowserContextID, &local.BrowserContextID)
	applyBool(FieldPersistFullToolResults, group.PersistFullToolResults, &local.PersistFullToolResults)
	applyBool(FieldShowToolCallsInIM, group.ShowToolCallsInIm, &local.ShowToolCallsInIM)
	applyJSON(FieldToolApprovalConfig, group.ToolApprovalConfig, func(raw []byte) {
		local.ToolApprovalConfig = parseToolApprovalConfig(raw)
	})
	applyString(FieldOverlayProvider, group.OverlayProvider, &local.OverlayProvider)
	applyBool(FieldOverlayEnabled, group.OverlayEnabled, &local.OverlayEnabled)
	applyJSON(FieldOverlayConfig, group.OverlayConfig, func(raw []byte) {
		local.OverlayConfig = normalizeJSONObject(raw)
	})

	local.Sources = sources
	return local
}

func applySystemFallbackSources(local Settings, mask OverrideMask) Settings {
	local.OverrideMask = mask
	sources := make([]FieldSource, 0, len(InheritableFields))
	for _, field := range InheritableFields {
		source := SourceSystem
		if overrideEnabled(mask, field) {
			source = SourceBot
		}
		sources = append(sources, FieldSource{Field: field, Source: source})
	}
	local.Sources = sources
	return local
}
