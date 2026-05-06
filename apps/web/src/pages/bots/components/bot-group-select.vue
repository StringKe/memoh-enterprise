<template>
  <Select :model-value="modelValue || NONE_VALUE" @update:model-value="handleUpdate">
    <SelectTrigger class="w-full">
      <SelectValue :placeholder="placeholder || $t('botGroups.selectPlaceholder')" />
    </SelectTrigger>
    <SelectContent>
      <SelectItem :value="NONE_VALUE">
        {{ $t("common.none") }}
      </SelectItem>
      <SelectItem v-for="group in groups" :key="group.id" :value="group.id">
        {{ group.name }}
      </SelectItem>
    </SelectContent>
  </Select>
</template>

<script setup lang="ts">
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@stringke/ui";
import type { BotGroup } from "@stringke/sdk/connect";
import { useI18n } from "vue-i18n";

const NONE_VALUE = "__none__";

defineProps<{
  modelValue: string;
  groups: BotGroup[];
  placeholder?: string;
}>();

const emit = defineEmits<{
  "update:modelValue": [value: string];
}>();

const { t: $t } = useI18n();

function handleUpdate(value: string | undefined) {
  emit("update:modelValue", value === NONE_VALUE ? "" : (value ?? ""));
}
</script>
