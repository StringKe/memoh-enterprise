<template>
  <div class="mx-auto max-w-7xl space-y-6 p-6">
    <div class="flex items-center justify-between">
      <h1 class="text-2xl font-semibold tracking-tight">
        {{ $t("usage.title") }}
      </h1>
      <Button
        variant="outline"
        size="sm"
        :disabled="isLoading || !selectedBotId"
        @click="refreshAll()"
      >
        <Spinner v-if="isLoading" class="mr-2 size-4" />
        {{ $t("common.refresh") }}
      </Button>
    </div>

    <div class="flex flex-wrap items-end gap-4">
      <div class="space-y-1.5">
        <Label>{{ $t("usage.selectBot") }}</Label>
        <BotSelect
          v-model="selectedBotId"
          trigger-class="w-56"
          :placeholder="$t('usage.selectBotPlaceholder')"
        />
      </div>

      <div class="space-y-1.5">
        <Label>{{ $t("usage.timeRange") }}</Label>
        <Select v-model="timeRange">
          <SelectTrigger class="w-40">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="7">
              {{ $t("usage.last7Days") }}
            </SelectItem>
            <SelectItem value="30">
              {{ $t("usage.last30Days") }}
            </SelectItem>
            <SelectItem value="90">
              {{ $t("usage.last90Days") }}
            </SelectItem>
          </SelectContent>
        </Select>
      </div>

      <div class="space-y-1.5">
        <Label>{{ $t("usage.dateFrom") }}</Label>
        <Input v-model="dateFrom" type="date" class="w-40" />
      </div>
      <div class="space-y-1.5">
        <Label>{{ $t("usage.dateTo") }}</Label>
        <Input v-model="dateTo" type="date" class="w-40" />
      </div>
    </div>

    <template v-if="!selectedBotId">
      <div class="text-muted-foreground flex min-h-[60vh] items-center justify-center">
        {{ $t("usage.selectBotPlaceholder") }}
      </div>
    </template>

    <template v-else-if="isLoading">
      <div class="flex min-h-[60vh] items-center justify-center">
        <Spinner class="size-8" />
      </div>
    </template>

    <template v-else>
      <div class="grid grid-cols-2 gap-4 lg:grid-cols-4">
        <Card>
          <CardHeader class="pb-2">
            <CardDescription>{{ $t("usage.totalInputTokens") }}</CardDescription>
          </CardHeader>
          <CardContent>
            <p class="text-2xl font-bold tabular-nums">
              {{ formatNumber(summary.promptTokens) }}
            </p>
          </CardContent>
        </Card>
        <Card>
          <CardHeader class="pb-2">
            <CardDescription>{{ $t("usage.totalOutputTokens") }}</CardDescription>
          </CardHeader>
          <CardContent>
            <p class="text-2xl font-bold tabular-nums">
              {{ formatNumber(summary.completionTokens) }}
            </p>
          </CardContent>
        </Card>
        <Card>
          <CardHeader class="pb-2">
            <CardDescription>{{ $t("usage.tokens") }}</CardDescription>
          </CardHeader>
          <CardContent>
            <p class="text-2xl font-bold tabular-nums">
              {{ formatNumber(summary.totalTokens) }}
            </p>
          </CardContent>
        </Card>
        <Card>
          <CardHeader class="pb-2">
            <CardDescription>{{ $t("usage.currency") }}</CardDescription>
          </CardHeader>
          <CardContent>
            <p class="text-2xl font-bold tabular-nums">
              {{ summary.currency || "-" }}
            </p>
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader class="flex flex-row items-center justify-between pb-2">
          <CardTitle class="text-sm">
            {{ $t("usage.records") }}
          </CardTitle>
          <span v-if="recordsPaginationSummary" class="text-muted-foreground text-xs tabular-nums">
            {{ recordsPaginationSummary }}
          </span>
        </CardHeader>
        <CardContent class="space-y-3">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{{ $t("usage.colTime") }}</TableHead>
                <TableHead>{{ $t("usage.colBot") }}</TableHead>
                <TableHead>{{ $t("usage.colSessionType") }}</TableHead>
                <TableHead>{{ $t("usage.colModel") }}</TableHead>
                <TableHead>{{ $t("usage.colProvider") }}</TableHead>
                <TableHead class="text-right">
                  {{ $t("usage.colInputTokens") }}
                </TableHead>
                <TableHead class="text-right">
                  {{ $t("usage.colOutputTokens") }}
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-if="isRecordsInitialLoading">
                <TableCell :colspan="7" class="p-0">
                  <div class="flex h-[480px] items-center justify-center">
                    <Spinner class="size-6" />
                  </div>
                </TableCell>
              </TableRow>
              <TableRow v-else-if="recordsList.length === 0">
                <TableCell :colspan="7" class="p-0">
                  <div class="text-muted-foreground flex h-[480px] items-center justify-center">
                    {{ $t("usage.noRecords") }}
                  </div>
                </TableCell>
              </TableRow>
              <template v-else>
                <TableRow
                  v-for="r in recordsList"
                  :key="r.id"
                  :class="
                    isRecordsFetching ? 'opacity-60 transition-opacity' : 'transition-opacity'
                  "
                >
                  <TableCell class="text-muted-foreground tabular-nums">
                    {{ formatRecordTime(r) }}
                  </TableCell>
                  <TableCell>{{ selectedBotName }}</TableCell>
                  <TableCell>{{
                    sessionTypeLabel(recordMetadataString(r, "session_type"))
                  }}</TableCell>
                  <TableCell>{{ recordModelLabel(r) }}</TableCell>
                  <TableCell class="text-muted-foreground">
                    {{ recordMetadataString(r, "provider_name") || "-" }}
                  </TableCell>
                  <TableCell class="text-right tabular-nums">
                    {{ formatNumber(toNumber(r.promptTokens)) }}
                  </TableCell>
                  <TableCell class="text-right tabular-nums">
                    {{ formatNumber(toNumber(r.completionTokens)) }}
                  </TableCell>
                </TableRow>
              </template>
            </TableBody>
          </Table>

          <div class="flex justify-end gap-2">
            <Button
              variant="outline"
              size="sm"
              :disabled="recordsPageNumber <= 1 || isRecordsFetching"
              @click="setRecordsPage(recordsPageNumber - 1)"
            >
              {{ $t("usage.previousPage") }}
            </Button>
            <Button
              variant="outline"
              size="sm"
              :disabled="!recordsNextPageToken || isRecordsFetching"
              @click="setRecordsPage(recordsPageNumber + 1)"
            >
              {{ $t("usage.nextPage") }}
            </Button>
          </div>
        </CardContent>
      </Card>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, watch } from "vue";
