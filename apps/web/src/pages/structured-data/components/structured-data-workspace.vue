<template>
  <section class="space-y-4">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <div class="min-w-0">
        <h3 class="text-sm font-medium">{{ $t("structuredData.title") }}</h3>
        <p class="text-xs text-muted-foreground">{{ selectedSpace?.schemaName || "-" }}</p>
      </div>
      <div class="flex items-center gap-2">
        <Select v-model="selectedSpaceId">
          <SelectTrigger class="w-72">
            <SelectValue :placeholder="$t('structuredData.selectSpace')" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem v-for="space in spaces" :key="space.id" :value="space.id">
              {{ spaceLabel(space) }}
            </SelectItem>
          </SelectContent>
        </Select>
        <Button variant="outline" size="sm" :disabled="loading" @click="loadSpaces">
          <RefreshCw class="mr-1.5 size-3.5" />
          {{ $t("common.refresh") }}
        </Button>
      </div>
    </div>

    <div v-if="loading" class="py-8 text-center text-sm text-muted-foreground">
      {{ $t("common.loading") }}
    </div>
    <div
      v-else-if="spaces.length === 0"
      class="rounded-md border p-6 text-center text-sm text-muted-foreground"
    >
      {{ $t("structuredData.empty") }}
    </div>

    <template v-else>
      <div class="grid gap-4 xl:grid-cols-[1fr_360px]">
        <div class="space-y-3 rounded-md border p-4">
          <div class="flex items-center justify-between gap-3">
            <Label>{{ $t("structuredData.sql") }}</Label>
            <Button size="sm" :disabled="!canExecute || executing" @click="executeSql">
              <Spinner v-if="executing" class="mr-1.5" />
              <Play class="mr-1.5 size-3.5" v-else />
              {{ $t("structuredData.run") }}
            </Button>
          </div>
          <Textarea v-model="sqlText" class="min-h-40 font-mono text-xs" spellcheck="false" />
          <div class="grid gap-3 md:grid-cols-[160px_1fr]">
            <div class="space-y-1">
              <Label>{{ $t("structuredData.maxRows") }}</Label>
              <Input v-model.number="maxRows" type="number" min="1" max="5000" />
            </div>
            <div class="space-y-1">
              <Label>{{ $t("structuredData.commandTag") }}</Label>
              <Input :model-value="lastResult?.commandTag || ''" readonly />
            </div>
          </div>
        </div>

        <div class="space-y-3 rounded-md border p-4">
          <div class="flex items-center justify-between">
            <Label>{{ $t("structuredData.grants") }}</Label>
            <Button
              size="sm"
              variant="outline"
              :disabled="!selectedSpaceId || savingGrant"
              @click="saveGrant"
            >
              <Spinner v-if="savingGrant" class="mr-1.5" />
              <Share2 class="mr-1.5 size-3.5" v-else />
              {{ $t("common.save") }}
            </Button>
          </div>
          <Select v-model="grantForm.targetType">
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="bot">{{ $t("structuredData.targetBot") }}</SelectItem>
              <SelectItem value="bot_group">{{ $t("structuredData.targetBotGroup") }}</SelectItem>
            </SelectContent>
          </Select>
          <Input v-model="grantForm.targetId" :placeholder="$t('structuredData.targetId')" />
          <div class="flex flex-wrap gap-3">
            <label
              v-for="privilege in privilegeOptions"
              :key="privilege"
              class="flex items-center gap-1.5 text-xs"
            >
              <Checkbox
                :model-value="grantForm.privileges.includes(privilege)"
                @update:model-value="(checked: boolean) => togglePrivilege(privilege, checked)"
              />
              {{ $t(`structuredData.privileges.${privilege}`) }}
            </label>
          </div>
          <div class="overflow-hidden rounded-md border">
            <table class="w-full text-xs">
              <thead class="bg-muted/50 text-left">
                <tr>
                  <th class="px-2 py-2 font-medium">{{ $t("structuredData.target") }}</th>
                  <th class="px-2 py-2 font-medium">{{ $t("structuredData.privilegesTitle") }}</th>
                  <th class="px-2 py-2 text-right font-medium">{{ $t("common.operation") }}</th>
                </tr>
              </thead>
              <tbody>
                <tr v-if="grants.length === 0">
                  <td colspan="3" class="px-2 py-5 text-center text-muted-foreground">
                    {{ $t("structuredData.grantsEmpty") }}
                  </td>
                </tr>
                <tr v-for="grant in grants" :key="grant.id" class="border-t">
                  <td class="px-2 py-2">
                    {{ grant.targetType }}: {{ grant.targetBotId || grant.targetBotGroupId }}
                  </td>
                  <td class="px-2 py-2">{{ grant.privileges.join(", ") }}</td>
                  <td class="px-2 py-2 text-right">
                    <Button variant="ghost" size="sm" @click="deleteGrant(grant.id)">
                      <Trash2 class="size-3.5" />
                    </Button>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>

      <div class="grid gap-4 xl:grid-cols-2">
        <div class="overflow-hidden rounded-md border">
          <div class="border-b bg-muted/30 px-3 py-2 text-xs font-medium">
            {{ $t("structuredData.tables") }}
          </div>
          <table class="w-full text-xs">
            <tbody>
              <tr v-if="tables.length === 0">
                <td class="px-3 py-6 text-center text-muted-foreground">
                  {{ $t("structuredData.tablesEmpty") }}
                </td>
              </tr>
              <tr v-for="table in tables" :key="table.name" class="border-t">
                <td class="px-3 py-2 align-top font-medium">{{ table.name }}</td>
                <td class="px-3 py-2 text-muted-foreground">
                  {{ table.columns.map((column) => `${column.name} ${column.type}`).join(", ") }}
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        <div class="overflow-hidden rounded-md border">
          <div class="border-b bg-muted/30 px-3 py-2 text-xs font-medium">
            {{ $t("structuredData.result") }}
          </div>
          <div v-if="!lastResult" class="px-3 py-6 text-center text-xs text-muted-foreground">
            {{ $t("structuredData.resultEmpty") }}
          </div>
          <div
            v-else-if="lastResult.columns.length === 0"
            class="px-3 py-6 text-xs text-muted-foreground"
          >
            {{ $t("structuredData.affectedRows", { count: lastResult.rowCount }) }}
          </div>
          <div v-else class="max-h-96 overflow-auto">
            <table class="w-full text-xs">
              <thead class="sticky top-0 bg-muted/80 text-left">
                <tr>
                  <th
                    v-for="column in lastResult.columns"
                    :key="column"
                    class="px-3 py-2 font-medium"
                  >
                    {{ column }}
                  </th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="(row, index) in lastResult.rows" :key="index" class="border-t">
                  <td
                    v-for="column in lastResult.columns"
                    :key="column"
                    class="px-3 py-2 font-mono"
                  >
                    {{ formatCell(row[column]) }}
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </template>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from "vue";
import { toast } from "vue-sonner";
import { useI18n } from "vue-i18n";
import type {
  StructuredDataGrant,
  StructuredDataSpace,
  StructuredDataSqlResult,
  StructuredDataTable,
} from "@stringke/sdk/connect";
import {
  Button,
  Checkbox,
  Input,
  Label,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Spinner,
  Textarea,
} from "@stringke/ui";
import { Play, RefreshCw, Share2, Trash2 } from "lucide-vue-next";
import { connectClients } from "@/lib/connect-client";
import { resolveApiErrorMessage } from "@/utils/api-error";

