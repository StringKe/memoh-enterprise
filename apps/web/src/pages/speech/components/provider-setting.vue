<template>
  <SettingsShell width="wide">
    <section class="flex items-center gap-3">
      <span class="flex size-10 shrink-0 items-center justify-center rounded-full bg-muted">
        <ProviderIcon v-if="curProvider?.type" :icon="curProvider.type" size="1.5em">
          <span class="text-xs font-medium text-muted-foreground">
            {{ getInitials(curProvider?.name) }}
          </span>
        </ProviderIcon>
      </span>
      <div class="min-w-0">
        <h2 class="text-sm font-semibold truncate">
          {{ curProvider?.name }}
        </h2>
        <p class="text-xs text-muted-foreground">
          {{ currentMeta?.displayName ?? curProvider?.type }}
        </p>
      </div>
      <div class="ml-auto flex items-center gap-2">
        <span class="text-xs text-muted-foreground">
          {{ $t("common.enable") }}
        </span>
        <Switch
          :model-value="curProvider?.enabled ?? false"
          :disabled="!curProvider?.id || enableLoading"
          @update:model-value="handleToggleEnable"
        />
      </div>
    </section>
    <Separator class="mt-4 mb-6" />

    <form @submit.prevent="handleSaveProvider">
      <div class="grid gap-4 md:grid-cols-2">
        <section class="space-y-2 md:col-span-2">
          <Label for="speech-provider-name">{{ $t("common.name") }}</Label>
          <Input
            id="speech-provider-name"
            v-model="providerName"
            type="text"
            :placeholder="$t('common.namePlaceholder')"
          />
        </section>

        <section
          v-for="field in orderedProviderFields"
          :key="field.key"
          class="space-y-2"
          :class="isWideField(field) ? 'md:col-span-2' : ''"
        >
          <Label
            :for="
              field.type === 'bool' || field.type === 'enum'
                ? undefined
                : `speech-provider-${field.key}`
            "
          >
            {{ field.title || field.key }}
          </Label>
          <p v-if="field.description" class="text-xs text-muted-foreground">
            {{ field.description }}
          </p>
          <div v-if="field.type === 'secret'" class="relative">
            <Input
              :id="`speech-provider-${field.key}`"
              v-model="providerConfig[field.key] as string"
              :type="visibleSecrets[field.key] ? 'text' : 'password'"
              :placeholder="field.example ? String(field.example) : ''"
            />
            <button
              type="button"
              class="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
              @click="visibleSecrets[field.key] = !visibleSecrets[field.key]"
            >
              <component :is="visibleSecrets[field.key] ? EyeOff : Eye" class="size-3.5" />
            </button>
          </div>
          <Switch
            v-else-if="field.type === 'bool'"
            :model-value="!!providerConfig[field.key]"
            @update:model-value="(val) => (providerConfig[field.key] = !!val)"
          />
          <Input
            v-else-if="field.type === 'number'"
            :id="`speech-provider-${field.key}`"
            v-model.number="providerConfig[field.key] as number"
            type="number"
            :placeholder="field.example ? String(field.example) : ''"
          />
          <Select
            v-else-if="field.type === 'enum' && field.enum"
            :model-value="String(providerConfig[field.key] ?? '')"
            @update:model-value="(val) => (providerConfig[field.key] = val)"
          >
            <SelectTrigger>
              <SelectValue :placeholder="field.title || field.key" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem v-for="opt in field.enum" :key="opt" :value="opt">
                {{ opt }}
              </SelectItem>
            </SelectContent>
          </Select>
          <Input
            v-else
            :id="`speech-provider-${field.key}`"
            v-model="providerConfig[field.key] as string"
            type="text"
            :placeholder="field.example ? String(field.example) : ''"
          />
        </section>
      </div>

      <div class="flex justify-end mt-4">
        <LoadingButton type="submit" :loading="saveLoading">
          {{ $t("provider.saveChanges") }}
        </LoadingButton>
      </div>
    </form>

    <Separator class="mt-6 mb-6" />

    <section>
      <div class="flex justify-between items-center mb-4">
        <h3 class="text-xs font-medium">
          {{ $t("speech.synthesis.models") }}
        </h3>
        <div v-if="curProviderId" class="flex items-center gap-2">
          <LoadingButton
            type="button"
            variant="outline"
            size="sm"
            :loading="importLoading"
            @click="handleImportModels"
          >
            {{ $t("speech.importModels") }}
          </LoadingButton>
          <CreateModel
            :id="curProviderId"
            default-type="speech"
            hide-type
            :type-options="speechTypeOptions"
            :invalidate-keys="['speech-provider-models', 'speech-models']"
          />
        </div>
      </div>

      <div
        v-if="providerModels.length === 0"
        class="text-xs text-muted-foreground py-4 text-center"
      >
        {{ $t("speech.noModels") }}
      </div>

      <div
        v-for="model in providerModels"
        :key="model.id"
        class="border border-border rounded-lg mb-4"
      >
        <button
          type="button"
          class="w-full flex items-center justify-between p-3 text-left hover:bg-accent/50 rounded-t-lg transition-colors"
          @click="toggleModel(model.id ?? '')"
        >
          <div>
            <span class="text-xs font-medium">{{ model.displayName || model.modelId }}</span>
            <span v-if="model.displayName" class="text-xs text-muted-foreground ml-2">{{
              model.modelId
            }}</span>
          </div>
          <component
            :is="expandedModelId === model.id ? ChevronUp : ChevronDown"
            class="size-3 text-muted-foreground"
          />
        </button>

        <div
          v-if="expandedModelId === model.id"
          class="px-3 pb-3 space-y-4 border-t border-border pt-3"
        >
          <ModelConfigEditor
            :model-id="model.id ?? ''"
            :model-name="model.modelId ?? ''"
            :config="model.metadata || {}"
            :schema="getModelSchema(model.modelId ?? '')"
            :on-test="(text, cfg) => handleTestModel(model.id ?? '', text as string, cfg)"
            @save="(cfg) => handleSaveModel(model.id ?? '', cfg)"
          />
        </div>
      </div>
    </section>
  </SettingsShell>
