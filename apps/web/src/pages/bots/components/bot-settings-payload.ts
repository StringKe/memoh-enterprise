import type { BotSettings } from "@stringke/sdk/connect";

export const FIELD_CHAT_MODEL_ID = "chat_model_id";
export const FIELD_TITLE_MODEL_ID = "title_model_id";
export const FIELD_IMAGE_MODEL_ID = "image_model_id";
export const FIELD_SEARCH_PROVIDER_ID = "search_provider_id";
export const FIELD_MEMORY_PROVIDER_ID = "memory_provider_id";
export const FIELD_TTS_MODEL_ID = "tts_model_id";
export const FIELD_TRANSCRIPTION_MODEL_ID = "transcription_model_id";
export const FIELD_BROWSER_CONTEXT_ID = "browser_context_id";
export const FIELD_LANGUAGE = "language";
export const FIELD_REASONING_ENABLED = "reasoning_enabled";
export const FIELD_REASONING_EFFORT = "reasoning_effort";
export const FIELD_SHOW_TOOL_CALLS_IN_IM = "show_tool_calls_in_im";

export interface BotSettingsFormPayload {
  chat_model_id: string;
  title_model_id: string;
  image_model_id: string;
  search_provider_id: string;
  memory_provider_id: string;
  tts_model_id: string;
  transcription_model_id: string;
  browser_context_id: string;
  language: string;
  reasoning_enabled: boolean;
  reasoning_effort: string;
  show_tool_calls_in_im: boolean;
}

export interface InitialBotSettingsSelection {
  chat_model_id: string;
  memory_provider_id: string;
}

export function buildRestoreBotSettingsInheritanceRequest(
  botId: string,
  fields: readonly string[],
) {
  return {
    botId,
    fields: [...fields],
  };
}

export function settingsFormToProto(payload: BotSettingsFormPayload): Partial<BotSettings> {
  return {
    chatModelId: payload.chat_model_id,
    titleModelId: payload.title_model_id,
    imageModelId: payload.image_model_id,
    searchProviderId: payload.search_provider_id,
    memoryProviderId: payload.memory_provider_id,
    ttsModelId: payload.tts_model_id,
    transcriptionModelId: payload.transcription_model_id,
    browserContextId: payload.browser_context_id,
    language: payload.language,
    reasoningEnabled: payload.reasoning_enabled,
    reasoningEffort: payload.reasoning_effort,
    showToolCallsInIm: payload.show_tool_calls_in_im,
  };
}

export function buildInitialBotSettingsUpdateRequest(
  botId: string,
  selection: InitialBotSettingsSelection,
) {
  const settings: Partial<BotSettings> = {};
  const fields: Record<string, boolean> = {};

  if (selection.chat_model_id) {
    settings.chatModelId = selection.chat_model_id;
    fields[FIELD_CHAT_MODEL_ID] = true;
  }
  if (selection.memory_provider_id) {
    settings.memoryProviderId = selection.memory_provider_id;
    fields[FIELD_MEMORY_PROVIDER_ID] = true;
  }

  if (Object.keys(fields).length === 0) {
    return null;
  }

  return {
    botId,
    settings,
    overrideMask: { fields },
  };
}
