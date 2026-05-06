<template>
  <section class="mx-auto max-w-4xl space-y-6 p-4">
    <div class="flex items-center justify-between gap-3">
      <div class="min-w-0">
        <h2 class="truncate text-lg font-semibold">
          {{ group?.name || $t("botGroups.title") }}
        </h2>
        <p class="text-xs text-muted-foreground">
          {{ $t("botGroups.detailSubtitle") }}
        </p>
      </div>
      <Button variant="outline" size="sm" @click="router.push({ name: 'bot-groups' })">
        {{ $t("common.back") }}
      </Button>
    </div>

    <div class="rounded-md border p-4">
      <div class="mb-4 space-y-1">
        <h3 class="text-sm font-medium">{{ $t("botGroups.profile") }}</h3>
        <p class="text-xs text-muted-foreground">{{ $t("botGroups.profileHint") }}</p>
      </div>

      <div class="grid gap-4 md:grid-cols-2">
        <div class="space-y-2">
          <Label>{{ $t("common.name") }}</Label>
          <Input v-model="form.name" />
        </div>
        <div class="space-y-2">
          <Label>{{ $t("botGroups.description") }}</Label>
          <Input v-model="form.description" />
        </div>
      </div>

      <div class="mt-4 flex justify-end">
        <Button :disabled="!form.name.trim() || savingProfile" @click="handleSaveProfile">
          <Spinner v-if="savingProfile" class="mr-1.5" />
          {{ $t("common.save") }}
        </Button>
      </div>
    </div>

    <div class="rounded-md border p-4">
      <div class="mb-4 space-y-1">
        <h3 class="text-sm font-medium">{{ $t("botGroups.settingsTitle") }}</h3>
        <p class="text-xs text-muted-foreground">{{ $t("botGroups.settingsHint") }}</p>
      </div>
      <GroupSettingsForm v-if="groupId" :group-id="groupId" />
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, reactive, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { toast } from "vue-sonner";
import { useI18n } from "vue-i18n";
import { useMutation, useQuery, useQueryCache } from "@pinia/colada";
import { Button, Input, Label, Spinner } from "@stringke/ui";
import { connectClients } from "@/lib/connect-client";
import { resolveApiErrorMessage } from "@/utils/api-error";
import GroupSettingsForm from "./components/group-settings-form.vue";

const route = useRoute();
const router = useRouter();
const { t: $t } = useI18n();
const queryCache = useQueryCache();

const groupId = computed(() => String(route.params.groupId ?? ""));

const form = reactive({
  name: "",
  description: "",
});

const { data: group } = useQuery({
  key: () => ["bot-group", groupId.value],
  query: async () => (await connectClients.botGroups.getBotGroup({ id: groupId.value })).group,
  enabled: () => !!groupId.value,
});

const { mutateAsync: updateGroup, isLoading: savingProfile } = useMutation({
  mutation: async () =>
    connectClients.botGroups.updateBotGroup({
      id: groupId.value,
      name: form.name.trim(),
      description: form.description.trim(),
    }),
  onSettled: () => {
    queryCache.invalidateQueries({ key: ["bot-group", groupId.value] });
    queryCache.invalidateQueries({ key: ["bot-groups"] });
  },
});

watch(
  group,
  (value) => {
    form.name = value?.name ?? "";
    form.description = value?.description ?? "";
  },
  { immediate: true },
);

async function handleSaveProfile() {
  if (!form.name.trim()) return;
  try {
    await updateGroup();
    toast.success($t("botGroups.saveSuccess"));
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, $t("common.saveFailed")));
  }
}
</script>