</template>

<script setup lang="ts">
import {
  Input,
  Label,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Separator,
  Switch,
} from "@stringke/ui";
import ModelConfigEditor from "./model-config-editor.vue";
import { ChevronDown, ChevronUp, Eye, EyeOff } from "lucide-vue-next";
import { computed, inject, reactive, ref, watch } from "vue";
import { toast } from "vue-sonner";
import { useI18n } from "vue-i18n";
import { useQueryCache } from "@pinia/colada";
import type { JsonObject } from "@bufbuild/protobuf";
import type {
  SpeechProvider,
  SpeechProviderMeta as ConnectSpeechProviderMeta,
} from "@stringke/sdk/connect";
import LoadingButton from "@/components/loading-button/index.vue";
import ProviderIcon from "@/components/provider-icon/index.vue";
import CreateModel from "@/components/create-model/index.vue";
import SettingsShell from "@/components/settings-shell/index.vue";
import { connectClients } from "@/lib/connect-client";
import { useConnectQuery } from "@/lib/connect-colada";

interface SpeechFieldSchema {
  key: string;
  type: string;
  title?: string;
  description?: string;
  required?: boolean;
  advanced?: boolean;
  enum?: string[];
  example?: unknown;
  order?: number;
}

interface SpeechConfigSchema {
  fields?: SpeechFieldSchema[];
}

interface SpeechProviderMetaView {
  type: string;
  displayName: string;
  schema?: SpeechConfigSchema;
}

function getInitials(name: string | undefined) {
  const label = name?.trim() ?? "";
  return label ? label.slice(0, 2).toUpperCase() : "?";
}

