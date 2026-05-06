<template>
  <div class="flex items-center border-b px-3">
    <Search class="mr-2 size-3.5 shrink-0 text-muted-foreground" />
    <input
      v-model="searchTerm"
      :placeholder="$t('bots.settings.searchModel')"
      aria-label="Search models"
      class="flex h-10 w-full bg-transparent py-3 text-xs outline-none placeholder:text-muted-foreground"
    />
  </div>

  <div class="max-h-64 overflow-y-auto" role="listbox">
    <div v-if="filteredGroups.length === 0" class="py-6 text-center text-xs text-muted-foreground">
      {{ $t("bots.settings.noModel") }}
    </div>

    <div v-for="group in filteredGroups" :key="group.key" class="p-1">
      <div v-if="group.label" class="px-2 py-1.5 text-xs font-medium text-muted-foreground">
        {{ group.label }}
      </div>

      <button
        v-for="option in group.items"
        :key="option.value"
        type="button"
        role="option"
        :aria-selected="modelValue === option.value"
        class="relative flex w-full cursor-pointer items-start gap-2 rounded-md px-2 py-1.5 text-xs outline-none hover:bg-accent hover:text-accent-foreground [&_+button]:mt-1"
        :class="{ 'bg-accent': modelValue === option.value }"
        @click="$emit('update:modelValue', option.value)"
      >
        <Check v-if="modelValue === option.value" class="size-3.5 shrink-0 mt-0.5" />
        <span v-else class="size-3.5 shrink-0" />
        <span class="flex min-w-0 flex-1 flex-col gap-0.5">
          <span class="flex min-w-0 items-center gap-2">
            <span class="truncate flex-1 text-left" :title="option.label">{{ option.label }}</span>
            <span class="flex items-center gap-1.5 shrink-0">
              <ModelCapabilities
                v-if="option.compatibilities?.length"
                :compatibilities="option.compatibilities"
              />
              <ContextWindowBadge :context-window="option.contextWindow" />
            </span>
          </span>
          <span
            v-if="option.description && option.description !== option.label"
            class="text-xs text-muted-foreground truncate text-left"
            :title="option.description"
          >
            {{ option.description }}
          </span>
        </span>
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { Search, Check } from "lucide-vue-next";
import type { JsonObject } from "@bufbuild/protobuf";
import type { Model, Provider } from "@stringke/sdk/connect";
import ModelCapabilities from "@/components/model-capabilities/index.vue";
import ContextWindowBadge from "@/components/context-window-badge/index.vue";

export interface ModelOption {
  value: string;
  label: string;
  description?: string;
  groupKey: string;
  groupLabel: string;
  keywords: string[];
  compatibilities?: string[];
  contextWindow?: number;
}

const props = defineProps<{
  models: Model[];
  providers: Provider[];
  modelType: "chat" | "embedding";
  open?: boolean;
}>();

defineEmits<{
  "update:modelValue": [value: string];
}>();

const modelValue = defineModel<string>({ default: "" });

const searchTerm = ref("");

watch(
  () => props.open,
  (v) => {
    if (v) searchTerm.value = "";
  },
);

const providerMap = computed(() => {
  const map = new Map<string, string>();
  for (const p of props.providers) {
    if (p.id) map.set(p.id, p.displayName || p.name || p.id);
  }
  return map;
});

const typeFilteredModels = computed(() => props.models.filter((m) => m.type === props.modelType));

const options = computed<ModelOption[]>(() =>
  typeFilteredModels.value.map((model) => {
    const providerId = model.providerId ?? "";
    const metadata = model.metadata as
      | { compatibilities?: string[]; context_window?: number }
      | undefined;
    return {
      value: model.id || model.modelId || "",
      label: model.displayName || model.modelId || "",
      description: model.displayName ? model.modelId : undefined,
      groupKey: providerId,
      groupLabel: providerMap.value.get(providerId) ?? providerId,
      keywords: [model.modelId ?? "", model.displayName ?? ""],
      compatibilities: getStringArray(metadata, "compatibilities"),
      contextWindow: getNumber(metadata, "context_window"),
    };
  }),
);

function getStringArray(value: JsonObject | undefined, key: string): string[] | undefined {
  const item = value?.[key];
  return Array.isArray(item)
    ? item.filter((entry): entry is string => typeof entry === "string")
    : undefined;
}

function getNumber(value: JsonObject | undefined, key: string): number | undefined {
  const item = value?.[key];
  return typeof item === "number" ? item : undefined;
}

const filteredOptions = computed(() => {
  const keyword = searchTerm.value.trim().toLowerCase();
  if (!keyword) return options.value;
  return options.value.filter((opt) => {
    const terms = [opt.label, opt.description, ...opt.keywords]
      .filter((t): t is string => Boolean(t))
      .join(" ")
      .toLowerCase();
    return terms.includes(keyword);
  });
});

const filteredGroups = computed(() => {
  const groups = new Map<string, { key: string; label: string; items: ModelOption[] }>();
  for (const opt of filteredOptions.value) {
    if (!groups.has(opt.groupKey)) {
      groups.set(opt.groupKey, { key: opt.groupKey, label: opt.groupLabel, items: [] });
    }
    groups.get(opt.groupKey)!.items.push(opt);
  }
  return Array.from(groups.values());
});
</script>
