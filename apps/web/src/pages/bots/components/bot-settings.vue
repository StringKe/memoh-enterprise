<template>
  <div class="max-w-2xl mx-auto space-y-6">
    <!-- Chat Model -->
    <div class="space-y-2">
      <Label>{{ $t("bots.settings.chatModel") }}</Label>
      <ModelSelect
        v-model="form.chat_model_id"
        :models="models"
        :providers="providers"
        model-type="chat"
        :placeholder="$t('bots.settings.chatModel')"
      />
      <InheritanceField
        :fields="[FIELD_CHAT_MODEL_ID]"
        :sources="effectiveSettings?.sources"
        :loading="isRestoringInheritance"
        @restore="handleRestoreInheritance([FIELD_CHAT_MODEL_ID])"
      />
    </div>

    <!-- Title Model -->
    <div class="space-y-2">
      <Label>{{ $t("bots.settings.titleModel") }}</Label>
      <p class="text-xs text-muted-foreground">
        {{ $t("bots.settings.titleModelDescription") }}
      </p>
      <ModelSelect
        v-model="form.title_model_id"
        :models="models"
        :providers="providers"
        model-type="chat"
        :placeholder="$t('bots.settings.titleModelPlaceholder')"
      />
      <InheritanceField
        :fields="[FIELD_TITLE_MODEL_ID]"
        :sources="effectiveSettings?.sources"
        :loading="isRestoringInheritance"
        @restore="handleRestoreInheritance([FIELD_TITLE_MODEL_ID])"
      />
    </div>

    <!-- Memory Provider -->
    <div class="space-y-2">
      <Label>{{ $t("bots.settings.memoryProvider") }}</Label>
      <MemoryProviderSelect
        v-model="form.memory_provider_id"
        :providers="memoryProviders"
        :placeholder="$t('bots.settings.memoryProviderPlaceholder')"
      />
      <InheritanceField
        :fields="[FIELD_MEMORY_PROVIDER_ID]"
        :sources="effectiveSettings?.sources"
        :loading="isRestoringInheritance"
        @restore="handleRestoreInheritance([FIELD_MEMORY_PROVIDER_ID])"
      />
      <div
        v-if="selectedBuiltinMemoryProvider"
        class="rounded-md border border-border bg-card px-3 py-2 text-xs text-muted-foreground"
      >
        {{
          $t("bots.settings.memoryModePreview", {
            mode: $t(`memory.modeNames.${selectedBuiltinMemoryMode}`),
          })
        }}
      </div>
      <div
        v-if="showMemoryProviderStatusCard"
        class="rounded-lg border border-border bg-card p-4 space-y-4"
      >
        <div class="flex items-start justify-between gap-3">
          <div class="space-y-1">
            <p class="text-xs font-medium text-foreground">
              {{ indexedMemoryStatusTitle }}
            </p>
            <p class="text-xs text-muted-foreground">
              {{
                isSelectedMemoryProviderPersisted
                  ? indexedMemoryStatusHint
                  : $t("bots.settings.indexedMemoryStatusPendingSave")
              }}
            </p>
          </div>
          <Button
            variant="outline"
            size="sm"
            :disabled="
              !isSelectedMemoryProviderPersisted || isRebuilding || !memoryStatus?.can_manual_sync
            "
            @click="handleMemorySync"
          >
            <Spinner v-if="isRebuilding" class="mr-1.5" />
            {{ $t("bots.settings.memorySyncAction") }}
          </Button>
        </div>

        <div v-if="isMemoryStatusLoading" class="text-xs text-muted-foreground">
          {{ $t("common.loading") }}
        </div>

        <div v-else-if="statusCardData" class="grid gap-3 md:grid-cols-2">
          <div class="rounded-md border border-border bg-background/60 px-3 py-2">
            <p class="text-xs text-muted-foreground">
              {{ $t("bots.settings.memorySourceDir") }}
            </p>
            <p class="mt-1 text-xs font-medium text-foreground break-all">
              {{ statusCardData.source_dir || "-" }}
            </p>
          </div>
          <div class="rounded-md border border-border bg-background/60 px-3 py-2">
            <p class="text-xs text-muted-foreground">
              {{ $t("bots.settings.memoryOverviewPath") }}
            </p>
            <p class="mt-1 text-xs font-medium text-foreground break-all">
              {{ statusCardData.overview_path || "-" }}
            </p>
          </div>
          <div class="rounded-md border border-border bg-background/60 px-3 py-2">
            <p class="text-xs text-muted-foreground">
              {{ $t("bots.settings.memoryMarkdownFiles") }}
            </p>
            <p class="mt-1 text-xs font-medium text-foreground">
              {{ statusCardData.markdown_file_count ?? 0 }}
            </p>
          </div>
          <div class="rounded-md border border-border bg-background/60 px-3 py-2">
            <p class="text-xs text-muted-foreground">
              {{ $t("bots.settings.memorySourceEntries") }}
            </p>
            <p class="mt-1 text-xs font-medium text-foreground">
              {{ statusCardData.source_count ?? 0 }}
            </p>
          </div>
          <div class="rounded-md border border-border bg-background/60 px-3 py-2">
            <p class="text-xs text-muted-foreground">
              {{ $t("bots.settings.memoryIndexedEntries") }}
            </p>
            <p class="mt-1 text-xs font-medium text-foreground">
              {{ statusCardData.indexed_count ?? 0 }}
            </p>
          </div>
          <div
            v-if="showQdrantDetails"
            class="rounded-md border border-border bg-background/60 px-3 py-2"
          >
            <p class="text-xs text-muted-foreground">
              {{ $t("bots.settings.memoryQdrantCollection") }}
            </p>
            <p class="mt-1 text-xs font-medium text-foreground break-all">
              {{ statusCardData.qdrant_collection || "-" }}
            </p>
          </div>
          <div
            v-if="showEncoderHealth"
            class="rounded-md border border-border bg-background/60 px-3 py-2"
          >
            <p class="text-xs text-muted-foreground">
              {{ encoderHealthLabel }}
            </p>
            <p
              class="mt-1 text-xs font-medium"
              :class="healthTextClass(statusCardData.encoder?.ok)"
            >
              {{ healthLabel(statusCardData.encoder?.ok, statusCardData.encoder?.error) }}
            </p>
          </div>
          <div
            v-if="showQdrantHealth"
            class="rounded-md border border-border bg-background/60 px-3 py-2"
          >
            <p class="text-xs text-muted-foreground">
              {{ $t("bots.settings.memoryQdrantHealth") }}
            </p>
            <p class="mt-1 text-xs font-medium" :class="healthTextClass(statusCardData.qdrant?.ok)">
              {{ healthLabel(statusCardData.qdrant?.ok, statusCardData.qdrant?.error) }}
            </p>
          </div>
        </div>
      </div>
    </div>

    <!-- Search Provider -->
    <div class="space-y-2">
      <Label>{{ $t("bots.settings.searchProvider") }}</Label>
      <SearchProviderSelect
        v-model="form.search_provider_id"
        :providers="searchProviders"
        :placeholder="$t('bots.settings.searchProviderPlaceholder')"
      />
      <InheritanceField
        :fields="[FIELD_SEARCH_PROVIDER_ID]"
        :sources="effectiveSettings?.sources"
        :loading="isRestoringInheritance"
        @restore="handleRestoreInheritance([FIELD_SEARCH_PROVIDER_ID])"
      />
    </div>

    <!-- TTS Model -->
    <div class="space-y-2">
      <Label>{{ $t("bots.settings.ttsModel") }}</Label>
      <TtsModelSelect
        v-model="form.tts_model_id"
        :models="ttsModels"
        :providers="ttsProviders"
        :placeholder="$t('bots.settings.ttsModelPlaceholder')"
      />
      <InheritanceField
        :fields="[FIELD_TTS_MODEL_ID]"
        :sources="effectiveSettings?.sources"
        :loading="isRestoringInheritance"
        @restore="handleRestoreInheritance([FIELD_TTS_MODEL_ID])"
      />
    </div>

    <!-- Transcription Model -->
    <div class="space-y-2">
      <Label>{{ $t("bots.settings.transcriptionModel") }}</Label>
      <TtsModelSelect
        v-model="form.transcription_model_id"
        :models="transcriptionModels"
        :providers="ttsProviders"
        :placeholder="$t('bots.settings.transcriptionModelPlaceholder')"
      />
      <InheritanceField
        :fields="[FIELD_TRANSCRIPTION_MODEL_ID]"
        :sources="effectiveSettings?.sources"
        :loading="isRestoringInheritance"
        @restore="handleRestoreInheritance([FIELD_TRANSCRIPTION_MODEL_ID])"
      />
    </div>

    <!-- Image Generation Model -->
    <div class="space-y-2">
      <Label>{{ $t("bots.settings.imageModel") }}</Label>
      <p class="text-xs text-muted-foreground">
        {{ $t("bots.settings.imageModelDescription") }}
      </p>
      <ModelSelect
        v-model="form.image_model_id"
        :models="imageCapableModels"
        :providers="providers"
        model-type="chat"
        :placeholder="$t('bots.settings.imageModelPlaceholder')"
      />
      <InheritanceField
        :fields="[FIELD_IMAGE_MODEL_ID]"
        :sources="effectiveSettings?.sources"
        :loading="isRestoringInheritance"
        @restore="handleRestoreInheritance([FIELD_IMAGE_MODEL_ID])"
      />
    </div>

    <!-- Timezone -->
    <div class="space-y-2">
      <Label>{{ $t("bots.timezone") }}</Label>
      <TimezoneSelect
        :model-value="form.timezone || emptyTimezoneValue"
        :placeholder="$t('bots.timezonePlaceholder')"
        allow-empty
        :empty-label="$t('bots.timezoneInherited')"
        @update:model-value="
          (val: string) => (form.timezone = val === emptyTimezoneValue ? '' : val)
        "
      />
    </div>

    <Separator />
    <!-- Language -->
    <div class="space-y-2">
      <Label>{{ $t("bots.settings.language") }}</Label>
      <Input v-model="form.language" type="text" :aria-label="$t('bots.settings.language')" />
      <InheritanceField
        :fields="[FIELD_LANGUAGE]"
        :sources="effectiveSettings?.sources"
        :loading="isRestoringInheritance"
        @restore="handleRestoreInheritance([FIELD_LANGUAGE])"
      />
    </div>

    <!-- Timezone -->
    <div class="space-y-2">
      <Label>{{ $t("bots.timezone") }}</Label>
      <TimezoneSelect
        :model-value="form.timezone || emptyTimezoneValue"
        :placeholder="$t('bots.timezonePlaceholder')"
        allow-empty
        :empty-label="$t('bots.timezoneInherited')"
        @update:model-value="
          (val: string) => (form.timezone = val === emptyTimezoneValue ? '' : val)
        "
      />
    </div>

    <Separator />

    <!-- Reasoning -->
    <div class="space-y-2">
      <Label>{{ $t("bots.settings.reasoningEffort") }}</Label>
      <Popover v-model:open="reasoningPopoverOpen">
        <PopoverTrigger as-child>
          <Button
            variant="outline"
            role="combobox"
            :disabled="!chatModelSupportsReasoning"
            class="w-full justify-between font-normal"
          >
            <span class="flex items-center gap-2">
              <Lightbulb
                class="size-3.5"
                :style="{ opacity: EFFORT_OPACITY[reasoningFormValue] ?? 0.5 }"
              />
              {{
                reasoningFormValue === "off"
                  ? $t("chat.reasoningOff")
                  : $t(EFFORT_LABELS[reasoningFormValue] ?? reasoningFormValue)
              }}
            </span>
            <ChevronDown class="size-3.5 shrink-0 text-muted-foreground" />
          </Button>
        </PopoverTrigger>
        <PopoverContent class="w-[--reka-popover-trigger-width] p-0" align="start">
          <ReasoningEffortSelect
            v-model="reasoningFormValue"
            :efforts="availableReasoningEfforts"
            @update:model-value="reasoningPopoverOpen = false"
          />
        </PopoverContent>
      </Popover>
      <InheritanceField
        :fields="[FIELD_REASONING_ENABLED, FIELD_REASONING_EFFORT]"
        :sources="effectiveSettings?.sources"
        :loading="isRestoringInheritance"
        @restore="handleRestoreInheritance([FIELD_REASONING_ENABLED, FIELD_REASONING_EFFORT])"
      />
    </div>

    <!-- Show Tool Calls in IM -->
    <div class="flex items-center justify-between">
      <div class="space-y-1">
        <Label>{{ $t("bots.settings.showToolCallsInIM") }}</Label>
        <p class="text-xs text-muted-foreground">
          {{ $t("bots.settings.showToolCallsInIMDescription") }}
        </p>
      </div>
      <Switch
        :model-value="form.show_tool_calls_in_im"
        @update:model-value="(val) => (form.show_tool_calls_in_im = !!val)"
      />
    </div>
    <InheritanceField
      :fields="[FIELD_SHOW_TOOL_CALLS_IN_IM]"
      :sources="effectiveSettings?.sources"
      :loading="isRestoringInheritance"
      @restore="handleRestoreInheritance([FIELD_SHOW_TOOL_CALLS_IN_IM])"
    />

    <!-- Save -->
    <div class="flex justify-end">
      <Button :disabled="!hasChanges || saveLoading" @click="handleSave">
        <Spinner v-if="saveLoading" />
        {{ $t("bots.settings.save") }}
      </Button>
    </div>

    <Separator />

    <!-- Danger Zone -->
    <div class="rounded-lg border border-destructive/50 bg-destructive/5 p-4 space-y-3">
      <h3 class="text-xs font-semibold text-destructive">
        {{ $t("bots.settings.dangerZone") }}
      </h3>
      <p class="text-xs text-muted-foreground">
        {{ $t("bots.settings.deleteBotDescription") }}
      </p>
      <div class="flex items-center justify-end">
        <ConfirmPopover
          :message="$t('bots.deleteConfirm')"
          :loading="deleteLoading"
          :confirm-text="$t('common.delete')"
          @confirm="handleDeleteBot"
        >
          <template #trigger>
            <Button variant="destructive" :disabled="deleteLoading">
              <Spinner v-if="deleteLoading" class="mr-1.5" />
              {{ $t("bots.settings.deleteBot") }}
            </Button>
          </template>
        </ConfirmPopover>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import {
  Label,
  Input,
  Button,
  Separator,
  Spinner,
  Popover,
  PopoverTrigger,
  PopoverContent,
  Switch,
} from "@stringke/ui";
import { Lightbulb, ChevronDown } from "lucide-vue-next";
import { reactive, computed, ref, watch } from "vue";
import { useRouter } from "vue-router";
import { toast } from "vue-sonner";
import { useI18n } from "vue-i18n";
import ConfirmPopover from "@/components/confirm-popover/index.vue";
import TimezoneSelect from "@/components/timezone-select/index.vue";
import ModelSelect from "./model-select.vue";
import ReasoningEffortSelect from "./reasoning-effort-select.vue";
import { EFFORT_LABELS, EFFORT_OPACITY } from "./reasoning-effort";
import SearchProviderSelect from "./search-provider-select.vue";
import MemoryProviderSelect from "./memory-provider-select.vue";
import TtsModelSelect from "./tts-model-select.vue";
import InheritanceField from "./inheritance-field.vue";
import {
  FIELD_CHAT_MODEL_ID,
  FIELD_IMAGE_MODEL_ID,
  FIELD_LANGUAGE,
  FIELD_MEMORY_PROVIDER_ID,
  FIELD_REASONING_EFFORT,
  FIELD_REASONING_ENABLED,
  FIELD_SEARCH_PROVIDER_ID,
  FIELD_SHOW_TOOL_CALLS_IN_IM,
  FIELD_TITLE_MODEL_ID,
  FIELD_TRANSCRIPTION_MODEL_ID,
  FIELD_TTS_MODEL_ID,
  buildRestoreBotSettingsInheritanceRequest,
  settingsFormToProto,
} from "./bot-settings-payload";
import { useQuery, useMutation, useQueryCache } from "@pinia/colada";
import type {
  BotSettings,
  BotMemoryStatus,
  RebuildBotMemoryResult,
  SpeechModel,
  SpeechProvider,
} from "@stringke/sdk/connect";
import type { JsonObject } from "@bufbuild/protobuf";
import { connectClients } from "@/lib/connect-client";
import type { Ref } from "vue";
import { resolveApiErrorMessage } from "@/utils/api-error";
import { emptyTimezoneValue } from "@/utils/timezones";

