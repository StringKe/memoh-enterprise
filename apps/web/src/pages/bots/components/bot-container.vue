<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { toast } from "vue-sonner";
import { useI18n } from "vue-i18n";
import { useRoute } from "vue-router";
import { useQuery } from "@pinia/colada";
import { ChevronRight } from "lucide-vue-next";
import {
  postBotsByBotIdContainerStream,
  type ContainerCreateLayerStatus,
  type ContainerCreateStreamEvent,
} from "@/composables/api/useContainerStream";
import {
  Button,
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
  Input,
  Label,
  Separator,
  Spinner,
  Switch,
  Textarea,
} from "@stringke/ui";
import ConfirmPopover from "@/components/confirm-popover/index.vue";
import ContainerCreateProgress from "./container-create-progress.vue";
import ContainerMetricsPanel from "./container-metrics-panel.vue";
import { useSyncedQueryParam } from "@/composables/useSyncedQueryParam";
import { useBotStatusMeta } from "@/composables/useBotStatusMeta";
import { useCapabilitiesStore } from "@/store/capabilities";
import { formatDateTime } from "@/utils/date-time";
import { shortenImageRef } from "@/utils/image-ref";
import { resolveApiErrorMessage } from "@/utils/api-error";
import { connectClients } from "@/lib/connect-client";
import { apiHttpUrl } from "@/lib/runtime-url";

const route = useRoute();
const { t } = useI18n();

type ContainerAction =
  | "refresh"
  | "create"
  | "start"
  | "stop"
  | "delete"
  | "delete-preserve"
  | "snapshot"
  | "export"
  | "import"
  | "restore"
  | "rollback"
  | "recreate"
  | "";

const containerLoading = ref(false);
const containerAction = ref<ContainerAction>("");
const rollbackVersion = ref<number | null>(null);
const createRestoreData = ref(false);
const createImage = ref("");
const createImagePrefilled = ref(false);
const createGPUEnabled = ref(false);
const createGPUDevices = ref("");
const createGPUPrefilled = ref(false);
const createAdvancedOpen = ref(false);
const newSnapshotName = ref("");
const importInputRef = ref<HTMLInputElement | null>(null);

interface CreateProgress {
  phase: "preserving" | "pulling" | "creating" | "restoring" | "complete" | "error";
  layers?: ContainerCreateLayerStatus[];
  image?: string;
  error?: string;
}
const createProgress = ref<CreateProgress | null>(null);

const createProgressPercent = computed(() => {
  const layers = createProgress.value?.layers;
  if (!layers || layers.length === 0) return 0;
  let totalOffset = 0;
  let totalSize = 0;
  for (const l of layers) {
    totalOffset += l.offset;
    totalSize += l.total;
  }
  return totalSize > 0 ? Math.round((totalOffset / totalSize) * 100) : 0;
});

const capabilitiesStore = useCapabilitiesStore();
const botId = computed(() => route.params.botId as string);
const containerBusy = computed(() => containerLoading.value || containerAction.value !== "");

type BotContainerInfo = {
  workspace_backend?: string;
  container_id?: string;
  status?: string;
  task_running?: boolean;
  namespace?: string;
  image?: string;
  cdi_devices?: string[];
  container_path?: string;
  data_restored?: boolean;
  has_preserved_data?: boolean;
  legacy?: boolean;
  created_at?: string;
  updated_at?: string;
  [key: string]: unknown;
};

type BotContainerMetrics = {
  cpu_percent?: number;
  memory_usage_bytes?: number;
  memory_limit_bytes?: number;
  storage_usage_bytes?: number;
  storage_limit_bytes?: number;
  storage_path?: string;
  sampled_at?: string;
  supported?: boolean;
  [key: string]: unknown;
};

type BotContainerSnapshot = {
  id?: string;
  name?: string;
  display_name?: string;
  runtime_snapshot_name?: string;
  version?: number;
  source?: string;
  parent_id?: string;
  managed?: boolean;
  created_at?: string;
  [key: string]: unknown;
};

type CreateContainerRequest = {
  restore_data?: boolean;
  image?: string;
  gpu?: {
    devices?: string[];
  };
};

const containerInfo = ref<BotContainerInfo | null>(null);
const containerMetrics = ref<BotContainerMetrics | null>(null);
const containerMissing = ref(false);
const snapshots = ref<BotContainerSnapshot[]>([]);
const metricsLoading = ref(false);
const snapshotsLoading = ref(false);

function resolveErrorMessage(error: unknown, fallback: string): string {
  return resolveApiErrorMessage(error, fallback);
}

function authHeaders(extra?: HeadersInit): HeadersInit {
  const token = localStorage.getItem("token") || "";
  return {
    ...(token ? { Authorization: `Bearer ${token}` } : {}),
    ...extra,
  };
}

function botRuntimeUrl(path: string, query?: URLSearchParams): string {
  const url = apiHttpUrl(`/bots/${encodeURIComponent(botId.value)}${path}`);
  const queryString = query?.toString();
  return queryString ? `${url}?${queryString}` : url;
}

async function readJsonResponse<T>(response: Response): Promise<T> {
  if (!response.ok) {
    const message = await response.text();
    throw new Error(message || `Request failed with status ${response.status}`);
  }
  return response.json() as Promise<T>;
}

async function readBlobResponse(response: Response): Promise<Blob> {
  if (!response.ok) {
    const message = await response.text();
    throw new Error(message || `Request failed with status ${response.status}`);
  }
  return response.blob();
}

