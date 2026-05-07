<template>
  <SettingsShell width="wide" class="space-y-5">
    <div class="flex flex-wrap items-center justify-between gap-4">
      <div class="min-w-0 space-y-1">
        <Label>{{ $t("bots.settings.displayEnabled") }}</Label>
        <p class="text-xs text-muted-foreground">
          {{ $t("bots.settings.displayEnabledDescription") }}
        </p>
      </div>
      <div class="flex items-center gap-2">
        <Button variant="outline" size="sm" @click="openRuntimeDialog">
          <Info class="mr-2 size-4" />
          {{ $t("bots.display.runtimeButton") }}
        </Button>
        <Switch
          :model-value="settingsForm.displayEnabled"
          @update:model-value="(val) => (settingsForm.displayEnabled = !!val)"
        />
        <Button size="sm" :disabled="!settingsChanged || isSaving" @click="handleSaveSettings">
          <Spinner v-if="isSaving" class="mr-2 size-4" />
          {{ $t("bots.settings.save") }}
        </Button>
      </div>
    </div>

    <section class="space-y-3">
      <div class="flex items-center justify-between gap-3">
        <div>
          <h3 class="text-sm font-medium">
            {{ $t("bots.display.liveTitle") }}
          </h3>
          <p class="mt-1 text-xs text-muted-foreground">
            {{ $t("bots.display.liveDescription") }}
          </p>
        </div>
        <Button variant="outline" size="sm" :disabled="isSessionsFetching" @click="refetchSessions">
          <RefreshCw class="mr-2 size-4" :class="{ 'animate-spin': isSessionsFetching }" />
          {{ $t("common.refresh") }}
        </Button>
      </div>

      <div
        class="relative h-[min(62vh,620px)] min-h-[360px] overflow-hidden rounded-md border border-border bg-black"
      >
        <DisplayPane
          :bot-id="props.botId"
          tab-id="settings-display"
          :title="$t('bots.display.liveTitle')"
          active
          :closable="false"
          @snapshot="handleSnapshot"
        />
      </div>
    </section>

    <section class="space-y-3">
      <div>
        <h3 class="text-sm font-medium">
          {{ $t("bots.display.previewTitle") }}
        </h3>
        <p class="mt-1 text-xs text-muted-foreground">
          {{ $t("bots.display.previewDescription") }}
        </p>
      </div>

      <div v-if="previewItems.length" class="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
        <div
          v-for="item in previewItems"
          :key="item.key"
          class="group relative aspect-video overflow-hidden rounded-md border border-border bg-muted"
        >
          <img
            v-if="item.snapshot"
            :src="item.snapshot"
            :alt="item.title"
            class="size-full object-cover"
          />
          <div
            v-else
            class="flex size-full items-center justify-center text-xs text-muted-foreground"
          >
            {{ $t("bots.display.previewEmpty") }}
          </div>
          <div
            class="absolute inset-x-0 bottom-0 flex items-center justify-between gap-2 bg-background/90 px-3 py-2 text-xs"
          >
            <span class="min-w-0 truncate font-mono">{{ item.title }}</span>
            <Badge :variant="item.state === 'connected' ? 'secondary' : 'default'">
              {{ item.state || $t("bots.display.unknown") }}
            </Badge>
          </div>
          <button
            v-if="item.sessionId"
            type="button"
            class="absolute right-2 top-2 inline-flex size-7 items-center justify-center rounded-md border border-border bg-background/90 text-muted-foreground opacity-0 transition-opacity group-hover:opacity-100"
            :title="$t('bots.display.closeSession')"
            :aria-label="$t('bots.display.closeSession')"
            @click="handleCloseSession(item.sessionId)"
          >
            <Spinner v-if="closingSessionId === item.sessionId" class="size-4" />
            <X v-else class="size-4" />
          </button>
        </div>
      </div>
      <div
        v-else
        class="rounded-md border border-dashed border-border px-3 py-6 text-center text-xs text-muted-foreground"
      >
        {{ $t("bots.display.noSessions") }}
      </div>
    </section>

    <Dialog v-model:open="runtimeDialogOpen">
      <DialogContent class="sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>{{ $t("bots.display.runtimeTitle") }}</DialogTitle>
          <DialogDescription>
            {{ runtimeSummary }}
          </DialogDescription>
        </DialogHeader>
        <div class="grid gap-3 sm:grid-cols-2">
          <div
            v-for="item in runtimeItems"
            :key="item.key"
            class="flex items-center justify-between rounded-md border border-border px-3 py-2"
          >
            <span class="text-xs text-muted-foreground">{{ item.label }}</span>
            <Badge :variant="item.ok ? 'secondary' : 'destructive'">
              {{ item.value }}
            </Badge>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  </SettingsShell>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { toast } from "vue-sonner";
import { storeToRefs } from "pinia";
import { useMutation, useQuery, useQueryCache } from "@pinia/colada";
import {
  Badge,
  Button,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  Label,
  Spinner,
  Switch,
} from "@stringke/ui";
import { Info, RefreshCw, X } from "lucide-vue-next";
import SettingsShell from "@/components/settings-shell/index.vue";
import { resolveApiErrorMessage } from "@/utils/api-error";
import DisplayPane from "@/pages/home/components/display-pane.vue";
import { useDisplaySnapshotsStore } from "@/store/display-snapshots";
import { connectClients } from "@/lib/connect-client";
import type { DisplaySession, GetDisplayInfoResponse } from "@stringke/sdk/connect";

const props = defineProps<{
  botId: string;
}>();