import { useI18n } from "vue-i18n";
import { useQuery } from "@pinia/colada";
import {
  Button,
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  Input,
  Label,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Spinner,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@stringke/ui";
import { createTimestampFromDate, type TokenUsageRecord } from "@stringke/sdk/connect";
import BotSelect from "@/components/bot-select/index.vue";
import { connectClients } from "@/lib/connect-client";
import { useSyncedQueryParam } from "@/composables/useSyncedQueryParam";
import { formatDateTimeSeconds } from "@/utils/date-time";

const { t } = useI18n();

const selectedBotId = useSyncedQueryParam("bot", "");
const timeRange = useSyncedQueryParam("range", "7");
const recordsPage = useSyncedQueryParam("rpage", "1");

const RECORDS_PAGE_SIZE = 20;

function daysAgo(days: number): string {
  const d = new Date();
  d.setDate(d.getDate() - days + 1);
  return formatDate(d);
}

function tomorrow(): string {
  const d = new Date();
  d.setDate(d.getDate() + 1);
  return formatDate(d);
}

const initDays = parseInt(timeRange.value, 10) || 30;
const dateFrom = useSyncedQueryParam("from", daysAgo(initDays));
const dateTo = useSyncedQueryParam("to", tomorrow());

watch(timeRange, (val) => {
  const days = parseInt(val, 10);
  if (days > 0) {
    dateFrom.value = daysAgo(days);
    dateTo.value = tomorrow();
  }
});

const { data: botData } = useQuery({
  key: () => ["connect-bots-for-usage"],
  query: () => connectClients.bots.listBots({}),
});
const botList = computed(() => botData.value?.bots ?? []);

watch(
  botList,
  (list) => {
    if (!selectedBotId.value && list.length > 0 && list[0]!.id) {
      selectedBotId.value = list[0]!.id;
    }
  },
  { immediate: true },
);

const dateRange = computed(() => ({
  from: createTimestampFromDate(new Date(`${dateFrom.value}T00:00:00`)),
  to: createTimestampFromDate(new Date(`${dateTo.value}T00:00:00`)),
}));

const {
  data: usageData,
  asyncStatus,
  refetch,
} = useQuery({
  key: () => ["connect-token-usage", selectedBotId.value, dateFrom.value, dateTo.value],
  query: () =>
    connectClients.usage.getTokenUsage({
      botId: selectedBotId.value,
      from: dateRange.value.from,
      to: dateRange.value.to,
    }),
  enabled: () => !!selectedBotId.value,
});

const isFetching = computed(() => asyncStatus.value === "loading");
const isLoading = computed(() => isFetching.value && !usageData.value);

const recordsPageNumber = computed(() => {
  const parsed = parseInt(recordsPage.value, 10);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : 1;
});

const {
  data: recordsData,
  asyncStatus: recordsAsyncStatus,
  refetch: refetchRecords,
} = useQuery({
  key: () => [
    "connect-token-usage-records",
    selectedBotId.value,
    dateFrom.value,
    dateTo.value,
    recordsPageNumber.value,
  ],
  query: () =>
    connectClients.usage.listTokenUsageRecords({
      botId: selectedBotId.value,
      from: dateRange.value.from,
      to: dateRange.value.to,
      page: {
        pageSize: RECORDS_PAGE_SIZE,
        pageToken:
          recordsPageNumber.value > 1
            ? String((recordsPageNumber.value - 1) * RECORDS_PAGE_SIZE)
            : "",
      },
    }),
  enabled: () => !!selectedBotId.value,
});

const recordsList = computed(() => recordsData.value?.records ?? []);
const recordsNextPageToken = computed(() => recordsData.value?.page?.nextPageToken ?? "");
const isRecordsFetching = computed(() => recordsAsyncStatus.value === "loading");
const isRecordsInitialLoading = computed(() => isRecordsFetching.value && !recordsData.value);

const recordsPaginationSummary = computed(() => {
  if (recordsList.value.length === 0) return "";
  const start = (recordsPageNumber.value - 1) * RECORDS_PAGE_SIZE + 1;
  const end = start + recordsList.value.length - 1;
  return `${start}-${end}`;
});

const selectedBotName = computed(() => {
  const bot = botList.value.find((b) => b.id === selectedBotId.value);
  return bot?.displayName || bot?.id || "";
});

const summary = computed(() => {
  const value = usageData.value?.summary;
  return {
    promptTokens: toNumber(value?.promptTokens),
    completionTokens: toNumber(value?.completionTokens),
    totalTokens: toNumber(value?.totalTokens),
    currency: value?.currency ?? "",
  };
});

onMounted(() => {
  if (selectedBotId.value) {
    refreshAll();
  }
});

watch(
  () => [selectedBotId.value, dateFrom.value, dateTo.value],
  () => {
    if (recordsPage.value !== "1") {
      recordsPage.value = "1";
    }
  },
);

function refreshAll() {
  void refetch();
  void refetchRecords();
}

function setRecordsPage(page: number) {
  recordsPage.value = String(Math.max(1, page));
}

function sessionTypeLabel(type: string | undefined): string {
  switch (type) {
    case "chat":
      return t("usage.chat");
    case "heartbeat":
      return t("usage.heartbeat");
    case "schedule":
      return t("usage.schedule");
    default:
      return type || "-";
  }
}

function recordModelLabel(record: TokenUsageRecord): string {
  return (
    recordMetadataString(record, "model_name") ||
    recordMetadataString(record, "model_slug") ||
    record.modelId ||
    "-"
  );
}

function recordMetadataString(record: TokenUsageRecord, key: string): string {
  const value = record.metadata?.[key];
  return typeof value === "string" ? value : "";
}

function formatRecordTime(record: TokenUsageRecord): string {
  if (!record.createdAt) return "-";
  return formatDateTimeSeconds(new Date(Number(record.createdAt.seconds) * 1000).toISOString());
}

function formatDate(d: Date): string {
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${y}-${m}-${day}`;
}

function formatNumber(value: number): string {
  if (value >= 1_000_000) return (value / 1_000_000).toFixed(1) + "M";
  if (value >= 1_000) return (value / 1_000).toFixed(1) + "K";
  return String(value);
}

function toNumber(value: bigint | number | string | undefined): number {
  if (typeof value === "bigint") return Number(value);
  if (typeof value === "number") return value;
  if (typeof value === "string") return Number(value);
  return 0;
}
</script>