async function ensureOk(response: Response): Promise<void> {
  if (response.ok) return;
  const message = await response.text();
  throw new Error(message || `Request failed with status ${response.status}`);
}

async function runContainerAction<T>(
  action: ContainerAction,
  operation: () => Promise<T>,
  successMessage?: string | ((result: T) => string),
) {
  containerAction.value = action;
  try {
    const result = await operation();
    const message = typeof successMessage === "function" ? successMessage(result) : successMessage;
    if (message) {
      toast.success(message);
    }
    return result;
  } catch (error) {
    toast.error(resolveErrorMessage(error, t("bots.container.actionFailed")));
    return undefined;
  } finally {
    containerAction.value = "";
  }
}

async function loadContainerData(showLoadingToast: boolean) {
  await capabilitiesStore.load();
  containerLoading.value = true;
  try {
    const response = await fetch(botRuntimeUrl("/container"), {
      headers: authHeaders(),
    });
    if (response.status === 404) {
      containerInfo.value = null;
      containerMetrics.value = null;
      containerMissing.value = true;
      snapshots.value = [];
      return;
    }

    containerInfo.value = await readJsonResponse<BotContainerInfo>(response);
    containerMissing.value = false;

    const metricsPromise = loadContainerMetrics(showLoadingToast);

    if (capabilitiesStore.snapshotSupported) {
      await Promise.all([metricsPromise, loadSnapshots()]);
    } else {
      snapshots.value = [];
      await metricsPromise;
    }
  } catch (error) {
    if (showLoadingToast) {
      toast.error(resolveErrorMessage(error, t("bots.container.loadFailed")));
    }
  } finally {
    containerLoading.value = false;
  }
}

async function loadContainerMetrics(showLoadingToast: boolean) {
  metricsLoading.value = true;
  try {
    containerMetrics.value = await readJsonResponse<BotContainerMetrics>(
      await fetch(botRuntimeUrl("/container/metrics"), {
        headers: authHeaders(),
      }),
    );
  } catch (error) {
    containerMetrics.value = null;
    if (showLoadingToast) {
      toast.error(resolveErrorMessage(error, t("bots.container.metricsLoadFailed")));
    }
  } finally {
    metricsLoading.value = false;
  }
}

async function loadSnapshots() {
  if (!containerInfo.value || !capabilitiesStore.snapshotSupported) {
    snapshots.value = [];
    return;
  }

  snapshotsLoading.value = true;
  try {
    const data = await readJsonResponse<{ snapshots?: BotContainerSnapshot[] }>(
      await fetch(botRuntimeUrl("/container/snapshots"), {
        headers: authHeaders(),
      }),
    );
    snapshots.value = data.snapshots ?? [];
  } catch (error) {
    snapshots.value = [];
    toast.error(resolveErrorMessage(error, t("bots.container.snapshotLoadFailed")));
  } finally {
    snapshotsLoading.value = false;
  }
}

async function handleRefreshContainer() {
  await runContainerAction("refresh", () => loadContainerData(false));
}

const { data: bot, refetch: refetchBot } = useQuery({
  key: () => ["bot", botId.value],
  query: async () => {
    const response = await connectClients.bots.getBot({ id: botId.value });
    return response.bot ?? null;
  },
  enabled: () => !!botId.value,
});

function rememberedWorkspaceImage(metadata: Record<string, unknown> | undefined): string {
  const workspace = metadata?.workspace;
  if (!workspace || typeof workspace !== "object" || Array.isArray(workspace)) return "";
  const image = (workspace as Record<string, unknown>).image;
  return typeof image === "string" ? shortenImageRef(image) : "";
}

type RememberedWorkspaceGPU = {
  exists: boolean;
  devices: string[];
};

function rememberedWorkspaceGPU(
  metadata: Record<string, unknown> | undefined,
): RememberedWorkspaceGPU {
  const workspace = metadata?.workspace;
  if (!workspace || typeof workspace !== "object" || Array.isArray(workspace)) {
    return { exists: false, devices: [] };
  }

  const workspaceRecord = workspace as Record<string, unknown>;
  if (!Object.prototype.hasOwnProperty.call(workspaceRecord, "gpu")) {
    return { exists: false, devices: [] };
  }

  const gpu = workspaceRecord.gpu;
  if (!gpu || typeof gpu !== "object" || Array.isArray(gpu)) {
    return { exists: true, devices: [] };
  }

  const rawDevices = (gpu as Record<string, unknown>).devices;
  const devices = Array.isArray(rawDevices)
    ? rawDevices
        .filter((value): value is string => typeof value === "string")
        .map((value) => value.trim())
        .filter(Boolean)
    : [];

  return { exists: true, devices: [...new Set(devices)] };
}

function parseCDIDevices(value: string): string[] {
  return [
    ...new Set(
      value
        .split(/[\n,]/)
        .map((item) => item.trim())
        .filter(Boolean),
    ),
  ];
}

const rememberedCreateImage = computed(() =>
  rememberedWorkspaceImage(bot.value?.metadata as Record<string, unknown> | undefined),
);
const rememberedCreateGPU = computed(() =>
  rememberedWorkspaceGPU(bot.value?.metadata as Record<string, unknown> | undefined),
);
const displayedContainerImage = computed(() => shortenImageRef(containerInfo.value?.image));
const displayedCDIDevices = computed(() => containerInfo.value?.cdi_devices ?? []);

const { isPending: botLifecyclePending } = useBotStatusMeta(bot, t);

