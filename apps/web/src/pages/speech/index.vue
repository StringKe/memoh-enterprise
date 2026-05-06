<script setup lang="ts">
import { computed, ref, provide, watch } from "vue";
import {
  ScrollArea,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  Toggle,
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
  Badge,
} from "@stringke/ui";
import type { SpeechProvider } from "@stringke/sdk/connect";
import ProviderSetting from "./components/provider-setting.vue";
import { Volume2 } from "lucide-vue-next";
import MasterDetailSidebarLayout from "@/components/master-detail-sidebar-layout/index.vue";
import ProviderIcon from "@/components/provider-icon/index.vue";
import { connectClients } from "@/lib/connect-client";
import { useConnectQuery } from "@/lib/connect-colada";

function getInitials(name: string | undefined) {
  const label = name?.trim() ?? "";
  return label ? label.slice(0, 2).toUpperCase() : "?";
}

const { data: providerData } = useConnectQuery({
  key: () => ["speech-providers"],
  query: () => connectClients.speech.listSpeechProviders({}),
});
const curProvider = ref<SpeechProvider>();
provide("curTtsProvider", curProvider);

const selectProvider = (name: string) =>
  computed(() => {
    return curProvider.value?.name === name;
  });

const filteredProviders = computed(() => {
  const providers = providerData.value?.providers ?? [];
  return [...providers].sort((a, b) => {
    const ae = a.enabled ? 1 : 0;
    const be = b.enabled ? 1 : 0;
    return be - ae;
  });
});

watch(
  filteredProviders,
  (list) => {
    if (!list || list.length === 0) {
      curProvider.value = { id: "" };
      return;
    }
    const currentId = curProvider.value?.id;
    if (currentId) {
      const stillExists = list.find((p: SpeechProvider) => p.id === currentId);
      if (stillExists) {
        curProvider.value = stillExists;
        return;
      }
    }
    curProvider.value = list[0];
  },
  { immediate: true },
);
</script>

<template>
  <MasterDetailSidebarLayout>
    <template #sidebar-content>
      <SidebarMenu v-for="item in filteredProviders" :key="item.id">
        <SidebarMenuItem>
          <SidebarMenuButton as-child class="justify-start py-5! px-4">
            <Toggle
              :class="[
                'py-4 border',
                curProvider?.id === item.id ? 'border-border' : 'border-transparent',
              ]"
              :model-value="selectProvider(item.name ?? '').value"
              @update:model-value="
                (isSelect) => {
                  if (isSelect) curProvider = item;
                }
              "
            >
              <span class="relative shrink-0">
                <span class="flex size-7 items-center justify-center rounded-full bg-muted">
                  <ProviderIcon :icon="item.type" size="1.25em">
                    <span class="text-xs font-medium text-muted-foreground">
                      {{ getInitials(item.name) }}
                    </span>
                  </ProviderIcon>
                </span>
                <Badge
                  v-if="item.enabled"
                  class="absolute -bottom-0.5 -right-0.5 size-2.5 p-0 rounded-full ring-2 ring-background"
                  variant="success"
                />
              </span>
              <span class="truncate">{{ item.name }}</span>
            </Toggle>
          </SidebarMenuButton>
        </SidebarMenuItem>
      </SidebarMenu>
    </template>

    <template #detail>
      <ScrollArea v-if="curProvider?.id" class="max-h-full h-full">
        <ProviderSetting />
      </ScrollArea>
      <Empty v-else class="h-full flex justify-center items-center">
        <EmptyHeader>
          <EmptyMedia variant="icon">
            <Volume2 />
          </EmptyMedia>
        </EmptyHeader>
        <EmptyTitle>{{ $t("speech.emptyTitle") }}</EmptyTitle>
        <EmptyDescription>{{ $t("speech.emptyDescription") }}</EmptyDescription>
      </Empty>
    </template>
  </MasterDetailSidebarLayout>
</template>
