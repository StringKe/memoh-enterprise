<template>
  <section class="p-4 mx-auto space-y-4">
    <div class="flex items-center justify-between gap-3 flex-wrap">
      <h2 class="text-xs font-medium">{{ $t("botGroups.title") }}</h2>
      <div class="flex items-center gap-2">
        <Button @click="router.push({ name: 'bot-group-new' })">
          <Plus class="mr-1.5 size-3.5" />
          {{ $t("common.create") }}
        </Button>
      </div>
    </div>

    <div v-if="loading" class="text-sm text-muted-foreground">
      {{ $t("common.loading") }}
    </div>
    <div v-else-if="groups.length === 0" class="py-16 text-center text-sm text-muted-foreground">
      {{ $t("botGroups.empty") }}
    </div>
    <div v-else class="overflow-hidden border rounded-md">
      <table class="w-full text-sm">
        <thead class="bg-muted/50 text-left">
          <tr>
            <th class="px-3 py-2 font-medium">{{ $t("common.name") }}</th>
            <th class="px-3 py-2 font-medium">{{ $t("botGroups.visibility") }}</th>
            <th class="px-3 py-2 font-medium">{{ $t("botGroups.description") }}</th>
            <th class="px-3 py-2 font-medium">{{ $t("botGroups.botCount") }}</th>
            <th class="px-3 py-2 font-medium text-right">{{ $t("common.operation") }}</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="group in groups"
            :key="group.id"
            class="cursor-pointer border-t hover:bg-muted/40"
            @click="router.push({ name: 'bot-group-detail', params: { groupId: group.id } })"
          >
            <td class="px-3 py-2 font-medium">{{ group.name }}</td>
            <td class="px-3 py-2 text-muted-foreground">
              {{ visibilityLabel(group.visibility) }}
            </td>
            <td class="px-3 py-2 text-muted-foreground">{{ group.description || "-" }}</td>
            <td class="px-3 py-2">{{ group.botCount.toString() }}</td>
            <td class="px-3 py-2 text-right">
              <Button variant="ghost" size="sm" @click.stop="deleteGroup(group.id)">
                <Trash2 class="size-3.5" />
              </Button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { Button } from "@stringke/ui";
import { Plus, Trash2 } from "lucide-vue-next";
import { useRouter } from "vue-router";
import { useI18n } from "vue-i18n";
import { useConnectMutation, useConnectQuery } from "@/lib/connect-colada";
import { connectClients } from "@/lib/connect-client";

const router = useRouter();
const { t } = useI18n();

const {
  data: groupsData,
  isLoading: loading,
  refetch: loadGroups,
} = useConnectQuery({
  key: ["bot-groups"],
  query: () => connectClients.botGroups.listBotGroups({}),
});

const groups = computed(() => groupsData.value?.groups ?? []);

function visibilityLabel(value: string) {
  if (value === "organization") return t("botGroups.visibilityOrganization");
  if (value === "public") return t("botGroups.visibilityPublic");
  return t("botGroups.visibilityPrivate");
}

const { mutateAsync: deleteGroupMutation } = useConnectMutation({
  mutation: (id: string) => connectClients.botGroups.deleteBotGroup({ id }),
});

async function deleteGroup(id: string) {
  await deleteGroupMutation(id);
  await loadGroups();
}
</script>
