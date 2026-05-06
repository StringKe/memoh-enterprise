<template>
  <section class="p-4 mx-auto max-w-2xl">
    <h2 class="text-lg font-semibold mb-6">
      {{ $t("bots.createBot") }}
    </h2>

    <form @submit.prevent="handleSubmit">
      <!-- Basic Info -->
      <div>
        <h3 class="text-sm font-medium mb-4">
          {{ $t("bots.steps.basicInfo") }}
        </h3>
        <div class="flex items-start gap-4">
          <div
            class="group/avatar relative size-16 shrink-0 rounded-full overflow-hidden cursor-pointer"
          >
            <Avatar class="size-16 rounded-full">
              <AvatarImage
                v-if="form.avatar_url?.trim()"
                :src="form.avatar_url.trim()"
                :alt="form.display_name"
              />
              <AvatarFallback class="text-xl">
                {{ avatarFallback }}
              </AvatarFallback>
            </Avatar>
            <button
              type="button"
              class="absolute inset-0 flex items-center justify-center rounded-full bg-black/40 opacity-0 transition-opacity group-hover/avatar:opacity-100"
              :title="$t('common.edit')"
              :aria-label="$t('common.edit')"
              @click="avatarDialogOpen = true"
            >
              <SquarePen class="size-6 text-white" />
            </button>
          </div>
          <div class="flex-1 min-w-0">
            <Label class="mb-2">
              {{ $t("bots.displayName") }}
              <span class="text-destructive">*</span>
            </Label>
            <Input
              v-model="form.display_name"
              type="text"
              :placeholder="$t('bots.displayNamePlaceholder')"
            />
          </div>
        </div>
      </div>

      <Separator class="my-6" />

      <!-- Bot Group -->
      <div>
        <h3 class="text-sm font-medium mb-4">
          {{ $t("botGroups.title") }}
        </h3>
        <p class="text-xs text-muted-foreground mb-3">
          {{ $t("botGroups.createBotHint") }}
        </p>
        <BotGroupSelect
          v-model="form.group_id"
          :groups="botGroups"
          :placeholder="$t('botGroups.selectPlaceholder')"
        />
      </div>

      <Separator class="my-6" />

      <!-- Workspace (conditional) -->
      <template v-if="localWorkspaceEnabled">
        <div>
          <h3 class="text-sm font-medium mb-4">
            {{ $t("bots.steps.workspace") }}
          </h3>
          <div class="flex flex-col gap-4">
            <div>
              <div class="mb-2 flex items-center gap-2">
                <Label>{{ $t("bots.workspaceBackend") }}</Label>
                <Tooltip>
                  <TooltipTrigger as-child>
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon-sm"
                      class="size-5 text-muted-foreground hover:text-foreground"
                    >
                      <CircleHelp class="size-3.5" />
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent class="max-w-80 text-left leading-relaxed">
                    {{ $t("bots.workspaceBackendHint") }}
                  </TooltipContent>
                </Tooltip>
              </div>
              <Select v-model="form.workspace_backend">
                <SelectTrigger class="w-full">
                  <SelectValue :placeholder="$t('bots.workspaceBackend')" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="container">
                    {{ $t("bots.workspaceBackends.container") }}
                  </SelectItem>
                  <SelectItem value="local">
                    {{ $t("bots.workspaceBackends.local") }}
                  </SelectItem>
                </SelectContent>
              </Select>
            </div>

            <template v-if="form.workspace_backend === 'local'">
              <div>
                <Label class="mb-2">
                  {{ $t("bots.localWorkspacePath") }}
                  <span class="text-destructive">*</span>
                </Label>
                <Input
                  v-model="form.local_workspace_path"
                  type="text"
                  :placeholder="$t('bots.localWorkspacePathPlaceholder')"
                />
              </div>
              <div
                class="rounded-md border border-amber-500/30 bg-amber-500/5 px-3 py-2 text-xs text-amber-700 dark:text-amber-400"
              >
                {{ $t("bots.localWorkspaceWarning") }}
              </div>
            </template>
          </div>
        </div>

        <Separator class="my-6" />
      </template>

      <!-- Security Policy -->
      <div>
        <h3 class="text-sm font-medium mb-4">
          {{ $t("bots.steps.security") }}
        </h3>
        <div class="flex flex-col gap-3">
          <div class="mb-2 flex items-center gap-2">
            <Label>
              {{ $t("bots.aclPreset") }}
              <span class="text-destructive">*</span>
            </Label>
            <Tooltip>
              <TooltipTrigger as-child>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon-sm"
                  class="size-5 text-muted-foreground hover:text-foreground"
                >
                  <CircleHelp class="size-3.5" />
                </Button>
              </TooltipTrigger>
              <TooltipContent class="max-w-80 text-left leading-relaxed">
                {{ $t("bots.aclPresetHelp") }}
              </TooltipContent>
            </Tooltip>
          </div>
          <Select v-model="form.acl_preset">
            <SelectTrigger class="w-full">
              <SelectValue :placeholder="$t('bots.aclPreset')" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem
                v-for="preset in aclPresetOptions"
                :key="preset.value"
                :value="preset.value"
              >
                {{ $t(preset.titleKey) }}
              </SelectItem>
            </SelectContent>
          </Select>
          <p v-if="aclDescription" class="text-xs text-muted-foreground">
            {{ aclDescription }}
          </p>
        </div>
      </div>

      <Separator class="my-6" />

      <!-- Model -->
      <div>
        <h3 class="text-sm font-medium mb-4">
          {{ $t("bots.steps.model") }}
        </h3>
        <p class="text-xs text-muted-foreground mb-3">
          {{ $t("bots.steps.modelDesc") }}
        </p>
        <Label class="mb-2">{{ $t("bots.settings.chatModel") }}</Label>
        <ModelSelect
          v-model="form.chat_model_id"
          :models="models"
          :providers="providers"
          model-type="chat"
          :placeholder="$t('common.none')"
        />
      </div>

      <Separator class="my-6" />

      <!-- Memory -->
      <div>
        <h3 class="text-sm font-medium mb-4">
          {{ $t("bots.steps.memory") }}
        </h3>
        <p class="text-xs text-muted-foreground mb-3">
          {{ $t("bots.steps.memoryDesc") }}
        </p>
        <Label class="mb-2">{{ $t("bots.settings.memoryProvider") }}</Label>
        <MemoryProviderSelect
          v-model="form.memory_provider_id"
          :providers="memoryProviders"
          :placeholder="$t('common.none')"
        />
      </div>

      <Separator class="my-6" />

      <!-- Settings -->
      <div>
        <h3 class="text-sm font-medium mb-4">
          {{ $t("bots.steps.settings") }}
        </h3>
        <Label class="mb-2">
          {{ $t("bots.timezone") }}
          <span class="text-muted-foreground text-xs ml-1">({{ $t("common.optional") }})</span>
        </Label>
        <TimezoneSelect
          v-model="form.timezone"
          :placeholder="$t('bots.timezonePlaceholder')"
          allow-empty
          :empty-label="$t('bots.timezoneInherited')"
        />
      </div>

      <!-- Hint -->
      <div class="rounded-md border bg-muted/40 px-3 py-2 text-xs text-muted-foreground mt-6">
        {{ $t("bots.createBotWaitHint") }}
      </div>

      <!-- Actions -->
      <div class="flex justify-end gap-3 mt-6 pb-4">
        <Button type="button" variant="outline" @click="router.back()">
          {{ $t("common.cancel") }}
        </Button>
        <Button type="submit" :disabled="!canSubmit || submitLoading">
          <Spinner v-if="submitLoading" />
          {{ $t("bots.createBot") }}
        </Button>
      </div>
    </form>

    <AvatarEditDialog
      v-model:open="avatarDialogOpen"
      v-model:avatar-url="form.avatar_url"
      :fallback-text="avatarFallback"
    />
  </section>