function applyCreateContainerEvent(event: ContainerCreateStreamEvent): boolean {
  switch (event.type) {
    case "pulling":
      createProgress.value = { phase: "pulling", image: event.image };
      return false;
    case "pull_progress":
      createProgress.value = {
        phase: "pulling",
        image: createProgress.value?.image,
        layers: event.layers,
      };
      return false;
    case "pull_skipped":
    case "pull_delegated":
      createProgress.value = { phase: "pulling", image: event.image };
      return false;
    case "creating":
      createProgress.value = { phase: "creating" };
      return false;
    case "restoring":
      createProgress.value = { phase: "restoring" };
      return false;
    case "complete":
      // Keep the last visible progress state until the container detail view loads.
      // Rendering a separate "complete" phase here looks like the bar jumped back to 0.
      return !!event.container.data_restored;
    case "error":
      createProgress.value = { phase: "error", error: event.message };
      throw new Error(event.message || "Unknown error");
  }
}

async function createContainerSSE(
  body: CreateContainerRequest,
): Promise<{ dataRestored: boolean }> {
  const { stream } = await postBotsByBotIdContainerStream({
    path: { bot_id: botId.value },
    body,
    throwOnError: true,
  });

  let dataRestored = false;
  for await (const event of stream) {
    dataRestored = applyCreateContainerEvent(event) || dataRestored;
  }

  return { dataRestored };
}

async function handleCreateContainer() {
  if (botLifecyclePending.value) return;

  containerAction.value = "create";
  createProgress.value = { phase: "pulling" };
  try {
    const gpuDevices = parseCDIDevices(createGPUDevices.value);
    if (createGPUEnabled.value && gpuDevices.length === 0) {
      throw new Error(t("bots.container.gpuDevicesRequired"));
    }

    const body: CreateContainerRequest = {
      restore_data: createRestoreData.value,
    };
    const trimmedImage = createImage.value.trim();
    if (trimmedImage) body.image = trimmedImage;
    if (createGPUEnabled.value || rememberedCreateGPU.value.exists) {
      body.gpu = {
        devices: createGPUEnabled.value ? gpuDevices : [],
      };
    }

    const { dataRestored } = await createContainerSSE(body);
    createRestoreData.value = false;
    createImage.value = "";
    createGPUEnabled.value = false;
    createGPUDevices.value = "";
    await loadContainerData(false);
    await refetchBot();
    toast.success(
      dataRestored ? t("bots.container.createRestoreSuccess") : t("bots.container.createSuccess"),
    );
  } catch (error) {
    toast.error(resolveErrorMessage(error, t("bots.container.actionFailed")));
  } finally {
    containerAction.value = "";
    createProgress.value = null;
  }
}

const isContainerTaskRunning = computed(() => {
  const info = containerInfo.value;
  if (!info) return false;

  const status = (info.status ?? "").trim().toLowerCase();
  if (status === "stopped" || status === "exited") return false;
  return !!info.task_running;
});

const hasPreservedData = computed(() => !!containerInfo.value?.has_preserved_data);
const isLegacy = computed(() => !!containerInfo.value?.legacy);

async function handleRecreateContainer() {
  if (botLifecyclePending.value || !containerInfo.value) return;

  containerAction.value = "recreate";
  try {
    createProgress.value = { phase: "preserving" };
    await ensureOk(
      await fetch(botRuntimeUrl("/container", new URLSearchParams({ preserve_data: "true" })), {
        method: "DELETE",
        headers: authHeaders(),
      }),
    );

    createProgress.value = { phase: "pulling" };
    await createContainerSSE({ restore_data: true });
    await loadContainerData(false);
    toast.success(t("bots.container.legacyRecreateSuccess"));
  } catch (error) {
    toast.error(resolveErrorMessage(error, t("bots.container.actionFailed")));
  } finally {
    containerAction.value = "";
    createProgress.value = null;
  }
}

async function handleStopContainer() {
  if (botLifecyclePending.value || !containerInfo.value) return;

  await runContainerAction(
    "stop",
    async () => {
      await ensureOk(
        await fetch(botRuntimeUrl("/container/stop"), {
          method: "POST",
          headers: authHeaders(),
        }),
      );
      await loadContainerData(false);
    },
    t("bots.container.stopSuccess"),
  );
}

async function handleStartContainer() {
  if (botLifecyclePending.value || !containerInfo.value) return;

  await runContainerAction(
    "start",
    async () => {
      await ensureOk(
        await fetch(botRuntimeUrl("/container/start"), {
          method: "POST",
          headers: authHeaders(),
        }),
      );
      await loadContainerData(false);
    },
    t("bots.container.startSuccess"),
  );
}

async function handleDeleteContainer(preserveData: boolean) {
  if (botLifecyclePending.value || !containerInfo.value) return;

  const action: ContainerAction = preserveData ? "delete-preserve" : "delete";
  const successMessage = preserveData
    ? t("bots.container.deletePreserveSuccess")
    : t("bots.container.deleteSuccess");
  const lastImage = shortenImageRef(containerInfo.value.image);

  await runContainerAction(
    action,
    async () => {
      await ensureOk(
        await fetch(
          botRuntimeUrl(
            "/container",
            preserveData ? new URLSearchParams({ preserve_data: "true" }) : undefined,
          ),
          {
            method: "DELETE",
            headers: authHeaders(),
          },
        ),
      );
      containerInfo.value = null;
      containerMetrics.value = null;
      containerMissing.value = true;
      snapshots.value = [];
      createRestoreData.value = preserveData;
      createImage.value = lastImage;
      createImagePrefilled.value = !!lastImage;
    },
    successMessage,
  );
}