const props = defineProps<{
  ownerType?: "bot" | "bot_group";
  ownerBotId?: string;
  ownerBotGroupId?: string;
}>();

const { t: $t } = useI18n();
const privilegeOptions = ["read", "write", "ddl"];
const loading = ref(false);
const executing = ref(false);
const savingGrant = ref(false);
const spaces = ref<StructuredDataSpace[]>([]);
const tables = ref<StructuredDataTable[]>([]);
const grants = ref<StructuredDataGrant[]>([]);
const selectedSpaceId = ref("");
const sqlText = ref("select now() as server_time;");
const maxRows = ref(500);
const lastResult = ref<StructuredDataSqlResult | null>(null);

const grantForm = reactive({
  targetType: "bot",
  targetId: "",
  privileges: ["read"],
});

const selectedSpace = computed(
  () => spaces.value.find((space) => space.id === selectedSpaceId.value) ?? null,
);
const canExecute = computed(() => selectedSpaceId.value !== "" && sqlText.value.trim() !== "");

watch(selectedSpaceId, async (spaceId) => {
  if (!spaceId) return;
  await loadSpaceDetail(spaceId);
});

onMounted(() => {
  void loadSpaces();
});

async function loadSpaces() {
  loading.value = true;
  try {
    const response = await connectClients.structuredData.listStructuredDataSpaces({
      ownerType: props.ownerType ?? "",
      ownerBotId: props.ownerBotId ?? "",
      ownerBotGroupId: props.ownerBotGroupId ?? "",
    });
    spaces.value = response.spaces;
    if (!spaces.value.some((space) => space.id === selectedSpaceId.value)) {
      selectedSpaceId.value = spaces.value[0]?.id ?? "";
    }
    if (!selectedSpaceId.value) {
      tables.value = [];
      grants.value = [];
    }
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, $t("structuredData.loadFailed")));
  } finally {
    loading.value = false;
  }
}