interface PreviewItem {
  key: string;
  sessionId?: string;
  title: string;
  state?: string;
  snapshot?: string;
}

const { t } = useI18n();
const queryCache = useQueryCache();
const displaySnapshots = useDisplaySnapshotsStore();
const { items: snapshotItems } = storeToRefs(displaySnapshots);

const settingsForm = reactive({
  displayEnabled: false,
});
const runtimeDialogOpen = ref(false);
const closingSessionId = ref("");
const liveSessionId = ref("");

const { data: bot } = useQuery({
  key: () => ["bot", props.botId],
  query: async () => {
    const response = await connectClients.bots.getBot({ id: props.botId });
    return response.bot ?? null;
  },
  enabled: () => !!props.botId,
});

const { data: displayInfo, refetch: refetchDisplay } = useQuery({
  key: () => ["bot-display-info", props.botId],
  query: async () => {
    return await connectClients.containers.getDisplayInfo({ botId: props.botId });
  },
  enabled: () => !!props.botId,
  refetchOnWindowFocus: true,
});

const {
  data: sessionData,
  refetch: refetchSessions,
  isFetching: isSessionsFetching,
} = useQuery({
  key: () => ["bot-display-sessions", props.botId],
  query: async () => {
    return await connectClients.containers.listDisplaySessions({ botId: props.botId });
  },
  enabled: () => !!props.botId,
  refetchOnWindowFocus: true,
});

const { mutateAsync: updateSettings, isLoading: isSaving } = useMutation({
  mutation: async (displayEnabled: boolean) => {
    const response = await connectClients.bots.updateBot({
      id: props.botId,
      displayEnabled,
    });
    return response.bot ?? null;
  },
  onSettled: () => {
    queryCache.invalidateQueries({ key: ["bot", props.botId] });
    queryCache.invalidateQueries({ key: ["bots"] });
  },
});

watch(
  bot,
  (value) => {
    settingsForm.displayEnabled = value?.displayEnabled ?? false;
  },
  { immediate: true },
);

const settingsChanged = computed(
  () => settingsForm.displayEnabled !== (bot.value?.displayEnabled ?? false),
);

const sessions = computed<DisplaySession[]>(() => sessionData.value?.sessions ?? []);
const info = computed<Partial<GetDisplayInfoResponse>>(() => displayInfo.value ?? {});

const runtimeSummary = computed(() => {
  if (!info.value.enabled) return t("bots.display.summaryDisabled");
  if (info.value.available && info.value.running) return t("bots.display.summaryReady");
  if (info.value.unavailableReason) return info.value.unavailableReason;
  return t("bots.display.summaryPreparing");
});

const runtimeItems = computed(() => [
  statusItem("enabled", t("bots.display.enabled"), info.value.enabled),
  statusItem("runtime", t("bots.display.runtime"), info.value.available),
  statusItem("vnc", t("bots.display.vnc"), info.value.running),
  statusItem("session", t("bots.display.session"), info.value.sessionAvailable),
  statusItem("browser", t("bots.display.browser"), info.value.browserAvailable),
  statusItem("toolkit", t("bots.display.toolkit"), info.value.toolkitAvailable),
  {
    key: "system",
    label: t("bots.display.system"),
    ok: info.value.prepareSupported !== false,
    value: info.value.prepareSystem || "-",
  },
]);

const previewItems = computed<PreviewItem[]>(() => {
  const seen = new Set<string>();
  const items: PreviewItem[] = [];
  for (const session of sessions.value) {
    if (!session.id || session.id === liveSessionId.value) continue;
    seen.add(session.id);
    items.push({
      key: session.id,
      sessionId: session.id,
      title: shortSessionID(session.id),
      state: session.state,
      snapshot: displaySnapshots.find(props.botId, session.id)?.dataUrl,
    });
  }
  for (const snapshot of snapshotItems.value) {
    if (snapshot.botId !== props.botId) continue;
    const id = snapshot.sessionId || snapshot.tabId;
    if (!id || id === liveSessionId.value || snapshot.tabId === "settings-display" || seen.has(id))
      continue;
    seen.add(id);
    items.push({
      key: id,
      sessionId: snapshot.sessionId,
      title: shortSessionID(id),
      snapshot: snapshot.dataUrl,
    });
  }
  return items;
});

function statusItem(key: string, label: string, value: boolean | undefined) {
  return {
    key,
    label,
    ok: value === true,
    value: value ? t("common.yes") : t("common.no"),
  };
}

function shortSessionID(value: string) {
  return value.length > 12 ? value.slice(0, 8) : value;
}

async function openRuntimeDialog() {
  runtimeDialogOpen.value = true;
  await refetchDisplay();
}

async function handleSaveSettings() {
  try {
    await updateSettings(settingsForm.displayEnabled);
    await refetchDisplay();
    toast.success(t("bots.settings.saveSuccess"));
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t("common.saveFailed")));
  }
}

async function handleCloseSession(sessionId: string | undefined) {
  if (!sessionId) return;
  closingSessionId.value = sessionId;
  try {
    await connectClients.containers.closeDisplaySession({
      botId: props.botId,
      sessionId,
    });
    await refetchSessions();
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, t("bots.display.closeFailed")));
  } finally {
    closingSessionId.value = "";
  }
}

function handleSnapshot(payload: { tabId: string; sessionId?: string; dataUrl: string }) {
  if (payload.sessionId) {
    liveSessionId.value = payload.sessionId;
  }
  displaySnapshots.upsert(props.botId, payload);
}
</script>
