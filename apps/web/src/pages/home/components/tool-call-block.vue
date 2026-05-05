<template>
  <ToolCallInline :block="block" />
</template>

<script setup lang="ts">
import type { ToolCallBlock } from "@/store/chat-list";
import ToolCallInline from "./tool-call-inline.vue";

defineProps<{
  block: ToolCallBlock;
}>();
</script>