async function loadSpaceDetail(spaceId: string) {
  try {
    const [detail, grantData] = await Promise.all([
      connectClients.structuredData.describeStructuredDataSpace({ spaceId }),
      connectClients.structuredData.listStructuredDataGrants({ spaceId }),
    ]);
    tables.value = detail.tables;
    grants.value = grantData.grants;
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, $t("structuredData.loadFailed")));
  }
}

async function executeSql() {
  if (!canExecute.value) return;
  executing.value = true;
  try {
    const response = await connectClients.structuredData.executeStructuredDataSql({
      spaceId: selectedSpaceId.value,
      sql: sqlText.value,
      maxRows: Number(maxRows.value) || 500,
    });
    lastResult.value = response.result ?? null;
    await loadSpaceDetail(selectedSpaceId.value);
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, $t("structuredData.executeFailed")));
  } finally {
    executing.value = false;
  }
}

async function saveGrant() {
  if (!selectedSpaceId.value || !grantForm.targetId.trim() || grantForm.privileges.length === 0)
    return;
  savingGrant.value = true;
  try {
    await connectClients.structuredData.upsertStructuredDataGrant({
      spaceId: selectedSpaceId.value,
      targetType: grantForm.targetType,
      targetBotId: grantForm.targetType === "bot" ? grantForm.targetId.trim() : "",
      targetBotGroupId: grantForm.targetType === "bot_group" ? grantForm.targetId.trim() : "",
      privileges: grantForm.privileges,
    });
    grantForm.targetId = "";
    await loadSpaceDetail(selectedSpaceId.value);
    toast.success($t("structuredData.grantSaved"));
  } catch (error) {
    toast.error(resolveApiErrorMessage(error, $t("common.saveFailed")));
  } finally {
    savingGrant.value = false;
  }
}

async function deleteGrant(grantId: string) {
  await connectClients.structuredData.deleteStructuredDataGrant({ grantId });
  await loadSpaceDetail(selectedSpaceId.value);
}

function togglePrivilege(privilege: string, checked: boolean) {
  if (checked && !grantForm.privileges.includes(privilege)) {
    grantForm.privileges.push(privilege);
  }
  if (!checked) {
    grantForm.privileges = grantForm.privileges.filter((item) => item !== privilege);
  }
}

function spaceLabel(space: StructuredDataSpace): string {
  const owner = space.ownerBotId || space.ownerBotGroupId;
  return `${space.ownerType}: ${owner}`;
}

function formatCell(value: unknown): string {
  if (value === null || value === undefined) return "";
  if (typeof value === "object") return JSON.stringify(value);
  return String(value);
}
</script>
