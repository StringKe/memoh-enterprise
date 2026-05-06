<template>
  <section class="p-4 mx-auto">
    <!-- Header: search + create -->
    <div class="flex items-center justify-between mb-6 flex-wrap">
      <h2 class="text-xs font-medium max-md:hidden">
        {{ $t("bots.title") }}
      </h2>
      <div class="flex items-center gap-3">
        <div class="relative">
          <Search class="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground size-3.5" />
          <Input
            v-model="searchText"
            :placeholder="$t('bots.searchPlaceholder')"
            class="pl-9 w-64"
          />
        </div>
        <Button variant="default" @click="router.push({ name: 'bot-new' })">
          <Plus class="mr-1.5" />
          {{ $t("bots.createBot") }}
        </Button>
      </div>
    </div>

    <!-- Bot grid -->
    <div v-if="groupedBots.length > 0" class="space-y-6">
      <section v-for="section in groupedBots" :key="section.id" class="space-y-3">
        <div class="flex items-center gap-2">
          <h3 class="text-sm font-medium">
            {{ section.name }}
          </h3>
          <span class="text-xs text-muted-foreground">
            {{ section.bots.length }}
          </span>
        </div>
        <div class="grid gap-4 grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
          <BotCard v-for="bot in section.bots" :key="bot.id" :bot="bot" />
        </div>
      </section>
    </div>

    <!-- Empty state -->
    <Empty v-else-if="!isLoading" class="mt-20 flex flex-col items-center justify-center">
      <EmptyHeader>
        <EmptyMedia variant="icon">
          <Bot />
        </EmptyMedia>
      </EmptyHeader>
      <EmptyTitle>{{ $t("bots.emptyTitle") }}</EmptyTitle>
      <EmptyDescription>{{ $t("bots.emptyDescription") }}</EmptyDescription>
      <EmptyContent />
    </Empty>
  </section>
</template>

<script setup lang="ts">
import {
  Button,
  Input,
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@stringke/ui";
import { Search, Bot, Plus } from "lucide-vue-next";
import { ref, computed, watch, onUnmounted } from "vue";
import { useRouter } from "vue-router";
import { useI18n } from "vue-i18n";
import BotCard from "./components/bot-card.vue";
import { useQueryCache } from "@pinia/colada";
import { connectClients } from "@/lib/connect-client";
import { useConnectQuery } from "@/lib/connect-colada";
import type { Bot as ConnectBot } from "@stringke/sdk/connect";

const router = useRouter();
const { t } = useI18n();
const searchText = ref("");
const queryCache = useQueryCache();

const { data: botData, isLoading } = useConnectQuery({
  key: ["bots"],
  query: () => connectClients.bots.listBots({}),
});

const { data: botGroupData } = useConnectQuery({
  key: ["bot-groups"],
  query: () => connectClients.botGroups.listBotGroups({}),
});

const allBots = computed(() => botData.value?.bots ?? []);
const botGroups = computed(() => botGroupData.value?.groups ?? []);
const groupNameByID = computed(
  () => new Map(botGroups.value.map((group) => [group.id, group.name])),
);

const filteredBots = computed(() => {
  const keyword = searchText.value.trim().toLowerCase();
  if (!keyword) return allBots.value;
  return allBots.value.filter(
    (bot) =>
      bot.displayName.toLowerCase().includes(keyword) || bot.id.toLowerCase().includes(keyword),
  );
});

const groupedBots = computed(() => {
  const sections = new Map<string, { id: string; name: string; bots: ConnectBot[] }>();
  for (const bot of filteredBots.value) {
    const id = bot.groupId || "__ungrouped__";
    const name = bot.groupId
      ? (groupNameByID.value.get(bot.groupId) ?? bot.groupId)
      : t("botGroups.ungrouped");
    const section = sections.get(id) ?? { id, name, bots: [] };
    section.bots.push(bot);
    sections.set(id, section);
  }
  return Array.from(sections.values()).sort((a, b) => {
    if (a.id === "__ungrouped__") return 1;
    if (b.id === "__ungrouped__") return -1;
    return a.name.localeCompare(b.name);
  });
});

const hasPendingBots = computed(() =>
  allBots.value.some((bot) => bot.status === "creating" || bot.status === "deleting"),
);

let pollTimer: ReturnType<typeof setInterval> | null = null;

watch(
  hasPendingBots,
  (pending) => {
    if (pending) {
      if (pollTimer == null) {
        pollTimer = setInterval(() => {
          queryCache.invalidateQueries({ key: ["bots"] });
        }, 2000);
      }
      return;
    }
    if (pollTimer != null) {
      clearInterval(pollTimer);
      pollTimer = null;
    }
  },
  { immediate: true },
);

onUnmounted(() => {
  if (pollTimer != null) {
    clearInterval(pollTimer);
    pollTimer = null;
  }
});
</script>
