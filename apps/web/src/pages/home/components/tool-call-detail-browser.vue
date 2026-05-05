<template>
  <div class="space-y-1.5">
    <pre
      v-if="resultText"
      class="text-xs text-muted-foreground overflow-x-auto whitespace-pre-wrap break-all max-h-48 overflow-y-auto rounded-sm bg-muted/30 px-2 py-1"
      >{{ resultText }}</pre
    >
    <p v-else class="text-xs text-muted-foreground italic">
      {{ t("chat.tools.detail.noOutput") }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import type { ToolCallBlock } from "@/store/chat-list";

const props = defineProps<{ block: ToolCallBlock }>();
const { t } = useI18n();

function resolveResult(): Record<string, unknown> | null {
  if (!props.block.result) return null;
  const result = props.block.result as Record<string, unknown>;
  return (result.structuredContent as Record<string, unknown>) ?? result;
}

const resultText = computed(() => {
  if (!props.block.done) return "";
  const r = resolveResult();
  if (!r) return "";
  if (Array.isArray(r.content)) {
    const texts = (r.content as Array<Record<string, unknown>>)
      .filter((c) => c.type === "text")
      .map((c) => c.text as string)
      .filter(Boolean);
    if (texts.length) return texts.join("\n");
  }
  const { content: _content, ...rest } = r;
  void _content;
  const display = Object.keys(rest).length ? rest : r;
  try {
    return JSON.stringify(display, null, 2);
  } catch {
    return String(r);
  }
});
</script>
