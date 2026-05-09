<template>
  <div class="space-y-5">
    <div class="grid gap-4 md:grid-cols-2">
      <div class="space-y-2">
        <Label>{{ $t("bots.settings.chatModel") }}</Label>
        <ModelSelect
          v-model="form.chat_model_id"
          :models="models"
          :providers="providers"
          model-type="chat"
          :placeholder="$t('common.none')"
        />
      </div>

      <div class="space-y-2">
        <Label>{{ $t("bots.settings.titleModel") }}</Label>
        <ModelSelect
          v-model="form.title_model_id"
          :models="models"
          :providers="providers"
          model-type="chat"
          :placeholder="$t('common.none')"
        />
      </div>

      <div class="space-y-2">
        <Label>{{ $t("bots.settings.memoryProvider") }}</Label>
        <MemoryProviderSelect
          v-model="form.memory_provider_id"
          :providers="memoryProviders"
          :placeholder="$t('common.none')"
        />
      </div>

      <div class="space-y-2">
        <Label>{{ $t("bots.settings.searchProvider") }}</Label>
        <SearchProviderSelect
          v-model="form.search_provider_id"
          :providers="searchProviders"
          :placeholder="$t('common.none')"
        />
      </div>

      <div class="space-y-2">
        <Label>{{ $t("bots.settings.ttsModel") }}</Label>
        <TtsModelSelect
          v-model="form.tts_model_id"
          :models="ttsModels"
          :providers="ttsProviders"
          :placeholder="$t('common.none')"
        />
      </div>

      <div class="space-y-2">
        <Label>{{ $t("bots.settings.transcriptionModel") }}</Label>
        <TtsModelSelect
          v-model="form.transcription_model_id"
          :models="transcriptionModels"
          :providers="transcriptionProviders"
          :placeholder="$t('common.none')"
        />
      </div>

      <div class="space-y-2">
        <Label>{{ $t("bots.settings.language") }}</Label>
        <Input v-model="form.language" />
      </div>
    </div>

    <div class="space-y-2">
      <Label>{{ $t("bots.settings.reasoningEffort") }}</Label>
      <div class="flex gap-2">
        <Switch
          :model-value="form.reasoning_enabled"
          @update:model-value="(value) => (form.reasoning_enabled = !!value)"
        />
        <Select v-model="form.reasoning_effort" :disabled="!form.reasoning_enabled">
          <SelectTrigger class="w-44">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="low">{{ $t("bots.settings.reasoningEffortLow") }}</SelectItem>
            <SelectItem value="medium">{{ $t("bots.settings.reasoningEffortMedium") }}</SelectItem>
            <SelectItem value="high">{{ $t("bots.settings.reasoningEffortHigh") }}</SelectItem>
            <SelectItem value="xhigh">{{ $t("bots.settings.reasoningEffortXHigh") }}</SelectItem>
          </SelectContent>
        </Select>
      </div>
    </div>

    <div class="flex items-center justify-between gap-4">
      <div class="space-y-1">
        <Label>{{ $t("bots.settings.showToolCallsInIM") }}</Label>
        <p class="text-xs text-muted-foreground">
          {{ $t("bots.settings.showToolCallsInIMDescription") }}
        </p>
      </div>
      <Switch
        :model-value="form.show_tool_calls_in_im"
        @update:model-value="(value) => (form.show_tool_calls_in_im = !!value)"
      />
    </div>

    <div class="flex justify-end gap-2">
      <Button variant="outline" :disabled="resetting" @click="handleReset">
        <Spinner v-if="resetting" class="mr-1.5" />
        {{ $t("botGroups.resetSettings") }}
      </Button>
      <Button :disabled="saving" @click="handleSave">
        <Spinner v-if="saving" class="mr-1.5" />
        {{ $t("bots.settings.save") }}
      </Button>
    </div>
  </div>
</template>

<script setup lang="ts">
import {
  Button,
  Input,
  Label,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Spinner,
  Switch,
} from "@stringke/ui";
import { computed, reactive, watch } from "vue";
import { toast } from "vue-sonner";
import { useI18n } from "vue-i18n";
import { useMutation, useQuery, useQueryCache } from "@pinia/colada";
import type { BotSettings, SpeechModel, SpeechProvider } from "@stringke/sdk/connect";
import { connectClients } from "@/lib/connect-client";
import { resolveApiErrorMessage } from "@/utils/api-error";
import ModelSelect from "@/pages/bots/components/model-select.vue";
import MemoryProviderSelect from "@/pages/bots/components/memory-provider-select.vue";
import SearchProviderSelect from "@/pages/bots/components/search-provider-select.vue";
import TtsModelSelect from "@/pages/bots/components/tts-model-select.vue";

const props = defineProps<{
  groupId: string;
}>();

const { t: $t } = useI18n();
const queryCache = useQueryCache();

const { data: settingsData } = useQuery({
  key: () => ["bot-group-settings", props.groupId],
  query: async () =>
    (await connectClients.botGroups.getBotGroupSettings({ groupId: props.groupId })).settings,
  enabled: () => !!props.groupId,
});

const { data: modelData } = useQuery({
  key: ["models"],
  query: async () => (await connectClients.models.listModels({})).models,
});