const props = defineProps<{
  botId: string;
}>();

const { t } = useI18n();
const router = useRouter();

const botIdRef = computed(() => props.botId) as Ref<string>;

// ---- Data ----
const queryCache = useQueryCache();

const { data: effectiveSettings } = useQuery({
  key: () => ["bot-settings", botIdRef.value],
  query: async () => {
    const response = await connectClients.settings.getBotSettings({ botId: botIdRef.value });
    return response.settings;
  },
  enabled: () => !!botIdRef.value,
});

const settings = computed(() => effectiveSettings.value?.settings);

const { data: bot } = useQuery({
  key: () => ["bot", botIdRef.value],
  query: async () => (await connectClients.bots.getBot({ id: botIdRef.value })).bot,
  enabled: () => !!botIdRef.value,
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

const { data: ttsProviderData } = useQuery({
  key: ["speech-providers"],
  query: async () => (await connectClients.speech.listSpeechProviders({})).providers,
});

const { data: ttsModelData } = useQuery({
  key: ["speech-models"],
  query: async () => (await connectClients.speech.listSpeechModels({})).models,
});

const { data: transcriptionModelData } = useQuery({
  key: ["transcription-models"],
  query: async () => (await connectClients.speech.listTranscriptionModels({})).models,
});

const { data: transcriptionProviderData } = useQuery({
  key: ["transcription-providers"],
  query: async () => (await connectClients.speech.listTranscriptionProviders({})).providers,
});

const { mutateAsync: updateSettings, isLoading } = useMutation({
  mutation: async (body: Partial<BotSettings>) => {
    const response = await connectClients.settings.updateBotSettings({
      botId: botIdRef.value,
      settings: body,
    });
    return response.settings?.settings;
  },
  onSettled: () => queryCache.invalidateQueries({ key: ["bot-settings", botIdRef.value] }),
});

const { mutateAsync: restoreInheritance, isLoading: isRestoringInheritance } = useMutation({
  mutation: async (fields: string[]) => {
    const response = await connectClients.settings.restoreBotSettingsInheritance(
      buildRestoreBotSettingsInheritanceRequest(botIdRef.value, fields),
    );
    return response.settings;
  },
  onSettled: () => queryCache.invalidateQueries({ key: ["bot-settings", botIdRef.value] }),
});

const { mutateAsync: updateBot, isLoading: isUpdatingBot } = useMutation({
  mutation: async (timezone: string) => {
    const response = await connectClients.bots.updateBot({
      id: botIdRef.value,
      timezone,
    });
    return response.bot;
  },
  onSettled: () => {
    queryCache.invalidateQueries({ key: ["bot", botIdRef.value] });
    queryCache.invalidateQueries({ key: ["bots"] });
  },
});

const { mutateAsync: deleteBot, isLoading: deleteLoading } = useMutation({
  mutation: async () => {
    await connectClients.bots.deleteBot({ id: botIdRef.value });
  },
  onSettled: () => {
    queryCache.invalidateQueries({ key: ["bots"] });
    queryCache.invalidateQueries({ key: ["bot"] });
  },
});

const models = computed(() => modelData.value ?? []);
const providers = computed(() => providerData.value ?? []);
const imageCapableModels = computed(() =>
  models.value.filter((m) => m.modalities.includes("image-output")),
);
const searchProviders = computed(() => (searchProviderData.value ?? []).filter((p) => p.enabled));
const memoryProviders = computed(() => memoryProviderData.value ?? []);
const ttsProviders = computed(() =>
  (ttsProviderData.value ?? []).filter((p) => p.enabled).map(speechProviderToSelectOption),
);
const enabledTtsProviderIds = computed(() => new Set(ttsProviders.value.map((p) => p.id)));
const transcriptionProviders = computed(() =>
  (transcriptionProviderData.value ?? [])
    .filter((p) => p.enabled)
    .map(speechProviderToSelectOption),
);
const enabledTranscriptionProviderIds = computed(
  () => new Set(transcriptionProviders.value.map((p) => p.id)),
);
const ttsModels = computed(() =>
  (ttsModelData.value ?? [])
    .filter((model) => enabledTtsProviderIds.value.has(model.providerId))
    .map(speechModelToSelectOption),
);
const transcriptionModels = computed(() =>
  (transcriptionModelData.value ?? [])
    .filter((model) => enabledTranscriptionProviderIds.value.has(model.providerId))
    .map(speechModelToSelectOption),
);
// ---- Form ----
const form = reactive({
  chat_model_id: "",
  title_model_id: "",
  image_model_id: "",
  search_provider_id: "",
  memory_provider_id: "",
  tts_model_id: "",
  transcription_model_id: "",
  timezone: "",
  language: "",
  reasoning_enabled: false,
  reasoning_effort: "medium",
  show_tool_calls_in_im: false,
});

interface BotMemoryStatusView {
  can_manual_sync: boolean;
  source_dir: string;
  overview_path: string;
  markdown_file_count: number;
  source_count: number;
  indexed_count: number;
  qdrant_collection: string;
  encoder?: { ok: boolean; error: string };
  qdrant?: { ok: boolean; error: string };
}

interface RebuildResultView {
  fs_count: number;
  storage_count: number;
  restored_count: number;
}

const selectedMemoryProvider = computed(() =>
  memoryProviders.value.find((provider) => provider.id === form.memory_provider_id),
);
const selectedMemoryProviderType = computed(() => selectedMemoryProvider.value?.type ?? "");
const selectedBuiltinMemoryProvider = computed(() =>
  selectedMemoryProvider.value?.type === "builtin" ? selectedMemoryProvider.value : null,
);
const selectedMem0MemoryProvider = computed(() =>
  selectedMemoryProvider.value?.type === "mem0" ? selectedMemoryProvider.value : null,
);
const selectedBuiltinMemoryMode = computed(
  () => getStringValue(selectedBuiltinMemoryProvider.value?.config, "memory_mode") || "off",
);
const persistedMemoryProviderID = computed(() => settings.value?.memoryProviderId ?? "");
const isSelectedMemoryProviderPersisted = computed(
  () => !!form.memory_provider_id && form.memory_provider_id === persistedMemoryProviderID.value,
);
const showBuiltinIndexedMemoryStatus = computed(
  () => selectedBuiltinMemoryMode.value === "sparse" || selectedBuiltinMemoryMode.value === "dense",
);
const showMem0MemoryStatus = computed(() => !!selectedMem0MemoryProvider.value);
const showMemoryProviderStatusCard = computed(
  () => showBuiltinIndexedMemoryStatus.value || showMem0MemoryStatus.value,
);
const shouldLoadMemoryStatus = computed(
  () =>
    !!botIdRef.value &&
    showMemoryProviderStatusCard.value &&
    isSelectedMemoryProviderPersisted.value,
);
const indexedMemoryStatusTitle = computed(() =>
  selectedMemoryProviderType.value === "mem0"
    ? t("bots.settings.mem0StatusTitle")
    : selectedBuiltinMemoryMode.value === "dense"
      ? t("bots.settings.denseStatusTitle")
      : t("bots.settings.sparseStatusTitle"),
);
const indexedMemoryStatusHint = computed(() =>
  selectedMemoryProviderType.value === "mem0"
    ? t("bots.settings.mem0StatusHint")
    : selectedBuiltinMemoryMode.value === "dense"
      ? t("bots.settings.denseStatusHint")
      : t("bots.settings.sparseStatusHint"),
);

const chatModelSupportsReasoning = computed(() => {
  if (!form.chat_model_id) return false;
  const m = models.value.find((m) => m.id === form.chat_model_id);
  return !!m?.modalities.includes("reasoning");
});

const availableReasoningEfforts = computed(() => {
  if (!form.chat_model_id) return ["low", "medium", "high"];
  const model = models.value.find((m) => m.id === form.chat_model_id);
  const efforts = getStringArray(model?.reasoning, "reasoning_efforts").filter((effort) =>
    ["none", "low", "medium", "high", "xhigh"].includes(effort),
  );
  return efforts.length > 0 ? efforts : ["low", "medium", "high"];
});

watch(
  availableReasoningEfforts,
  (efforts) => {
    if (!efforts.includes(form.reasoning_effort)) {
      form.reasoning_effort = efforts.includes("medium") ? "medium" : (efforts[0] ?? "medium");
    }
  },
  { immediate: true },
);

const reasoningPopoverOpen = ref(false);

const reasoningFormValue = computed({
  get: () => (form.reasoning_enabled ? form.reasoning_effort : "off"),
  set: (v: string) => {
    if (v === "off") {
      form.reasoning_enabled = false;
    } else {
      form.reasoning_enabled = true;
      form.reasoning_effort = v;
    }
  },
});

const { data: memoryStatusData, isLoading: isMemoryStatusLoading } = useQuery({
  key: () => ["bot-memory-status", botIdRef.value, persistedMemoryProviderID.value],
  query: async () => {
    const response = await connectClients.memory.getBotMemoryStatus({ botId: botIdRef.value });
    return response.status ? botMemoryStatusToView(response.status) : null;
  },
  enabled: () => shouldLoadMemoryStatus.value,
});

const { mutateAsync: rebuildMemory, isLoading: isRebuilding } = useMutation({
  mutation: async () => {
    const response = await connectClients.memory.rebuildBotMemory({
      botId: botIdRef.value,
    });
    return rebuildResultToView(response.result);
  },
  onSettled: () => {
    queryCache.invalidateQueries({
      key: ["bot-memory-status", botIdRef.value, persistedMemoryProviderID.value],
    });
  },
});

const memoryStatus = computed(() => memoryStatusData.value ?? null);
const statusCardData = computed(() => memoryStatus.value);
const showQdrantDetails = computed(
  () => selectedBuiltinMemoryMode.value === "sparse" || selectedBuiltinMemoryMode.value === "dense",
);
const showEncoderHealth = computed(
  () => selectedBuiltinMemoryMode.value === "sparse" || selectedBuiltinMemoryMode.value === "dense",
);
const showQdrantHealth = computed(
  () => selectedBuiltinMemoryMode.value === "sparse" || selectedBuiltinMemoryMode.value === "dense",
);
const encoderHealthLabel = computed(() =>
  selectedBuiltinMemoryMode.value === "dense"
    ? t("bots.settings.memoryDenseEmbeddingHealth")
    : t("bots.settings.memoryEncoderHealth"),
);

watch(
  settings,
  (val) => {
    if (val) {
      form.chat_model_id = val.chatModelId ?? "";
      form.title_model_id = val.titleModelId ?? "";
      form.image_model_id = val.imageModelId ?? "";
      form.search_provider_id = val.searchProviderId ?? "";
      form.memory_provider_id = val.memoryProviderId ?? "";
      form.tts_model_id = val.ttsModelId ?? "";
      form.transcription_model_id = val.transcriptionModelId ?? "";
      form.language = val.language ?? "";
      form.timezone = val.timezone ?? "";
      form.reasoning_enabled = val.reasoningEnabled ?? false;
      form.reasoning_effort = val.reasoningEffort || "medium";
      form.show_tool_calls_in_im = val.showToolCallsInIm ?? false;
    }
  },
  { immediate: true },
);

watch(
  bot,
  (val) => {
    form.timezone = val?.timezone ?? "";
  },
  { immediate: true },
);

const hasSettingsChanges = computed(() => {
  if (!settings.value) return true;
  const s = settings.value;
  return (
    form.chat_model_id !== (s.chatModelId ?? "") ||
    form.title_model_id !== (s.titleModelId ?? "") ||
    form.image_model_id !== (s.imageModelId ?? "") ||
    form.search_provider_id !== (s.searchProviderId ?? "") ||
    form.memory_provider_id !== (s.memoryProviderId ?? "") ||
    form.tts_model_id !== (s.ttsModelId ?? "") ||
    form.transcription_model_id !== (s.transcriptionModelId ?? "") ||
    form.language !== (s.language ?? "") ||
    form.timezone !== (s.timezone ?? "") ||
    form.reasoning_enabled !== (s.reasoningEnabled ?? false) ||
    form.reasoning_effort !== (s.reasoningEffort || "medium") ||
    form.show_tool_calls_in_im !== (s.showToolCallsInIm ?? false)
  );
});

const hasTimezoneChanges = computed(() => form.timezone !== (bot.value?.timezone ?? ""));
const hasChanges = computed(() => hasSettingsChanges.value || hasTimezoneChanges.value);
const saveLoading = computed(() => isLoading.value || isUpdatingBot.value);

async function handleSave() {
  try {
    if (hasSettingsChanges.value) {
      const { timezone: _timezone, ...settingsPayload } = form;
      await updateSettings(settingsFormToProto(settingsPayload));
    }
    if (hasTimezoneChanges.value) {
      await updateBot(form.timezone);
    }
    toast.success(t("bots.settings.saveSuccess"));
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t("common.saveFailed")));
  }
}

