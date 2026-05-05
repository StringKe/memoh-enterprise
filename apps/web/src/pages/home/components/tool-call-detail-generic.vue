<template>
  <div class="space-y-1.5">
    <div v-if="hasInput">
      <div class="text-[10px] uppercase tracking-wide text-muted-foreground/70 mb-0.5">
        {{ t("chat.tools.detail.input") }}
      </div>
      <pre
        class="text-xs text-muted-foreground overflow-x-auto whitespace-pre-wrap break-all max-h-48 overflow-y-auto rounded-sm bg-muted/30 px-2 py-1"
        >{{ inputText }}</pre
      >
    </div>
    <div v-if="hasResult">
      <div class="text-[10px] uppercase tracking-wide text-muted-foreground/70 mb-0.5">
        {{ t("chat.tools.detail.result") }}
      </div>
      <pre
        class="text-xs overflow-x-auto whitespace-pre-wrap break-all max-h-48 overflow-y-auto rounded-sm px-2 py-1"
        :class="isError ? 'text-destructive bg-destructive/5' : 'text-muted-foreground bg-muted/30'"
        >{{ resultText }}</pre
      >
    </div>
    <p v-if="!hasInput && !hasResult" class="text-xs text-muted-foreground italic">
      {{ t("chat.tools.detail.noData") }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import type { ToolCallBlock } from "@/store/chat-list";

const props = defineProps<{ block: ToolCallBlock }>();
const { t } = useI18n();

function formatJson(val: unknown): string {
  if (val == null) return "";
  if (typeof val === "string") return val;
  try {
    return JSON.stringify(val, null, 2);
  } catch {
    return String(val);
  }
}

const inputText = computed(() => formatJson(props.block.input));

const isError = computed(() => {
  if (!props.block.result) return false;
  const r = props.block.result as Record<string, unknown>;
  return r.isError === true;
});

const resultText = computed(() => {
  if (!props.block.result) return "";
  const r = props.block.result as Record<string, unknown>;
  if (Array.isArray(r.content)) {
    const texts = (r.content as Array<Record<string, unknown>>)
      .filter((c) => c.type === "text")
      .map((c) => c.text as string)
      .filter(Boolean);
    if (texts.length) return texts.join("\n");
  }
  const sc = r.structuredContent;
  if (sc) return formatJson(sc);
  return formatJson(r);
});

const hasInput = computed(() => Boolean(inputText.value));
const hasResult = computed(() => Boolean(resultText.value));
</script>