const { t } = useI18n();
const curProvider = inject("curTtsProvider", ref<SpeechProvider>());
const curProviderId = computed(() => curProvider.value?.id);
const providerName = ref("");
const providerConfig = reactive<Record<string, unknown>>({});
const visibleSecrets = reactive<Record<string, boolean>>({});
const expandedModelId = ref("");
const enableLoading = ref(false);
const saveLoading = ref(false);
const importLoading = ref(false);
const queryCache = useQueryCache();
const speechTypeOptions = [{ value: "speech", label: "Speech" }];

const { data: providerDetail } = useConnectQuery({
  key: () => ["speech-provider-detail", curProviderId.value],
  query: async () =>
    curProviderId.value
      ? ((await connectClients.speech.getSpeechProvider({ id: curProviderId.value })).provider ??
        null)
      : null,
});

const { data: metaList } = useConnectQuery({
  key: () => ["speech-providers-meta"],
  query: async () => {
    const response = await connectClients.speech.listSpeechProviderMeta({});
    return response.providers.map(mapSpeechProviderMeta);
  },
});

const currentMeta = computed(() => {
  if (!metaList.value || !curProvider.value?.type) return null;
  return metaList.value.find((m) => m.type === curProvider.value?.type) ?? null;
});

const orderedProviderFields = computed(() => {
  const fields = currentMeta.value?.schema?.fields ?? [];
  return [...fields].sort((a, b) => (a.order ?? 0) - (b.order ?? 0));
});

function isWideField(field: SpeechFieldSchema) {
  if (field.type === "secret") return true;
  const key = field.key.toLowerCase();
  if (
    key.includes("url") ||
    key.includes("endpoint") ||
    key.includes("key") ||
    key.includes("token") ||
    key.includes("path") ||
    key.includes("uri")
  )
    return true;
  if ((field.description ?? "").length > 80) return true;
  return false;
}

const { data: providerSpeechModels } = useConnectQuery({
  key: () => ["speech-provider-models", curProviderId.value],
  query: async () =>
    curProviderId.value
      ? (await connectClients.speech.listSpeechModels({ providerId: curProviderId.value })).models
      : [],
});

const providerModels = computed(() => providerSpeechModels.value ?? []);

watch(
  () => providerDetail.value,
  (provider) => {
    providerName.value = provider?.name ?? curProvider.value?.name ?? "";
    Object.keys(providerConfig).forEach((key) => delete providerConfig[key]);
    Object.assign(providerConfig, { ...provider?.config });
  },
  { immediate: true, deep: true },
);

function getModelSchema(_modelID: string): SpeechConfigSchema | null {
  return null;
}

function toggleModel(id: string) {
  expandedModelId.value = expandedModelId.value === id ? "" : id;
}

async function handleToggleEnable(value: boolean) {
  if (!curProviderId.value || !curProvider.value) return;
  const prev = curProvider.value.enabled ?? false;
  curProvider.value = { ...curProvider.value, enabled: value };

  enableLoading.value = true;
  try {
    await connectClients.providers.updateProvider({
      id: curProviderId.value,
      displayName: providerName.value.trim() || curProvider.value.name,
      clientType: curProvider.value.type,
      enabled: value,
      config: sanitizeConfig(providerConfig),
    });
    queryCache.invalidateQueries({ key: ["speech-providers"] });
    queryCache.invalidateQueries({ key: ["speech-provider-detail", curProviderId.value] });
  } catch {
    curProvider.value = { ...curProvider.value, enabled: prev };
    toast.error(t("common.saveFailed"));
  } finally {
    enableLoading.value = false;
  }
}

async function handleSaveProvider() {
  if (!curProviderId.value || !curProvider.value) return;
  saveLoading.value = true;
  try {
    await connectClients.providers.updateProvider({
      id: curProviderId.value,
      displayName: providerName.value.trim() || curProvider.value.name,
      clientType: curProvider.value.type,
      enabled: curProvider.value.enabled,
      config: sanitizeConfig(providerConfig),
    });
    toast.success(t("speech.saveSuccess"));
    queryCache.invalidateQueries({ key: ["speech-providers"] });
    queryCache.invalidateQueries({ key: ["speech-provider-detail", curProviderId.value] });
  } catch {
    toast.error(t("common.saveFailed"));
  } finally {
    saveLoading.value = false;
  }
}