async function handleRestoreInheritance(fields: string[]) {
  try {
    await restoreInheritance(fields);
    toast.success(t("bots.settings.inheritance.restoreSuccess"));
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t("bots.settings.inheritance.restoreFailed")));
  }
}

function healthTextClass(ok: boolean | undefined) {
  return ok ? "text-foreground" : "text-destructive";
}

function healthLabel(ok: boolean | undefined, error?: string) {
  if (ok) return t("bots.settings.memoryHealthOk");
  if (error) return error;
  return t("bots.settings.memoryHealthUnavailable");
}

async function handleMemorySync() {
  if (!isSelectedMemoryProviderPersisted.value) {
    toast.error(t("bots.settings.indexedMemoryStatusPendingSave"));
    return;
  }
  try {
    const result = await rebuildMemory();
    toast.success(
      t("bots.settings.memorySyncSuccess", {
        fsCount: result?.fs_count ?? 0,
        restoredCount: result?.restored_count ?? 0,
        storageCount: result?.storage_count ?? 0,
      }),
    );
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t("bots.settings.memorySyncFailed")));
  }
}

async function handleDeleteBot() {
  try {
    await deleteBot();
    await router.push({ name: "bots" });
    toast.success(t("bots.deleteSuccess"));
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t("bots.lifecycle.deleteFailed")));
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

function botMemoryStatusToView(status: BotMemoryStatus): BotMemoryStatusView {
  return {
    can_manual_sync: status.canManualSync,
    source_dir: status.sourceDir,
    overview_path: status.overviewPath,
    markdown_file_count: status.markdownFileCount,
    source_count: status.sourceCount,
    indexed_count: status.indexedCount,
    qdrant_collection: status.qdrantCollection,
    encoder: status.encoder ? { ok: status.encoder.ok, error: status.encoder.error } : undefined,
    qdrant: status.qdrant ? { ok: status.qdrant.ok, error: status.qdrant.error } : undefined,
  };
}

function rebuildResultToView(result: RebuildBotMemoryResult | undefined): RebuildResultView {
  return {
    fs_count: result?.fsCount ?? 0,
    storage_count: result?.storageCount ?? 0,
    restored_count: result?.restoredCount ?? 0,
  };
}

function getStringValue(value: JsonObject | undefined, key: string): string | undefined {
  const item = value?.[key];
  return typeof item === "string" ? item : undefined;
}

function getStringArray(value: JsonObject | undefined, key: string): string[] {
  const item = value?.[key];
  return Array.isArray(item)
    ? item.filter((entry): entry is string => typeof entry === "string")
    : [];
}
</script>