function buildExportFilename() {
  const timestamp = new Date().toISOString().replaceAll(":", "-");
  return `bot-${botId.value}-data-${timestamp}.tar.gz`;
}

function downloadBlob(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = filename;
  anchor.click();
  window.setTimeout(() => URL.revokeObjectURL(url), 0);
}

async function handleExportData() {
  if (botLifecyclePending.value || !containerInfo.value) return;

  await runContainerAction(
    "export",
    async () => {
      const blob = await readBlobResponse(
        await fetch(botRuntimeUrl("/container/data/export"), {
          method: "POST",
          headers: authHeaders(),
        }),
      );
      downloadBlob(blob, buildExportFilename());
    },
    t("bots.container.exportSuccess"),
  );
}

function triggerImportData() {
  importInputRef.value?.click();
}

async function handleImportData(event: Event) {
  if (botLifecyclePending.value || !containerInfo.value) return;

  const input = event.target as HTMLInputElement;
  const file = input.files?.[0];
  if (!file) return;

  await runContainerAction(
    "import",
    async () => {
      const body = new FormData();
      body.set("file", file);
      await ensureOk(
        await fetch(botRuntimeUrl("/container/data/import"), {
          method: "POST",
          headers: authHeaders(),
          body,
        }),
      );
      await loadContainerData(false);
    },
    t("bots.container.importSuccess"),
  );

  input.value = "";
}

async function handleRestorePreservedData() {
  if (botLifecyclePending.value || !containerInfo.value || !hasPreservedData.value) return;

  await runContainerAction(
    "restore",
    async () => {
      await ensureOk(
        await fetch(botRuntimeUrl("/container/data/restore"), {
          method: "POST",
          headers: authHeaders(),
        }),
      );
      await loadContainerData(false);
    },
    t("bots.container.restoreSuccess"),
  );
}

const statusKeyMap: Record<string, string> = {
  created: "statusCreated",
  running: "statusRunning",
  stopped: "statusStopped",
  exited: "statusExited",
};

const containerStatusText = computed(() => {
  const status = (containerInfo.value?.status ?? "").trim().toLowerCase();
  const key = statusKeyMap[status] ?? "statusUnknown";
  return t(`bots.container.${key}`);
});

const containerTaskText = computed(() => {
  const info = containerInfo.value;
  if (!info) return "-";

  const status = (info.status ?? "").trim().toLowerCase();
  if (status === "exited") return t("bots.container.taskCompleted");
  return info.task_running ? t("bots.container.taskRunning") : t("bots.container.taskStopped");
});

const preservedDataText = computed(() =>
  hasPreservedData.value
    ? t("bots.container.preservedDataAvailableShort")
    : t("bots.container.preservedDataEmpty"),
);

function formatDate(value: string | undefined): string {
  return formatDateTime(value, { fallback: "-" });
}

function snapshotCreatedAt(value: BotContainerSnapshot) {
  const timestamp = Date.parse(value.created_at ?? "");
  return Number.isNaN(timestamp) ? Number.NEGATIVE_INFINITY : timestamp;
}

function snapshotDisplayName(value: BotContainerSnapshot) {
  return (value.display_name ?? value.name ?? value.runtime_snapshot_name ?? "").trim() || "-";
}

function snapshotRuntimeName(value: BotContainerSnapshot) {
  const runtimeName = (value.runtime_snapshot_name ?? "").trim();
  return runtimeName && runtimeName !== snapshotDisplayName(value) ? runtimeName : "";
}

function snapshotVersionText(value: BotContainerSnapshot) {
  return value.version !== undefined ? `v${value.version}` : "-";
}

function snapshotSourceText(value: BotContainerSnapshot) {
  const source = (value.source ?? "").trim().toLowerCase();
  if (!source) return "-";

  const sourceKeyMap: Record<string, string> = {
    manual: "sourceManual",
    pre_exec: "sourcePreExec",
    rollback: "sourceRollback",
  };
  const sourceKey = sourceKeyMap[source];
  return sourceKey ? t(`bots.container.${sourceKey}`) : source;
}

function canRollbackSnapshot(value: BotContainerSnapshot) {
  return !!value.managed && typeof value.version === "number" && value.version > 0;
}

async function handleRollbackSnapshot(snapshot: BotContainerSnapshot) {
  if (
    botLifecyclePending.value ||
    !containerInfo.value ||
    !canRollbackSnapshot(snapshot) ||
    snapshot.version === undefined
  ) {
    return;
  }

  rollbackVersion.value = snapshot.version;
  await runContainerAction(
    "rollback",
    async () => {
      await ensureOk(
        await fetch(botRuntimeUrl("/container/snapshots/rollback"), {
          method: "POST",
          headers: authHeaders({ "Content-Type": "application/json" }),
          body: JSON.stringify({ version: snapshot.version }),
        }),
      );
      await loadContainerData(false);
    },
    t("bots.container.rollbackSuccess"),
  );
  rollbackVersion.value = null;
}

async function handleCreateSnapshot() {
  if (botLifecyclePending.value || !containerInfo.value || !capabilitiesStore.snapshotSupported)
    return;

  await runContainerAction(
    "snapshot",
    async () => {
      await ensureOk(
        await fetch(botRuntimeUrl("/container/snapshots"), {
          method: "POST",
          headers: authHeaders({ "Content-Type": "application/json" }),
          body: JSON.stringify({ snapshot_name: newSnapshotName.value.trim() }),
        }),
      );
      newSnapshotName.value = "";
      await loadSnapshots();
    },
    t("bots.container.snapshotSuccess"),
  );
}

