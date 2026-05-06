<template>
  <Card
    class="group relative transition-shadow h-full flex flex-col"
    :class="isPending ? 'opacity-80 cursor-not-allowed' : 'hover:shadow-md cursor-pointer'"
    role="button"
    :tabindex="isPending ? -1 : 0"
    :aria-disabled="isPending"
    :aria-label="`Open bot ${bot.displayName || bot.id}`"
    @click="onOpenDetail"
    @keydown.enter.prevent="onOpenDetail"
    @keydown.space.prevent="onOpenDetail"
  >
    <CardHeader class="flex flex-row items-start gap-3 space-y-0 pb-2">
      <Avatar class="size-11 shrink-0">
        <AvatarImage v-if="bot.avatarUrl" :src="bot.avatarUrl" :alt="bot.displayName" />
        <AvatarFallback class="text-sm">
          {{ avatarFallback }}
        </AvatarFallback>
      </Avatar>
      <div class="flex-1 min-w-0 flex flex-col gap-1.5">
        <div class="flex items-center justify-between gap-2">
          <CardTitle class="text-sm truncate">
            {{ bot.displayName || bot.id }}
          </CardTitle>
          <Badge
            :variant="statusVariant"
            class="shrink-0 text-xs"
            :title="hasIssue ? issueTitle : undefined"
          >
            <LoaderCircle v-if="isPending" class="mr-1 size-3 animate-spin" />
            {{ statusLabel }}
          </Badge>
        </div>
        <div class="flex flex-wrap items-center gap-x-2 gap-y-0.5 text-xs text-muted-foreground">
          <span v-if="formattedDate"> {{ $t("common.createdAt") }} {{ formattedDate }} </span>
        </div>
      </div>
    </CardHeader>
  </Card>
</template>

<script setup lang="ts">
import {
  Card,
  CardHeader,
  CardTitle,
  Avatar,
  AvatarImage,
  AvatarFallback,
  Badge,
} from "@stringke/ui";
import { LoaderCircle } from "lucide-vue-next";
import { computed } from "vue";
import { useRouter } from "vue-router";
import { useI18n } from "vue-i18n";
import type { Bot, TimestampMessage } from "@stringke/sdk/connect";
import { formatDate } from "@/utils/date-time";
import { useAvatarInitials } from "@/composables/useAvatarInitials";
import { useBotStatusMeta } from "@/composables/useBotStatusMeta";

const router = useRouter();
const { t } = useI18n();

const props = defineProps<{
  bot: Bot;
}>();

const botRef = computed(() => props.bot);

const avatarFallback = useAvatarInitials(() => props.bot.displayName || props.bot.id);

const formattedDate = computed(() => {
  const createdAt = timestampToIso(props.bot.audit?.createdAt);
  if (!createdAt) return "";
  return formatDate(createdAt);
});

const { hasIssue, isPending, issueTitle, statusLabel, statusVariant } = useBotStatusMeta(botRef, t);

function onOpenDetail() {
  if (isPending.value) return;
  router.push({ name: "bot-detail", params: { botId: props.bot.id } });
}

function timestampToIso(value?: TimestampMessage): string {
  if (!value) return "";
  return new Date(Number(value.seconds) * 1000 + Math.floor(value.nanos / 1_000_000)).toISOString();
}
</script>
