<template>
  <SettingsShell width="wide">
    <section class="flex items-center gap-3">
      <span class="flex size-10 shrink-0 items-center justify-center rounded-full bg-muted">
        <ProviderIcon v-if="curProvider?.icon" :icon="curProvider.icon" size="1.5em" />
        <span v-else class="text-xs font-medium text-muted-foreground">
          {{ getInitials(curProvider?.name) }}
        </span>
      </span>
      <h4 class="scroll-m-20 tracking-tight min-w-0 truncate">
        {{ curProvider?.name }}
      </h4>
      <div class="ml-auto flex items-center gap-2">
        <span class="text-xs text-muted-foreground">
          {{ $t("provider.enable") }}
        </span>
        <Switch
          :model-value="curProvider?.enable ?? true"
          :disabled="!curProvider?.id || enableLoading"
          @update:model-value="handleToggleEnable"
        />
      </div>
    </section>
    <Separator class="mt-4 mb-6" />

    <ProviderForm
      :provider="curProvider"
      :edit-loading="editLoading"
      :delete-loading="deleteLoading"
      @submit="changeProvider"
      @delete="deleteProvider"
    />

    <Separator class="mt-4 mb-6" />

    <ModelList
      :provider-id="curProvider?.id"
      :models="modelDataList"
      :delete-model-loading="deleteModelLoading"
      @edit="handleEditModel"
      @delete="deleteModel"
    />
  </SettingsShell>
</template>

<script setup lang="ts">
import { Separator, Switch } from "@stringke/ui";
import ProviderIcon from "@/components/provider-icon/index.vue";
import SettingsShell from "@/components/settings-shell/index.vue";

function getInitials(name: string | undefined) {
  const label = name?.trim() ?? "";
  return label ? label.slice(0, 2).toUpperCase() : "?";
}
import ProviderForm from "./components/provider-form.vue";
import ModelList from "./components/model-list.vue";
import { computed, inject, provide, reactive, ref, toRef, watch } from "vue";
import { useQuery, useMutation, useQueryCache } from "@pinia/colada";
import { useI18n } from "vue-i18n";
import { toast } from "vue-sonner";
import { connectClients } from "@/lib/connect-client";
import { resolveConnectErrorMessage } from "@/lib/connect-errors";
import type { Model, Provider, ProviderModelSummary } from "@stringke/sdk/connect";

type ProviderView = {
  id?: string;
  name?: string;
  icon?: string;
  enable?: boolean;
  client_type?: string;
  config?: Record<string, unknown>;
  metadata?: Record<string, unknown>;
};

type ModelView = {
  id?: string;
  provider_id?: string;
  model_id?: string;
  name?: string;
  type?: string;
  config?: {
    compatibilities?: string[];
    dimensions?: number;
    context_window?: number;
    reasoning_efforts?: string[];
  };
};

function providerToView(provider: Provider): ProviderView {
  return {
    id: provider.id,
    name: provider.displayName || provider.name,
    enable: provider.enabled,
    client_type: provider.clientType,
    config: provider.config as Record<string, unknown> | undefined,
  };
}

function modelToView(model: Model | ProviderModelSummary): ModelView {
  const metadata = model.metadata as ModelView["config"] | undefined;
  return {
    id: model.id,
    provider_id: model.providerId,
    model_id: model.modelId,
    name: model.displayName,
    type: model.type,
    config: {
      ...metadata,
      compatibilities: "modalities" in model ? model.modalities : metadata?.compatibilities,
    },
  };
}

function providerUpdatePayload(data: Record<string, unknown>) {
  const config = data.config as Record<string, unknown> | undefined;
  return {
    displayName: typeof data.name === "string" ? data.name : undefined,
    baseUrl: typeof config?.base_url === "string" ? config.base_url : undefined,
    apiKey: typeof config?.api_key === "string" ? config.api_key : undefined,
    clientType: typeof data.client_type === "string" ? data.client_type : undefined,
    enabled: typeof data.enable === "boolean" ? data.enable : undefined,
    config,
  };
}

// ---- Model 编辑状态（provide 给 CreateModel） ----
const openModel = reactive<{
  state: boolean;
  title: "title" | "edit";
  curState: ModelView | null;
}>({
  state: false,
  title: "title",
  curState: null,
});

provide("openModel", toRef(openModel, "state"));
provide("openModelTitle", toRef(openModel, "title"));
provide("openModelState", toRef(openModel, "curState"));

function handleEditModel(model: ModelView) {
  openModel.state = true;
  openModel.title = "edit";
  openModel.curState = { ...model };
}

// ---- 当前 Provider ----
const curProvider = inject("curProvider", ref<ProviderView>());
const curProviderId = computed(() => curProvider.value?.id);
const enableLoading = ref(false);
const { t } = useI18n();

// ---- API Hooks ----
const queryCache = useQueryCache();

function invalidateProviderQueries() {
  queryCache.invalidateQueries({ key: ["providers"] });
  queryCache.invalidateQueries({ key: ["models"] });
}

function invalidateModelQueries() {
  queryCache.invalidateQueries({ key: ["provider-models"] });
  queryCache.invalidateQueries({ key: ["models"] });
}

const { mutate: deleteProvider, isLoading: deleteLoading } = useMutation({
  mutation: async () => {
    if (!curProviderId.value) return;
    await connectClients.providers.deleteProvider({ id: curProviderId.value });
  },
  onSettled: invalidateProviderQueries,
});

const { mutate: changeProvider, isLoading: editLoading } = useMutation({
  mutation: async (data: Record<string, unknown>) => {
    if (!curProviderId.value) return;
    const result = await connectClients.providers.updateProvider({
      id: curProviderId.value,
      ...providerUpdatePayload(data),
    });
    return result.provider ? providerToView(result.provider) : undefined;
  },
  onSettled: invalidateProviderQueries,
});

async function handleToggleEnable(value: boolean) {
  if (!curProviderId.value || !curProvider.value) return;

  const prev = curProvider.value.enable ?? true;
  curProvider.value = {
    ...curProvider.value,
    enable: value,
  };

  enableLoading.value = true;
  try {
    await connectClients.providers.updateProvider({ id: curProviderId.value, enabled: value });
    invalidateProviderQueries();
  } catch (error) {
    curProvider.value = {
      ...curProvider.value,
      enable: prev,
    };
    toast.error(resolveConnectErrorMessage(error, t("common.saveFailed")));
  } finally {
    enableLoading.value = false;
  }
}

const { mutate: deleteModel, isLoading: deleteModelLoading } = useMutation({
  mutation: async (modelID: string) => {
    if (!modelID) return;
    await connectClients.models.deleteModel({ id: modelID });
  },
  onSettled: invalidateModelQueries,
});

const { data: modelDataList } = useQuery({
  key: () => ["provider-models", curProviderId.value ?? ""],
  query: async () => {
    if (!curProviderId.value) return [];
    const response = await connectClients.providers.listProviderModels({
      providerId: curProviderId.value,
    });
    return response.models.map(modelToView);
  },
  enabled: () => !!curProviderId.value,
});

watch(
  curProvider,
  () => {
    queryCache.invalidateQueries({ key: ["provider-models"] });
  },
  { immediate: true },
);
</script>