</template>

<script setup lang="ts">
import {
  Avatar,
  AvatarImage,
  AvatarFallback,
  Button,
  Input,
  Label,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Separator,
  Spinner,
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@stringke/ui";
import { SquarePen, CircleHelp } from "lucide-vue-next";
import { ref, reactive, computed, watch, onMounted } from "vue";
import { useRouter } from "vue-router";
import { toast } from "vue-sonner";
import { useI18n } from "vue-i18n";
import { useMutation, useQueryCache } from "@pinia/colada";
import { connectClients } from "@/lib/connect-client";
import { useConnectQuery } from "@/lib/connect-colada";
import { useCapabilitiesStore } from "@/store/capabilities";
import { useAvatarInitials } from "@/composables/useAvatarInitials";
import { resolveApiErrorMessage } from "@/utils/api-error";
import { aclPresetOptions, defaultAclPreset } from "@/constants/acl-presets";
import { emptyTimezoneValue } from "@/utils/timezones";
import TimezoneSelect from "@/components/timezone-select/index.vue";
import ModelSelect from "./components/model-select.vue";
import MemoryProviderSelect from "./components/memory-provider-select.vue";
import AvatarEditDialog from "./components/avatar-edit-dialog.vue";
import BotGroupSelect from "./components/bot-group-select.vue";
import { buildInitialBotSettingsUpdateRequest } from "./components/bot-settings-payload";

const router = useRouter();
const { t } = useI18n();
const queryCache = useQueryCache();
const capabilities = useCapabilitiesStore();

onMounted(() => {
  void capabilities.load();
});

const localWorkspaceEnabled = computed(() => capabilities.localWorkspaceEnabled);

const form = reactive({
  display_name: "",
  group_id: "",
  avatar_url: "",
  acl_preset: defaultAclPreset as string,
  chat_model_id: "",
  memory_provider_id: "",
  timezone: emptyTimezoneValue,
  workspace_backend: "container",
  local_workspace_path: "",
});

watch(
  localWorkspaceEnabled,
  (enabled) => {
    if (enabled) {
      form.workspace_backend = "local";
    }
  },
  { immediate: true },
);

const localPathTouched = ref(false);

watch([() => form.display_name, () => form.workspace_backend], ([displayName, backend]) => {
  if (backend !== "local" || !displayName?.trim()) return;
  if (!localPathTouched.value && !form.local_workspace_path.trim()) {
    form.local_workspace_path = displayName
      .trim()
      .toLowerCase()
      .replace(/[^a-z0-9._-]+/g, "-");
  }
});

watch(
  () => form.local_workspace_path,
  () => {
    localPathTouched.value = true;
  },
);

const avatarDialogOpen = ref(false);
const avatarFallback = useAvatarInitials(() => form.display_name || "");

// Data queries
const { data: modelData } = useConnectQuery({
  key: ["models"],
  query: () => connectClients.models.listModels({}),
});

const { data: providerData } = useConnectQuery({
  key: ["providers"],
  query: () => connectClients.providers.listProviders({}),
});

const { data: memoryProviderData } = useConnectQuery({
  key: ["memory-providers"],
  query: () => connectClients.memoryProviders.listMemoryProviders({}),
});

const { data: botGroupData } = useConnectQuery({
  key: ["bot-groups"],
  query: () => connectClients.botGroups.listBotGroups({}),
});

const models = computed(() => modelData.value?.models ?? []);
const providers = computed(() => providerData.value?.providers ?? []);
const memoryProviders = computed(() => memoryProviderData.value?.providers ?? []);
const botGroups = computed(() => botGroupData.value?.groups ?? []);

watch(
  memoryProviders,
  (list) => {
    if (form.memory_provider_id) return;
    if (form.group_id) return;
    const builtin = list.find((p) => p.type === "builtin");
    if (builtin?.id) {
      form.memory_provider_id = builtin.id;
    }
  },
  { immediate: true },
);

watch(
  () => form.group_id,
  (groupId, previousGroupId) => {
    if (groupId && !previousGroupId) {
      form.memory_provider_id = "";
    }
  },
);

// ACL description
const aclDescription = computed(() => {
  const opt = aclPresetOptions.find((o) => o.value === form.acl_preset);
  return opt ? t(opt.descriptionKey) : "";
});

// Validation
const canSubmit = computed(() => {
  if (!form.display_name.trim()) return false;
  if (!form.acl_preset) return false;
  if (
    localWorkspaceEnabled.value &&
    form.workspace_backend === "local" &&
    !form.local_workspace_path.trim()
  )
    return false;
  return true;
});

// Submit
const { mutateAsync: createBot, isLoading: submitLoading } = useMutation({
  mutation: (input: Parameters<typeof connectClients.bots.createBot>[0]) =>
    connectClients.bots.createBot(input),
  onSettled: () => queryCache.invalidateQueries({ key: ["bots"] }),
});

async function handleSubmit() {
  if (!canSubmit.value || submitLoading.value) return;

  const metadata =
    localWorkspaceEnabled.value && form.workspace_backend === "local"
      ? {
          workspace: {
            backend: "local",
            local_workspace_path: form.local_workspace_path,
          },
        }
      : undefined;

  const tz = form.timezone === emptyTimezoneValue ? undefined : form.timezone || undefined;

  try {
    const response = await createBot({
      displayName: form.display_name.trim(),
      groupId: form.group_id,
      avatarUrl: form.avatar_url.trim(),
      timezone: tz,
      isActive: true,
      aclPreset: form.acl_preset,
      metadata,
    });

    const botId = response.bot?.id;
    if (botId && (form.chat_model_id || form.memory_provider_id)) {
      try {
        const payload = buildInitialBotSettingsUpdateRequest(botId, form);
        if (payload) {
          await connectClients.settings.updateBotSettings(payload);
        }
      } catch {
        // Bot created successfully, settings save failed — non-fatal
      }
    }

    toast.success(t("bots.createBotSuccess"));
    if (botId) {
      router.push({ name: "bot-detail", params: { botId } });
    } else {
      router.push({ name: "bots" });
    }
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t("common.saveFailed")));
  }
}
</script>