const sortedSnapshots = computed(() => {
  return [...snapshots.value].sort((left, right) => {
    const managedDiff = Number(!!right.managed) - Number(!!left.managed);
    if (managedDiff !== 0) return managedDiff;

    const leftVersion = left.version ?? Number.NEGATIVE_INFINITY;
    const rightVersion = right.version ?? Number.NEGATIVE_INFINITY;
    if (leftVersion !== rightVersion) return rightVersion - leftVersion;

    const createdDiff = snapshotCreatedAt(right) - snapshotCreatedAt(left);
    if (createdDiff !== 0) return createdDiff;

    return snapshotDisplayName(left).localeCompare(snapshotDisplayName(right));
  });
});

const activeTab = useSyncedQueryParam("tab", "overview");

watch(containerMissing, (missing) => {
  if (!missing) {
    createImagePrefilled.value = false;
    createGPUPrefilled.value = false;
    createAdvancedOpen.value = false;
  }
});

watch(
  [containerMissing, rememberedCreateImage],
  ([missing, remembered]) => {
    if (!missing || createImagePrefilled.value) return;
    if (!remembered || createImage.value.trim()) return;
    createImage.value = remembered;
    createImagePrefilled.value = true;
  },
  { immediate: true },
);

watch(
  [containerMissing, rememberedCreateGPU],
  ([missing, remembered]) => {
    if (!missing || createGPUPrefilled.value) return;
    if (!remembered.exists) return;
    if (createGPUEnabled.value || createGPUDevices.value.trim()) return;
    createGPUEnabled.value = remembered.devices.length > 0;
    createGPUDevices.value = remembered.devices.join("\n");
    createGPUPrefilled.value = true;
  },
  { immediate: true },
);

watch(
  [activeTab, botId],
  ([tab]) => {
    if (!botId.value) return;
    if (tab === "container") {
      void loadContainerData(true);
    }
  },
  { immediate: true },
);
</script>