async function handleSaveModel(modelId: string, config: Record<string, unknown>) {
  const model = providerModels.value.find((item) => item.id === modelId);
  if (!model) return;
  try {
    await connectClients.speech.updateSpeechModel({
      id: modelId,
      displayName: model.displayName || model.modelId,
      metadata: config,
    });
    toast.success(t("speech.saveSuccess"));
    queryCache.invalidateQueries({ key: ["speech-provider-models", curProviderId.value] });
    queryCache.invalidateQueries({ key: ["speech-models"] });
  } catch {
    toast.error(t("common.saveFailed"));
  }
}

async function handleImportModels() {
  if (!curProviderId.value) return;
  importLoading.value = true;
  try {
    const response = await connectClients.speech.importSpeechProviderModels({
      providerId: curProviderId.value,
    });
    toast.success(
      t("speech.importSuccess", {
        created: response.models.length,
        skipped: 0,
      }),
    );
    queryCache.invalidateQueries({ key: ["speech-provider-models", curProviderId.value] });
    queryCache.invalidateQueries({ key: ["speech-models"] });
    queryCache.invalidateQueries({ key: ["speech-providers-meta"] });
  } catch {
    toast.error(t("speech.importFailed"));
  } finally {
    importLoading.value = false;
  }
}

async function handleTestModel(modelId: string, text: string, _config: Record<string, unknown>) {
  const response = await connectClients.speech.synthesizeBotSpeech({
    modelId,
    text,
  });
  return new Blob([response.audio], { type: response.contentType || "audio/mpeg" });
}

function mapSpeechProviderMeta(meta: ConnectSpeechProviderMeta): SpeechProviderMetaView {
  return {
    type: meta.type,
    displayName: meta.displayName,
    schema: normalizeConfigSchema(meta.schema),
  };
}

function normalizeConfigSchema(value: JsonObject | undefined): SpeechConfigSchema {
  const fields = Array.isArray(value?.fields) ? value.fields : [];
  return {
    fields: fields
      .filter(
        (field): field is JsonObject =>
          typeof field === "object" && field !== null && !Array.isArray(field),
      )
      .map((field) => ({
        key: getString(field, "key"),
        type: getString(field, "type"),
        title: getOptionalString(field, "title"),
        description: getOptionalString(field, "description"),
        required: getOptionalBool(field, "required"),
        advanced: getOptionalBool(field, "advanced"),
        enum: getStringArray(field, "enum"),
        example: field.example,
        order: getOptionalNumber(field, "order"),
      }))
      .filter((field) => field.key && field.type),
  };
}

function getString(value: JsonObject, key: string): string {
  const item = value[key];
  return typeof item === "string" ? item : "";
}

function getOptionalString(value: JsonObject, key: string): string | undefined {
  const item = value[key];
  return typeof item === "string" ? item : undefined;
}

function getOptionalBool(value: JsonObject, key: string): boolean | undefined {
  const item = value[key];
  return typeof item === "boolean" ? item : undefined;
}

function getOptionalNumber(value: JsonObject, key: string): number | undefined {
  const item = value[key];
  return typeof item === "number" ? item : undefined;
}

function getStringArray(value: JsonObject, key: string): string[] | undefined {
  const item = value[key];
  return Array.isArray(item)
    ? item.filter((entry): entry is string => typeof entry === "string")
    : undefined;
}

function sanitizeConfig(input: Record<string, unknown>) {
  const result: Record<string, unknown> = {};
  for (const [key, value] of Object.entries(input)) {
    if (value === "" || value == null) continue;
    result[key] = value;
  }
  return result;
}
</script>
