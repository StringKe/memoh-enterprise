<template>
  <Select :model-value="modelValue" @update:model-value="$emit('update:modelValue', $event)">
    <SelectTrigger :class="triggerClass">
      <SelectValue :placeholder="placeholder || $t('supermarket.selectBotPlaceholder')">
        <div v-if="selectedBot" class="flex items-center gap-2">
          <Avatar class="size-5 shrink-0">
            <AvatarImage
              v-if="selectedBot.avatarUrl"
              :src="selectedBot.avatarUrl"
              :alt="selectedBot.displayName"
            />
            <AvatarFallback class="text-[9px]">
              {{ initials(selectedBot.displayName || selectedBot.id || "") }}
            </AvatarFallback>
          </Avatar>
          <span class="truncate text-xs">{{ selectedBot.displayName || selectedBot.id }}</span>
        </div>
      </SelectValue>
    </SelectTrigger>
    <SelectContent>
      <SelectItem v-for="bot in bots" :key="bot.id" :value="bot.id!">
        <div class="flex items-center gap-2">
          <Avatar class="size-5 shrink-0">
            <AvatarImage v-if="bot.avatarUrl" :src="bot.avatarUrl" :alt="bot.displayName" />
            <AvatarFallback class="text-[9px]">
              {{ initials(bot.displayName || bot.id || "") }}
            </AvatarFallback>
          </Avatar>
          <span class="truncate text-xs">{{ bot.displayName || bot.id }}</span>
        </div>
      </SelectItem>
    </SelectContent>
  </Select>
</template>

<script setup lang="ts">
import { computed } from "vue";
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
  Avatar,
  AvatarImage,
  AvatarFallback,
} from "@stringke/ui";
import type { Bot } from "@stringke/sdk/connect";
import { connectClients } from "@/lib/connect-client";
import { useConnectQuery } from "@/lib/connect-colada";

const props = defineProps<{
  modelValue: string;
  placeholder?: string;
  triggerClass?: string;
}>();

defineEmits<{
  "update:modelValue": [value: string];
}>();

const { data: botsData } = useConnectQuery({
  key: ["bots"],
  query: () => connectClients.bots.listBots({}),
});
const bots = computed<Bot[]>(() => botsData.value?.bots ?? []);

const selectedBot = computed(() => bots.value.find((b) => b.id === props.modelValue));

function initials(name: string): string {
  return name
    .split(/[\s_-]+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((w) => w[0])
    .join("")
    .toUpperCase();
}
</script>