<template>
  <div class="mx-auto space-y-5">
    <div class="flex items-start justify-between gap-3">
      <div class="min-w-0 space-y-1">
        <h3 class="text-sm font-semibold">
          {{ $t("bots.container.title") }}
        </h3>
        <p class="text-xs text-muted-foreground">
          {{ $t("bots.container.subtitle") }}
        </p>
      </div>
      <div class="flex shrink-0 flex-wrap justify-end gap-2">
        <Button
          variant="outline"
          size="sm"
          :disabled="containerBusy"
          @click="handleRefreshContainer"
        >
          <Spinner v-if="containerLoading || containerAction === 'refresh'" class="mr-1.5" />
          {{ $t("common.refresh") }}
        </Button>
        <Button
          v-if="containerInfo"
          variant="secondary"
          size="sm"
          :disabled="containerBusy || botLifecyclePending"
          @click="isContainerTaskRunning ? handleStopContainer() : handleStartContainer()"
        >
          <Spinner
            v-if="containerAction === 'start' || containerAction === 'stop'"
            class="mr-1.5"
          />
          {{
            isContainerTaskRunning
              ? $t("bots.container.actions.stop")
              : $t("bots.container.actions.start")
          }}
        </Button>
      </div>
    </div>

    <div
      v-if="botLifecyclePending"
      class="rounded-md border border-yellow-300/50 bg-yellow-50/70 p-3 text-xs text-yellow-800 dark:border-yellow-800/50 dark:bg-yellow-900/10 dark:text-yellow-200"
    >
      {{ $t("bots.container.botNotReady") }}
    </div>

    <div
      v-if="containerLoading && !containerInfo && !containerMissing"
      class="flex items-center gap-2 text-xs text-muted-foreground"
    >
      <Spinner />
      <span>{{ $t("common.loading") }}</span>
    </div>

    <div v-else-if="containerMissing" class="space-y-4 rounded-md border p-4">
      <p class="text-xs text-muted-foreground">
        {{ $t("bots.container.empty") }}
      </p>

      <div class="rounded-md border p-4 space-y-4">
        <div class="space-y-1">
          <p class="text-xs font-medium">
            {{ $t("bots.container.actions.create") }}
          </p>
          <p class="text-xs text-muted-foreground">
            {{ $t("bots.container.createHint") }}
          </p>
        </div>

        <div class="flex items-start justify-between gap-4 rounded-md border p-3">
          <div class="space-y-1">
            <Label>{{ $t("bots.container.createRestoreDataLabel") }}</Label>
            <p class="text-xs text-muted-foreground">
              {{ $t("bots.container.createRestoreDataDescription") }}
            </p>
          </div>
          <Switch
            :model-value="createRestoreData"
            :disabled="containerBusy || botLifecyclePending"
            @update:model-value="(value) => (createRestoreData = !!value)"
          />
        </div>

        <div class="space-y-2">
          <Label>{{ $t("bots.container.createImageLabel") }}</Label>
          <Input
            v-model="createImage"
            placeholder="debian:bookworm-slim"
            :disabled="containerBusy || botLifecyclePending"
            class="font-mono"
          />
          <p class="text-xs text-muted-foreground">
            {{ $t("bots.container.createImageDescription") }}
          </p>
        </div>

        <Collapsible v-model:open="createAdvancedOpen">
          <div class="rounded-md border">
            <CollapsibleTrigger
              class="flex w-full items-center justify-between gap-3 px-3 py-2 text-left hover:bg-accent/40"
            >
              <div class="space-y-1">
                <p class="text-xs font-medium">
                  {{ $t("bots.container.createAdvancedTitle") }}
                </p>
                <p class="text-xs text-muted-foreground">
                  {{ $t("bots.container.createAdvancedDescription") }}
                </p>
              </div>
              <ChevronRight
                class="size-4 shrink-0 text-muted-foreground transition-transform"
                :class="{ 'rotate-90': createAdvancedOpen }"
              />
            </CollapsibleTrigger>

            <CollapsibleContent>
              <div class="space-y-4 border-t px-3 py-3">
                <div class="flex items-start justify-between gap-4 rounded-md border p-3">
                  <div class="space-y-1">
                    <Label>{{ $t("bots.container.createGpuLabel") }}</Label>
                    <p class="text-xs text-muted-foreground">
                      {{ $t("bots.container.createGpuDescription") }}
                    </p>
                  </div>
                  <Switch
                    :model-value="createGPUEnabled"
                    :disabled="containerBusy || botLifecyclePending"
                    @update:model-value="(value) => (createGPUEnabled = !!value)"
                  />
                </div>

                <div v-if="createGPUEnabled" class="space-y-2">
                  <Label>{{ $t("bots.container.createGpuDevicesLabel") }}</Label>
                  <Textarea
                    v-model="createGPUDevices"
                    :placeholder="$t('bots.container.createGpuDevicesPlaceholder')"
                    :disabled="containerBusy || botLifecyclePending"
                    class="min-h-24 font-mono text-xs"
                  />
                  <p class="text-xs text-muted-foreground">
                    {{ $t("bots.container.createGpuDevicesDescription") }}
                  </p>
                </div>
              </div>
            </CollapsibleContent>
          </div>
        </Collapsible>

        <div class="flex justify-end">
          <Button :disabled="containerBusy || botLifecyclePending" @click="handleCreateContainer">
            <Spinner v-if="containerAction === 'create'" class="mr-1.5" />
            {{ $t("bots.container.actions.create") }}
          </Button>
        </div>

        <div v-if="createProgress && containerAction === 'create'" class="space-y-2">
          <ContainerCreateProgress
            :phase="createProgress.phase"
            :percent="createProgressPercent"
            :error="createProgress.error"
          />
        </div>
      </div>
    </div>

    <div v-else-if="containerInfo" class="space-y-5">
      <div
        v-if="isLegacy"
        class="flex items-center justify-between gap-3 rounded-md border border-amber-300/50 bg-amber-50/70 p-3 dark:border-amber-800/50 dark:bg-amber-900/10"
      >
        <p class="text-xs text-amber-800 dark:text-amber-200">
          {{ $t("bots.container.legacyWarning") }}
        </p>
        <Button
          variant="outline"
          size="sm"
          class="shrink-0"
          :disabled="containerBusy || botLifecyclePending"
          @click="handleRecreateContainer"
        >
          <Spinner v-if="containerAction === 'recreate'" class="mr-1.5" />
          {{ $t("bots.container.legacyRecreate") }}
        </Button>
      </div>

      <div
        v-if="createProgress && containerAction === 'recreate'"
        class="space-y-2 rounded-md border p-3"
      >
        <ContainerCreateProgress
          :phase="createProgress.phase"
          :percent="createProgressPercent"
          :error="createProgress.error"
        />
      </div>

      <div class="rounded-md border p-4">
        <dl class="grid grid-cols-1 gap-3 text-xs sm:grid-cols-2">
          <div class="space-y-1">
            <dt class="text-muted-foreground">
              {{ $t("bots.container.fields.id") }}
            </dt>
            <dd class="break-all font-mono">
              {{ containerInfo.container_id }}
            </dd>
          </div>
          <div class="space-y-1">
            <dt class="text-muted-foreground">
              {{ $t("bots.container.fields.status") }}
            </dt>
            <dd>{{ containerStatusText }}</dd>
          </div>
          <div class="space-y-1">
            <dt class="text-muted-foreground">
              {{ $t("bots.container.fields.task") }}
            </dt>
            <dd>{{ containerTaskText }}</dd>
          </div>
          <div class="space-y-1">
            <dt class="text-muted-foreground">
              {{ $t("bots.container.fields.namespace") }}
            </dt>
            <dd>{{ containerInfo.namespace }}</dd>
          </div>
          <div class="space-y-1 sm:col-span-2">
            <dt class="text-muted-foreground">
              {{ $t("bots.container.fields.image") }}
            </dt>
            <dd class="break-all">
              {{ displayedContainerImage }}
            </dd>
          </div>
          <div class="space-y-1 sm:col-span-2">
            <dt class="text-muted-foreground">
              {{ $t("bots.container.fields.cdiDevices") }}
            </dt>
            <dd v-if="displayedCDIDevices.length === 0" class="text-muted-foreground">
              {{ $t("bots.container.cdiDevicesEmpty") }}
            </dd>
            <dd v-else class="space-y-1 font-mono text-xs">
              <div v-for="device in displayedCDIDevices" :key="device" class="break-all">
                {{ device }}
              </div>
            </dd>
          </div>
          <div class="space-y-1 sm:col-span-2">
            <dt class="text-muted-foreground">
              {{ $t("bots.container.fields.containerPath") }}
            </dt>
            <dd class="break-all">
              {{ containerInfo.container_path }}
            </dd>
          </div>
          <div class="space-y-1">
            <dt class="text-muted-foreground">
              {{ $t("bots.container.fields.preservedData") }}
            </dt>
            <dd>{{ preservedDataText }}</dd>
          </div>
          <div class="space-y-1">
            <dt class="text-muted-foreground">
              {{ $t("bots.container.fields.createdAt") }}
            </dt>
            <dd>{{ formatDate(containerInfo.created_at) }}</dd>
          </div>
          <div class="space-y-1">
            <dt class="text-muted-foreground">
              {{ $t("bots.container.fields.updatedAt") }}
            </dt>
            <dd>{{ formatDate(containerInfo.updated_at) }}</dd>
          </div>
        </dl>
      </div>

      <ContainerMetricsPanel
        :backend="capabilitiesStore.containerBackend"
        :loading="metricsLoading"
        :metrics="containerMetrics"
      />

      <div class="rounded-md border px-3 py-2 text-xs text-muted-foreground">
        {{ $t("bots.container.gpuRecreateHint") }}
      </div>

      <div class="space-y-4 rounded-md border p-4">
        <div class="space-y-1">
          <h4 class="text-xs font-medium">
            {{ $t("bots.container.dataTitle") }}
          </h4>
          <p class="text-xs text-muted-foreground">
            {{ $t("bots.container.dataSubtitle") }}
          </p>
        </div>

        <div
          v-if="hasPreservedData"
          class="rounded-md border border-primary/20 bg-primary/5 px-3 py-2 text-xs"
        >
          {{ $t("bots.container.preservedDataAvailable") }}
        </div>

        <div class="flex flex-wrap gap-2">
          <Button
            variant="outline"
            :disabled="containerBusy || botLifecyclePending"
            @click="handleExportData"
          >
            <Spinner v-if="containerAction === 'export'" class="mr-1.5" />
            {{ $t("bots.container.actions.exportData") }}
          </Button>
          <Button
            variant="outline"
            :disabled="containerBusy || botLifecyclePending"
            @click="triggerImportData"
          >
            <Spinner v-if="containerAction === 'import'" class="mr-1.5" />
            {{ $t("bots.container.actions.importData") }}
          </Button>
          <ConfirmPopover
            :message="$t('bots.container.restoreConfirm')"
            :loading="containerAction === 'restore'"
            @confirm="handleRestorePreservedData"
          >
            <template #trigger>
              <Button
                variant="outline"
                :disabled="containerBusy || botLifecyclePending || !hasPreservedData"
              >
                <Spinner v-if="containerAction === 'restore'" class="mr-1.5" />
                {{ $t("bots.container.actions.restoreData") }}
              </Button>
            </template>
          </ConfirmPopover>
        </div>

        <input
          ref="importInputRef"
          type="file"
          accept=".tar.gz,.tgz,application/gzip,application/x-gzip,application/x-tar"
          class="hidden"
          @change="handleImportData"
        />
        <Separator />
        <div class="space-y-3">
          <div class="space-y-1">
            <h4 class="text-xs font-medium text-destructive">
              {{ $t("bots.container.deleteTitle") }}
            </h4>
            <p class="text-xs text-muted-foreground">
              {{ $t("bots.container.deleteSubtitle") }}
            </p>
          </div>

          <div class="flex flex-wrap gap-2">
            <ConfirmPopover
              :message="$t('bots.container.deletePreserveConfirm')"
              :loading="containerAction === 'delete-preserve'"
              @confirm="handleDeleteContainer(true)"
            >
              <template #trigger>
                <Button variant="outline" :disabled="containerBusy || botLifecyclePending">
                  <Spinner v-if="containerAction === 'delete-preserve'" class="mr-1.5" />
                  {{ $t("bots.container.actions.deletePreserve") }}
                </Button>
              </template>
            </ConfirmPopover>

            <ConfirmPopover
              :message="$t('bots.container.deleteConfirm')"
              :loading="containerAction === 'delete'"
              @confirm="handleDeleteContainer(false)"
            >
              <template #trigger>
                <Button variant="destructive" :disabled="containerBusy || botLifecyclePending">
                  <Spinner v-if="containerAction === 'delete'" class="mr-1.5" />
                  {{ $t("bots.container.actions.delete") }}
                </Button>
              </template>
            </ConfirmPopover>
          </div>
        </div>
      </div>

      <Separator v-if="capabilitiesStore.snapshotSupported" />

      <div v-if="capabilitiesStore.snapshotSupported" class="space-y-3">
        <div class="space-y-2">
          <div class="flex flex-col gap-2 sm:flex-row">
            <Input
              v-model="newSnapshotName"
              :placeholder="$t('bots.container.snapshotNamePlaceholder')"
              :disabled="containerBusy || snapshotsLoading || botLifecyclePending"
              class="sm:max-w-72"
            />
            <Button
              :disabled="containerBusy || snapshotsLoading || botLifecyclePending"
              @click="handleCreateSnapshot"
            >
              <Spinner v-if="containerAction === 'snapshot'" class="mr-1.5" />
              {{ $t("bots.container.actions.snapshot") }}
            </Button>
          </div>
          <p class="text-xs text-muted-foreground">
            {{ $t("bots.container.snapshotNameHint") }}
          </p>
        </div>

        <div v-if="snapshotsLoading" class="flex items-center gap-2 text-xs text-muted-foreground">
          <Spinner />
          <span>{{ $t("common.loading") }}</span>
        </div>
        <div v-else-if="sortedSnapshots.length === 0" class="text-xs text-muted-foreground">
          {{ $t("bots.container.snapshotEmpty") }}
        </div>
        <div v-else class="space-y-3">
          <div class="space-y-3 md:hidden">
            <div
              v-for="item in sortedSnapshots"
              :key="`${item.snapshotter}:${item.runtime_snapshot_name || item.name}`"
              class="rounded-md border p-4 space-y-4"
            >
              <div class="space-y-1">
                <p class="text-xs text-muted-foreground">
                  {{ $t("bots.container.snapshotColumns.name") }}
                </p>
                <div class="break-all font-medium">
                  {{ snapshotDisplayName(item) }}
                </div>
                <div
                  v-if="snapshotRuntimeName(item)"
                  class="break-all font-mono text-xs text-muted-foreground"
                >
                  {{ snapshotRuntimeName(item) }}
                </div>
              </div>

              <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
                <div class="space-y-1">
                  <p class="text-xs text-muted-foreground">
                    {{ $t("bots.container.snapshotColumns.version") }}
                  </p>
                  <div>{{ snapshotVersionText(item) }}</div>
                </div>
                <div class="space-y-1">
                  <p class="text-xs text-muted-foreground">
                    {{ $t("bots.container.snapshotColumns.source") }}
                  </p>
                  <div>{{ snapshotSourceText(item) }}</div>
                </div>
                <div class="space-y-1">
                  <p class="text-xs text-muted-foreground">
                    {{ $t("bots.container.snapshotColumns.parent") }}
                  </p>
                  <div class="break-all">
                    {{ item.parent || "-" }}
                  </div>
                </div>
                <div class="space-y-1">
                  <p class="text-xs text-muted-foreground">
                    {{ $t("bots.container.snapshotColumns.createdAt") }}
                  </p>
                  <div>{{ formatDate(item.created_at) }}</div>
                </div>
              </div>

              <div class="space-y-1">
                <p class="text-xs text-muted-foreground">
                  {{ $t("bots.container.snapshotColumns.actions") }}
                </p>
                <ConfirmPopover
                  v-if="canRollbackSnapshot(item)"
                  :message="$t('bots.container.rollbackConfirm')"
                  :loading="containerAction === 'rollback' && rollbackVersion === item.version"
                  @confirm="handleRollbackSnapshot(item)"
                >
                  <template #trigger>
                    <Button
                      variant="outline"
                      size="sm"
                      class="w-full"
                      :disabled="containerBusy || botLifecyclePending"
                    >
                      <Spinner
                        v-if="containerAction === 'rollback' && rollbackVersion === item.version"
                        class="mr-1.5"
                      />
                      {{ $t("bots.container.actions.rollback") }}
                    </Button>
                  </template>
                </ConfirmPopover>
                <div v-else class="text-xs text-muted-foreground">-</div>
              </div>
            </div>
          </div>

          <div class="hidden overflow-x-auto rounded-md border md:block">
            <table class="w-full text-xs">
              <thead class="bg-muted/50 text-left">
                <tr>
                  <th class="px-3 py-2 font-medium">
                    {{ $t("bots.container.snapshotColumns.name") }}
                  </th>
                  <th class="px-3 py-2 font-medium">
                    {{ $t("bots.container.snapshotColumns.version") }}
                  </th>
                  <th class="px-3 py-2 font-medium">
                    {{ $t("bots.container.snapshotColumns.source") }}
                  </th>
                  <th class="px-3 py-2 font-medium">
                    {{ $t("bots.container.snapshotColumns.parent") }}
                  </th>
                  <th class="px-3 py-2 font-medium">
                    {{ $t("bots.container.snapshotColumns.createdAt") }}
                  </th>
                  <th class="px-3 py-2 font-medium">
                    {{ $t("bots.container.snapshotColumns.actions") }}
                  </th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="item in sortedSnapshots"
                  :key="`${item.snapshotter}:${item.runtime_snapshot_name || item.name}`"
                  class="border-t align-top"
                >
                  <td class="px-3 py-2">
                    <div class="space-y-1">
                      <div class="break-all font-medium">
                        {{ snapshotDisplayName(item) }}
                      </div>
                      <div
                        v-if="snapshotRuntimeName(item)"
                        class="break-all font-mono text-xs text-muted-foreground"
                      >
                        {{ snapshotRuntimeName(item) }}
                      </div>
                    </div>
                  </td>
                  <td class="px-3 py-2">
                    {{ snapshotVersionText(item) }}
                  </td>
                  <td class="px-3 py-2">
                    {{ snapshotSourceText(item) }}
                  </td>
                  <td class="px-3 py-2 break-all">
                    {{ item.parent || "-" }}
                  </td>
                  <td class="px-3 py-2">
                    {{ formatDate(item.created_at) }}
                  </td>
                  <td class="px-3 py-2">
                    <ConfirmPopover
                      v-if="canRollbackSnapshot(item)"
                      :message="$t('bots.container.rollbackConfirm')"
                      :loading="containerAction === 'rollback' && rollbackVersion === item.version"
                      @confirm="handleRollbackSnapshot(item)"
                    >
                      <template #trigger>
                        <Button
                          variant="outline"
                          size="sm"
                          :disabled="containerBusy || botLifecyclePending"
                        >
                          <Spinner
                            v-if="
                              containerAction === 'rollback' && rollbackVersion === item.version
                            "
                            class="mr-1.5"
                          />
                          {{ $t("bots.container.actions.rollback") }}
                        </Button>
                      </template>
                    </ConfirmPopover>
                    <span v-else class="text-muted-foreground"> - </span>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
