<template>
  <Popover v-model:open="open">
    <PopoverTrigger as-child>
      <Button
        variant="outline"
        role="combobox"
        :aria-expanded="open"
        :aria-label="placeholder || 'Select model'"
        class="w-full justify-between font-normal"
      >
        <span class="truncate" :title="displayLabel || placeholder">
          {{ displayLabel || placeholder }}
        </span>
        <Search class="ml-2 size-3.5 shrink-0 text-muted-foreground" />
      </Button>
    </PopoverTrigger>
    <PopoverContent class="w-[--reka-popover-trigger-width] p-0" align="start">
      <ModelOptions
        v-model="selected"
        :models="models"
        :providers="providers"
        :model-type="modelType"
        :open="open"
      />
    </PopoverContent>
  </Popover>
</template>

<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { Search } from "lucide-vue-next";
import { Popover, PopoverTrigger, PopoverContent, Button } from "@stringke/ui";
import type { Model, Provider } from "@stringke/sdk/connect";
import ModelOptions from "./model-options.vue";

const props = defineProps<{
  models: Model[];
  providers: Provider[];
  modelType: "chat" | "embedding";
  placeholder?: string;
}>();

const selected = defineModel<string>({ default: "" });
const open = ref(false);

watch(selected, () => {
  open.value = false;
});

const displayLabel = computed(() => {
  const model = props.models.find((m) => (m.id || m.modelId) === selected.value);
  return model?.displayName || model?.modelId || selected.value;
});
</script>
