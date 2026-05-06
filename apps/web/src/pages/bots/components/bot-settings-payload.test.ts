import { describe, expect, it } from "vitest";

import {
  FIELD_CHAT_MODEL_ID,
  FIELD_MEMORY_PROVIDER_ID,
  FIELD_REASONING_EFFORT,
  FIELD_REASONING_ENABLED,
  buildInitialBotSettingsUpdateRequest,
  buildRestoreBotSettingsInheritanceRequest,
  settingsFormToProto,
} from "./bot-settings-payload";

describe("buildRestoreBotSettingsInheritanceRequest", () => {
  it("builds the restore inheritance payload with exact field names", () => {
    const fields = [FIELD_REASONING_ENABLED, FIELD_REASONING_EFFORT];

    const payload = buildRestoreBotSettingsInheritanceRequest("bot-1", fields);

    expect(payload).toEqual({
      botId: "bot-1",
      fields: ["reasoning_enabled", "reasoning_effort"],
    });
  });

  it("copies the field list so callers cannot mutate the request", () => {
    const fields = [FIELD_CHAT_MODEL_ID];

    const payload = buildRestoreBotSettingsInheritanceRequest("bot-1", fields);
    fields.push(FIELD_MEMORY_PROVIDER_ID);

    expect(payload.fields).toEqual(["chat_model_id"]);
  });
});

describe("buildInitialBotSettingsUpdateRequest", () => {
  it("builds create-bot settings override mask for explicit model and memory selections", () => {
    expect(
      buildInitialBotSettingsUpdateRequest("bot-1", {
        chat_model_id: "model-1",
        memory_provider_id: "memory-1",
      }),
    ).toEqual({
      botId: "bot-1",
      settings: {
        chatModelId: "model-1",
        memoryProviderId: "memory-1",
      },
      overrideMask: {
        fields: {
          chat_model_id: true,
          memory_provider_id: true,
        },
      },
    });
  });

  it("omits inherited create-bot settings when no explicit selection exists", () => {
    expect(
      buildInitialBotSettingsUpdateRequest("bot-1", {
        chat_model_id: "",
        memory_provider_id: "",
      }),
    ).toBeNull();
  });
});

describe("settingsFormToProto", () => {
  it("maps editable settings form fields to Connect field names", () => {
    expect(
      settingsFormToProto({
        chat_model_id: "chat-1",
        title_model_id: "title-1",
        image_model_id: "image-1",
        search_provider_id: "search-1",
        memory_provider_id: "memory-1",
        tts_model_id: "tts-1",
        transcription_model_id: "transcription-1",
        browser_context_id: "browser-1",
        language: "zh-CN",
        reasoning_enabled: true,
        reasoning_effort: "high",
        show_tool_calls_in_im: true,
      }),
    ).toEqual({
      chatModelId: "chat-1",
      titleModelId: "title-1",
      imageModelId: "image-1",
      searchProviderId: "search-1",
      memoryProviderId: "memory-1",
      ttsModelId: "tts-1",
      transcriptionModelId: "transcription-1",
      browserContextId: "browser-1",
      language: "zh-CN",
      reasoningEnabled: true,
      reasoningEffort: "high",
      showToolCallsInIm: true,
    });
  });
});
