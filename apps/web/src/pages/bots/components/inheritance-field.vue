<template>
  <div class="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
    <span>{{ $t("bots.settings.inheritance.source") }}: {{ sourceText }}</span>
    <Button
      v-if="canRestore"
      variant="ghost"
      size="sm"
      class="h-7 px-2 text-xs"
      :disabled="loading"
      @click="$emit('restore')"
    >
      <Spinner v-if="loading" class="mr-1 size-3" />
      {{ $t("bots.settings.inheritance.restore") }}
    </Button>
  </div>
</template>

<script setup lang="ts">
import { Button, Spinner } from "@stringke/ui";
import type { FieldSource } from "@stringke/sdk/connect";
import { computed } from "vue";
import { useI18n } from "vue-i18n";

const props = defineProps<{
  fields: string[];
  sources?: FieldSource[];
  loading?: boolean;
}>();

defineEmits<{
  restore: [];
}>();

const { t } = useI18n();

const fieldSources = computed(() =>
  props.fields
    .map((field) => props.sources?.find((item) => item.field === field))
    .filter((item): item is FieldSource => !!item),
);

const canRestore = computed(() => fieldSources.value.some((item) => item.source === "bot"));

const sourceText = computed(() => {
  const values = Array.from(new Set(fieldSources.value.map((item) => item.source).filter(Boolean)));
  if (!values.length) return t("bots.settings.inheritance.unknown");
  if (values.length > 1) return t("bots.settings.inheritance.mixed");
  return t(`bots.settings.inheritance.sources.${values[0]}`);
});
</script>