const { data: providerData } = useQuery({
  key: ["providers"],
  query: async () => (await connectClients.providers.listProviders({})).providers,
});

const { data: searchProviderData } = useQuery({
  key: ["search-providers"],
  query: async () => (await connectClients.searchProviders.listSearchProviders({})).providers,
});

const { data: memoryProviderData } = useQuery({
  key: ["memory-providers"],
  query: async () => (await connectClients.memoryProviders.listMemoryProviders({})).providers,
});

const { data: speechProviderData } = useQuery({
  key: ["speech-providers"],
  query: async () => (await connectClients.speech.listSpeechProviders({})).providers,
});

const { data: speechModelData } = useQuery({
  key: ["speech-models"],
  query: async () => (await connectClients.speech.listSpeechModels({})).models,
});

const { data: transcriptionProviderData } = useQuery({
  key: ["transcription-providers"],
  query: async () => (await connectClients.speech.listTranscriptionProviders({})).providers,
});

const { data: transcriptionModelData } = useQuery({
  key: ["transcription-models"],
  query: async () => (await connectClients.speech.listTranscriptionModels({})).models,
});

const form = reactive({
  chat_model_id: "",
  title_model_id: "",
  search_provider_id: "",
  memory_provider_id: "",
  tts_model_id: "",
  transcription_model_id: "",
  language: "",
  reasoning_enabled: false,
  reasoning_effort: "medium",
  show_tool_calls_in_im: false,
});

const models = computed(() => modelData.value ?? []);
const providers = computed(() => providerData.value ?? []);
const searchProviders = computed(() => (searchProviderData.value ?? []).filter((p) => p.enabled));
const memoryProviders = computed(() => memoryProviderData.value ?? []);
const ttsProviders = computed(() =>
  (speechProviderData.value ?? []).filter((p) => p.enabled).map(speechProviderToSelectOption),
);
const transcriptionProviders = computed(() =>
  (transcriptionProviderData.value ?? [])
    .filter((p) => p.enabled)
    .map(speechProviderToSelectOption),
);
const ttsProviderIds = computed(() => new Set(ttsProviders.value.map((provider) => provider.id)));
const transcriptionProviderIds = computed(
  () => new Set(transcriptionProviders.value.map((provider) => provider.id)),
);
const ttsModels = computed(() =>
  (speechModelData.value ?? [])
    .filter((model) => ttsProviderIds.value.has(model.providerId))
    .map(speechModelToSelectOption),
);
const transcriptionModels = computed(() =>
  (transcriptionModelData.value ?? [])
    .filter((model) => transcriptionProviderIds.value.has(model.providerId))
    .map(speechModelToSelectOption),
);

const { mutateAsync: saveSettings, isLoading: saving } = useMutation({
  mutation: async (settings: Partial<BotSettings>) => {
    const response = await connectClients.botGroups.updateBotGroupSettings({
      groupId: props.groupId,
      settings,
    });
    return response.settings;
  },
  onSettled: () => queryCache.invalidateQueries({ key: ["bot-group-settings", props.groupId] }),
});

const { mutateAsync: resetSettings, isLoading: resetting } = useMutation({
  mutation: async () => connectClients.botGroups.deleteBotGroupSettings({ groupId: props.groupId }),
  onSettled: () => queryCache.invalidateQueries({ key: ["bot-group-settings", props.groupId] }),
});

watch(
  settingsData,
  (settings) => {
    form.chat_model_id = settings?.chatModelId ?? "";
    form.title_model_id = settings?.titleModelId ?? "";
    form.search_provider_id = settings?.searchProviderId ?? "";
    form.memory_provider_id = settings?.memoryProviderId ?? "";
    form.tts_model_id = settings?.ttsModelId ?? "";
    form.transcription_model_id = settings?.transcriptionModelId ?? "";
    form.language = settings?.language ?? "";
    form.reasoning_enabled = settings?.reasoningEnabled ?? false;
    form.reasoning_effort = settings?.reasoningEffort || "medium";
    form.show_tool_calls_in_im = settings?.showToolCallsInIm ?? false;
  },
  { immediate: true },
);

async function handleSave() {
  try {
    await saveSettings({
      chatModelId: form.chat_model_id,
      titleModelId: form.title_model_id,
      searchProviderId: form.search_provider_id,
      memoryProviderId: form.memory_provider_id,
      ttsModelId: form.tts_model_id,
      transcriptionModelId: form.transcription_model_id,
      language: form.language,
      reasoningEnabled: form.reasoning_enabled,
      reasoningEffort: form.reasoning_effort,
      showToolCallsInIm: form.show_tool_calls_in_im,
    });
    toast.success($t("bots.settings.saveSuccess"));
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, $t("common.saveFailed")));
  }
}

async function handleReset() {
  try {
    await resetSettings();
    toast.success($t("botGroups.resetSettingsSuccess"));
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, $t("botGroups.resetSettingsFailed")));
  }
}

function speechProviderToSelectOption(provider: SpeechProvider) {
  return {
    id: provider.id,
    name: provider.name,
    client_type: provider.type,
  };
}

function speechModelToSelectOption(model: SpeechModel) {
  return {
    id: model.id,
    model_id: model.modelId,
    name: model.displayName || model.modelId,
    provider_id: model.providerId,
  };
}
</script>
