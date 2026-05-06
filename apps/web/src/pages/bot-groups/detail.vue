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
        <div class="space-y-2">
          <Label>{{ $t("botGroups.visibility") }}</Label>
          <Select v-model="form.visibility">
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="private">{{ $t("botGroups.visibilityPrivate") }}</SelectItem>
              <SelectItem value="organization">
                {{ $t("botGroups.visibilityOrganization") }}
              </SelectItem>
              <SelectItem value="public">{{ $t("botGroups.visibilityPublic") }}</SelectItem>
            </SelectContent>
          </Select>
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
        <h3 class="text-sm font-medium">{{ $t("botGroups.permissionsTitle") }}</h3>
        <p class="text-xs text-muted-foreground">{{ $t("botGroups.permissionsHint") }}</p>
      </div>

      <form
        class="grid gap-3 md:grid-cols-[140px_1fr_180px_auto]"
        @submit.prevent="handleAssignRole"
      >
        <Select v-model="roleForm.principalType">
          <SelectTrigger>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="user">{{ $t("botGroups.principalUser") }}</SelectItem>
            <SelectItem value="group">{{ $t("botGroups.principalGroup") }}</SelectItem>
          </SelectContent>
        </Select>
        <Input v-model="roleForm.principalId" :placeholder="$t('botGroups.principalId')" />
        <Select v-model="roleForm.role">
          <SelectTrigger>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="bot_group_viewer">{{ $t("botGroups.roleViewer") }}</SelectItem>
            <SelectItem value="bot_group_operator">{{ $t("botGroups.roleOperator") }}</SelectItem>
            <SelectItem value="bot_group_editor">{{ $t("botGroups.roleEditor") }}</SelectItem>
            <SelectItem value="bot_group_owner">{{ $t("botGroups.roleOwner") }}</SelectItem>
          </SelectContent>
        </Select>
        <Button type="submit" :disabled="!roleForm.principalId.trim() || assigningRole">
          <Spinner v-if="assigningRole" class="mr-1.5" />
          {{ $t("common.add") }}
        </Button>
      </form>

      <div class="mt-4 overflow-hidden rounded-md border">
        <table class="w-full text-sm">
          <thead class="bg-muted/50 text-left">
            <tr>
              <th class="px-3 py-2 font-medium">{{ $t("botGroups.principal") }}</th>
              <th class="px-3 py-2 font-medium">{{ $t("botGroups.role") }}</th>
              <th class="px-3 py-2 text-right font-medium">{{ $t("common.operation") }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-if="roles.length === 0">
              <td colspan="3" class="px-3 py-6 text-center text-muted-foreground">
                {{ $t("botGroups.permissionsEmpty") }}
              </td>
            </tr>
            <tr v-for="role in roles" :key="role.id" class="border-t">
              <td class="px-3 py-2">{{ role.principalType }}: {{ role.principalId }}</td>
              <td class="px-3 py-2">{{ role.role }}</td>
              <td class="px-3 py-2 text-right">
                <Button variant="ghost" size="sm" @click="handleDeleteRole(role.id)">
                  <Trash2 class="size-3.5" />
                </Button>
              </td>
            </tr>
          </tbody>
        </table>
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
import {
  Button,
  Input,
  Label,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Spinner,
} from "@stringke/ui";
import { Trash2 } from "lucide-vue-next";
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
  visibility: "private",
});

const roleForm = reactive({
  principalType: "user",
  principalId: "",
  role: "bot_group_viewer",
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
      visibility: form.visibility,
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
    form.visibility = value?.visibility || "private";
  },
  { immediate: true },
);

const { data: rolesData, refetch: refetchRoles } = useQuery({
  key: () => ["bot-group-principal-roles", groupId.value],
  query: async () =>
    (await connectClients.botGroups.listBotGroupPrincipalRoles({ groupId: groupId.value })).roles,
  enabled: () => !!groupId.value,
});

const roles = computed(() => rolesData.value ?? []);

const { mutateAsync: assignRole, isLoading: assigningRole } = useMutation({
  mutation: async () =>
    connectClients.botGroups.assignBotGroupPrincipalRole({
      groupId: groupId.value,
      principalType: roleForm.principalType,
      principalId: roleForm.principalId.trim(),
      role: roleForm.role,
    }),
  onSettled: () => refetchRoles(),
});

const { mutateAsync: deleteRole } = useMutation({
  mutation: async (id: string) =>
    connectClients.botGroups.deleteBotGroupPrincipalRole({ groupId: groupId.value, id }),
  onSettled: () => refetchRoles(),
});

async function handleSaveProfile() {
  if (!form.name.trim()) return;
  try {
    await updateGroup();
    toast.success($t("botGroups.saveSuccess"));
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, $t("common.saveFailed")));
  }
}

async function handleAssignRole() {
  if (!roleForm.principalId.trim()) return;
  try {
    await assignRole();
    roleForm.principalId = "";
    toast.success($t("botGroups.roleSaved"));
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, $t("common.saveFailed")));
  }
}

async function handleDeleteRole(id: string) {
  try {
    await deleteRole(id);
    toast.success($t("botGroups.roleDeleted"));
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, $t("botGroups.roleDeleteFailed")));
  }
}
</script>
