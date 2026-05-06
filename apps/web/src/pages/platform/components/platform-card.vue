<template>
  <Card class="flex h-full flex-col">
    <CardHeader>
      <CardTitle class="text-muted-foreground flex items-center justify-between gap-3">
        <span class="flex min-w-0 items-center gap-2">
          <ChannelIcon :channel="platform.id" size="1.25em" />
          <span class="truncate">{{ platform.displayName || platform.id }}</span>
        </span>
        <Badge v-if="platform.supportsWebhook" variant="outline"> Webhook </Badge>
      </CardTitle>
      <CardContent class="px-0 pb-0">
        <ol class="space-y-2 text-xs">
          <li>{{ $t("platform.platformLabel") }}: {{ platform.id }}</li>
          <li>
            Identity Config:
            {{ platform.supportsIdentityConfig ? $t("common.yes") : $t("common.no") }}
          </li>
        </ol>
      </CardContent>
    </CardHeader>
    <CardFooter class="text-muted-foreground mt-auto text-xs">
      {{ capabilitiesText }}
    </CardFooter>
  </Card>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { Card, CardHeader, CardFooter, CardContent, CardTitle, Badge } from "@stringke/ui";
import type { Channel } from "@stringke/sdk/connect";
import ChannelIcon from "@/components/channel-icon/index.vue";

const props = defineProps<{
  platform: Channel;
}>();

const capabilitiesText = computed(() => {
  const capabilities = props.platform.metadata?.capabilities;
  if (!capabilities || typeof capabilities !== "object") return "";
  const enabled = Object.entries(capabilities)
    .filter(([, value]) => value === true)
    .map(([key]) => key.replaceAll("_", " "));
  return enabled.slice(0, 5).join(", ");
});
</script>
