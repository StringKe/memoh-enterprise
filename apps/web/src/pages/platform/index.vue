<template>
  <section class="p-4">
    <div v-if="isLoading" class="flex min-h-[50vh] items-center justify-center">
      <Spinner class="size-6" />
    </div>
    <div v-else class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
      <PlatformCard v-for="item in platformList" :key="item.id" :platform="item" />
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { useQuery } from "@pinia/colada";
import { Spinner } from "@stringke/ui";
import PlatformCard from "./components/platform-card.vue";
import { connectClients } from "@/lib/connect-client";

const { data, asyncStatus } = useQuery({
  key: () => ["connect-platform-channels"],
  query: () => connectClients.channels.listChannels({}),
});
const platformList = computed(() => data.value?.channels ?? []);
const isLoading = computed(() => asyncStatus.value === "loading" && !data.value);
</script>
