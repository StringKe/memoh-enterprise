<template>
  <SearchableSelectPopover
    v-model="selected"
    :options="options"
    :placeholder="placeholder || ''"
    :aria-label="placeholder || 'Select memory provider'"
    :search-placeholder="$t('memory.searchPlaceholder')"
    search-aria-label="Search memory providers"
    :empty-text="$t('memory.empty')"
    :show-group-headers="false"
  >
    <template #trigger="{ open, displayLabel }">
      <Button
        variant="outline"
        role="combobox"
        :aria-expanded="open"
        :aria-label="placeholder || 'Select memory provider'"
        class="w-full justify-between font-normal"
      >
        <span class="flex min-w-0 items-center gap-2 truncate">
          <Brain v-if="selected" class="size-3.5 shrink-0 text-primary" />
          <span class="truncate" :title="displayLabel || placeholder">{{
            displayLabel || placeholder
          }}</span>
        </span>
        <Search class="ml-2 size-3.5 shrink-0 text-muted-foreground" />
      </Button>
    </template>

    <template #option-icon="{ option }">
      <Brain v-if="option.value" class="size-3.5 shrink-0 text-primary" />
    </template>

    <template #option-label="{ option }">
      <span
        class="truncate flex-1 text-left"
        :class="{ 'text-muted-foreground': !option.value }"
        :title="option.label"
      >
        {{ option.label }}
      </span>
    </template>
  </SearchableSelectPopover>
</template>

<script setup lang="ts">
import { Brain, Search } from "lucide-vue-next";
import { Button } from "@stringke/ui";
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import type { JsonObject } from "@bufbuild/protobuf";
import type { MemoryProvider } from "@stringke/sdk/connect";
import SearchableSelectPopover from "@/components/searchable-select-popover/index.vue";
import type { SearchableSelectOption } from "@/components/searchable-select-popover/index.vue";

const props = defineProps<{
  providers: MemoryProvider[];
  placeholder?: string;
}>();
const { t } = useI18n();

const selected = defineModel<string>({ default: "" });

const options = computed<SearchableSelectOption[]>(() => {
  const noneOption: SearchableSelectOption = {
    value: "",
    label: t("common.none"),
    keywords: [t("common.none")],
  };
  const providerOptions = props.providers.map((provider) => ({
    value: provider.id || "",
    label: provider.name || provider.id || "",
    description:
      provider.type === "builtin"
        ? t(`memory.modeNames.${getString(provider.config, "memory_mode") || "off"}`)
        : provider.type,
    keywords: [
      provider.name ?? "",
      provider.type ?? "",
      getString(provider.config, "memory_mode") ?? "",
    ],
  }));
  return [noneOption, ...providerOptions];
});

function getString(value: JsonObject | undefined, key: string): string | undefined {
  const item = value?.[key];
  return typeof item === "string" ? item : undefined;
}
</script>
